// SPDX-License-Identifier: GPL-3.0-or-later

package manager

import (
	"bytes"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"maps"
	"strconv"
	"strings"
	"time"

	"github.com/aren55555/signal-cli-go/internal/content"
	"github.com/aren55555/signal-cli-go/internal/crypto/curve25519"
	"github.com/aren55555/signal-cli-go/internal/proto/signalpb"
	"github.com/aren55555/signal-cli-go/internal/protocol"
	"github.com/aren55555/signal-cli-go/internal/store"
	"google.golang.org/protobuf/proto"
)

// Envelope types (spec/protocol/capture.md §3).
const (
	envCiphertext    = 1
	envPreKeyBundle  = 3
	envUnidentified  = 6
	envServerReceipt = 5
	envPlaintext     = 8
	envSenderKey     = 7 // only inside sealed sender
)

// ErrUnsupportedEnvelope reports an envelope kind not handled yet.
var ErrUnsupportedEnvelope = errors.New("unsupported envelope type")

// Message is a decrypted envelope.
type Message struct {
	Sender          protocol.Address
	ToPNI           bool // addressed to our PNI
	Sealed          bool
	ServerReceipt   bool   // a server delivery receipt: no content
	InnerType       int    // 3 PreKey or 1 Whisper, after unsealing
	Content         []byte // the unpadded Content protobuf
	ClientTimestamp uint64
	ServerTimestamp uint64
}

// Decryptor decrypts envelopes for an account and keeps its store
// current: sessions, remote identities and consumed prekeys are saved
// before Decrypt returns.
type Decryptor struct {
	Acct  *store.Account
	Rand  io.Reader
	Now   func() time.Time
	Roots []*curve25519.PublicKey // sealed-sender trust roots; default staging
	// Retrying marks a cache retry pass: identity changes are not stored
	// (capture.md §5).
	Retrying bool
	// TrustNew is --trust-new-identities (on-first-use, always, never).
	TrustNew string
}

func (d *Decryptor) rand() io.Reader {
	if d.Rand != nil {
		return d.Rand
	}
	return rand.Reader
}

// sessionKey names a session in the store: the local identity kind, the
// peer's service ID and device.
func sessionKey(toPNI bool, remote protocol.Address) string {
	kind := "aci"
	if toPNI {
		kind = "pni"
	}
	return kind + ":" + remote.ServiceID + "." + strconv.Itoa(int(remote.DeviceID))
}

// preKeys looks up one identity's private prekeys in the store.
type preKeys struct{ k *store.IdentityKeys }

func (p preKeys) SignedPreKey(id uint32) (*curve25519.PrivateKey, error) {
	sk, ok := p.k.SignedPreKeys[id]
	if !ok {
		return nil, protocol.ErrInvalidSignedPreKeyID
	}
	r, err := protocol.LoadSignedPreKeyRecord(sk.Record)
	if err != nil {
		return nil, err
	}
	return r.PrivateKey()
}

func (p preKeys) KyberPreKey(id uint32) (*protocol.KEMSecretKey, error) {
	sk, ok := p.k.KyberPreKeys[id]
	if !ok {
		return nil, protocol.ErrInvalidKyberPreKeyID
	}
	r, err := protocol.LoadKyberPreKeyRecord(sk.Record)
	if err != nil {
		return nil, err
	}
	return r.SecretKey()
}

func (p preKeys) PreKey(id uint32) (*curve25519.PrivateKey, error) {
	sk, ok := p.k.PreKeys[id]
	if !ok {
		return nil, protocol.ErrInvalidPreKeyID
	}
	r, err := protocol.LoadPreKeyRecord(sk.Record)
	if err != nil {
		return nil, err
	}
	return r.PrivateKey()
}

