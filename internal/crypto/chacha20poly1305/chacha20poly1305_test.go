package chacha20poly1305

import (
	"bytes"
	"encoding/hex"
	"math/big"
	"math/rand/v2"
	"strings"
	"testing"

	"github.com/aren55555/signal-cli-go/internal/testvec"
)

func unhex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(strings.NewReplacer(" ", "", ":", "", "\n", "", "\t", "").Replace(s))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// TestRFC8439AEAD is the example of RFC 8439 §2.8.2.
func TestRFC8439AEAD(t *testing.T) {
	plain := []byte("Ladies and Gentlemen of the class of '99: If I could offer you only one tip for the future, sunscreen would be it.")
	ad := unhex(t, "50515253c0c1c2c3c4c5c6c7")
	key := unhex(t, "808182838485868788898a8b8c8d8e8f909192939495969798999a9b9c9d9e9f")
	nonce := unhex(t, "070000004041424344454647")
	ct := unhex(t, `d31a8d34648e60db7b86afbc53ef7ec2a4aded51296e08fea9e2b5a736ee62d6
		3dbea45e8ca9671282fafb69da92728b1a71de0a9e060b2905d6a5b67ecd3b36
		92ddbd7f2d778b8c9803aee328091b58fab324e4fad675945585808b4831d7bc
		3ff4def08e4b7a9de576d26586cec64b6116`)
	tag := unhex(t, "1ae10b594f09e26a7e902ecbd0600691")
	pk := PolyKey((*[32]byte)(key), (*[12]byte)(nonce))
	if hex.EncodeToString(pk[:]) != "7bac2b252db447af09b67a55a4e955840ae1d6731075d9eb2a9375783ed553ff" {
		t.Errorf("poly key %x", pk)
	}
	a, _ := New(key)
	sealed := a.Seal(nil, nonce, plain, ad)
	if !bytes.Equal(sealed, append(append([]byte{}, ct...), tag...)) {
		t.Fatalf("seal %x", sealed)
	}
	got, err := a.Open(nil, nonce, sealed, ad)
	if err != nil || !bytes.Equal(got, plain) {
		t.Fatalf("open %q %v", got, err)
	}
	for i := range sealed {
		bad := bytes.Clone(sealed)
		bad[i] ^= 0x80
		if _, err := a.Open(nil, nonce, bad, ad); err == nil {
			t.Fatalf("flipped byte %d accepted", i)
		}
	}
	if _, err := a.Open(nil, nonce, sealed, ad[1:]); err == nil {
		t.Error("changed AD accepted")
	}
}

// TestSplitWrites: the tag does not depend on how the input is chunked.
func TestSplitWrites(t *testing.T) {
	var key [32]byte
	for i := range key {
		key[i] = byte(i * 7)
	}
	msg := bytes.Repeat([]byte("0123456789abcdefXYZ"), 9)
	var want [16]byte
	Poly1305(&want, msg, &key)
	for split := range len(msg) {
		p := newPoly1305(&key)
		p.write(msg[:split])
		p.write(msg[split:])
		var got [16]byte
		p.sum(&got)
		if got != want {
			t.Fatalf("split %d: %x != %x", split, got, want)
		}
	}
}

// bigPoly1305 is RFC 8439 §2.5 written directly with math/big.
func bigPoly1305(msg []byte, key *[32]byte) [16]byte {
	le := func(b []byte) *big.Int {
		r := make([]byte, len(b))
		for i := range b {
			r[len(b)-1-i] = b[i]
		}
		return new(big.Int).SetBytes(r)
	}
	rb := bytes.Clone(key[:16])
	rb[3] &= 15
	rb[7] &= 15
	rb[11] &= 15
	rb[15] &= 15
	rb[4] &= 252
	rb[8] &= 252
	rb[12] &= 252
	r, s := le(rb), le(key[16:])
	p := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 130), big.NewInt(5))
	acc := new(big.Int)
	for len(msg) > 0 {
		n := min(16, len(msg))
		blk := append(bytes.Clone(msg[:n]), 1)
		acc.Add(acc, le(blk))
		acc.Mul(acc, r)
		acc.Mod(acc, p)
		msg = msg[n:]
	}
	acc.Add(acc, s)
	var out [16]byte
	b := acc.Bytes()
	for i := 0; i < 16 && i < len(b); i++ {
		out[i] = b[len(b)-1-i]
	}
	return out
}

