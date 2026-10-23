package protocol

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/aren55555/signal-cli-go/internal/crypto/curve25519"
	"github.com/aren55555/signal-cli-go/internal/proto/signalpb"
	"github.com/aren55555/signal-cli-go/internal/protocol/spqr"
)

// Loading libsignal's serialized records as signal-cli 0.14.7 stores them
// (spec/protocol/double-ratchet.md §10, sender-keys.md §7, capture.md §2).

func stateErr(format string, args ...any) error {
	return fmt.Errorf("%w: "+format, append([]any{ErrInvalidState}, args...)...)
}

func loadKey(b []byte, what string) (*curve25519.PublicKey, error) {
	k, err := curve25519.ParsePublicKey(b)
	if err != nil {
		return nil, stateErr("%s: %v", what, err)
	}
	return k, nil
}

// LoadSessionRecord parses a SessionRecord. Archived states that cannot
// be decoded are skipped, as libsignal skips them when decrypting.
func LoadSessionRecord(b []byte) (*SessionRecord, error) {
	var pb signalpb.StoredSessionRecord
	if err := proto.Unmarshal(b, &pb); err != nil {
		return nil, stateErr("%v", err)
	}
	rec := new(SessionRecord)
	if pb.Current != nil {
		s, err := loadSession(pb.Current)
		if err != nil {
			return nil, err
		}
		rec.current = s
	}
	for _, raw := range pb.Previous {
		var ps signalpb.StoredSession
		if proto.Unmarshal(raw, &ps) != nil {
			continue
		}
		if s, err := loadSession(&ps); err == nil {
			rec.previous = append(rec.previous, s)
		}
	}
	return rec, nil
}

func loadChainKey(ck *signalpb.StoredSession_Chain_ChainKey) chain {
	if ck == nil {
		return chain{}
	}
	return chain{key: ck.Key, index: ck.Index}
}

func loadSession(ps *signalpb.StoredSession) (*SessionState, error) {
	s := &SessionState{
		version:              uint8(ps.Version),
		rootKey:              ps.RootKey,
		previousCounter:      ps.PreviousCounter,
		aliceBaseKey:         ps.AliceBaseKey,
		localRegistrationID:  ps.LocalRegistrationId,
		remoteRegistrationID: ps.RemoteRegistrationId,
		pqRaw:                ps.PqRatchetState,
	}
	if ps.Version == 0 {
		s.version = 2 // never matches a parsable message (double-ratchet.md §9)
	}
	var err error
	if len(ps.LocalIdentity) > 0 {
		if s.localIdentity, err = loadKey(ps.LocalIdentity, "local identity"); err != nil {
			return nil, err
		}
	}
	if len(ps.RemoteIdentity) > 0 {
		if s.remoteIdentity, err = loadKey(ps.RemoteIdentity, "remote identity"); err != nil {
			return nil, err
		}
	}
	if sc := ps.SenderChain; sc != nil {
		s.hasSenderChain = true
		s.sendChain = loadChainKey(sc.ChainKey)
		if len(sc.RatchetKeyPrivate) > 0 {
			if s.senderRatchet, err = curve25519.NewPrivateKey(sc.RatchetKeyPrivate); err != nil {
				return nil, stateErr("sender ratchet key: %v", err)
			}
		}
	}
	for _, c := range ps.ReceiverChains {
		key, err := loadKey(c.RatchetKey, "receiver ratchet key")
		if err != nil {
			return nil, err
		}
		rc := &receiverChain{ratchetKey: key, chain: loadChainKey(c.ChainKey)}
		// libsignal keeps skipped keys newest first; this keeps them oldest first.
		for i := len(c.MessageKeys) - 1; i >= 0; i-- {
			mk := c.MessageKeys[i]
			sk := skippedSeed{index: mk.Index}
			if len(mk.Seed) > 0 {
				sk.seed = mk.Seed
			} else {
				sk.legacy = &messageKeys{cipherKey: mk.CipherKey, macKey: mk.MacKey, iv: mk.Iv}
			}
			rc.skipped = append(rc.skipped, sk)
		}
		s.receivers = append(s.receivers, rc)
	}
	if pp := ps.PendingPreKey; pp != nil {
		base, err := loadKey(pp.BaseKey, "pending base key")
		if err != nil {
			return nil, err
		}
		s.pending = &pendingPreKey{
			oneTimePreKeyID: pp.OneTimePreKeyId,
			signedPreKeyID:  uint32(pp.SignedPreKeyId),
			baseKey:         base,
			created:         time.Unix(int64(pp.Timestamp), 0),
		}
	}
	if pk := ps.PendingKyberPreKey; pk != nil {
		s.pendingKyber = &pendingKyberPreKey{kyberPreKeyID: pk.KyberPreKeyId, ciphertext: pk.Ciphertext}
	}
	s.pq, s.pqErr = spqr.Import(ps.PqRatchetState)
	return s, nil
}

