// SPDX-License-Identifier: GPL-3.0-or-later

package spqr

import (
	"bytes"
	"encoding/hex"
	"math/rand/v2"
	"testing"

	"github.com/aren55555/signal-cli-go/internal/testvec"
)

// mulRef is carry-less multiplication then reduction by x^16 + x^12 + x^3
// + x + 1, written the slow way as an independent reference.
func mulRef(a, b uint16) uint16 {
	var p uint32
	for i := range 16 {
		if b>>i&1 == 1 {
			p ^= uint32(a) << i
		}
	}
	for i := 31; i >= 16; i-- {
		if p>>i&1 == 1 {
			p ^= 0x1100b << (i - 16)
		}
	}
	return uint16(p)
}

func TestGF(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	for range 20000 {
		a, b, c := uint16(r.Uint32()), uint16(r.Uint32()), uint16(r.Uint32())
		if gfMul(a, b) != mulRef(a, b) {
			t.Fatalf("gfMul(%#x, %#x) = %#x, want %#x", a, b, gfMul(a, b), mulRef(a, b))
		}
		if gfMul(gfMul(a, b), c) != gfMul(a, gfMul(b, c)) {
			t.Fatal("not associative")
		}
		if gfMul(a, b^c) != gfMul(a, b)^gfMul(a, c) {
			t.Fatal("not distributive")
		}
		if a != 0 && gfMul(a, gfInv(a)) != 1 {
			t.Fatalf("a * inv(a) != 1 for %#x", a)
		}
	}
}

// SPQR's four stream lengths (chunking.md §2).
var streamLens = []int{96, 1152, 960, 160}

func TestChunksAreSystematic(t *testing.T) {
	r := rand.New(rand.NewPCG(3, 4))
	for _, l := range streamLens {
		data := make([]byte, l)
		for i := range data {
			data[i] = byte(r.Uint32())
		}
		enc, err := newEncoder(data)
		if err != nil {
			t.Fatal(err)
		}
		for c := range l / 32 {
			if got := enc.chunk(uint16(c)); !bytes.Equal(got[:], data[32*c:32*c+32]) {
				t.Fatalf("L=%d: data chunk %d differs from the string", l, c)
			}
		}
	}
}

// TestAnyNChunksDecode: any n distinct indices, data or parity, in any
// order, rebuild the string (chunking.md §3).
func TestAnyNChunksDecode(t *testing.T) {
	r := rand.New(rand.NewPCG(5, 6))
	for _, l := range streamLens {
		n := l / 32
		data := make([]byte, l)
		for i := range data {
			data[i] = byte(r.Uint32())
		}
		enc, _ := newEncoder(data)
		for range 20 {
			// n distinct indices from 0..3n, shuffled.
			idx := r.Perm(3 * n)[:n]
			dec, err := newDecoder(l)
			if err != nil {
				t.Fatal(err)
			}
			for k, c := range idx {
				if dec.complete() {
					t.Fatalf("L=%d: complete after %d of %d chunks", l, k, n)
				}
				ch := enc.chunk(uint16(c))
				dec.add(uint16(c), ch[:])
			}
			if !dec.complete() {
				t.Fatalf("L=%d: not complete after %d chunks %v", l, n, idx)
			}
			if got := dec.bytes(); !bytes.Equal(got, data) {
				t.Fatalf("L=%d, chunks %v: decoded string differs", l, idx)
			}
		}
	}
}

// TestUpstreamCheckCases are the behaviours in chunking.md §4.
func TestUpstreamCheckCases(t *testing.T) {
	// 10 bytes: chunks 1 and 2 alone decode it.
	msg := []byte("abcdefghij")
	enc, _ := newEncoder(msg)
	dec, _ := newDecoder(len(msg))
	for _, c := range []uint16{1, 2} {
		ch := enc.chunk(c)
		dec.add(c, ch[:])
	}
	if !dec.complete() || !bytes.Equal(dec.bytes(), msg) {
		t.Errorf("10-byte message from chunks 1, 2: %q", dec.bytes())
	}

	// 1088 bytes of 0x03: parity chunks from 35 complete at the 34th (68).
	data := bytes.Repeat([]byte{3}, 1088)
	enc, _ = newEncoder(data)
	dec, _ = newDecoder(len(data))
	done := -1
	for c := 35; c <= 70; c++ {
		ch := enc.chunk(uint16(c))
		dec.add(uint16(c), ch[:])
		if dec.complete() {
			done = c
			break
		}
	}
	if done != 68 || !bytes.Equal(dec.bytes(), data) {
		t.Errorf("1088 bytes: complete at chunk %d, want 68", done)
	}
}

