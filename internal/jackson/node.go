// SPDX-License-Identifier: GPL-3.0-or-later

// Package jackson is a JSON tree with the behavior of the Jackson
// databind library as signal-cli 0.14.7 uses it: the parser reports
// Jackson's error messages and locations, numbers keep Jackson's node
// types (int, long, BigInteger, double), and the encoder writes Jackson's
// compact form (spec/surface/jsonrpc/errors.md, observations.md, and
// black-box probes of the reference's JSON-RPC reader).
package jackson

import (
	"math"
	"math/big"
	"strconv"
	"strings"
)

// Kind is a node type.
type Kind uint8

const (
	Missing Kind = iota // no value (an empty HTTP body)
	Null
	Bool
	Int    // Java Integer (IntNode)
	Long   // Java Long (LongNode)
	BigInt // java.math.BigInteger (BigIntegerNode)
	Double // Java Double (DoubleNode)
	String
	Array
	Object
)

// Node is one JSON value.
type Node struct {
	Kind  Kind
	B     bool
	I     int64    // Int, Long
	Big   *big.Int // BigInt
	F     float64  // Double
	S     string   // String
	Elems []*Node  // Array
	Keys  []string // Object, in insertion order
	Vals  []*Node
}

// Constructors.
func NewNull() *Node           { return &Node{Kind: Null} }
func NewBool(b bool) *Node     { return &Node{Kind: Bool, B: b} }
func NewString(s string) *Node { return &Node{Kind: String, S: s} }
func NewObject() *Node         { return &Node{Kind: Object} }
func NewArray(e ...*Node) *Node {
	return &Node{Kind: Array, Elems: e}
}

// NewInt makes the node Jackson would read for the integer n.
func NewInt(n int64) *Node {
	if n >= math.MinInt32 && n <= math.MaxInt32 {
		return &Node{Kind: Int, I: n}
	}
	return &Node{Kind: Long, I: n}
}

// Get is the member k of an object (nil if absent or not an object).
func (n *Node) Get(k string) *Node {
	if n == nil || n.Kind != Object {
		return nil
	}
	for i, key := range n.Keys {
		if key == k {
			return n.Vals[i]
		}
	}
	return nil
}

// Has reports whether an object has member k (even if null).
func (n *Node) Has(k string) bool { return n.Get(k) != nil }

// Set puts k, replacing an existing member in place (LinkedHashMap).
func (n *Node) Set(k string, v *Node) {
	for i, key := range n.Keys {
		if key == k {
			n.Vals[i] = v
			return
		}
	}
	n.Keys = append(n.Keys, k)
	n.Vals = append(n.Vals, v)
}

// Remove deletes member k.
func (n *Node) Remove(k string) {
	for i, key := range n.Keys {
		if key == k {
			n.Keys = append(n.Keys[:i:i], n.Keys[i+1:]...)
			n.Vals = append(n.Vals[:i:i], n.Vals[i+1:]...)
			return
		}
	}
}

// IsNull reports a missing node or JSON null.
func (n *Node) IsNull() bool { return n == nil || n.Kind == Null || n.Kind == Missing }

// IsNumber reports any numeric kind.
func (n *Node) IsNumber() bool {
	return n != nil && (n.Kind == Int || n.Kind == Long || n.Kind == BigInt || n.Kind == Double)
}

// IsContainer reports an array or object.
func (n *Node) IsContainer() bool { return n != nil && (n.Kind == Array || n.Kind == Object) }

// AsText is JsonNode.asText(): a string's value, a number or boolean in
// Java's text form, "null" for null and "" for containers.
func (n *Node) AsText() string {
	if n == nil {
		return ""
	}
	switch n.Kind {
	case String:
		return n.S
	case Null:
		return "null"
	case Bool:
		return strconv.FormatBool(n.B)
	case Int, Long:
		return strconv.FormatInt(n.I, 10)
	case BigInt:
		return n.Big.String()
	case Double:
		return JavaDouble(n.F)
	}
	return ""
}

