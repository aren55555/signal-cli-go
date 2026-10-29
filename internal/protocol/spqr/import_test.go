// SPDX-License-Identifier: GPL-3.0-or-later

package spqr

import (
	"bytes"
	"errors"
	"math/rand/v2"
	"testing"

	"github.com/aren55555/signal-cli-go/internal/testvec"
)

// TestImportIssue1275States imports two real libsignal states saved
// mid-epoch (one in HeaderSent, one in Ct1Sampled with a byte-swapped
// encapsulation state) and runs the upstream regression test's
// exchange: 20 rounds in which every key matches, ending in epoch 2
// (spqr_issue1275_states.json).
func TestImportIssue1275States(t *testing.T) {
	var f struct {
		Vectors []struct {
			A string `json:"state_a_hex"`
			B string `json:"state_b_hex"`
		}
	}
	testvec.Load(t, "spqr_issue1275_states.json", &f)
	sa, err := Import(testvec.Hex(t, f.Vectors[0].A))
	if err != nil {
		t.Fatalf("import A: %v", err)
	}
	sb, err := Import(testvec.Hex(t, f.Vectors[0].B))
	if err != nil {
		t.Fatalf("import B: %v", err)
	}
	kinds := map[stateKind]bool{sa.scka.kind: true, sb.scka.kind: true}
	if !kinds[headerSent] || !kinds[ct1Sampled] {
		t.Errorf("imported states %v and %v, want HeaderSent and Ct1Sampled", sa.scka.kind, sb.scka.kind)
	}
	a := &side{"A", sa, rand.NewChaCha8([32]byte{3})}
	b := &side{"B", sb, rand.NewChaCha8([32]byte{4})}
	for range 20 {
		b.recv(t, a.send(t))
		a.recv(t, b.send(t))
	}
	if a.epoch() != 2 || b.epoch() != 2 {
		t.Errorf("epochs after 20 rounds: A %d, B %d; want 2 and 2", a.epoch(), b.epoch())
	}
}

func TestImportEdgeCases(t *testing.T) {
	if s, err := Import(nil); err != nil || s.scka != nil || s.Epoch() != 0 {
		t.Errorf("empty state: %v (want V0)", err)
	}
	if _, err := Import([]byte{0x01}); !errors.Is(err, ErrStateDecode) {
		t.Errorf("garbage: %v", err)
	}
}

// TestImportRequiresCore: an SCKA state without its core data (epoch and
// authenticator) is a decode error.
func TestImportRequiresCore(t *testing.T) {
	if _, err := Import(testvec.Hex(t, "1a020a00")); !errors.Is(err, ErrStateDecode) { // v1 { keys_unsampled {} }
		t.Errorf("state without its core: %v", err)
	}
}

// TestEncoderCoefficientForm: an encoder imported in coefficient form (as
// libsignal stores it after the first parity chunk) yields the same data
// and parity chunks as the point form. The real vectors don't reach this
// form, so the coefficients are computed here by expanding the Lagrange
// basis.
func TestEncoderCoefficientForm(t *testing.T) {
	data := make([]byte, ct1StreamLen)
	for i := range data {
		data[i] = byte(i*7 + 3)
	}
	pointsForm, _ := newEncoder(data)
	coeffForm := &encoder{coeffs: new([polys][]uint16)}
	for j, pts := range pointsForm.pts {
		coeffForm.coeffs[j] = coefficients(pts)
	}
	for c := uint16(0); c < 100; c++ {
		if pointsForm.chunk(c) != coeffForm.chunk(c) {
			t.Fatalf("chunk %d differs between the two forms", c)
		}
	}
}

// coefficients expands sum_i y_i * prod_{m != i} (x - x_m)/(x_i - x_m).
func coefficients(pts []point) []uint16 {
	out := make([]uint16, len(pts))
	for i, pi := range pts {
		basis := []uint16{1} // polynomial in x, lowest degree first
		den := uint16(1)
		for m, pm := range pts {
			if m == i {
				continue
			}
			next := make([]uint16, len(basis)+1)
			for k, b := range basis { // multiply by (x + x_m); minus is plus
				next[k+1] ^= b
				next[k] ^= gfMul(b, pm.x)
			}
			basis = next
			den = gfMul(den, pi.x^pm.x)
		}
		scale := gfMul(pi.y, gfInv(den))
		for k, b := range basis {
			out[k] ^= gfMul(b, scale)
		}
	}
	return out
}

