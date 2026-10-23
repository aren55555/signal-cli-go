package protocol

import (
	"crypto/hkdf"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"

	"google.golang.org/protobuf/proto"

	"github.com/aren55555/signal-cli-go/internal/crypto/curve25519"
	"github.com/aren55555/signal-cli-go/internal/proto/signalpb"
)

// Sender keys for group messages (spec/protocol/sender-keys.md).

// DistributionID names a sender-key audience (a UUID, big-endian bytes).
type DistributionID [16]byte

const (
	MaxSenderKeyStates = 5
	senderKeyVersion   = 3
	senderKeyInfo      = "WhisperGroup"
	signatureSize      = 64
)

var (
	ErrNoSenderKeyState = errors.New("protocol: no sender key state")
	ErrSignatureInvalid = fmt.Errorf("%w: signature validation failed", ErrInvalidMessage)
)

// senderMessageKeys expands a seed: HKDF-SHA256, no salt, info
// "WhisperGroup", 48 bytes split IV (16) then AES key (32), the reverse of
// the 1:1 order.
func senderMessageKeys(seed []byte) (iv, key []byte) {
	out, err := hkdf.Key(sha256.New, seed, nil, senderKeyInfo, 48)
	if err != nil {
		panic(err)
	}
	return out[:16], out[16:]
}

type senderKeyState struct {
	version     uint8
	chainID     uint32
	iteration   uint32 // the next iteration to use
	chainKey    []byte // the chain key at iteration
	signingPub  *curve25519.PublicKey
	signingPriv *curve25519.PrivateKey // nil on receivers
	skipped     []skippedSeed          // oldest first, at most MaxMessageKeys
}

func (s *senderKeyState) clone() *senderKeyState {
	c := *s
	c.skipped = append([]skippedSeed(nil), s.skipped...)
	return &c
}

// SenderKeyRecord holds the sender-key states for one (sender address,
// distribution ID), newest first. Like SessionRecord it is a value: every
// operation returns a new record and the caller stores it on success.
type SenderKeyRecord struct {
	states []*senderKeyState
}

// withFront returns a record with s at the front, then the other states
// except those for which drop reports true, capped at MaxSenderKeyStates.
func (r *SenderKeyRecord) withFront(s *senderKeyState, drop func(*senderKeyState) bool) *SenderKeyRecord {
	var rest []*senderKeyState
	for _, o := range r.states {
		if !drop(o) {
			rest = append(rest, o)
		}
	}
	for len(rest) >= MaxSenderKeyStates {
		rest = rest[:len(rest)-1]
	}
	return &SenderKeyRecord{states: append([]*senderKeyState{s}, rest...)}
}

func checkSenderKeyVersion(b byte) error {
	switch v := b >> 4; {
	case v < senderKeyVersion:
		return fmt.Errorf("%w %d", ErrLegacyVersion, v)
	case v > senderKeyVersion:
		return fmt.Errorf("%w %d", ErrUnrecognizedVersion, v)
	}
	return nil
}

func senderVersionByte() byte { return senderKeyVersion<<4 | senderKeyVersion }

// NewDistribution returns a SenderKeyDistributionMessage for this record,
// first creating our sending state if there is none (sender-keys.md §3).
func (r *SenderKeyRecord) NewDistribution(rand io.Reader, id DistributionID) (*SenderKeyRecord, []byte, error) {
	rec := r
	if len(r.states) == 0 {
		var b [4 + 32]byte
		if _, err := io.ReadFull(rand, b[:]); err != nil {
			return nil, nil, err
		}
		priv, err := curve25519.GeneratePrivateKey(rand)
		if err != nil {
			return nil, nil, err
		}
		s := &senderKeyState{
			version:     senderKeyVersion,
			chainID:     binary.BigEndian.Uint32(b[:4]) >> 1,
			chainKey:    append([]byte(nil), b[4:]...),
			signingPub:  priv.PublicKey(),
			signingPriv: priv,
		}
		rec = &SenderKeyRecord{states: []*senderKeyState{s}}
	}
	s := rec.states[0]
	body, err := proto.MarshalOptions{Deterministic: true}.Marshal(&signalpb.GroupSenderKeyHandoff{
		DistributionId: id[:],
		ChainId:        proto.Uint32(s.chainID),
		Iteration:      proto.Uint32(s.iteration),
		ChainKey:       s.chainKey,
		SigningKey:     s.signingPub.Bytes(),
	})
	if err != nil {
		return nil, nil, err
	}
	return rec, append([]byte{senderVersionByte()}, body...), nil
}

