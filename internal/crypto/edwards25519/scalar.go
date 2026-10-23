package edwards25519

import (
	"crypto/subtle"
	"encoding/binary"
	"errors"
	"math/big"
	"math/bits"
)

// Scalar is an integer modulo the group order
// ℓ = 2^252 + 27742317777372353535851937790883648493. The zero value is 0.
//
// It is stored fully reduced as four little-endian 64-bit limbs.
// Multiplication uses Montgomery reduction with R = 2^256. All operations
// are constant-time.
type Scalar struct {
	l [4]uint64
}

// Montgomery constants. They are public, so deriving them with math/big
// at init is fine and avoids transcribing magic numbers.
var (
	orderLimbs [4]uint64 // ℓ
	rr         [4]uint64 // R^2 mod ℓ
	rrr        [4]uint64 // R^3 mod ℓ
	nPrime     uint64    // -ℓ^-1 mod 2^64
)

func init() {
	l, _ := new(big.Int).SetString("7237005577332262213973186563042994240857116359379907606001950938285454250989", 10)
	r := new(big.Int).Lsh(big.NewInt(1), 256)
	limbs := func(x *big.Int) (out [4]uint64) {
		var b [32]byte
		x.FillBytes(b[:])
		for i := range out {
			out[i] = binary.BigEndian.Uint64(b[24-8*i:])
		}
		return out
	}
	orderLimbs = limbs(l)
	r2 := new(big.Int).Mod(new(big.Int).Mul(r, r), l)
	rr = limbs(r2)
	rrr = limbs(new(big.Int).Mod(new(big.Int).Mul(r2, r), l))
	two64 := new(big.Int).Lsh(big.NewInt(1), 64)
	inv := new(big.Int).ModInverse(new(big.Int).Mod(l, two64), two64)
	nPrime = new(big.Int).Sub(two64, inv).Uint64()
}

// NewScalar returns a new zero Scalar.
func NewScalar() *Scalar { return new(Scalar) }

// Set sets s = a and returns s.
func (s *Scalar) Set(a *Scalar) *Scalar {
	*s = *a
	return s
}

func load(b []byte) (out [4]uint64) {
	for i := range out {
		out[i] = binary.LittleEndian.Uint64(b[8*i:])
	}
	return out
}

// sub returns a - b and the borrow (0 or 1).
func sub(a, b *[4]uint64) (d [4]uint64, borrow uint64) {
	for i := range d {
		d[i], borrow = bits.Sub64(a[i], b[i], borrow)
	}
	return d, borrow
}

// reduceOnce returns a mod ℓ for a < 2ℓ.
func reduceOnce(a [4]uint64) [4]uint64 {
	d, borrow := sub(&a, &orderLimbs)
	m := borrow - 1 // all ones when a >= ℓ
	for i := range a {
		a[i] = d[i]&m | a[i]&^m
	}
	return a
}

// montMul returns a*b/R mod ℓ, for any a < 2^256 and b < ℓ.
func montMul(a, b *[4]uint64) [4]uint64 {
	// Schoolbook product into t[0..7]; t[8] absorbs the reduction carry.
	var t [9]uint64
	for i := range 4 {
		var carry uint64
		for j := range 4 {
			hi, lo := bits.Mul64(a[i], b[j])
			var c uint64
			lo, c = bits.Add64(lo, t[i+j], 0)
			hi += c
			lo, c = bits.Add64(lo, carry, 0)
			hi += c
			t[i+j], carry = lo, hi
		}
		t[i+4] = carry
	}
	// Add multiples of ℓ to clear the low four limbs. The result (t >> 256)
	// is below (2^256 ℓ + 2^256 ℓ) / 2^256 = 2ℓ.
	for i := range 4 {
		m := t[i] * nPrime
		var carry uint64
		for j := range 4 {
			hi, lo := bits.Mul64(m, orderLimbs[j])
			var c uint64
			lo, c = bits.Add64(lo, t[i+j], 0)
			hi += c
			lo, c = bits.Add64(lo, carry, 0)
			hi += c
			t[i+j], carry = lo, hi
		}
		for k := i + 4; k < 9; k++ {
			t[k], carry = bits.Add64(t[k], carry, 0)
		}
	}
	return reduceOnce([4]uint64{t[4], t[5], t[6], t[7]})
}

