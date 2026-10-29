// SPDX-License-Identifier: GPL-3.0-or-later

package protocol

import (
	"crypto/subtle"
	"errors"
	"fmt"

	"google.golang.org/protobuf/proto"

	"github.com/aren55555/signal-cli-go/internal/crypto/curve25519"
	"github.com/aren55555/signal-cli-go/internal/proto/signalpb"
)

// Message parse errors, matching libsignal's error kinds
// (double-ratchet.md §4, pqxdh.md §5).
var (
	ErrMessageTooShort     = errors.New("protocol: ciphertext message too short")
	ErrLegacyVersion       = errors.New("protocol: legacy ciphertext version")
	ErrUnrecognizedVersion = errors.New("protocol: unrecognized ciphertext version")
	ErrInvalidMessage      = errors.New("protocol: invalid protobuf encoding")
)

// currentVersion is the low nibble of every version byte libsignal writes.
const currentVersion = 4

func versionByte(sessionVersion uint8) byte { return sessionVersion<<4 | currentVersion }

// checkVersion reads the session version from the high nibble; the low
// nibble is ignored.
func checkVersion(b byte) (uint8, error) {
	switch v := b >> 4; {
	case v < 3:
		return 0, fmt.Errorf("%w %d", ErrLegacyVersion, v)
	case v > 4:
		return 0, fmt.Errorf("%w %d", ErrUnrecognizedVersion, v)
	default:
		return v, nil
	}
}

func parseKey(b []byte, what string) (*curve25519.PublicKey, error) {
	k, err := curve25519.ParsePublicKey(b)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrInvalidMessage, what, err)
	}
	return k, nil
}

// ratchetMessage is a SignalMessage (ciphertext message type 2): version
// byte, protobuf, 8-byte MAC.
type ratchetMessage struct {
	version         uint8
	ratchetKey      *curve25519.PublicKey
	counter         uint32
	previousCounter uint32
	ciphertext      []byte
	pqRatchet       []byte // empty: field omitted
	addresses       []byte // nil: field omitted

	// Set by parseRatchetMessage: the MAC input exactly as received
	// (version byte and protobuf) and the received MAC.
	serialized []byte
	mac        []byte
}

// encodeRatchetMessage serializes m and appends its MAC. Fields are written
// in ascending order, present zero values included.
func encodeRatchetMessage(m *ratchetMessage, macKey []byte, sender, receiver *curve25519.PublicKey) []byte {
	pb := &signalpb.RatchetMessage{
		RatchetKey:      m.ratchetKey.Bytes(),
		Counter:         proto.Uint32(m.counter),
		PreviousCounter: proto.Uint32(m.previousCounter),
		Ciphertext:      m.ciphertext,
	}
	if len(m.pqRatchet) > 0 {
		pb.PqRatchet = m.pqRatchet
	}
	if m.addresses != nil {
		pb.Addresses = m.addresses
	}
	body, err := proto.MarshalOptions{Deterministic: true}.Marshal(pb)
	if err != nil {
		panic(err) // marshalling a well-formed message can't fail
	}
	serialized := append([]byte{versionByte(m.version)}, body...)
	return append(serialized, messageMAC(macKey, sender, receiver, serialized)...)
}

// parseRatchetMessage parses a serialized SignalMessage. It does not check
// the MAC; that needs the message keys (verifyMAC).
func parseRatchetMessage(b []byte) (*ratchetMessage, error) {
	if len(b) < 1+macSize {
		return nil, ErrMessageTooShort
	}
	version, err := checkVersion(b[0])
	if err != nil {
		return nil, err
	}
	var pb signalpb.RatchetMessage
	if err := proto.Unmarshal(b[1:len(b)-macSize], &pb); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidMessage, err)
	}
	if pb.RatchetKey == nil || pb.Counter == nil || pb.Ciphertext == nil {
		return nil, fmt.Errorf("%w: missing required field", ErrInvalidMessage)
	}
	key, err := parseKey(pb.RatchetKey, "ratchet key")
	if err != nil {
		return nil, err
	}
	return &ratchetMessage{
		version:         version,
		ratchetKey:      key,
		counter:         pb.GetCounter(),
		previousCounter: pb.GetPreviousCounter(),
		ciphertext:      pb.Ciphertext,
		pqRatchet:       pb.PqRatchet,
		addresses:       pb.Addresses,
		serialized:      b[: len(b)-macSize : len(b)-macSize],
		mac:             b[len(b)-macSize:],
	}, nil
}

