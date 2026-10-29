// SPDX-License-Identifier: GPL-3.0-or-later

// Package field implements constant-time arithmetic in GF(2^255-19), the
// base field of Curve25519 and edwards25519.
//
// An Element holds five 51-bit limbs (radix 2^51). Every operation leaves
// each limb below 2^51 + 2^18, which keeps all intermediate products in
// Multiply within 128 bits. Encoding to bytes fully reduces mod p.
//
// No operation branches on, or indexes memory by, the value of an
// Element; conditions are passed as 0/1 ints and applied with masks.
package field

import (
	"crypto/subtle"
	"encoding/binary"
	"errors"
	"math/bits"
)

// Element is an element of GF(2^255-19). The zero value is 0.
type Element struct {
	l [5]uint64
}

const mask51 = 1<<51 - 1

// Zero sets v = 0 and returns v.
func (v *Element) Zero() *Element {
	*v = Element{}
	return v
}

// One sets v = 1 and returns v.
func (v *Element) One() *Element {
	*v = Element{[5]uint64{1}}
	return v
}

// Set sets v = a and returns v.
func (v *Element) Set(a *Element) *Element {
	*v = *a
	return v
}

// SetBytes sets v to the little-endian value of b, which must be 32 bytes.
// As in RFC 7748, the top bit is ignored, and values from p to 2^255-1 are
// accepted and reduced. Callers that need canonical encodings compare the
// re-encoding with the input.
func (v *Element) SetBytes(b []byte) (*Element, error) {
	if len(b) != 32 {
		return nil, errors.New("field: invalid element length")
	}
	v.l[0] = binary.LittleEndian.Uint64(b[0:8]) & mask51
	v.l[1] = binary.LittleEndian.Uint64(b[6:14]) >> 3 & mask51
	v.l[2] = binary.LittleEndian.Uint64(b[12:20]) >> 6 & mask51
	v.l[3] = binary.LittleEndian.Uint64(b[19:27]) >> 1 & mask51
	v.l[4] = binary.LittleEndian.Uint64(b[24:32]) >> 12 & mask51
	return v, nil
}

// Bytes returns the canonical 32-byte little-endian encoding of v.
func (v *Element) Bytes() []byte {
	var out [32]byte
	return v.appendBytes(out[:0])
}

func (v *Element) appendBytes(dst []byte) []byte {
	t := *v
	t.carry()
	// t < 2p now. q is 1 exactly when t >= p, i.e. when t + 19 >= 2^255.
	q := (t.l[0] + 19) >> 51
	q = (t.l[1] + q) >> 51
	q = (t.l[2] + q) >> 51
	q = (t.l[3] + q) >> 51
	q = (t.l[4] + q) >> 51
	// Subtract q*p by adding 19q and dropping bit 255.
	t.l[0] += 19 * q
	t.l[1] += t.l[0] >> 51
	t.l[0] &= mask51
	t.l[2] += t.l[1] >> 51
	t.l[1] &= mask51
	t.l[3] += t.l[2] >> 51
	t.l[2] &= mask51
	t.l[4] += t.l[3] >> 51
	t.l[3] &= mask51
	t.l[4] &= mask51

	dst = binary.LittleEndian.AppendUint64(dst, t.l[0]|t.l[1]<<51)
	dst = binary.LittleEndian.AppendUint64(dst, t.l[1]>>13|t.l[2]<<38)
	dst = binary.LittleEndian.AppendUint64(dst, t.l[2]>>26|t.l[3]<<25)
	dst = binary.LittleEndian.AppendUint64(dst, t.l[3]>>39|t.l[4]<<12)
	return dst
}

// carry brings every limb below 2^51 + 2^18, given limbs below 2^64.
func (v *Element) carry() {
	c0, c1, c2, c3, c4 := v.l[0]>>51, v.l[1]>>51, v.l[2]>>51, v.l[3]>>51, v.l[4]>>51
	v.l[0] = v.l[0]&mask51 + 19*c4
	v.l[1] = v.l[1]&mask51 + c0
	v.l[2] = v.l[2]&mask51 + c1
	v.l[3] = v.l[3]&mask51 + c2
	v.l[4] = v.l[4]&mask51 + c3
}

// Equal returns 1 if v and u are equal, and 0 otherwise.
func (v *Element) Equal(u *Element) int {
	var a, b [32]byte
	return subtle.ConstantTimeCompare(v.appendBytes(a[:0]), u.appendBytes(b[:0]))
}

// IsNegative returns 1 if v is negative in the sense of RFC 8032 and RFC
// 9496 (its canonical encoding is odd), and 0 otherwise.
func (v *Element) IsNegative() int {
	var b [32]byte
	return int(v.appendBytes(b[:0])[0] & 1)
}

// Absolute sets v = |u| (the non-negative one of u and -u) and returns v.
func (v *Element) Absolute(u *Element) *Element {
	var neg Element
	neg.Negate(u)
	return v.Select(&neg, u, u.IsNegative())
}

