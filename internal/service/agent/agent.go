// SPDX-License-Identifier: GPL-3.0-or-later

// Package agent builds signal-cli's agent strings
// (spec/service/transport.md §4.2).
package agent

// EnvOverride names the environment variable whose value replaces the
// first token.
const EnvOverride = "SIGNAL_CLI_USER_AGENT"

// String is the REST User-Agent and X-Signal-Agent: the first token
// ("Signal-Android/8.21.1", or override if non-empty) and
// "signal-cli/0.14.7".
func String(override string) string {
	first := "Signal-Android/8.21.1"
	if override != "" {
		first = override
	}
	return first + " signal-cli/0.14.7"
}

// Chat is the User-Agent on the libsignal-net chat sockets.
func Chat(override string) string { return String(override) + " libsignal/0.99.1" }
