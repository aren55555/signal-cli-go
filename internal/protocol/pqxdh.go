// SPDX-License-Identifier: GPL-3.0-or-later

package protocol

import (
	"bytes"
	"crypto/hkdf"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"

	"github.com/aren55555/signal-cli-go/internal/crypto/curve25519"
)

// PQXDH parameters as libsignal instantiates the PQXDH specification
// (signal.org, revision 3): curve25519, SHA-256, Kyber1024, and this info
// string (spec/crypto/kem.md).
const pqxdhInfo = "WhisperText_X25519_SHA-256_CRYSTALS-KYBER-1024"

// PQXDHSecrets is the 96-byte key-schedule output, split as libsignal
// splits it (spec/protocol/pqxdh.md §3).
type PQXDHSecrets struct {
	RootKey  []byte // bytes 0..31
	ChainKey []byte // bytes 32..63: a ready-made chain on Bob's signed prekey
	PQRKey   []byte // bytes 64..95: the SPQR auth-key seed
}

// PreKeyBundle is the responder's published keys, as fetched by the
// initiator.
type PreKeyBundle struct {
	RegistrationID        uint32
	IdentityKey           *curve25519.PublicKey
	SignedPreKeyID        uint32
	SignedPreKey          *curve25519.PublicKey
	SignedPreKeySignature []byte
	PreKeyID              *uint32 // nil when the bundle has no one-time prekey
	PreKey                *curve25519.PublicKey
	KyberPreKeyID         uint32
	KyberPreKey           *KEMPublicKey
	KyberPreKeySignature  []byte
}

// PQXDHInitiation is the initiator's result: the shared secret and what
// the initial message must carry.
type PQXDHInitiation struct {
	Secrets            *PQXDHSecrets
	BaseKey            *curve25519.PublicKey // EK_A
	BaseKeyPrivate     *curve25519.PrivateKey
	KyberCiphertext    []byte // serialized, with the 0x08 type byte
	UsedPreKeyID       *uint32
	UsedSignedPreKeyID uint32
	UsedKyberPreKeyID  uint32
}

// pqxdhKDF is the key schedule: HKDF-SHA256 with IKM = 32 bytes of 0xFF ||
// km, no salt, the info string, and 96 bytes of output (the specification
// takes 32; libsignal takes three keys at once).
func pqxdhKDF(km []byte) (*PQXDHSecrets, error) {
	ikm := append(bytes.Repeat([]byte{0xff}, 32), km...)
	out, err := hkdf.Key(sha256.New, ikm, nil, pqxdhInfo, 96)
	if err != nil {
		return nil, err
	}
	return &PQXDHSecrets{RootKey: out[:32], ChainKey: out[32:64], PQRKey: out[64:]}, nil
}

// InitiatePQXDH runs the initiator side (section 3.3): verify both prekey
// signatures, generate the ephemeral key EK_A (the first draw from rand),
// encapsulate to the Kyber prekey, and derive SK from
// DH1 = DH(IK_A, SPK_B), DH2 = DH(EK_A, IK_B), DH3 = DH(EK_A, SPK_B),
// optionally DH4 = DH(EK_A, OPK_B), then SS.
func InitiatePQXDH(rand io.Reader, identity *curve25519.PrivateKey, b *PreKeyBundle) (*PQXDHInitiation, error) {
	if b.IdentityKey == nil || b.SignedPreKey == nil {
		return nil, errors.New("protocol: incomplete prekey bundle")
	}
	if b.KyberPreKey == nil {
		return nil, errors.New("protocol: kyber_pre_key is required")
	}
	if (b.PreKeyID == nil) != (b.PreKey == nil) {
		return nil, errors.New("protocol: one-time prekey without its ID, or the reverse")
	}
	if !b.IdentityKey.Verify(b.SignedPreKey.Bytes(), b.SignedPreKeySignature) {
		return nil, errors.New("protocol: invalid signed prekey signature")
	}
	if !b.IdentityKey.Verify(b.KyberPreKey.Bytes(), b.KyberPreKeySignature) {
		return nil, errors.New("protocol: invalid kyber prekey signature")
	}

	ek, err := curve25519.GeneratePrivateKey(rand)
	if err != nil {
		return nil, err
	}
	ct, ss, err := b.KyberPreKey.Encapsulate(rand)
	if err != nil {
		return nil, err
	}

	var km []byte
	for _, p := range []struct {
		priv *curve25519.PrivateKey
		pub  *curve25519.PublicKey
	}{{identity, b.SignedPreKey}, {ek, b.IdentityKey}, {ek, b.SignedPreKey}, {ek, b.PreKey}} {
		if p.pub == nil {
			continue // no one-time prekey: DH4 is omitted
		}
		s, err := p.priv.Agree(p.pub)
		if err != nil {
			return nil, err
		}
		km = append(km, s...)
	}
	secrets, err := pqxdhKDF(append(km, ss...))
	if err != nil {
		return nil, err
	}
	return &PQXDHInitiation{
		Secrets:            secrets,
		BaseKey:            ek.PublicKey(),
		BaseKeyPrivate:     ek,
		KyberCiphertext:    ct,
		UsedPreKeyID:       b.PreKeyID,
		UsedSignedPreKeyID: b.SignedPreKeyID,
		UsedKyberPreKeyID:  b.KyberPreKeyID,
	}, nil
}

// PQXDHResponderKeys are the responder's private keys named by the
// initial message. PreKey is nil when the initiator used no one-time
// prekey.
type PQXDHResponderKeys struct {
	Identity     *curve25519.PrivateKey
	SignedPreKey *curve25519.PrivateKey
	PreKey       *curve25519.PrivateKey
	KyberPreKey  *KEMSecretKey
}

// RespondPQXDH runs the responder side (section 3.4) and returns the
// key-schedule output. As in
// libsignal, the initiator's base key must be canonical (in range and
// torsion-free) before anything is computed.
func RespondPQXDH(keys PQXDHResponderKeys, theirIdentity, baseKey *curve25519.PublicKey, kyberCiphertext []byte) (*PQXDHSecrets, error) {
	if !curve25519.IsCanonical(baseKey.U()) {
		return nil, errors.New("protocol: incoming base key is invalid")
	}
	ss, err := keys.KyberPreKey.Decapsulate(kyberCiphertext)
	if err != nil {
		return nil, err
	}
	var km []byte
	for _, p := range []struct {
		priv *curve25519.PrivateKey
		pub  *curve25519.PublicKey
	}{{keys.SignedPreKey, theirIdentity}, {keys.Identity, baseKey}, {keys.SignedPreKey, baseKey}, {keys.PreKey, baseKey}} {
		if p.priv == nil {
			continue
		}
		s, err := p.priv.Agree(p.pub)
		if err != nil {
			return nil, fmt.Errorf("protocol: key agreement: %w", err)
		}
		km = append(km, s...)
	}
	return pqxdhKDF(append(km, ss...))
}
