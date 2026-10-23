// Package kyber implements CRYSTALS-Kyber1024 as submitted to round 3 of
// the NIST PQC process (the non-90s variant), which libsignal v0.99.1 uses
// for PQXDH prekeys (key type 0x08; see spec/crypto/kem.md). It is not
// FIPS 203 ML-KEM: Go's crypto/mlkem cannot produce or consume these keys.
//
// The algorithm follows draft-cfrg-schwabe-kyber-04, which matches the
// round-3 specification v3.02. The core is parameterized and also has a
// FIPS 203 variant, which differs in exactly four places (key-generation
// seed domain separation, no hashing of the encapsulation randomness, no
// final KDF, and the implicit-rejection value). That variant is unexported
// and exists so tests can check the shared core against crypto/mlkem.
//
// Constant time: secret-dependent code has no branches or secret-indexed
// memory accesses. Reductions mod q are divisions by the constant 3329,
// which the Go compiler turns into multiply-and-shift (the KyberSlash
// concern is division by a variable divisor). Decapsulation selects the
// result with crypto/subtle. Uniform sampling of the public matrix is
// variable-time, which is fine because its seed is public.
//
// Encoded coefficients of 4096 > c >= q in a public or secret key are read
// mod q. Round-3 Kyber defines no check for them and libsignal does none.
// Honest keys never contain them.
package kyber

import (
	"crypto/sha3"
	"crypto/subtle"
	"errors"
	"fmt"
	"io"
)

const (
	n = 256
	q = 3329

	// SharedKeySize is the size of a shared secret.
	SharedKeySize = 32
	// SeedSize is the size of the key-generation input d || z.
	SeedSize = 64

	// Sizes of Kyber1024 objects, without libsignal's type byte.
	EncapsulationKeySize1024 = 1568
	DecapsulationKeySize1024 = 3168
	CiphertextSize1024       = 1568
)

// params is a parameter set. Only eta1 = eta2 = 2 is supported, which
// covers Kyber768/1024 and ML-KEM-768/1024.
type params struct {
	k, du, dv int
}

var (
	params1024 = &params{k: 4, du: 11, dv: 5}
	params768  = &params{k: 3, du: 10, dv: 4}
)

func (p *params) cpaSKSize() int { return 384 * p.k }
func (p *params) pkSize() int    { return 384*p.k + 32 }
func (p *params) ctSize() int    { return 32 * (p.du*p.k + p.dv) }
func (p *params) dkSize() int    { return p.cpaSKSize() + p.pkSize() + 64 }

type variant int

const (
	kyberVariant variant = iota // round-3 Kyber
	mlkemVariant                // FIPS 203 ML-KEM
)

// poly is an element of Rq = GF(q)[x]/(x^256+1), coefficients in [0, q).
type poly [n]uint16

func addq(a, b uint16) uint16 { return uint16((uint32(a) + uint32(b)) % q) }
func subq(a, b uint16) uint16 { return uint16((uint32(a) + q - uint32(b)) % q) }
func mulq(a, b uint16) uint16 { return uint16(uint32(a) * uint32(b) % q) }

// zetas[i] = 17^brv7(i) and gammas[i] = 17^(2*brv7(i)+1), mod q.
var zetas, gammas [128]uint16

func init() {
	pow := func(e int) uint16 {
		r := uint16(1)
		for range e {
			r = mulq(r, 17)
		}
		return r
	}
	for i := range 128 {
		b := 0
		for j := range 7 {
			b |= (i >> j & 1) << (6 - j)
		}
		zetas[i] = pow(b)
		gammas[i] = pow(2*b + 1)
	}
}

// ntt maps f into the NTT domain in place (draft section 5.1.3), using
// Cooley-Tukey butterflies with zeta^brv(1), zeta^brv(2), ... in order.
func (f *poly) ntt() {
	k := 1
	for l := 128; l >= 2; l >>= 1 {
		for start := 0; start < n; start += 2 * l {
			z := zetas[k]
			k++
			for j := start; j < start+l; j++ {
				t := mulq(z, f[j+l])
				f[j+l] = subq(f[j], t)
				f[j] = addq(f[j], t)
			}
		}
	}
}

// invNTT inverts ntt in place, with Gentleman-Sande butterflies and the
// factors of 1/2 collected into one multiplication by 128^-1 = 3303.
func (f *poly) invNTT() {
	k := 127
	for l := 2; l <= 128; l <<= 1 {
		for start := 0; start < n; start += 2 * l {
			z := zetas[k]
			k--
			for j := start; j < start+l; j++ {
				t := f[j]
				f[j] = addq(t, f[j+l])
				f[j+l] = mulq(z, subq(f[j+l], t))
			}
		}
	}
	for i := range f {
		f[i] = mulq(f[i], 3303)
	}
}

