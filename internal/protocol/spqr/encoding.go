// SPDX-License-Identifier: GPL-3.0-or-later

// Package spqr implements SPQR, the Sparse Post-Quantum Ratchet, as
// libsignal v0.99.1 uses it (spqr 1.5.3, protocol V1), from the specs in
// spec/protocol/spqr/ and Signal's published Double Ratchet document.
package spqr

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// GF(2^16) with the reduction polynomial x^16 + x^12 + x^3 + x + 1
// (chunking.md §1). Chunk data is public (KEM keys and ciphertexts), so
// none of this needs to be constant-time.
const gfPoly = 0x1100b

func gfMul(a, b uint16) uint16 {
	var p uint32
	x := uint32(a)
	for b != 0 {
		if b&1 == 1 {
			p ^= x
		}
		b >>= 1
		x <<= 1
	}
	for i := 31; i >= 16; i-- {
		if p>>i&1 == 1 {
			p ^= gfPoly << (i - 16)
		}
	}
	return uint16(p)
}

// gfInv returns a^(2^16-2), the inverse of a nonzero a.
func gfInv(a uint16) uint16 {
	r, base := uint16(1), a
	for e := uint32(1<<16 - 2); e > 0; e >>= 1 {
		if e&1 == 1 {
			r = gfMul(r, base)
		}
		base = gfMul(base, base)
	}
	return r
}

type point struct{ x, y uint16 }

// interpolate evaluates at x the polynomial of degree < len(pts) through
// pts (Lagrange form). Subtraction in GF(2^16) is XOR.
func interpolate(pts []point, x uint16) uint16 {
	var sum uint16
	for i, pi := range pts {
		num, den := uint16(1), uint16(1)
		for m, pm := range pts {
			if m == i {
				continue
			}
			num = gfMul(num, x^pm.x)
			den = gfMul(den, pi.x^pm.x)
		}
		sum ^= gfMul(pi.y, gfMul(num, gfInv(den)))
	}
	return sum
}

const (
	chunkSize = 32
	polys     = 16
)

// pointCounts returns n_j for each of the 16 polynomials of a string with
// w big-endian 16-bit words: word k belongs to polynomial k mod 16 at
// x = k / 16.
func pointCounts(w int) (n [polys]int) {
	for j := range n {
		n[j] = w / polys
		if j < w%polys {
			n[j]++
		}
	}
	return n
}

func checkStreamLen(l int) error {
	if l%2 != 0 || l > polys*2*65536 {
		return fmt.Errorf("spqr: cannot chunk a %d-byte string", l)
	}
	return nil
}

// encoder produces the chunks of one string: chunk c holds P_0(c) ...
// P_15(c), big-endian, so chunks below n are the string itself.
type encoder struct {
	pts [polys][]point
	// coeffs, when set, holds each polynomial's coefficients (lowest
	// degree first) instead of points: the form libsignal stores once an
	// encoder has produced a parity chunk (state.md §2).
	coeffs *[polys][]uint16
}

func newEncoder(data []byte) (*encoder, error) {
	if err := checkStreamLen(len(data)); err != nil {
		return nil, err
	}
	e := new(encoder)
	for k := 0; k < len(data)/2; k++ {
		j := k % polys
		e.pts[j] = append(e.pts[j], point{uint16(k / polys), binary.BigEndian.Uint16(data[2*k:])})
	}
	return e, nil
}

func (e *encoder) chunk(c uint16) (out [chunkSize]byte) {
	if e.coeffs != nil {
		for j, cs := range e.coeffs {
			var y uint16 // Horner's rule
			for k := len(cs) - 1; k >= 0; k-- {
				y = gfMul(y, c) ^ cs[k]
			}
			binary.BigEndian.PutUint16(out[2*j:], y)
		}
		return out
	}
	for j, pts := range e.pts {
		var y uint16
		switch {
		case int(c) < len(pts):
			y = pts[c].y
		case len(pts) > 0:
			y = interpolate(pts, c)
		}
		binary.BigEndian.PutUint16(out[2*j:], y)
	}
	return out
}

// decoder collects chunks of a string of known length until every
// polynomial has enough points (chunking.md §3).
type decoder struct {
	length int
	need   [polys]int
	pts    [polys][]point
}

func newDecoder(length int) (*decoder, error) {
	if err := checkStreamLen(length); err != nil {
		return nil, err
	}
	return &decoder{length: length, need: pointCounts(length / 2)}, nil
}

func (d *decoder) add(c uint16, data []byte) {
	for j := range polys {
		y := binary.BigEndian.Uint16(data[2*j:])
		if !(int(c) < d.need[j] || len(d.pts[j]) < d.need[j]) {
			continue
		}
		dup := false
		for _, p := range d.pts[j] {
			if p.x == c {
				dup = true // the first point for an x wins
				break
			}
		}
		if !dup {
			d.pts[j] = append(d.pts[j], point{c, y})
		}
	}
}

func (d *decoder) complete() bool {
	for j := range polys {
		if len(d.pts[j]) < d.need[j] {
			return false
		}
	}
	return true
}

