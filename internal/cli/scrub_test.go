// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import "testing"

// The worked examples of spec/surface/cli/logging.md §6.4.
func TestScrubLog(t *testing.T) {
	for _, c := range [][2]string{
		{"Sending to +4915112345678 now", "Sending to +***********78 now"},
		{"+12345678 and +1234567 and +123456 and +1234567890123456789", "+******78 and +*****67 and +123456 and +*************456789"},
		{"file account_4915112345678.db %2B4915112345678 %2b4915112345678", "file account_***********78.db %2B***********78 %2b4915112345678"},
		{"uuid 2c2a7cd9-5a8b-4e1f-9a0c-3b4d5e6f7a8b done", "uuid ********-****-****-****-**********8b done"},
		{"UUID 2C2A7CD9_5A8B_4E1F_9A0C_3B4D5E6F7A8B JOB::2c2a7cd9-5a8b-4e1f-9a0c-3b4d5e6f7a8b", "UUID ********-****-****-****-**********8B JOB::2c2a7cd9-5a8b-4e1f-9a0c-3b4d5e6f7a8b"},
		{"Mail alice@example.com, contact /home/u/bob@mail.de x", "Mail a...@... contact /home/u/b...@... x"},
		{`<alice@example.com> and "x":"y@z"`, `<a...@... and "x...@...`},
		{"group abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQ= v1 abcdefghijklmnopqrstuv==", "group ab***************************************PQ v1 ab******************uv"},
		{"path /org/asamk/Signal/_4915112345678/Groups/abcdefghijklmnopqrstuv__", "path /org/asamk/Signal/_***********78/Groups/ab******************uv"},
		{"Connecting to chat.signal.org and storage.example.com:443 and cdn.whispersystems.org", "Connecting to chat.signal.org and ***.com:443 and cdn.whispersystems.org"},
		{"www.Example.CO.uk foo-bar.com my_site.de example.com-foo chat.Signal.org App org.asamk.signal.App notsignal.org", "***.uk foo-***.com my_***.de example.com-foo ***.org App ***.App notsignal.org"},
		{"ip 192.168.1.20 and 10.0.0.1:8080 and 256.1.1.1 and 1.2.3.4.5", "ip ...ipv4... and ...ipv4...:8080 and 256.1.1.1 and ...ipv4....5"},
		{"2026-10-29T09:22:29.953Z [main] DEBUG org.asamk.signal.util.IOUtils - XDG_RUNTIME_DIR not set", "2026-10-29T09:22:29.953Z [main] DEBUG org.asamk.signal.util.IOUtils - XDG_RUNTIME_DIR not set"},
	} {
		if got := scrubLog(c[0]); got != c[1] {
			t.Errorf("%q:\n got %q\nwant %q", c[0], got, c[1])
		}
	}
}
