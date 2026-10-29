// SPDX-License-Identifier: GPL-3.0-or-later

package kyber

import (
	"bytes"
	"crypto/aes"
	"crypto/mlkem"
	"crypto/sha256"
	"crypto/sha3"
	"encoding/hex"
	"math/rand/v2"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/aren55555/signal-cli-go/internal/testvec"
)

// TestDraftExamples checks the worked examples in draft-cfrg-schwabe-kyber-04
// (sections 5.1.3.2, 7.1 and 7.2), which pin the NTT ordering, uniform
// sampling and CBD independently of the KATs.
func TestDraftExamples(t *testing.T) {
	var a poly
	a[0], a[1] = 1, 1
	a.ntt()
	for i, c := range a {
		if c != 1 {
			t.Fatalf("NTT(1,1,0,...)[%d] = %d, want 1", i, c)
		}
	}

	for i := range a {
		a[i] = uint16(i)
	}
	orig := a
	a.ntt()
	// The draft prints (2429, 2845, 425, 1865, ..., 2502, 2134, 2717, 2303).
	// The fourth value is an erratum: the draft's own matrix definition
	// gives 795 (TestNTTMatchesDefinition), and the NIST KATs and
	// crypto/mlkem agree with that. The other printed values are checked.
	head, tail := []uint16{2429, 2845, 425}, []uint16{2502, 2134, 2717, 2303}
	if !equal16(a[:3], head) || !equal16(a[252:], tail) {
		t.Errorf("NTT(0..255) = %v ... %v, want %v ... %v", a[:3], a[252:], head, tail)
	}
	a.invNTT()
	if a != orig {
		t.Error("InvNTT(NTT(x)) != x")
	}

	var u poly
	x := sha3.NewSHAKE128()
	u.sampleUniform(x)
	if h, tl := []uint16{3199, 697, 2212, 2302}, []uint16{255, 846, 1}; !equal16(u[:4], h) || !equal16(u[253:], tl) {
		t.Errorf("sampleUniform(SHAKE-128('')) = %v ... %v", u[:4], u[253:])
	}

	buf := make([]byte, 128)
	for i := range buf {
		buf[i] = byte(i)
	}
	var c poly
	c.cbd2(buf)
	const q1 = q - 1
	if h, tl := []uint16{0, 0, 1, 0, 1, 0}, []uint16{q1, 1, 0, 1}; !equal16(c[:6], h) || !equal16(c[252:], tl) {
		t.Errorf("CBD((0..127), 2) = %v ... %v", c[:6], c[252:])
	}
}

func equal16(a, b []uint16) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestMultiplyMatchesSchoolbook checks NTT-domain multiplication against
// the negacyclic schoolbook product of draft section 5.1.3.
func TestMultiplyMatchesSchoolbook(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	for range 20 {
		var a, b poly
		for i := range a {
			a[i], b[i] = uint16(r.IntN(q)), uint16(r.IntN(q))
		}
		var want poly
		for i := range 256 {
			for j := range 256 {
				p := uint32(a[i]) * uint32(b[j]) % q
				k := i + j
				if k >= 256 {
					k -= 256
					p = (q - p) % q // x^256 = -1
				}
				want[k] = uint16((uint32(want[k]) + p) % q)
			}
		}
		an, bn := a, b
		an.ntt()
		bn.ntt()
		var got poly
		got.mulAddNTT(&an, &bn)
		got.invNTT()
		if got != want {
			t.Fatal("InvNTT(NTT(a) o NTT(b)) != a*b")
		}
	}
}

// drbg is the AES-256 CTR_DRBG without derivation function that NIST's PQC
// KAT harness uses (SP 800-90A Rev. 1 section 10.2.1), as described in
// spec/crypto/kem.md.
type drbg struct {
	key [32]byte
	v   [16]byte
}

func newDRBG(seed []byte) *drbg {
	d := new(drbg)
	d.update(seed)
	return d
}

func (d *drbg) incV() {
	for i := 15; i >= 0; i-- {
		d.v[i]++
		if d.v[i] != 0 {
			break
		}
	}
}

