package edwards25519

import (
	"bytes"
	"math/big"
	"math/rand/v2"
	"testing"
)

// order is the prime order ℓ = 2^252 + 27742317777372353535851937790883648493.
var order, _ = new(big.Int).SetString("7237005577332262213973186563042994240857116359379907606001950938285454250989", 10)

func leToBig(b []byte) *big.Int {
	r := bytes.Clone(b)
	for i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 {
		r[i], r[j] = r[j], r[i]
	}
	return new(big.Int).SetBytes(r)
}

func bigToLE(x *big.Int) []byte {
	b := x.FillBytes(make([]byte, 32))
	for i, j := 0, 31; i < j; i, j = i+1, j-1 {
		b[i], b[j] = b[j], b[i]
	}
	return b
}

func randScalarBytes(r *rand.Rand) []byte {
	switch r.IntN(4) {
	case 0:
		return bigToLE(big.NewInt(int64(r.IntN(5))))
	case 1:
		return bigToLE(new(big.Int).Sub(order, big.NewInt(int64(r.IntN(5)+1))))
	}
	b := make([]byte, 40)
	for i := range b {
		b[i] = byte(r.Uint32())
	}
	return bigToLE(new(big.Int).Mod(new(big.Int).SetBytes(b), order))
}

func mustScalar(t *testing.T, b []byte) *Scalar {
	t.Helper()
	s, err := NewScalar().SetCanonicalBytes(b)
	if err != nil {
		t.Fatalf("SetCanonicalBytes(%x): %v", b, err)
	}
	return s
}

func TestScalarCanonical(t *testing.T) {
	if _, err := NewScalar().SetCanonicalBytes(bigToLE(order)); err == nil {
		t.Error("accepted ℓ")
	}
	if _, err := NewScalar().SetCanonicalBytes(bigToLE(new(big.Int).Add(order, big.NewInt(1)))); err == nil {
		t.Error("accepted ℓ+1")
	}
	max := bytes.Repeat([]byte{0xff}, 32)
	if _, err := NewScalar().SetCanonicalBytes(max); err == nil {
		t.Error("accepted 2^256-1")
	}
	if _, err := NewScalar().SetCanonicalBytes(make([]byte, 31)); err == nil {
		t.Error("accepted 31 bytes")
	}
	lm1 := bigToLE(new(big.Int).Sub(order, big.NewInt(1)))
	if got := mustScalar(t, lm1).Bytes(); !bytes.Equal(got, lm1) {
		t.Errorf("ℓ-1 round trip: %x", got)
	}
}

func TestScalarMatchesBig(t *testing.T) {
	r := rand.New(rand.NewPCG(7, 8))
	for range 2000 {
		ab, bb, cb := randScalarBytes(r), randScalarBytes(r), randScalarBytes(r)
		a, b, c := mustScalar(t, ab), mustScalar(t, bb), mustScalar(t, cb)
		x, y, z := leToBig(ab), leToBig(bb), leToBig(cb)

		check := func(name string, got *Scalar, want *big.Int) {
			t.Helper()
			if w := bigToLE(new(big.Int).Mod(want, order)); !bytes.Equal(got.Bytes(), w) {
				t.Fatalf("%s(%x, %x) = %x, want %x", name, ab, bb, got.Bytes(), w)
			}
		}
		check("Add", NewScalar().Add(a, b), new(big.Int).Add(x, y))
		check("Subtract", NewScalar().Subtract(a, b), new(big.Int).Sub(x, y))
		check("Negate", NewScalar().Negate(a), new(big.Int).Neg(x))
		check("Multiply", NewScalar().Multiply(a, b), new(big.Int).Mul(x, y))
		check("MultiplyAdd", NewScalar().MultiplyAdd(a, b, c), new(big.Int).Add(new(big.Int).Mul(x, y), z))
		s := NewScalar().Set(a)
		check("Multiply(aliased)", s.Multiply(s, s), new(big.Int).Mul(x, x))

		wantEq := 0
		if x.Cmp(y) == 0 {
			wantEq = 1
		}
		if a.Equal(b) != wantEq || a.Equal(NewScalar().Set(a)) != 1 {
			t.Fatalf("Equal(%x, %x) wrong", ab, bb)
		}

		wide := make([]byte, 64)
		for i := range wide {
			wide[i] = byte(r.Uint32())
		}
		if r.IntN(8) == 0 {
			wide = bytes.Repeat([]byte{0xff}, 64)
		}
		u, err := NewScalar().SetUniformBytes(wide)
		if err != nil {
			t.Fatal(err)
		}
		check("SetUniformBytes", u, leToBig(wide))

		clamped := bytes.Clone(wide[:32])
		clamped[0] &= 248
		clamped[31] &= 127
		clamped[31] |= 64
		k, err := NewScalar().SetBytesWithClamping(wide[:32])
		if err != nil {
			t.Fatal(err)
		}
		check("SetBytesWithClamping", k, leToBig(clamped))
	}
	if _, err := NewScalar().SetUniformBytes(make([]byte, 32)); err == nil {
		t.Error("SetUniformBytes accepted 32 bytes")
	}
}
