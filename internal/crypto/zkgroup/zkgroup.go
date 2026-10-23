// Package zkgroup implements the parts of Signal's zkgroup that
// signal-cli-go needs, from spec/crypto/zkgroup-profile.md: the Sho
// construction and the profile-key version and commitment.
package zkgroup

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"

	"github.com/aren55555/signal-cli-go/internal/crypto/edwards25519"
	"github.com/aren55555/signal-cli-go/internal/crypto/ristretto255"
)

// Sho is the HMAC-SHA256 Sho construction (§2).
type Sho struct{ cv [32]byte }

func hmacSHA256(key []byte, parts ...[]byte) [32]byte {
	m := hmac.New(sha256.New, key)
	for _, p := range parts {
		m.Write(p)
	}
	var out [32]byte
	m.Sum(out[:0])
	return out
}

// NewSho starts from a zero chaining value and absorbs label.
func NewSho(label []byte) *Sho {
	s := &Sho{}
	s.Absorb(label)
	return s
}

// Absorb absorbs data and ratchets: cv = HMAC(cv, data || 0x00).
func (s *Sho) Absorb(data []byte) { s.cv = hmacSHA256(s.cv[:], data, []byte{0x00}) }

// Squeeze returns n bytes and ratchets.
func (s *Sho) Squeeze(n int) []byte {
	out := make([]byte, 0, n+31)
	var ctr [9]byte
	ctr[8] = 0x01
	for i := uint64(0); len(out) < n; i++ {
		binary.BigEndian.PutUint64(ctr[:8], i)
		b := hmacSHA256(s.cv[:], ctr[:])
		out = append(out, b[:]...)
	}
	binary.BigEndian.PutUint64(ctr[:8], uint64(n))
	ctr[8] = 0x02
	s.cv = hmacSHA256(s.cv[:], ctr[:])
	return out[:n]
}

func oneShot(label string, data []byte) *Sho {
	s := NewSho([]byte(label))
	s.Absorb(data)
	return s
}

func (s *Sho) point() *ristretto255.Element {
	e, _ := new(ristretto255.Element).SetUniformBytes(s.Squeeze(64))
	return e
}

func (s *Sho) scalar() *edwards25519.Scalar {
	k, _ := edwards25519.NewScalar().SetUniformBytes(s.Squeeze(64))
	return k
}

const labelPrefix = "Signal_ZKGroup_20200424_ProfileKeyAndUid_"

// ku is the 48-byte combined input (§3.1).
func ku(key [32]byte, aci [16]byte) []byte { return append(key[:], aci[:]...) }

func m3(key [32]byte, aci [16]byte) *ristretto255.Element {
	e, _ := new(ristretto255.Element).SetElligatorBytes(oneShot(labelPrefix+"ProfileKey_CalcM3", ku(key, aci)).Squeeze(32))
	return e
}

func m4(key [32]byte) *ristretto255.Element {
	masked := key
	masked[0] &= 0xFE
	masked[31] &= 0x3F
	e, _ := new(ristretto255.Element).SetElligatorBytes(masked[:])
	return e
}

func j3(key [32]byte, aci [16]byte) *edwards25519.Scalar {
	return oneShot(labelPrefix+"ProfileKeyCommitment_Calcj3", ku(key, aci)).scalar()
}

// generatorsHex is libsignal's hardcoded system parameters (§4.1),
// checked against their derivation by the tests.
const generatorsHex = "a8ca0bbd1148c466725860640ac53d2772b14eeae0170a38c62c7b3dd29c3e4a" +
	"14b9462d948f059450799f4cc2a06e55dec807735670b94a5ce80f59f1950861" +
	"b0c0f7b91f6ef9c7556093d8930a86bd36188cec740554657d92dcd86aad251c"

func generators() [3]*ristretto255.Element {
	raw, _ := hex.DecodeString(generatorsHex)
	var g [3]*ristretto255.Element
	for i := range g {
		e, err := new(ristretto255.Element).SetCanonicalBytes(raw[32*i : 32*i+32])
		if err != nil {
			panic("zkgroup: bad system parameters")
		}
		g[i] = e
	}
	return g
}

func derivedGenerators() [3]*ristretto255.Element {
	s := oneShot("Signal_ZKGroup_20200424_Constant_ProfileKeyCommitment_SystemParams_Generate", nil)
	return [3]*ristretto255.Element{s.point(), s.point(), s.point()}
}

// ProfileKeyVersion is the 64-character lowercase hex version of a
// profile key for an ACI (§5).
func ProfileKeyVersion(key [32]byte, aci [16]byte) string {
	return hex.EncodeToString(oneShot(labelPrefix+"ProfileKey_GetProfileKeyVersion", ku(key, aci)).Squeeze(32))
}

// ProfileKeyCommitment is the 97-byte serialized commitment (§4).
func ProfileKeyCommitment(key [32]byte, aci [16]byte) []byte {
	g := generators()
	j := j3(key, aci)
	j1 := new(ristretto255.Element).ScalarMult(j, g[0])
	j1.Add(j1, m3(key, aci))
	j2 := new(ristretto255.Element).ScalarMult(j, g[1])
	j2.Add(j2, m4(key))
	j3p := new(ristretto255.Element).ScalarMult(j, g[2])
	out := []byte{0x00}
	out = append(out, j1.Bytes()...)
	out = append(out, j2.Bytes()...)
	return append(out, j3p.Bytes()...)
}
