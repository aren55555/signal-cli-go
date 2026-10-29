// SPDX-License-Identifier: GPL-3.0-or-later

package protocol

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/aren55555/signal-cli-go/internal/crypto/curve25519"
	"github.com/aren55555/signal-cli-go/internal/testvec"
)

type kdfVector struct {
	Kind              string
	Name              string
	ChainKey          string            `json:"chain_key"`
	ChainIndex        uint32            `json:"chain_index"`
	CipherKey         string            `json:"expected_cipher_key"`
	MacKey            string            `json:"expected_mac_key"`
	NextChainKey      string            `json:"expected_next_chain_key"`
	NextChainIndex    uint32            `json:"expected_next_chain_index"`
	Derived           map[string]string `json:"derived_by_spec_agent"`
	Key, IV           string
	Plaintext         string
	Ciphertext        string
	ExpectedCT        string `json:"expected_ciphertext"`
	ExpectedPlaintext string `json:"expected_plaintext"`
}

func loadKDF(t *testing.T) []kdfVector {
	var f struct{ Vectors []kdfVector }
	testvec.Load(t, "ratchet_kdf.json", &f)
	if len(f.Vectors) != 5 {
		t.Fatalf("got %d vectors, want 5", len(f.Vectors))
	}
	return f.Vectors
}

func TestChainKeyVector(t *testing.T) {
	for _, v := range loadKDF(t) {
		if v.Kind != "chain_key_step" {
			continue
		}
		seed, next := chainStep(testvec.Hex(t, v.ChainKey))
		if hex.EncodeToString(next) != v.NextChainKey {
			t.Errorf("next chain key %x, want %s", next, v.NextChainKey)
		}
		if hex.EncodeToString(seed) != v.Derived["message_key_seed_hmac_0x01"] {
			t.Errorf("seed %x", seed)
		}
		mk := deriveMessageKeys(seed, nil)
		if hex.EncodeToString(mk.cipherKey) != v.CipherKey || hex.EncodeToString(mk.macKey) != v.MacKey {
			t.Errorf("message keys %x / %x", mk.cipherKey, mk.macKey)
		}
		if hex.EncodeToString(mk.iv) != v.Derived["iv"] {
			t.Errorf("iv %x", mk.iv)
		}
	}
}

func TestAESCBCVectors(t *testing.T) {
	for _, v := range loadKDF(t) {
		key, iv := testvec.Hex(t, v.Key), testvec.Hex(t, v.IV)
		switch v.Kind {
		case "aes_256_cbc_pkcs7":
			ct, err := cbcEncrypt(key, iv, testvec.Hex(t, v.Plaintext))
			if err != nil || hex.EncodeToString(ct) != v.ExpectedCT {
				t.Errorf("%s: %x, %v", v.Name, ct, err)
			}
			pt, err := cbcDecrypt(key, iv, ct)
			if err != nil || hex.EncodeToString(pt) != v.Plaintext {
				t.Errorf("%s: decrypt %x, %v", v.Name, pt, err)
			}
		case "aes_256_cbc_pkcs7_decrypt_error":
			if _, err := cbcDecrypt(key, iv, testvec.Hex(t, v.Ciphertext)); err == nil {
				t.Errorf("%s: no error", v.Name)
			}
		case "aes_256_cbc_pkcs7_decrypt":
			pt, err := cbcDecrypt(key, iv, testvec.Hex(t, v.Ciphertext))
			if err != nil || hex.EncodeToString(pt) != v.ExpectedPlaintext {
				t.Errorf("%s: %x, %v", v.Name, pt, err)
			}
		}
	}
	if _, err := cbcDecrypt(make([]byte, 32), make([]byte, 16), nil); err == nil {
		t.Error("empty ciphertext accepted")
	}
}

// TestSPQRSaltChangesKeys: the SPQR key is the HKDF salt
// (double-ratchet.md §3); no salt equals 32 zero bytes (RFC 5869).
func TestSPQRSaltChangesKeys(t *testing.T) {
	seed := bytes.Repeat([]byte{9}, 32)
	none := deriveMessageKeys(seed, nil)
	zero := deriveMessageKeys(seed, make([]byte, 32))
	salted := deriveMessageKeys(seed, bytes.Repeat([]byte{1}, 32))
	if !bytes.Equal(none.cipherKey, zero.cipherKey) {
		t.Error("absent salt differs from 32 zero bytes")
	}
	if bytes.Equal(none.cipherKey, salted.cipherKey) {
		t.Error("SPQR salt had no effect")
	}
}

// TestMessageMAC checks the MAC against its definition in
// double-ratchet.md §5 (upstream has no fixed MAC vector).
func TestMessageMAC(t *testing.T) {
	r := testRand(20)
	a, _ := curve25519.GeneratePrivateKey(r)
	b, _ := curve25519.GeneratePrivateKey(r)
	macKey := bytes.Repeat([]byte{7}, 32)
	msg := []byte{0x44, 0x0a, 0x01, 0x02}
	h := hmac.New(sha256.New, macKey)
	h.Write(a.PublicKey().Bytes())
	h.Write(b.PublicKey().Bytes())
	h.Write(msg)
	want := h.Sum(nil)[:8]
	got := messageMAC(macKey, a.PublicKey(), b.PublicKey(), msg)
	if !bytes.Equal(got, want) {
		t.Errorf("MAC %x, want %x", got, want)
	}
	if bytes.Equal(messageMAC(macKey, b.PublicKey(), a.PublicKey(), msg), want) {
		t.Error("MAC ignores the sender/receiver order")
	}
}
