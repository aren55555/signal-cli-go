package spqr

import (
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