func (d *drbg) update(provided []byte) {
	block, err := aes.NewCipher(d.key[:])
	if err != nil {
		panic(err)
	}
	var tmp [48]byte
	for i := 0; i < 3; i++ {
		d.incV()
		block.Encrypt(tmp[16*i:], d.v[:])
	}
	for i := range provided {
		tmp[i] ^= provided[i]
	}
	copy(d.key[:], tmp[:32])
	copy(d.v[:], tmp[32:])
}

func (d *drbg) generate(n int) []byte {
	block, err := aes.NewCipher(d.key[:])
	if err != nil {
		panic(err)
	}
	out := make([]byte, 0, n+16)
	var blk [16]byte
	for len(out) < n {
		d.incV()
		block.Encrypt(blk[:], d.v[:])
		out = append(out, blk[:]...)
	}
	d.update(nil)
	return out[:n]
}

// TestKyber1024KAT runs the first 10 NIST round-3 Kyber1024 KATs.
func TestKyber1024KAT(t *testing.T) {
	var f struct {
		Vectors []struct {
			Count        int
			Seed         string
			PublicKey    string `json:"public_key"`
			SecretKey    string `json:"secret_key"`
			Ciphertext   string
			SharedSecret string `json:"shared_secret"`
		}
	}
	testvec.Load(t, "kyber1024_kat.json", &f)
	if len(f.Vectors) != 10 {
		t.Fatalf("got %d KATs, want 10", len(f.Vectors))
	}
	for _, v := range f.Vectors {
		checkKAT(t, v.Count, testvec.Hex(t, v.Seed), v.PublicKey, v.SecretKey, v.Ciphertext, v.SharedSecret)
	}
}

// checkKAT runs one KAT entry: key generation from two 32-byte DRBG
// outputs (d, then z), encapsulation with a third, and decapsulation with
// the expanded secret key as libsignal stores it. Expected values are hex.
func checkKAT(t *testing.T, count int, seed []byte, wantPK, wantSK, wantCT, wantSS string) {
	t.Helper()
	rng := newDRBG(seed)
	d, z := rng.generate(32), rng.generate(32)
	dk, err := NewDecapsulationKey1024FromSeed(append(d, z...))
	if err != nil {
		t.Fatal(err)
	}
	if hex.EncodeToString(dk.EncapsulationKey().Bytes()) != strings.ToLower(wantPK) {
		t.Errorf("count %d: public key mismatch", count)
	}
	if hex.EncodeToString(dk.Bytes()) != strings.ToLower(wantSK) {
		t.Errorf("count %d: secret key mismatch", count)
	}
	ct, ss := dk.EncapsulationKey().encapsulateWith(rng.generate(32))
	if hex.EncodeToString(ct) != strings.ToLower(wantCT) {
		t.Errorf("count %d: ciphertext mismatch", count)
	}
	if hex.EncodeToString(ss) != strings.ToLower(wantSS) {
		t.Errorf("count %d: shared secret %x, want %s", count, ss, wantSS)
	}
	parsed, err := NewDecapsulationKey1024(testvec.Hex(t, strings.ToLower(wantSK)))
	if err != nil {
		t.Fatal(err)
	}
	got, err := parsed.Decapsulate(testvec.Hex(t, strings.ToLower(wantCT)))
	if err != nil {
		t.Fatal(err)
	}
	if hex.EncodeToString(got) != strings.ToLower(wantSS) {
		t.Errorf("count %d: decapsulated %x, want %s", count, got, wantSS)
	}
}

// fullKATSHA256 is the SHA-256 of PQCkemKAT_3168.rsp from the NIST
// round-3 Kyber package, as recorded in testdata/vectors/kyber1024_kat.json.
const fullKATSHA256 = "89248f2f33f7f4f7051729111f3049c409a933ec904aedadf035f30fa5646cd5"

