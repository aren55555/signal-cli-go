// SPDX-License-Identifier: GPL-3.0-or-later

package groups

import (
	"encoding/hex"
	"errors"
	"testing"

	"github.com/aren55555/signal-cli-go/internal/crypto/zkgroup"
	"github.com/aren55555/signal-cli-go/internal/testvec"
)

type idVectors struct {
	Derivation struct {
		Cases []struct {
			MasterKey string `json:"master_key_hex"`
			GroupID   string `json:"group_id_base64"`
		}
	} `json:"group_id_derivation"`
	Parse struct {
		Cases []struct{ Input, Result, Canonical, Error string }
	} `json:"group_id_parse"`
	InviteLink struct {
		URL string
	} `json:"invite_link"`
	Inputs struct {
		MasterKey string `json:"master_key_hex"`
		Password  string `json:"invite_link_password_hex"`
	}
	LinkParse struct {
		Cases []struct {
			Input, Outcome, Message string
			MasterKey               string `json:"master_key_hex"`
			Password                string `json:"password_hex"`
		}
	} `json:"invite_link_parse"`
	ContentRange struct {
		Valid []struct {
			Header string
			End    uint32
		}
		Invalid []*string
	} `json:"content_range"`
}

func TestGroupIDs(t *testing.T) {
	var v idVectors
	testvec.Load(t, "groups.json", &v)
	for _, c := range v.Derivation.Cases {
		var mk [32]byte
		copy(mk[:], testvec.Hex(t, c.MasterKey))
		g := zkgroup.DeriveGroupSecretParams(mk)
		if got := FormatGroupID(g.GroupID[:]); got != c.GroupID {
			t.Errorf("%s: %s", c.MasterKey, got)
		}
	}
	for _, c := range v.Parse.Cases {
		id, err := ParseGroupID(c.Input)
		switch c.Result {
		case "error":
			if err == nil || err.Error() != c.Error {
				t.Errorf("%q: %v", c.Input, err)
			}
		default:
			if err != nil || FormatGroupID(id) != c.Canonical || (len(id) == 32) != (c.Result == "v2") {
				t.Errorf("%q: %v %x", c.Input, err, id)
			}
		}
	}
}

func TestInviteLinks(t *testing.T) {
	var v idVectors
	testvec.Load(t, "groups.json", &v)
	if got := InviteLinkURL(testvec.Hex(t, v.Inputs.MasterKey), testvec.Hex(t, v.Inputs.Password)); got != v.InviteLink.URL {
		t.Errorf("url %s", got)
	}
	for _, c := range v.LinkParse.Cases {
		mk, pw, err := ParseInviteLink(c.Input)
		var le *InviteLinkError
		var ue *ErrUncaught
		switch c.Outcome {
		case "ok":
			if err != nil || hex.EncodeToString(mk[:]) != c.MasterKey || hex.EncodeToString(pw) != c.Password {
				t.Errorf("%s: %v", c.Input, err)
			}
		case "not_a_link":
			if !errors.Is(err, ErrNotALink) {
				t.Errorf("%s: %v", c.Input, err)
			}
		case "invalid", "unknown_version":
			if !errors.As(err, &le) || le.Message != c.Message || le.UnknownVersion != (c.Outcome == "unknown_version") {
				t.Errorf("%s: %v", c.Input, err)
			}
		case "uncaught":
			if !errors.As(err, &ue) || ue.Message != c.Message {
				t.Errorf("%s: %v", c.Input, err)
			}
		}
	}
}

func TestContentRange(t *testing.T) {
	var v idVectors
	testvec.Load(t, "groups.json", &v)
	for _, c := range v.ContentRange.Valid {
		if next, ok := ParseContentRange(c.Header); !ok || next != c.End+1 {
			t.Errorf("%s: %d %v", c.Header, next, ok)
		}
	}
	for _, h := range v.ContentRange.Invalid {
		s := ""
		if h != nil {
			s = *h
		}
		if _, ok := ParseContentRange(s); ok {
			t.Errorf("%q accepted", s)
		}
	}
}
