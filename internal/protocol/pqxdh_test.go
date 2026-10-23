package protocol

import (
	"bytes"
	"crypto/hkdf"
	"crypto/sha256"
	"math/rand/v2"
	"testing"

	"github.com/aren55555/signal-cli-go/internal/crypto/curve25519"
	"github.com/aren55555/signal-cli-go/internal/crypto/kyber"
)

func testRand(seed uint64) *rand.ChaCha8 {
	var s [32]byte
	s[0] = byte(seed)
	return rand.NewChaCha8(s)
}

type bob struct {
	identity, signedPreKey, oneTime *curve25519.PrivateKey
	kem                             *KEMSecretKey
	bundle                          *PreKeyBundle
}

func newBob(t *testing.T, r *rand.ChaCha8, withOneTime bool) *bob {
	t.Helper()
	b := new(bob)
	var err error
	if b.identity, err = curve25519.GeneratePrivateKey(r); err != nil {
		t.Fatal(err)
	}
	b.signedPreKey, _ = curve25519.GeneratePrivateKey(r)
	dk, err := kyber.GenerateKey1024(r)
	if err != nil {
		t.Fatal(err)
	}
	b.kem = NewKEMSecretKey(dk)
	spkSig, _ := b.identity.Sign(r, b.signedPreKey.PublicKey().Bytes())
	kemSig, _ := b.identity.Sign(r, b.kem.PublicKey().Bytes())
	b.bundle = &PreKeyBundle{
		IdentityKey:           b.identity.PublicKey(),
		SignedPreKeyID:        7,
		SignedPreKey:          b.signedPreKey.PublicKey(),
		SignedPreKeySignature: spkSig,
		KyberPreKeyID:         9,
		KyberPreKey:           b.kem.PublicKey(),
		KyberPreKeySignature:  kemSig,
	}
	if withOneTime {
		b.oneTime, _ = curve25519.GeneratePrivateKey(r)
		id := uint32(3)
		b.bundle.PreKeyID = &id
		b.bundle.PreKey = b.oneTime.PublicKey()
	}
	return b
}

func (b *bob) respond(t *testing.T, alice *curve25519.PublicKey, res *PQXDHInitiation) (*PQXDHSecrets, error) {
	t.Helper()
	keys := PQXDHResponderKeys{
		Identity:     b.identity,
		SignedPreKey: b.signedPreKey,
		PreKey:       b.oneTime,
		KyberPreKey:  b.kem,
	}
	return RespondPQXDH(keys, alice, res.BaseKey, res.KyberCiphertext)
}

func TestPQXDHAgreement(t *testing.T) {
	for _, withOneTime := range []bool{false, true} {
		r := testRand(1)
		b := newBob(t, r, withOneTime)
		alice, _ := curve25519.GeneratePrivateKey(r)
		res, err := InitiatePQXDH(r, alice, b.bundle)
		if err != nil {
			t.Fatal(err)
		}
		got, err := b.respond(t, alice.PublicKey(), res)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got.RootKey, res.Secrets.RootKey) || !bytes.Equal(got.ChainKey, res.Secrets.ChainKey) || !bytes.Equal(got.PQRKey, res.Secrets.PQRKey) {
			t.Errorf("one-time prekey %v: secrets differ", withOneTime)
		}
		if res.UsedPreKeyID != b.bundle.PreKeyID || res.UsedSignedPreKeyID != 7 || res.UsedKyberPreKeyID != 9 {
			t.Errorf("used IDs: %v %d %d", res.UsedPreKeyID, res.UsedSignedPreKeyID, res.UsedKyberPreKeyID)
		}
	}
}