// TestKyber1024FullKAT runs all 100 official KATs when KYBER1024_KAT_RSP
// names a copy of PQCkemKAT_3168.rsp (1.3 MB, not committed). The file is
// checked against its pinned SHA-256 first.
func TestKyber1024FullKAT(t *testing.T) {
	path := os.Getenv("KYBER1024_KAT_RSP")
	if path == "" {
		t.Skip("KYBER1024_KAT_RSP not set; the committed 10 KATs run in TestKyber1024KAT")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if sum := sha256.Sum256(data); hex.EncodeToString(sum[:]) != fullKATSHA256 {
		t.Fatalf("%s: SHA-256 %x, want %s", path, sum, fullKATSHA256)
	}
	entry := map[string]string{}
	run := 0
	flush := func() {
		if len(entry) == 0 {
			return
		}
		count, _ := strconv.Atoi(entry["count"])
		checkKAT(t, count, testvec.Hex(t, strings.ToLower(entry["seed"])), entry["pk"], entry["sk"], entry["ct"], entry["ss"])
		run++
		entry = map[string]string{}
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			flush()
			continue
		}
		if k, v, ok := strings.Cut(line, " = "); ok {
			entry[k] = v
		}
	}
	flush()
	if run != 100 {
		t.Errorf("ran %d KATs, want 100", run)
	}
}

// TestImplicitRejection: a tampered ciphertext yields KDF(z || H(c)), per
// draft section 11.3. (libcrux computes a different, equally pseudorandom
// value here; spec/crypto/kem.md explains why that has no interop impact.)
func TestImplicitRejection(t *testing.T) {
	seed := bytes.Repeat([]byte{0x5a}, 64)
	dk, _ := NewDecapsulationKey1024FromSeed(seed)
	ct, ss := dk.EncapsulationKey().encapsulateWith(bytes.Repeat([]byte{1}, 32))
	ct[0] ^= 1
	got, err := dk.Decapsulate(ct)
	if err != nil {
		t.Fatal(err)
	}
	hc := sha3.Sum256(ct)
	want := sha3.SumSHAKE256(append(bytes.Clone(seed[32:]), hc[:]...), 32)
	if !bytes.Equal(got, want) {
		t.Errorf("rejection value %x, want KDF(z||H(c)) = %x", got, want)
	}
	if bytes.Equal(got, ss) {
		t.Error("tampered ciphertext gave the real shared secret")
	}
}

func TestLengths(t *testing.T) {
	if _, err := NewDecapsulationKey1024FromSeed(make([]byte, 63)); err == nil {
		t.Error("63-byte seed accepted")
	}
	if _, err := NewDecapsulationKey1024(make([]byte, 3167)); err == nil {
		t.Error("3167-byte secret key accepted")
	}
	if _, err := NewEncapsulationKey1024(make([]byte, 1569)); err == nil {
		t.Error("1569-byte public key accepted")
	}
	dk, _ := NewDecapsulationKey1024FromSeed(make([]byte, 64))
	if _, err := dk.Decapsulate(make([]byte, 1567)); err == nil {
		t.Error("1567-byte ciphertext accepted")
	}
	ek, err := NewEncapsulationKey1024(dk.EncapsulationKey().Bytes())
	if err != nil {
		t.Fatal(err)
	}
	ct, ss, err := ek.Encapsulate(rand.NewChaCha8([32]byte{3, 4}))
	if err != nil {
		t.Fatal(err)
	}
	if len(ct) != CiphertextSize1024 || len(ss) != SharedKeySize {
		t.Fatalf("sizes %d, %d", len(ct), len(ss))
	}
	if got, _ := dk.Decapsulate(ct); !bytes.Equal(got, ss) {
		t.Error("round trip failed")
	}
}