// Decrypt decrypts one envelope. On failure nothing in the store changes.
func (d *Decryptor) Decrypt(env *signalpb.ServiceEnvelope) (*Message, error) {
	a := d.Acct
	dest := env.GetDestinationServiceId()
	if b := env.GetDestinationServiceIdBinary(); dest == "" && len(b) > 0 {
		var err error
		if dest, err = protocol.ServiceIDFromBinary(b); err != nil {
			return nil, err
		}
	}
	m := &Message{ToPNI: strings.HasPrefix(dest, "PNI:"), ClientTimestamp: env.GetClientTimestamp(), ServerTimestamp: env.GetServerTimestamp()}
	pair, regID, keys, localID := a.Data.ACIIdentity, a.Data.ACIRegistrationID, &a.Keys.ACI, a.Data.ACI
	if m.ToPNI {
		pair, regID, keys, localID = a.Data.PNIIdentity, a.Data.PNIRegistrationID, &a.Keys.PNI, "PNI:"+a.Data.PNI
	}
	identity, err := curve25519.NewPrivateKey(pair.Private)
	if err != nil {
		return nil, fmt.Errorf("local identity: %w", err)
	}
	local := protocol.Address{ServiceID: localID, DeviceID: uint32(a.Data.DeviceID)}

	kind, wire := int(env.GetType()), env.GetContent()
	src := env.GetSourceServiceId()
	if b := env.GetSourceServiceIdBinary(); src == "" && len(b) > 0 {
		if src, err = protocol.ServiceIDFromBinary(b); err != nil {
			return nil, err
		}
	}
	m.Sender = protocol.Address{ServiceID: src, DeviceID: env.GetSourceDeviceId()}
	if kind == envServerReceipt {
		m.ServerReceipt = true
		return m, nil
	}
	if kind == envUnidentified {
		roots := d.Roots
		if roots == nil {
			roots = protocol.TrustRoots(protocol.Staging)
		}
		sc, err := protocol.Unseal(identity, roots, env.GetServerTimestamp(), wire)
		if err != nil {
			return nil, err
		}
		m.Sealed = true
		m.Sender = protocol.Address{ServiceID: sc.Sender.SenderUUID, DeviceID: sc.Sender.DeviceID}
		wire = sc.Content
		if m.Sender.ServiceID == a.Data.ACI && m.Sender.DeviceID == uint32(a.Data.DeviceID) {
			return nil, ErrSelfSend
		}
		switch sc.Kind {
		case protocol.SealedContentPreKey:
			kind = envPreKeyBundle
		case protocol.SealedContentMessage:
			kind = envCiphertext
		case protocol.SealedContentPlaintext:
			kind = envPlaintext
		case protocol.SealedContentSenderKey:
			kind = envSenderKey
		default:
			return nil, fmt.Errorf("%w: sealed content kind %d", ErrUnsupportedEnvelope, sc.Kind)
		}
	}

	m.InnerType = kind
	if kind == envPlaintext {
		return plaintextContent(m, wire)
	}
	if kind == envSenderKey {
		return d.decryptSenderKey(m, wire)
	}
	key := sessionKey(m.ToPNI, m.Sender)
	rec := new(protocol.SessionRecord)
	if b, ok := a.Sessions[key]; ok {
		if rec, err = protocol.LoadSessionRecord(b); err != nil {
			return nil, err
		}
	}
	var pt []byte
	var used *protocol.UsedPreKeys
	var newIdentity, changedIdentity []byte
	switch kind {
	case envPreKeyBundle:
		trust := func(k *curve25519.PublicKey) bool {
			known, ok := a.Identities[m.Sender.ServiceID]
			if !ok {
				newIdentity = k.Bytes()
				return identityTrust(d.TrustNew, false) != store.Untrusted
			}
			if !bytes.Equal(known.Key, k.Bytes()) {
				changedIdentity = k.Bytes()
				return identityTrust(d.TrustNew, true) != store.Untrusted
			}
			return known.Trust != store.Untrusted
		}
		rec, pt, used, err = rec.DecryptPreKey(d.rand(), wire, protocol.LocalIdentity{Key: identity, RegistrationID: regID}, preKeys{keys}, trust, local, m.Sender)
	case envCiphertext:
		rec, pt, err = rec.Decrypt(d.rand(), wire, local, m.Sender)
	default:
		return nil, fmt.Errorf("%w: %d", ErrUnsupportedEnvelope, kind)
	}
	if err != nil {
		if errors.Is(err, protocol.ErrUntrustedIdentity) && (changedIdentity != nil || newIdentity != nil) && !d.Retrying {
			// The key is stored untrusted, so that listIdentities and
			// trust can see it.
			a.Identities[m.Sender.ServiceID] = store.Identity{Key: append(changedIdentity, newIdentity...), Trust: store.Untrusted, Added: d.now().UnixMilli()}
			if serr := a.SaveIdentities(); serr != nil {
				return nil, serr
			}
		}
		if errors.Is(err, protocol.ErrUntrustedIdentity) || errors.Is(err, protocol.ErrDuplicateMessage) {
			return nil, err
		}
		msgType := protocol.MessageTypeWhisper
		if kind == envPreKeyBundle {
			msgType = protocol.MessageTypePreKey
		}
		return nil, &DecryptError{Sender: m.Sender, ToPNI: m.ToPNI, Sealed: m.Sealed, InnerType: kind, Timestamp: m.ClientTimestamp,
			RatchetKey: protocol.SenderRatchetKey(wire, msgType), Err: err}
	}
	if c, ok := content.Unpad(pt); ok {
		m.Content = c
	} else {
		m.Content = pt // unpadded already (or not ours to judge)
	}

	// Commit: session, identity, consumed prekeys.
	b, err := rec.Serialize()
	if err != nil {
		return nil, err
	}
	a.Sessions[key] = b
	if err := a.SaveSessions(); err != nil {
		return nil, err
	}
	if newIdentity != nil || changedIdentity != nil {
		now := time.Now()
		if d.Now != nil {
			now = d.Now()
		}
		a.Identities[m.Sender.ServiceID] = store.Identity{Key: append(newIdentity, changedIdentity...), Trust: store.TrustedUnverified, Added: now.UnixMilli()}
		if err := a.SaveIdentities(); err != nil {
			return nil, err
		}
	}
	if used != nil {
		changed := false
		if used.OneTimePreKeyID != nil {
			if _, ok := keys.PreKeys[*used.OneTimePreKeyID]; ok {
				keys.PreKeys = maps.Clone(keys.PreKeys)
				delete(keys.PreKeys, *used.OneTimePreKeyID)
				changed = true
			}
		}
		if k, ok := keys.KyberPreKeys[used.KyberPreKeyID]; ok && !k.LastResort {
			keys.KyberPreKeys = maps.Clone(keys.KyberPreKeys)
			delete(keys.KyberPreKeys, used.KyberPreKeyID)
			changed = true
		}
		if changed {
			if err := a.SaveKeys(); err != nil {
				return nil, err
			}
		}
	}
	return m, nil
}

