package h2

import (
	"bytes"
	"encoding/hex"
	"errors"
	"math/rand/v2"
	"testing"

	"github.com/aren55555/signal-cli-go/internal/testvec"
)

// TestIntegers are RFC 7541 C.1.
func TestIntegers(t *testing.T) {
	for _, c := range []struct {
		prefix int
		v      uint64
		hex    string
	}{{5, 10, "0a"}, {5, 1337, "1f9a0a"}, {8, 42, "2a"}} {
		got := appendInt(nil, c.prefix, 0, c.v)
		if hex.EncodeToString(got) != c.hex {
			t.Errorf("encode %d on %d bits: %x, want %s", c.v, c.prefix, got, c.hex)
		}
		v, n, err := readInt(got, c.prefix)
		if err != nil || v != c.v || n != len(got) {
			t.Errorf("decode %s: %d, %d, %v", c.hex, v, n, err)
		}
	}
	if _, _, err := readInt([]byte{0x1f, 0xff}, 5); !errors.Is(err, ErrHPACK) {
		t.Errorf("truncated integer: %v", err)
	}
	if _, _, err := readInt([]byte{0x1f, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x01}, 5); !errors.Is(err, ErrHPACK) {
		t.Errorf("overflowing integer: %v", err)
	}
}

func TestHuffmanRoundTrip(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	for range 2000 {
		s := make([]byte, r.IntN(60))
		for i := range s {
			s[i] = byte(r.Uint32())
		}
		enc := huffmanEncode(nil, s)
		dec, err := huffmanDecode(enc)
		if err != nil || !bytes.Equal(dec, s) {
			t.Fatalf("round trip of %x: %x, %v", s, dec, err)
		}
	}
	// Padding longer than 7 bits, or not all ones, is an error (RFC 7541 §5.2).
	for _, bad := range []string{"ff", "fe", "1f"} {
		b, _ := hex.DecodeString(bad)
		if _, err := huffmanDecode(b); err == nil && bad != "1f" {
			t.Errorf("padding %s accepted", bad)
		}
	}
}

type hpackFile struct {
	Sequences []struct {
		Name           string
		Representation string
		Huffman        bool
		TableSizeLimit uint32 `json:"table_size_limit"`
		Steps          []struct {
			Headers    [][2]string
			Encoded    string
			EntrySizes []uint32 `json:"dynamic_table_entry_sizes"`
			TableSize  uint32   `json:"dynamic_table_size"`
		}
	}
}

func sizesOf(tb *dynamicTable) []uint32 {
	var out []uint32
	for i := range tb.len() {
		f := tb.at(i)
		out = append(out, entrySize(f))
	}
	return out
}

// TestRFC7541Examples decodes every Appendix C example, checking the
// dynamic table after each step, and encodes the C.3-C.6 sequences byte
// for byte.
func TestRFC7541Examples(t *testing.T) {
	var f hpackFile
	testvec.Load(t, "rfc7541_hpack.json", &f)
	if len(f.Sequences) != 8 {
		t.Fatalf("got %d sequences", len(f.Sequences))
	}
	for _, s := range f.Sequences {
		dec := NewDecoder(s.TableSizeLimit)
		enc := NewEncoder(s.TableSizeLimit, s.Huffman)
		for i, st := range s.Steps {
			block := testvec.Hex(t, st.Encoded)
			got, err := dec.Decode(block)
			if err != nil {
				t.Fatalf("%s step %d: %v", s.Name, i, err)
			}
			if len(got) != len(st.Headers) {
				t.Fatalf("%s step %d: %d fields, want %d", s.Name, i, len(got), len(st.Headers))
			}
			for j, h := range st.Headers {
				if got[j].Name != h[0] || got[j].Value != h[1] {
					t.Errorf("%s step %d field %d: %q: %q, want %q: %q", s.Name, i, j, got[j].Name, got[j].Value, h[0], h[1])
				}
			}
			if dec.table.size != st.TableSize || !equalU32(sizesOf(&dec.table), st.EntrySizes) {
				t.Errorf("%s step %d: decoder table %v (%d), want %v (%d)", s.Name, i, sizesOf(&dec.table), dec.table.size, st.EntrySizes, st.TableSize)
			}
			if len(s.Name) != 3 { // C.2.x pick specific representations; skip encoding
				continue
			}
			var fields []HeaderField
			for _, h := range st.Headers {
				fields = append(fields, HeaderField{Name: h[0], Value: h[1]})
			}
			if out := enc.Encode(nil, fields); hex.EncodeToString(out) != st.Encoded {
				t.Errorf("%s step %d: encoded\n%x\nwant\n%s", s.Name, i, out, st.Encoded)
			}
		}
	}
}

func equalU32(a, b []uint32) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestDecoderErrors(t *testing.T) {
	for name, in := range map[string]string{
		"index 0":                    "80",
		"index past the tables":      "be", // 62, empty dynamic table
		"truncated literal":          "400a6375",
		"table size update too big":  "3fe21f", // 4097, one over the limit
		"size update after a header": "823f00",
	} {
		b, _ := hex.DecodeString(in)
		if _, err := NewDecoder(4096).Decode(b); !errors.Is(err, ErrHPACK) {
			t.Errorf("%s (%s): %v", name, in, err)
		}
	}
}

func TestSensitiveFieldsNeverIndexed(t *testing.T) {
	enc := NewEncoder(4096, false)
	out := enc.Encode(nil, []HeaderField{{Name: "authorization", Value: "Basic x", Sensitive: true}})
	if out[0]&0xf0 != 0x10 {
		t.Errorf("first byte %#x, want a never-indexed literal", out[0])
	}
	if enc.table.len() != 0 {
		t.Error("sensitive field entered the dynamic table")
	}
	got, err := NewDecoder(4096).Decode(out)
	if err != nil || !got[0].Sensitive || got[0].Value != "Basic x" {
		t.Errorf("decoded %+v, %v", got, err)
	}
}

// FuzzDecode: header blocks come from the server.
func FuzzDecode(f *testing.F) {
	f.Add([]byte{0x82, 0x86, 0x84, 0x41, 0x0f, 'w', 'w', 'w', '.', 'e', 'x', 'a', 'm', 'p', 'l', 'e', '.', 'c', 'o', 'm'})
	f.Fuzz(func(t *testing.T, in []byte) {
		d := NewDecoder(256)
		_, _ = d.Decode(in)
		if d.table.size > 256 {
			t.Fatalf("table size %d over the limit", d.table.size)
		}
	})
}