func TestPoly1305AgainstBig(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	for i := range 3000 {
		var key [32]byte
		for j := range key {
			key[j] = byte(rng.Uint32())
		}
		msg := make([]byte, rng.IntN(100))
		for j := range msg {
			msg[j] = byte(rng.Uint32())
		}
		if i%3 == 0 { // all-ones data and key push limbs to their limits
			for j := range msg {
				msg[j] = 0xff
			}
			for j := range key {
				key[j] = 0xff
			}
		}
		var got [16]byte
		Poly1305(&got, msg, &key)
		if want := bigPoly1305(msg, &key); got != want {
			t.Fatalf("case %d: %x != %x", i, got, want)
		}
	}
}

// TestRFC8439Vectors runs every block, cipher, Poly1305, key-generation
// and AEAD vector of RFC 8439 §2 and Appendix A
// (testdata/vectors/rfc8439_chacha20poly1305.json).
func TestRFC8439Vectors(t *testing.T) {
	var f struct {
		Vectors []struct {
			Kind, Section, Key, Nonce, IV, Constant, AAD, Plaintext, Ciphertext, Keystream, Message, Tag, OTK string
			Counter                                                                                           uint32
		}
	}
	testvec.Load(t, "rfc8439_chacha20poly1305.json", &f)
	n := 0
	for _, v := range f.Vectors {
		key := [32]byte(unhexOpt(t, v.Key, 32))
		switch v.Kind {
		case "chacha20_block":
			var b [64]byte
			Block(&b, &key, v.Counter, (*[12]byte)(unhex(t, v.Nonce)))
			if hex.EncodeToString(b[:]) != v.Keystream {
				t.Errorf("%s: %x", v.Section, b)
			}
		case "chacha20_encrypt":
			p := unhex(t, v.Plaintext)
			out := make([]byte, len(p))
			XORKeyStream(out, p, &key, v.Counter, (*[12]byte)(unhex(t, v.Nonce)))
			if hex.EncodeToString(out) != v.Ciphertext {
				t.Errorf("%s: %x", v.Section, out)
			}
		case "poly1305":
			var tag [16]byte
			Poly1305(&tag, unhex(t, v.Message), &key)
			if hex.EncodeToString(tag[:]) != v.Tag {
				t.Errorf("%s: %x", v.Section, tag)
			}
		case "poly1305_key_gen":
			if k := PolyKey(&key, (*[12]byte)(unhex(t, v.Nonce))); hex.EncodeToString(k[:]) != v.OTK {
				t.Errorf("%s: %x", v.Section, k)
			}
		case "aead_encrypt", "aead_decrypt":
			nonce := v.Nonce
			if nonce == "" {
				nonce = v.Constant + v.IV
			}
			a, _ := New(key[:])
			sealed := a.Seal(nil, unhex(t, nonce), unhex(t, v.Plaintext), unhex(t, v.AAD))
			want := v.Ciphertext + v.Tag
			if v.Tag == "" {
				want = v.Ciphertext
			}
			if hex.EncodeToString(sealed)[:len(v.Ciphertext)] != v.Ciphertext {
				t.Errorf("%s: %x", v.Section, sealed)
			}
			if v.Tag != "" && hex.EncodeToString(sealed) != want {
				t.Errorf("%s: tag %x", v.Section, sealed[len(sealed)-16:])
			}
		default:
			continue
		}
		n++
	}
	if n < 25 {
		t.Errorf("only %d vectors ran", n)
	}
}

func unhexOpt(t *testing.T, s string, n int) []byte {
	if s == "" {
		return make([]byte, n)
	}
	return unhex(t, s)
}