// usesSPQR mirrors libsignal's check: non-empty stored SPQR bytes, or a
// live V1 state.
func (s *SessionState) usesSPQR() bool {
	return len(s.pqRaw) > 0 || (s.pq != nil && s.pq.Epoch() > 0)
}

// HasUsableSenderChain is libsignal's "has a usable sender chain with a
// required PQ ratio" (pqxdh.md §7). signal-cli passes ratio 0.
func (r *SessionRecord) HasUsableSenderChain(now time.Time, requirePQRatio float64) bool {
	s := r.current
	if s == nil || !s.hasSenderChain {
		return false
	}
	if s.pending != nil && now.Sub(s.pending.created) > MaxUnacknowledgedSessionAge {
		return false
	}
	if s.version > 3 && s.usesSPQR() {
		return true
	}
	return useNonPQSession(s.aliceBaseKey, requirePQRatio)
}

// useNonPQSession accepts a non-PQ session when floor((2^32-1) * ratio) <=
// the first 4 bytes of the base key (big-endian).
func useNonPQSession(aliceBaseKey []byte, ratio float64) bool {
	switch {
	case ratio <= 0:
		return true
	case ratio >= 1 || len(aliceBaseKey) < 4:
		return false
	}
	threshold := uint32(math.Floor(float64(math.MaxUint32) * ratio))
	return threshold <= binary.BigEndian.Uint32(aliceBaseKey)
}

// PreKeyRecord is a stored one-time EC prekey.
type PreKeyRecord struct {
	ID                    uint32
	publicKey, privateKey []byte
}

// LoadPreKeyRecord parses a PreKeyRecord.
func LoadPreKeyRecord(b []byte) (*PreKeyRecord, error) {
	var pb signalpb.StoredPreKey
	if err := proto.Unmarshal(b, &pb); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidMessage, err)
	}
	return &PreKeyRecord{ID: pb.Id, publicKey: pb.PublicKey, privateKey: pb.PrivateKey}, nil
}

// PrivateKey returns the prekey's private key.
func (r *PreKeyRecord) PrivateKey() (*curve25519.PrivateKey, error) {
	return curve25519.NewPrivateKey(r.privateKey)
}

// SignedPreKeyRecord is a stored signed EC prekey.
type SignedPreKeyRecord struct {
	ID                    uint32
	Signature             []byte
	Timestamp             uint64 // milliseconds
	publicKey, privateKey []byte
}

func loadSigned(b []byte) (*signalpb.StoredSignedPreKey, error) {
	var pb signalpb.StoredSignedPreKey
	if err := proto.Unmarshal(b, &pb); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidMessage, err)
	}
	return &pb, nil
}

// LoadSignedPreKeyRecord parses a SignedPreKeyRecord.
func LoadSignedPreKeyRecord(b []byte) (*SignedPreKeyRecord, error) {
	pb, err := loadSigned(b)
	if err != nil {
		return nil, err
	}
	return &SignedPreKeyRecord{ID: pb.Id, Signature: pb.Signature, Timestamp: pb.Timestamp, publicKey: pb.PublicKey, privateKey: pb.PrivateKey}, nil
}

