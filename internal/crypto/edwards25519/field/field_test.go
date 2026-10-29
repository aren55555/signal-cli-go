// SPDX-License-Identifier: GPL-3.0-or-later

package field

import (
	"bytes"
	"crypto/ecdh"
	"encoding/hex"
	"math/big"
	"math/rand/v2"
	"testing"

	"github.com/aren55555/signal-cli-go/internal/testvec"
)

var p = new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 255), big.NewInt(19))

// le converts between little-endian bytes and big.Int.
func toBig(b []byte) *big.Int {
	r := make([]byte, len(b))
	for i := range b {
		r[len(b)-1-i] = b[i]
	}
	return new(big.Int).SetBytes(r)
}

func fromBig(x *big.Int) []byte {
	b := x.FillBytes(make([]byte, 32))
	for i, j := 0, 31; i < j; i, j = i+1, j-1 {
		b[i], b[j] = b[j], b[i]
	}
	return b
}

// randBytes returns 32 random bytes, biased towards edge cases: values
// near 0, near p, and above p (non-canonical encodings).
func randBytes(r *rand.Rand) []byte {
	b := make([]byte, 32)
	switch r.IntN(6) {
	case 0:
		b[0] = byte(r.IntN(20))
	case 1:
		copy(b, fromBig(new(big.Int).Sub(p, big.NewInt(int64(r.IntN(20)+1)))))
	case 2:
		copy(b, fromBig(new(big.Int).Add(p, big.NewInt(int64(r.IntN(18))))))
	default:
		for i := range b {
			b[i] = byte(r.Uint32())
		}
	}
	return b
}

