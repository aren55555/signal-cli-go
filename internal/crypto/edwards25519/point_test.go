// SPDX-License-Identifier: GPL-3.0-or-later

package edwards25519

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha512"
	"encoding/hex"
	"math/big"
	"math/rand/v2"
	"testing"

	"github.com/aren55555/signal-cli-go/internal/testvec"
)

// The Ed25519 functions below follow RFC 8032 section 5.1 using only this
// package and crypto/sha512, so the RFC vectors test the group operations.

func ed25519Expand(t *testing.T, seed []byte) (*Scalar, []byte) {
	h := sha512.Sum512(seed)
	s, err := NewScalar().SetBytesWithClamping(h[:32])
	if err != nil {
		t.Fatal(err)
	}
	return s, h[32:]
}

func ed25519Public(t *testing.T, seed []byte) []byte {
	s, _ := ed25519Expand(t, seed)
	return NewIdentityPoint().ScalarBaseMult(s).Bytes()
}

func hashToScalar(t *testing.T, parts ...[]byte) *Scalar {
	h := sha512.New()
	for _, p := range parts {
		h.Write(p)
	}
	k, err := NewScalar().SetUniformBytes(h.Sum(nil))
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func ed25519Sign(t *testing.T, seed, msg []byte) []byte {
	s, prefix := ed25519Expand(t, seed)
	pub := NewIdentityPoint().ScalarBaseMult(s).Bytes()
	r := hashToScalar(t, prefix, msg)
	R := NewIdentityPoint().ScalarBaseMult(r).Bytes()
	k := hashToScalar(t, R, pub, msg)
	S := NewScalar().MultiplyAdd(k, s, r)
	return append(R, S.Bytes()...)
}

// ed25519Verify checks [S]B == R + [k]A by comparing encodings.
func ed25519Verify(t *testing.T, pub, msg, sig []byte) bool {
	A, err := NewIdentityPoint().SetBytes(pub)
	if err != nil {
		return false
	}
	S, err := NewScalar().SetCanonicalBytes(sig[32:])
	if err != nil {
		return false
	}
	k := hashToScalar(t, sig[:32], pub, msg)
	// R' = [S]B - [k]A
	kA := NewIdentityPoint().ScalarMult(k, A)
	R := NewIdentityPoint().Subtract(NewIdentityPoint().ScalarBaseMult(S), kA)
	return bytes.Equal(R.Bytes(), sig[:32])
}

func TestEd25519RFC8032(t *testing.T) {
	var f struct {
		Vectors []struct {
			Name      string
			SecretKey string `json:"secret_key"`
			PublicKey string `json:"public_key"`
			Message   string
			Signature string
		}
	}
	testvec.Load(t, "rfc8032_ed25519.json", &f)
	if len(f.Vectors) != 5 {
		t.Fatalf("got %d vectors, want 5", len(f.Vectors))
	}
	for _, v := range f.Vectors {
		t.Run(v.Name, func(t *testing.T) {
			seed, msg := testvec.Hex(t, v.SecretKey), testvec.Hex(t, v.Message)
			pub, sig := testvec.Hex(t, v.PublicKey), testvec.Hex(t, v.Signature)

			// The vectors themselves, checked against crypto/ed25519.
			if !ed25519.Verify(pub, msg, sig) {
				t.Fatal("crypto/ed25519 rejects the RFC vector")
			}
			if got := ed25519Public(t, seed); !bytes.Equal(got, pub) {
				t.Errorf("public key %x, want %x", got, pub)
			}
			if got := ed25519Sign(t, seed, msg); !bytes.Equal(got, sig) {
				t.Errorf("signature %x, want %x", got, sig)
			}
			if !ed25519Verify(t, pub, msg, sig) {
				t.Error("verify rejected the RFC signature")
			}
			bad := bytes.Clone(sig)
			bad[5] ^= 1
			if ed25519Verify(t, pub, msg, bad) {
				t.Error("verify accepted a corrupted signature")
			}
		})
	}
}

func TestEd25519MatchesCrypto(t *testing.T) {
	r := rand.New(rand.NewPCG(9, 10))
	for range 50 {
		seed := make([]byte, 32)
		msg := make([]byte, r.IntN(100))
		for i := range seed {
			seed[i] = byte(r.Uint32())
		}
		for i := range msg {
			msg[i] = byte(r.Uint32())
		}
		want := ed25519.Sign(ed25519.NewKeyFromSeed(seed), msg)
		if got := ed25519Sign(t, seed, msg); !bytes.Equal(got, want) {
			t.Fatalf("seed %x: signature %x, crypto/ed25519 says %x", seed, got, want)
		}
	}
}

// TestMontgomeryRFC7748 maps [k]B to its Montgomery u-coordinate; RFC 7748
// section 6.1's public keys are X25519(k, 9), and 9 is the image of B.
func TestMontgomeryRFC7748(t *testing.T) {
	var f struct {
		ECDH map[string]string `json:"ecdh"`
	}
	testvec.Load(t, "rfc7748_x25519.json", &f)
	for _, who := range []string{"alice", "bob"} {
		k, err := NewScalar().SetBytesWithClamping(testvec.Hex(t, f.ECDH[who+"_private"]))
		if err != nil {
			t.Fatal(err)
		}
		got := hex.EncodeToString(NewIdentityPoint().ScalarBaseMult(k).BytesMontgomery())
		if got != f.ECDH[who+"_public"] {
			t.Errorf("%s: u = %s, want %s", who, got, f.ECDH[who+"_public"])
		}
	}
	nine := make([]byte, 32)
	nine[0] = 9
	if got := NewGeneratorPoint().BytesMontgomery(); !bytes.Equal(got, nine) {
		t.Errorf("generator u = %x, want 9", got)
	}
}

func TestGroupLaws(t *testing.T) {
	r := rand.New(rand.NewPCG(11, 12))
	B := NewGeneratorPoint()
	I := NewIdentityPoint()
	for range 30 {
		a, b := mustScalar(t, randScalarBytes(r)), mustScalar(t, randScalarBytes(r))
		aB := NewIdentityPoint().ScalarBaseMult(a)
		bB := NewIdentityPoint().ScalarBaseMult(b)

		if NewIdentityPoint().ScalarMult(a, B).Equal(aB) != 1 {
			t.Fatal("ScalarMult(a, B) != ScalarBaseMult(a)")
		}
		sum := NewIdentityPoint().ScalarBaseMult(NewScalar().Add(a, b))
		if NewIdentityPoint().Add(aB, bB).Equal(sum) != 1 {
			t.Fatal("aB + bB != (a+b)B")
		}
		if NewIdentityPoint().Subtract(sum, bB).Equal(aB) != 1 {
			t.Fatal("(a+b)B - bB != aB")
		}
		if NewIdentityPoint().Add(aB, NewIdentityPoint().Negate(aB)).Equal(I) != 1 {
			t.Fatal("aB + (-aB) != identity")
		}
		two := mustScalar(t, bigToLE(big.NewInt(2)))
		if NewIdentityPoint().Add(aB, aB).Equal(NewIdentityPoint().ScalarMult(two, aB)) != 1 {
			t.Fatal("aB + aB != [2]aB")
		}
		ab := NewIdentityPoint().ScalarMult(b, aB)
		if ab.Equal(NewIdentityPoint().ScalarBaseMult(NewScalar().Multiply(a, b))) != 1 {
			t.Fatal("[b]([a]B) != [ab]B")
		}
		// Encoding round trip.
		dec, err := NewIdentityPoint().SetBytes(aB.Bytes())
		if err != nil || dec.Equal(aB) != 1 {
			t.Fatalf("round trip of %x failed: %v", aB.Bytes(), err)
		}
	}
	// [ℓ-1]B + B is the identity.
	lm1 := mustScalar(t, bigToLE(new(big.Int).Sub(order, big.NewInt(1))))
	if NewIdentityPoint().Add(NewIdentityPoint().ScalarBaseMult(lm1), B).Equal(I) != 1 {
		t.Error("[ℓ]B != identity")
	}
	id := make([]byte, 32)
	id[0] = 1
	if !bytes.Equal(I.Bytes(), id) {
		t.Errorf("identity encodes as %x", I.Bytes())
	}
}

// TestDecodingStrict covers the RFC 8032 section 5.1.3 rejections.
func TestDecodingStrict(t *testing.T) {
	p := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 255), big.NewInt(19))
	cases := map[string][]byte{
		// y = p+1 encodes y = 1 non-canonically.
		"y >= p": bigToLE(new(big.Int).Add(p, big.NewInt(1))),
		// y = 1 gives x = 0, which must not carry the sign bit.
		"x = 0 with sign bit": func() []byte { b := make([]byte, 32); b[0] = 1; b[31] = 0x80; return b }(),
		"not on curve":        bigToLE(offCurveY(p)),
		"short":               make([]byte, 31),
	}
	for name, enc := range cases {
		if _, err := NewIdentityPoint().SetBytes(enc); err == nil {
			t.Errorf("%s: %x accepted", name, enc)
		}
	}
}

