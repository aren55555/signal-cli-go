// Package ristretto255 implements the ristretto255 prime-order group of
// RFC 9496 on top of edwards25519. zkgroup builds on it.
//
// Scalars are edwards25519 scalars: the group order is the same ℓ.
// Every function here is constant-time; decoding failures depend only on
// the (public) encoding being decoded.
package ristretto255

import (
	"errors"
	"math/big"

	"github.com/aren55555/signal-cli-go/internal/crypto/edwards25519"
	"github.com/aren55555/signal-cli-go/internal/crypto/edwards25519/field"
)

// RFC 9496 section 4.1 constants, from the decimal values in the RFC.
var (
	constD              = decimal("37095705934669439343138083508754565189542113879843219016388785533085940283555")
	constSqrtM1         = decimal("19681161376707505956807079304988542015446066515923890162744021073123829784752")
	constSqrtADMinusOne = decimal("25063068953384623474111414158702152701244531502492656460079210482610430750235")
	constInvSqrtAMinusD = decimal("54469307008909316920995813868745141605393597292927456921205312896311721017578")
	constOneMinusDSq    = decimal("1159843021668779879193775521855586647937357759715417654439879720876111806838")
	constDMinusOneSq    = decimal("40440834346308536858101042469323190826248399146238708352240133220865137265952")
)

func decimal(s string) *field.Element {
	x, ok := new(big.Int).SetString(s, 10)
	if !ok {
		panic("ristretto255: bad constant")
	}
	var b [32]byte
	x.FillBytes(b[:])
	for i, j := 0, 31; i < j; i, j = i+1, j-1 {
		b[i], b[j] = b[j], b[i]
	}
	e, err := new(field.Element).SetBytes(b[:])
	if err != nil {
		panic(err)
	}
	return e
}

// Element is a ristretto255 group element. Use NewIdentityElement or
// NewGeneratorElement; the zero value is not valid.
type Element struct {
	p edwards25519.Point
}

// NewIdentityElement returns a new element set to the identity.
func NewIdentityElement() *Element {
	e := new(Element)
	e.p.Set(edwards25519.NewIdentityPoint())
	return e
}

// NewGeneratorElement returns a new element set to the canonical
// generator (internally, the edwards25519 base point).
func NewGeneratorElement() *Element {
	e := new(Element)
	e.p.Set(edwards25519.NewGeneratorPoint())
	return e
}

// Set sets e = a and returns e.
func (e *Element) Set(a *Element) *Element {
	e.p.Set(&a.p)
	return e
}

func fe() *field.Element { return new(field.Element) }

// SetCanonicalBytes decodes a 32-byte encoding (RFC 9496 section 4.3.1)
// into e and returns e. Non-canonical encodings are rejected.
func (e *Element) SetCanonicalBytes(b []byte) (*Element, error) {
	if len(b) != 32 {
		return nil, errors.New("ristretto255: invalid encoding length")
	}
	s, _ := fe().SetBytes(b)
	// field.SetBytes masks the top bit and reduces mod p; re-encoding
	// differs from b exactly when b's value is >= p.
	if string(s.Bytes()) != string(b) {
		return nil, errors.New("ristretto255: non-canonical encoding")
	}
	if s.IsNegative() == 1 {
		return nil, errors.New("ristretto255: negative encoding")
	}

	one := fe().One()
	ss := fe().Square(s)
	u1 := fe().Subtract(one, ss)
	u2 := fe().Add(one, ss)
	u2Sqr := fe().Square(u2)
	v := fe().Multiply(constD, fe().Square(u1))
	v.Negate(v)
	v.Subtract(v, u2Sqr)

	invSqrt, wasSquare := fe().SqrtRatio(one, fe().Multiply(v, u2Sqr))
	denX := fe().Multiply(invSqrt, u2)
	denY := fe().Multiply(fe().Multiply(invSqrt, denX), v)

	x := fe().Multiply(fe().Add(s, s), denX)
	x.Absolute(x)
	y := fe().Multiply(u1, denY)
	t := fe().Multiply(x, y)

	if wasSquare == 0 || t.IsNegative() == 1 || y.Equal(fe()) == 1 {
		return nil, errors.New("ristretto255: invalid encoding")
	}
	e.p.SetExtendedCoordinates(x, y, one, t)
	return e, nil
}

// Bytes returns the 32-byte canonical encoding of e (RFC 9496 section 4.3.2).
func (e *Element) Bytes() []byte {
	x0, y0, z0, t0 := e.p.ExtendedCoordinates()

	u1 := fe().Multiply(fe().Add(z0, y0), fe().Subtract(z0, y0))
	u2 := fe().Multiply(x0, y0)
	invSqrt, _ := fe().SqrtRatio(fe().One(), fe().Multiply(u1, fe().Square(u2)))
	den1 := fe().Multiply(invSqrt, u1)
	den2 := fe().Multiply(invSqrt, u2)
	zInv := fe().Multiply(fe().Multiply(den1, den2), t0)
	ix0 := fe().Multiply(x0, constSqrtM1)
	iy0 := fe().Multiply(y0, constSqrtM1)
	enchanted := fe().Multiply(den1, constInvSqrtAMinusD)
	rotate := fe().Multiply(t0, zInv).IsNegative()

	x := fe().Select(iy0, x0, rotate)
	y := fe().Select(ix0, y0, rotate)
	denInv := fe().Select(enchanted, den2, rotate)
	y.Select(fe().Negate(y), y, fe().Multiply(x, zInv).IsNegative())
	s := fe().Multiply(denInv, fe().Subtract(z0, y))
	return s.Absolute(s).Bytes()
}

