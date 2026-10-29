// SPDX-License-Identifier: GPL-3.0-or-later

package curve25519

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha512"
	"encoding/hex"
	"math/big"
	"math/rand/v2"
	"testing"

	"github.com/aren55555/signal-cli-go/internal/crypto/edwards25519"
	"github.com/aren55555/signal-cli-go/internal/crypto/edwards25519/field"
	"github.com/aren55555/signal-cli-go/internal/testvec"
)

// vector is the union of the entry kinds in testdata/vectors/xeddsa.json.
type vector struct {
	Kind          string
	Name          string
	PrivateKey    string `json:"private_key"`
	PublicKey     string `json:"public_key_with_prefix"`
	PublicKeyRaw  string `json:"public_key_without_prefix"`
	Message       string
	Signature     string
	ExpectedValid bool `json:"expected_valid"`
	InRange       bool `json:"expected_in_range"`

	AlicePrivate string `json:"alice_private_key"`
	AlicePublic  string `json:"alice_public_key_with_prefix"`
	BobPrivate   string `json:"bob_private_key"`
	BobPublic    string `json:"bob_public_key_with_prefix"`
	Shared       string `json:"shared_secret"`
}

func load(t *testing.T, kind string) []vector {
	var f struct{ Vectors []vector }
	testvec.Load(t, "xeddsa.json", &f)
	var out []vector
	for _, v := range f.Vectors {
		if v.Kind == kind {
			out = append(out, v)
		}
	}
	if len(out) == 0 {
		t.Fatalf("no %s vectors", kind)
	}
	return out
}

func h(t *testing.T, s string) []byte { return testvec.Hex(t, s) }

func TestVerifyVectors(t *testing.T) {
	vs := load(t, "xeddsa_verify")
	if len(vs) != 68 {
		t.Fatalf("got %d verify vectors, want 68", len(vs))
	}
	for _, v := range vs {
		pub, err := ParsePublicKey(h(t, v.PublicKey))
		if err != nil {
			t.Fatalf("%s: %v", v.Name, err)
		}
		if got := pub.Verify(h(t, v.Message), h(t, v.Signature)); got != v.ExpectedValid {
			t.Errorf("%s: Verify = %v, want %v", v.Name, got, v.ExpectedValid)
		}
		if !bytes.Equal(pub.Bytes(), h(t, v.PublicKey)) {
			t.Errorf("%s: public key re-serializes as %x", v.Name, pub.Bytes())
		}
	}
}

func TestAgreementVector(t *testing.T) {
	v := load(t, "x25519_agreement")[0]
	alice, err := NewPrivateKey(h(t, v.AlicePrivate))
	if err != nil {
		t.Fatal(err)
	}
	bob, err := NewPrivateKey(h(t, v.BobPrivate))
	if err != nil {
		t.Fatal(err)
	}
	if got := hex.EncodeToString(alice.PublicKey().Bytes()); got != v.AlicePublic {
		t.Errorf("alice public %s, want %s", got, v.AlicePublic)
	}
	if got := hex.EncodeToString(bob.PublicKey().Bytes()); got != v.BobPublic {
		t.Errorf("bob public %s, want %s", got, v.BobPublic)
	}
	for _, c := range []struct {
		priv *PrivateKey
		peer string
	}{{alice, v.BobPublic}, {bob, v.AlicePublic}} {
		peer, err := ParsePublicKey(h(t, c.peer))
		if err != nil {
			t.Fatal(err)
		}
		shared, err := c.priv.Agree(peer)
		if err != nil {
			t.Fatal(err)
		}
		if got := hex.EncodeToString(shared); got != v.Shared {
			t.Errorf("shared %s, want %s", got, v.Shared)
		}
	}
}

func TestPublicKeyDerivationVectors(t *testing.T) {
	for _, v := range load(t, "x25519_public_key_derivation") {
		priv, err := NewPrivateKey(h(t, v.PrivateKey))
		if err != nil {
			t.Fatal(err)
		}
		if got := hex.EncodeToString(priv.PublicKey().Bytes()); got != v.PublicKey {
			t.Errorf("%s: %s, want %s", v.Name, got, v.PublicKey)
		}
	}
}

func TestRangeCheckVectors(t *testing.T) {
	vs := load(t, "montgomery_u_range_check")
	if len(vs) != 23 {
		t.Fatalf("got %d range vectors, want 23", len(vs))
	}
	for _, v := range vs {
		if got := inRange(h(t, v.PublicKeyRaw)); got != v.InRange {
			t.Errorf("%s: inRange = %v, want %v", v.Name, got, v.InRange)
		}
	}
}

