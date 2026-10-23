// Package h2 is a minimal HTTP/2 client (RFC 9113) with HPACK (RFC 7541),
// enough to open the WebSocket-over-HTTP/2 stream (RFC 8441) that
// libsignal-net uses for Signal's chat service. Go's standard library
// implements extended CONNECT internally but exposes no client API that
// can send the :protocol pseudo-header, so this lives in-repo.
package h2

import (
	"errors"
	"fmt"
)

// ErrHPACK reports a malformed header block (a COMPRESSION_ERROR).
var ErrHPACK = errors.New("h2: HPACK decoding error")

func hpackErr(format string, args ...any) error {
	return fmt.Errorf("%w: "+format, append([]any{ErrHPACK}, args...)...)
}

// appendInt appends v with an N-bit prefix (RFC 7541 §5.1); flags holds
// the bits above the prefix in the first byte.
func appendInt(dst []byte, prefix int, flags byte, v uint64) []byte {
	max := uint64(1)<<prefix - 1
	if v < max {
		return append(dst, flags|byte(v))
	}
	dst = append(dst, flags|byte(max))
	v -= max
	for v >= 128 {
		dst = append(dst, byte(v%128)|0x80)
		v /= 128
	}
	return append(dst, byte(v))
}

// readInt reads an N-bit-prefix integer from the start of b.
func readInt(b []byte, prefix int) (uint64, int, error) {
	if len(b) == 0 {
		return 0, 0, hpackErr("missing integer")
	}
	max := uint64(1)<<prefix - 1
	v := uint64(b[0]) & max
	if v < max {
		return v, 1, nil
	}
	for i, shift := 1, 0; i < len(b); i, shift = i+1, shift+7 {
		if shift > 56 {
			return 0, 0, hpackErr("integer too large")
		}
		v += uint64(b[i]&0x7f) << shift
		if b[i]&0x80 == 0 {
			return v, i + 1, nil
		}
	}
	return 0, 0, hpackErr("truncated integer")
}

// Huffman coding with the RFC 7541 Appendix B code (hpack_tables.go).

func huffmanEncode(dst, s []byte) []byte {
	var acc uint64
	bits := 0
	for _, c := range s {
		acc = acc<<huffmanLengths[c] | uint64(huffmanCodes[c])
		bits += int(huffmanLengths[c])
		for bits >= 8 {
			bits -= 8
			dst = append(dst, byte(acc>>bits))
		}
	}
	if bits > 0 { // pad with the high bits of EOS (all ones)
		dst = append(dst, byte(acc<<(8-bits))|byte(0xff>>bits))
	}
	return dst
}

func huffmanLen(s []byte) int {
	n := 0
	for _, c := range s {
		n += int(huffmanLengths[c])
	}
	return (n + 7) / 8
}

// huffmanTree is a binary decoding tree: node i has children
// [2*i], [2*i+1] in children; a negative child -1-sym is a leaf.
var huffmanTree [][2]int

func init() {
	huffmanTree = [][2]int{{0, 0}}
	for sym := range 257 {
		code, n := huffmanCodes[sym], int(huffmanLengths[sym])
		node := 0
		for i := n - 1; i >= 0; i-- {
			bit := code >> i & 1
			if i == 0 {
				huffmanTree[node][bit] = -1 - sym
				break
			}
			if huffmanTree[node][bit] == 0 {
				huffmanTree = append(huffmanTree, [2]int{0, 0})
				huffmanTree[node][bit] = len(huffmanTree) - 1
			}
			node = huffmanTree[node][bit]
		}
	}
}

