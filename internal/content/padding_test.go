// SPDX-License-Identifier: GPL-3.0-or-later

package content

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/aren55555/signal-cli-go/internal/testvec"
)

type paddingFile struct {
	UpstreamRanges []struct {
		Min    int `json:"input_length_min"`
		Max    int `json:"input_length_max"`
		Padded int `json:"padded_length"`
	} `json:"upstream_ranges"`
	DerivedCases []struct {
		Length int `json:"input_length"`
		Padded int `json:"padded_length"`
		Zeros  int `json:"zero_bytes_after_terminator"`
	} `json:"derived_cases"`
	UnpaddingCases []struct {
		Note    string
		Padded  string `json:"padded_hex"`
		Content string `json:"content_hex"`
	} `json:"unpadding_cases"`
}

// TestPaddingVectors: every length in the upstream ranges
// (PushTransportDetailsTest) pads to the stated length, the derived
// boundary cases hold, and the layout is content || 0x80 || zeros
// (spec/service/content.md §2).
func TestPaddingVectors(t *testing.T) {
	var f paddingFile
	testvec.Load(t, "content_padding.json", &f)
	if len(f.UpstreamRanges) != 3 {
		t.Fatalf("got %d ranges", len(f.UpstreamRanges))
	}
	for _, r := range f.UpstreamRanges {
		for l := r.Min; l <= r.Max; l++ {
			in := bytes.Repeat([]byte{0}, l)
			out := Pad(in)
			if len(out) != r.Padded {
				t.Fatalf("length %d pads to %d, want %d", l, len(out), r.Padded)
			}
			if !bytes.Equal(out[:l], in) || out[l] != 0x80 || len(bytes.Trim(out[l+1:], "\x00")) != 0 {
				t.Fatalf("length %d: bad layout", l)
			}
		}
	}
	for _, c := range f.DerivedCases {
		out := Pad(bytes.Repeat([]byte{1}, c.Length))
		if len(out) != c.Padded || len(out)-c.Length-1 != c.Zeros {
			t.Errorf("length %d: %d bytes", c.Length, len(out))
		}
	}
}

func TestUnpadding(t *testing.T) {
	var f paddingFile
	testvec.Load(t, "content_padding.json", &f)
	for _, c := range f.UnpaddingCases {
		in, _ := hex.DecodeString(c.Padded)
		got, _ := Unpad(in)
		if hex.EncodeToString(got) != c.Content {
			t.Errorf("%s: %x, want %s", c.Note, got, c.Content)
		}
	}
	// Round trip, and the malformed case keeps the whole buffer.
	for l := range 300 {
		in := bytes.Repeat([]byte{0x42}, l)
		got, ok := Unpad(Pad(in))
		if !ok || !bytes.Equal(got, in) {
			t.Fatalf("round trip of %d bytes", l)
		}
	}
	bad := []byte{1, 2, 3, 0, 0}
	if got, ok := Unpad(bad); ok || !bytes.Equal(got, bad) {
		t.Errorf("malformed padding: %x %v (want the whole buffer, not ok)", got, ok)
	}
	if got, ok := Unpad([]byte{0, 0}); ok || len(got) != 0 {
		t.Errorf("all-zero buffer: %x %v", got, ok)
	}
}
