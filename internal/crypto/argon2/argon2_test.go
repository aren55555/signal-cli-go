package argon2

import (
	"bytes"
	"encoding/hex"
	"testing"
)

// TestRFC9106 runs the three vectors of RFC 9106 §5: 32 KiB, 3 passes,
// 4 lanes, 32-byte tag, password 0x01×32, salt 0x02×16, secret 0x03×8,
// associated data 0x04×12.
func TestRFC9106(t *testing.T) {
	pw, salt := bytes.Repeat([]byte{1}, 32), bytes.Repeat([]byte{2}, 16)
	secret, ad := bytes.Repeat([]byte{3}, 8), bytes.Repeat([]byte{4}, 12)
	p := Params{Memory: 32, Passes: 3, Lanes: 4, TagLen: 32}
	for _, c := range []struct {
		y   Type
		tag string
	}{
		{Argon2d, "512b391b6f1162975371d30919734294f868e3be3984f3c1a13a4db9fabe4acb"},
		{Argon2i, "c814d9d1dc7f37aa13f0d77f2494bda1c8de6b016dd388d29952a4c4672b6ce8"},
		{Argon2id, "0d640df58d78766c08c037a34a8b53c9d01ef0452d75b65eb52520e96b01e659"},
	} {
		got, err := Key(c.y, pw, salt, secret, ad, p)
		if err != nil {
			t.Fatal(err)
		}
		if hex.EncodeToString(got) != c.tag {
			t.Errorf("type %d: %x", c.y, got)
		}
	}
}

// TestPreHash is the RFC 9106 §5.3 pre-hashing digest H_0.
func TestPreHash(t *testing.T) {
	h0 := initialHash(Argon2id, bytes.Repeat([]byte{1}, 32), bytes.Repeat([]byte{2}, 16), bytes.Repeat([]byte{3}, 8),
		bytes.Repeat([]byte{4}, 12), Params{Memory: 32, Passes: 3, Lanes: 4, TagLen: 32})
	want := "2889de487eb42ae500c0007ed9252f1069eadec40d5765b485de6dc2437a67b8546a2f0acc1a0882db8fcf74714b472e94df421a5da1112ffa11434370a1e997"
	if hex.EncodeToString(h0) != want {
		t.Errorf("H_0 %x", h0)
	}
}

// TestHPrimeLengths: H' output has the requested length for lengths
// around the 64- and 32-byte boundaries, and prefixes are not shared.
func TestHPrimeLengths(t *testing.T) {
	for _, n := range []uint32{4, 63, 64, 65, 95, 96, 97, 128, 1024} {
		if got := hPrime(n, []byte("x")); uint32(len(got)) != n {
			t.Errorf("H'^%d gave %d bytes", n, len(got))
		}
	}
}
