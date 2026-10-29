// SPDX-License-Identifier: GPL-3.0-or-later

package manager

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/aren55555/signal-cli-go/internal/content"
	"github.com/aren55555/signal-cli-go/internal/crypto/curve25519"
	"github.com/aren55555/signal-cli-go/internal/proto/signalpb"
	"github.com/aren55555/signal-cli-go/internal/protocol"
	"github.com/aren55555/signal-cli-go/internal/service/api"
	"github.com/aren55555/signal-cli-go/internal/service/chat"
	"github.com/aren55555/signal-cli-go/internal/service/rest"
	"github.com/aren55555/signal-cli-go/internal/store"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
)

// ChangeNumberError is a failure signal-cli prints as
// "Failed to change number: <Message> (<Class>)" (change-number.md §2.3,
// §3.4, §3.8).
type ChangeNumberError struct{ Message, Class string }

func (e *ChangeNumberError) Error() string { return e.Message + " (" + e.Class + ")" }

// StartChangeNumber is startChangeNumber (change-number.md §2): the
// registration session flow for the new number, authenticated as that
// number with the account's password, with no pre-steps.
func (r *Registrar) StartChangeNumber(ctx context.Context, a *store.Account, number, captcha string, voice bool) error {
	cr := *r
	cr.authNumber = number
	err := cr.requestCode(ctx, a, number, captcha, voice)
	var se *rest.StatusError
	var rl *rest.RateLimitError
	switch {
	case errors.As(err, &rl):
		if rl.After > 0 && rl.Status == 0 {
			return err // local session metadata: Rate limit reached
		}
		return &ChangeNumberError{fmt.Sprintf("[%d] Rate Limited", rl.Status), "RateLimitException"}
	case errors.As(err, &se) && se.Status == 400:
		return &ChangeNumberError{"StatusCode: 400", "InvalidRegistrationSessionIdException"}
	}
	return err
}

// pniMaterial is one device's new PNI key material (§3.2).
type pniMaterial struct {
	device uint32
	signed *protocol.SignedPreKeyRecord
	kyber  *protocol.KyberPreKeyRecord
	regID  uint32
}

// changeNumberRequest is PUT /v2/accounts/number's body; the member
// order and the emitted nulls are the reference's (§3.5).
type changeNumberRequest struct {
	SessionID                    string                             `json:"sessionId"`
	RecoveryPassword             *string                            `json:"recoveryPassword"`
	Number                       string                             `json:"number"`
	PNIIdentityKey               string                             `json:"pniIdentityKey"`
	DeviceMessages               []outgoingMessage                  `json:"deviceMessages"`
	DevicePNISignedPrekeys       map[string]*api.SignedPreKeyEntity `json:"devicePniSignedPrekeys"`
	PNIRegistrationIDs           map[string]uint32                  `json:"pniRegistrationIds"`
	Reglock                      *string                            `json:"reglock"`
	DevicePNIPqLastResortPrekeys map[string]*api.SignedPreKeyEntity `json:"devicePniPqLastResortPrekeys"`
}

// identityKeyPairBytes is the serialized identity key pair (§6.1).
func identityKeyPairBytes(pub, priv []byte) []byte {
	var b []byte
	b = protowire.AppendTag(b, 1, protowire.BytesType)
	b = protowire.AppendBytes(b, pub)
	b = protowire.AppendTag(b, 2, protowire.BytesType)
	return protowire.AppendBytes(b, priv)
}

// parseIdentityKeyPair reads §6.1.
func parseIdentityKeyPair(b []byte) (pub, priv []byte, err error) {
	for len(b) > 0 {
		num, typ, n := protowire.ConsumeTag(b)
		if n < 0 || typ != protowire.BytesType {
			return nil, nil, errors.New("invalid identity key pair")
		}
		b = b[n:]
		v, m := protowire.ConsumeBytes(b)
		if m < 0 {
			return nil, nil, errors.New("invalid identity key pair")
		}
		b = b[m:]
		switch num {
		case 1:
			pub = v
		case 2:
			priv = v
		}
	}
	if len(pub) != 33 || len(priv) != 32 {
		return nil, nil, errors.New("invalid identity key pair")
	}
	return pub, priv, nil
}

