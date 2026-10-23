package protocol

import (
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/aren55555/signal-cli-go/internal/crypto/curve25519"
)

// Ciphertext message types (libsignal's numbering).
const (
	MessageTypeWhisper = 2 // SignalMessage
	MessageTypePreKey  = 3 // PreKeySignalMessage
)

// Record limits (double-ratchet.md §6, pqxdh.md §4 and §7).
const (
	MaxArchivedStates           = 40
	MaxUnacknowledgedSessionAge = 30 * 24 * time.Hour
)

// Errors with libsignal's meanings (pqxdh.md §7-8, double-ratchet.md §7).
var (
	ErrSessionNotFound       = errors.New("protocol: session not found")
	ErrDecryptionFailed      = fmt.Errorf("%w: decryption failed", ErrInvalidMessage)
	ErrUntrustedIdentity     = errors.New("protocol: untrusted identity")
	ErrInvalidSignedPreKeyID = errors.New("protocol: invalid signed prekey ID")
	ErrInvalidKyberPreKeyID  = errors.New("protocol: invalid Kyber prekey ID")
	ErrInvalidPreKeyID       = errors.New("protocol: invalid prekey ID")
)

// Address is a protocol address: a service ID (an ACI as a hyphenated
// UUID, or "PNI:" and a UUID) and a device ID.
type Address struct {
	ServiceID string
	DeviceID  uint32
}

// serviceIDBinary is the 17-byte fixed-width form: a kind byte (0x00 ACI,
// 0x01 PNI) and the UUID's 16 bytes. ok is false if the name is not a
// service ID.
func serviceIDBinary(id string) (b []byte, ok bool) {
	kind := byte(0x00)
	if rest, isPNI := strings.CutPrefix(id, "PNI:"); isPNI {
		kind, id = 0x01, rest
	}
	if len(id) != 36 || id[8] != '-' || id[13] != '-' || id[18] != '-' || id[23] != '-' {
		return nil, false
	}
	raw, err := hex.DecodeString(strings.ReplaceAll(id, "-", ""))
	if err != nil || len(raw) != 16 {
		return nil, false
	}
	return append([]byte{kind}, raw...), true
}

// addressBinding is the 36-byte SignalMessage field 6: sender service ID,
// sender device, recipient service ID, recipient device
// (double-ratchet.md §4). ok is false if either name is not a service ID.
func addressBinding(sender, recipient Address) ([]byte, bool) {
	s, ok1 := serviceIDBinary(sender.ServiceID)
	r, ok2 := serviceIDBinary(recipient.ServiceID)
	if !ok1 || !ok2 {
		return nil, false
	}
	out := append(s, byte(sender.DeviceID))
	out = append(out, r...)
	return append(out, byte(recipient.DeviceID)), true
}

type pendingPreKey struct {
	oneTimePreKeyID *uint32
	signedPreKeyID  uint32
	baseKey         *curve25519.PublicKey
	created         time.Time // whole seconds
}

type pendingKyberPreKey struct {
	kyberPreKeyID uint32
	ciphertext    []byte
}

// SessionRecord is one peer device's sessions: the current state and up
// to MaxArchivedStates archived ones, newest first. Records are values:
// every operation returns a new record, and the caller stores it only on
// success, so a failure changes nothing.
type SessionRecord struct {
	current  *SessionState
	previous []*SessionState
}

// withCurrent installs s as the current state, archiving the old one
// (with its pending prekey info cleared) at the front.
func (r *SessionRecord) withCurrent(s *SessionState) *SessionRecord {
	n := &SessionRecord{current: s}
	if r.current != nil {
		old := r.current.clone()
		old.pending, old.pendingKyber = nil, nil
		n.previous = append(n.previous, old)
	}
	n.previous = append(n.previous, r.previous...)
	if len(n.previous) > MaxArchivedStates {
		n.previous = n.previous[:MaxArchivedStates]
	}
	return n
}

// replaceCurrent swaps in an updated current state without archiving.
func (r *SessionRecord) replaceCurrent(s *SessionState) *SessionRecord {
	return &SessionRecord{current: s, previous: r.previous}
}

// promote makes archived state i current (it may also be an updated
// version of that state), archiving the old current state.
func (r *SessionRecord) promote(i int, s *SessionState) *SessionRecord {
	rest := append(append([]*SessionState(nil), r.previous[:i]...), r.previous[i+1:]...)
	return (&SessionRecord{current: r.current, previous: rest}).withCurrent(s)
}