// JavaString is the Java toString() of the value Jackson converts the
// node to inside a Map<String,Object> (String, Integer, ..., ArrayList,
// LinkedHashMap): "[a, b]" for arrays, "{k=v}" for objects.
func (n *Node) JavaString() string {
	if n == nil {
		return "null"
	}
	switch n.Kind {
	case Array:
		parts := make([]string, len(n.Elems))
		for i, e := range n.Elems {
			parts[i] = e.JavaString()
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case Object:
		parts := make([]string, len(n.Keys))
		for i, k := range n.Keys {
			parts[i] = k + "=" + n.Vals[i].JavaString()
		}
		return "{" + strings.Join(parts, ", ") + "}"
	}
	return n.AsText()
}

// JavaClass is the Java class of the converted value (for
// ClassCastException messages).
func (n *Node) JavaClass() string {
	if n == nil {
		return "null"
	}
	switch n.Kind {
	case String:
		return "java.lang.String"
	case Bool:
		return "java.lang.Boolean"
	case Int:
		return "java.lang.Integer"
	case Long:
		return "java.lang.Long"
	case BigInt:
		return "java.math.BigInteger"
	case Double:
		return "java.lang.Double"
	case Array:
		return "java.util.ArrayList"
	case Object:
		return "java.util.LinkedHashMap"
	}
	return "null"
}

// NodeClass is the Jackson node class's simple name (IntNode, ...).
func (n *Node) NodeClass() string {
	switch n.Kind {
	case Null:
		return "NullNode"
	case Bool:
		return "BooleanNode"
	case Int:
		return "IntNode"
	case Long:
		return "LongNode"
	case BigInt:
		return "BigIntegerNode"
	case Double:
		return "DoubleNode"
	case String:
		return "TextNode"
	case Array:
		return "ArrayNode"
	case Object:
		return "ObjectNode"
	}
	return "MissingNode"
}

// TypeName is JsonNodeType's name (NULL, NUMBER, STRING, ...).
func (n *Node) TypeName() string {
	switch n.Kind {
	case Null:
		return "NULL"
	case Bool:
		return "BOOLEAN"
	case Int, Long, BigInt, Double:
		return "NUMBER"
	case String:
		return "STRING"
	case Array:
		return "ARRAY"
	case Object:
		return "OBJECT"
	}
	return "MISSING"
}

// AsInt is JsonNode.asInt(): numbers convert as Java casts do, text is
// parsed leniently (0 when it is not a number), booleans are 1 or 0,
// and anything else is 0.
func (n *Node) AsInt() int32 {
	if n == nil {
		return 0
	}
	switch n.Kind {
	case Int, Long:
		return int32(n.I)
	case BigInt:
		return int32(new(big.Int).And(n.Big, big.NewInt(0xffffffff)).Uint64())
	case Double:
		return javaDoubleToInt(n.F)
	case Bool:
		if n.B {
			return 1
		}
	case String:
		return parseAsInt(n.S)
	}
	return 0
}

// AsLong is JsonNode.asLong().
func (n *Node) AsLong() int64 {
	if n == nil {
		return 0
	}
	switch n.Kind {
	case Int, Long:
		return n.I
	case BigInt:
		return int64(new(big.Int).And(n.Big, new(big.Int).SetUint64(math.MaxUint64)).Uint64())
	case Double:
		return javaDoubleToLong(n.F)
	case Bool:
		if n.B {
			return 1
		}
	case String:
		s := strings.TrimSpace(n.S)
		if v, err := strconv.ParseInt(strings.TrimPrefix(s, "+"), 10, 64); err == nil {
			return v
		}
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			return javaDoubleToLong(f)
		}
	}
	return 0
}

func javaDoubleToInt(f float64) int32 {
	switch {
	case math.IsNaN(f):
		return 0
	case f >= math.MaxInt32:
		return math.MaxInt32
	case f <= math.MinInt32:
		return math.MinInt32
	}
	return int32(f)
}