// TestIsCanonical covers the torsion half of the PQXDH base-key check,
// which has no fixed upstream vectors.
func TestIsCanonical(t *testing.T) {
	nine := make([]byte, 32)
	nine[0] = 9
	if !IsCanonical(nine) {
		t.Error("base point u=9 rejected")
	}
	// u = 0 is (0, -1) on the Edwards curve, of order 2.
	if IsCanonical(make([]byte, 32)) {
		t.Error("u=0 (order 2) accepted")
	}
	// B + (0,-1) has a torsion component.
	minus1 := new(field.Element).Negate(new(field.Element).One()).Bytes()
	order2, err := edwards25519.NewIdentityPoint().SetBytes(minus1)
	if err != nil {
		t.Fatal(err)
	}
	mixed := edwards25519.NewIdentityPoint().Add(edwards25519.NewGeneratorPoint(), order2)
	if u := mixed.BytesMontgomery(); IsCanonical(u) {
		t.Errorf("B + T (u=%x) accepted", u)
	}
	// u = p - 1 has no Edwards image.
	if IsCanonical(minus1) {
		t.Error("u = p-1 accepted")
	}
	// A random honest key is canonical.
	priv, _ := NewPrivateKey(bytes.Repeat([]byte{7}, 32))
	if !IsCanonical(priv.PublicKey().Bytes()[1:]) {
		t.Error("honest public key rejected")
	}
}

func TestParsePublicKey(t *testing.T) {
	u := bytes.Repeat([]byte{0x11}, 32)
	good := append([]byte{0x05}, u...)
	if _, err := ParsePublicKey(nil); err == nil {
		t.Error("empty accepted")
	}
	if _, err := ParsePublicKey(append([]byte{0x06}, u...)); err == nil {
		t.Error("type 0x06 accepted")
	}
	if _, err := ParsePublicKey(good[:32]); err == nil {
		t.Error("short key accepted")
	}
	// Trailing bytes beyond 33 are ignored, as in libsignal.
	k, err := ParsePublicKey(append(bytes.Clone(good), 0xaa, 0xbb))
	if err != nil {
		t.Fatalf("trailing bytes rejected: %v", err)
	}
	if !bytes.Equal(k.Bytes(), good) {
		t.Errorf("re-serialized %x", k.Bytes())
	}
	if _, err := NewPrivateKey(make([]byte, 31)); err == nil {
		t.Error("31-byte private key accepted")
	}
}

func TestPrivateKeyClamped(t *testing.T) {
	priv, err := NewPrivateKey(bytes.Repeat([]byte{0xff}, 32))
	if err != nil {
		t.Fatal(err)
	}
	want := bytes.Repeat([]byte{0xff}, 32)
	want[0] = 0xf8
	want[31] = 0x7f
	if !bytes.Equal(priv.Bytes(), want) {
		t.Errorf("stored %x, want clamped %x", priv.Bytes(), want)
	}
}

func TestAgreeRejectsZero(t *testing.T) {
	priv, _ := NewPrivateKey(bytes.Repeat([]byte{3}, 32))
	zero, err := ParsePublicKey(append([]byte{0x05}, make([]byte, 32)...))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := priv.Agree(zero); err == nil {
		t.Error("all-zero shared secret accepted")
	}
}

// fixedReader yields the same bytes forever.
type fixedReader byte

func (r fixedReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = byte(r)
	}
	return len(p), nil
}

// edwardsKey converts a libsignal XEd25519 public key and signature to an
// Ed25519 public key and signature, following spec/crypto/xeddsa.md:
// y = (u-1)/(u+1), with bit 255 taken from bit 7 of signature byte 63,
// which is then cleared.
func edwardsKey(t *testing.T, u, sig []byte) (ed25519.PublicKey, []byte) {
	ue, _ := new(field.Element).SetBytes(u)
	one := new(field.Element).One()
	y := new(field.Element).Multiply(new(field.Element).Subtract(ue, one),
		new(field.Element).Invert(new(field.Element).Add(ue, one)))
	pub := y.Bytes()
	pub[31] |= sig[63] & 0x80
	s := bytes.Clone(sig)
	s[63] &= 0x7f
	return pub, s
}

func TestSignRoundTripAndEd25519(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	signBits := map[byte]int{}
	for i := range 64 {
		seed := make([]byte, 32)
		for j := range seed {
			seed[j] = byte(r.Uint32())
		}
		msg := make([]byte, r.IntN(200))
		for j := range msg {
			msg[j] = byte(r.Uint32())
		}
		priv, err := NewPrivateKey(seed)
		if err != nil {
			t.Fatal(err)
		}
		pub := priv.PublicKey()
		sig, err := priv.Sign(fixedReader(i), msg)
		if err != nil {
			t.Fatal(err)
		}
		if len(sig) != 64 {
			t.Fatalf("signature length %d", len(sig))
		}
		signBits[sig[63]>>7]++
		if !pub.Verify(msg, sig) {
			t.Fatalf("own signature rejected (key %x)", seed)
		}
		edPub, edSig := edwardsKey(t, pub.Bytes()[1:], sig)
		if !ed25519.Verify(edPub, msg, edSig) {
			t.Fatalf("crypto/ed25519 rejects the converted signature (key %x)", seed)
		}
		if pub.Verify(append(bytes.Clone(msg), 0), sig) {
			t.Fatal("signature verified for a different message")
		}
		// Bit 255 of u is ignored.
		hiU := pub.Bytes()
		hiU[32] |= 0x80
		hiPub, err := ParsePublicKey(hiU)
		if err != nil {
			t.Fatal(err)
		}
		if !hiPub.Verify(msg, sig) {
			t.Fatal("verify rejected u with bit 255 set")
		}
	}
	// Both Edwards sign bits must occur, or the sign-bit path is untested.
	if signBits[0] == 0 || signBits[1] == 0 {
		t.Errorf("sign bits seen: %v", signBits)
	}
}