// FinishChangeNumber is finishChangeNumber (change-number.md §3-4) on
// the primary: new PNI material for every own device with a session,
// the code, PUT /v2/accounts/number over the authenticated socket, and
// on success the local change. rc is the REST client for the code.
func (s *Sender) FinishChangeNumber(ctx context.Context, rc *rest.Client, number, code string) error {
	a := s.Acct
	rd, now := s.rand(), s.now()
	// §3.2: identity and per-device material, before the session check.
	identity, err := curve25519.GeneratePrivateKey(rd)
	if err != nil {
		return err
	}
	pairBytes := identityKeyPairBytes(identity.PublicKey().Bytes(), identity.Bytes())
	recs, err := s.sessions(a.Data.ACI)
	if err != nil {
		return err
	}
	devices := []uint32{1}
	others := slices.Sorted(maps.Keys(recs))
	for _, d := range others {
		if d != 1 && d != uint32(a.Data.DeviceID) {
			devices = append(devices, d)
		}
	}
	var mats []pniMaterial
	used := map[uint32]bool{}
	for _, d := range devices {
		m := pniMaterial{device: d}
		sid, err := randUint32(rd, idSpace)
		if err != nil {
			return err
		}
		kid, err := randUint32(rd, idSpace)
		if err != nil {
			return err
		}
		if m.signed, err = protocol.GenerateSignedPreKey(rd, identity, sid, now); err != nil {
			return err
		}
		if m.kyber, err = protocol.GenerateKyberPreKey(rd, identity, kid, now); err != nil {
			return err
		}
		for m.regID == 0 || used[m.regID] {
			v, err := randUint32(rd, 16380)
			if err != nil {
				return err
			}
			m.regID = v + 1
		}
		used[m.regID] = true
		mats = append(mats, m)
	}
	// §3.3: one sync message per linked device.
	messages := []outgoingMessage{}
	for _, m := range mats[1:] {
		sm := &signalpb.AppSyncMessage{PniChangeNumber: &signalpb.AppSyncMessage_PniChangeNumber{
			IdentityKeyPair: pairBytes, SignedPreKey: m.signed.Serialize(), LastResortKyberPreKey: m.kyber.Serialize(),
			RegistrationId: proto.Uint32(m.regID), NewE164: proto.String(number)}}
		var n [2]byte
		if _, err := io.ReadFull(rd, n[:]); err != nil {
			return err
		}
		sm.Padding = make([]byte, 1+int(binary.BigEndian.Uint16(n[:]))%512)
		if _, err := io.ReadFull(rd, sm.Padding); err != nil {
			return err
		}
		c, err := proto.Marshal(&signalpb.AppContent{Body: &signalpb.AppContent_SyncMessage{SyncMessage: sm}})
		if err != nil {
			return err
		}
		remote := protocol.Address{ServiceID: a.Data.ACI, DeviceID: m.device}
		next, typ, wire, err := recs[m.device].Encrypt(rd, content.Pad(c), now, s.local(), remote)
		if err != nil {
			return err
		}
		envType := envCiphertext
		if typ == protocol.MessageTypePreKey {
			envType = envPreKeyBundle
		}
		messages = append(messages, outgoingMessage{Type: envType, DestinationDeviceID: m.device,
			DestinationRegistrationID: next.RemoteRegistrationID(), Content: base64.StdEncoding.EncodeToString(wire)})
		if err := s.saveSession(a.Data.ACI, m.device, next); err != nil {
			return err
		}
	}
	if len(messages) > 0 {
		if err := a.SaveSessions(); err != nil {
			return err
		}
	}
	// §3.1 step 3: the session.
	if a.Data.SessionID == "" || a.Data.SessionNumber != number {
		return &ChangeNumberError{"No change number verification session active", "IOException"}
	}
	// §3.4: the code.
	user := a.Data.ACI
	var meta sessionMeta
	err = rc.Do(ctx, "PUT", "/v1/verification/session/"+a.Data.SessionID+"/code", &rest.Basic{User: user, Password: a.Data.Password},
		map[string]string{"code": strings.ReplaceAll(code, "-", "")}, &meta)
	if err := codeError(err); err != nil {
		return err
	}
	// §3.5: the request.
	req := changeNumberRequest{SessionID: a.Data.SessionID, Number: number, PNIIdentityKey: api.KeyBase64(identity.PublicKey().Bytes()),
		DeviceMessages: messages, DevicePNISignedPrekeys: map[string]*api.SignedPreKeyEntity{}, PNIRegistrationIDs: map[string]uint32{},
		DevicePNIPqLastResortPrekeys: map[string]*api.SignedPreKeyEntity{}}
	for _, m := range mats {
		k := strconv.Itoa(int(m.device))
		req.DevicePNISignedPrekeys[k] = api.SignedPreKey(m.signed)
		req.DevicePNIPqLastResortPrekeys[k] = api.KyberPreKey(m.kyber)
		req.PNIRegistrationIDs[k] = m.regID
	}
	var body bytes.Buffer
	enc := json.NewEncoder(&body)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(req); err != nil {
		return err
	}
	resp, err := s.Chat.Request(ctx, "PUT", "/v2/accounts/number", []chat.Header{{Name: "content-type", Value: "application/json"}}, bytes.TrimSuffix(body.Bytes(), []byte("\n")))
	if err != nil {
		return &ChangeNumberError{err.Error(), "IOException"}
	}
	switch {
	case resp.Status == 401 || resp.Status == 403:
		return &ChangeNumberError{fmt.Sprintf("[%d] Authorization failed!", resp.Status), "AuthorizationFailedException"}
	case resp.Status == 413 || resp.Status == 429:
		return &ChangeNumberError{fmt.Sprintf("[%d] Rate Limited", resp.Status), "RateLimitException"}
	case resp.Status < 200 || resp.Status > 299:
		return &ChangeNumberError{fmt.Sprintf("[%d] ", resp.Status), "NonSuccessfulResponseCodeException"}
	}
	var out struct {
		PNI string `json:"pni"`
	}
	if err := json.Unmarshal(resp.Body, &out); err != nil {
		return err
	}
	if _, ok := uuidRaw(out.PNI); !ok {
		return errors.New("change number response without a valid pni")
	}
	// §4.
	a.Data.SessionID, a.Data.SessionNumber = "", ""
	if err := a.Save(); err != nil {
		return err
	}
	if out.PNI == a.Data.PNI {
		s.logf("PNI is unchanged after change number")
		return nil
	}
	me := mats[0]
	if err := applyPniChange(a, out.PNI, number, identity.PublicKey().Bytes(), identity.Bytes(), me.regID, me.signed, me.kyber, now.UnixMilli()); err != nil {
		s.logf("Failed to handle change number message: %v", err)
		return nil
	}
	s.cert = nil // the next sealed send fetches a certificate for the new number
	if err := RefreshPreKeys(ctx, a, s.Chat, now); err != nil {
		s.logf("Failed to handle change number message: %v", err)
	}
	return nil
}

