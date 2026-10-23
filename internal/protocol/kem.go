// Package protocol implements the Signal protocol core: PQXDH session
// setup and the Double Ratchet, following Signal's published
// specifications (signal.org/docs) and the libsignal v0.99.1 specifics in
// spec/protocol/ and spec/crypto/.
package protocol

import (
	"errors"
	"fmt"
	"io"

	"github.com/aren55555/signal-cli-go/internal/crypto/kyber"
)

// KEMTypeKyber1024 is libsignal's type byte for round-3 Kyber1024 keys and
// ciphertexts, the only KEM type its shipped build accepts
// (spec/crypto/kem.md).
const KEMTypeKyber1024 = 0x08

// kemTyped checks the type byte and exact length of a serialized KEM
// object and returns the raw bytes after the type byte.
func kemTyped(b []byte, rawLen int, what string) ([]byte, error) {
	switch {
	case len(b) == 0:
		return nil, errors.New("protocol: no key type identifier")
	case b[0] != KEMTypeKyber1024:
		return nil, fmt.Errorf("protocol: bad KEM key type %#02x", b[0])
	case len(b) != 1+rawLen:
		return nil, fmt.Errorf("protocol: KEM %s is %d bytes, want %d", what, len(b), 1+rawLen)
	}
	return b[1:], nil
}

// KEMPublicKey is a Kyber1024 public key with libsignal's framing.
type KEMPublicKey struct {
	ek *kyber.EncapsulationKey1024
}

// ParseKEMPublicKey parses a serialized public key: 0x08 then 1568 bytes.
func ParseKEMPublicKey(b []byte) (*KEMPublicKey, error) {
	raw, err := kemTyped(b, kyber.EncapsulationKeySize1024, "public key")
	if err != nil {
		return nil, err
	}
	ek, err := kyber.NewEncapsulationKey1024(raw)
	if err != nil {
		return nil, err
	}
	return &KEMPublicKey{ek}, nil
}

// Bytes returns the 1569-byte serialized form. Prekey signatures cover
// exactly these bytes.
func (k *KEMPublicKey) Bytes() []byte {
	return append([]byte{KEMTypeKyber1024}, k.ek.Bytes()...)
}

// Encapsulate returns a serialized ciphertext (0x08 then 1568 bytes) and
// the 32-byte shared secret.
func (k *KEMPublicKey) Encapsulate(rand io.Reader) (ciphertext, sharedSecret []byte, err error) {
	ct, ss, err := k.ek.Encapsulate(rand)
	if err != nil {
		return nil, nil, err
	}
	return append([]byte{KEMTypeKyber1024}, ct...), ss, nil
}

// KEMSecretKey is a Kyber1024 secret key in libsignal's framing: 0x08
// then the expanded 3168-byte key.
type KEMSecretKey struct {
	dk *kyber.DecapsulationKey1024
}

// NewKEMSecretKey wraps a decapsulation key.
func NewKEMSecretKey(dk *kyber.DecapsulationKey1024) *KEMSecretKey { return &KEMSecretKey{dk} }

// ParseKEMSecretKey parses a serialized secret key.
func ParseKEMSecretKey(b []byte) (*KEMSecretKey, error) {
	raw, err := kemTyped(b, kyber.DecapsulationKeySize1024, "secret key")
	if err != nil {
		return nil, err
	}
	dk, err := kyber.NewDecapsulationKey1024(raw)
	if err != nil {
		return nil, err
	}
	return &KEMSecretKey{dk}, nil
}

// Bytes returns the 3169-byte serialized form.
func (k *KEMSecretKey) Bytes() []byte {
	return append([]byte{KEMTypeKyber1024}, k.dk.Bytes()...)
}

// PublicKey returns the matching public key.
func (k *KEMSecretKey) PublicKey() *KEMPublicKey { return &KEMPublicKey{k.dk.EncapsulationKey()} }

// Decapsulate checks the ciphertext's type byte and length and returns the
// shared secret.
func (k *KEMSecretKey) Decapsulate(ciphertext []byte) ([]byte, error) {
	raw, err := kemTyped(ciphertext, kyber.CiphertextSize1024, "ciphertext")
	if err != nil {
		return nil, err
	}
	return k.dk.Decapsulate(raw)
}