func huffmanDecode(b []byte) ([]byte, error) {
	var out []byte
	node, depth, allOnes := 0, 0, true
	for _, c := range b {
		for i := 7; i >= 0; i-- {
			bit := int(c >> i & 1)
			next := huffmanTree[node][bit]
			switch {
			case next == 0:
				return nil, hpackErr("invalid Huffman code")
			case next < 0:
				sym := -1 - next
				if sym == 256 {
					return nil, hpackErr("EOS in a Huffman string")
				}
				out = append(out, byte(sym))
				node, depth, allOnes = 0, 0, true
			default:
				node, depth = next, depth+1
				allOnes = allOnes && bit == 1
			}
		}
	}
	// Leftover bits are padding: fewer than 8, and all ones (a prefix of EOS).
	if depth > 7 || !allOnes {
		return nil, hpackErr("invalid Huffman padding")
	}
	return out, nil
}

// HeaderField is one header. Sensitive fields are sent as never-indexed
// literals and never enter a dynamic table.
type HeaderField struct {
	Name, Value string
	Sensitive   bool
}

func entrySize(f HeaderField) uint32 { return uint32(len(f.Name) + len(f.Value) + 32) }

// dynamicTable holds entries newest first (RFC 7541 §2.3.2).
type dynamicTable struct {
	entries []HeaderField
	size    uint32
	maxSize uint32
}

func (t *dynamicTable) len() int             { return len(t.entries) }
func (t *dynamicTable) at(i int) HeaderField { return t.entries[i] }

func (t *dynamicTable) evictTo(limit uint32) {
	for t.size > limit && len(t.entries) > 0 {
		last := t.entries[len(t.entries)-1]
		t.size -= entrySize(last)
		t.entries = t.entries[:len(t.entries)-1]
	}
}

// add inserts f, evicting old entries; an entry larger than the table
// empties it (§4.4).
func (t *dynamicTable) add(f HeaderField) {
	n := entrySize(f)
	if n > t.maxSize {
		t.evictTo(0)
		return
	}
	t.evictTo(t.maxSize - n)
	t.entries = append([]HeaderField{f}, t.entries...)
	t.size += n
}

func (t *dynamicTable) setMaxSize(n uint32) {
	t.maxSize = n
	t.evictTo(n)
}

// lookup returns the field at a 1-based HPACK index (static then dynamic).
func (t *dynamicTable) lookup(i uint64) (HeaderField, error) {
	switch {
	case i == 0:
		return HeaderField{}, hpackErr("index 0")
	case i <= uint64(len(staticTable)):
		e := staticTable[i-1]
		return HeaderField{Name: e[0], Value: e[1]}, nil
	case i-uint64(len(staticTable)) <= uint64(len(t.entries)):
		return t.entries[i-uint64(len(staticTable))-1], nil
	}
	return HeaderField{}, hpackErr("index %d past the tables", i)
}

// maxHeaderListBytes bounds one decoded header block.
const maxHeaderListBytes = 64 << 10

// Decoder decodes header blocks from the peer.
type Decoder struct {
	table dynamicTable
	limit uint32 // the SETTINGS_HEADER_TABLE_SIZE we advertised
}

// NewDecoder returns a decoder whose table may grow to maxTableSize.
func NewDecoder(maxTableSize uint32) *Decoder {
	return &Decoder{table: dynamicTable{maxSize: maxTableSize}, limit: maxTableSize}
}

func readString(b []byte) (string, int, error) {
	if len(b) == 0 {
		return "", 0, hpackErr("missing string")
	}
	huff := b[0]&0x80 != 0
	n, m, err := readInt(b, 7)
	if err != nil {
		return "", 0, err
	}
	if n > uint64(len(b)-m) {
		return "", 0, hpackErr("string of %d bytes with %d left", n, len(b)-m)
	}
	raw := b[m : m+int(n)]
	if !huff {
		return string(raw), m + int(n), nil
	}
	s, err := huffmanDecode(raw)
	if err != nil {
		return "", 0, err
	}
	return string(s), m + int(n), nil
}

