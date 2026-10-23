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
)

// Envelope types (spec/protocol/capture.md §3).
const (
	envCiphertext    = 1
	envPreKeyBundle  = 3
	envUnidentified  = 6
	envServerReceipt = 5
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
		switch sc.Kind {
		case protocol.SealedContentPreKey:
			kind = envPreKeyBundle
		case protocol.SealedContentMessage:
			kind = envCiphertext
		default:
			return nil, fmt.Errorf("%w: sealed content kind %d", ErrUnsupportedEnvelope, sc.Kind)
		}
	}

	m.InnerType = kind
	key := sessionKey(m.ToPNI, m.Sender)
	rec := new(protocol.SessionRecord)
	if b, ok := a.Sessions[key]; ok {
		if rec, err = protocol.LoadSessionRecord(b); err != nil {
			return nil, err
		}
	}
	var pt []byte
	var used *protocol.UsedPreKeys
	var newIdentity []byte
	switch kind {
	case envPreKeyBundle:
		trust := func(k *curve25519.PublicKey) bool {
			known, ok := a.Identities[m.Sender.ServiceID]
			if !ok {
				newIdentity = k.Bytes()
				return true
			}
			return bytes.Equal(known.Key, k.Bytes()) && known.Trust != store.Untrusted
		}
		rec, pt, used, err = rec.DecryptPreKey(d.rand(), wire, protocol.LocalIdentity{Key: identity, RegistrationID: regID}, preKeys{keys}, trust, local, m.Sender)
	case envCiphertext:
		rec, pt, err = rec.Decrypt(d.rand(), wire, local, m.Sender)
	default:
		return nil, fmt.Errorf("%w: %d", ErrUnsupportedEnvelope, kind)
	}
	if err != nil {
		return nil, err
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
	if newIdentity != nil {
		now := time.Now()
		if d.Now != nil {
			now = d.Now()
		}
		a.Identities[m.Sender.ServiceID] = store.Identity{Key: newIdentity, Trust: store.TrustedUnverified, Added: now.UnixMilli()}
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