// SetUniformBytes sets e to the element derived from 64 uniformly random
// bytes (RFC 9496 section 4.3.4) and returns e.
func (e *Element) SetUniformBytes(b []byte) (*Element, error) {
	if len(b) != 64 {
		return nil, errors.New("ristretto255: invalid uniform input length")
	}
	var p1, p2 edwards25519.Point
	mapToPoint(&p1, b[:32])
	mapToPoint(&p2, b[32:])
	e.p.Add(&p1, &p2)
	return e, nil
}

// mapToPoint is MAP from RFC 9496 section 4.3.4.
func mapToPoint(out *edwards25519.Point, b []byte) {
	t, _ := fe().SetBytes(b) // masks the top bit and reduces, as MAP requires
	one := fe().One()
	minusOne := fe().Negate(one)

	r := fe().Multiply(constSqrtM1, fe().Square(t))
	u := fe().Multiply(fe().Add(r, one), constOneMinusDSq)
	v := fe().Multiply(fe().Subtract(minusOne, fe().Multiply(r, constD)), fe().Add(r, constD))

	s, wasSquare := fe().SqrtRatio(u, v)
	sPrime := fe().Multiply(s, t)
	sPrime.Absolute(sPrime)
	sPrime.Negate(sPrime)
	s.Select(s, sPrime, wasSquare)
	c := fe().Select(minusOne, r, wasSquare)

	n := fe().Multiply(fe().Multiply(c, fe().Subtract(r, one)), constDMinusOneSq)
	n.Subtract(n, v)

	w0 := fe().Multiply(fe().Add(s, s), v)
	w1 := fe().Multiply(n, constSqrtADMinusOne)
	ss := fe().Square(s)
	w2 := fe().Subtract(one, ss)
	w3 := fe().Add(one, ss)

	out.SetExtendedCoordinates(fe().Multiply(w0, w3), fe().Multiply(w2, w1),
		fe().Multiply(w1, w3), fe().Multiply(w0, w2))
}

// Equal returns 1 if e and u are the same group element, and 0 otherwise
// (RFC 9496 section 4.3.3).
func (e *Element) Equal(u *Element) int {
	x1, y1, _, _ := e.p.ExtendedCoordinates()
	x2, y2, _, _ := u.p.ExtendedCoordinates()
	a := fe().Multiply(x1, y2).Equal(fe().Multiply(y1, x2))
	b := fe().Multiply(y1, y2).Equal(fe().Multiply(x1, x2))
	return a | b
}

// Add sets e = a + b and returns e.
func (e *Element) Add(a, b *Element) *Element {
	e.p.Add(&a.p, &b.p)
	return e
}

// Subtract sets e = a - b and returns e.
func (e *Element) Subtract(a, b *Element) *Element {
	e.p.Subtract(&a.p, &b.p)
	return e
}

// Negate sets e = -a and returns e.
func (e *Element) Negate(a *Element) *Element {
	e.p.Negate(&a.p)
	return e
}

// ScalarMult sets e = [s]a and returns e.
func (e *Element) ScalarMult(s *edwards25519.Scalar, a *Element) *Element {
	e.p.ScalarMult(s, &a.p)
	return e
}

// ScalarBaseMult sets e = [s]G and returns e.
func (e *Element) ScalarBaseMult(s *edwards25519.Scalar) *Element {
	e.p.ScalarBaseMult(s)
	return e
}

// SetElligatorBytes sets e to MAP of one 32-byte string (RFC 9496 section
// 4.3.4, a single application with no addition): the top bit is cleared
// and the value reduced mod p. zkgroup derives some points this way
// (spec/crypto/zkgroup-profile.md §2.4).
func (e *Element) SetElligatorBytes(b []byte) (*Element, error) {
	if len(b) != 32 {
		return nil, errors.New("ristretto255: invalid Elligator input length")
	}
	mapToPoint(&e.p, b)
	return e, nil
}

// Affine returns the affine Edwards coordinates (x, y) of e's internal
// representative. Different representatives of one element give
// different coordinates.
func (e *Element) Affine() (x, y *field.Element) {
	X, Y, Z, _ := e.p.ExtendedCoordinates()
	zInv := fe().Invert(Z)
	return fe().Multiply(X, zInv), fe().Multiply(Y, zInv)
}

// SetMap sets e to MAP(r) of RFC 9496 section 4.3.4 for a field element.
func (e *Element) SetMap(r *field.Element) *Element {
	mapToPoint(&e.p, r.Bytes())
	return e
}

// D, SqrtM1 and InvSqrtAMinusD return copies of RFC 9496 constants.
func D() *field.Element              { return fe().Set(constD) }
func SqrtM1() *field.Element         { return fe().Set(constSqrtM1) }
func InvSqrtAMinusD() *field.Element { return fe().Set(constInvSqrtAMinusD) }