// TestExportImportEverywhere runs a lossy, reordered exchange twice with
// the same randomness and schedule: once plainly, once replacing each
// party's state with Import(Export(state)) after every operation. Both
// runs must produce byte-identical messages, the same keys and the same
// errors (late messages for dropped epochs legitimately fail in both).
// The round-tripped run must pass through all 11 SCKA states and export
// encoders in coefficient form.
func TestExportImportEverywhere(t *testing.T) {
	type party struct {
		st  *State
		rng *rand.ChaCha8
	}
	type run struct {
		a, b      party
		roundTrip bool
	}
	newRun := func(rt bool) *run {
		return &run{
			a:         party{New(authKey, true, Params{}), rand.NewChaCha8([32]byte{5})},
			b:         party{New(authKey, false, Params{}), rand.NewChaCha8([32]byte{6})},
			roundTrip: rt,
		}
	}
	plain, tripped := newRun(false), newRun(true)
	seen := map[stateKind]bool{}
	coeffSeen := false
	fix := func(r *run, p *party) {
		if !r.roundTrip {
			return
		}
		got, err := Import(p.st.Export())
		if err != nil {
			t.Fatalf("import of export failed in %v: %v", p.st.scka.kind, err)
		}
		seen[got.scka.kind] = true
		for _, e := range []*streamEnc{got.scka.hdrEnc, got.scka.ekEnc, got.scka.ct1Enc, got.scka.ct2Enc} {
			if e != nil && e.enc.coeffs != nil {
				coeffSeen = true
			}
		}
		p.st = got
	}
	send := func(r *run, p *party) ([]byte, []byte) {
		next, msg, key, err := p.st.Send(p.rng)
		if err != nil {
			t.Fatalf("send: %v", err)
		}
		p.st = next
		fix(r, p)
		return msg, key
	}
	recv := func(r *run, p *party, msg []byte) string {
		next, key, err := p.st.Receive(msg)
		if err != nil {
			return "error: " + err.Error()
		}
		p.st = next
		fix(r, p)
		return string(key)
	}

	sched := rand.New(rand.NewPCG(13, 13))
	var toA, toB [][]byte
	for step := range 1500 {
		ma1, ka1 := send(plain, &plain.a)
		ma2, ka2 := send(tripped, &tripped.a)
		mb1, kb1 := send(plain, &plain.b)
		mb2, kb2 := send(tripped, &tripped.b)
		if !bytes.Equal(ma1, ma2) || !bytes.Equal(ka1, ka2) || !bytes.Equal(mb1, mb2) || !bytes.Equal(kb1, kb2) {
			t.Fatalf("step %d: sends differ after export/import", step)
		}
		if sched.IntN(10) >= 3 {
			toB = append(toB, ma1)
		}
		if sched.IntN(10) >= 3 {
			toA = append(toA, mb1)
		}
		for _, q := range []struct {
			queue  *[][]byte
			p1, p2 *party
		}{{&toB, &plain.b, &tripped.b}, {&toA, &plain.a, &tripped.a}} {
			if len(*q.queue) == 0 || sched.IntN(3) == 0 {
				continue
			}
			i := sched.IntN(len(*q.queue))
			m := (*q.queue)[i]
			*q.queue = append((*q.queue)[:i], (*q.queue)[i+1:]...)
			if r1, r2 := recv(plain, q.p1, m), recv(tripped, q.p2, m); r1 != r2 {
				t.Fatalf("step %d: receive results differ: %q vs %q", step, r1, r2)
			}
		}
	}
	if len(seen) != 11 {
		t.Errorf("visited %d of 11 states: %v", len(seen), seen)
	}
	if !coeffSeen {
		t.Error("no encoder was exported in coefficient form")
	}
}