// TestPQXDHKeySchedule recomputes the key schedule from spec/protocol/pqxdh.md
// §2-3 (PQXDH revision 3 with libsignal's parameters): IKM = 32 bytes of
// 0xFF || DH1 || DH2 || DH3 || DH4 || SS, HKDF-SHA256 with no salt, info
// "WhisperText_X25519_SHA-256_CRYSTALS-KYBER-1024", 96 bytes split into root
// key, chain key and SPQR seed.
func TestPQXDHKeySchedule(t *testing.T) {
	r := testRand(2)
	b := newBob(t, r, true)
	alice, _ := curve25519.GeneratePrivateKey(r)

	// Capture Alice's ephemeral key by replaying the random stream.
	r2 := testRand(2)
	_ = newBob(t, r2, true)
	_, _ = curve25519.GeneratePrivateKey(r2)
	res, err := InitiatePQXDH(r, alice, b.bundle)
	if err != nil {
		t.Fatal(err)
	}
	ek, _ := curve25519.GeneratePrivateKey(r2) // same draw as InitiatePQXDH's first
	if !bytes.Equal(ek.PublicKey().Bytes(), res.BaseKey.Bytes()) {
		t.Fatal("InitiatePQXDH must draw the ephemeral key first (test assumption)")
	}

	dh := func(priv *curve25519.PrivateKey, pub *curve25519.PublicKey) []byte {
		s, err := priv.Agree(pub)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	ss, err := b.kem.Decapsulate(res.KyberCiphertext)
	if err != nil {
		t.Fatal(err)
	}
	km := bytes.Repeat([]byte{0xff}, 32)
	km = append(km, dh(alice, b.bundle.SignedPreKey)...)
	km = append(km, dh(ek, b.bundle.IdentityKey)...)
	km = append(km, dh(ek, b.bundle.SignedPreKey)...)
	km = append(km, dh(ek, b.bundle.PreKey)...)
	km = append(km, ss...)
	want, err := hkdf.Key(sha256.New, km, nil, "WhisperText_X25519_SHA-256_CRYSTALS-KYBER-1024", 96)
	if err != nil {
		t.Fatal(err)
	}
	got := append(append(bytes.Clone(res.Secrets.RootKey), res.Secrets.ChainKey...), res.Secrets.PQRKey...)
	if !bytes.Equal(got, want) {
		t.Errorf("key schedule = %x, want %x", got, want)
	}
}

func TestPQXDHRejectsBadSignatures(t *testing.T) {
	r := testRand(3)
	b := newBob(t, r, false)
	alice, _ := curve25519.GeneratePrivateKey(r)

	bad := *b.bundle
	bad.SignedPreKeySignature = bytes.Clone(bad.SignedPreKeySignature)
	bad.SignedPreKeySignature[0] ^= 1
	if _, err := InitiatePQXDH(r, alice, &bad); err == nil {
		t.Error("bad signed-prekey signature accepted")
	}
	bad = *b.bundle
	bad.KyberPreKeySignature = bytes.Clone(bad.KyberPreKeySignature)
	bad.KyberPreKeySignature[0] ^= 1
	if _, err := InitiatePQXDH(r, alice, &bad); err == nil {
		t.Error("bad Kyber-prekey signature accepted")
	}
	bad = *b.bundle
	bad.KyberPreKey = nil
	if _, err := InitiatePQXDH(r, alice, &bad); err == nil {
		t.Error("bundle without a Kyber prekey accepted")
	}
}

func TestPQXDHResponderChecks(t *testing.T) {
	r := testRand(4)
	b := newBob(t, r, false)
	alice, _ := curve25519.GeneratePrivateKey(r)
	res, err := InitiatePQXDH(r, alice, b.bundle)
	if err != nil {
		t.Fatal(err)
	}

	// A non-canonical base key (u = 0, small order) is rejected.
	zero, _ := curve25519.NewPublicKey(make([]byte, 32))
	tampered := *res
	tampered.BaseKey = zero
	if _, err := b.respond(t, alice.PublicKey(), &tampered); err == nil {
		t.Error("non-canonical base key accepted")
	}

	// A tampered Kyber ciphertext gives a different secret (implicit
	// rejection), not an error.
	tampered = *res
	tampered.KyberCiphertext = bytes.Clone(res.KyberCiphertext)
	tampered.KyberCiphertext[100] ^= 1
	got, err := b.respond(t, alice.PublicKey(), &tampered)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(got.RootKey, res.Secrets.RootKey) {
		t.Error("tampered ciphertext gave the same secret")
	}
}
