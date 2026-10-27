package cdsi

import (
	"encoding/hex"
	"errors"
	"testing"

	"github.com/aren55555/signal-cli-go/internal/testvec"
)

// Vectors: testdata/vectors/cdsi.json (spec/service/cdsi.md).
type vectors struct {
	E164s struct {
		Numbers    []string
		Serialized string `json:"serialized_hex"`
	} `json:"e164_serialization"`
	Parsing struct {
		Cases []struct {
			Input string
			Value uint64
			Error bool
		}
	} `json:"e164_string_parsing"`
	Full struct {
		Inputs struct {
			Pairs []struct {
				ACI       string
				AccessKey string `json:"access_key_hex"`
			} `json:"aci_uak_pairs"`
			Prev  []string `json:"prev_e164s"`
			New   []string `json:"new_e164s"`
			Token string   `json:"token_hex"`
		}
		Encoded string `json:"encoded_hex"`
	} `json:"client_request_full"`
	Partial struct {
		Inputs struct {
			New []string `json:"new_e164s"`
		}
		Encoded string `json:"encoded_hex"`
	} `json:"client_request_partial"`
	Ack struct {
		Encoded string `json:"encoded_hex"`
	} `json:"client_token_ack"`
	TokenResp struct {
		Encoded string `json:"encoded_hex"`
		Token   string `json:"expected_token_hex"`
	} `json:"server_token_response"`
	Final struct {
		Encoded  string `json:"encoded_hex"`
		Expected struct {
			Permits int32 `json:"debug_permits_used"`
			Results map[string]struct{ ACI, PNI *string }
			Absent  []string
		}
	} `json:"server_final_response"`
	Published struct {
		Triple  string `json:"triple_hex"`
		Repeat  int
		Records []struct{ E164, PNI, ACI string } `json:"expected_records"`
	} `json:"response_parse_published"`
	Close struct {
		Cases []struct {
			Code     int
			Reason   string
			Expected string
		}
	} `json:"close_codes"`
}

func load(t *testing.T) *vectors {
	var v vectors
	testvec.Load(t, "cdsi.json", &v)
	return &v
}

func uuid16(t *testing.T, s string) [16]byte {
	b, _ := hex.DecodeString(s[0:8] + s[9:13] + s[14:18] + s[19:23] + s[24:])
	return [16]byte(b)
}

func TestNumbers(t *testing.T) {
	v := load(t)
	b, err := e164s(v.E164s.Numbers)
	if err != nil || hex.EncodeToString(b) != v.E164s.Serialized {
		t.Errorf("serialization %x %v", b, err)
	}
	for _, c := range v.Parsing.Cases {
		got, err := ParseE164(c.Input)
		if c.Error != (err != nil) || (!c.Error && got != c.Value) {
			t.Errorf("%q: %d %v", c.Input, got, err)
		}
	}
}

func TestRequests(t *testing.T) {
	v := load(t)
	in := v.Full.Inputs
	r := &Request{Prev: in.Prev, New: in.New, Token: testvec.Hex(t, in.Token)}
	for _, p := range in.Pairs {
		r.Pairs = append(r.Pairs, Pair{ACI: uuid16(t, p.ACI), AccessKey: [16]byte(testvec.Hex(t, p.AccessKey))})
	}
	if b, err := r.Encode(); err != nil || hex.EncodeToString(b) != v.Full.Encoded {
		t.Errorf("full %x %v", b, err)
	}
	if b, _ := (&Request{New: v.Partial.Inputs.New}).Encode(); hex.EncodeToString(b) != v.Partial.Encoded {
		t.Errorf("partial %x", b)
	}
	if b, _ := (&Request{TokenAck: true}).Encode(); hex.EncodeToString(b) != v.Ack.Encoded {
		t.Errorf("ack %x", b)
	}
	if _, err := (&Request{New: []string{"+0"}}).Encode(); !errors.Is(err, ErrInvalidNumber) {
		t.Errorf("bad number: %v", err)
	}
}

func TestResponses(t *testing.T) {
	v := load(t)
	tr, err := ParseResponse(testvec.Hex(t, v.TokenResp.Encoded))
	if err != nil || hex.EncodeToString(tr.Token) != v.TokenResp.Token {
		t.Errorf("token response %x %v", tr.Token, err)
	}
	fr, err := ParseResponse(testvec.Hex(t, v.Final.Encoded))
	if err != nil || fr.PermitsUsed != v.Final.Expected.Permits {
		t.Fatalf("final %v", err)
	}
	res, err := fr.Results()
	if err != nil || len(res) != len(v.Final.Expected.Results) {
		t.Fatalf("results %v %v", res, err)
	}
	for num, want := range v.Final.Expected.Results {
		got := res[num]
		if (want.ACI == nil) != (got.ACI == "") || (want.ACI != nil && *want.ACI != got.ACI) || got.PNI != *want.PNI {
			t.Errorf("%s: %+v", num, got)
		}
	}
	for _, num := range v.Final.Expected.Absent {
		if _, ok := res[num]; ok {
			t.Errorf("%s present", num)
		}
	}
	triple := testvec.Hex(t, v.Published.Triple)
	var all []byte
	for range v.Published.Repeat {
		all = append(all, triple...)
	}
	pr := &Response{Triples: all}
	pres, _ := pr.Results()
	rec := v.Published.Records[0]
	if got := pres[rec.E164]; len(pres) != 1 || got.ACI != rec.ACI || got.PNI != rec.PNI {
		t.Errorf("published %v", pres)
	}
	if _, err := (&Response{Triples: make([]byte, 41)}).Results(); err == nil {
		t.Error("41-byte triples accepted")
	}
}

func TestCloseCodes(t *testing.T) {
	v := load(t)
	for _, c := range v.Close.Cases {
		err := ClassifyClose(c.Code, c.Reason)
		var ce *CloseError
		switch {
		case c.Code == 1000:
			if err != nil {
				t.Errorf("1000: %v", err)
			}
		case c.Code == 4008 && errors.As(err, &ce):
			if ce.RetryAfter == 0 {
				t.Errorf("4008 %q: no retry-after", c.Reason)
			}
		case err == nil:
			t.Errorf("%d %q accepted", c.Code, c.Reason)
		}
	}
	if err := ClassifyClose(4008, "not json"); !errors.Is(err, ErrProtocol) {
		t.Errorf("4008 without JSON: %v", err)
	}
}

func hexDecode(s string) ([]byte, error) { return hex.DecodeString(s) }
