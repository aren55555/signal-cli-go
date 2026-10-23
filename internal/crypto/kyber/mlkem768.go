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
	es          []byte // set when resumed from a libcrux state without the header
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
	if e.hdr == nil {
		// Resumed from y-hat and e2: there is no header to check against,
		// and the payload was checked when it arrived.
		if !encodingInRange(ekPayload) {
			return nil, ErrHeaderMismatch
		}
		return e.finishFromState(ekPayload), nil
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

// EncapsulationStateSizeMLKEM768 is the size of libcrux's stored
// encapsulation state: y-hat (3x256), e2 (256) as signed 16-bit
// little-endian words, then m (32) (mlkem-incremental.md §5).
const EncapsulationStateSizeMLKEM768 = 3*256*2 + 256*2 + 32

// ResumeEncapsulationMLKEM768 rebuilds a split encapsulation from stored
// state, for importing libsignal sessions. With the header it reruns the
// deterministic encapsulation from m (the state's last 32 bytes); without
// it (libsignal's EkReceivedCt1Sampled state) it uses y-hat and e2 from
// the state, after the issue 1275 repair. ct1 is the c1 already sent.
func ResumeEncapsulationMLKEM768(es, hdr, ct1 []byte) (*IncrementalEncapsulation, error) {
	if err := checkLen(es, EncapsulationStateSizeMLKEM768, "encapsulation state"); err != nil {
		return nil, err
	}
	if err := checkLen(ct1, Ct1SizeMLKEM768, "c1"); err != nil {
		return nil, err
	}
	e := &IncrementalEncapsulation{m: append([]byte(nil), es[2048:]...), ct1: append([]byte(nil), ct1...)}
	if hdr != nil {
		if err := checkLen(hdr, HeaderSizeMLKEM768, "header"); err != nil {
			return nil, err
		}
		e.hdr = append([]byte(nil), hdr...)
		return e, nil
	}
	e.es = repairIssue1275(append([]byte(nil), es...))
	return e, nil
}

// repairIssue1275 detects a state whose 16-bit words were written
// big-endian and swaps them back (mlkem-incremental.md §5). The first e2
// word that is neither 0 nor -1 decides.
func repairIssue1275(es []byte) []byte {
	for i := 1536; i < 2048; i += 2 {
		switch uint16(es[i]) | uint16(es[i+1])<<8 {
		case 0x0000, 0xffff:
			continue
		case 0x0001, 0x0002, 0xfffe:
			return es
		case 0x0100, 0x0200, 0xfeff:
			for j := 0; j < 2048; j += 2 {
				es[j], es[j+1] = es[j+1], es[j]
			}
			return es
		default:
			return es
		}
	}
	return es
}

func wordModQ(lo, hi byte) uint16 {
	v := int32(int16(uint16(lo)|uint16(hi)<<8)) % q
	if v < 0 {
		v += q
	}
	return uint16(v)
}

// finishFromState computes c2 from the stored y-hat, e2 and m and the
// t-hat payload.
func (e *IncrementalEncapsulation) finishFromState(ekPayload []byte) []byte {
	p := params768
	var t, r [3]poly
	for i := range 3 {
		t[i].decode(ekPayload[384*i:], 12)
		for j := range n {
			off := 512*i + 2*j
			r[i][j] = wordModQ(e.es[off], e.es[off+1])
		}
	}
	var vp, e2, mp poly
	for i := range 3 {
		vp.mulAddNTT(&t[i], &r[i])
	}
	vp.invNTT()
	for j := range n {
		e2[j] = wordModQ(e.es[1536+2*j], e.es[1537+2*j])
	}
	vp.add(&e2)
	mp.decode(e.m, 1)
	mp.decompress(1)
	vp.add(&mp)
	vp.compress(p.dv)
	return vp.encode(nil, p.dv)
}

// encapsulationState writes libcrux's state format for (hdr, m); tests use
// it to check ResumeEncapsulationMLKEM768.
func encapsulationState(hdr, m []byte) []byte {
	_, coins := hashG(m, hdr[32:])
	signed := func(c uint16) uint16 { // [0, q) to a signed 16-bit word
		if c > q/2 {
			return uint16(int16(int32(c) - q))
		}
		return c
	}
	var out []byte
	for i := range 3 {
		r := noise(coins, byte(i))
		r.ntt()
		for _, c := range r {
			out = append(out, byte(c), byte(c>>8))
		}
	}
	e2 := noise(coins, byte(6))
	for _, c := range e2 {
		w := signed(c)
		out = append(out, byte(w), byte(w>>8))
	}
	return append(out, m...)
}

// State returns the split encapsulation in libcrux's 2080-byte state
// format, for storing a session mid-epoch.
func (e *IncrementalEncapsulation) State() []byte {
	if e.es != nil {
		return append([]byte(nil), e.es...)
	}
	return encapsulationState(e.hdr, e.m)
}

// Header returns the stored header, or nil if the encapsulation was
// resumed without one.
func (e *IncrementalEncapsulation) Header() []byte { return e.hdr }

// Ct1 returns the c1 already produced.
func (e *IncrementalEncapsulation) Ct1() []byte { return e.ct1 }
