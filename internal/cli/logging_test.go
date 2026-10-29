// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The worked examples of spec/surface/cli/logging.md §3.
func TestAbbreviateLogger(t *testing.T) {
	for in, want := range map[string]string{
		"org.asamk.signal.App":                                      "org.asamk.signal.App",
		"org.asamk.signal.util.IOUtils":                             "org.asamk.signal.util.IOUtils",
		"org.asamk.signal.manager.util.Utils":                       "org.asamk.signal.manager.util.Utils",
		"org.asamk.signal.dbus.DbusSignalImpl":                      "o.asamk.signal.dbus.DbusSignalImpl",
		"org.asamk.signal.commands.BlockCommand":                    "o.asamk.signal.commands.BlockCommand",
		"org.asamk.signal.manager.helper.ReceiveHelper":             "o.a.s.manager.helper.ReceiveHelper",
		"org.asamk.signal.manager.helper.UnidentifiedAccessHelper":  "o.a.s.m.h.UnidentifiedAccessHelper",
		"org.asamk.signal.manager.storage.groups.LegacyGroupStore":  "o.a.s.m.s.groups.LegacyGroupStore",
		"org.asamk.signal.manager.internal.ProvisioningManagerImpl": "o.a.s.m.i.ProvisioningManagerImpl",
		"org.asamk.signal.manager.helper.SyncHelper":                "o.a.signal.manager.helper.SyncHelper",
		"LibSignalService":                                          "LibSignalService",
	} {
		if got := abbreviateLogger(in); got != want {
			t.Errorf("%s: %s, want %s", in, got, want)
		}
	}
}

// Console and file routing (logging.md §5.2).
func TestLogRouting(t *testing.T) {
	fixed := func() time.Time { return time.Date(2026, 10, 29, 9, 22, 29, 953e6, time.UTC) }
	run := func(s logSettings, lines ...string) (string, string) {
		var console strings.Builder
		l := newLogger(s, &console)
		l.now = fixed
		w := &logWriter{l: l, out: &console}
		for _, line := range lines {
			w.Write([]byte(line + "\n"))
		}
		file := ""
		if s.logFile != "" {
			b, _ := os.ReadFile(s.logFile)
			file = string(b)
		}
		return console.String(), file
	}
	in := []string{"WARN  ReceiveHelper - oops +4915112345678", "DEBUG App - Starting signal-cli 0.14.7", "Failed to send message"}
	c, _ := run(logSettings{}, in...)
	if c != "WARN  ReceiveHelper - oops +4915112345678\nFailed to send message\n" {
		t.Errorf("V=0: %q", c)
	}
	c, _ = run(logSettings{verbose: 1, scrub: true}, in...)
	want := "2026-10-29T09:22:29.953Z [main] WARN  o.a.s.manager.helper.ReceiveHelper - oops +***********78\n" +
		"2026-10-29T09:22:29.953Z [main] DEBUG ***.App - Starting signal-cli 0.14.7\nFailed to send message\n"
	if c != want {
		t.Errorf("V=1 scrubbed:\n%q\nwant\n%q", c, want)
	}
	f := filepath.Join(t.TempDir(), "sub", "n.log")
	os.MkdirAll(filepath.Dir(f), 0o755)
	os.WriteFile(f, []byte("preexisting\n"), 0o644)
	c, file := run(logSettings{verbose: 1, logFile: f}, in...)
	if c != "WARN  ReceiveHelper - oops +4915112345678\nFailed to send message\n" {
		t.Errorf("V=1 with file, console: %q", c)
	}
	if !strings.HasPrefix(file, "preexisting\n2026-10-29T09:22:29.953Z [main] WARN  o.a.s.manager.helper.ReceiveHelper") || !strings.Contains(file, "DEBUG org.asamk.signal.App") {
		t.Errorf("file: %q", file)
	}
}
