// Package chacha20poly1305 implements the ChaCha20 stream cipher, the
// Poly1305 one-time authenticator and their AEAD combination, as defined
// in RFC 8439. The enclave channels (Noise) use the AEAD.
package chacha20poly1305

import (
	"crypto/cipher"
	"crypto/subtle"
	"encoding/binary"
	"errors"
	"math/bits"
)

const (
	KeySize   = 32
	NonceSize = 12
	Overhead  = 16
)

var errOpen = errors.New("chacha20poly1305: message authentication failed")

// The four constant words, "expand 32-byte k" (RFC 8439 §2.3).
const c0, c1, c2, c3 = 0x61707865, 0x3320646e, 0x79622d32, 0x6b206574

func quarter(a, b, c, d uint32) (uint32, uint32, uint32, uint32) {
	a += b
	d = bits.RotateLeft32(d^a, 16)
	c += d
	b = bits.RotateLeft32(b^c, 12)
	a += b
	d = bits.RotateLeft32(d^a, 8)
	c += d
	b = bits.RotateLeft32(b^c, 7)
	return a, b, c, d
}

// Block writes the 64-byte ChaCha20 block for key, counter and nonce
// (RFC 8439 §2.3).
func Block(out *[64]byte, key *[KeySize]byte, counter uint32, nonce *[NonceSize]byte) {
	var in [16]uint32
	in[0], in[1], in[2], in[3] = c0, c1, c2, c3
	for i := range 8 {
		in[4+i] = binary.LittleEndian.Uint32(key[4*i:])
	}
	in[12] = counter
	for i := range 3 {
		in[13+i] = binary.LittleEndian.Uint32(nonce[4*i:])
	}
	x := in
	for range 10 {
		x[0], x[4], x[8], x[12] = quarter(x[0], x[4], x[8], x[12])
		x[1], x[5], x[9], x[13] = quarter(x[1], x[5], x[9], x[13])
		x[2], x[6], x[10], x[14] = quarter(x[2], x[6], x[10], x[14])
		x[3], x[7], x[11], x[15] = quarter(x[3], x[7], x[11], x[15])
		x[0], x[5], x[10], x[15] = quarter(x[0], x[5], x[10], x[15])
		x[1], x[6], x[11], x[12] = quarter(x[1], x[6], x[11], x[12])
		x[2], x[7], x[8], x[13] = quarter(x[2], x[7], x[8], x[13])
		x[3], x[4], x[9], x[14] = quarter(x[3], x[4], x[9], x[14])
	}
	for i := range 16 {
		binary.LittleEndian.PutUint32(out[4*i:], x[i]+in[i])
	}
}

// XORKeyStream encrypts or decrypts src into dst (which may alias it
// exactly) starting at block counter (RFC 8439 §2.4). It panics if the
// counter would wrap.
func XORKeyStream(dst, src []byte, key *[KeySize]byte, counter uint32, nonce *[NonceSize]byte) {
	if len(dst) < len(src) {
		panic("chacha20poly1305: output smaller than input")
	}
	if uint64(len(src)) > (1<<32-uint64(counter))*64 {
		panic("chacha20poly1305: counter overflow")
	}
	var ks [64]byte
	for len(src) > 0 {
		Block(&ks, key, counter, nonce)
		counter++
		n := subtle.XORBytes(dst, src, ks[:])
		dst, src = dst[n:], src[n:]
	}
}

// poly1305 is the authenticator state: 26-bit limbs for the accumulator h
// and the clamped r, and s (RFC 8439 §2.5).
type poly1305 struct {
	r, h [5]uint32
	s    [4]uint32
	buf  [16]byte
	n    int
}

func newPoly1305(key *[32]byte) *poly1305 {
	p := &poly1305{}
	p.r[0] = binary.LittleEndian.Uint32(key[0:]) & 0x3ffffff
	p.r[1] = (binary.LittleEndian.Uint32(key[3:]) >> 2) & 0x3ffff03
	p.r[2] = (binary.LittleEndian.Uint32(key[6:]) >> 4) & 0x3ffc0ff
	p.r[3] = (binary.LittleEndian.Uint32(key[9:]) >> 6) & 0x3f03fff
	p.r[4] = (binary.LittleEndian.Uint32(key[12:]) >> 8) & 0x00fffff
	for i := range 4 {
		p.s[i] = binary.LittleEndian.Uint32(key[16+4*i:])
	}
	return p
}