// NewInitiatorRecord runs PQXDH against a prekey bundle and returns a
// record whose current state sends PreKeySignalMessages until the peer
// replies (pqxdh.md §4).
func NewInitiatorRecord(rand io.Reader, identity *curve25519.PrivateKey, localRegistrationID uint32, b *PreKeyBundle, now time.Time) (*SessionRecord, error) {
	init, err := InitiatePQXDH(rand, identity, b)
	if err != nil {
		return nil, err
	}
	s, err := NewAliceSession(rand, init, identity.PublicKey(), b.IdentityKey, b.SignedPreKey)
	if err != nil {
		return nil, err
	}
	s.localRegistrationID = localRegistrationID
	s.remoteRegistrationID = b.RegistrationID
	s.pending = &pendingPreKey{
		oneTimePreKeyID: init.UsedPreKeyID,
		signedPreKeyID:  init.UsedSignedPreKeyID,
		baseKey:         init.BaseKey,
		created:         now.Truncate(time.Second),
	}
	s.pendingKyber = &pendingKyberPreKey{kyberPreKeyID: init.UsedKyberPreKeyID, ciphertext: init.KyberCiphertext}
	return (&SessionRecord{}).withCurrent(s), nil
}

// Encrypt encrypts plaintext (already padded by the caller) with the
// current state. While the state is unacknowledged the result is a
// PreKeySignalMessage carrying the address binding.
//
// SPQR is not wired in yet: no SPQR key is mixed in and pq_ratchet stays
// empty, so libsignal peers (which require SPQR V1) will reject these
// messages until it is.
func (r *SessionRecord) Encrypt(plaintext []byte, now time.Time, local, remote Address) (*SessionRecord, int, []byte, error) {
	cur := r.current
	if cur == nil {
		return nil, 0, nil, ErrSessionNotFound
	}
	if cur.pending != nil && now.Sub(cur.pending.created) > MaxUnacknowledgedSessionAge {
		return nil, 0, nil, fmt.Errorf("%w: unacknowledged session is older than 30 days", ErrSessionNotFound)
	}
	next, res, err := cur.sendStep(nil)
	if err != nil {
		return nil, 0, nil, err
	}
	ct, err := cbcEncrypt(res.keys.cipherKey, res.keys.iv, plaintext)
	if err != nil {
		return nil, 0, nil, err
	}
	m := &ratchetMessage{
		version:         cur.version,
		ratchetKey:      res.ratchetKey,
		counter:         res.counter,
		previousCounter: res.previousCounter,
		ciphertext:      ct,
	}
	if cur.pending != nil {
		if b, ok := addressBinding(local, remote); ok {
			m.addresses = b
		}
	}
	wire := encodeRatchetMessage(m, res.keys.macKey, cur.localIdentity, cur.remoteIdentity)
	typ := MessageTypeWhisper
	if p := cur.pending; p != nil {
		wire = encodePreKeyMessage(&preKeyMessage{
			version:         cur.version,
			registrationID:  cur.localRegistrationID,
			oneTimePreKeyID: p.oneTimePreKeyID,
			signedPreKeyID:  p.signedPreKeyID,
			kyberPreKeyID:   cur.pendingKyber.kyberPreKeyID,
			kyberCiphertext: cur.pendingKyber.ciphertext,
			baseKey:         p.baseKey,
			identityKey:     cur.localIdentity,
			message:         wire,
		})
		typ = MessageTypePreKey
	}
	return r.replaceCurrent(next), typ, wire, nil
}

var errStateMismatch = errors.New("protocol: message does not fit this session state")

// decryptWith tries one session state. The returned state is a new copy;
// st is unchanged.
func decryptWith(st *SessionState, rand io.Reader, m *ratchetMessage, local, remote Address) (*SessionState, []byte, error) {
	if m.version != st.version {
		return nil, nil, errStateMismatch
	}
	next, keys, err := st.receiveStep(rand, m.ratchetKey, m.counter, nil)
	if err != nil {
		return nil, nil, err
	}
	if !m.verifyMAC(keys.macKey, st.remoteIdentity, st.localIdentity) {
		return nil, nil, errStateMismatch
	}
	if m.addresses != nil {
		want, ok := addressBinding(remote, local)
		if !ok || subtle.ConstantTimeCompare(want, m.addresses) != 1 {
			return nil, nil, errStateMismatch
		}
	}
	pt, err := cbcDecrypt(keys.cipherKey, keys.iv, m.ciphertext)
	if err != nil {
		return nil, nil, errStateMismatch
	}
	next.pending, next.pendingKyber = nil, nil // the peer has heard from us
	return next, pt, nil
}

// Decrypt decrypts a SignalMessage, trying the current state and then the
// archived ones, newest first; an archived state that works is promoted
// (double-ratchet.md §8).
func (r *SessionRecord) Decrypt(rand io.Reader, wire []byte, local, remote Address) (*SessionRecord, []byte, error) {
	m, err := parseRatchetMessage(wire)
	if err != nil {
		return nil, nil, err
	}
	if r.current == nil && len(r.previous) == 0 {
		return nil, nil, ErrSessionNotFound
	}
	if r.current != nil {
		next, pt, err := decryptWith(r.current, rand, m, local, remote)
		switch {
		case err == nil:
			return r.replaceCurrent(next), pt, nil
		case errors.Is(err, ErrDuplicateMessage):
			return nil, nil, err
		}
	}
	for i, st := range r.previous {
		next, pt, err := decryptWith(st, rand, m, local, remote)
		switch {
		case err == nil:
			return r.promote(i, next), pt, nil
		case errors.Is(err, ErrDuplicateMessage):
			return nil, nil, err
		}
	}
	return nil, nil, ErrDecryptionFailed
}