// Add sets v = a + b and returns v.
func (v *Element) Add(a, b *Element) *Element {
	for i := range v.l {
		v.l[i] = a.l[i] + b.l[i]
	}
	v.carry()
	return v
}

// Subtract sets v = a - b and returns v.
func (v *Element) Subtract(a, b *Element) *Element {
	// Add 2p first so no limb underflows: b's limbs are below 2^51 + 2^18,
	// and 2p's limbs are 2^52-38 and 2^52-2.
	v.l[0] = a.l[0] + (1<<52 - 38) - b.l[0]
	for i := 1; i < 5; i++ {
		v.l[i] = a.l[i] + (1<<52 - 2) - b.l[i]
	}
	v.carry()
	return v
}

// Negate sets v = -a and returns v.
func (v *Element) Negate(a *Element) *Element {
	return v.Subtract(new(Element), a)
}

// u128 is a 128-bit accumulator.
type u128 struct{ hi, lo uint64 }

func mul64(a, b uint64) u128 {
	hi, lo := bits.Mul64(a, b)
	return u128{hi, lo}
}

func (x u128) add(y u128) u128 {
	lo, c := bits.Add64(x.lo, y.lo, 0)
	return u128{x.hi + y.hi + c, lo}
}

// shr51 returns x >> 51. x is below 2^115 here, so the result fits.
func (x u128) shr51() uint64 { return x.hi<<13 | x.lo>>51 }

// reduceWide sets v from five column sums r0..r4 of a product, each below
// 2^115, carrying the excess of each into the next and wrapping the top
// with the factor 19 (2^255 = 19 mod p).
func (v *Element) reduceWide(r0, r1, r2, r3, r4 u128) {
	c0, c1, c2, c3, c4 := r0.shr51(), r1.shr51(), r2.shr51(), r3.shr51(), r4.shr51()
	// c4 < 2^64/19 because each column sum is below 2^115 - see Multiply.
	v.l[0] = r0.lo&mask51 + 19*c4
	v.l[1] = r1.lo&mask51 + c0
	v.l[2] = r2.lo&mask51 + c1
	v.l[3] = r3.lo&mask51 + c2
	v.l[4] = r4.lo&mask51 + c3
	v.carry()
}

// Multiply sets v = a * b and returns v.
func (v *Element) Multiply(a, b *Element) *Element {
	// With limbs below 2^51 + 2^18, each product with a 19-scaled limb is
	// below 2^107, so each column of five products is below 2^110 and its
	// carry below 2^59: 19 times that fits in 64 bits.
	a0, a1, a2, a3, a4 := a.l[0], a.l[1], a.l[2], a.l[3], a.l[4]
	b0, b1, b2, b3, b4 := b.l[0], b.l[1], b.l[2], b.l[3], b.l[4]
	b1x, b2x, b3x, b4x := 19*b1, 19*b2, 19*b3, 19*b4

	r0 := mul64(a0, b0).add(mul64(a1, b4x)).add(mul64(a2, b3x)).add(mul64(a3, b2x)).add(mul64(a4, b1x))
	r1 := mul64(a0, b1).add(mul64(a1, b0)).add(mul64(a2, b4x)).add(mul64(a3, b3x)).add(mul64(a4, b2x))
	r2 := mul64(a0, b2).add(mul64(a1, b1)).add(mul64(a2, b0)).add(mul64(a3, b4x)).add(mul64(a4, b3x))
	r3 := mul64(a0, b3).add(mul64(a1, b2)).add(mul64(a2, b1)).add(mul64(a3, b0)).add(mul64(a4, b4x))
	r4 := mul64(a0, b4).add(mul64(a1, b3)).add(mul64(a2, b2)).add(mul64(a3, b1)).add(mul64(a4, b0))
	v.reduceWide(r0, r1, r2, r3, r4)
	return v
}

// Square sets v = a * a and returns v.
func (v *Element) Square(a *Element) *Element {
	a0, a1, a2, a3, a4 := a.l[0], a.l[1], a.l[2], a.l[3], a.l[4]
	d0, d1 := 2*a0, 2*a1
	a3x, a4x := 19*a3, 19*a4

	r0 := mul64(a0, a0).add(mul64(d1, a4x)).add(mul64(2*a2, a3x))
	r1 := mul64(d0, a1).add(mul64(2*a2, a4x)).add(mul64(a3, a3x))
	r2 := mul64(d0, a2).add(mul64(a1, a1)).add(mul64(2*a3, a4x))
	r3 := mul64(d0, a3).add(mul64(d1, a2)).add(mul64(a4, a4x))
	r4 := mul64(d0, a4).add(mul64(d1, a3)).add(mul64(a2, a2))
	v.reduceWide(r0, r1, r2, r3, r4)
	return v
}

