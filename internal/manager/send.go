// SPDX-License-Identifier: GPL-3.0-or-later

package manager

import (
	"bytes"
	"context"
	"crypto/rand"
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
	"time"

	"github.com/aren55555/signal-cli-go/internal/content"
	"github.com/aren55555/signal-cli-go/internal/crypto/curve25519"
	"github.com/aren55555/signal-cli-go/internal/proto/signalpb"
	"github.com/aren55555/signal-cli-go/internal/protocol"
	"github.com/aren55555/signal-cli-go/internal/service/api"
	"github.com/aren55555/signal-cli-go/internal/service/chat"
	"github.com/aren55555/signal-cli-go/internal/store"
	"google.golang.org/protobuf/proto"
)

// Requester makes chat-socket requests (a *chat.Conn).
type Requester interface {
	Request(ctx context.Context, verb, path string, headers []chat.Header, body []byte) (*chat.Response, error)
}

// Send errors.
var (
	ErrUnregistered      = errors.New("unregistered user")
	ErrNoValidBundles    = errors.New("no valid prekey bundles")
	ErrConflictsUnsolved = errors.New("Failed to resolve conflicts after 4 attempts!")
	errAccessRejected    = errors.New("unidentified access rejected")
)

// SendStatusError is a send answered with an unexpected status.
type SendStatusError struct{ Status int }

func (e *SendStatusError) Error() string { return fmt.Sprintf("send failed with status %d", e.Status) }

var jsonHeader = []chat.Header{{Name: "content-type", Value: "application/json"}}

// Sender sends 1:1 messages, unsealed over the authenticated socket
// (spec/service/send.md; sealed sender comes with profiles).
type Sender struct {
	Acct *store.Account
	Chat Requester
	Rand io.Reader
	Now  func() time.Time

	lastTS uint64

	// Unauth is the unauthenticated socket for sealed sends; nil sends
	// everything unsealed.
	Unauth Requester

	// CDN uploads attachments; nil makes attachment sends fail.
	CDN CDN
	// AvatarsDir is <config>/avatars for profile avatars; TrustNew is the
	// --trust-new-identities mode; Log receives warnings.
	AvatarsDir string
	TrustNew   string
	// DisableSendLog is --disable-send-log: nothing is written to the
	// send log (send-log.md §2.4).
	DisableSendLog bool
	Log            func(format string, args ...any)
	// DialCDSI opens CDSI channels; nil disables number lookups.
	DialCDSI CDSIDialer
	// DialSVR2 opens SVR2 channels; nil disables PIN operations.
	DialSVR2    SVR2Dialer
	cert        []byte
	certExpires uint64
}

func (s *Sender) rand() io.Reader {
	if s.Rand != nil {
		return s.Rand
	}
	return rand.Reader
}

func (s *Sender) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// timestamp is the current time in ms, strictly increasing per Sender
// (send.md §1 step 4).
func (s *Sender) timestamp() uint64 {
	for {
		ts := uint64(s.now().UnixMilli())
		if ts > s.lastTS {
			s.lastTS = ts
			return ts
		}
		time.Sleep(time.Millisecond)
	}
}

// SendText sends a plain text message to an ACI and, as the account is
// multi-device, the sent transcript to our other devices. It returns the
// message timestamp.
func (s *Sender) SendText(ctx context.Context, recipientACI, text string) (uint64, error) {
	return s.SendTextAt(ctx, recipientACI, text, 0, true)
}

// SendTextAt is SendText with a given timestamp (0: a new one), so one
// message to several recipients shares it, and with the urgent flag.
func (s *Sender) SendTextAt(ctx context.Context, recipientACI, text string, ts uint64, urgent bool) (uint64, error) {
	dm := &signalpb.AppDataMessage{}
	if text != "" {
		dm.Body = proto.String(text)
	}
	ts, res := s.SendData(ctx, []string{recipientACI}, DataSend{Message: dm, Urgent: urgent, Timestamp: ts})
	var te errTranscript
	if errors.As(res[0].Err, &te) {
		return ts, nil // the message itself went out
	}
	return ts, res[0].Err
}