func TestDecoderFirstPointWins(t *testing.T) {
	data := bytes.Repeat([]byte{7}, 96)
	enc, _ := newEncoder(data)
	dec, _ := newDecoder(96)
	junk := bytes.Repeat([]byte{0xee}, 32)
	c0 := enc.chunk(0)
	dec.add(0, c0[:])
	dec.add(0, junk) // same index again: ignored
	for _, c := range []uint16{1, 2} {
		ch := enc.chunk(c)
		dec.add(c, ch[:])
	}
	if !bytes.Equal(dec.bytes(), data) {
		t.Error("a repeated index replaced the first chunk")
	}
}

func TestEncoderRejectsBadLengths(t *testing.T) {
	if _, err := newEncoder(make([]byte, 7)); err == nil {
		t.Error("odd length accepted")
	}
	if _, err := newDecoder(7); err == nil {
		t.Error("odd decoder length accepted")
	}
}

func TestVarintVectors(t *testing.T) {
	var f struct {
		Vectors []struct {
			Op       string
			Value    uint64
			BytesHex string `json:"bytes_hex"`
			InputHex string `json:"input_hex"`
			Start    int    `json:"start_offset"`
			End      int    `json:"end_offset"`
		}
	}
	testvec.Load(t, "spqr_varint.json", &f)
	if len(f.Vectors) != 3 {
		t.Fatalf("got %d vectors", len(f.Vectors))
	}
	for _, v := range f.Vectors {
		switch v.Op {
		case "encode":
			if got := hex.EncodeToString(appendVarint(nil, v.Value)); got != v.BytesHex {
				t.Errorf("encode %d: %s, want %s", v.Value, got, v.BytesHex)
			}
		case "decode":
			in := testvec.Hex(t, v.InputHex)
			got, n, err := consumeVarint(in[v.Start:])
			if err != nil || got != v.Value || v.Start+n != v.End {
				t.Errorf("decode %s: %d, %d, %v", v.InputHex, got, n, err)
			}
		}
	}
	// Non-minimal forms are accepted; 10 continuation bytes are not.
	if v, n, err := consumeVarint([]byte{0x80, 0x00}); err != nil || v != 0 || n != 2 {
		t.Errorf("non-minimal: %d %d %v", v, n, err)
	}
	if _, _, err := consumeVarint(bytes.Repeat([]byte{0x80}, 10)); err == nil {
		t.Error("unterminated 10-byte varint accepted")
	}
	if _, _, err := consumeVarint([]byte{0x80}); err == nil {
		t.Error("truncated varint accepted")
	}
}

func TestMessageCodec(t *testing.T) {
	data := bytes.Repeat([]byte{0xd0}, 32)
	// The example layout in wire.md §5.
	m := message{epoch: 3, keyIndex: 17, payload: payloadEk, chunkIndex: 200, chunk: data}
	b := m.encode()
	want := append(testvec.Hex(t, "010311"+"02"+"c801"), data...)
	if !bytes.Equal(b, want) {
		t.Errorf("encoded %x, want %x", b, want)
	}
	got, err := parseMessage(append(b, 0xff, 0xff)) // trailing bytes are ignored
	if err != nil {
		t.Fatal(err)
	}
	if got.epoch != 3 || got.keyIndex != 17 || got.payload != payloadEk || got.chunkIndex != 200 || !bytes.Equal(got.chunk, data) {
		t.Errorf("parsed %+v", got)
	}
	none := message{epoch: 1, keyIndex: 1, payload: payloadNone}
	if b := none.encode(); len(b) != 4 {
		t.Errorf("chunkless message is %d bytes, want 4", len(b))
	}

	for name, in := range map[string]string{
		"epoch 0":           "01" + "00" + "01" + "00",
		"index > 2^32-1":    "01" + "01" + "8080808010" + "00",
		"no type":           "01" + "01" + "01",
		"type 7":            "01" + "01" + "01" + "07",
		"chunk index 65536": "01" + "01" + "01" + "02" + "808004" + hex.EncodeToString(data),
		"short chunk":       "01" + "01" + "01" + "02" + "00" + "d0d0",
		"bad epoch varint":  "01" + "80",
	} {
		if _, err := parseMessage(testvec.Hex(t, in)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

// FuzzPeerInput: SPQR messages and chunks come from the peer. Parsing
// must not panic; a parsed message re-encodes to a prefix of the input
// when its varints were minimal; any chunk can be fed to a decoder.
func FuzzPeerInput(f *testing.F) {
	f.Add(message{epoch: 3, keyIndex: 17, payload: payloadEk, chunkIndex: 200, chunk: make([]byte, 32)}.encode())
	f.Add([]byte{1, 1, 1, 4})
	f.Fuzz(func(t *testing.T, in []byte) {
		m, err := parseMessage(in)
		if err != nil {
			return
		}
		if m.payload.hasChunk() {
			dec, _ := newDecoder(96)
			dec.add(m.chunkIndex, m.chunk)
			if dec.complete() {
				_ = dec.bytes()
			}
		}
		if out := m.encode(); len(out) > len(in) {
			t.Fatalf("re-encoding %x is longer than the input %x", out, in)
		}
	})
}