// bytes rebuilds the string; call it only when complete. Missing words are
// interpolated through the n_j stored points with the smallest x.
func (d *decoder) bytes() []byte {
	out := make([]byte, d.length)
	var basis [polys][]point
	for j := range polys {
		pts := append([]point(nil), d.pts[j]...)
		for a := 1; a < len(pts); a++ { // insertion sort by x
			for b := a; b > 0 && pts[b].x < pts[b-1].x; b-- {
				pts[b], pts[b-1] = pts[b-1], pts[b]
			}
		}
		basis[j] = pts[:d.need[j]]
	}
	for k := 0; k < d.length/2; k++ {
		j, x := k%polys, uint16(k/polys)
		y, found := uint16(0), false
		for _, p := range d.pts[j] {
			if p.x == x {
				y, found = p.y, true
				break
			}
		}
		if !found {
			y = interpolate(basis[j], x)
		}
		binary.BigEndian.PutUint16(out[2*k:], y)
	}
	return out
}

// The V1 message format (wire.md §3).

var errMsgDecode = errors.New("spqr: malformed message")

func appendVarint(b []byte, v uint64) []byte {
	for v >= 0x80 {
		b = append(b, byte(v)|0x80)
		v >>= 7
	}
	return append(b, byte(v))
}

// consumeVarint reads a varint of at most 10 bytes. Non-minimal forms are
// accepted (wire.md §3). The spec doesn't say what happens to bits beyond
// 64 in a tenth byte; an honest sender never writes them, and this
// rejects them.
func consumeVarint(b []byte) (uint64, int, error) {
	var v uint64
	for i := 0; i < 10 && i < len(b); i++ {
		if i == 9 && b[i] > 1 {
			return 0, 0, errMsgDecode
		}
		v |= uint64(b[i]&0x7f) << (7 * i)
		if b[i] < 0x80 {
			return v, i + 1, nil
		}
	}
	return 0, 0, errMsgDecode
}

type payloadType byte

const (
	payloadNone payloadType = iota
	payloadHdr
	payloadEk
	payloadEkCt1Ack
	payloadCt1Ack
	payloadCt1
	payloadCt2
)

func (p payloadType) hasChunk() bool { return p != payloadNone && p != payloadCt1Ack }

const versionV1 = 0x01

type message struct {
	epoch      uint64
	keyIndex   uint32
	payload    payloadType
	chunkIndex uint16
	chunk      []byte // 32 bytes for chunk-carrying types
}

func (m message) encode() []byte {
	b := []byte{versionV1}
	b = appendVarint(b, m.epoch)
	b = appendVarint(b, uint64(m.keyIndex))
	b = append(b, byte(m.payload))
	if m.payload.hasChunk() {
		b = appendVarint(b, uint64(m.chunkIndex))
		b = append(b, m.chunk...)
	}
	return b
}

// parseMessage parses a V1 message (the caller has seen the 0x01 version
// byte). Trailing bytes are ignored.
func parseMessage(b []byte) (*message, error) {
	if len(b) == 0 || b[0] != versionV1 {
		return nil, errMsgDecode
	}
	b = b[1:]
	m := new(message)
	epoch, n, err := consumeVarint(b)
	if err != nil || epoch == 0 {
		return nil, errMsgDecode
	}
	m.epoch, b = epoch, b[n:]
	idx, n, err := consumeVarint(b)
	if err != nil || idx > 1<<32-1 {
		return nil, errMsgDecode
	}
	m.keyIndex, b = uint32(idx), b[n:]
	if len(b) == 0 || b[0] > byte(payloadCt2) {
		return nil, errMsgDecode
	}
	m.payload, b = payloadType(b[0]), b[1:]
	if m.payload.hasChunk() {
		ci, n, err := consumeVarint(b)
		if err != nil || ci > 0xffff || len(b)-n < chunkSize {
			return nil, errMsgDecode
		}
		m.chunkIndex = uint16(ci)
		m.chunk = append([]byte(nil), b[n:n+chunkSize]...)
	}
	return m, nil
}

// toCoefficients returns the coefficients (lowest degree first) of the
// polynomial through pts, via Newton's divided differences.
func toCoefficients(pts []point) []uint16 {
	n := len(pts)
	dd := make([]uint16, n)
	for i, p := range pts {
		dd[i] = p.y
	}
	for k := 1; k < n; k++ {
		for i := n - 1; i >= k; i-- {
			dd[i] = gfMul(dd[i]^dd[i-1], gfInv(pts[i].x^pts[i-k].x))
		}
	}
	// Horner in Newton form: poly = dd[n-1]; poly = poly*(x - x_k) + dd[k].
	poly := make([]uint16, n)
	if n == 0 {
		return poly
	}
	poly[0] = dd[n-1]
	deg := 0
	for k := n - 2; k >= 0; k-- {
		// multiply by (x + x_k)
		for d := deg + 1; d >= 1; d-- {
			poly[d] = poly[d-1] ^ gfMul(poly[d], pts[k].x)
		}
		poly[0] = gfMul(poly[0], pts[k].x)
		deg++
		poly[0] ^= dd[k]
	}
	return poly
}