// block adds one 16-byte block with the given high bit (1<<24 for a full
// block, 0 for the padded final one) and multiplies by r mod 2^130-5.
func (p *poly1305) block(m []byte, hibit uint32) {
	r0, r1, r2, r3, r4 := uint64(p.r[0]), uint64(p.r[1]), uint64(p.r[2]), uint64(p.r[3]), uint64(p.r[4])
	s1, s2, s3, s4 := r1*5, r2*5, r3*5, r4*5
	h0 := uint64(p.h[0] + binary.LittleEndian.Uint32(m[0:])&0x3ffffff)
	h1 := uint64(p.h[1] + (binary.LittleEndian.Uint32(m[3:])>>2)&0x3ffffff)
	h2 := uint64(p.h[2] + (binary.LittleEndian.Uint32(m[6:])>>4)&0x3ffffff)
	h3 := uint64(p.h[3] + (binary.LittleEndian.Uint32(m[9:])>>6)&0x3ffffff)
	h4 := uint64(p.h[4] + (binary.LittleEndian.Uint32(m[12:])>>8 | hibit))

	d0 := h0*r0 + h1*s4 + h2*s3 + h3*s2 + h4*s1
	d1 := h0*r1 + h1*r0 + h2*s4 + h3*s3 + h4*s2
	d2 := h0*r2 + h1*r1 + h2*r0 + h3*s4 + h4*s3
	d3 := h0*r3 + h1*r2 + h2*r1 + h3*r0 + h4*s4
	d4 := h0*r4 + h1*r3 + h2*r2 + h3*r1 + h4*r0

	c := d0 >> 26
	h0 = d0 & 0x3ffffff
	d1 += c
	c = d1 >> 26
	h1 = d1 & 0x3ffffff
	d2 += c
	c = d2 >> 26
	h2 = d2 & 0x3ffffff
	d3 += c
	c = d3 >> 26
	h3 = d3 & 0x3ffffff
	d4 += c
	c = d4 >> 26
	h4 = d4 & 0x3ffffff
	h0 += c * 5
	c = h0 >> 26
	h0 &= 0x3ffffff
	h1 += c
	p.h = [5]uint32{uint32(h0), uint32(h1), uint32(h2), uint32(h3), uint32(h4)}
}

func (p *poly1305) write(m []byte) {
	if p.n > 0 {
		k := copy(p.buf[p.n:], m)
		p.n += k
		m = m[k:]
		if p.n < 16 {
			return
		}
		p.block(p.buf[:], 1<<24)
		p.n = 0
	}
	for len(m) >= 16 {
		p.block(m[:16], 1<<24)
		m = m[16:]
	}
	p.n = copy(p.buf[:], m)
}

func (p *poly1305) sum(out *[16]byte) {
	if p.n > 0 {
		p.buf[p.n] = 1
		clear(p.buf[p.n+1:])
		p.block(p.buf[:], 0)
	}
	h0, h1, h2, h3, h4 := p.h[0], p.h[1], p.h[2], p.h[3], p.h[4]
	// Full carry.
	c := h1 >> 26
	h1 &= 0x3ffffff
	h2 += c
	c = h2 >> 26
	h2 &= 0x3ffffff
	h3 += c
	c = h3 >> 26
	h3 &= 0x3ffffff
	h4 += c
	c = h4 >> 26
	h4 &= 0x3ffffff
	h0 += c * 5
	c = h0 >> 26
	h0 &= 0x3ffffff
	h1 += c
	// g = h + 5 - 2^130; select g when it does not go negative.
	g0 := h0 + 5
	c = g0 >> 26
	g0 &= 0x3ffffff
	g1 := h1 + c
	c = g1 >> 26
	g1 &= 0x3ffffff
	g2 := h2 + c
	c = g2 >> 26
	g2 &= 0x3ffffff
	g3 := h3 + c
	c = g3 >> 26
	g3 &= 0x3ffffff
	g4 := h4 + c - (1 << 26)
	mask := (g4 >> 31) - 1 // all ones if g4 did not underflow
	g0, g1, g2, g3, g4 = g0&mask, g1&mask, g2&mask, g3&mask, g4&mask
	mask = ^mask
	h0, h1, h2, h3, h4 = h0&mask|g0, h1&mask|g1, h2&mask|g2, h3&mask|g3, h4&mask|g4
	// h mod 2^128, then + s.
	w0 := uint64(h0 | h1<<26)
	w1 := uint64(h1>>6 | h2<<20)
	w2 := uint64(h2>>12 | h3<<14)
	w3 := uint64(h3>>18 | h4<<8)
	f := w0&0xffffffff + uint64(p.s[0])
	binary.LittleEndian.PutUint32(out[0:], uint32(f))
	f = w1&0xffffffff + uint64(p.s[1]) + f>>32
	binary.LittleEndian.PutUint32(out[4:], uint32(f))
	f = w2&0xffffffff + uint64(p.s[2]) + f>>32
	binary.LittleEndian.PutUint32(out[8:], uint32(f))
	f = w3&0xffffffff + uint64(p.s[3]) + f>>32
	binary.LittleEndian.PutUint32(out[12:], uint32(f))
}