// TestMLKEMVariantMatchesCryptoMLKEM checks the shared core, in its FIPS 203
// variant, against Go's crypto/mlkem for both parameter sets: same
// encapsulation key from the same seed, and decapsulating crypto/mlkem's
// ciphertexts gives its shared key.
func TestMLKEMVariantMatchesCryptoMLKEM(t *testing.T) {
	r := rand.New(rand.NewPCG(5, 6))
	for range 20 {
		seed := make([]byte, 64)
		for i := range seed {
			seed[i] = byte(r.Uint32())
		}

		ref1024, err := mlkem.NewDecapsulationKey1024(seed)
		if err != nil {
			t.Fatal(err)
		}
		checkVariant(t, params1024, seed, ref1024.EncapsulationKey().Bytes(), func(ek []byte) ([]byte, []byte) {
			k, err := mlkem.NewEncapsulationKey1024(ek)
			if err != nil {
				t.Fatal(err)
			}
			ss, ct := k.Encapsulate()
			return ct, ss
		})

		ref768, err := mlkem.NewDecapsulationKey768(seed)
		if err != nil {
			t.Fatal(err)
		}
		checkVariant(t, params768, seed, ref768.EncapsulationKey().Bytes(), func(ek []byte) ([]byte, []byte) {
			k, err := mlkem.NewEncapsulationKey768(ek)
			if err != nil {
				t.Fatal(err)
			}
			ss, ct := k.Encapsulate()
			return ct, ss
		})
	}
}

func checkVariant(t *testing.T, p *params, seed, refEK []byte, refEncaps func([]byte) ([]byte, []byte)) {
	t.Helper()
	ek, dk := keyGen(p, mlkemVariant, seed[:32], seed[32:])
	if !bytes.Equal(ek, refEK) {
		t.Fatalf("k=%d: encapsulation key differs from crypto/mlkem", p.k)
	}
	ct, want := refEncaps(ek)
	if got := decaps(p, mlkemVariant, dk, ct); !bytes.Equal(got, want) {
		t.Fatalf("k=%d: decapsulated %x, crypto/mlkem says %x", p.k, got, want)
	}
	// And our encapsulation decapsulates under crypto/mlkem semantics.
	m := make([]byte, 32)
	m[0] = byte(p.k)
	ct2, ss2 := encaps(p, mlkemVariant, ek, m)
	if got := decaps(p, mlkemVariant, dk, ct2); !bytes.Equal(got, ss2) {
		t.Fatalf("k=%d: own round trip failed", p.k)
	}
}

// FuzzEncapsulate: any 1568-byte public key (peer prekeys are untrusted)
// must encapsulate without panicking, and decapsulation must accept any
// 1568-byte ciphertext.
func FuzzEncapsulate(f *testing.F) {
	dk, _ := NewDecapsulationKey1024FromSeed(make([]byte, 64))
	f.Add(dk.EncapsulationKey().Bytes(), byte(0))
	f.Add(bytes.Repeat([]byte{0xff}, EncapsulationKeySize1024), byte(1))
	f.Fuzz(func(t *testing.T, pk []byte, m byte) {
		ek, err := NewEncapsulationKey1024(pk)
		if err != nil {
			return
		}
		ct, _ := ek.encapsulateWith(bytes.Repeat([]byte{m}, 32))
		if _, err := dk.Decapsulate(ct); err != nil {
			t.Fatal(err)
		}
	})
}

// TestNTTMatchesDefinition computes the NTT from the matrix definition in
// draft section 5.1.3.2: NTT_ij = zeta^((2 brv(i>>1) + 1)(j>>1)) when
// i = j mod 2, else 0, with zeta = 17.
func TestNTTMatchesDefinition(t *testing.T) {
	brv := func(x int) int {
		b := 0
		for j := range 7 {
			b |= (x >> j & 1) << (6 - j)
		}
		return b
	}
	pow := func(e int) uint32 {
		r := uint32(1)
		for range e % 256 { // zeta has order 256
			r = r * 17 % q
		}
		return r
	}
	var a poly
	for i := range a {
		a[i] = uint16(i)
	}
	var want poly
	for i := range n {
		var s uint32
		for j := i % 2; j < n; j += 2 {
			s = (s + pow((2*brv(i>>1)+1)*(j>>1))*uint32(a[j])) % q
		}
		want[i] = uint16(s)
	}
	got := a
	got.ntt()
	if got != want {
		t.Errorf("NTT(0..255) differs from the matrix definition: got %v..., want %v...", got[:6], want[:6])
	}
}
