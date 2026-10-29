// SPDX-License-Identifier: GPL-3.0-or-later

package zkgroup

import (
	"crypto/sha256"
	"encoding/binary"

	"github.com/aren55555/signal-cli-go/internal/crypto/edwards25519"
	"github.com/aren55555/signal-cli-go/internal/crypto/ristretto255"
)

// poksho proofs of knowledge of linear relations
// (spec/crypto/zkgroup-credentials.md §3).

type term struct{ scalar, point string }

type equation struct {
	lhs   string
	terms []term
}

// statement is an ordered list of equations; names get indices in order
// of first appearance (§3.1), with "G" fixed at point index 0.
type statement struct {
	eqs     []equation
	points  []string // index order, points[0] == "G"
	scalars []string
	pIdx    map[string]int
	sIdx    map[string]int
}

func newStatement() *statement {
	return &statement{points: []string{"G"}, pIdx: map[string]int{"G": 0}, sIdx: map[string]int{}}
}

func (s *statement) point(name string) {
	if _, ok := s.pIdx[name]; !ok {
		s.pIdx[name] = len(s.points)
		s.points = append(s.points, name)
	}
}

func (s *statement) scalar(name string) {
	if _, ok := s.sIdx[name]; !ok {
		s.sIdx[name] = len(s.scalars)
		s.scalars = append(s.scalars, name)
	}
}

// eq adds "lhs = s1·P1 + s2·P2 + …"; pairs are scalar, point names.
func (s *statement) eq(lhs string, pairs ...string) *statement {
	e := equation{lhs: lhs}
	s.point(lhs)
	for i := 0; i+1 < len(pairs); i += 2 {
		s.scalar(pairs[i])
		s.point(pairs[i+1])
		e.terms = append(e.terms, term{pairs[i], pairs[i+1]})
	}
	s.eqs = append(s.eqs, e)
	return s
}

// description is D (§3.2).
func (s *statement) description() []byte {
	d := []byte{byte(len(s.eqs))}
	for _, e := range s.eqs {
		d = append(d, byte(s.pIdx[e.lhs]), byte(len(e.terms)))
		for _, t := range e.terms {
			d = append(d, byte(s.sIdx[t.scalar]), byte(s.pIdx[t.point]))
		}
	}
	return d
}

// transcript is step 1 of §3.4: Sho over D and the point list A.
func (s *statement) transcript(points map[string]*ristretto255.Element) (*Sho, bool) {
	buf := s.description()
	for _, name := range s.points {
		p := points[name]
		if name == "G" {
			p = ristretto255.NewGeneratorElement()
		}
		if p == nil {
			return nil, false
		}
		buf = append(buf, p.Bytes()...)
	}
	sho := NewSho([]byte(labelPokshoSHA256))
	sho.Absorb(buf)
	return sho, true
}

func (s *statement) pointValue(points map[string]*ristretto255.Element, name string) *ristretto255.Element {
	if name == "G" {
		return ristretto255.NewGeneratorElement()
	}
	return points[name]
}

func wideScalar(b []byte) *edwards25519.Scalar {
	k, _ := edwards25519.NewScalar().SetUniformBytes(b)
	return k
}

// prove is §3.4; randomness is 32 bytes.
func (s *statement) prove(scalars map[string]*edwards25519.Scalar, points map[string]*ristretto255.Element, msg, randomness []byte) []byte {
	sho, ok := s.transcript(points)
	if !ok {
		panic("poksho: missing point")
	}
	nonceSho := *sho
	buf := append([]byte{}, randomness...)
	for _, name := range s.scalars {
		buf = append(buf, scalars[name].Bytes()...)
	}
	nonceSho.Absorb(buf)
	nonceSho.Absorb(msg)
	raw := nonceSho.Squeeze(64 * len(s.scalars))
	r := map[string]*edwards25519.Scalar{}
	for i, name := range s.scalars {
		r[name] = wideScalar(raw[64*i : 64*i+64])
	}
	var commit []byte
	for _, e := range s.eqs {
		commit = append(commit, s.combine(e, r, points).Bytes()...)
	}
	sho.Absorb(append(commit, msg...))
	c := wideScalar(sho.Squeeze(64))
	out := append([]byte{}, c.Bytes()...)
	for _, name := range s.scalars {
		resp := edwards25519.NewScalar().MultiplyAdd(c, scalars[name], r[name])
		out = append(out, resp.Bytes()...)
	}
	return out
}

func (s *statement) combine(e equation, k map[string]*edwards25519.Scalar, points map[string]*ristretto255.Element) *ristretto255.Element {
	sum := ristretto255.NewIdentityElement()
	for _, t := range e.terms {
		sum.Add(sum, new(ristretto255.Element).ScalarMult(k[t.scalar], s.pointValue(points, t.point)))
	}
	return sum
}

// verify is §3.6.
func (s *statement) verify(proof []byte, points map[string]*ristretto255.Element, msg []byte) bool {
	n := len(s.scalars)
	if len(proof) != 32*(n+1) || n == 0 || n > 256 {
		return false
	}
	c, err := edwards25519.NewScalar().SetCanonicalBytes(proof[:32])
	if err != nil {
		return false
	}
	resp := map[string]*edwards25519.Scalar{}
	for i, name := range s.scalars {
		k, err := edwards25519.NewScalar().SetCanonicalBytes(proof[32*(i+1) : 32*(i+2)])
		if err != nil {
			return false
		}
		resp[name] = k
	}
	sho, ok := s.transcript(points)
	if !ok {
		return false
	}
	var commit []byte
	for _, e := range s.eqs {
		r := s.combine(e, resp, points)
		r.Subtract(r, new(ristretto255.Element).ScalarMult(c, s.pointValue(points, e.lhs)))
		commit = append(commit, r.Bytes()...)
	}
	sho.Absorb(append(commit, msg...))
	return wideScalar(sho.Squeeze(64)).Equal(c) == 1
}

// shoSHA256 is the plain-SHA-256 Sho of §1.2 (zkcredential generators).
type shoSHA256 struct{ cv [32]byte }

func newShoSHA256(label []byte) *shoSHA256 {
	s := &shoSHA256{}
	s.absorbRatchet(label)
	return s
}

func (s *shoSHA256) absorbRatchet(data []byte) {
	h := sha256.New()
	h.Write(make([]byte, 64))
	h.Write(s.cv[:])
	h.Write(data)
	inner := h.Sum(nil)
	s.cv = sha256.Sum256(inner)
}

func (s *shoSHA256) squeeze(n int) []byte {
	var out []byte
	prefix := append(make([]byte, 63), 0x01)
	for i := uint64(0); len(out) < n; i++ {
		block := sha256.Sum256(binary.BigEndian.AppendUint64(append(append([]byte{}, prefix...), s.cv[:]...), i))
		out = append(out, block[:]...)
	}
	next := append(make([]byte, 63), 0x02)
	next = append(next, s.cv[:]...)
	s.cv = sha256.Sum256(binary.BigEndian.AppendUint64(next, uint64(n)))
	return out[:n]
}