// codeError maps the code submission's outcome (§3.4).
func codeError(err error) error {
	var se *rest.StatusError
	var rl *rest.RateLimitError
	switch {
	case err == nil:
		return nil
	case errors.As(err, &rl):
		return &ChangeNumberError{fmt.Sprintf("[%d] Rate Limited", rl.Status), "RateLimitException"}
	case errors.As(err, &se):
		var m sessionMeta
		json.Unmarshal(se.Body, &m)
		switch se.Status {
		case 409:
			if m.Verified {
				return nil
			}
			if m.NextVerificationAttempt == nil {
				return &ChangeNumberError{"StatusCode: 409", "MustRequestNewCodeException"}
			}
			return &ChangeNumberError{"StatusCode: 409", "HttpConflictException"}
		case 400:
			return &ChangeNumberError{"StatusCode: 400", "InvalidTransportModeException"}
		case 404:
			return &ChangeNumberError{"StatusCode: 404", "NoSuchSessionException"}
		case 440:
			return &ChangeNumberError{"StatusCode: 502", "ExternalServiceFailureException"}
		}
		return &ChangeNumberError{se.Error(), "NonSuccessfulResponseCodeException"}
	}
	return &ChangeNumberError{err.Error(), "IOException"}
}

// applyPniChange replaces the account's number, PNI and PNI key material
// (change-number.md §4 step 3, sync.md §6.13).
func applyPniChange(a *store.Account, pni, number string, pub, priv []byte, regID uint32,
	signed *protocol.SignedPreKeyRecord, kyber *protocol.KyberPreKeyRecord, nowMs int64) error {
	d := &a.Data
	if d.PNI != "" {
		delete(a.Identities, "PNI:"+d.PNI)
	}
	d.PNI = pni
	d.PNIIdentity = store.KeyPair{Public: pub, Private: priv}
	d.PNIRegistrationID = regID
	a.PutIdentity("PNI:"+pni, store.Identity{Key: pub, Trust: store.TrustedVerified, Added: nowMs})
	keys := &a.Keys.PNI
	if keys.SignedPreKeys == nil {
		keys.SignedPreKeys = map[uint32]store.StoredKey{}
	}
	if keys.KyberPreKeys == nil {
		keys.KyberPreKeys = map[uint32]store.StoredKey{}
	}
	keys.SignedPreKeys[signed.ID] = store.StoredKey{Record: signed.Serialize()}
	d.PNIKeys.ActiveSignedPreKeyID = int64(signed.ID)
	d.PNIKeys.NextSignedPreKeyID = (signed.ID + 1) % idSpace
	if kyber != nil {
		keys.KyberPreKeys[kyber.ID] = store.StoredKey{Record: kyber.Serialize(), LastResort: true}
		d.PNIKeys.ActiveLastResortKyberID = int64(kyber.ID)
		d.PNIKeys.NextKyberPreKeyID = (kyber.ID + 1) % idSpace
	}
	if err := a.SetNumber(number); err != nil {
		return err
	}
	a.EnsureSelf()
	for _, save := range []func() error{a.SaveKeys, a.SaveIdentities, a.SaveRecipients} {
		if err := save(); err != nil {
			return err
		}
	}
	return nil
}

