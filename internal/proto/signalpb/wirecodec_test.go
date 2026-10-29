// SPDX-License-Identifier: GPL-3.0-or-later

// This file is the hand-written protobuf wire codec that the project used
// before the protobuf amendment (PLAN.md, Decisions). It is kept only as an
// independent oracle for differential tests of the generated code, which
// is why it lives in a _test.go file and is never compiled into the binary.
//
// Original description: varints, fixed-width values, length-delimited
// values, and tags.
//
// Decoding is for untrusted input: every length is checked against the
// buffer, varints are limited to 10 bytes, and group nesting is limited to
// MaxDepth. Parsed fields keep their raw bytes so messages can preserve
// unknown fields by appending them back unchanged.
package signalpb

import (
	"encoding/binary"
	"errors"
	"fmt"
	"iter"
	"math/bits"
)

// Number is a field number.
type Number int32

// Type is a wire type.
type Type int8

// The wire types. Groups are deprecated but still parsed, so that unknown
// group fields survive a round trip.
const (
	VarintType     Type = 0
	Fixed64Type    Type = 1
	BytesType      Type = 2
	StartGroupType Type = 3
	EndGroupType   Type = 4
	Fixed32Type    Type = 5
)

// MaxNumber is the largest valid field number.
const MaxNumber Number = 1<<29 - 1

// MaxDepth is the deepest group nesting the parser accepts.
const MaxDepth = 100

// Decoding errors. Returned errors wrap one of these.
var (
	ErrTruncated   = errors.New("wire: truncated input")
	ErrOverflow    = errors.New("wire: varint overflows 64 bits")
	ErrFieldNumber = errors.New("wire: invalid field number")
	ErrWireType    = errors.New("wire: invalid wire type")
	ErrGroup       = errors.New("wire: malformed group")
)

// AppendTag appends the tag for field num with wire type typ.
func AppendTag(b []byte, num Number, typ Type) []byte {
	return AppendVarint(b, uint64(num)<<3|uint64(typ&7))
}

// AppendVarint appends v as a base-128 varint.
func AppendVarint(b []byte, v uint64) []byte {
	for v >= 0x80 {
		b = append(b, byte(v)|0x80)
		v >>= 7
	}
	return append(b, byte(v))
}

// SizeVarint returns the encoded length of v.
func SizeVarint(v uint64) int {
	// 1 byte per started group of 7 bits, and 1 for v = 0.
	return (bits.Len64(v|1) + 6) / 7
}

// AppendFixed32 appends v little-endian (fixed32, sfixed32, float).
func AppendFixed32(b []byte, v uint32) []byte { return binary.LittleEndian.AppendUint32(b, v) }

// AppendFixed64 appends v little-endian (fixed64, sfixed64, double).
func AppendFixed64(b []byte, v uint64) []byte { return binary.LittleEndian.AppendUint64(b, v) }

// AppendBytes appends v with its length prefix (bytes, string, embedded
// messages, packed repeated fields).
func AppendBytes(b, v []byte) []byte {
	return append(AppendVarint(b, uint64(len(v))), v...)
}

// EncodeZigZag maps a signed value to the unsigned varint value of the
// sint32/sint64 types: 0, -1, 1, -2 ... to 0, 1, 2, 3 ...
func EncodeZigZag(v int64) uint64 { return uint64(v<<1) ^ uint64(v>>63) }

// DecodeZigZag inverts EncodeZigZag.
func DecodeZigZag(v uint64) int64 { return int64(v>>1) ^ -int64(v&1) }

// ConsumeVarint parses a varint at the start of b and returns it with the
// number of bytes read. Non-minimal encodings are accepted, as protobuf
// requires.
func ConsumeVarint(b []byte) (uint64, int, error) {
	var v uint64
	for i := 0; i < 10; i++ {
		if i >= len(b) {
			return 0, 0, ErrTruncated
		}
		c := b[i]
		if i == 9 && c > 1 {
			return 0, 0, ErrOverflow // bits beyond 64, or an 11th byte
		}
		v |= uint64(c&0x7f) << (7 * i)
		if c < 0x80 {
			return v, i + 1, nil
		}
	}
	panic("unreachable")
}

// ConsumeFixed32 parses a little-endian uint32 at the start of b.
func ConsumeFixed32(b []byte) (uint32, int, error) {
	if len(b) < 4 {
		return 0, 0, ErrTruncated
	}
	return binary.LittleEndian.Uint32(b), 4, nil
}

