// SPDX-License-Identifier: GPL-3.0-or-later

package manager

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/aren55555/signal-cli-go/internal/protocol"
)

// PUT /v1/messages/multi_recipient (spec/service/stories.md §3.4).

const mrmContentType = "application/vnd.signal-messenger.mrm"

// multiRecipientPath builds the request path; parameters in this order.
func multiRecipientPath(ts uint64, online, urgent, story bool) string {
	p := "/v1/messages/multi_recipient?ts=" + strconv.FormatUint(ts, 10) + "&online=" + strconv.FormatBool(online) + "&urgent=" + strconv.FormatBool(urgent)
	if story {
		p += "&story=true"
	}
	return p
}

// mismatch is one recipient's device mismatch in a 409 or 410.
type mismatch struct {
	Account               string
	Missing, Extra, Stale []uint32
}

// mrResult is the parsed response.
type mrResult struct {
	Unregistered []string   // 200: uuids404
	Mismatches   []mismatch // 409/410
}

var (
	errMRUnauthorized = errors.New("invalid unidentified access header")
	errMRUnexpected   = errors.New("unexpected multi-recipient response")
)

// mrServerError is a 5xx or a rate limit.
type mrServerError struct{ Status int }

func (e *mrServerError) Error() string {
	return fmt.Sprintf("multi-recipient send failed with status %d", e.Status)
}

func validDeviceIDs(ids []json.RawMessage) ([]uint32, bool) {
	var out []uint32
	for _, raw := range ids {
		var n int
		if bytes.HasPrefix(bytes.TrimSpace(raw), []byte(`"`)) || json.Unmarshal(raw, &n) != nil || n < 1 || n > 127 {
			return nil, false
		}
		out = append(out, uint32(n))
	}
	return out, true
}

// parseMultiRecipientResponse is §3.4's response table.
func parseMultiRecipientResponse(status int, body []byte) (*mrResult, error) {
	switch status {
	case 200:
		var ok struct {
			UUIDs404 []string `json:"uuids404"`
		}
		if err := json.Unmarshal(body, &ok); err != nil {
			return nil, errMRUnexpected
		}
		for _, id := range ok.UUIDs404 {
			if _, valid := protocol.ServiceIDToBinary(id); !valid {
				return nil, errMRUnexpected
			}
		}
		return &mrResult{Unregistered: ok.UUIDs404}, nil
	case 401:
		return nil, errMRUnauthorized
	case 409, 410:
		var entries []struct {
			UUID    string `json:"uuid"`
			Devices struct {
				Missing []json.RawMessage `json:"missingDevices"`
				Extra   []json.RawMessage `json:"extraDevices"`
				Stale   []json.RawMessage `json:"staleDevices"`
			} `json:"devices"`
		}
		if body == nil || json.Unmarshal(body, &entries) != nil {
			return nil, errMRUnexpected
		}
		res := &mrResult{}
		for _, e := range entries {
			if _, ok := protocol.ServiceIDToBinary(e.UUID); !ok {
				return nil, errMRUnexpected
			}
			missing, ok1 := validDeviceIDs(e.Devices.Missing)
			extra, ok2 := validDeviceIDs(e.Devices.Extra)
			stale, ok3 := validDeviceIDs(e.Devices.Stale)
			if !ok1 || !ok2 || !ok3 || len(missing)+len(extra)+len(stale) == 0 {
				return nil, errMRUnexpected
			}
			res.Mismatches = append(res.Mismatches, mismatch{Account: e.UUID, Missing: missing, Extra: extra, Stale: stale})
		}
		return res, nil
	}
	if status == 429 || status >= 500 {
		return nil, &mrServerError{Status: status}
	}
	return nil, errMRUnexpected
}
