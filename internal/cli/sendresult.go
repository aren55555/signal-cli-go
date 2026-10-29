// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"fmt"
	"strings"

	"github.com/aren55555/signal-cli-go/internal/errcode"
)

// Result kinds (spec/surface/cli/_send-results.md).
type resultKind int

const (
	resultSuccess resultKind = iota
	resultNetwork
	resultUnregistered
	resultUntrusted
	resultRateLimit
	resultInvalidPreKey
)

var resultType = map[resultKind]string{
	resultSuccess: "SUCCESS", resultNetwork: "NETWORK_FAILURE", resultUnregistered: "UNREGISTERED_FAILURE",
	resultUntrusted: "IDENTITY_FAILURE", resultRateLimit: "RATE_LIMIT_FAILURE", resultInvalidPreKey: "INVALID_PRE_KEY_FAILURE",
}

// sendResult is one recipient's outcome.
type sendResult struct {
	key                   string // the recipient as the user named it, normalised
	groupID               string // base64, for results of a group recipient
	aci, number, username string
	kind                  resultKind
	retryAfterMs          int64
}

func (r sendResult) legacyID() string {
	for _, s := range []string{r.number, r.aci, r.username} {
		if s != "" {
			return s
		}
	}
	return ""
}

func ceilSeconds(ms int64) int64 { return (ms + 999) / 1000 }

// renderSendResults returns stdout, the exit code and the stderr message
// for a send (plain text or JSON).
func renderSendResults(asJSON bool, ts uint64, results []sendResult) (string, int, string) {
	var out strings.Builder
	if asJSON {
		top := &jsonObject{}
		top.add("timestamp", ts)
		top.add("results", resultsJSON(results))
		out.WriteString(top.String() + "\n")
	} else {
		var failures []string
		for _, r := range results {
			var msg string
			switch r.kind {
			case resultNetwork:
				msg = fmt.Sprintf("Network failure for %q", r.legacyID())
			case resultRateLimit:
				if r.retryAfterMs > 0 {
					msg = fmt.Sprintf("Rate limit failure for %q, retry after %d seconds", r.legacyID(), ceilSeconds(r.retryAfterMs))
				} else {
					msg = fmt.Sprintf("Rate limit failure for %q", r.legacyID())
				}
			case resultUnregistered:
				msg = fmt.Sprintf("Unregistered user %q", r.legacyID())
			case resultUntrusted:
				msg = fmt.Sprintf("Untrusted Identity for %q", r.legacyID())
			default:
				continue
			}
			failures = append(failures, r.key+": "+msg)
		}
		if len(failures) > 0 {
			out.WriteString("Failed to send (some) messages:\n")
			for _, f := range failures {
				out.WriteString(f + "\n")
			}
		}
		fmt.Fprintf(&out, "%d\n", ts)
	}
	if len(results) == 0 {
		return out.String(), 0, ""
	}
	allUntrusted, allRateLimit := true, true
	for _, r := range results {
		if r.kind == resultSuccess {
			return out.String(), 0, ""
		}
		allUntrusted = allUntrusted && r.kind == resultUntrusted
		allRateLimit = allRateLimit && r.kind == resultRateLimit
	}
	switch {
	case allUntrusted:
		return out.String(), 4, "Failed to send message due to untrusted identities"
	case allRateLimit:
		return out.String(), 5, "Failed to send message due to rate limiting"
	}
	return out.String(), 1, "Failed to send message"
}

// resultsJSON is the JSON array of SendMessageResult objects.
func resultsJSON(results []sendResult) rawJSON {
	var items []string
	for _, r := range results {
		o := &jsonObject{}
		addr := &jsonObject{}
		addr.add("uuid", strOrNull(r.aci))
		addr.add("number", strOrNull(r.number))
		addr.add("username", strOrNull(r.username))
		o.add("recipientAddress", addr)
		if r.groupID != "" {
			o.add("groupId", r.groupID)
		}
		o.add("type", resultType[r.kind])
		if r.kind == resultRateLimit && r.retryAfterMs > 0 {
			o.add("retryAfterSeconds", ceilSeconds(r.retryAfterMs))
		}
		items = append(items, o.String())
	}
	return rawJSON("[" + strings.Join(items, ",") + "]")
}

// sendFailure is the error for a send whose results all failed: exit
// code 4 when every failure was an untrusted identity, 5 when every one
// was a rate limit, else 1 (msg is the matching reference text).
func sendFailure(code int, msg string) error {
	switch code {
	case 4:
		return errcode.SendUntrusted
	case 5:
		return errcode.SendRateLimited
	}
	return errcode.SendFailed
}
