// SPDX-License-Identifier: GPL-3.0-or-later

// Package repocheck holds tests that enforce repository-wide rules from
// PROMPT.md as amended (PLAN.md, Decisions): only the pinned, vendored
// modules (protobuf everywhere, modernc.org/sqlite and its modules in the
// importer only), no C in any shipped binary, no secrets in the tree or in
// any commit (secrets_test.go),
// and the GPLv3 license header on every Go file we wrote.
package repocheck

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

const repoRoot = "../.."

// The main binary's one permitted module (brief amendment, 2026-10-23),
// pinned so that any upgrade is a deliberate edit here.
const (
	allowedModule  = "google.golang.org/protobuf"
	allowedVersion = "v1.36.12"
)

// allowedModules is every permitted module at its pinned version:
// protobuf, plus modernc.org/sqlite and its transitive modules for the
// importer only (second amendment, 2026-10-29). go.mod, go.sum and
// vendor/modules.txt must list exactly these.
var allowedModules = map[string]string{
	allowedModule: allowedVersion,

	// Importer only (cmd/signal-cli-go-import).
	"modernc.org/sqlite":               "v1.61.0",
	"modernc.org/libc":                 "v1.79.0",
	"modernc.org/mathutil":             "v1.7.1",
	"modernc.org/memory":               "v1.12.1",
	"golang.org/x/sys":                 "v0.49.0",
	"github.com/dustin/go-humanize":    "v1.1.0",
	"github.com/google/uuid":           "v1.6.0",
	"github.com/mattn/go-isatty":       "v0.0.24",
	"github.com/ncruces/go-strftime":   "v1.1.0",
	"github.com/remyoudompheng/bigfft": "v0.0.0-20230129092748-24d4a6f8daec",
}

func sortedAllowed() []string {
	var out []string
	for m, v := range allowedModules {
		out = append(out, m+" "+v)
	}
	slices.Sort(out)
	return out
}

// requires returns the "module version" pairs of a go.mod's require
// directives, in both the single-line and the block form.
func requires(gomod string) []string {
	var out []string
	inBlock := false
	for _, line := range strings.Split(gomod, "\n") {
		line = strings.TrimSpace(line)
		if i := strings.Index(line, "//"); i >= 0 {
			line = strings.TrimSpace(line[:i])
		}
		switch {
		case inBlock && line == ")":
			inBlock = false
		case inBlock && line != "":
			out = append(out, strings.Join(strings.Fields(line)[:2], " "))
		case line == "require (":
			inBlock = true
		case strings.HasPrefix(line, "require "):
			out = append(out, strings.Join(strings.Fields(line)[1:3], " "))
		}
	}
	return out
}

func TestRequiresParser(t *testing.T) {
	mod := "module x\n\nrequire a.b/c v1.0.0 // indirect\nrequire (\n\td.e/f v2.0.0\n\t// comment\n\tg.h/i v3.0.0 // indirect\n)\n"
	got := strings.Join(requires(mod), ",")
	if want := "a.b/c v1.0.0,d.e/f v2.0.0,g.h/i v3.0.0"; got != want {
		t.Errorf("requires = %q, want %q", got, want)
	}
}

// TestOnlyAllowedModule enforces the dependency rule on every go.mod,
// go.sum and vendor/modules.txt.
func TestOnlyAllowedModule(t *testing.T) {
	var mods []string
	err := filepath.WalkDir(repoRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && (d.Name() == ".git" || d.Name() == "third_party" || d.Name() == "vendor") {
			return filepath.SkipDir
		}
		if d.Name() == "go.mod" {
			mods = append(mods, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(repoRoot, "go.mod")
	want := sortedAllowed()
	for _, mod := range mods {
		data, err := os.ReadFile(mod)
		if err != nil {
			t.Fatal(err)
		}
		reqs := requires(string(data))
		slices.Sort(reqs)
		if filepath.Clean(mod) == filepath.Clean(root) {
			if !slices.Equal(reqs, want) {
				t.Errorf("%s requires %q; exactly %q is allowed", mod, reqs, want)
			}
		} else if len(reqs) != 0 {
			t.Errorf("%s requires %q; only the root module may require modules", mod, reqs)
		}
	}

	sum, err := os.ReadFile(filepath.Join(repoRoot, "go.sum"))
	if err != nil {
		t.Fatal(err)
	}
	summed := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(string(sum)), "\n") {
		f := strings.Fields(line)
		if len(f) != 3 || (f[1] != allowedModules[f[0]] && f[1] != allowedModules[f[0]]+"/go.mod") {
			t.Errorf("go.sum: unexpected line %q", line)
			continue
		}
		summed[f[0]+" "+f[1]] = true
	}
	for m, v := range allowedModules {
		if !summed[m+" "+v] || !summed[m+" "+v+"/go.mod"] {
			t.Errorf("go.sum lacks the hashes of %s %s", m, v)
		}
	}

	vend, err := os.ReadFile(filepath.Join(repoRoot, "vendor", "modules.txt"))
	if err != nil {
		t.Fatal(err)
	}
	var vendored []string
	for _, line := range strings.Split(string(vend), "\n") {
		if m, ok := strings.CutPrefix(line, "# "); ok {
			vendored = append(vendored, m)
		}
	}
	slices.Sort(vendored)
	if !slices.Equal(vendored, want) {
		t.Errorf("vendor/modules.txt lists %q; exactly %q is allowed", vendored, want)
	}
}

const spdxLine = "// SPDX-License-Identifier: GPL-3.0-or-later"

var generatedRe = regexp.MustCompile(`(?m)^// Code generated .* DO NOT EDIT\.$`)

// TestLicenseHeaders: every Go file we wrote starts with the SPDX
// line (operator decision 2026-10-28: GPLv3). vendor/, third_party/ and
// generated files are exempt.
func TestLicenseHeaders(t *testing.T) {
	n := 0
	err := filepath.WalkDir(repoRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(repoRoot, path)
		if d.IsDir() {
			if rel != "." && (strings.HasPrefix(d.Name(), ".") || rel == "vendor" || rel == "third_party") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if generatedRe.Match(data) {
			return nil
		}
		n++
		if first, _, _ := strings.Cut(string(data), "\n"); first != spdxLine {
			t.Errorf("%s: first line must be %q", rel, spdxLine)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if n < 100 {
		t.Fatalf("only %d Go files checked", n)
	}
	lic, err := os.ReadFile(filepath.Join(repoRoot, "LICENSE"))
	if err != nil || !strings.Contains(string(lic), "GNU GENERAL PUBLIC LICENSE\n                       Version 3, 29 June 2007") {
		t.Errorf("LICENSE is missing or not the GPLv3 text")
	}
}
