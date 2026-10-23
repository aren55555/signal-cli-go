package wire

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"math"
	"math/rand/v2"
	"testing"
)

func unhex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// TestEncodingGuideExamples uses the worked examples from the protobuf
// encoding guide (https://protobuf.dev/programming-guides/encoding/).
func TestEncodingGuideExamples(t *testing.T) {
	// message Test1 { int32 a = 1; } with a = 150
	if got := hex.EncodeToString(AppendVarint(AppendTag(nil, 1, VarintType), 150)); got != "089601" {
		t.Errorf("a=150: %s", got)
	}
	// message Test2 { string b = 2; } with b = "testing"
	if got := hex.EncodeToString(AppendBytes(AppendTag(nil, 2, BytesType), []byte("testing"))); got != "120774657374696e67" {
		t.Errorf("b=testing: %s", got)
	}
	// message Test3 { Test1 c = 3; } with c.a = 150
	inner := AppendVarint(AppendTag(nil, 1, VarintType), 150)
	if got := hex.EncodeToString(AppendBytes(AppendTag(nil, 3, BytesType), inner)); got != "1a03089601" {
		t.Errorf("c.a=150: %s", got)
	}
	// ZigZag table.
	for _, c := range []struct {
		v int64
		z uint64
	}{{0, 0}, {-1, 1}, {1, 2}, {-2, 3}, {0x7fffffff, 0xfffffffe}, {-0x80000000, 0xffffffff}} {
		if got := EncodeZigZag(c.v); got != c.z {
			t.Errorf("EncodeZigZag(%d) = %d, want %d", c.v, got, c.z)
		}
		if got := DecodeZigZag(c.z); got != c.v {
			t.Errorf("DecodeZigZag(%d) = %d, want %d", c.z, got, c.v)
		}
	}
}

func edgeUint64s(r *rand.Rand) []uint64 {
	vs := []uint64{0, 1, 127, 128, 255, 300, 16383, 16384, math.MaxUint32, math.MaxUint32 + 1, math.MaxInt64, math.MaxUint64}
	for i := range 64 {
		vs = append(vs, 1<<i, 1<<i-1)
	}
	for range 1000 {
		vs = append(vs, r.Uint64()>>r.IntN(64))
	}
	return vs
}

// TestVarintMatchesEncodingBinary uses encoding/binary's LEB128 varints,
// which are the protobuf varint format, as an independent oracle.
func TestVarintMatchesEncodingBinary(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 1))
	for _, v := range edgeUint64s(r) {
		got := AppendVarint(nil, v)
		want := binary.AppendUvarint(nil, v)
		if !bytes.Equal(got, want) {
			t.Fatalf("AppendVarint(%d) = %x, want %x", v, got, want)
		}
		if SizeVarint(v) != len(want) {
			t.Fatalf("SizeVarint(%d) = %d, want %d", v, SizeVarint(v), len(want))
		}
		dec, n, err := ConsumeVarint(append(got, 0xaa))
		if err != nil || dec != v || n != len(want) {
			t.Fatalf("ConsumeVarint(%x) = %d, %d, %v", got, dec, n, err)
		}
		// sint64 uses zigzag, which is what binary.AppendVarint does too.
		s := int64(v)
		if got, want := AppendVarint(nil, EncodeZigZag(s)), binary.AppendVarint(nil, s); !bytes.Equal(got, want) {
			t.Fatalf("zigzag varint of %d = %x, want %x", s, got, want)
		}
	}
	if got := hex.EncodeToString(AppendVarint(nil, math.MaxUint64)); got != "ffffffffffffffffff01" {
		t.Errorf("max uint64 = %s", got)
	}
	// Non-minimal encodings are valid protobuf and decode normally.
	if v, n, err := ConsumeVarint([]byte{0x80, 0x80, 0x00}); err != nil || v != 0 || n != 3 {
		t.Errorf("non-minimal zero: %d, %d, %v", v, n, err)
	}
}

func TestFixed(t *testing.T) {
	if got := hex.EncodeToString(AppendFixed32(nil, 0x01020304)); got != "04030201" {
		t.Errorf("fixed32: %s", got)
	}
	if got := hex.EncodeToString(AppendFixed64(nil, 0x0102030405060708)); got != "0807060504030201" {
		t.Errorf("fixed64: %s", got)
	}
	if v, n, err := ConsumeFixed32(unhex(t, "04030201ff")); err != nil || v != 0x01020304 || n != 4 {
		t.Errorf("ConsumeFixed32: %x %d %v", v, n, err)
	}
	if v, n, err := ConsumeFixed64(unhex(t, "0807060504030201")); err != nil || v != 0x0102030405060708 || n != 8 {
		t.Errorf("ConsumeFixed64: %x %d %v", v, n, err)
	}
	if _, _, err := ConsumeFixed32(unhex(t, "040302")); !errors.Is(err, ErrTruncated) {
		t.Errorf("short fixed32: %v", err)
	}
	if _, _, err := ConsumeFixed64(unhex(t, "08070605040302")); !errors.Is(err, ErrTruncated) {
		t.Errorf("short fixed64: %v", err)
	}
}

func TestTags(t *testing.T) {
	for _, c := range []struct {
		num Number
		typ Type
		hex string
	}{{1, VarintType, "08"}, {2, BytesType, "12"}, {15, Fixed32Type, "7d"}, {16, VarintType, "8001"}, {MaxNumber, Fixed64Type, "f9ffffff0f"}} {
		b := AppendTag(nil, c.num, c.typ)
		if got := hex.EncodeToString(b); got != c.hex {
			t.Errorf("AppendTag(%d, %d) = %s, want %s", c.num, c.typ, got, c.hex)
		}
		num, typ, n, err := ConsumeTag(b)
		if err != nil || num != c.num || typ != c.typ || n != len(b) {
			t.Errorf("ConsumeTag(%s) = %d, %d, %d, %v", c.hex, num, typ, n, err)
		}
	}
}