// mulAddNTT adds a o b (NTT-domain product, draft section 5.1.3.3) to f.
func (f *poly) mulAddNTT(a, b *poly) {
	for i := 0; i < n; i += 2 {
		a0, a1, b0, b1 := a[i], a[i+1], b[i], b[i+1]
		c0 := addq(mulq(a0, b0), mulq(gammas[i/2], mulq(a1, b1)))
		c1 := addq(mulq(a0, b1), mulq(a1, b0))
		f[i] = addq(f[i], c0)
		f[i+1] = addq(f[i+1], c1)
	}
}

func (f *poly) add(a *poly) {
	for i := range f {
		f[i] = addq(f[i], a[i])
	}
}

// sampleUniform fills f by rejection sampling 12-bit values from an XOF
// stream (draft section 7.1).
func (f *poly) sampleUniform(r io.Reader) {
	var buf [168]byte // the SHAKE-128 rate, a multiple of 3
	i := 0
	for {
		if _, err := io.ReadFull(r, buf[:]); err != nil {
			panic(err) // a SHAKE stream never ends
		}
		for j := 0; j < len(buf); j += 3 {
			d1 := uint16(buf[j]) | uint16(buf[j+1]&0x0f)<<8
			d2 := uint16(buf[j+1]>>4) | uint16(buf[j+2])<<4
			for _, d := range [2]uint16{d1, d2} {
				if d < q {
					f[i] = d
					i++
					if i == n {
						return
					}
				}
			}
		}
	}
}

// cbd2 sets f to CBD(b, 2) for 128 bytes b (draft section 7.2): each
// coefficient is (bit0 + bit1) - (bit2 + bit3) of a 4-bit group.
func (f *poly) cbd2(b []byte) {
	for i := range n {
		x := b[i/2] >> (4 * (i % 2))
		a := uint16(x&1 + x>>1&1)
		c := uint16(x>>2&1 + x>>3&1)
		f[i] = (a + q - c) % q
	}
}

// encode appends the 256 coefficients of f packed w bits each, little-endian.
func (f *poly) encode(dst []byte, w int) []byte {
	var acc uint64
	bits := 0
	for _, c := range f {
		acc |= uint64(c) << bits
		bits += w
		for bits >= 8 {
			dst = append(dst, byte(acc))
			acc >>= 8
			bits -= 8
		}
	}
	return dst
}

// decode sets f from 32*w bytes of w-bit values, reduced mod q.
func (f *poly) decode(b []byte, w int) {
	var acc uint64
	bits, j := 0, 0
	mask := uint64(1)<<w - 1
	for i := range f {
		for bits < w {
			acc |= uint64(b[j]) << bits
			j++
			bits += 8
		}
		f[i] = uint16((acc & mask) % q)
		acc >>= w
		bits -= w
	}
}

func compress(x uint16, d int) uint16 {
	return uint16((uint32(x)<<d+q/2)/q) & (1<<d - 1)
}

func decompress(y uint16, d int) uint16 {
	return uint16((uint32(y)*q + 1<<(d-1)) >> d)
}

func (f *poly) compress(d int) {
	for i := range f {
		f[i] = compress(f[i], d)
	}
}

func (f *poly) decompress(d int) {
	for i := range f {
		f[i] = decompress(f[i], d)
	}
}

// Symmetric primitives (draft section 6).

func hashH(b ...[]byte) []byte {
	h := sha3.New256()
	for _, x := range b {
		h.Write(x)
	}
	return h.Sum(nil)
}

func hashG(b ...[]byte) (first, second []byte) {
	h := sha3.New512()
	for _, x := range b {
		h.Write(x)
	}
	out := h.Sum(nil)
	return out[:32], out[32:]
}

func shake256(outLen int, b ...[]byte) []byte {
	h := sha3.NewSHAKE256()
	for _, x := range b {
		h.Write(x)
	}
	out := make([]byte, outLen)
	h.Read(out)
	return out
}

// noise returns CBD(PRF(seed, nonce), 2).
func noise(seed []byte, nonce byte) poly {
	var f poly
	f.cbd2(shake256(128, seed, []byte{nonce}))
	return f
}