func javaDoubleToLong(f float64) int64 {
	switch {
	case math.IsNaN(f):
		return 0
	case f >= math.MaxInt64:
		return math.MaxInt64
	case f <= math.MinInt64:
		return math.MinInt64
	}
	return int64(f)
}

// parseAsInt is Jackson's NumberInput.parseAsInt(s, 0).
func parseAsInt(s string) int32 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	t := strings.TrimPrefix(s, "+")
	digits := t
	if strings.HasPrefix(digits, "-") {
		digits = digits[1:]
	}
	for _, c := range digits {
		if c < '0' || c > '9' {
			f, err := strconv.ParseFloat(t, 64)
			if err != nil {
				return 0
			}
			return javaDoubleToInt(f)
		}
	}
	v, err := strconv.ParseInt(t, 10, 32)
	if err != nil {
		return 0
	}
	return int32(v)
}

// JavaDouble is Java's Double.toString.
func JavaDouble(f float64) string {
	switch {
	case math.IsNaN(f):
		return "NaN"
	case math.IsInf(f, 1):
		return "Infinity"
	case math.IsInf(f, -1):
		return "-Infinity"
	case f == 0:
		if math.Signbit(f) {
			return "-0.0"
		}
		return "0.0"
	}
	neg := f < 0
	a := math.Abs(f)
	// Shortest digits and decimal exponent: d.ddd × 10^exp.
	e := strconv.FormatFloat(a, 'e', -1, 64)
	mant, expStr, _ := strings.Cut(e, "e")
	exp, _ := strconv.Atoi(expStr)
	digits := strings.Replace(mant, ".", "", 1)
	var out string
	if a >= 1e-3 && a < 1e7 {
		point := exp + 1 // digits before the decimal point
		switch {
		case point <= 0:
			out = "0." + strings.Repeat("0", -point) + digits
		case point >= len(digits):
			out = digits + strings.Repeat("0", point-len(digits)) + ".0"
		default:
			out = digits[:point] + "." + digits[point:]
		}
	} else {
		frac := digits[1:]
		if frac == "" {
			frac = "0"
		}
		out = digits[:1] + "." + frac + "E" + strconv.Itoa(exp)
	}
	if neg {
		out = "-" + out
	}
	return out
}

// Encode writes the node as Jackson's compact JSON.
func (n *Node) Encode() string {
	var b strings.Builder
	n.encode(&b)
	return b.String()
}

func (n *Node) encode(b *strings.Builder) {
	if n == nil {
		b.WriteString("null")
		return
	}
	switch n.Kind {
	case Null, Missing:
		b.WriteString("null")
	case Bool, Int, Long, BigInt:
		b.WriteString(n.AsText())
	case Double:
		if math.IsNaN(n.F) || math.IsInf(n.F, 0) {
			b.WriteString(Quote(JavaDouble(n.F)))
		} else {
			b.WriteString(JavaDouble(n.F))
		}
	case String:
		b.WriteString(Quote(n.S))
	case Array:
		b.WriteByte('[')
		for i, e := range n.Elems {
			if i > 0 {
				b.WriteByte(',')
			}
			e.encode(b)
		}
		b.WriteByte(']')
	case Object:
		b.WriteByte('{')
		for i, k := range n.Keys {
			if i > 0 {
				b.WriteByte(',')
			}
			b.WriteString(Quote(k))
			b.WriteByte(':')
			n.Vals[i].encode(b)
		}
		b.WriteByte('}')
	}
}

// Quote is Jackson's string encoding: only '"', '\\' and control
// characters below 0x20 are escaped (short forms where JSON has them,
// else \u00XX with upper-case hex).
func Quote(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 2)
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\b':
			b.WriteString(`\b`)
		case '\f':
			b.WriteString(`\f`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if r < 0x20 {
				const hex = "0123456789ABCDEF"
				b.WriteString(`\u00`)
				b.WriteByte(hex[r>>4])
				b.WriteByte(hex[r&0xf])
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}
