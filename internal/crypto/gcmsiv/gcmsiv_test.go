package gcmsiv

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/aren55555/signal-cli-go/internal/testvec"
)

type vector struct {
	Section, Plaintext, AAD, Key, Nonce, Tag, Result string
	AuthKey                                          string `json:"record_authentication_key"`
	EncKey                                           string `json:"record_encryption_key"`
	Polyval                                          string `json:"polyval_result"`
}

func loadVectors(t *testing.T) []vector {
	var f struct{ Vectors []vector }
	testvec.Load(t, "rfc8452_aes_gcm_siv.json", &f)
	if len(f.Vectors) != 50 {
		t.Fatalf("got %d vectors, want 50", len(f.Vectors))
	}
	return f.Vectors
}

// TestRFC8452 runs every appendix C vector: the derived keys, the POLYVAL
// result, sealing and opening.
func TestRFC8452(t *testing.T) {
	for i, v := range loadVectors(t) {
		h := func(s string) []byte { return testvec.Hex(t, s) }
		key, nonce, pt, aad := h(v.Key), h(v.Nonce), h(v.Plaintext), h(v.AAD)

		authKey, encKey, err := deriveKeys(key, nonce)
		if err != nil {
			t.Fatal(err)
		}
		if hex.EncodeToString(authKey) != v.AuthKey || hex.EncodeToString(encKey) != v.EncKey {
			t.Errorf("%s #%d: derived keys differ", v.Section, i)
		}
		if got := hex.EncodeToString(polyvalInput(authKey, aad, pt)); got != v.Polyval {
			t.Errorf("%s #%d: POLYVAL %s, want %s", v.Section, i, got, v.Polyval)
		}

		aead, err := New(key)
		if err != nil {
			t.Fatal(err)
		}
		got := aead.Seal(nil, nonce, pt, aad)
		if hex.EncodeToString(got) != v.Result {
			t.Errorf("%s #%d: sealed %x, want %s", v.Section, i, got, v.Result)
			continue
		}
		opened, err := aead.Open(nil, nonce, got, aad)
		if err != nil || !bytes.Equal(opened, pt) {
			t.Errorf("%s #%d: open: %v", v.Section, i, err)
		}
		for j := range got {
			bad := bytes.Clone(got)
			bad[j] ^= 0x01
			if _, err := aead.Open(nil, nonce, bad, aad); err == nil {
				t.Fatalf("%s #%d: flipped byte %d accepted", v.Section, i, j)
			}
		}
		if len(aad) > 0 {
			if _, err := aead.Open(nil, nonce, got, nil); err == nil {
				t.Errorf("%s #%d: wrong AAD accepted", v.Section, i)
			}
		}
	}
}

func TestSizes(t *testing.T) {
	if _, err := New(make([]byte, 24)); err == nil {
		t.Error("24-byte key accepted")
	}
	a, _ := New(make([]byte, 32))
	if a.NonceSize() != 12 || a.Overhead() != 16 {
		t.Errorf("nonce %d, overhead %d", a.NonceSize(), a.Overhead())
	}
	if _, err := a.Open(nil, make([]byte, 12), make([]byte, 15), nil); err == nil {
		t.Error("15-byte ciphertext accepted")
	}
}
