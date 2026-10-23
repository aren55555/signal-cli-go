package kyber

import (
	"bytes"
	"errors"
	"fmt"
)

// FIPS 203 ML-KEM-768, as SPQR uses it (spec/protocol/spqr/
// mlkem-incremental.md). Key generation and decapsulation are standard;
// encapsulation can run in two parts, because SPQR sends the ciphertext's
// c1 before it has received the encapsulation key's t-hat. The keys are
// the expanded FIPS 203 forms that libsignal stores.

// Sizes of ML-KEM-768 objects and of the pieces SPQR sends separately.
const (
	EncapsulationKeySizeMLKEM768 = 1184
	DecapsulationKeySizeMLKEM768 = 2400
	CiphertextSizeMLKEM768       = 1088
	HeaderSizeMLKEM768           = 64   // rho || H(ek)
	EKPayloadSizeMLKEM768        = 1152 // ByteEncode12(t-hat), the first part of ek
	Ct1SizeMLKEM768              = 960  // c1
	Ct2SizeMLKEM768              = 128  // c2
)

// ErrHeaderMismatch reports an encapsulation-key payload that does not
// hash to the header's H(ek), or that fails the FIPS 203 modulus check.
var ErrHeaderMismatch = errors.New("kyber: encapsulation key does not match its header")

func checkLen(b []byte, n int, what string) error {
	if len(b) != n {
		return fmt.Errorf("kyber: %s is %d bytes, want %d", what, len(b), n)
	}
	return nil
}

// GenerateKeyMLKEM768 is ML-KEM.KeyGen_internal(d, z) for the 64-byte
// seed d || z. It returns the 1184-byte encapsulation key and the
// 2400-byte expanded decapsulation key.
func GenerateKeyMLKEM768(seed []byte) (ek, dk []byte, err error) {
	if err := checkLen(seed, SeedSize, "seed"); err != nil {
		return nil, nil, err
	}
	ek, dk = keyGen(params768, mlkemVariant, seed[:32], seed[32:])
	return ek, dk, nil
}

// EncapsulateMLKEM768 is ML-KEM.Encaps_internal(ek, m) with the 32-byte
// message m supplied by the caller.
func EncapsulateMLKEM768(ek, m []byte) (ciphertext, sharedKey []byte, err error) {
	if err := checkLen(ek, EncapsulationKeySizeMLKEM768, "encapsulation key"); err != nil {
		return nil, nil, err
	}
	if err := checkLen(m, 32, "message"); err != nil {
		return nil, nil, err
	}
	ct, ss := encaps(params768, mlkemVariant, ek, m)
	return ct, ss, nil
}

// DecapsulateMLKEM768 is ML-KEM.Decaps_internal with the expanded
// decapsulation key, including implicit rejection.
func DecapsulateMLKEM768(dk, ciphertext []byte) ([]byte, error) {
	if err := checkLen(dk, DecapsulationKeySizeMLKEM768, "decapsulation key"); err != nil {
		return nil, err
	}
	if err := checkLen(ciphertext, CiphertextSizeMLKEM768, "ciphertext"); err != nil {
		return nil, err
	}
	return decaps(params768, mlkemVariant, dk, ciphertext), nil
}

// IncrementalEncapsulation holds what the second part of a split
// encapsulation needs: the header and the message. The second part reruns
// the deterministic encapsulation on the full key (mlkem-incremental.md
// §3.3), which reproduces the c1 already sent.
type IncrementalEncapsulation struct {
	hdr, m, ct1 []byte
}

// StartEncapsulationMLKEM768 is the first part: from the header
// rho || H(ek) and the 32-byte message m it derives (K, r) = G(m || H(ek))
// and c1. The shared key K is final already.
func StartEncapsulationMLKEM768(hdr, m []byte) (enc *IncrementalEncapsulation, ct1, sharedKey []byte, err error) {
	if err := checkLen(hdr, HeaderSizeMLKEM768, "header"); err != nil {
		return nil, nil, nil, err
	}
	if err := checkLen(m, 32, "message"); err != nil {
		return nil, nil, nil, err
	}
	k, coins := hashG(m, hdr[32:])
	ct1, _ = innerEncU(params768, hdr[:32], coins)
	enc = &IncrementalEncapsulation{
		hdr: append([]byte(nil), hdr...),
		m:   append([]byte(nil), m...),
		ct1: ct1,
	}
	return enc, append([]byte(nil), ct1...), k, nil
}

// Finish is the second part: it checks the 1152-byte encapsulation-key
// payload against the header (hash and modulus check) and returns c2.
func (e *IncrementalEncapsulation) Finish(ekPayload []byte) (ct2 []byte, err error) {
	if err := checkLen(ekPayload, EKPayloadSizeMLKEM768, "encapsulation key payload"); err != nil {
		return nil, err
	}
	ek := append(append([]byte(nil), ekPayload...), e.hdr[:32]...)
	if !bytes.Equal(hashH(ek), e.hdr[32:]) || !encodingInRange(ekPayload) {
		return nil, ErrHeaderMismatch
	}
	ct, _ := encaps(params768, mlkemVariant, ek, e.m)
	if !bytes.Equal(ct[:Ct1SizeMLKEM768], e.ct1) {
		// Unreachable unless SHA3-256 collides; peer data never panics.
		return nil, ErrHeaderMismatch
	}
	return ct[Ct1SizeMLKEM768:], nil
}

// encodingInRange is the FIPS 203 modulus check: every 12-bit value in the
// encoding is below q.
func encodingInRange(b []byte) bool {
	for i := 0; i+3 <= len(b); i += 3 {
		d1 := uint16(b[i]) | uint16(b[i+1]&0x0f)<<8
		d2 := uint16(b[i+1]>>4) | uint16(b[i+2])<<4
		if d1 >= q || d2 >= q {
			return false
		}
	}
	return true
}
