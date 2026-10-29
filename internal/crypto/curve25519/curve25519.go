// SPDX-License-Identifier: GPL-3.0-or-later

// Package curve25519 implements Signal's Curve25519 key layer: the
// 0x05-prefixed public key encoding, clamped private keys, X25519
// agreement, XEd25519 signatures, and the canonical-key check PQXDH
// applies to an initiator's base key.
//
// XEd25519 follows Signal's XEdDSA specification (signal.org/docs,
// revision 1) with the libsignal v0.99.1 deviations recorded in
// spec/crypto/xeddsa.md:
//
//   - the private scalar is never negated; the real sign bit of the
//     Edwards public key travels in bit 7 of signature byte 63;
//   - the nonce hash takes the clamped private key bytes, not the reduced
//     scalar;
//   - verification accepts a non-canonical u (bit 255 ignored, reduced
//     mod p) and an unreduced S below 2^253.
//
// VXEdDSA is not implemented: libsignal v0.99.1 doesn't use it.
package curve25519

import (
	"crypto/ecdh"
	"crypto/sha512"
	"crypto/subtle"
	"errors"
	"fmt"
	"io"

	"github.com/aren55555/signal-cli-go/internal/crypto/edwards25519"
	"github.com/aren55555/signal-cli-go/internal/crypto/edwards25519/field"
)

// KeyType is the type byte of a serialized Curve25519 public key.
const KeyType = 0x05

// PrivateKey is a clamped Curve25519 private key.
type PrivateKey struct {
	k [32]byte
}

// PublicKey is a Curve25519 public key: a Montgomery u-coordinate.
type PublicKey struct {
	u [32]byte
}

// NewPrivateKey returns the private key with the 32 bytes of b, clamped
// as in RFC 7748. libsignal stores and uses the clamped value.
func NewPrivateKey(b []byte) (*PrivateKey, error) {
	if len(b) != 32 {
		return nil, fmt.Errorf("curve25519: private key is %d bytes, want 32", len(b))
	}
	k := new(PrivateKey)
	copy(k.k[:], b)
	k.k[0] &= 248
	k.k[31] &= 127
	k.k[31] |= 64
	return k, nil
}

// GeneratePrivateKey returns a new private key from 32 bytes of rand.
func GeneratePrivateKey(rand io.Reader) (*PrivateKey, error) {
	var b [32]byte
	if _, err := io.ReadFull(rand, b[:]); err != nil {
		return nil, err
	}
	return NewPrivateKey(b[:])
}

// Bytes returns the clamped 32-byte private key.
func (k *PrivateKey) Bytes() []byte { return append([]byte(nil), k.k[:]...) }

func (k *PrivateKey) ecdh() *ecdh.PrivateKey {
	priv, err := ecdh.X25519().NewPrivateKey(k.k[:])
	if err != nil {
		panic(err) // any 32 bytes are a valid X25519 key
	}
	return priv
}

// PublicKey returns the public key X25519(k, 9).
func (k *PrivateKey) PublicKey() *PublicKey {
	p := new(PublicKey)
	copy(p.u[:], k.ecdh().PublicKey().Bytes())
	return p
}

// Agree returns the X25519 shared secret with peer. An all-zero result
// (a low-order peer key) is an error, as in libsignal.
func (k *PrivateKey) Agree(peer *PublicKey) ([]byte, error) {
	pub, err := ecdh.X25519().NewPublicKey(peer.u[:])
	if err != nil {
		return nil, err
	}
	// crypto/ecdh checks for the all-zero output in constant time.
	shared, err := k.ecdh().ECDH(pub)
	if err != nil {
		return nil, errors.New("curve25519: invalid key agreement")
	}
	return shared, nil
}

// ParsePublicKey parses a serialized public key: the type byte 0x05 then
// the 32-byte u. Like libsignal, it ignores bytes after the 33rd.
func ParsePublicKey(b []byte) (*PublicKey, error) {
	switch {
	case len(b) == 0:
		return nil, errors.New("curve25519: no key type identifier")
	case b[0] != KeyType:
		return nil, fmt.Errorf("curve25519: bad key type %#02x", b[0])
	case len(b) < 33:
		return nil, fmt.Errorf("curve25519: public key is %d bytes, want 33", len(b))
	}
	p := new(PublicKey)
	copy(p.u[:], b[1:33])
	return p, nil
}

// NewPublicKey returns the public key with the raw 32-byte u.
func NewPublicKey(u []byte) (*PublicKey, error) {
	if len(u) != 32 {
		return nil, fmt.Errorf("curve25519: public key is %d bytes, want 32", len(u))
	}
	p := new(PublicKey)
	copy(p.u[:], u)
	return p, nil
}

// Bytes returns the 33-byte serialized form, 0x05 || u.
func (p *PublicKey) Bytes() []byte { return append([]byte{KeyType}, p.u[:]...) }

