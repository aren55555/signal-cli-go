// SPDX-License-Identifier: GPL-3.0-or-later

package manager

import (
	"errors"
	"slices"
	"testing"

	"github.com/aren55555/signal-cli-go/internal/testvec"
)

func TestMultiRecipientVectors(t *testing.T) {
	var v struct {
		Requests []struct {
			Name   string
			Inputs struct {
				Timestamp      uint64
				Online, Urgent bool
				Auth           string
			}
			Expected struct{ Path string }
		} `json:"multi_recipient_requests"`
		Responses []struct {
			Status       int
			Body         *string
			Expect       string
			Unregistered []string `json:"unregistered_ids"`
			Entries      []struct {
				Account               string
				Missing, Extra, Stale []uint32
			}
		} `json:"multi_recipient_responses"`
	}
	testvec.Load(t, "stories.json", &v)
	for _, r := range v.Requests {
		if got := multiRecipientPath(r.Inputs.Timestamp, r.Inputs.Online, r.Inputs.Urgent, r.Inputs.Auth == "story"); got != r.Expected.Path {
			t.Errorf("%s: %s, want %s", r.Name, got, r.Expected.Path)
		}
	}
	for i, r := range v.Responses {
		var body []byte
		if r.Body != nil {
			body = []byte(*r.Body)
		}
		res, err := parseMultiRecipientResponse(r.Status, body)
		var se *mrServerError
		switch r.Expect {
		case "ok":
			if err != nil || !slices.Equal(res.Unregistered, r.Unregistered) && len(r.Unregistered)+len(res.Unregistered) > 0 {
				t.Errorf("%d: %v %v", i, err, res)
			}
		case "unauthorized":
			if !errors.Is(err, errMRUnauthorized) {
				t.Errorf("%d: %v", i, err)
			}
		case "server_side_error":
			if !errors.As(err, &se) {
				t.Errorf("%d: %v", i, err)
			}
		case "unexpected":
			if !errors.Is(err, errMRUnexpected) {
				t.Errorf("%d: %v, want unexpected", i, err)
			}
		case "mismatched_devices":
			if err != nil || len(res.Mismatches) != len(r.Entries) {
				t.Fatalf("%d: %v %v", i, err, res)
			}
			for j, e := range r.Entries {
				m := res.Mismatches[j]
				if m.Account != e.Account || !slices.Equal(m.Missing, e.Missing) && len(e.Missing)+len(m.Missing) > 0 ||
					!slices.Equal(m.Extra, e.Extra) && len(e.Extra)+len(m.Extra) > 0 || !slices.Equal(m.Stale, e.Stale) && len(e.Stale)+len(m.Stale) > 0 {
					t.Errorf("%d/%d: %+v, want %+v", i, j, m, e)
				}
			}
		default:
			t.Errorf("%d: unknown expectation %q", i, r.Expect)
		}
	}
}