// sendSync sends a SyncMessage to our own other devices (sync.md §3):
// random padding, unsealed, content hint IMPLICIT, not online; ts 0 means
// the current wall-clock time. Callers apply the multi-device gate.
func (s *Sender) sendSync(ctx context.Context, sm *signalpb.AppSyncMessage, ts uint64, urgent bool) error {
	var n [2]byte
	if _, err := io.ReadFull(s.rand(), n[:]); err != nil {
		return err
	}
	sm.Padding = make([]byte, 1+int(binary.BigEndian.Uint16(n[:]))%512)
	if _, err := io.ReadFull(s.rand(), sm.Padding); err != nil {
		return err
	}
	c, err := proto.Marshal(&signalpb.AppContent{Body: &signalpb.AppContent_SyncMessage{SyncMessage: sm}})
	if err != nil {
		return err
	}
	if ts == 0 {
		ts = uint64(s.now().UnixMilli())
	}
	_, _, err = s.send(ctx, s.Acct.Data.ACI, c, ts, urgent, protocol.ContentHintImplicit)
	return err
}

// SendSync sends sm if the account is multi-device; otherwise nothing is
// sent and it succeeds (sync.md §3 rule 1).
func (s *Sender) SendSync(ctx context.Context, sm *signalpb.AppSyncMessage, urgent bool) error {
	if !s.Acct.IsMultiDevice() {
		return nil
	}
	return s.sendSync(ctx, sm, 0, urgent)
}

func (s *Sender) local() protocol.Address {
	return protocol.Address{ServiceID: s.Acct.Data.ACI, DeviceID: uint32(s.Acct.Data.DeviceID)}
}

// sessions returns our session records with dest's devices.
func (s *Sender) sessions(dest string) (map[uint32]*protocol.SessionRecord, error) {
	out := map[uint32]*protocol.SessionRecord{}
	prefix := "aci:" + dest + "."
	for k, b := range s.Acct.Sessions {
		rest, ok := strings.CutPrefix(k, prefix)
		if !ok {
			continue
		}
		id, err := strconv.ParseUint(rest, 10, 32)
		if err != nil {
			continue
		}
		rec, err := protocol.LoadSessionRecord(b)
		if err != nil {
			return nil, err
		}
		if rec.HasCurrent() {
			out[uint32(id)] = rec
		}
	}
	return out, nil
}

func (s *Sender) saveSession(dest string, dev uint32, rec *protocol.SessionRecord) error {
	b, err := rec.Serialize()
	if err != nil {
		return err
	}
	s.Acct.Sessions[sessionKey(false, protocol.Address{ServiceID: dest, DeviceID: dev})] = b
	return nil
}

type bundleResponse struct {
	IdentityKey string `json:"identityKey"`
	Devices     []struct {
		DeviceID       int64                   `json:"deviceId"`
		RegistrationID uint32                  `json:"registrationId"`
		SignedPreKey   *api.SignedPreKeyEntity `json:"signedPreKey"`
		PreKey         *api.PreKeyEntity       `json:"preKey"`
		PqPreKey       *api.SignedPreKeyEntity `json:"pqPreKey"`
	} `json:"devices"`
}

// trusted applies trust on first use to a fetched identity key; our own
// ACI must match our identity.
func (s *Sender) trusted(dest string, key *curve25519.PublicKey) bool {
	if dest == s.Acct.Data.ACI {
		return bytes.Equal(key.Bytes(), s.Acct.Data.ACIIdentity.Public)
	}
	known, ok := s.Acct.Identities[dest]
	if !ok || (!bytes.Equal(known.Key, key.Bytes()) && s.TrustNew == "always") {
		trust := identityTrust(s.TrustNew, ok)
		s.Acct.Identities[dest] = store.Identity{Key: key.Bytes(), Trust: trust, Added: s.now().UnixMilli()}
		return trust != store.Untrusted
	}
	return bytes.Equal(known.Key, key.Bytes()) && known.Trust != store.Untrusted
}

