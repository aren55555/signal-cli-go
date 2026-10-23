package kyber

import (
	"bytes"
	"crypto/mlkem"
	"crypto/sha3"
	"encoding/hex"
	"errors"
	"math/rand/v2"
	"testing"

	"github.com/aren55555/signal-cli-go/internal/testvec"
)

func sha3hex(b []byte) string { s := sha3.Sum256(b); return hex.EncodeToString(s[:]) }

// TestMLKEM768KAT runs libcrux's 100 ML-KEM-768 KATs (which libcrux also
// runs through its incremental API) through the one-shot and the split
// encapsulation, per spec/protocol/spqr/mlkem-incremental.md.
func TestMLKEM768KAT(t *testing.T) {
	var f struct {
		Vectors []struct {
			KeygenSeed string `json:"key_generation_seed"`
			EncapsSeed string `json:"encapsulation_seed"`
			HashPK     string `json:"sha3_256_hash_of_public_key"`
			HashSK     string `json:"sha3_256_hash_of_secret_key"`
			HashCT     string `json:"sha3_256_hash_of_ciphertext"`
			SS         string `json:"shared_secret"`
		}
	}
	testvec.Load(t, "spqr_mlkem768_incremental_kat.json", &f)
	if len(f.Vectors) != 100 {
		t.Fatalf("got %d KATs", len(f.Vectors))
	}
	for i, v := range f.Vectors {
		ek, dk, err := GenerateKeyMLKEM768(testvec.Hex(t, v.KeygenSeed))
		if err != nil {
			t.Fatal(err)
		}
		if sha3hex(ek) != v.HashPK || sha3hex(dk) != v.HashSK {
			t.Fatalf("KAT %d: key hashes differ", i)
		}
		m := testvec.Hex(t, v.EncapsSeed)
		ct, ss, err := EncapsulateMLKEM768(ek, m)
		if err != nil {
			t.Fatal(err)
		}
		if sha3hex(ct) != v.HashCT || hex.EncodeToString(ss) != v.SS {
			t.Fatalf("KAT %d: one-shot encapsulation differs", i)
		}
		if got, err := DecapsulateMLKEM768(dk, ct); err != nil || hex.EncodeToString(got) != v.SS {
			t.Fatalf("KAT %d: decapsulation: %x %v", i, got, err)
		}

		hk := sha3.Sum256(ek)
		hdr := append(bytes.Clone(ek[EKPayloadSizeMLKEM768:]), hk[:]...)
		enc, ct1, k, err := StartEncapsulationMLKEM768(hdr, m)
		if err != nil {
			t.Fatal(err)
		}
		if hex.EncodeToString(k) != v.SS || !bytes.Equal(ct1, ct[:Ct1SizeMLKEM768]) {
			t.Fatalf("KAT %d: first part differs", i)
		}
		ct2, err := enc.Finish(ek[:EKPayloadSizeMLKEM768])
		if err != nil {
			t.Fatal(err)
		}
		if sha3hex(append(bytes.Clone(ct1), ct2...)) != v.HashCT {
			t.Fatalf("KAT %d: split ciphertext differs", i)
		}
	}
}

// TestMLKEM768HeaderChecks: the ek payload must hash to the header's H(ek)
// and pass the FIPS 203 modulus check (mlkem-incremental.md §3.2).
func TestMLKEM768HeaderChecks(t *testing.T) {
	ek, _, _ := GenerateKeyMLKEM768(bytes.Repeat([]byte{4}, 64))
	payload, rho := ek[:EKPayloadSizeMLKEM768], ek[EKPayloadSizeMLKEM768:]
	hk := sha3.Sum256(ek)
	hdr := append(bytes.Clone(rho), hk[:]...)
	m := bytes.Repeat([]byte{5}, 32)

	enc, _, _, _ := StartEncapsulationMLKEM768(hdr, m)
	bad := bytes.Clone(payload)
	bad[10] ^= 1
	if _, err := enc.Finish(bad); !errors.Is(err, ErrHeaderMismatch) {
		t.Errorf("tampered payload: %v", err)
	}

	// A payload with a coefficient >= q, and a header that matches it.
	big := bytes.Clone(payload)
	big[0], big[1] = 0xff, big[1]|0x0f // first 12-bit value = 4095
	hb := sha3.Sum256(append(bytes.Clone(big), rho...))
	enc2, _, _, err := StartEncapsulationMLKEM768(append(bytes.Clone(rho), hb[:]...), m)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := enc2.Finish(big); !errors.Is(err, ErrHeaderMismatch) {
		t.Errorf("out-of-range coefficient: %v", err)
	}

	if _, _, _, err := StartEncapsulationMLKEM768(hdr[:63], m); err == nil {
		t.Error("63-byte header accepted")
	}
	if _, err := enc.Finish(payload[:1151]); err == nil {
		t.Error("short payload accepted")
	}
}

func TestMLKEM768MatchesCryptoMLKEM(t *testing.T) {
	r := rand.New(rand.NewPCG(7, 8))
	for range 20 {
		seed := make([]byte, 64)
		for i := range seed {
			seed[i] = byte(r.Uint32())
		}
		ek, dk, err := GenerateKeyMLKEM768(seed)
		if err != nil {
			t.Fatal(err)
		}
		ref, _ := mlkem.NewDecapsulationKey768(seed)
		if !bytes.Equal(ek, ref.EncapsulationKey().Bytes()) {
			t.Fatal("encapsulation key differs from crypto/mlkem")
		}
		ss, ct := ref.EncapsulationKey().Encapsulate()
		if got, err := DecapsulateMLKEM768(dk, ct); err != nil || !bytes.Equal(got, ss) {
			t.Fatal("decapsulating a crypto/mlkem ciphertext failed")
		}
		ct2, ss2, _ := EncapsulateMLKEM768(ek, seed[:32])
		if got, err := ref.Decapsulate(ct2); err != nil || !bytes.Equal(got, ss2) {
			t.Fatal("crypto/mlkem can't decapsulate our ciphertext")
		}
	}
}
