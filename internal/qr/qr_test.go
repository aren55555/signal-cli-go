// SPDX-License-Identifier: GPL-3.0-or-later

package qr

import (
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"
)

// testdata/link-qr.json holds QR blocks printed by the reference's
// `link` on a terminal, with the URIs they encode (black-box capture).
func TestReferenceLinkCodes(t *testing.T) {
	data, err := os.ReadFile("testdata/link-qr.json")
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Vectors []struct {
			URI   string
			Lines []string
		}
	}
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatal(err)
	}
	if len(f.Vectors) < 3 {
		t.Fatal("vectors missing")
	}
	for _, v := range f.Vectors {
		c, err := Encode([]byte(v.URI))
		if err != nil {
			t.Fatal(err)
		}
		if got := c.Lines(); !slices.Equal(got, v.Lines) {
			t.Errorf("%d-byte URI: version %d, got\n%s\nwant\n%s", len(v.URI), c.Version, strings.Join(got, "\n"), strings.Join(v.Lines, "\n"))
		}
	}
}

// The EC table agrees with the capacity formula for every version.
func TestTables(t *testing.T) {
	for v := 1; v <= 40; v++ {
		e := ecL[v]
		if total := e[1]*(e[2]+e[0]) + e[3]*(e[4]+e[0]); total != rawCodewords(v) {
			t.Errorf("version %d: %d codewords, want %d", v, total, rawCodewords(v))
		}
	}
	for v, want := range map[int][]int{2: {6, 18}, 7: {6, 22, 38}, 14: {6, 26, 46, 66}, 32: {6, 34, 60, 86, 112, 138}, 36: {6, 24, 50, 76, 102, 128, 154}, 40: {6, 30, 58, 86, 114, 142, 170}} {
		if got := alignment(v); !slices.Equal(got, want) {
			t.Errorf("alignment %d: %v, want %v", v, got, want)
		}
	}
}

func TestVersionSelection(t *testing.T) {
	for n, want := range map[int]int{17: 1, 18: 2, 106: 5, 107: 6, 2953: 40} {
		c, err := Encode(make([]byte, n))
		if err != nil || c.Version != want {
			t.Errorf("%d bytes: version %v err %v, want %d", n, c, err, want)
		}
	}
	if _, err := Encode(make([]byte, 2954)); err != ErrTooLong {
		t.Errorf("2954 bytes: %v", err)
	}
}