// fetchBundles fetches dest's bundles (device "*" or a number, keys.md
// §4) and builds a session with every returned device except ourselves.
func (s *Sender) fetchBundles(ctx context.Context, dest, device string, acc *access) error {
	var r *chat.Response
	var err error
	if acc != nil {
		r, err = s.Unauth.Request(ctx, "GET", "/v2/keys/"+dest+"/"+device, []chat.Header{acc.header()}, nil)
	} else {
		r, err = s.Chat.Request(ctx, "GET", "/v2/keys/"+dest+"/"+device, nil, nil)
	}
	if err != nil {
		return err
	}
	switch {
	case r.Status == 401 && acc != nil:
		return errAccessRejected
	case r.Status == 404:
		return ErrUnregistered
	case r.Status != 200:
		return &SendStatusError{Status: r.Status}
	}
	var br bundleResponse
	if err := json.Unmarshal(r.Body, &br); err != nil {
		return fmt.Errorf("bundle response: %w", err)
	}
	ikb, err := api.DecodeBase64(br.IdentityKey)
	if err != nil {
		return fmt.Errorf("bundle identity key: %w", err)
	}
	ik, err := curve25519.ParsePublicKey(ikb)
	if err != nil {
		return fmt.Errorf("bundle identity key: %w", err)
	}
	if !s.trusted(dest, ik) {
		return fmt.Errorf("%w: %s", protocol.ErrUntrustedIdentity, dest)
	}
	identity, err := curve25519.NewPrivateKey(s.Acct.Data.ACIIdentity.Private)
	if err != nil {
		return err
	}
	built := 0
	for _, d := range br.Devices {
		if d.SignedPreKey == nil || d.PqPreKey == nil || d.DeviceID < 1 || d.DeviceID > 0x7fffffff {
			continue
		}
		if dest == s.Acct.Data.ACI && d.DeviceID == int64(s.Acct.Data.DeviceID) {
			continue
		}
		b, err := bundleFrom(ik, d.RegistrationID, d.SignedPreKey, d.PreKey, d.PqPreKey)
		if err != nil {
			continue
		}
		remote := protocol.Address{ServiceID: dest, DeviceID: uint32(d.DeviceID)}
		rec := new(protocol.SessionRecord)
		if old, ok := s.Acct.Sessions[sessionKey(false, remote)]; ok {
			if rec, err = protocol.LoadSessionRecord(old); err != nil {
				rec = new(protocol.SessionRecord)
			}
		}
		if rec, err = rec.Initiate(s.rand(), identity, s.Acct.Data.ACIRegistrationID, b, s.now(), s.local(), remote); err != nil {
			continue
		}
		if err := s.saveSession(dest, uint32(d.DeviceID), rec); err != nil {
			return err
		}
		built++
	}
	if built == 0 && !(dest == s.Acct.Data.ACI && len(br.Devices) > 0) {
		return ErrNoValidBundles
	}
	if err := s.Acct.SaveIdentities(); err != nil {
		return err
	}
	return s.Acct.SaveSessions()
}

func bundleFrom(ik *curve25519.PublicKey, regID uint32, spk *api.SignedPreKeyEntity, opk *api.PreKeyEntity, kpk *api.SignedPreKeyEntity) (*protocol.PreKeyBundle, error) {
	dec := func(s string) []byte { b, _ := api.DecodeBase64(s); return b }
	spub, err := curve25519.ParsePublicKey(dec(spk.PublicKey))
	if err != nil {
		return nil, err
	}
	kpub, err := protocol.ParseKEMPublicKey(dec(kpk.PublicKey))
	if err != nil {
		return nil, err
	}
	b := &protocol.PreKeyBundle{RegistrationID: regID, IdentityKey: ik, SignedPreKeyID: spk.KeyID, SignedPreKey: spub,
		SignedPreKeySignature: dec(spk.Signature), KyberPreKeyID: kpk.KeyID, KyberPreKey: kpub, KyberPreKeySignature: dec(kpk.Signature)}
	if opk != nil {
		opub, err := curve25519.ParsePublicKey(dec(opk.PublicKey))
		if err != nil {
			return nil, err
		}
		id := opk.KeyID
		b.PreKeyID, b.PreKey = &id, opub
	}
	return b, nil
}

type outgoingMessage struct {
	Type                      int    `json:"type"`
	DestinationDeviceID       uint32 `json:"destinationDeviceId"`
	DestinationRegistrationID uint32 `json:"destinationRegistrationId"`
	Content                   string `json:"content"`
}

type outgoingMessages struct {
	Destination string            `json:"destination"`
	Timestamp   uint64            `json:"timestamp"`
	Messages    []outgoingMessage `json:"messages"`
	Online      bool              `json:"online"`
	Urgent      bool              `json:"urgent"`
}

// send runs the send loop of send.md §5 for one destination (an ACI). It
// returns the response's needsSync and whether the message went out
// sealed and was accepted on the unauthenticated socket.
func (s *Sender) send(ctx context.Context, dest string, plaintext []byte, ts uint64, urgent bool, hint uint32) (needsSync, unidentified bool, err error) {
	return s.sendOnline(ctx, dest, plaintext, ts, urgent, false, hint)
}

