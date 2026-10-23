// Package repocheck holds tests that enforce repository-wide rules from
// PROMPT.md: no third-party modules, and no secrets in testdata/.
package repocheck

import (
	"bufio"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const repoRoot = "../.."

// TestGoModHasNoRequires enforces the standard-library-only rule.
func TestGoModHasNoRequires(t *testing.T) {
	var mods []string
	err := filepath.WalkDir(repoRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && (d.Name() == ".git" || d.Name() == "third_party") {
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
	if len(mods) == 0 {
		t.Fatal("no go.mod found")
	}
	for _, mod := range mods {
		f, err := os.Open(mod)
		if err != nil {
			t.Fatal(err)
		}
		sc := bufio.NewScanner(f)
		for n := 1; sc.Scan(); n++ {
			if line := strings.TrimSpace(sc.Text()); strings.HasPrefix(line, "require") {
				t.Errorf("%s:%d: %q: third-party modules are not allowed", mod, n, line)
			}
		}
		f.Close()
	}
	if _, err := os.Stat(filepath.Join(repoRoot, "go.sum")); err == nil {
		t.Error("go.sum exists; there must be no module dependencies")
	}
}

// secretPatterns flag likely real identifiers. Golden transcripts must use
// the reserved placeholders documented in testdata/README.md instead.
var secretPatterns = []struct {
	name string
	re   *regexp.Regexp
}{
	// E.164 numbers. The fictional +1-555-01xx range is allowed below.
	{"phone number", regexp.MustCompile(`\+[1-9][0-9]{7,14}\b`)},
	{"captcha token", regexp.MustCompile(`signalcaptcha://`)},
	{"PEM private key", regexp.MustCompile(`BEGIN [A-Z ]*PRIVATE KEY`)},
	{"Anthropic API key", regexp.MustCompile(`sk-ant-`)},
	// A 32-byte value in base64 next to a key-ish JSON name.
	{"key material", regexp.MustCompile(`(?i)"[a-z]*(private|secret|password|pin)[a-z]*"\s*:\s*"[A-Za-z0-9+/=_-]{20,}"`)},
}

// allowedPhone is the NANP range reserved for fiction, +1 NPA 555-0100..0199.
var allowedPhone = regexp.MustCompile(`^\+1[2-9][0-9]{2}55501[0-9]{2}$`)

// TestTestdataHasNoSecrets scans testdata/ for phone numbers and keys.
// Test vectors from published standards are exempt only for the key
// material pattern, and only under testdata/vectors/.
func TestTestdataHasNoSecrets(t *testing.T) {
	root := filepath.Join(repoRoot, "testdata")
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		inVectors := strings.HasPrefix(rel, "vectors"+string(filepath.Separator))
		for _, p := range secretPatterns {
			if inVectors && p.name == "key material" {
				continue
			}
			for _, m := range p.re.FindAllString(string(data), -1) {
				if p.name == "phone number" && allowedPhone.MatchString(m) {
					continue
				}
				t.Errorf("%s: possible %s: %q", rel, p.name, m)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestSecretPatternsFire(t *testing.T) {
	bad := []string{`"number":"+14155552671"`, `signalcaptcha://abc`, `"pinHash":"AAAAAAAAAAAAAAAAAAAAAAAAAAAA"`}
	for _, s := range bad {
		hit := false
		for _, p := range secretPatterns {
			for _, m := range p.re.FindAllString(s, -1) {
				if !(p.name == "phone number" && allowedPhone.MatchString(m)) {
					hit = true
				}
			}
		}
		if !hit {
			t.Errorf("no pattern matched %q", s)
		}
	}
	for _, ok := range []string{"+12025550100", "+12025550199"} {
		if !allowedPhone.MatchString(ok) {
			t.Errorf("placeholder %s should be allowed", ok)
		}
	}
}