// ConsumeFixed64 parses a little-endian uint64 at the start of b.
func ConsumeFixed64(b []byte) (uint64, int, error) {
	if len(b) < 8 {
		return 0, 0, ErrTruncated
	}
	return binary.LittleEndian.Uint64(b), 8, nil
}

// ConsumeBytes parses a length-prefixed value at the start of b. The
// returned slice aliases b.
func ConsumeBytes(b []byte) ([]byte, int, error) {
	l, n, err := ConsumeVarint(b)
	if err != nil {
		return nil, 0, err
	}
	if l > uint64(len(b)-n) {
		return nil, 0, fmt.Errorf("%w: length %d with %d bytes left", ErrTruncated, l, len(b)-n)
	}
	end := n + int(l)
	return b[n:end:end], end, nil
}

// ConsumeTag parses a tag at the start of b.
func ConsumeTag(b []byte) (Number, Type, int, error) {
	v, n, err := ConsumeVarint(b)
	if err != nil {
		return 0, 0, 0, err
	}
	num, typ := v>>3, Type(v&7)
	if num == 0 || num > uint64(MaxNumber) {
		return 0, 0, 0, fmt.Errorf("%w: %d", ErrFieldNumber, num)
	}
	if typ > Fixed32Type {
		return 0, 0, 0, fmt.Errorf("%w: %d", ErrWireType, typ)
	}
	return Number(num), typ, n, nil
}

// Field is one parsed field.
type Field struct {
	Num  Number
	Type Type
	// Value holds the value of varint, fixed32 and fixed64 fields.
	Value uint64
	// Bytes holds the content of a length-delimited field, or for a group
	// everything between its start and end tags. It aliases the input.
	Bytes []byte
	// Raw is the whole field, tag included. Appending the Raw of every
	// field reproduces the input; messages keep the Raw of fields they
	// don't know to preserve them.
	Raw []byte
}

// Fields iterates over the fields of a message. On a decoding error it
// yields the error once and stops.
func Fields(b []byte) iter.Seq2[Field, error] {
	return func(yield func(Field, error) bool) {
		for len(b) > 0 {
			f, n, err := consumeField(b, 0)
			if err != nil {
				yield(Field{}, err)
				return
			}
			if !yield(f, nil) {
				return
			}
			b = b[n:]
		}
	}
}

// consumeField parses one field at the start of b. depth is the number of
// groups already open around it.
func consumeField(b []byte, depth int) (Field, int, error) {
	num, typ, n, err := ConsumeTag(b)
	if err != nil {
		return Field{}, 0, err
	}
	f := Field{Num: num, Type: typ}
	rest := b[n:]
	var m int
	switch typ {
	case VarintType:
		f.Value, m, err = ConsumeVarint(rest)
	case Fixed64Type:
		f.Value, m, err = ConsumeFixed64(rest)
	case Fixed32Type:
		var v uint32
		v, m, err = ConsumeFixed32(rest)
		f.Value = uint64(v)
	case BytesType:
		f.Bytes, m, err = ConsumeBytes(rest)
	case StartGroupType:
		f.Bytes, m, err = consumeGroup(num, rest, depth+1)
	case EndGroupType:
		return Field{}, 0, fmt.Errorf("%w: end tag for field %d without a start", ErrGroup, num)
	}
	if err != nil {
		return Field{}, 0, err
	}
	f.Raw = b[: n+m : n+m]
	return f, n + m, nil
}

// consumeGroup parses fields up to the end tag for field num. It returns
// the content before the end tag and the length including the end tag.
func consumeGroup(num Number, b []byte, depth int) ([]byte, int, error) {
	if depth > MaxDepth {
		return nil, 0, fmt.Errorf("%w: nested deeper than %d", ErrGroup, MaxDepth)
	}
	off := 0
	for {
		if off >= len(b) {
			return nil, 0, fmt.Errorf("%w: group %d not terminated", ErrTruncated, num)
		}
		endNum, typ, n, err := ConsumeTag(b[off:])
		if err != nil {
			return nil, 0, err
		}
		if typ == EndGroupType {
			if endNum != num {
				return nil, 0, fmt.Errorf("%w: group %d ended by field %d", ErrGroup, num, endNum)
			}
			return b[:off:off], off + n, nil
		}
		_, m, err := consumeField(b[off:], depth)
		if err != nil {
			return nil, 0, err
		}
		off += m
	}
}
