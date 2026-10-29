// SPDX-License-Identifier: GPL-3.0-or-later

package argparse

import (
	"strings"
	"testing"

	"github.com/aren55555/signal-cli-go/internal/testvec"
)

type textVectors struct {
	Segmentation map[string][]struct {
		Text     string   `json:"text"`
		Segments []string `json:"segments"`
	} `json:"segmentation"`
	AdjustSpace []struct {
		Line     string `json:"line"`
		Width    int    `json:"width"`
		CurWidth int    `json:"cur_width"`
		Result   string `json:"result"`
	} `json:"adjust_space"`
	Wrap []struct {
		Text             string `json:"text"`
		Width            int    `json:"width"`
		InitialOffset    int    `json:"initial_offset"`
		InitialIndent    string `json:"initial_indent"`
		SubsequentIndent string `json:"subsequent_indent"`
		Result           string `json:"result"`
	} `json:"wrap"`
	HelpRows []struct {
		Title  string `json:"title"`
		Help   string `json:"help"`
		Width  int    `json:"width"`
		Result string `json:"result"`
	} `json:"help_rows"`
	Levenshtein []struct {
		Input     string `json:"input"`
		Candidate string `json:"candidate"`
		Distance  int    `json:"distance"`
	} `json:"levenshtein"`
}

func loadText(t *testing.T) textVectors {
	var v textVectors
	testvec.Load(t, "argparse_text.json", &v)
	return v
}

func TestSegmentationVectors(t *testing.T) {
	v := loadText(t)
	n := 0
	for kind, cases := range v.Segmentation {
		for _, c := range cases {
			n++
			if got := Segments(c.Text); strings.Join(got, "|") != strings.Join(c.Segments, "|") {
				t.Errorf("%s %q:\n got %q\nwant %q", kind, c.Text, got, c.Segments)
			}
		}
	}
	if n != 800 {
		t.Errorf("%d segmentation cases", n)
	}
}

func TestAdjustSpaceVectors(t *testing.T) {
	for _, c := range loadText(t).AdjustSpace {
		if got := adjustSpace(c.Line, c.Width, c.CurWidth); got != c.Result {
			t.Errorf("%q w=%d cur=%d:\n got %q\nwant %q", c.Line, c.Width, c.CurWidth, got, c.Result)
		}
	}
}

func TestWrapVectors(t *testing.T) {
	v := loadText(t)
	if len(v.Wrap) != 220 {
		t.Fatalf("%d wrap cases", len(v.Wrap))
	}
	for _, c := range v.Wrap {
		if got := Wrap(c.Text, c.Width, c.InitialOffset, c.InitialIndent, c.SubsequentIndent); got != c.Result {
			t.Errorf("%.40q w=%d off=%d:\n got %q\nwant %q", c.Text, c.Width, c.InitialOffset, got, c.Result)
		}
	}
}

func TestHelpRowVectors(t *testing.T) {
	for _, c := range loadText(t).HelpRows {
		if got := helpRow(c.Title, c.Help, c.Width); got != c.Result {
			t.Errorf("%q w=%d:\n got %q\nwant %q", c.Title, c.Width, got, c.Result)
		}
	}
}

func TestEditDistanceVectors(t *testing.T) {
	for _, c := range loadText(t).Levenshtein {
		if got := editDistance(c.Input, c.Candidate); got != c.Distance {
			t.Errorf("%q -> %q: %d, want %d", c.Input, c.Candidate, got, c.Distance)
		}
	}
}
