package fingerprint

import (
	"encoding/base64"
	"encoding/hex"
	"errors"
	"testing"

	"github.com/aren55555/signal-cli-go/internal/testvec"
)

// Vectors: testdata/vectors/fingerprint.json (spec/crypto/fingerprint.md).
type side struct {
	Identifier string
	Text       string `json:"identifier_text"`
	Key        string
}

type vectors struct {
	Keys      map[string]string
	Published struct {
		Alice      string `json:"alice_identifier_utf8"`
		Bob        string `json:"bob_identifier_utf8"`
		V1, V2     string `json:"-"`
		Display1   string `json:"display_v1"`
		Display2   string `json:"display_v2"`
		AliceScan1 string `json:"alice_scannable_v1"`
		BobScan1   string `json:"bob_scannable_v1"`
		AliceScan2 string `json:"alice_scannable_v2"`
		BobScan2   string `json:"bob_scannable_v2"`
	}
	PerSide struct {
		Entries []struct {
			Name, Identifier, Key, Digest, Digits string
		}
	} `json:"per_side"`
	Cases []struct {
		Name                  string
		Version               uint32
		Local, Remote         side
		Display, Grouped      string
		LocalView, RemoteView string `json:"-"`
		ScannableLocal        string `json:"scannable_local_view"`
		ScannableRemote       string `json:"scannable_remote_view"`
		ScannableLocalB64     string `json:"scannable_local_view_b64"`
	} `json:"signal_cli_cases"`
	Compare struct {
		Cases []struct{ Theirs, Result string }
	} `json:"scannable_compare"`
	Parsing struct {
		Cases []struct{ Input, Kind, Value string }
	} `json:"trust_input_parsing"`
	Formatting struct {
		Cases []struct{ Input, Output string }
	} `json:"listIdentities_formatting"`
}

func load(t *testing.T) *vectors {
	var v vectors
	testvec.Load(t, "fingerprint.json", &v)
	return &v
}

func (s side) id(t *testing.T) []byte {
	if s.Text != "" {
		return []byte(s.Text)
	}
	return testvec.Hex(t, s.Identifier)
}

func TestPerSide(t *testing.T) {
	v := load(t)
	for _, e := range v.PerSide.Entries {
		d := Digest(testvec.Hex(t, e.Identifier), testvec.Hex(t, v.Keys[e.Key+"_identity_key"]))
		if hex.EncodeToString(d[:]) != e.Digest || Digits(d) != e.Digits {
			t.Errorf("%s: %x %s", e.Name, d, Digits(d))
		}
	}
}

func TestPublished(t *testing.T) {
	v := load(t)
	p := v.Published
	a, b := testvec.Hex(t, v.Keys["alice_identity_key"]), testvec.Hex(t, v.Keys["bob_identity_key"])
	f := New(1, []byte(p.Alice), a, []byte(p.Bob), b)
	if f.Display != p.Display1 || hex.EncodeToString(f.Scannable) != p.AliceScan1 {
		t.Errorf("v1 alice: %s %x", f.Display, f.Scannable)
	}
	g := New(2, []byte(p.Bob), b, []byte(p.Alice), a)
	if g.Display != p.Display2 || hex.EncodeToString(g.Scannable) != p.BobScan2 {
		t.Errorf("v2 bob: %s %x", g.Display, g.Scannable)
	}
	if ok, err := Compare(f.Scannable, testvec.Hex(t, p.BobScan1)); !ok || err != nil {
		t.Errorf("alice v1 vs bob's code: %v %v", ok, err)
	}
}

func TestSignalCLICases(t *testing.T) {
	v := load(t)
	for _, c := range v.Cases {
		f := New(c.Version, c.Local.id(t), testvec.Hex(t, v.Keys[c.Local.Key+"_identity_key"]),
			c.Remote.id(t), testvec.Hex(t, v.Keys[c.Remote.Key+"_identity_key"]))
		if f.Display != c.Display || FormatSafetyNumber(f.Display) != c.Grouped {
			t.Errorf("%s: display %s", c.Name, f.Display)
		}
		if hex.EncodeToString(f.Scannable) != c.ScannableLocal || base64.StdEncoding.EncodeToString(f.Scannable) != c.ScannableLocalB64 {
			t.Errorf("%s: scannable %x", c.Name, f.Scannable)
		}
	}
}

func TestCompare(t *testing.T) {
	v := load(t)
	c0 := v.Cases[0]
	ours := testvec.Hex(t, c0.ScannableLocal)
	for _, c := range v.Compare.Cases {
		theirs, _ := hex.DecodeString(c.Theirs)
		ok, err := Compare(ours, theirs)
		var got string
		switch {
		case errors.Is(err, ErrVersionMismatch):
			got = "version_mismatch"
		case err != nil:
			got = "parse_error"
		case ok:
			got = "match"
		default:
			got = "no_match"
		}
		if got != c.Result {
			t.Errorf("%s: got %s, want %s", c.Theirs, got, c.Result)
		}
	}
}

func TestParseVerificationCode(t *testing.T) {
	v := load(t)
	for _, c := range v.Parsing.Cases {
		code, err := ParseVerificationCode(c.Input)
		if c.Kind == "format_error" {
			if !errors.Is(err, ErrInvalidFormat) {
				t.Errorf("%q: %v", c.Input, err)
			}
			continue
		}
		if err != nil {
			t.Errorf("%q: %v", c.Input, err)
			continue
		}
		var kind, val string
		switch code.Kind {
		case KindHex:
			kind, val = "hex_fingerprint", hex.EncodeToString(code.Bytes)
		case KindNumeric:
			kind, val = "numeric", code.Numeric
		case KindScannable:
			kind, val = "scannable", hex.EncodeToString(code.Bytes)
		}
		if kind != c.Kind || (c.Value != "" && val != c.Value) {
			t.Errorf("%q: %s %s, want %s %s", c.Input, kind, val, c.Kind, c.Value)
		}
	}
}

func TestFormatSafetyNumber(t *testing.T) {
	v := load(t)
	for _, c := range v.Formatting.Cases {
		if got := FormatSafetyNumber(c.Input); got != c.Output {
			t.Errorf("%s: %q", c.Input, got)
		}
	}
}

// TestBase64SingleCharGroup: a final group of one character is rejected,
// as is wrong padding (Java's basic decoder, §6.1).
func TestBase64SingleCharGroup(t *testing.T) {
	for _, in := range []string{"abcde", "ab=", "a===", "ab-_"} {
		if _, err := ParseVerificationCode(in); !errors.Is(err, ErrInvalidFormat) {
			t.Errorf("%q accepted", in)
		}
	}
}
