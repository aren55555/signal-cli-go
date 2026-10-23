package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAllowlistMatching(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ALLOWED_HOSTS")
	content := "# comment\napi.example.com   # trailing comment\n*.signal.org\n\nGitHub.com.\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	al, err := loadAllowlist(path)
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]bool{
		"api.example.com":         true,
		"API.EXAMPLE.COM.":        true,
		"example.com":             false,
		"notapi.example.com":      false,
		"chat.staging.signal.org": true,
		"signal.org":              false, // "*." requires a subdomain
		"evilsignal.org":          false,
		"github.com":              true,
		"":                        false,
	}
	for host, want := range cases {
		if got := al.allows(host); got != want {
			t.Errorf("allows(%q) = %v, want %v", host, got, want)
		}
	}
}

func TestEmptyAllowlistRefused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ALLOWED_HOSTS")
	if err := os.WriteFile(path, []byte("# nothing here\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadAllowlist(path); err == nil {
		t.Fatal("expected an error for an empty allowlist")
	}
}