// Poly1305 computes the tag of msg under a one-time key (RFC 8439 §2.5).
func Poly1305(out *[16]byte, msg []byte, key *[32]byte) {
	p := newPoly1305(key)
	p.write(msg)
	p.sum(out)
}

// PolyKey derives the one-time Poly1305 key (RFC 8439 §2.6).
func PolyKey(key *[KeySize]byte, nonce *[NonceSize]byte) [32]byte {
	var b [64]byte
	Block(&b, key, 0, nonce)
	return [32]byte(b[:32])
}

type aead struct{ key [KeySize]byte }

// New returns the AEAD_CHACHA20_POLY1305 construction (RFC 8439 §2.8).
func New(key []byte) (cipher.AEAD, error) {
	if len(key) != KeySize {
		return nil, errors.New("chacha20poly1305: bad key length")
	}
	return &aead{key: [KeySize]byte(key)}, nil
}

func (a *aead) NonceSize() int { return NonceSize }
func (a *aead) Overhead() int  { return Overhead }

func (a *aead) tag(out *[16]byte, nonce *[NonceSize]byte, ad, ct []byte) {
	pk := PolyKey(&a.key, nonce)
	p := newPoly1305(&pk)
	var zero [16]byte
	p.write(ad)
	p.write(zero[:(16-len(ad)%16)%16])
	p.write(ct)
	p.write(zero[:(16-len(ct)%16)%16])
	var lens [16]byte
	binary.LittleEndian.PutUint64(lens[0:], uint64(len(ad)))
	binary.LittleEndian.PutUint64(lens[8:], uint64(len(ct)))
	p.write(lens[:])
	p.sum(out)
}

// grow extends b by n bytes, returning the whole slice and the new tail.
func grow(b []byte, n int) (whole, tail []byte) {
	if total := len(b) + n; cap(b) >= total {
		whole = b[:total]
	} else {
		whole = make([]byte, total)
		copy(whole, b)
	}
	return whole, whole[len(b):]
}

func (a *aead) Seal(dst, nonce, plaintext, ad []byte) []byte {
	if len(nonce) != NonceSize {
		panic("chacha20poly1305: bad nonce length")
	}
	n := [NonceSize]byte(nonce)
	ret, out := grow(dst, len(plaintext)+Overhead)
	XORKeyStream(out, plaintext, &a.key, 1, &n)
	var t [16]byte
	a.tag(&t, &n, ad, out[:len(plaintext)])
	copy(out[len(plaintext):], t[:])
	return ret
}

func (a *aead) Open(dst, nonce, ciphertext, ad []byte) ([]byte, error) {
	if len(nonce) != NonceSize {
		panic("chacha20poly1305: bad nonce length")
	}
	if len(ciphertext) < Overhead {
		return nil, errOpen
	}
	n := [NonceSize]byte(nonce)
	ct, tag := ciphertext[:len(ciphertext)-Overhead], ciphertext[len(ciphertext)-Overhead:]
	var want [16]byte
	a.tag(&want, &n, ad, ct)
	if subtle.ConstantTimeCompare(want[:], tag) != 1 {
		return nil, errOpen
	}
	ret, out := grow(dst, len(ct))
	XORKeyStream(out, ct, &a.key, 1, &n)
	return ret, nil
}