// LocalIdentity is the identity a session is addressed to (ACI or PNI).
type LocalIdentity struct {
	Key            *curve25519.PrivateKey
	RegistrationID uint32
}

// PreKeyLookup loads the private prekeys a PreKeySignalMessage names.
// Implementations return ErrInvalidSignedPreKeyID, ErrInvalidKyberPreKeyID
// or ErrInvalidPreKeyID for unknown IDs.
type PreKeyLookup interface {
	SignedPreKey(id uint32) (*curve25519.PrivateKey, error)
	KyberPreKey(id uint32) (*KEMSecretKey, error)
	PreKey(id uint32) (*curve25519.PrivateKey, error)
}

// UsedPreKeys tells the caller which prekeys a new session consumed. The
// caller removes the one-time prekey and marks the Kyber prekey used
// (signal-cli deletes it unless it is a last-resort key).
type UsedPreKeys struct {
	OneTimePreKeyID *uint32
	SignedPreKeyID  uint32
	KyberPreKeyID   uint32
	BaseKey         *curve25519.PublicKey
}

// DecryptPreKey handles a PreKeySignalMessage as the responder, in the
// order of pqxdh.md §7. trust is the identity check for the sender's
// identity key. On success it returns the new record and, if a new session
// was built, the prekeys it used; the caller then saves the identity,
// handles the prekeys and stores the record. On failure nothing changes.
func (r *SessionRecord) DecryptPreKey(rand io.Reader, wire []byte, localID LocalIdentity, store PreKeyLookup, trust func(*curve25519.PublicKey) bool, local, remote Address) (*SessionRecord, []byte, *UsedPreKeys, error) {
	pm, err := parsePreKeyMessage(wire)
	if err != nil {
		return nil, nil, nil, err
	}
	if !trust(pm.identityKey) {
		return nil, nil, nil, ErrUntrustedIdentity
	}

	rec, used := r.matchBaseKey(pm), (*UsedPreKeys)(nil)
	if rec == nil {
		if rec, used, err = r.newResponderState(pm, localID, store); err != nil {
			return nil, nil, nil, err
		}
	}
	next, pt, err := decryptWith(rec.current, rand, pm.inner, local, remote)
	switch {
	case errors.Is(err, ErrDuplicateMessage):
		return nil, nil, nil, err
	case err != nil:
		return nil, nil, nil, ErrDecryptionFailed
	}
	return rec.replaceCurrent(next), pt, used, nil
}

// matchBaseKey finds a state for this message's base key (pqxdh.md §7 step
// 3), promoting an archived one. It returns nil if there is none.
func (r *SessionRecord) matchBaseKey(pm *preKeyMessage) *SessionRecord {
	matches := func(st *SessionState) bool {
		return st.version == pm.version && st.aliceBaseKey != nil &&
			subtle.ConstantTimeCompare(st.aliceBaseKey.Bytes(), pm.baseKey.Bytes()) == 1
	}
	if r.current != nil && matches(r.current) {
		return r
	}
	for i, st := range r.previous {
		if matches(st) {
			return r.promote(i, st)
		}
	}
	return nil
}

// newResponderState builds a new current state from the message's prekeys
// (pqxdh.md §7 steps 4-9).
func (r *SessionRecord) newResponderState(pm *preKeyMessage, localID LocalIdentity, store PreKeyLookup) (*SessionRecord, *UsedPreKeys, error) {
	if pm.version == 3 {
		return nil, nil, fmt.Errorf("%w: X3DH no longer supported", ErrInvalidMessage)
	}
	spk, err := store.SignedPreKey(pm.signedPreKeyID)
	if err != nil {
		return nil, nil, err
	}
	if pm.kyberCiphertext == nil {
		return nil, nil, fmt.Errorf("%w: missing pq ciphertext", ErrInvalidMessage)
	}
	kem, err := store.KyberPreKey(pm.kyberPreKeyID)
	if err != nil {
		return nil, nil, err
	}
	var otk *curve25519.PrivateKey
	if pm.oneTimePreKeyID != nil {
		if otk, err = store.PreKey(*pm.oneTimePreKeyID); err != nil {
			return nil, nil, err
		}
	}
	secrets, err := RespondPQXDH(PQXDHResponderKeys{
		Identity: localID.Key, SignedPreKey: spk, PreKey: otk, KyberPreKey: kem,
	}, pm.identityKey, pm.baseKey, pm.kyberCiphertext)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %v", ErrInvalidMessage, err)
	}
	st := NewBobSession(secrets, spk, localID.Key.PublicKey(), pm.identityKey, pm.baseKey)
	st.localRegistrationID = localID.RegistrationID
	st.remoteRegistrationID = pm.registrationID
	used := &UsedPreKeys{
		OneTimePreKeyID: pm.oneTimePreKeyID,
		SignedPreKeyID:  pm.signedPreKeyID,
		KyberPreKeyID:   pm.kyberPreKeyID,
		BaseKey:         pm.baseKey,
	}
	return r.withCurrent(st), used, nil
}
