package cli

import (
	"testing"
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
