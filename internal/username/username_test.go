// SPDX-License-Identifier: GPL-3.0-or-later

package username

import (
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/aren55555/signal-cli-go/internal/testvec"
)

// Vectors: testdata/vectors/usernames.json (spec/crypto/usernames.md).
type vectors struct {
	Invalid []struct {
		Input string
		Valid bool
		Hash  string
		Error *struct{ Class, Message string }
	} `json:"invalid_usernames"`
	Candidates []struct {
		Nickname string
		Count    int
		Sample   []string
		Error    *struct{ Class, Message string }
	}
	Links []struct {
		Username, Entropy, IV, Plaintext string
		Blob                             string `json:"encrypted_username"`
	}
	Upstream []struct {
		Entropy  string
		Blob     string `json:"encrypted_username_b64url"`
		Username string
	} `json:"link_upstream"`
	URLs []struct {
		Input, Entropy, Error string
		Valid                 bool
		ServerID              string `json:"server_id"`
		Canonical             string `json:"canonical_url"`
	} `json:"link_url_cases"`
	Construction []struct {
		Entropy, URL string
		ServerID     string `json:"server_id"`
	} `json:"link_url_construction"`
}

func load(t *testing.T) *vectors {
	var v vectors
	testvec.Load(t, "usernames.json", &v)
	return &v
}

func TestParse(t *testing.T) {
	v := load(t)
	for _, c := range v.Invalid {
		u, err := Parse(c.Input)
		if c.Error != nil {
			var e *Error
			if !errors.As(err, &e) || e.Message != c.Error.Message || e.Class != c.Error.Class {
				t.Errorf("%q: %v, want %s: %s", c.Input, err, c.Error.Class, c.Error.Message)
			}
			continue
		}
		if err != nil {
			t.Errorf("%q: %v", c.Input, err)
			continue
		}
		if c.Hash != "" {
			h, _ := u.Hash()
			if hex.EncodeToString(h) != c.Hash {
				t.Errorf("%q: hash %x", c.Input, h)
			}
		}
		if u.String() != c.Input && !strings.Contains(c.Input, ".0") {
			t.Errorf("%q round-trips as %q", c.Input, u.String())
		}
	}
}

func TestCandidates(t *testing.T) {
	v := load(t)
	ranges := [][3]uint64{{1, 100, 4}, {100, 1000, 3}, {1000, 10000, 3}, {10000, 100000, 2}, {100000, 1000000, 2},
		{1000000, 10000000, 2}, {10000000, 100000000, 2}, {100000000, 1000000000, 2}}
	for _, c := range v.Candidates {
		got, err := Candidates(c.Nickname, 3, 32, nil)
		if c.Error != nil {
			var e *Error
			if !errors.As(err, &e) || e.Message != c.Error.Message {
				t.Errorf("%q: %v", c.Nickname, err)
			}
			continue
		}
		if err != nil || len(got) != 20 {
			t.Fatalf("%q: %d %v", c.Nickname, len(got), err)
		}
		i := 0
		for _, r := range ranges {
			seen := map[uint64]bool{}
			for range r[2] {
				d, _ := strconv.ParseUint(got[i][strings.LastIndexByte(got[i], '.')+1:], 10, 64)
				if d < r[0] || d >= r[1] || seen[d] || !strings.HasPrefix(got[i], c.Nickname+".") {
					t.Errorf("%q candidate %d %s outside %v or repeated", c.Nickname, i, got[i], r)
				}
				if _, err := Parse(got[i]); err != nil {
					t.Errorf("candidate %s invalid: %v", got[i], err)
				}
				seen[d] = true
				i++
			}
		}
	}
}

func TestLinkEncryption(t *testing.T) {
	v := load(t)
	for _, c := range v.Links {
		blob, err := EncryptLink([32]byte(testvec.Hex(t, c.Entropy)), [16]byte(testvec.Hex(t, c.IV)), c.Username)
		if err != nil || hex.EncodeToString(blob) != c.Blob {
			t.Errorf("%s: %x %v", c.Username, blob, err)
		}
		got, err := DecryptLink(testvec.Hex(t, c.Entropy), blob)
		if err != nil || got != c.Username {
			t.Errorf("%s: decrypt %q %v", c.Username, got, err)
		}
	}
	u := v.Upstream[0]
	blob, _ := base64.RawURLEncoding.DecodeString(u.Blob)
	if got, err := DecryptLink(testvec.Hex(t, u.Entropy), blob); err != nil || got != u.Username {
		t.Errorf("upstream: %q %v", got, err)
	}
	wrong := testvec.Hex(t, v.Upstream[1].Entropy)
	if _, err := DecryptLink(wrong, blob); err == nil || !strings.Contains(err.Error(), "HMAC") {
		t.Errorf("wrong entropy: %v", err)
	}
	if _, err := DecryptLink(make([]byte, 16), blob); err == nil {
		t.Error("16-byte entropy accepted")
	}
	if _, err := DecryptLink(testvec.Hex(t, u.Entropy), blob[:48]); err == nil || !strings.Contains(err.Error(), "too short") {
		t.Errorf("48-byte blob: %v", err)
	}
	if _, err := EncryptLink([32]byte{}, [16]byte{}, strings.Repeat("a", 62)); err == nil {
		t.Error("62-byte username accepted")
	}
	if _, err := EncryptLink([32]byte{}, [16]byte{}, strings.Repeat("a", 61)); err != nil {
		t.Errorf("61-byte username: %v", err)
	}
}

func TestLinkURLs(t *testing.T) {
	v := load(t)
	for _, c := range v.URLs {
		ent, id, err := ParseLinkURL(c.Input)
		if !c.Valid {
			if err == nil || !strings.HasSuffix(c.Error, err.Error()) {
				t.Errorf("%q: %v, want %s", c.Input, err, c.Error)
			}
			continue
		}
		if err != nil || hex.EncodeToString(ent[:]) != c.Entropy || id != c.ServerID || LinkURL(ent, id) != c.Canonical {
			t.Errorf("%q: %x %s %v", c.Input, ent, id, err)
		}
	}
	for _, c := range v.Construction {
		if got := LinkURL([32]byte(testvec.Hex(t, c.Entropy)), c.ServerID); got != c.URL {
			t.Errorf("%s: %s", c.ServerID, got)
		}
	}
}
