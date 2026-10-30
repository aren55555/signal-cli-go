// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aren55555/signal-cli-go/internal/errcode"
	"github.com/aren55555/signal-cli-go/internal/store"
)

func writeIndex(t *testing.T, dataDir string, entries ...[2]string) {
	t.Helper()
	os.MkdirAll(filepath.Join(dataDir, "data"), 0o700)
	s := "{\n  \"accounts\" : [ "
	for i, e := range entries {
		if i > 0 {
			s += ", "
		}
		s += "{\n    \"path\" : \"" + e[0] + "\",\n    \"environment\" : \"LIVE\",\n    \"number\" : \"" + e[1] + "\",\n    \"uuid\" : null\n  }"
	}
	if len(entries) > 0 {
		s += " "
	}
	s += "],\n  \"version\" : 2\n}\n"
	os.WriteFile(filepath.Join(dataDir, "data", "accounts.json"), []byte(s), 0o600)
}

// TestSelectAccount: index.md §2 "Account selection rules".
func TestSelectAccount(t *testing.T) {
	empty := t.TempDir()
	two := t.TempDir()
	writeIndex(t, two, [2]string{"111111", "+15555550111"}, [2]string{"222222", "+15555550122"})
	for _, tc := range []struct {
		name, dir, account string
		local              bool
		wantErr            string
		wantCode           int
	}{
		{"none", empty, "", true, "No local users found, you first need to register or link an account", 1},
		{"multiple", two, "", true, "Multiple users found, you need to specify an account (phone number) with -a", 1},
		{"invalid", two, "12345", true, "Invalid account (phone number), make sure you include the country code.", 1},
		{"not registered", empty, "+15555550123", true, "User +15555550123 is not registered.", 1},
		{"foreign account", two, "+15555550111", true, "User +15555550111 is not registered.", 1},
		{"uuid unknown", empty, "0b6ab9b6-0000-4000-8000-00000000c5d3", true, "User 0b6ab9b6-0000-4000-8000-00000000c5d3 is not registered.", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := selectAccount(&globals{serviceEnv: "live"}, store.Open(filepath.Join(tc.dir, "data")), tc.account) // the index entries are LIVE
			if err == nil || errcode.Message(err) != tc.wantErr || errcode.ExitCode(err) != tc.wantCode {
				t.Errorf("got %v, want %q (%d)", err, tc.wantErr, tc.wantCode)
			}
		})
	}
}

func TestValidAccountNumber(t *testing.T) {
	for in, want := range map[string]bool{"+15555550123": true, "+4915112345678": true, "12345": false, "+1": false, "+0123456789": false, "abc": false} {
		if got := validNumber(in); got != want {
			t.Errorf("%q: %v", in, got)
		}
	}
}

// TestAccountsOfOtherEnvironmentsAreInvisible: an index entry belongs to
// the run only if its environment is the run's or unset (data-dir §2.2).
func TestAccountsOfOtherEnvironmentsAreInvisible(t *testing.T) {
	entries := []store.IndexEntry{{Path: "1", Environment: "LIVE"}, {Path: "2", Environment: "STAGING"}, {Path: "3"}}
	for env, want := range map[string]string{"staging": "2,3", "live": "1,3"} {
		var got []string
		for _, e := range visible(&globals{serviceEnv: env}, entries) {
			got = append(got, e.Path)
		}
		if strings.Join(got, ",") != want {
			t.Errorf("%s: visible %v, want %s", env, got, want)
		}
	}
	two := t.TempDir()
	writeIndex(t, two, [2]string{"111111", "+15555550111"}, [2]string{"222222", "+15555550122"})
	_, err := selectAccount(&globals{serviceEnv: "staging"}, store.Open(filepath.Join(two, "data")), "")
	if errcode.Message(err) != "No local users found, you first need to register or link an account" {
		t.Errorf("LIVE accounts seen from a staging run: %v", err)
	}
}