// offCurveY returns the smallest y > 1 for which (y^2-1)/(d*y^2+1) is not a
// square mod p, so no point has that y.
func offCurveY(p *big.Int) *big.Int {
	d := new(big.Int).Mul(big.NewInt(-121665), new(big.Int).ModInverse(big.NewInt(121666), p))
	for y := big.NewInt(2); ; y.Add(y, big.NewInt(1)) {
		y2 := new(big.Int).Mul(y, y)
		u := new(big.Int).Sub(y2, big.NewInt(1))
		v := new(big.Int).Add(new(big.Int).Mul(d, y2), big.NewInt(1))
		x2 := new(big.Int).Mod(new(big.Int).Mul(u, new(big.Int).ModInverse(v.Mod(v, p), p)), p)
		if big.Jacobi(x2, p) == -1 {
			return y
		}
	}
}

// FuzzSetBytes checks that decoding is strict: an accepted encoding
// re-encodes to the same bytes.
func FuzzSetBytes(f *testing.F) {
	f.Add(NewGeneratorPoint().Bytes())
	f.Add(NewIdentityPoint().Bytes())
	f.Add(bytes.Repeat([]byte{0xff}, 32))
	f.Fuzz(func(t *testing.T, b []byte) {
		p, err := NewIdentityPoint().SetBytes(b)
		if err != nil {
			return
		}
		if got := p.Bytes(); !bytes.Equal(got, b) {
			t.Fatalf("SetBytes(%x) re-encodes as %x", b, got)
		}
	})
}
