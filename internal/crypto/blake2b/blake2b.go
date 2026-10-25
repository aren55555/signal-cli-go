// Package blake2b implements BLAKE2b (RFC 7693) with variable output
// length and optional key, as Argon2 (RFC 9106) needs it.
package blake2b

import (
	"encoding/binary"
	"errors"
	"math/bits"
)

const (
	BlockSize = 128
	Size      = 64 // largest output
)

var iv = [8]uint64{
	0x6a09e667f3bcc908, 0xbb67ae8584caa73b, 0x3c6ef372fe94f82b, 0xa54ff53a5f1d36f1,
	0x510e527fade682d1, 0x9b05688c2b3e6c1f, 0x1f83d9abfb41bd6b, 0x5be0cd19137e2179,
}

// sigma is the message schedule (RFC 7693 §2.7); rounds 10 and 11 reuse
// rows 0 and 1.
var sigma = [10][16]byte{
	{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15},
	{14, 10, 4, 8, 9, 15, 13, 6, 1, 12, 0, 2, 11, 7, 5, 3},
	{11, 8, 12, 0, 5, 2, 15, 13, 10, 14, 3, 6, 7, 1, 9, 4},
	{7, 9, 3, 1, 13, 12, 11, 14, 2, 6, 5, 10, 4, 0, 15, 8},
	{9, 0, 5, 7, 2, 4, 10, 15, 14, 1, 11, 12, 6, 8, 3, 13},
	{2, 12, 6, 10, 0, 11, 8, 3, 4, 13, 7, 5, 15, 14, 1, 9},
	{12, 5, 1, 15, 14, 13, 4, 10, 0, 7, 6, 3, 9, 2, 8, 11},
	{13, 11, 7, 14, 12, 1, 3, 9, 5, 0, 15, 4, 8, 6, 2, 10},
	{6, 15, 14, 9, 11, 3, 0, 8, 12, 2, 13, 7, 1, 4, 10, 5},
	{10, 2, 8, 4, 7, 6, 1, 5, 15, 11, 9, 14, 3, 12, 13, 0},
}

// Digest is an incremental BLAKE2b hash.
type Digest struct {
	h      [8]uint64
	t      [2]uint64 // byte counter
	buf    [BlockSize]byte
	n      int
	size   int
	keyed  bool
	keyLen int
	keyBuf [BlockSize]byte
}

// New returns a BLAKE2b digest with an output of size bytes (1..64) and
// an optional key of at most 64 bytes.
func New(size int, key []byte) (*Digest, error) {
	if size < 1 || size > Size {
		return nil, errors.New("blake2b: bad output size")
	}
	if len(key) > Size {
		return nil, errors.New("blake2b: key too long")
	}
	d := &Digest{size: size}
	if len(key) > 0 {
		d.keyed, d.keyLen = true, len(key)
		copy(d.keyBuf[:], key)
	}
	d.init(len(key))
	return d, nil
}

func (d *Digest) init(keyLen int) {
	d.h = iv
	d.h[0] ^= 0x01010000 ^ uint64(keyLen)<<8 ^ uint64(d.size)
	d.t = [2]uint64{}
	d.n = 0
	if d.keyed {
		d.buf = d.keyBuf
		d.n = BlockSize
	}
}

func (d *Digest) compress(block []byte, last bool) {
	var m [16]uint64
	for i := range m {
		m[i] = binary.LittleEndian.Uint64(block[8*i:])
	}
	var v [16]uint64
	copy(v[:8], d.h[:])
	copy(v[8:], iv[:])
	v[12] ^= d.t[0]
	v[13] ^= d.t[1]
	if last {
		v[14] = ^v[14]
	}
	g := func(a, b, c, e int, x, y uint64) {
		v[a] += v[b] + x
		v[e] = bits.RotateLeft64(v[e]^v[a], -32)
		v[c] += v[e]
		v[b] = bits.RotateLeft64(v[b]^v[c], -24)
		v[a] += v[b] + y
		v[e] = bits.RotateLeft64(v[e]^v[a], -16)
		v[c] += v[e]
		v[b] = bits.RotateLeft64(v[b]^v[c], -63)
	}
	for r := range 12 {
		s := &sigma[r%10]
		g(0, 4, 8, 12, m[s[0]], m[s[1]])
		g(1, 5, 9, 13, m[s[2]], m[s[3]])
		g(2, 6, 10, 14, m[s[4]], m[s[5]])
		g(3, 7, 11, 15, m[s[6]], m[s[7]])
		g(0, 5, 10, 15, m[s[8]], m[s[9]])
		g(1, 6, 11, 12, m[s[10]], m[s[11]])
		g(2, 7, 8, 13, m[s[12]], m[s[13]])
		g(3, 4, 9, 14, m[s[14]], m[s[15]])
	}
	for i := range 8 {
		d.h[i] ^= v[i] ^ v[i+8]
	}
}

func (d *Digest) addCount(n uint64) {
	var c uint64
	d.t[0], c = bits.Add64(d.t[0], n, 0)
	d.t[1] += c
}

// Write absorbs p. A full buffer is compressed only once more input
// arrives, since the last block is compressed differently.
func (d *Digest) Write(p []byte) (int, error) {
	n := len(p)
	for len(p) > 0 {
		if d.n == BlockSize {
			d.addCount(BlockSize)
			d.compress(d.buf[:], false)
			d.n = 0
		}
		k := copy(d.buf[d.n:], p)
		d.n += k
		p = p[k:]
	}
	return n, nil
}

// Sum appends the digest to b without changing the state.
func (d *Digest) Sum(b []byte) []byte {
	c := *d
	c.addCount(uint64(c.n))
	clear(c.buf[c.n:])
	c.compress(c.buf[:], true)
	var out [Size]byte
	for i := range 8 {
		binary.LittleEndian.PutUint64(out[8*i:], c.h[i])
	}
	return append(b, out[:c.size]...)
}

// Reset restores the initial state (keeping size and key).
func (d *Digest) Reset() { d.init(d.keyLen) }

// Size returns the output length.
func (d *Digest) Size() int { return d.size }

// BlockSize returns 128.
func (d *Digest) BlockSize() int { return BlockSize }

// Sum is a one-shot unkeyed hash of size bytes.
func Sum(size int, data []byte) []byte {
	d, err := New(size, nil)
	if err != nil {
		panic(err)
	}
	d.Write(data)
	return d.Sum(nil)
}