// PrivateKey returns the signed prekey's private key.
func (r *SignedPreKeyRecord) PrivateKey() (*curve25519.PrivateKey, error) {
	return curve25519.NewPrivateKey(r.privateKey)
}

// KyberPreKeyRecord is a stored Kyber prekey (the signed-prekey layout
// with framed Kyber keys).
type KyberPreKeyRecord struct {
	ID                    uint32
	Signature             []byte
	Timestamp             uint64
	publicKey, privateKey []byte
}

// LoadKyberPreKeyRecord parses a KyberPreKeyRecord.
func LoadKyberPreKeyRecord(b []byte) (*KyberPreKeyRecord, error) {
	pb, err := loadSigned(b)
	if err != nil {
		return nil, err
	}
	return &KyberPreKeyRecord{ID: pb.Id, Signature: pb.Signature, Timestamp: pb.Timestamp, publicKey: pb.PublicKey, privateKey: pb.PrivateKey}, nil
}

// SecretKey returns the Kyber secret key.
func (r *KyberPreKeyRecord) SecretKey() (*KEMSecretKey, error) {
	return ParseKEMSecretKey(r.privateKey)
}

// LoadSenderKeyRecord parses a SenderKeyRecord.
func LoadSenderKeyRecord(b []byte) (*SenderKeyRecord, error) {
	var pb signalpb.StoredSenderKeyRecord
	if err := proto.Unmarshal(b, &pb); err != nil {
		return nil, stateErr("%v", err)
	}
	rec := new(SenderKeyRecord)
	for _, ps := range pb.States {
		if ps.ChainKey == nil || ps.SigningKey == nil {
			return nil, stateErr("sender key state without chain or signing key")
		}
		pub, err := loadKey(ps.SigningKey.PublicKey, "signing key")
		if err != nil {
			return nil, err
		}
		s := &senderKeyState{
			version:    uint8(ps.MessageVersion),
			chainID:    ps.ChainId,
			iteration:  ps.ChainKey.Iteration,
			chainKey:   ps.ChainKey.Seed,
			signingPub: pub,
		}
		if s.version == 0 {
			s.version = senderKeyVersion
		}
		if len(ps.SigningKey.PrivateKey) > 0 {
			if s.signingPriv, err = curve25519.NewPrivateKey(ps.SigningKey.PrivateKey); err != nil {
				return nil, stateErr("signing private key: %v", err)
			}
		}
		for _, mk := range ps.MessageKeys {
			s.skipped = append(s.skipped, skippedSeed{index: mk.Iteration, seed: mk.Seed})
		}
		rec.states = append(rec.states, s)
	}
	return rec, nil
}

// cachedEnvelopeVersion is the msg-cache file format signal-cli 0.14.7
// writes (capture.md §3).
const cachedEnvelopeVersion = 9

// ParseCachedEnvelope parses a signal-cli msg-cache file: a big-endian
// uint32 version (9), a big-endian uint64 server-delivered timestamp, then
// the Envelope protobuf.
func ParseCachedEnvelope(b []byte) (serverDelivered uint64, env *signalpb.ServiceEnvelope, err error) {
	if len(b) < 12 {
		return 0, nil, errors.New("protocol: cached envelope too short")
	}
	if v := binary.BigEndian.Uint32(b); v != cachedEnvelopeVersion {
		return 0, nil, fmt.Errorf("protocol: cached envelope version %d, want %d", v, cachedEnvelopeVersion)
	}
	env = new(signalpb.ServiceEnvelope)
	if err := proto.Unmarshal(b[12:], env); err != nil {
		return 0, nil, fmt.Errorf("protocol: cached envelope: %w", err)
	}
	return binary.BigEndian.Uint64(b[4:]), env, nil
}
