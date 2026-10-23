// Package edwards25519 implements the edwards25519 group of RFC 8032
// (-x^2 + y^2 = 1 + d x^2 y^2 over GF(2^255-19)) and its scalars.
//
// Points use extended coordinates (X:Y:Z:T) with x = X/Z, y = Y/Z and
// xy = T/Z. The addition and doubling formulas are the complete ones for
// a = -1 (Hisil, Wong, Carter, Dawson 2008), so no input needs a special
// case. Scalar multiplication uses a fixed 4-bit window with a
// constant-time table lookup; nothing branches on or indexes memory by a
// secret value.
package edwards25519

import (
	"errors"
	"sync"

	"github.com/aren55555/signal-cli-go/internal/crypto/edwards25519/field"
)

// Point is a point on edwards25519. Use NewIdentityPoint or
// NewGeneratorPoint; the zero value is not a valid point.
type Point struct {
	x, y, z, t field.Element
}

var (
	curveD  = new(field.Element) // d = -121665/121666
	curveD2 = new(field.Element) // 2d
	gen     = new(Point)
)

func init() {
	small := func(n byte) *field.Element {
		var b [32]byte
		b[0] = n
		e, _ := new(field.Element).SetBytes(b[:])
		return e
	}
	k := func(lo, mid, hi byte) *field.Element {
		var b [32]byte
		b[0], b[1], b[2] = lo, mid, hi
		e, _ := new(field.Element).SetBytes(b[:])
		return e
	}
	// 121665 = 0x01db41, 121666 = 0x01db42.
	curveD.Multiply(k(0x41, 0xdb, 0x01), new(field.Element).Invert(k(0x42, 0xdb, 0x01)))
	curveD.Negate(curveD)
	curveD2.Add(curveD, curveD)

	// B has y = 4/5 and non-negative x (RFC 8032 section 5.1).
	y := new(field.Element).Multiply(small(4), new(field.Element).Invert(small(5)))
	if _, err := gen.SetBytes(y.Bytes()); err != nil {
		panic("edwards25519: bad generator")
	}
}

// NewIdentityPoint returns a new point set to the identity (0, 1).
func NewIdentityPoint() *Point {
	p := new(Point)
	p.y.One()
	p.z.One()
	return p
}

// NewGeneratorPoint returns a new point set to the RFC 8032 base point B.
func NewGeneratorPoint() *Point {
	return new(Point).Set(gen)
}

// Set sets v = u and returns v.
func (v *Point) Set(u *Point) *Point {
	*v = *u
	return v
}

// ExtendedCoordinates returns v's (X:Y:Z:T) coordinates. Ristretto255
// builds on these.
func (v *Point) ExtendedCoordinates() (X, Y, Z, T *field.Element) {
	return new(field.Element).Set(&v.x), new(field.Element).Set(&v.y),
		new(field.Element).Set(&v.z), new(field.Element).Set(&v.t)
}

// SetExtendedCoordinates sets v = (X:Y:Z:T) and returns v. The caller must
// supply a valid point: Z != 0, XY = ZT, and on the curve. Nothing is
// checked.
func (v *Point) SetExtendedCoordinates(X, Y, Z, T *field.Element) *Point {
	v.x.Set(X)
	v.y.Set(Y)
	v.z.Set(Z)
	v.t.Set(T)
	return v
}

// SetBytes sets v to the point encoded by the 32 bytes of b and returns v.
// Decoding is strict, per RFC 8032 section 5.1.3: it rejects y >= p, a
// y with no matching x, and x = 0 with the sign bit set.
func (v *Point) SetBytes(b []byte) (*Point, error) {
	if len(b) != 32 {
		return nil, errors.New("edwards25519: invalid point encoding length")
	}
	y, _ := new(field.Element).SetBytes(b)
	// SetBytes masks bit 255; any other difference means y >= p.
	enc := y.Bytes()
	enc[31] |= b[31] & 0x80
	if string(enc) != string(b) {
		return nil, errors.New("edwards25519: non-canonical point encoding")
	}

	// x^2 = (y^2 - 1) / (d y^2 + 1)
	one := new(field.Element).One()
	y2 := new(field.Element).Square(y)
	u := new(field.Element).Subtract(y2, one)
	w := new(field.Element).Multiply(y2, curveD)
	w.Add(w, one)
	x, wasSquare := new(field.Element).SqrtRatio(u, w)
	if wasSquare == 0 {
		return nil, errors.New("edwards25519: invalid point encoding")
	}
	sign := int(b[31] >> 7)
	if x.Equal(new(field.Element)) == 1 && sign == 1 {
		return nil, errors.New("edwards25519: invalid point encoding")
	}
	// SqrtRatio returns the non-negative root; negate it if the sign says so.
	x.Select(new(field.Element).Negate(x), x, sign)

	v.x.Set(x)
	v.y.Set(y)
	v.z.One()
	v.t.Multiply(x, y)
	return v, nil
}

// affine returns x and y.
func (v *Point) affine() (x, y *field.Element) {
	zInv := new(field.Element).Invert(&v.z)
	return new(field.Element).Multiply(&v.x, zInv), new(field.Element).Multiply(&v.y, zInv)
}