// sendOnline is send with the online flag (typing messages).
func (s *Sender) sendOnline(ctx context.Context, dest string, plaintext []byte, ts uint64, urgent, online bool, hint uint32) (needsSync, unidentified bool, err error) {
	return s.deliver(ctx, dest, plaintext, ts, deliverOpts{urgent: urgent, online: online, hint: hint})
}

// sendUnsealed sends on the authenticated socket only, not urgent, with
// no effect on the recipient's sealed-sender state (sync.md §6.5).
func (s *Sender) sendUnsealed(ctx context.Context, dest string, plaintext []byte, ts uint64) (needsSync bool, err error) {
	needsSync, _, err = s.deliver(ctx, dest, plaintext, ts, deliverOpts{unsealed: true, hint: protocol.ContentHintImplicit})
	return needsSync, err
}

// deliverOpts are the flags of one 1:1 send.
type deliverOpts struct {
	urgent, online bool
	unsealed       bool   // never sealed, no sticky downgrade
	story          bool   // ?story=true (story SKDMs)
	groupID        []byte // USMC group ID (group sends)
	groupToken     []byte // full group send token: sealed access without a key, or fallback
	hint           uint32
	// devices, if set, receives the device IDs of the successful attempt
	// (send-log.md §1.1).
	devices *[]uint32
	// noDowngrade skips the sticky sealed-sender downgrade (resends,
	// send-log.md §4.2).
	noDowngrade bool
}