// matrix returns AHat with AHat[i][j] = sampleUniform(XOF(rho || j || i)).
func matrix(p *params, rho []byte) [][]poly {
	a := make([][]poly, p.k)
	for i := range a {
		a[i] = make([]poly, p.k)
		for j := range a[i] {
			x := sha3.NewSHAKE128()
			x.Write(rho)
			x.Write([]byte{byte(j), byte(i)})
			a[i][j].sampleUniform(x)
		}
	}
	return a
}

// innerKeyGen is draft section 10.2 (FIPS 203 K-PKE.KeyGen for mlkem).
func innerKeyGen(p *params, v variant, d []byte) (pk, sk []byte) {
	var rho, sigma []byte
	if v == mlkemVariant {
		rho, sigma = hashG(d, []byte{byte(p.k)})
	} else {
		rho, sigma = hashG(d)
	}
	a := matrix(p, rho)
	s := make([]poly, p.k)
	for i := range s {
		s[i] = noise(sigma, byte(i))
		s[i].ntt()
	}
	for i := range p.k {
		t := noise(sigma, byte(p.k+i))
		t.ntt()
		for j := range p.k {
			t.mulAddNTT(&a[i][j], &s[j])
		}
		pk = t.encode(pk, 12)
	}
	pk = append(pk, rho...)
	for i := range s {
		sk = s[i].encode(sk, 12)
	}
	return pk, sk
}

// innerEnc is draft section 10.3.
func innerEnc(p *params, pk, m, coins []byte) []byte {
	t := make([]poly, p.k)
	for i := range t {
		t[i].decode(pk[384*i:], 12)
	}
	a := matrix(p, pk[384*p.k:])
	r := make([]poly, p.k)
	for i := range r {
		r[i] = noise(coins, byte(i))
		r[i].ntt()
	}

	var ct []byte
	for i := range p.k {
		var u poly
		for j := range p.k {
			u.mulAddNTT(&a[j][i], &r[j]) // AHat transposed
		}
		u.invNTT()
		e1 := noise(coins, byte(p.k+i))
		u.add(&e1)
		u.compress(p.du)
		ct = u.encode(ct, p.du)
	}

	var vp poly
	for j := range p.k {
		vp.mulAddNTT(&t[j], &r[j])
	}
	vp.invNTT()
	e2 := noise(coins, byte(2*p.k))
	vp.add(&e2)
	var mp poly
	mp.decode(m, 1)
	mp.decompress(1)
	vp.add(&mp)
	vp.compress(p.dv)
	return vp.encode(ct, p.dv)
}

// innerDec is draft section 10.4.
func innerDec(p *params, sk, ct []byte) []byte {
	var w poly
	for i := range p.k {
		var u, s poly
		u.decode(ct[32*p.du*i:], p.du)
		u.decompress(p.du)
		u.ntt()
		s.decode(sk[384*i:], 12)
		w.mulAddNTT(&s, &u)
	}
	w.invNTT()
	var vp poly
	vp.decode(ct[32*p.du*p.k:], p.dv)
	vp.decompress(p.dv)
	for i := range vp {
		vp[i] = subq(vp[i], w[i])
	}
	vp.compress(1)
	return vp.encode(nil, 1)
}

// keyGen is draft section 11.1 (FIPS 203 ML-KEM.KeyGen_internal for mlkem).
// It returns the encapsulation key and the expanded decapsulation key
// cpaSK || pk || H(pk) || z.
func keyGen(p *params, v variant, d, z []byte) (ek, dk []byte) {
	pk, sk := innerKeyGen(p, v, d)
	dk = append(sk, pk...)
	dk = append(dk, hashH(pk)...)
	dk = append(dk, z...)
	return pk, dk
}

// encaps is draft section 11.2, with the 32 random bytes passed in.
func encaps(p *params, v variant, ek, random []byte) (ct, ss []byte) {
	m := random
	if v == kyberVariant {
		m = hashH(random)
	}
	kbar, coins := hashG(m, hashH(ek))
	ct = innerEnc(p, ek, m, coins)
	if v == mlkemVariant {
		return ct, kbar
	}
	return ct, shake256(32, kbar, hashH(ct))
}

