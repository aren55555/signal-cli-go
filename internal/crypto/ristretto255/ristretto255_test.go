package ristretto255

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/aren55555/signal-cli-go/internal/crypto/edwards25519"
	"github.com/aren55555/signal-cli-go/internal/crypto/edwards25519/field"
	"github.com/aren55555/signal-cli-go/internal/testvec"
)

type rfc9496 struct {
	GeneratorMultiples []string `json:"generator_multiples"`
	InvalidEncodings   []struct {
		Reason   string
		Encoding string
	} `json:"invalid_encodings"`
	FromUniformBytes []struct {
		Input  string
		Output string
	} `json:"from_uniform_bytes"`
}

func load(t *testing.T) rfc9496 {
	var f rfc9496
	testvec.Load(t, "rfc9496_ristretto255.json", &f)
	return f
}

// TestConstants checks each RFC 9496 section 4.1 constant against its
// defining identity, with a = -1.
func TestConstants(t *testing.T) {
	one := new(field.Element).One()
	minusOne := new(field.Element).Negate(one)
	sq := func(x *field.Element) *field.Element { return new(field.Element).Square(x) }
	mul := func(x, y *field.Element) *field.Element { return new(field.Element).Multiply(x, y) }
	sub := func(x, y *field.Element) *field.Element { return new(field.Element).Subtract(x, y) }

	// d = -121665/121666
	k := func(n uint32) *field.Element {
		b := make([]byte, 32)
		b[0], b[1], b[2] = byte(n), byte(n>>8), byte(n>>16)
		e, _ := new(field.Element).SetBytes(b)
		return e
	}
	if mul(constD, k(121666)).Equal(new(field.Element).Negate(k(121665))) != 1 {
		t.Error("D * 121666 != -121665")
	}
	if constSqrtM1.Equal(field.SqrtM1()) != 1 {
		t.Error("SQRT_M1 differs from field.SqrtM1")
	}
	// SQRT_AD_MINUS_ONE^2 = a*d - 1 = -d - 1
	if sq(constSqrtADMinusOne).Equal(sub(new(field.Element).Negate(constD), one)) != 1 {
		t.Error("SQRT_AD_MINUS_ONE^2 != ad - 1")
	}
	// INVSQRT_A_MINUS_D^2 * (a - d) = 1
	if mul(sq(constInvSqrtAMinusD), sub(minusOne, constD)).Equal(one) != 1 {
		t.Error("INVSQRT_A_MINUS_D^2 * (a-d) != 1")
	}
	if constOneMinusDSq.Equal(sub(one, sq(constD))) != 1 {
		t.Error("ONE_MINUS_D_SQ != 1 - d^2")
	}
	if constDMinusOneSq.Equal(sq(sub(constD, one))) != 1 {
		t.Error("D_MINUS_ONE_SQ != (d-1)^2")
	}
}

// TestGeneratorMultiples uses RFC 9496 A.1: B[i] = i*G, built by adding G.
func TestGeneratorMultiples(t *testing.T) {
	f := load(t)
	if len(f.GeneratorMultiples) != 16 {
		t.Fatalf("got %d multiples", len(f.GeneratorMultiples))
	}
	G := NewGeneratorElement()
	acc := NewIdentityElement()
	for i, want := range f.GeneratorMultiples {
		if got := hex.EncodeToString(acc.Bytes()); got != want {
			t.Errorf("B[%d] by addition = %s, want %s", i, got, want)
		}
		sb := make([]byte, 32)
		sb[0] = byte(i)
		s, err := edwards25519.NewScalar().SetCanonicalBytes(sb)
		if err != nil {
			t.Fatal(err)
		}
		if got := hex.EncodeToString(NewIdentityElement().ScalarBaseMult(s).Bytes()); got != want {
			t.Errorf("B[%d] by ScalarBaseMult = %s, want %s", i, got, want)
		}
		if got := hex.EncodeToString(NewIdentityElement().ScalarMult(s, G).Bytes()); got != want {
			t.Errorf("B[%d] by ScalarMult = %s, want %s", i, got, want)
		}
		dec, err := NewIdentityElement().SetCanonicalBytes(testvec.Hex(t, want))
		if err != nil {
			t.Errorf("B[%d] does not decode: %v", i, err)
		} else if dec.Equal(acc) != 1 || !bytes.Equal(dec.Bytes(), acc.Bytes()) {
			t.Errorf("B[%d] decodes to a different element", i)
		}
		if i > 0 {
			// Subtract and Negate: B[i] - G = B[i-1], and B[i] + (-B[i]) = 0.
			prev := NewIdentityElement().Subtract(acc, G)
			if got := hex.EncodeToString(prev.Bytes()); got != f.GeneratorMultiples[i-1] {
				t.Errorf("B[%d] - G = %s", i, got)
			}
			zero := NewIdentityElement().Add(acc, NewIdentityElement().Negate(acc))
			if zero.Equal(NewIdentityElement()) != 1 {
				t.Errorf("B[%d] + (-B[%d]) != identity", i, i)
			}
		}
		acc.Add(acc, G)
	}
}

// TestInvalidEncodings uses RFC 9496 A.2.
func TestInvalidEncodings(t *testing.T) {
	f := load(t)
	if len(f.InvalidEncodings) != 29 {
		t.Fatalf("got %d invalid encodings", len(f.InvalidEncodings))
	}
	for _, v := range f.InvalidEncodings {
		if _, err := NewIdentityElement().SetCanonicalBytes(testvec.Hex(t, v.Encoding)); err == nil {
			t.Errorf("%s: %s accepted", v.Reason, v.Encoding)
		}
	}
	if _, err := NewIdentityElement().SetCanonicalBytes(make([]byte, 31)); err == nil {
		t.Error("31 bytes accepted")
	}
}

// TestFromUniformBytes uses RFC 9496 A.3.
func TestFromUniformBytes(t *testing.T) {
	f := load(t)
	if len(f.FromUniformBytes) != 11 {
		t.Fatalf("got %d vectors", len(f.FromUniformBytes))
	}
	for i, v := range f.FromUniformBytes {
		e, err := NewIdentityElement().SetUniformBytes(testvec.Hex(t, v.Input))
		if err != nil {
			t.Fatal(err)
		}
		if got := hex.EncodeToString(e.Bytes()); got != v.Output {
			t.Errorf("vector %d: got %s, want %s", i, got, v.Output)
		}
	}
	if _, err := NewIdentityElement().SetUniformBytes(make([]byte, 32)); err == nil {
		t.Error("32 bytes accepted")
	}
}

// FuzzSetCanonicalBytes checks that an accepted encoding re-encodes to
// the same bytes, as RFC 9496 section 4.3.2 requires.
func FuzzSetCanonicalBytes(f *testing.F) {
	f.Add(NewGeneratorElement().Bytes())
	f.Add(make([]byte, 32))
	f.Fuzz(func(t *testing.T, b []byte) {
		e, err := NewIdentityElement().SetCanonicalBytes(b)
		if err != nil {
			return
		}
		if got := e.Bytes(); !bytes.Equal(got, b) {
			t.Fatalf("%x re-encodes as %x", b, got)
		}
	})
}