// SetCanonicalBytes sets s to the little-endian value of b, which must be
// 32 bytes encoding an integer below ℓ.
func (s *Scalar) SetCanonicalBytes(b []byte) (*Scalar, error) {
	if len(b) != 32 {
		return nil, errors.New("edwards25519: invalid scalar length")
	}
	x := load(b)
	if _, borrow := sub(&x, &orderLimbs); borrow == 0 {
		return nil, errors.New("edwards25519: non-canonical scalar")
	}
	s.l = x
	return s, nil
}

// SetUniformBytes sets s to the 64-byte little-endian value of b reduced
// mod ℓ, as for the SHA-512 outputs in RFC 8032 and XEdDSA.
func (s *Scalar) SetUniformBytes(b []byte) (*Scalar, error) {
	if len(b) != 64 {
		return nil, errors.New("edwards25519: invalid uniform scalar length")
	}
	// With b = lo + hi*R: lo*R + hi*R^2 = R*b mod ℓ, then divide by R.
	lo, hi := load(b[:32]), load(b[32:])
	x := montMul(&lo, &rr)
	y := montMul(&hi, &rrr)
	var sum [4]uint64
	var c uint64
	for i := range sum {
		sum[i], c = bits.Add64(x[i], y[i], c)
	}
	sum = reduceOnce(sum)
	one := [4]uint64{1}
	s.l = montMul(&sum, &one)
	return s, nil
}

// SetBytesWithClamping applies the X25519/Ed25519 clamping to the 32 bytes
// of b (clear the low three bits and bit 255, set bit 254) and sets s to
// the result reduced mod ℓ.
func (s *Scalar) SetBytesWithClamping(b []byte) (*Scalar, error) {
	if len(b) != 32 {
		return nil, errors.New("edwards25519: invalid scalar length")
	}
	var wide [64]byte
	copy(wide[:], b)
	wide[0] &= 248
	wide[31] &= 127
	wide[31] |= 64
	return s.SetUniformBytes(wide[:])
}

// Bytes returns the canonical 32-byte little-endian encoding of s.
func (s *Scalar) Bytes() []byte {
	out := make([]byte, 0, 32)
	for _, w := range s.l {
		out = binary.LittleEndian.AppendUint64(out, w)
	}
	return out
}

// Equal returns 1 if s and a are equal, and 0 otherwise.
func (s *Scalar) Equal(a *Scalar) int {
	return subtle.ConstantTimeCompare(s.Bytes(), a.Bytes())
}

// Add sets s = a + b mod ℓ and returns s.
func (s *Scalar) Add(a, b *Scalar) *Scalar {
	var sum [4]uint64
	var c uint64
	for i := range sum {
		sum[i], c = bits.Add64(a.l[i], b.l[i], c)
	}
	s.l = reduceOnce(sum) // a + b < 2ℓ < 2^254: no carry out
	return s
}

// Subtract sets s = a - b mod ℓ and returns s.
func (s *Scalar) Subtract(a, b *Scalar) *Scalar {
	d, borrow := sub(&a.l, &b.l)
	m := -borrow // all ones when a < b: add ℓ back
	var c uint64
	for i := range d {
		d[i], c = bits.Add64(d[i], orderLimbs[i]&m, c)
	}
	s.l = d
	return s
}

// Negate sets s = -a mod ℓ and returns s.
func (s *Scalar) Negate(a *Scalar) *Scalar {
	return s.Subtract(new(Scalar), a)
}

// Multiply sets s = a * b mod ℓ and returns s.
func (s *Scalar) Multiply(a, b *Scalar) *Scalar {
	t := montMul(&a.l, &b.l) // ab/R
	s.l = montMul(&t, &rr)   // ab/R * R^2 / R
	return s
}

// MultiplyAdd sets s = a*b + c mod ℓ and returns s.
func (s *Scalar) MultiplyAdd(a, b, c *Scalar) *Scalar {
	cc := *c // c may alias s
	return s.Add(s.Multiply(a, b), &cc)
}

// bits4 returns the scalar as 64 little-endian 4-bit digits.
func (s *Scalar) bits4() (d [64]byte) {
	for i, w := range s.l {
		for j := range 16 {
			d[16*i+j] = byte(w>>(4*j)) & 15
		}
	}
	return d
}