// decaps is draft section 11.3. ct must have the right length.
func decaps(p *params, v variant, dk, ct []byte) []byte {
	skc := dk[:p.cpaSKSize()]
	pk := dk[p.cpaSKSize() : p.cpaSKSize()+p.pkSize()]
	h := dk[p.cpaSKSize()+p.pkSize() : p.dkSize()-32]
	z := dk[p.dkSize()-32:]

	m := innerDec(p, skc, ct)
	kbar, coins := hashG(m, h)
	ct2 := innerEnc(p, pk, m, coins)

	var good, reject []byte
	if v == mlkemVariant {
		good, reject = kbar, shake256(32, z, ct)
	} else {
		hc := hashH(ct)
		good, reject = shake256(32, kbar, hc), shake256(32, z, hc)
	}
	subtle.ConstantTimeCopy(subtle.ConstantTimeCompare(ct, ct2), reject, good)
	return reject
}

// EncapsulationKey1024 is a Kyber1024 public key.
type EncapsulationKey1024 struct {
	b []byte
}

// NewEncapsulationKey1024 parses a raw 1568-byte public key. As in
// libsignal, only the length is checked.
func NewEncapsulationKey1024(b []byte) (*EncapsulationKey1024, error) {
	if len(b) != EncapsulationKeySize1024 {
		return nil, fmt.Errorf("kyber: public key is %d bytes, want %d", len(b), EncapsulationKeySize1024)
	}
	return &EncapsulationKey1024{append([]byte(nil), b...)}, nil
}

// Bytes returns the raw 1568-byte public key.
func (k *EncapsulationKey1024) Bytes() []byte { return append([]byte(nil), k.b...) }

// Encapsulate returns a ciphertext and shared secret, reading 32 random
// bytes from rand.
func (k *EncapsulationKey1024) Encapsulate(rand io.Reader) (ciphertext, sharedKey []byte, err error) {
	var r [32]byte
	if _, err := io.ReadFull(rand, r[:]); err != nil {
		return nil, nil, err
	}
	ct, ss := k.encapsulateWith(r[:])
	return ct, ss, nil
}

func (k *EncapsulationKey1024) encapsulateWith(random []byte) ([]byte, []byte) {
	return encaps(params1024, kyberVariant, k.b, random)
}

// DecapsulationKey1024 is a Kyber1024 secret key in the expanded 3168-byte
// form libsignal stores (cpaSK || pk || H(pk) || z).
type DecapsulationKey1024 struct {
	b []byte
}

// GenerateKey1024 returns a new key pair. Like the round-3 reference and
// libsignal, it reads d and then z, 32 bytes each.
func GenerateKey1024(rand io.Reader) (*DecapsulationKey1024, error) {
	var seed [SeedSize]byte
	if _, err := io.ReadFull(rand, seed[:32]); err != nil {
		return nil, err
	}
	if _, err := io.ReadFull(rand, seed[32:]); err != nil {
		return nil, err
	}
	return NewDecapsulationKey1024FromSeed(seed[:])
}

// NewDecapsulationKey1024FromSeed derives a key pair from the 64-byte
// d || z.
func NewDecapsulationKey1024FromSeed(seed []byte) (*DecapsulationKey1024, error) {
	if len(seed) != SeedSize {
		return nil, fmt.Errorf("kyber: seed is %d bytes, want %d", len(seed), SeedSize)
	}
	_, dk := keyGen(params1024, kyberVariant, seed[:32], seed[32:])
	return &DecapsulationKey1024{dk}, nil
}

// NewDecapsulationKey1024 parses an expanded 3168-byte secret key. As in
// libsignal, only the length is checked.
func NewDecapsulationKey1024(b []byte) (*DecapsulationKey1024, error) {
	if len(b) != DecapsulationKeySize1024 {
		return nil, fmt.Errorf("kyber: secret key is %d bytes, want %d", len(b), DecapsulationKeySize1024)
	}
	return &DecapsulationKey1024{append([]byte(nil), b...)}, nil
}

// Bytes returns the expanded 3168-byte secret key.
func (k *DecapsulationKey1024) Bytes() []byte { return append([]byte(nil), k.b...) }

// EncapsulationKey returns the public key embedded in k.
func (k *DecapsulationKey1024) EncapsulationKey() *EncapsulationKey1024 {
	p := params1024
	return &EncapsulationKey1024{append([]byte(nil), k.b[p.cpaSKSize():p.cpaSKSize()+p.pkSize()]...)}
}

// Decapsulate returns the shared secret for ciphertext. An invalid
// ciphertext of the right length yields a pseudorandom value, not an
// error (implicit rejection).
func (k *DecapsulationKey1024) Decapsulate(ciphertext []byte) ([]byte, error) {
	if len(ciphertext) != CiphertextSize1024 {
		return nil, errors.New("kyber: invalid ciphertext length")
	}
	return decaps(params1024, kyberVariant, k.b, ciphertext), nil
}