// deliver encrypts for dest's devices and puts the message, sealed when
// access allows unless unsealed is set.
func (s *Sender) deliver(ctx context.Context, dest string, plaintext []byte, ts uint64, o deliverOpts) (needsSync, unidentified bool, err error) {
	urgent, online, unsealed, hint := o.urgent, o.online, o.unsealed, o.hint
	self := dest == s.Acct.Data.ACI
	padded := content.Pad(plaintext)
	var acc *access
	if !unsealed {
		acc = s.accessFor(ctx, dest)
		if acc == nil && o.groupToken != nil {
			// No access key: the group send token (groups.md §12.4).
			if cert := s.senderCertificate(ctx); cert != nil {
				acc = &access{token: o.groupToken, cert: cert}
			}
		}
	}
	defer func() {
		if err == nil && !unidentified && !self && !unsealed && !o.noDowngrade {
			s.downgrade(dest)
		}
	}()
	for attempt := 0; attempt < 4; attempt++ {
		recs, err := s.sessions(dest)
		if err != nil {
			return false, false, err
		}
		// Targets: device 1 plus every device with a session (§4.1); for
		// ourselves, never the sending device.
		targets := slices.Collect(maps.Keys(recs))
		if !slices.Contains(targets, 1) {
			targets = append(targets, 1)
		}
		if self {
			targets = slices.DeleteFunc(targets, func(id uint32) bool { return id == uint32(s.Acct.Data.DeviceID) })
		}
		if _, ok := recs[1]; !ok && !(self && s.Acct.Data.DeviceID == 1) {
			err := s.fetchBundles(ctx, dest, "*", acc)
			if errors.Is(err, errAccessRejected) {
				if acc.token == nil && o.groupToken != nil {
					acc = &access{token: o.groupToken, cert: acc.cert} // token fallback (groups.md §12.3)
				} else {
					acc = nil // the next attempt is fully unsealed (§3.3)
				}
				continue
			}
			if err != nil {
				return false, false, err
			}
			if recs, err = s.sessions(dest); err != nil {
				return false, false, err
			}
		}
		slices.Sort(targets)

		var identity *curve25519.PrivateKey
		if acc != nil {
			if identity, err = curve25519.NewPrivateKey(s.Acct.Data.ACIIdentity.Private); err != nil {
				return false, false, err
			}
		}
		body := outgoingMessages{Destination: dest, Timestamp: ts, Messages: []outgoingMessage{}, Online: online, Urgent: urgent}
		for _, id := range targets {
			rec := recs[id]
			if rec == nil {
				continue // fetched just now; joins on the next attempt
			}
			remote := protocol.Address{ServiceID: dest, DeviceID: id}
			next, typ, wire, err := rec.Encrypt(s.rand(), padded, s.now(), s.local(), remote)
			if err != nil {
				return false, false, err
			}
			envType := envCiphertext
			if typ == protocol.MessageTypePreKey {
				envType = envPreKeyBundle
			}
			if acc != nil {
				kind := protocol.SealedContentMessage
				if typ == protocol.MessageTypePreKey {
					kind = protocol.SealedContentPreKey
				}
				usmc := protocol.EncodeSealedContent(kind, acc.cert, wire, hint, o.groupID)
				if wire, err = protocol.SealV1(s.rand(), identity, next.RemoteIdentity(), usmc); err != nil {
					return false, false, err
				}
				envType = envUnidentified
			}
			body.Messages = append(body.Messages, outgoingMessage{Type: envType, DestinationDeviceID: id,
				DestinationRegistrationID: next.RemoteRegistrationID(), Content: base64.StdEncoding.EncodeToString(wire)})
			if err := s.saveSession(dest, id, next); err != nil {
				return false, false, err
			}
		}
		if err := s.Acct.SaveSessions(); err != nil {
			return false, false, err
		}
		// With no session with any other own device the list stays empty:
		// the server answers 409 with the devices to set up (sync.md §1;
		// signal-cli encrypts for its own device as a placeholder instead).
		b, err := api.Marshal(body)
		if err != nil {
			return false, false, err
		}
		path := "/v1/messages/" + dest + "?story=" + strconv.FormatBool(o.story)
		var r *chat.Response
		sealedPut := acc != nil
		if sealedPut {
			r, err = s.Unauth.Request(ctx, "PUT", path, append(append([]chat.Header{}, jsonHeader...), acc.header()), b)
			if err == nil && r.Status == 401 {
				// Repeated unchanged on the authenticated socket (§3.3).
				sealedPut = false
				r, err = s.Chat.Request(ctx, "PUT", path, jsonHeader, b)
			}
		} else {
			r, err = s.Chat.Request(ctx, "PUT", path, jsonHeader, b)
		}
		if err != nil {
			return false, false, err
		}
		switch r.Status {
		case 200:
			var ok struct {
				NeedsSync bool `json:"needsSync"`
			}
			json.Unmarshal(r.Body, &ok)
			if o.devices != nil {
				*o.devices = (*o.devices)[:0]
				for _, m := range body.Messages {
					*o.devices = append(*o.devices, m.DestinationDeviceID)
				}
			}
			return ok.NeedsSync, sealedPut, nil
		case 401:
			return false, false, fmt.Errorf("authorization failed")
		case 404:
			return false, false, ErrUnregistered
		case 409:
			var mm struct {
				Missing []uint32 `json:"missingDevices"`
				Extra   []uint32 `json:"extraDevices"`
			}
			if err := json.Unmarshal(r.Body, &mm); err != nil {
				return false, false, fmt.Errorf("409 body: %w", err)
			}
			if err := s.archive(dest, mm.Extra); err != nil {
				return false, false, err
			}
			if self && len(mm.Missing) == 0 && len(body.Messages) == len(mm.Extra) {
				// Our other devices are gone: single-device from now on
				// (send.md §5, special case).
				if s.Acct.Data.MultiDevice {
					s.Acct.Data.MultiDevice = false
					s.Acct.Save()
				}
				return false, false, nil
			}
			for _, id := range mm.Missing {
				// Always on the authenticated socket (keys.md §4.1).
				if err := s.fetchBundles(ctx, dest, strconv.Itoa(int(id)), nil); err != nil {
					return false, false, err
				}
			}
		case 410:
			var st struct {
				Stale []uint32 `json:"staleDevices"`
			}
			if err := json.Unmarshal(r.Body, &st); err != nil {
				return false, false, fmt.Errorf("410 body: %w", err)
			}
			if err := s.archive(dest, st.Stale); err != nil {
				return false, false, err
			}
		default:
			return false, false, &SendStatusError{Status: r.Status}
		}
	}
	return false, false, ErrConflictsUnsolved
}

// archive archives our current sessions with dest's devices.
func (s *Sender) archive(dest string, devices []uint32) error {
	for _, id := range devices {
		key := sessionKey(false, protocol.Address{ServiceID: dest, DeviceID: id})
		b, ok := s.Acct.Sessions[key]
		if !ok {
			continue
		}
		rec, err := protocol.LoadSessionRecord(b)
		if err != nil {
			delete(s.Acct.Sessions, key)
			continue
		}
		if err := s.saveSession(dest, id, rec.ArchiveCurrent()); err != nil {
			return err
		}
	}
	return s.Acct.SaveSessions()
}