// ProcessDistribution adds a received SenderKeyDistributionMessage to the
// record for its sender and returns the distribution ID it names.
func (r *SenderKeyRecord) ProcessDistribution(b []byte) (*SenderKeyRecord, DistributionID, error) {
	var id DistributionID
	if len(b) < 1+signatureSize {
		return nil, id, ErrMessageTooShort
	}
	if err := checkSenderKeyVersion(b[0]); err != nil {
		return nil, id, err
	}
	var pb signalpb.GroupSenderKeyHandoff
	if err := proto.Unmarshal(b[1:], &pb); err != nil {
		return nil, id, fmt.Errorf("%w: %v", ErrInvalidMessage, err)
	}
	if pb.DistributionId == nil || pb.ChainId == nil || pb.Iteration == nil || pb.ChainKey == nil || pb.SigningKey == nil {
		return nil, id, fmt.Errorf("%w: missing required field", ErrInvalidMessage)
	}
	if len(pb.DistributionId) != 16 || len(pb.ChainKey) != 32 || len(pb.SigningKey) != 33 {
		return nil, id, fmt.Errorf("%w: wrong field length", ErrInvalidMessage)
	}
	signer, err := parseKey(pb.SigningKey, "signing key")
	if err != nil {
		return nil, id, err
	}
	copy(id[:], pb.DistributionId)
	chainID := pb.GetChainId()

	for _, s := range r.states {
		if s.chainID == chainID && string(s.signingPub.Bytes()) == string(signer.Bytes()) {
			// Known state: keep its position and skipped keys, move it to
			// the front.
			return r.withFront(s, func(o *senderKeyState) bool { return o == s }), id, nil
		}
	}
	s := &senderKeyState{
		version:    senderKeyVersion,
		chainID:    chainID,
		iteration:  pb.GetIteration(),
		chainKey:   pb.ChainKey,
		signingPub: signer,
	}
	return r.withFront(s, func(o *senderKeyState) bool { return o.chainID == chainID }), id, nil
}

// Encrypt encrypts a group message with our newest sending state
// (sender-keys.md §5).
func (r *SenderKeyRecord) Encrypt(rand io.Reader, id DistributionID, plaintext []byte) (*SenderKeyRecord, []byte, error) {
	if len(r.states) == 0 || r.states[0].signingPriv == nil {
		return nil, nil, ErrNoSenderKeyState
	}
	s := r.states[0].clone()
	if s.iteration == math.MaxUint32 {
		return nil, nil, errors.New("protocol: sender chain is too long")
	}
	seed, next := chainStep(s.chainKey)
	iv, key := senderMessageKeys(seed)
	ct, err := cbcEncrypt(key, iv, plaintext)
	if err != nil {
		return nil, nil, err
	}
	body, err := proto.MarshalOptions{Deterministic: true}.Marshal(&signalpb.GroupSenderMessage{
		DistributionId: id[:],
		ChainId:        proto.Uint32(s.chainID),
		Iteration:      proto.Uint32(s.iteration),
		Ciphertext:     ct,
	})
	if err != nil {
		return nil, nil, err
	}
	signed := append([]byte{senderVersionByte()}, body...)
	sig, err := s.signingPriv.Sign(rand, signed)
	if err != nil {
		return nil, nil, err
	}
	s.chainKey, s.iteration = next, s.iteration+1
	rec := &SenderKeyRecord{states: append([]*senderKeyState{s}, r.states[1:]...)}
	return rec, append(signed, sig...), nil
}

// Decrypt decrypts a SenderKeyMessage (sender-keys.md §6).
func (r *SenderKeyRecord) Decrypt(b []byte) (*SenderKeyRecord, []byte, error) {
	if len(b) < 1+signatureSize {
		return nil, nil, ErrMessageTooShort
	}
	if err := checkSenderKeyVersion(b[0]); err != nil {
		return nil, nil, err
	}
	signed, sig := b[:len(b)-signatureSize], b[len(b)-signatureSize:]
	var pb signalpb.GroupSenderMessage
	if err := proto.Unmarshal(signed[1:], &pb); err != nil {
		return nil, nil, fmt.Errorf("%w: %v", ErrInvalidMessage, err)
	}
	if pb.DistributionId == nil || pb.ChainId == nil || pb.Iteration == nil || pb.Ciphertext == nil || len(pb.DistributionId) != 16 {
		return nil, nil, fmt.Errorf("%w: missing or malformed field", ErrInvalidMessage)
	}
	idx := -1
	for i, s := range r.states {
		if s.chainID == pb.GetChainId() {
			idx = i
			break
		}
	}
	if idx < 0 {
		return nil, nil, ErrNoSenderKeyState
	}
	s := r.states[idx].clone()
	if b[0]>>4 != s.version {
		return nil, nil, fmt.Errorf("%w: message version %d, state %d", ErrUnrecognizedVersion, b[0]>>4, s.version)
	}
	if !s.signingPub.Verify(signed, sig) {
		return nil, nil, ErrSignatureInvalid
	}
	seed, err := s.seedFor(pb.GetIteration())
	if err != nil {
		return nil, nil, err
	}
	iv, key := senderMessageKeys(seed)
	pt, err := cbcDecrypt(key, iv, pb.Ciphertext)
	if err != nil {
		return nil, nil, ErrDecryptionFailed
	}
	states := append([]*senderKeyState(nil), r.states...)
	states[idx] = s
	return &SenderKeyRecord{states: states}, pt, nil
}

// seedFor returns the message-key seed for iteration j.
func (s *senderKeyState) seedFor(j uint32) ([]byte, error) {
	i := s.iteration
	if j < i {
		for k, sk := range s.skipped {
			if sk.index == j {
				s.skipped = append(s.skipped[:k], s.skipped[k+1:]...)
				return sk.seed, nil
			}
		}
		return nil, fmt.Errorf("%w: iteration %d, chain at %d", ErrDuplicateMessage, j, i)
	}
	if j-i > MaxForwardJumps {
		return nil, fmt.Errorf("%w: message from too far into the future", ErrInvalidMessage)
	}
	key := s.chainKey
	for it := i; it < j; it++ {
		seed, next := chainStep(key)
		s.skipped = append(s.skipped, skippedSeed{index: it, seed: seed})
		if len(s.skipped) > MaxMessageKeys {
			s.skipped = s.skipped[1:]
		}
		key = next
	}
	seed, next := chainStep(key)
	s.chainKey, s.iteration = next, j+1
	return seed, nil
}