// TestFieldsRoundTrip builds a message with every wire type, including an
// unknown group, and checks that the parsed fields carry the right values
// and that concatenating the raw fields reproduces the input exactly, which
// is how messages preserve unknown fields.
func TestFieldsRoundTrip(t *testing.T) {
	var msg []byte
	msg = AppendVarint(AppendTag(msg, 1, VarintType), 150)
	msg = AppendFixed64(AppendTag(msg, 2, Fixed64Type), 0x1122334455667788)
	msg = AppendBytes(AppendTag(msg, 3, BytesType), []byte("hi"))
	msg = AppendFixed32(AppendTag(msg, 4, Fixed32Type), 0xdeadbeef)
	// group 5 { field 1 varint 7; group 2 { } }
	msg = AppendTag(msg, 5, StartGroupType)
	msg = AppendVarint(AppendTag(msg, 1, VarintType), 7)
	msg = AppendTag(AppendTag(msg, 2, StartGroupType), 2, EndGroupType)
	msg = AppendTag(msg, 5, EndGroupType)
	msg = AppendBytes(AppendTag(msg, 1000, BytesType), nil)

	type want struct {
		num  Number
		typ  Type
		val  uint64
		data string
	}
	wants := []want{
		{1, VarintType, 150, ""},
		{2, Fixed64Type, 0x1122334455667788, ""},
		{3, BytesType, 0, "hi"},
		{4, Fixed32Type, 0xdeadbeef, ""},
		{5, StartGroupType, 0, string(AppendTag(AppendTag(AppendVarint(AppendTag(nil, 1, VarintType), 7), 2, StartGroupType), 2, EndGroupType))},
		{1000, BytesType, 0, ""},
	}
	var raw []byte
	i := 0
	for f, err := range Fields(msg) {
		if err != nil {
			t.Fatal(err)
		}
		if i >= len(wants) {
			t.Fatalf("extra field %+v", f)
		}
		w := wants[i]
		if f.Num != w.num || f.Type != w.typ || f.Value != w.val || string(f.Bytes) != w.data {
			t.Errorf("field %d = {%d %d %#x %q}, want %+v", i, f.Num, f.Type, f.Value, f.Bytes, w)
		}
		raw = append(raw, f.Raw...)
		i++
	}
	if i != len(wants) {
		t.Fatalf("got %d fields, want %d", i, len(wants))
	}
	if !bytes.Equal(raw, msg) {
		t.Errorf("raw fields %x, want %x", raw, msg)
	}
}

func TestMalformed(t *testing.T) {
	for _, c := range []struct {
		name string
		in   string
		err  error
	}{
		{"truncated tag", "80", ErrTruncated},
		{"truncated varint value", "0880", ErrTruncated},
		{"varint over 10 bytes", "08ffffffffffffffffffff01", ErrOverflow},
		{"varint 10th byte > 1", "08ffffffffffffffffff02", ErrOverflow},
		{"field number 0", "0001", ErrFieldNumber},
		{"field number too large", "8080808080010a", ErrFieldNumber},
		{"wire type 6", "0e", ErrWireType},
		{"wire type 7", "0f", ErrWireType},
		{"bytes longer than input", "1205616263", ErrTruncated},
		{"bytes length overflows int", "12ffffffffffffffff7f", ErrTruncated},
		{"truncated fixed32", "0d0102", ErrTruncated},
		{"truncated fixed64", "09010203040506", ErrTruncated},
		{"end group at top level", "0c", ErrGroup},
		{"unterminated group", "0b0801", ErrTruncated},
		{"mismatched end group", "0b14", ErrGroup},
	} {
		in := unhex(t, c.in)
		var got error
		for _, err := range Fields(in) {
			if err != nil {
				got = err
				break
			}
		}
		if !errors.Is(got, c.err) {
			t.Errorf("%s (%s): error %v, want %v", c.name, c.in, got, c.err)
		}
	}

	// Groups nested beyond MaxDepth are rejected rather than recursing.
	var deep []byte
	for range MaxDepth + 1 {
		deep = AppendTag(deep, 1, StartGroupType)
	}
	for range MaxDepth + 1 {
		deep = AppendTag(deep, 1, EndGroupType)
	}
	var got error
	for _, err := range Fields(deep) {
		got = err
	}
	if !errors.Is(got, ErrGroup) {
		t.Errorf("deep groups: %v", got)
	}
}

func TestFieldsStopsEarly(t *testing.T) {
	msg := AppendVarint(AppendTag(AppendVarint(AppendTag(nil, 1, VarintType), 1), 2, VarintType), 2)
	n := 0
	for range Fields(msg) {
		n++
		break
	}
	if n != 1 {
		t.Errorf("iterated %d times after break", n)
	}
}

// FuzzFields checks that parsing never panics, and that when a whole
// buffer parses, its raw fields reassemble it byte for byte.
func FuzzFields(f *testing.F) {
	for _, s := range []string{"089601120774657374696e67", "0b08010c", "1a03089601"} {
		b, _ := hex.DecodeString(s)
		f.Add(b)
	}
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, in []byte) {
		var raw []byte
		for fl, err := range Fields(in) {
			if err != nil {
				return
			}
			if fl.Num < 1 || fl.Num > MaxNumber {
				t.Fatalf("field number %d out of range", fl.Num)
			}
			raw = append(raw, fl.Raw...)
		}
		if !bytes.Equal(raw, in) {
			t.Fatalf("raw fields %x, input %x", raw, in)
		}
	})
}
