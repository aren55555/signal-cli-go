// SPDX-License-Identifier: GPL-3.0-or-later

package protocol

import (
	"bytes"
	"testing"

	"github.com/aren55555/signal-cli-go/internal/crypto/kyber"
)

// The framing rules come from spec/crypto/kem.md: type byte 0x08 on
// public keys, secret keys and ciphertexts, exact lengths, no trailing
// bytes.
func TestKEMFraming(t *testing.T) {
	dk, err := kyber.NewDecapsulationKey1024FromSeed(bytes.Repeat([]byte{1}, 64))
	if err != nil {
		t.Fatal(err)
	}
	sk := NewKEMSecretKey(dk)
	pk := sk.PublicKey()
	pkb := pk.Bytes()
	if len(pkb) != 1569 || pkb[0] != 0x08 {
		t.Fatalf("public key: %d bytes, type %#x", len(pkb), pkb[0])
	}
	if skb := sk.Bytes(); len(skb) != 3169 || skb[0] != 0x08 {
		t.Fatalf("secret key: %d bytes, type %#x", len(skb), skb[0])
	}
	pk2, err := ParseKEMPublicKey(pkb)
	if err != nil || !bytes.Equal(pk2.Bytes(), pkb) {
		t.Fatalf("public key round trip: %v", err)
	}
	sk2, err := ParseKEMSecretKey(sk.Bytes())
	if err != nil || !bytes.Equal(sk2.Bytes(), sk.Bytes()) {
		t.Fatalf("secret key round trip: %v", err)
	}

	ct, ss, err := pk.Encapsulate(bytes.NewReader(bytes.Repeat([]byte{2}, 32)))
	if err != nil {
		t.Fatal(err)
	}
	if len(ct) != 1569 || ct[0] != 0x08 {
		t.Fatalf("ciphertext: %d bytes, type %#x", len(ct), ct[0])
	}
	got, err := sk.Decapsulate(ct)
	if err != nil || !bytes.Equal(got, ss) {
		t.Fatalf("decapsulate: %v", err)
	}

	for name, b := range map[string][]byte{
		"empty":         nil,
		"ML-KEM type":   append([]byte{0x0a}, pkb[1:]...),
		"Kyber768 type": append([]byte{0x07}, pkb[1:]...),
		"short":         pkb[:1568],
		"trailing byte": append(bytes.Clone(pkb), 0),
	} {
		if _, err := ParseKEMPublicKey(b); err == nil {
			t.Errorf("public key %s accepted", name)
		}
	}
	if _, err := sk.Decapsulate(append([]byte{0x0a}, ct[1:]...)); err == nil {
		t.Error("ciphertext with type 0x0a accepted")
	}
	if _, err := sk.Decapsulate(ct[:1568]); err == nil {
		t.Error("short ciphertext accepted")
	}
	if _, err := ParseKEMSecretKey(sk.Bytes()[:3168]); err == nil {
		t.Error("short secret key accepted")
	}
}
