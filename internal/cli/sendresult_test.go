// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"testing"

	"github.com/aren55555/signal-cli-go/internal/store"
)

// TestSendResults: _send-results.md, plain and JSON, and exit codes.
func TestSendResults(t *testing.T) {
	ok := []sendResult{{key: "+15555550101", aci: "b0b0b0b0-0000-4000-8000-000000000002", number: "+15555550101", kind: resultSuccess}}
	if out, code, errMsg := renderSendResults(false, 1700000000000, ok); out != "1700000000000\n" || code != 0 || errMsg != "" {
		t.Errorf("plain ok: %q %d %q", out, code, errMsg)
	}
	if out, _, _ := renderSendResults(true, 1700000000000, ok); out != `{"timestamp":1700000000000,"results":[{"recipientAddress":{"uuid":"b0b0b0b0-0000-4000-8000-000000000002","number":"+15555550101","username":null},"type":"SUCCESS"}]}`+"\n" {
		t.Errorf("json ok: %s", out)
	}
	bad := []sendResult{{key: "b0b0b0b0-0000-4000-8000-000000000002", aci: "b0b0b0b0-0000-4000-8000-000000000002", kind: resultUntrusted}}
	out, code, errMsg := renderSendResults(false, 5, bad)
	if out != "Failed to send (some) messages:\nb0b0b0b0-0000-4000-8000-000000000002: Untrusted Identity for \"b0b0b0b0-0000-4000-8000-000000000002\"\n5\n" ||
		code != 4 || errMsg != "Failed to send message due to untrusted identities" {
		t.Errorf("untrusted: %q %d %q", out, code, errMsg)
	}
	rl := []sendResult{{key: "+15555550101", number: "+15555550101", kind: resultRateLimit, retryAfterMs: 1500}}
	out, code, _ = renderSendResults(true, 5, rl)
	if out != `{"timestamp":5,"results":[{"recipientAddress":{"uuid":null,"number":"+15555550101","username":null},"type":"RATE_LIMIT_FAILURE","retryAfterSeconds":2}]}`+"\n" || code != 5 {
		t.Errorf("rate limit: %s %d", out, code)
	}
	if out, code, _ := renderSendResults(false, 5, []sendResult{{key: "+1", number: "+1", kind: resultUnregistered}}); out != "Failed to send (some) messages:\n+1: Unregistered user \"+1\"\n5\n" || code != 1 {
		t.Errorf("unregistered: %q %d", out, code)
	}
}

// TestResultForCarriesNumber: a 1:1 result's address has the recipient's
// phone number when the store knows it, as signal-cli's does (found side
// by side on production: signal-cli-go said "number":null).
func TestResultForCarriesNumber(t *testing.T) {
	const aci = "b0b0b0b0-0000-4000-8000-000000000002"
	a := &store.Account{Recipients: map[string]store.Recipient{aci: {Number: "+15555550101"}}}
	r := resultFor(a, "+15555550101", aci, nil)
	if got := string(resultsJSON([]sendResult{r})); got != `[{"recipientAddress":{"uuid":"`+aci+`","number":"+15555550101","username":null},"type":"SUCCESS"}]` {
		t.Errorf("known number: %s", got)
	}
	r = resultFor(a, "c0c0c0c0-0000-4000-8000-000000000003", "c0c0c0c0-0000-4000-8000-000000000003", nil)
	if r.number != "" {
		t.Errorf("unknown recipient got number %q", r.number)
	}
}