// squareN sets v = a^(2^n) and returns v.
func (v *Element) squareN(a *Element, n int) *Element {
	v.Square(a)
	for i := 1; i < n; i++ {
		v.Square(v)
	}
	return v
}

// pow2250 returns a^(2^250-1) and a^11, the shared prefix of the
// exponentiation chains for inversion and square roots.
func pow2250(a *Element) (e2250, e11 Element) {
	var e2, e9, t, e5, e10, e20, e50, e100 Element
	e2.Square(a)
	e9.Multiply(t.squareN(&e2, 2), a)    // a^9
	e11.Multiply(&e9, &e2)               // a^11
	e5.Multiply(t.Square(&e11), &e9)     // a^(2^5-1)
	e10.Multiply(t.squareN(&e5, 5), &e5) // a^(2^10-1)
	e20.Multiply(t.squareN(&e10, 10), &e10)
	e50.Multiply(t.squareN(&e20, 20), &e20) // a^(2^40-1) for now
	e50.Multiply(t.squareN(&e50, 10), &e10) // a^(2^50-1)
	e100.Multiply(t.squareN(&e50, 50), &e50)
	e2250.Multiply(t.squareN(&e100, 100), &e100) // a^(2^200-1)
	e2250.Multiply(t.squareN(&e2250, 50), &e50)  // a^(2^250-1)
	return e2250, e11
}

// Invert sets v = 1/a (and 0 when a is 0) and returns v.
func (v *Element) Invert(a *Element) *Element {
	// a^(p-2), with p-2 = (2^250-1)*2^5 + 11.
	e2250, e11 := pow2250(a)
	var t Element
	return v.Multiply(t.squareN(&e2250, 5), &e11)
}

// Pow22523 sets v = a^((p-5)/8) and returns v.
func (v *Element) Pow22523(a *Element) *Element {
	// (p-5)/8 = 2^252-3 = (2^250-1)*4 + 1.
	e2250, _ := pow2250(a)
	var t Element
	return v.Multiply(t.squareN(&e2250, 2), a)
}

// Select sets v = a if cond is 1, and v = b if cond is 0, and returns v.
func (v *Element) Select(a, b *Element, cond int) *Element {
	m := -uint64(cond & 1)
	for i := range v.l {
		v.l[i] = a.l[i]&m | b.l[i]&^m
	}
	return v
}

// Swap swaps v and u if cond is 1, and leaves both unchanged if cond is 0.
func (v *Element) Swap(u *Element, cond int) {
	m := -uint64(cond & 1)
	for i := range v.l {
		t := m & (v.l[i] ^ u.l[i])
		v.l[i] ^= t
		u.l[i] ^= t
	}
}

// sqrtM1 is a square root of -1: 2^((p-1)/4) mod p.
var sqrtM1 = mustHex("b0a00e4a271beec478e42fad0618432fa7d7fb3d99004d2b0bdfc14f8024832b")

// SqrtM1 returns a new Element holding the square root of -1 that
// RFC 9496 calls SQRT_M1.
func SqrtM1() *Element { return new(Element).Set(sqrtM1) }

// SqrtRatio implements SQRT_RATIO_M1(u, v) from RFC 9496 section 4.2. It
// sets r to the non-negative square root of u/v and returns (r, 1) if u/v
// is square. Otherwise it sets r to the non-negative square root of
// SQRT_M1*u/v and returns (r, 0). If u is 0 it returns (0, 1); if v is 0
// and u is not, it returns (0, 0).
func (r *Element) SqrtRatio(u, v *Element) (*Element, int) {
	var v2, v3, v7, uv3, uv7, t Element
	v2.Square(v)
	v3.Multiply(&v2, v)
	v7.Multiply(t.Square(&v3), v)
	uv3.Multiply(u, &v3)
	uv7.Multiply(u, &v7)

	var cand Element
	cand.Multiply(&uv3, t.Pow22523(&uv7))

	var check, negU, negUi Element
	check.Multiply(v, t.Square(&cand))
	negU.Negate(u)
	negUi.Multiply(&negU, sqrtM1)
	correctSign := check.Equal(u)
	flipped := check.Equal(&negU)
	flippedI := check.Equal(&negUi)

	var rotated Element
	rotated.Multiply(&cand, sqrtM1)
	cand.Select(&rotated, &cand, flipped|flippedI)
	r.Absolute(&cand)
	return r, correctSign | flipped
}

func mustHex(s string) *Element {
	var b [32]byte
	for i := range b {
		b[i] = unhex(s[2*i])<<4 | unhex(s[2*i+1])
	}
	e, err := new(Element).SetBytes(b[:])
	if err != nil {
		panic(err)
	}
	return e
}

func unhex(c byte) byte {
	switch {
	case c >= '0' && c <= '9':
		return c - '0'
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10
	}
	panic("field: bad hex constant")
}
