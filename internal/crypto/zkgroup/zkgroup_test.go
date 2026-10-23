package zkgroup

import (
	"encoding/hex"
	"strings"
	"testing"

	"github.com/aren55555/signal-cli-go/internal/testvec"
)

type vectors struct {
	Sho []struct {
		Steps          string   `json:"steps"`
		Label          string   `json:"label_ascii"`
		Absorb         []string `json:"absorb_ascii"`
		CVAfterLabel   string   `json:"cv_after_label"`
		CVAfterAbsorb  string   `json:"cv_after_absorb"`
		SqueezeLen     int      `json:"squeeze_len"`
		Output         string   `json:"output"`
		CVAfterSqueeze string   `json:"cv_after_squeeze"`
	} `json:"sho_hmac_sha256"`
	SystemParams struct {
		CVAfterLabel string `json:"cv_after_label"`
		CVAfterEmpty string `json:"cv_after_absorbing_empty_data"`
		Generators   []struct {
			Output string `json:"sho_output_64"`
			Point  string `json:"point"`
		} `json:"generators"`
		Concatenated string `json:"concatenated_96"`
	} `json:"system_params"`
	Cases []struct {
		ProfileKey   string `json:"profile_key"`
		ACI          string `json:"aci"`
		Version      string `json:"version"`
		Commitment   string `json:"commitment"`
		Intermediate struct {
			KU        string `json:"sho_input_pk_aci"`
			M3Squeeze string `json:"m3_sho_output_32"`
			M3        string `json:"M3"`
			M4Masked  string `json:"m4_masked_key"`
			M4        string `json:"M4"`
			J3Squeeze string `json:"j3_sho_output_64"`
			J3        string `json:"j3_scalar_le"`
		} `json:"intermediate"`
	} `json:"cases"`
}

func load(t *testing.T) vectors {
	var v vectors
	testvec.Load(t, "zkgroup_profile.json", &v)
	if len(v.Sho) != 3 || len(v.Cases) != 9 {
		t.Fatalf("%d Sho vectors, %d cases", len(v.Sho), len(v.Cases))
	}
	return v
}

func hx(b []byte) string { return hex.EncodeToString(b) }

func TestSho(t *testing.T) {
	v := load(t)
	for i, c := range v.Sho[:2] {
		s := NewSho([]byte(c.Label))
		if c.CVAfterLabel != "" && hx(s.cv[:]) != c.CVAfterLabel {
			t.Errorf("%d: cv after label %x", i, s.cv)
		}
		s.Absorb([]byte(c.Absorb[0]))
		if c.CVAfterAbsorb != "" && hx(s.cv[:]) != c.CVAfterAbsorb {
			t.Errorf("%d: cv after absorb %x", i, s.cv)
		}
		if got := hx(s.Squeeze(c.SqueezeLen)); got != c.Output {
			t.Errorf("%d: output %s", i, got)
		}
		if c.CVAfterSqueeze != "" && hx(s.cv[:]) != c.CVAfterSqueeze {
			t.Errorf("%d: cv after squeeze %x", i, s.cv)
		}
	}
	// The long sequence, as its steps describe.
	c := v.Sho[2]
	if !strings.HasPrefix(c.Steps, "new(label='')") {
		t.Fatalf("unexpected steps %q", c.Steps)
	}
	s := NewSho(nil)
	s.Absorb([]byte("abc"))
	for _, n := range []int{63, 64, 65, 127, 128, 129} {
		s.Absorb(make([]byte, n))
	}
	for _, n := range []int{63, 64, 65, 127, 128, 129} {
		s.Squeeze(n)
	}
	s.Absorb([]byte("def"))
	if got := hx(s.Squeeze(63)); got != c.Output {
		t.Errorf("long sequence: %s", got)
	}
}

func TestSystemParams(t *testing.T) {
	v := load(t)
	var all []byte
	for i, g := range derivedGenerators() {
		if got := hx(g.Bytes()); got != v.SystemParams.Generators[i].Point {
			t.Errorf("G_j%d %s", i+1, got)
		}
		all = append(all, g.Bytes()...)
	}
	if hx(all) != v.SystemParams.Concatenated {
		t.Error("derived generators differ from the constant")
	}
	for i, g := range generators() {
		if hx(g.Bytes()) != v.SystemParams.Generators[i].Point {
			t.Errorf("constant G_j%d differs", i+1)
		}
	}
}

func TestProfileKeyVectors(t *testing.T) {
	v := load(t)
	for i, c := range v.Cases {
		key, _ := hex.DecodeString(c.ProfileKey)
		aci, _ := hex.DecodeString(strings.ReplaceAll(c.ACI, "-", ""))
		var k [32]byte
		var u [16]byte
		copy(k[:], key)
		copy(u[:], aci)
		in := c.Intermediate
		if hx(ku(k, u)) != in.KU {
			t.Errorf("case %d: KU", i)
		}
		if got := hx(m3(k, u).Bytes()); got != in.M3 {
			t.Errorf("case %d: M3 %s", i, got)
		}
		if got := hx(m4(k).Bytes()); got != in.M4 {
			t.Errorf("case %d: M4 %s", i, got)
		}
		if got := hx(j3(k, u).Bytes()); got != in.J3 {
			t.Errorf("case %d: j3 %s", i, got)
		}
		if got := ProfileKeyVersion(k, u); got != c.Version {
			t.Errorf("case %d: version %s", i, got)
		}
		if got := hx(ProfileKeyCommitment(k, u)); got != c.Commitment {
			t.Errorf("case %d: commitment %s", i, got)
		}
	}
}