// ErrSelfSend reports a sealed message from this very device; it is
// dropped silently (pipe.md §7.2).
var ErrSelfSend = errors.New("sealed sender message from this device")

// DecryptError is a protocol-level failure (invalid message, no session,
// invalid key or key ID) from a known sender: what a retry receipt and
// session renewal need (pipe.md §7.2).
type DecryptError struct {
	Sender     protocol.Address
	ToPNI      bool
	Sealed     bool
	InnerType  int    // envelope type of the failing message, after unsealing
	Timestamp  uint64 // the envelope's client timestamp
	RatchetKey []byte // nil if the message did not parse
	Err        error
}

func (e *DecryptError) Error() string { return e.Err.Error() }
func (e *DecryptError) Unwrap() error { return e.Err }

// plaintextContent unwraps PlaintextContent (0xC0, Content, 0x80), valid
// only when it carries a DecryptionErrorMessage alone (content.md §5.3).
func plaintextContent(m *Message, wire []byte) (*Message, error) {
	if len(wire) < 2 || wire[0] != 0xC0 || wire[len(wire)-1] != 0x80 {
		return nil, fmt.Errorf("%w: malformed plaintext content", protocol.ErrInvalidMessage)
	}
	var c signalpb.AppContent
	if err := proto.Unmarshal(wire[1:len(wire)-1], &c); err != nil || c.GetDecryptionErrorMessage() == nil ||
		c.SenderKeyDistributionMessage != nil || c.PniSignatureMessage != nil {
		return nil, fmt.Errorf("%w: plaintext content without a lone decryption error", protocol.ErrInvalidMessage)
	}
	m.Content = wire[1 : len(wire)-1]
	return m, nil
}

