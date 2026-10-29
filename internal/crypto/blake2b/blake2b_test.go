// SPDX-License-Identifier: GPL-3.0-or-later

package blake2b

import (
	"bytes"
	"encoding/hex"
	"testing"
)

// TestRFC7693Abc is RFC 7693 Appendix A.
func TestRFC7693Abc(t *testing.T) {
	want := "ba80a53f981c4d0d6a2797b69f12f6e94c212f14685ac4b74b12bb6fdbffa2d17d87c5392aab792dc252d5de4533cc9518d38aa8dbf1925ab92386edd4009923"
	if got := hex.EncodeToString(Sum(64, []byte("abc"))); got != want {
		t.Errorf("BLAKE2b-512(abc) = %s", got)
	}
}

// selftestSeq is the deterministic Fibonacci sequence of RFC 7693
// Appendix E.
func selftestSeq(n int, seed uint32) []byte {
	out := make([]byte, n)
	a, b := 0xDEAD4BAD*seed, uint32(1)
	for i := range out {
		a, b = b, a+b
		out[i] = byte(b >> 24)
	}
	return out
}

// TestRFC7693Selftest is the grand hash of RFC 7693 Appendix E: output
// lengths 20..64, input lengths across block boundaries, keyed and not.
func TestRFC7693Selftest(t *testing.T) {
	grand, _ := New(32, nil)
	for _, outlen := range []int{20, 32, 48, 64} {
		for _, inlen := range []int{0, 3, 128, 129, 255, 1024} {
			in := selftestSeq(inlen, uint32(inlen))
			grand.Write(Sum(outlen, in))
			d, err := New(outlen, selftestSeq(outlen, uint32(outlen)))
			if err != nil {
				t.Fatal(err)
			}
			d.Write(in)
			grand.Write(d.Sum(nil))
		}
	}
	if got := hex.EncodeToString(grand.Sum(nil)); got != "c23a7800d98123bd10f506c61e29da5603d763b8bbad2e737f5e765a7bccd475" {
		t.Errorf("grand hash %s", got)
	}
}

// TestIncremental: chunking and Reset do not change the result.
func TestIncremental(t *testing.T) {
	in := selftestSeq(700, 9)
	d, _ := New(48, []byte("key"))
	d.Write(in)
	want := d.Sum(nil)
	for split := 0; split <= len(in); split += 37 {
		d.Reset()
		d.Write(in[:split])
		d.Write(in[split:])
		if got := d.Sum(nil); !bytes.Equal(got, want) {
			t.Fatalf("split %d differs", split)
		}
	}
}