// TestSignNonceHash pins the libsignal nonce input: SHA-512 over
// 0xFE || 0xFF*31 || clamped key bytes || message || Z. A signer following
// signal.org literally (reduced, possibly negated scalar) gives a
// different R.
func TestSignNonceHash(t *testing.T) {
	priv, _ := NewPrivateKey(bytes.Repeat([]byte{0x42}, 32))
	msg := []byte("nonce")
	sig, err := priv.Sign(fixedReader(9), msg)
	if err != nil {
		t.Fatal(err)
	}
	r := nonce(t, priv.Bytes(), msg, bytes.Repeat([]byte{9}, 64))
	if R := edwards25519.NewIdentityPoint().ScalarBaseMult(r).Bytes(); !bytes.Equal(R, sig[:32]) {
		t.Errorf("R = %x, want %x", sig[:32], R)
	}
}

// TestVerifySRange: S in [ℓ, 2^253) is accepted (reduced), S >= 2^253 is
// rejected, per spec/crypto/xeddsa.md.
func TestVerifySRange(t *testing.T) {
	priv, _ := NewPrivateKey(bytes.Repeat([]byte{5}, 32))
	pub := priv.PublicKey()
	msg := []byte("range")
	var sig []byte
	for i := byte(0); ; i++ {
		s, err := priv.Sign(fixedReader(i), msg)
		if err != nil {
			t.Fatal(err)
		}
		// Need S + ℓ < 2^253 so the unreduced form fits.
		if leInt(s[32:], true).Cmp(new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 253), order())) < 0 {
			sig = s
			break
		}
	}
	signBit := sig[63] & 0x80
	unreduced := new(big.Int).Add(leInt(sig[32:], true), order())
	alt := append(bytes.Clone(sig[:32]), le32(unreduced)...)
	alt[63] |= signBit
	if !pub.Verify(msg, alt) {
		t.Error("S + ℓ (< 2^253) rejected")
	}
	tooBig := bytes.Clone(sig)
	tooBig[63] |= 0x20 // bit 253
	if pub.Verify(msg, tooBig) {
		t.Error("S >= 2^253 accepted")
	}
	if pub.Verify(msg, sig[:63]) {
		t.Error("63-byte signature accepted")
	}
}

func order() *big.Int {
	l, _ := new(big.Int).SetString("7237005577332262213973186563042994240857116359379907606001950938285454250989", 10)
	return l
}

// leInt reads little-endian bytes, optionally ignoring bit 7 of the last.
func leInt(b []byte, clearTop bool) *big.Int {
	r := bytes.Clone(b)
	if clearTop {
		r[len(r)-1] &= 0x7f
	}
	for i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 {
		r[i], r[j] = r[j], r[i]
	}
	return new(big.Int).SetBytes(r)
}

func le32(x *big.Int) []byte {
	b := x.FillBytes(make([]byte, 32))
	for i, j := 0, 31; i < j; i, j = i+1, j-1 {
		b[i], b[j] = b[j], b[i]
	}
	return b
}

// nonce computes the expected r from its definition in spec/crypto/xeddsa.md.
func nonce(t *testing.T, clampedKey, msg, z []byte) *edwards25519.Scalar {
	hh := sha512.New()
	hh.Write([]byte{0xfe})
	hh.Write(bytes.Repeat([]byte{0xff}, 31))
	hh.Write(clampedKey)
	hh.Write(msg)
	hh.Write(z)
	r, err := edwards25519.NewScalar().SetUniformBytes(hh.Sum(nil))
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// FuzzVerify feeds arbitrary keys and signatures to the verifier: it must
// not panic, and must never accept a corrupted honest signature.
func FuzzVerify(f *testing.F) {
	priv, _ := NewPrivateKey(bytes.Repeat([]byte{1}, 32))
	msg := []byte("fuzz")
	sig, _ := priv.Sign(fixedReader(0), msg)
	f.Add(priv.PublicKey().U(), sig)
	f.Fuzz(func(t *testing.T, u, s []byte) {
		if len(u) != 32 {
			return
		}
		pub, _ := NewPublicKey(u)
		ok := pub.Verify(msg, s)
		if ok && bytes.Equal(u, priv.PublicKey().U()) && len(s) == 64 && !bytes.Equal(s, sig) {
			// A different valid signature can only differ in an unreduced S.
			if !bytes.Equal(s[:32], sig[:32]) {
				t.Fatalf("accepted a forged R: %x", s)
			}
		}
		IsCanonical(u)
	})
}