// applyPniChangeSync applies a received pniChangeNumber on a linked
// device (sync.md §6.13); it reports whether the PNI prekeys need a
// refresh.
func (r *Receiver) applyPniChangeSync(env *signalpb.ServiceEnvelope, sourceDevice uint32, delivered uint64, p *signalpb.AppSyncMessage_PniChangeNumber) bool {
	a := r.Acct
	switch {
	case a.Data.DeviceID == 1:
		r.logf("Received a pni change number message on the primary device, ignoring.")
		return false
	case sourceDevice != 1:
		return false
	case int64(delivered) <= a.Data.LastPniChange:
		return false
	}
	pni := env.GetUpdatedPni()
	if raw := env.GetUpdatedPniBinary(); len(raw) == 16 {
		pni = uuidString(raw)
	}
	if pni == "" || p.IdentityKeyPair == nil || p.RegistrationId == nil || p.SignedPreKey == nil {
		return false
	}
	pub, priv, err := parseIdentityKeyPair(p.IdentityKeyPair)
	if err != nil {
		return false
	}
	signed, err := protocol.LoadSignedPreKeyRecord(p.SignedPreKey)
	if err != nil {
		return false
	}
	var kyber *protocol.KyberPreKeyRecord
	if p.LastResortKyberPreKey != nil {
		if kyber, err = protocol.LoadKyberPreKeyRecord(p.LastResortKyberPreKey); err != nil {
			return false
		}
	}
	number := p.GetNewE164()
	if number == "" {
		number = a.Data.Number
	}
	if err := applyPniChange(a, pni, number, pub, priv, p.GetRegistrationId(), signed, kyber, r.now().UnixMilli()); err != nil {
		r.logf("Failed to handle change number message: %v", err)
		return false
	}
	a.Data.LastPniChange = int64(delivered)
	a.Save()
	return true
}

// SubmitChallenge is submitRateLimitChallenge (rate-limit-challenge.md
// §2): PUT /v1/challenge on the authenticated socket. Errors carry the
// library's message (§2.4).
func (s *Sender) SubmitChallenge(ctx context.Context, token, captcha string) error {
	body, err := json.Marshal(struct {
		Type    string `json:"type"`
		Token   string `json:"token"`
		Captcha string `json:"captcha"`
	}{"captcha", token, strings.ReplaceAll(captcha, "signalcaptcha://", "")})
	if err != nil {
		return err
	}
	resp, err := s.Chat.Request(ctx, "PUT", "/v1/challenge", []chat.Header{{Name: "content-type", Value: "application/json"}}, body)
	switch {
	case err != nil:
		return err
	case resp.Status == 401 || resp.Status == 403:
		return fmt.Errorf("[%d] Authorization failed!", resp.Status)
	case resp.Status == 413 || resp.Status == 429:
		return fmt.Errorf("[%d] Rate Limited", resp.Status)
	case resp.Status < 200 || resp.Status > 299:
		return fmt.Errorf("[%d] ", resp.Status)
	}
	return nil
}
