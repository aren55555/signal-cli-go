package protocol

import (
	"bytes"
	"crypto/rand"
	"testing"
	"time"

	"github.com/aren55555/signal-cli-go/internal/crypto/curve25519"
)

// Generated prekeys (spec/service/keys.md §2.1, registration.md §1): the
// identity key signs the serialized public key, the stored form loads
// back, and the private half matches the public half.
func TestGeneratePreKeys(t *testing.T) {
	identity, _ := curve25519.GeneratePrivateKey(rand.Reader)
	now := time.UnixMilli(1700000000123)

	spk, err := GenerateSignedPreKey(rand.Reader, identity, 0xFFFFFE, now)
	if err != nil {
		t.Fatal(err)
	}
	if spk.ID != 0xFFFFFE || spk.Timestamp != 1700000000123 || len(spk.PublicKey()) != 33 ||
		!identity.PublicKey().Verify(spk.PublicKey(), spk.Signature) {
		t.Errorf("signed prekey %+v", spk)
	}
	back, err := LoadSignedPreKeyRecord(spk.Serialize())
	if err != nil || back.ID != spk.ID || !bytes.Equal(back.PublicKey(), spk.PublicKey()) || !bytes.Equal(back.Signature, spk.Signature) {
		t.Fatalf("signed prekey round trip: %v", err)
	}
	priv, _ := back.PrivateKey()
	if !bytes.Equal(priv.PublicKey().Bytes(), spk.PublicKey()) {
		t.Error("signed prekey private key does not match")
	}

	kpk, err := GenerateKyberPreKey(rand.Reader, identity, 0, now)
	if err != nil {
		t.Fatal(err)
	}
	if kpk.ID != 0 || len(kpk.PublicKey()) != 1569 || kpk.PublicKey()[0] != KEMTypeKyber1024 ||
		!identity.PublicKey().Verify(kpk.PublicKey(), kpk.Signature) {
		t.Errorf("kyber prekey: id %d, %d-byte key", kpk.ID, len(kpk.PublicKey()))
	}
	kback, err := LoadKyberPreKeyRecord(kpk.Serialize())
	if err != nil {
		t.Fatal(err)
	}
	pub, _ := ParseKEMPublicKey(kback.PublicKey())
	ct, ss, _ := pub.Encapsulate(rand.Reader)
	sk, _ := kback.SecretKey()
	if got, err := sk.Decapsulate(ct); err != nil || !bytes.Equal(got, ss) {
		t.Errorf("kyber round trip: %v", err)
	}

	pk, err := GeneratePreKey(rand.Reader, 42)
	if err != nil {
		t.Fatal(err)
	}
	pback, err := LoadPreKeyRecord(pk.Serialize())
	if err != nil || pback.ID != 42 || !bytes.Equal(pback.PublicKey(), pk.PublicKey()) {
		t.Fatalf("one-time prekey round trip: %v", err)
	}
	ppriv, _ := pback.PrivateKey()
	if !bytes.Equal(ppriv.PublicKey().Bytes(), pk.PublicKey()) {
		t.Error("one-time prekey private key does not match")
	}
}