// Bytes returns the RFC 8032 encoding of v: y, with the sign of x in bit 255.
func (v *Point) Bytes() []byte {
	x, y := v.affine()
	out := y.Bytes()
	out[31] |= byte(x.IsNegative() << 7)
	return out
}

// BytesMontgomery returns the RFC 7748 u-coordinate of the Curve25519
// point that corresponds to v under the birational map u = (1+y)/(1-y).
// The identity maps to u = 0.
func (v *Point) BytesMontgomery() []byte {
	// u = (Z+Y)/(Z-Y); Invert(0) = 0 gives the identity's 0.
	num := new(field.Element).Add(&v.z, &v.y)
	den := new(field.Element).Subtract(&v.z, &v.y)
	return num.Multiply(num, den.Invert(den)).Bytes()
}

// Equal returns 1 if v and u are the same point, and 0 otherwise.
func (v *Point) Equal(u *Point) int {
	var a, b, c, d field.Element
	a.Multiply(&v.x, &u.z)
	b.Multiply(&u.x, &v.z)
	c.Multiply(&v.y, &u.z)
	d.Multiply(&u.y, &v.z)
	return a.Equal(&b) & c.Equal(&d)
}

// Add sets v = p + q and returns v.
func (v *Point) Add(p, q *Point) *Point {
	var a, b, c, d, t field.Element
	a.Multiply(t.Subtract(&p.y, &p.x), new(field.Element).Subtract(&q.y, &q.x))
	b.Multiply(t.Add(&p.y, &p.x), new(field.Element).Add(&q.y, &q.x))
	c.Multiply(t.Multiply(&p.t, curveD2), &q.t)
	d.Multiply(t.Add(&p.z, &p.z), &q.z)
	return v.finish(&a, &b, &c, &d)
}

// finish computes the shared tail of addition: with E = B-A, F = D-C,
// G = D+C, H = B+A, the result is (EF : GH : FG : EH).
func (v *Point) finish(a, b, c, d *field.Element) *Point {
	var e, f, g, h field.Element
	e.Subtract(b, a)
	f.Subtract(d, c)
	g.Add(d, c)
	h.Add(b, a)
	v.x.Multiply(&e, &f)
	v.y.Multiply(&g, &h)
	v.z.Multiply(&f, &g)
	v.t.Multiply(&e, &h)
	return v
}

// Subtract sets v = p - q and returns v.
func (v *Point) Subtract(p, q *Point) *Point {
	var nq Point
	return v.Add(p, nq.Negate(q))
}

// Negate sets v = -p and returns v.
func (v *Point) Negate(p *Point) *Point {
	v.x.Negate(&p.x)
	v.y.Set(&p.y)
	v.z.Set(&p.z)
	v.t.Negate(&p.t)
	return v
}

// double sets v = 2p and returns v.
func (v *Point) double(p *Point) *Point {
	// dbl-2008-hwcd with a = -1.
	var a, b, c, d, e, g, f, h, t field.Element
	a.Square(&p.x)
	b.Square(&p.y)
	c.Square(&p.z)
	c.Add(&c, &c)
	d.Negate(&a)
	e.Square(t.Add(&p.x, &p.y))
	e.Subtract(&e, &a)
	e.Subtract(&e, &b)
	g.Add(&d, &b)
	f.Subtract(&g, &c)
	h.Subtract(&d, &b)
	v.x.Multiply(&e, &f)
	v.y.Multiply(&g, &h)
	v.z.Multiply(&f, &g)
	v.t.Multiply(&e, &h)
	return v
}

// table holds the multiples [0]P .. [15]P.
type table [16]Point

func (tb *table) init(p *Point) {
	tb[0] = *NewIdentityPoint()
	tb[1] = *p
	for i := 2; i < 16; i++ {
		tb[i].Add(&tb[i-1], p)
	}
}

// lookup sets v = tb[i] in constant time, touching every entry.
func (tb *table) lookup(v *Point, i byte) {
	*v = tb[0]
	for j := 1; j < 16; j++ {
		eq := int((uint32(i^byte(j)) - 1) >> 31) // 1 iff i == j
		v.x.Select(&tb[j].x, &v.x, eq)
		v.y.Select(&tb[j].y, &v.y, eq)
		v.z.Select(&tb[j].z, &v.z, eq)
		v.t.Select(&tb[j].t, &v.t, eq)
	}
}

func (v *Point) scalarMultTable(s *Scalar, tb *table) *Point {
	digits := s.bits4()
	acc := NewIdentityPoint()
	var sel Point
	for i := 63; i >= 0; i-- {
		acc.double(acc).double(acc).double(acc).double(acc)
		tb.lookup(&sel, digits[i])
		acc.Add(acc, &sel)
	}
	return v.Set(acc)
}

// ScalarMult sets v = [s]p and returns v, in constant time.
func (v *Point) ScalarMult(s *Scalar, p *Point) *Point {
	var tb table
	tb.init(p)
	return v.scalarMultTable(s, &tb)
}

var (
	genTableOnce sync.Once
	genTable     table
)

// ScalarBaseMult sets v = [s]B and returns v, in constant time.
func (v *Point) ScalarBaseMult(s *Scalar) *Point {
	genTableOnce.Do(func() { genTable.init(gen) })
	return v.scalarMultTable(s, &genTable)
}