func mustElement(t *testing.T, b []byte) *Element {
	t.Helper()
	e, err := new(Element).SetBytes(b)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

// reduced is the value SetBytes must produce: bit 255 ignored, reduced mod p.
func reduced(b []byte) *big.Int {
	c := bytes.Clone(b)
	c[31] &= 0x7f
	return new(big.Int).Mod(toBig(c), p)
}

func TestSetBytesReduces(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	for range 2000 {
		b := randBytes(r)
		if r.IntN(2) == 0 {
			b[31] |= 0x80
		}
		got := mustElement(t, b).Bytes()
		if want := fromBig(reduced(b)); !bytes.Equal(got, want) {
			t.Fatalf("SetBytes(%x).Bytes() = %x, want %x", b, got, want)
		}
	}
	if _, err := new(Element).SetBytes(make([]byte, 31)); err == nil {
		t.Error("SetBytes accepted 31 bytes")
	}
}

func TestArithmeticMatchesBig(t *testing.T) {
	r := rand.New(rand.NewPCG(3, 4))
	for range 2000 {
		ab, bb := randBytes(r), randBytes(r)
		a, b := mustElement(t, ab), mustElement(t, bb)
		x, y := reduced(ab), reduced(bb)

		check := func(name string, got *Element, want *big.Int) {
			t.Helper()
			if w := fromBig(new(big.Int).Mod(want, p)); !bytes.Equal(got.Bytes(), w) {
				t.Fatalf("%s(%x, %x) = %x, want %x", name, ab, bb, got.Bytes(), w)
			}
		}
		check("Add", new(Element).Add(a, b), new(big.Int).Add(x, y))
		check("Subtract", new(Element).Subtract(a, b), new(big.Int).Sub(x, y))
		check("Negate", new(Element).Negate(a), new(big.Int).Neg(x))
		check("Multiply", new(Element).Multiply(a, b), new(big.Int).Mul(x, y))
		check("Square", new(Element).Square(a), new(big.Int).Mul(x, x))
		inv := new(big.Int).ModInverse(x, p)
		if inv == nil {
			inv = new(big.Int) // x == 0: Invert(0) is defined as 0
		}
		check("Invert", new(Element).Invert(a), inv)
		// Pow22523 is a^((p-5)/8), used by square roots.
		e := new(big.Int).Div(new(big.Int).Sub(p, big.NewInt(5)), big.NewInt(8))
		check("Pow22523", new(Element).Pow22523(a), new(big.Int).Exp(x, e, p))

		// Aliasing: the receiver may be an operand.
		c := new(Element).Set(a)
		c.Multiply(c, c)
		check("Multiply(aliased)", c, new(big.Int).Mul(x, x))

		wantEq := 0
		if x.Cmp(y) == 0 {
			wantEq = 1
		}
		if got := a.Equal(b); got != wantEq {
			t.Fatalf("Equal(%x, %x) = %d, want %d", ab, bb, got, wantEq)
		}
		if a.Equal(new(Element).Set(a)) != 1 {
			t.Fatal("Equal(a, a) != 1")
		}
		wantNeg := int(x.Bit(0))
		if got := a.IsNegative(); got != wantNeg {
			t.Fatalf("IsNegative(%x) = %d, want %d", ab, got, wantNeg)
		}
		abs := new(Element).Absolute(a)
		if abs.IsNegative() != 0 || (abs.Equal(a) != 1 && abs.Equal(new(Element).Negate(a)) != 1) {
			t.Fatalf("Absolute(%x) = %x", ab, abs.Bytes())
		}
	}
	// Inverting zero gives zero (used by encodings of the identity).
	if new(Element).Invert(new(Element).Zero()).Equal(new(Element).Zero()) != 1 {
		t.Error("Invert(0) != 0")
	}
}

func TestSelectSwap(t *testing.T) {
	a := mustElement(t, fromBig(big.NewInt(7)))
	b := mustElement(t, fromBig(big.NewInt(11)))
	if new(Element).Select(a, b, 1).Equal(a) != 1 || new(Element).Select(a, b, 0).Equal(b) != 1 {
		t.Error("Select picked the wrong operand")
	}
	x, y := new(Element).Set(a), new(Element).Set(b)
	x.Swap(y, 0)
	if x.Equal(a) != 1 || y.Equal(b) != 1 {
		t.Error("Swap(0) swapped")
	}
	x.Swap(y, 1)
	if x.Equal(b) != 1 || y.Equal(a) != 1 {
		t.Error("Swap(1) did not swap")
	}
}

// TestSqrtRatioRFC9496 uses the SQRT_RATIO_M1 vectors of RFC 9496 A.4.
func TestSqrtRatioRFC9496(t *testing.T) {
	var file struct {
		SqrtRatioM1 []struct {
			U, V, R   string
			WasSquare bool `json:"was_square"`
		} `json:"sqrt_ratio_m1"`
	}
	testvec.Load(t, "rfc9496_ristretto255.json", &file)
	if len(file.SqrtRatioM1) == 0 {
		t.Fatal("no vectors")
	}
	for i, v := range file.SqrtRatioM1 {
		u := mustElement(t, testvec.Hex(t, v.U))
		w := mustElement(t, testvec.Hex(t, v.V))
		r, wasSquare := new(Element).SqrtRatio(u, w)
		if (wasSquare == 1) != v.WasSquare || hex.EncodeToString(r.Bytes()) != v.R {
			t.Errorf("vector %d: SqrtRatio = %x, %d; want %s, %v", i, r.Bytes(), wasSquare, v.R, v.WasSquare)
		}
	}
}

// x25519 is the RFC 7748 section 5 Montgomery ladder, written only in terms
// of this package's operations, so the RFC 7748 vectors exercise the field.
func x25519(scalar, u []byte) []byte {
	k := bytes.Clone(scalar)
	k[0] &= 248
	k[31] &= 127
	k[31] |= 64

	x1, _ := new(Element).SetBytes(u)
	x2 := new(Element).One()
	z2 := new(Element).Zero()
	x3 := new(Element).Set(x1)
	z3 := new(Element).One()
	a24, _ := new(Element).SetBytes(append([]byte{0x42, 0xdb, 0x01}, make([]byte, 29)...)) // 121666

	swap := 0
	var a, aa, b, bb, e, c, d, da, cb Element
	for i := 254; i >= 0; i-- {
		bit := int(k[i/8]>>(i%8)) & 1
		swap ^= bit
		x2.Swap(x3, swap)
		z2.Swap(z3, swap)
		swap = bit

		a.Add(x2, z2)
		aa.Square(&a)
		b.Subtract(x2, z2)
		bb.Square(&b)
		e.Subtract(&aa, &bb)
		c.Add(x3, z3)
		d.Subtract(x3, z3)
		da.Multiply(&d, &a)
		cb.Multiply(&c, &b)
		x3.Add(&da, &cb)
		x3.Square(x3)
		z3.Subtract(&da, &cb)
		z3.Square(z3)
		z3.Multiply(z3, x1)
		x2.Multiply(&aa, &bb)
		z2.Multiply(a24, &e)
		z2.Add(z2, &bb) // RFC 7748 writes a24 = 121665 with aa; 121666 with bb is equivalent
		z2.Multiply(z2, &e)
	}
	x2.Swap(x3, swap)
	z2.Swap(z3, swap)
	return new(Element).Multiply(x2, new(Element).Invert(z2)).Bytes()
}

type rfc7748File struct {
	Vectors []struct {
		Scalar, U, Output string
	}
	Iterated []struct {
		Iterations int
		Output     string
	}
	ECDH struct {
		AlicePrivate string `json:"alice_private"`
		AlicePublic  string `json:"alice_public"`
		BobPrivate   string `json:"bob_private"`
		BobPublic    string `json:"bob_public"`
		Shared       string
	}
}

func TestX25519RFC7748(t *testing.T) {
	var f rfc7748File
	testvec.Load(t, "rfc7748_x25519.json", &f)
	h := func(s string) []byte { return testvec.Hex(t, s) }

	for i, v := range f.Vectors {
		if got := hex.EncodeToString(x25519(h(v.Scalar), h(v.U))); got != v.Output {
			t.Errorf("section 5.2 vector %d: got %s, want %s", i, got, v.Output)
		}
	}

	nine := make([]byte, 32)
	nine[0] = 9
	for _, it := range f.Iterated {
		if it.Iterations > 1000 {
			continue // 1,000,000 iterations take minutes; covered by the 1,000 case
		}
		k, u := bytes.Clone(nine), bytes.Clone(nine)
		for range it.Iterations {
			k, u = x25519(k, u), k
		}
		if got := hex.EncodeToString(k); got != it.Output {
			t.Errorf("after %d iterations: got %s, want %s", it.Iterations, got, it.Output)
		}
	}

	e := f.ECDH
	for _, c := range []struct{ name, got, want string }{
		{"alice public", hex.EncodeToString(x25519(h(e.AlicePrivate), nine)), e.AlicePublic},
		{"bob public", hex.EncodeToString(x25519(h(e.BobPrivate), nine)), e.BobPublic},
		{"shared (alice)", hex.EncodeToString(x25519(h(e.AlicePrivate), h(e.BobPublic))), e.Shared},
		{"shared (bob)", hex.EncodeToString(x25519(h(e.BobPrivate), h(e.AlicePublic))), e.Shared},
	} {
		if c.got != c.want {
			t.Errorf("section 6.1 %s: got %s, want %s", c.name, c.got, c.want)
		}
	}
}

// TestX25519MatchesCryptoECDH cross-checks the ladder against crypto/ecdh.
func TestX25519MatchesCryptoECDH(t *testing.T) {
	r := rand.New(rand.NewPCG(5, 6))
	for range 200 {
		seed := make([]byte, 32)
		for i := range seed {
			seed[i] = byte(r.Uint32())
		}
		priv, err := ecdh.X25519().NewPrivateKey(seed)
		if err != nil {
			t.Fatal(err)
		}
		u := randBytes(r)
		u[31] &= 0x7f
		peer, err := ecdh.X25519().NewPublicKey(u)
		if err != nil {
			t.Fatal(err)
		}
		want, err := priv.ECDH(peer)
		if err != nil {
			continue // low-order point: crypto/ecdh rejects the all-zero result
		}
		if got := x25519(seed, u); !bytes.Equal(got, want) {
			t.Fatalf("x25519(%x, %x) = %x, crypto/ecdh says %x", seed, u, got, want)
		}
	}
}

func TestSqrtM1(t *testing.T) {
	s := SqrtM1()
	minusOne := new(Element).Negate(new(Element).One())
	if new(Element).Square(s).Equal(minusOne) != 1 {
		t.Error("SqrtM1^2 != -1")
	}
	if s.IsNegative() != 0 {
		t.Error("SqrtM1 is negative; RFC 9496 uses the non-negative root")
	}
}