// verifyMAC checks the received MAC over the received bytes.
func (m *ratchetMessage) verifyMAC(macKey []byte, sender, receiver *curve25519.PublicKey) bool {
	want := messageMAC(macKey, sender, receiver, m.serialized)
	return subtle.ConstantTimeCompare(want, m.mac) == 1
}

// preKeyMessage is a PreKeySignalMessage (ciphertext message type 3):
// version byte and protobuf, no MAC of its own.
type preKeyMessage struct {
	version         uint8
	registrationID  uint32
	oneTimePreKeyID *uint32 // nil: no one-time prekey (0 is a valid ID)
	signedPreKeyID  uint32
	kyberPreKeyID   uint32
	kyberCiphertext []byte // with its 0x08 type byte
	baseKey         *curve25519.PublicKey
	identityKey     *curve25519.PublicKey
	message         []byte // the serialized SignalMessage

	inner *ratchetMessage // set by parsePreKeyMessage
}

func encodePreKeyMessage(m *preKeyMessage) []byte {
	pb := &signalpb.PreKeyRatchetMessage{
		OneTimePreKeyId: m.oneTimePreKeyID,
		BaseKey:         m.baseKey.Bytes(),
		IdentityKey:     m.identityKey.Bytes(),
		RatchetMessage:  m.message,
		RegistrationId:  proto.Uint32(m.registrationID),
		SignedPreKeyId:  proto.Uint32(m.signedPreKeyID),
		KyberPreKeyId:   proto.Uint32(m.kyberPreKeyID),
		KyberCiphertext: m.kyberCiphertext,
	}
	body, err := proto.MarshalOptions{Deterministic: true}.Marshal(pb)
	if err != nil {
		panic(err)
	}
	return append([]byte{versionByte(m.version)}, body...)
}

func parsePreKeyMessage(b []byte) (*preKeyMessage, error) {
	if len(b) == 0 {
		return nil, ErrMessageTooShort
	}
	version, err := checkVersion(b[0])
	if err != nil {
		return nil, err
	}
	var pb signalpb.PreKeyRatchetMessage
	if err := proto.Unmarshal(b[1:], &pb); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidMessage, err)
	}
	if pb.BaseKey == nil || pb.IdentityKey == nil || pb.RatchetMessage == nil || pb.SignedPreKeyId == nil {
		return nil, fmt.Errorf("%w: missing required field", ErrInvalidMessage)
	}
	hasID, hasCT := pb.KyberPreKeyId != nil, pb.KyberCiphertext != nil
	switch {
	case hasID != hasCT:
		return nil, fmt.Errorf("%w: Kyber prekey ID and ciphertext must come together", ErrInvalidMessage)
	case !hasID && version > 3:
		return nil, fmt.Errorf("%w: Kyber pre key must be present", ErrInvalidMessage)
	}
	base, err := parseKey(pb.BaseKey, "base key")
	if err != nil {
		return nil, err
	}
	identity, err := parseKey(pb.IdentityKey, "identity key")
	if err != nil {
		return nil, err
	}
	inner, err := parseRatchetMessage(pb.RatchetMessage)
	if err != nil {
		return nil, err
	}
	return &preKeyMessage{
		version:         version,
		registrationID:  pb.GetRegistrationId(),
		oneTimePreKeyID: pb.OneTimePreKeyId,
		signedPreKeyID:  pb.GetSignedPreKeyId(),
		kyberPreKeyID:   pb.GetKyberPreKeyId(),
		kyberCiphertext: pb.KyberCiphertext,
		baseKey:         base,
		identityKey:     identity,
		message:         pb.RatchetMessage,
		inner:           inner,
	}, nil
}