func (d *Decryptor) now() time.Time {
	if d.Now != nil {
		return d.Now()
	}
	return time.Now()
}

// SenderKeyName is the sender-key store key for a sender device and
// distribution.
func SenderKeyName(serviceID string, device uint32, id protocol.DistributionID) string {
	return serviceID + "." + strconv.Itoa(int(device)) + "." + id.String()
}

// decryptSenderKey decrypts a SenderKeyMessage with the record for the
// sender device and the message's distribution (sender-keys.md §6). The
// record is stored only after success.
func (d *Decryptor) decryptSenderKey(m *Message, wire []byte) (*Message, error) {
	a := d.Acct
	fail := func(err error) (*Message, error) {
		if errors.Is(err, protocol.ErrDuplicateMessage) {
			return nil, err
		}
		return nil, &DecryptError{Sender: m.Sender, ToPNI: m.ToPNI, Sealed: m.Sealed, InnerType: envSenderKey, Timestamp: m.ClientTimestamp, Err: err}
	}
	id, err := protocol.MessageDistributionID(wire)
	if err != nil {
		return fail(err)
	}
	name := SenderKeyName(m.Sender.ServiceID, m.Sender.DeviceID, id)
	rec := new(protocol.SenderKeyRecord)
	if b, ok := a.SenderKeys.Records[name]; ok {
		if rec, err = protocol.LoadSenderKeyRecord(b); err != nil {
			return nil, err
		}
	}
	next, pt, err := rec.Decrypt(wire)
	if err != nil {
		return fail(err)
	}
	b, err := next.Serialize()
	if err != nil {
		return nil, err
	}
	a.SenderKeys.Records[name] = b
	if err := a.SaveSenderKeys(); err != nil {
		return nil, err
	}
	if c, ok := content.Unpad(pt); ok {
		m.Content = c
	} else {
		m.Content = pt
	}
	return m, nil
}

// ProcessSenderKeyDistribution stores a received SKDM under the sender
// device and the distribution it names (stories.md §3.8).
func ProcessSenderKeyDistribution(a *store.Account, sender protocol.Address, skdm []byte) error {
	var probe protocol.SenderKeyRecord
	_, id, err := probe.ProcessDistribution(skdm)
	if err != nil {
		return err
	}
	name := SenderKeyName(sender.ServiceID, sender.DeviceID, id)
	rec := new(protocol.SenderKeyRecord)
	if b, ok := a.SenderKeys.Records[name]; ok {
		if rec, err = protocol.LoadSenderKeyRecord(b); err != nil {
			return err
		}
	}
	next, _, err := rec.ProcessDistribution(skdm)
	if err != nil {
		return err
	}
	b, err := next.Serialize()
	if err != nil {
		return err
	}
	if _, ok := a.SenderKeys.Records[name]; !ok {
		a.SenderKeys.Created[name] = time.Now().UnixMilli()
	}
	a.SenderKeys.Records[name] = b
	return a.SaveSenderKeys()
}