// U returns the raw 32-byte u-coordinate.
func (p *PublicKey) U() []byte { return append([]byte(nil), p.u[:]...) }

// edwards converts u to the Edwards point with y = (u-1)/(u+1) and the
// given sign bit. u is read as RFC 7748 does (bit 255 ignored, reduced
// mod p). u = -1 has no image.
func edwards(u []byte, sign byte) (*edwards25519.Point, []byte, error) {
	ue, err := new(field.Element).SetBytes(u)
	if err != nil {
		return nil, nil, err
	}
	one := new(field.Element).One()
	den := new(field.Element).Add(ue, one)
	if den.Equal(new(field.Element)) == 1 {
		return nil, nil, errors.New("curve25519: u = -1 has no Edwards point")
	}
	y := new(field.Element).Multiply(new(field.Element).Subtract(ue, one), den.Invert(den))
	enc := y.Bytes()
	enc[31] |= sign << 7
	A, err := edwards25519.NewIdentityPoint().SetBytes(enc)
	if err != nil {
		return nil, nil, err
	}
	return A, enc, nil
}

// hashScalar returns SHA-512 of the concatenated parts, reduced mod ℓ.
func hashScalar(parts ...[]byte) *edwards25519.Scalar {
	h := sha512.New()
	for _, p := range parts {
		h.Write(p)
	}
	s, err := edwards25519.NewScalar().SetUniformBytes(h.Sum(nil))
	if err != nil {
		panic(err)
	}
	return s
}

// nonceDomain is hash_1's prefix from the XEdDSA specification: 0xFE then
// 31 bytes of 0xFF.
var nonceDomain = append([]byte{0xfe}, []byte{
	0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
	0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
}...)

// Sign returns a 64-byte XEd25519 signature of msg, reading the 64 bytes
// of random input Z from rand.
func (k *PrivateKey) Sign(rand io.Reader, msg []byte) ([]byte, error) {
	var z [64]byte
	if _, err := io.ReadFull(rand, z[:]); err != nil {
		return nil, err
	}
	var wide [64]byte
	copy(wide[:], k.k[:])
	a, err := edwards25519.NewScalar().SetUniformBytes(wide[:]) // k mod ℓ
	if err != nil {
		return nil, err
	}
	A := edwards25519.NewIdentityPoint().ScalarBaseMult(a).Bytes()

	r := hashScalar(nonceDomain, k.k[:], msg, z[:])
	R := edwards25519.NewIdentityPoint().ScalarBaseMult(r).Bytes()
	h := hashScalar(R, A, msg)
	S := edwards25519.NewScalar().MultiplyAdd(h, a, r).Bytes()

	sig := append(R, S...)
	sig[63] |= A[31] & 0x80 // S < 2^253, so bit 255 is free for A's sign
	return sig, nil
}

// Verify reports whether sig is a valid XEd25519 signature of msg by p.
func (p *PublicKey) Verify(msg, sig []byte) bool {
	if len(sig) != 64 {
		return false
	}
	A, Aenc, err := edwards(p.u[:], sig[63]>>7)
	if err != nil {
		return false
	}
	var s [64]byte
	copy(s[:32], sig[32:])
	s[31] &= 0x7f
	if s[31]&0xe0 != 0 {
		return false // S >= 2^253
	}
	S, err := edwards25519.NewScalar().SetUniformBytes(s[:]) // S may be unreduced
	if err != nil {
		return false
	}
	h := hashScalar(sig[:32], Aenc, msg)
	// R' = [S]B - [h]A, compared by canonical encoding.
	hA := edwards25519.NewIdentityPoint().ScalarMult(h, A)
	R := edwards25519.NewIdentityPoint().Subtract(edwards25519.NewIdentityPoint().ScalarBaseMult(S), hA)
	return subtle.ConstantTimeCompare(R.Bytes(), sig[:32]) == 1
}

// inRange reports whether u is a canonical field encoding: bit 255 clear
// and u < 2^255 - 19.
func inRange(u []byte) bool {
	ue, err := new(field.Element).SetBytes(u)
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(ue.Bytes(), u) == 1
}

// IsCanonical reports whether the 32-byte u passes the check libsignal
// applies to a PQXDH initiator's base key: u is in range, maps to an
// Edwards point (sign 0), and that point has no small-order component.
func IsCanonical(u []byte) bool {
	if !inRange(u) {
		return false
	}
	P, _, err := edwards(u, 0)
	if err != nil {
		return false
	}
	// Torsion-free iff [ℓ]P is the identity; ℓ itself isn't a canonical
	// scalar, so compute [ℓ-1]P + P.
	one, _ := edwards25519.NewScalar().SetUniformBytes(append([]byte{1}, make([]byte, 63)...))
	lMinus1 := edwards25519.NewScalar().Negate(one)
	Q := edwards25519.NewIdentityPoint().ScalarMult(lMinus1, P)
	return Q.Add(Q, P).Equal(edwards25519.NewIdentityPoint()) == 1
}
