package agent

import "testing"

// spec/service/transport.md §4.2.
func TestAgentStrings(t *testing.T) {
	if got := String(""); got != "Signal-Android/8.21.1 signal-cli/0.14.7" {
		t.Errorf("default agent %q", got)
	}
	if got := String("Custom/1"); got != "Custom/1 signal-cli/0.14.7" {
		t.Errorf("override %q", got)
	}
	if got := Chat(""); got != "Signal-Android/8.21.1 signal-cli/0.14.7 libsignal/0.99.1" {
		t.Errorf("chat agent %q", got)
	}
}