// Decode decodes one complete header block.
func (d *Decoder) Decode(b []byte) ([]HeaderField, error) {
	var out []HeaderField
	total := 0
	for len(b) > 0 {
		c := b[0]
		var f HeaderField
		switch {
		case c&0x80 != 0: // indexed
			i, n, err := readInt(b, 7)
			if err != nil {
				return nil, err
			}
			if f, err = d.table.lookup(i); err != nil {
				return nil, err
			}
			b = b[n:]
		case c&0xe0 == 0x20: // dynamic table size update
			if len(out) > 0 {
				return nil, hpackErr("table size update after a header field")
			}
			v, n, err := readInt(b, 5)
			if err != nil {
				return nil, err
			}
			if v > uint64(d.limit) {
				return nil, hpackErr("table size %d over the limit %d", v, d.limit)
			}
			d.table.setMaxSize(uint32(v))
			b = b[n:]
			continue
		default: // literals: 01 incremental, 0000 without, 0001 never indexed
			prefix, index := 4, false
			if c&0xc0 == 0x40 {
				prefix, index = 6, true
			}
			f.Sensitive = c&0xf0 == 0x10
			ni, n, err := readInt(b, prefix)
			if err != nil {
				return nil, err
			}
			b = b[n:]
			if ni > 0 {
				nf, err := d.table.lookup(ni)
				if err != nil {
					return nil, err
				}
				f.Name = nf.Name
			} else {
				if f.Name, n, err = readString(b); err != nil {
					return nil, err
				}
				b = b[n:]
			}
			if f.Value, n, err = readString(b); err != nil {
				return nil, err
			}
			b = b[n:]
			if index {
				d.table.add(HeaderField{Name: f.Name, Value: f.Value})
			}
		}
		total += int(entrySize(f))
		if total > maxHeaderListBytes {
			return nil, hpackErr("header list over %d bytes", maxHeaderListBytes)
		}
		out = append(out, f)
	}
	return out, nil
}

// Encoder encodes header blocks for the peer. It indexes exact matches,
// otherwise sends a literal with incremental indexing (reusing an indexed
// name, static table first), and never indexes sensitive fields: the
// strategy of RFC 7541's examples.
type Encoder struct {
	table   dynamicTable
	huffman bool
}

// NewEncoder returns an encoder for a peer table of tableSize bytes.
func NewEncoder(tableSize uint32, huffman bool) *Encoder {
	return &Encoder{table: dynamicTable{maxSize: tableSize}, huffman: huffman}
}

func (e *Encoder) appendString(dst []byte, s string) []byte {
	if e.huffman && huffmanLen([]byte(s)) < len(s)+1 {
		dst = appendInt(dst, 7, 0x80, uint64(huffmanLen([]byte(s))))
		return huffmanEncode(dst, []byte(s))
	}
	dst = appendInt(dst, 7, 0, uint64(len(s)))
	return append(dst, s...)
}

// find returns the index of an exact match and of a name match (0 if none),
// static table first.
func (e *Encoder) find(f HeaderField) (exact, name uint64) {
	for i, s := range staticTable {
		if s[0] == f.Name {
			if s[1] == f.Value && exact == 0 {
				exact = uint64(i + 1)
			}
			if name == 0 {
				name = uint64(i + 1)
			}
		}
	}
	for i, d := range e.table.entries {
		if d.Name == f.Name {
			idx := uint64(len(staticTable) + i + 1)
			if d.Value == f.Value && exact == 0 {
				exact = idx
			}
			if name == 0 {
				name = idx
			}
		}
	}
	return exact, name
}

// Encode appends the header block for fields to dst.
func (e *Encoder) Encode(dst []byte, fields []HeaderField) []byte {
	for _, f := range fields {
		exact, name := e.find(f)
		switch {
		case f.Sensitive:
			dst = appendInt(dst, 4, 0x10, name)
		case exact != 0:
			dst = appendInt(dst, 7, 0x80, exact)
			continue
		default:
			dst = appendInt(dst, 6, 0x40, name)
			e.table.add(HeaderField{Name: f.Name, Value: f.Value})
		}
		if name == 0 {
			dst = e.appendString(dst, f.Name)
		}
		dst = e.appendString(dst, f.Value)
	}
	return dst
}
