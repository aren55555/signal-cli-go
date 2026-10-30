// SPDX-License-Identifier: GPL-3.0-or-later

package repocheck

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// The secret scan (operator decision 2026-10-30): the working tree and
// every blob and commit message reachable from `git rev-list --all` must
// not contain phone numbers, Signal ACIs or PNIs, captcha tokens or
// private-key encodings. testdata/vectors/ (published and derived test
// vectors, cited there and in CLEANROOM.md), the published test numbers
// and the exact values in knownValues are exempt.

// secretPatterns flag likely real identifiers. Golden transcripts must use
// the reserved placeholders documented in testdata/README.md instead.
var secretPatterns = []struct {
	name string
	re   *regexp.Regexp
}{
	// E.164 numbers. The fictional +1 NPA 555-01xx range is allowed.
	{"phone number", regexp.MustCompile(`\+[1-9][0-9]{7,14}\b`)},
	// ACIs and PNIs are random version-4 UUIDs, plain or "PNI:" + UUID.
	{"Signal ACI/PNI", regexp.MustCompile(`(?i)\b[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}\b`)},
	// A captcha token: the signalcaptcha:// scheme followed by a token (the
	// bare scheme name in docs and parsers is fine), or an hCaptcha response.
	{"captcha token", regexp.MustCompile(`signalcaptcha:\/\/[A-Za-z0-9._-]{20,}|\bP1_eyJ[A-Za-z0-9_.-]{20,}`)},
	{"PEM private key", regexp.MustCompile(`BEGIN [A-Z ]*PRIVATE KEY`)},
	// A 32-byte value (base64 with padding, or 64 hex digits) shortly after
	// a key-ish name, in JSON, Go or prose.
	{"private key", regexp.MustCompile(`(?i)(?:priv(?:ate)?[_ ]?key|privkey|secret[_ ]?key|identity[_ ]?private|master[_ ]?key|signing[_ ]?key|seed)[A-Za-z_]*["'\x60]?\s*[:=,(]+\s*(?:[A-Za-z.]+\()?["'\x60]([A-Za-z0-9+/]{43}=|[0-9a-fA-F]{64})["'\x60]`)},
	{"key material", regexp.MustCompile(`(?i)"[a-z]*(?:private|secret|password|pin)[a-z]*"\s*:\s*"([A-Za-z0-9+/=_-]{20,})"`)},
	{"Anthropic API key", regexp.MustCompile(`sk\-ant\-[A-Za-z0-9_-]{20,}`)},
}

// allowedPhone is the NANP range reserved for fiction, +1 NPA 555-0100..0199.
var allowedPhone = regexp.MustCompile(`^\+1[2-9][0-9]{2}55501[0-9]{2}$`)

// publishedTestPhones are numbers from published upstream test vectors,
// kept verbatim so the vectors stay faithful to their source: libsignal
// v0.99.1 rust/net/src/cdsi.rs (serialize_e164s and the response-parsing
// test, testdata/vectors/cdsi.json) and rust/protocol/src/fingerprint.rs
// tests (testdata/vectors/fingerprint.json).
var publishedTestPhones = map[string]bool{
	"+18005551001": true, "+18005551002": true, "+18005551003": true, "+18005551004": true, "+18005551005": true,
	"+14152222222": true, "+14153333333": true, "+141512222222": true,
}

// knownValues are values outside testdata/vectors/ that match a pattern
// but were checked and are not secrets; each says where it comes from.
var knownValues = map[string]string{
	"258EAFA5-E914-47DA-95CA-C5AB0DC85B11": "RFC 6455 section 1.3 WebSocket GUID, a public constant",
	"939a7233-f79c-4ca9-940a-0db3957f0607": "Intel's SGX QE vendor ID (spec/enclave/attestation.md), a public constant",
	"3fa85f64-5717-4562-b3fc-2c963f66afa6": "the OpenAPI/Swagger example UUID, used as a sample argument in spec/surface/cli/observations.md",
	"2c2a7cd9-5a8b-4e1f-9a0c-3b4d5e6f7a8b": "a made-up UUID in the --scrub-log worked examples (spec/surface/cli/logging.md §6.4, internal/cli/scrub_test.go)",
	"+4915112345678":                       "conventional placeholder with sequential digits in the --scrub-log worked examples (logging.md §6.4) and the logging tests",
	"+12345678":                            "too short to be a valid number; a --scrub-log length example (logging.md §6.4)",
	"+15550000000":                         "area code 555 is unassigned; the placeholder number of the JSON-RPC surface specs",
	// The example account of spec/storage/signal-cli-data-dir.md and the
	// importer's test fixture: an unregistered scratch account created
	// offline with a fictional number and throwaway keys (CLEANROOM.md,
	// 2026-10-29).
	"oPyqJvqfMWt6HWpUkhY1galZGhU63s0BrlhF0UicEmk=": "throwaway identity private key of the offline scratch account (signal-cli-data-dir.md §3.2)",
	"UbffUm+Sy5hGlL2nQhUfLhvy":                     "throwaway password of the offline scratch account (signal-cli-data-dir.md §3.2)",
	"Ve3mYvM90mir28jVHNx80Bgl4c8wLbv9JPAUU5tfe0M=": "recovery password derived from the fictional master key 0x20..0x3f of testdata/vectors/pin.json (spec/service/svr.md example)",
	"signalcaptcha://signal-hcaptcha.TOKEN":        "a placeholder token in internal/manager/register_test.go",
	"8a5f0e4b-1b2c-4d3e-9f00-123456789abc":         "a made-up account UUID (sequential digits) in the JSON-RPC observations and goldens",
	// Vendored third-party code: numeric constants and a mail Message-ID.
	"+253402300799":                        "protobuf's largest Timestamp seconds (vendor, protoc-gen-go docs)",
	"+99999999":                            "protobuf's largest Timestamp nanos (vendor, protoc-gen-go docs)",
	"+2147483647":                          "int32 max in the transpiled SQLite (vendor)",
	"b9624545-2c80-49a1-ac3c-39264a591f7b": "a lore.kernel.org Message-ID in a comment of modernc.org/libc (vendor)",
}

// historyOnly are values that must not appear in the tree any more but
// are in commits that cannot be rewritten.
var historyOnly = map[string]string{
	// Assembled, so this file itself stays clean.
	strings.Join([]string{"244db90c", "c537", "4b7d", "8024", "76fd4ca4e3d9"}, "-"): "a staging sender-key distribution ID (a group's sender-key ID, not an ACI or PNI) copied from the 2026-10-29 interop run into internal/cli/render_test.go in 29a50ee; replaced with a fictional UUID",
	// Samples of the earlier TestSecretPatternsFire (repocheck_test.go before
	// 2026-10-30), which spelled its bad examples out.
	"+1" + "4155552671":     "the libphonenumber example number, a sample in the old pattern test",
	strings.Repeat("A", 28): "an all-A sample in the old pattern test",
}

// fictionalUUID reports a UUID that is evidently made up: test values use
// runs of zeros or few distinct digits, which random ACIs and PNIs almost
// never show (a run of four zeros has odds of about 1 in 2,000).
func fictionalUUID(s string) bool {
	h := strings.ToLower(strings.ReplaceAll(s, "-", ""))
	if strings.Contains(h, "0000") {
		return true
	}
	distinct := map[rune]bool{}
	for _, c := range h {
		distinct[c] = true
	}
	return len(distinct) < 8
}

type finding struct{ where, rule, match string }

// scan applies the patterns to one file's content.
func scan(path string, data []byte, history bool) []finding {
	if strings.HasPrefix(path, "testdata/vectors/") || bytes.IndexByte(data, 0) >= 0 {
		return nil // exempt, or binary
	}
	var out []finding
	text := string(data)
	for _, p := range secretPatterns {
		for _, sm := range p.re.FindAllStringSubmatch(text, -1) {
			m := sm[0]
			if len(sm) > 1 && sm[1] != "" {
				m = sm[1]
			}
			if _, ok := knownValues[m]; ok {
				continue
			}
			if _, ok := historyOnly[m]; ok && history {
				continue
			}
			switch p.name {
			case "phone number":
				if allowedPhone.MatchString(m) || publishedTestPhones[m] {
					continue
				}
			case "Signal ACI/PNI":
				if fictionalUUID(m) {
					continue
				}
			}
			out = append(out, finding{path, p.name, m})
		}
	}
	return out
}

func git(t *testing.T, args ...string) []byte {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %s: %v", strings.Join(args, " "), err)
	}
	return out
}

func requireGit(t *testing.T) {
	if raceEnabled {
		t.Skip("the secret scan runs without -race (just secret-scan, CI)")
	}
	cmd := exec.Command("git", "rev-parse", "--is-shallow-repository")
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err != nil {
		t.Skip("not a git checkout")
	}
	if strings.TrimSpace(string(out)) == "true" {
		t.Fatal("shallow clone: the secret scan needs the full history (fetch-depth 0)")
	}
}

func report(t *testing.T, fs []finding) {
	t.Helper()
	for _, f := range fs {
		m := f.match
		if len(m) > 12 {
			m = m[:6] + "…" + m[len(m)-4:] // never print a whole secret
		}
		t.Errorf("%s: possible %s: %q", f.where, f.rule, m)
	}
}

// TestTreeHasNoSecrets scans every tracked file and every new file that
// is not ignored.
func TestTreeHasNoSecrets(t *testing.T) {
	requireGit(t)
	files := strings.Split(strings.TrimRight(string(git(t, "ls-files", "-z", "--cached", "--others", "--exclude-standard")), "\x00"), "\x00")
	var fs []finding
	for _, f := range files {
		if f == "" {
			continue
		}
		data, err := readFile(f)
		if err != nil {
			continue // deleted in the working tree
		}
		fs = append(fs, scan(f, data, false)...)
	}
	report(t, fs)
}

// TestHistoryHasNoSecrets scans every blob reachable from any ref, once,
// under the first path git lists for it, and every commit message.
func TestHistoryHasNoSecrets(t *testing.T) {
	requireGit(t)
	paths := map[string]string{} // blob -> path
	sc := bufio.NewScanner(bytes.NewReader(git(t, "rev-list", "--all", "--objects")))
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	var ids []string
	for sc.Scan() {
		id, path, ok := strings.Cut(sc.Text(), " ")
		if ok && path != "" {
			if _, seen := paths[id]; !seen {
				paths[id] = path
				ids = append(ids, id)
			}
		}
	}
	cmd := exec.Command("git", "cat-file", "--batch")
	cmd.Dir = repoRoot
	cmd.Stdin = strings.NewReader(strings.Join(ids, "\n") + "\n")
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	r := bufio.NewReaderSize(out, 1<<20)
	var fs []finding
	blobs := 0
	for {
		header, err := r.ReadString('\n')
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		f := strings.Fields(header)
		if len(f) != 3 {
			t.Fatalf("cat-file header %q", header)
		}
		size, _ := strconv.Atoi(f[2])
		body := make([]byte, size+1) // content and the trailing newline
		if _, err := io.ReadFull(r, body); err != nil {
			t.Fatal(err)
		}
		if f[1] != "blob" {
			continue
		}
		blobs++
		for _, x := range scan(paths[f[0]], body[:size], true) {
			x.where = fmt.Sprintf("%s (blob %s, history)", x.where, f[0][:10])
			fs = append(fs, x)
		}
	}
	if err := cmd.Wait(); err != nil {
		t.Fatal(err)
	}
	commits := strings.Split(string(git(t, "log", "--all", "--format=%H%x00%B%x01")), "\x01")
	for _, c := range commits {
		id, msg, ok := strings.Cut(strings.TrimLeft(c, "\n"), "\x00")
		if !ok {
			continue
		}
		for _, x := range scan("commit message", []byte(msg), true) {
			x.where = "commit " + id[:10] + " message"
			fs = append(fs, x)
		}
	}
	t.Logf("scanned %d blobs and %d commit messages", blobs, len(commits)-1)
	report(t, fs)
}

// TestSecretPatternsFire: every rule reports a made-up secret, and the
// exemptions hold. The bad samples are assembled at run time, so this file
// does not trip the scan itself.
func TestSecretPatternsFire(t *testing.T) {
	j := strings.Join
	uuid := j([]string{"7f3c2a91", "5b8e", "4d26", "9a1f", "c84e6b2d3f57"}, "-")
	bad := map[string]string{
		`"number":"` + j([]string{"+1415", "555", "2671"}, "") + `"`:                             "phone number",
		j([]string{"signalcaptcha", "://signal-hcaptcha.5fad97ac-7d06.registration.P1_abc"}, ""): "captcha token",
		`"uuid":"` + uuid + `"`:                         "Signal ACI/PNI",
		`"PNI:` + uuid + `"`:                            "Signal ACI/PNI",
		`"pin` + `Hash":"AAAAAAAAAAAAAAAAAAAAAAAAAAAA"`: "key material",
		`identityPrivate` + `Key: "q8Hd2kLx9TzR0vB7mN4pW1sY6uE3oI5aG8fJ2cK7xQ0="`:                     "private key",
		`priv` + `Key := mustHex("9a3f1c7e5b2d4f608a1b3c5d7e9f0a2b4c6d8e0f1a3b5c7d9e1f3a5b7c9d1e3f")`: "private key",
		j([]string{"sk", "ant", "api03-" + strings.Repeat("x", 24)}, "-"):                             "Anthropic API key",
	}
	for s, rule := range bad {
		hit := false
		for _, f := range scan("x.go", []byte(s), false) {
			hit = hit || f.rule == rule
		}
		if !hit {
			t.Errorf("%q: %s not reported", s, rule)
		}
	}
	good := []string{
		`"number":"+12025550100"`, `"number":"+18005551001"`,
		`"uuid":"00000000-0000-4000-8000-000000000001"`, `"aci":"11111111-2222-4333-8444-555555555555"`,
		`keyGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"`,
		`take the token from the "signalcaptcha://" link`,
	}
	for _, s := range good {
		if fs := scan("x.go", []byte(s), false); len(fs) > 0 {
			t.Errorf("%q reported: %+v", s, fs)
		}
	}
	if fs := scan("testdata/vectors/x.json", []byte(`"number":"`+j([]string{"+1415", "555", "2671"}, "")+`"`), false); len(fs) > 0 {
		t.Error("testdata/vectors/ is exempt")
	}
	id := strings.Join([]string{"244db90c", "c537", "4b7d", "8024", "76fd4ca4e3d9"}, "-")
	if len(scan("x.go", []byte(id), false)) == 0 || len(scan("x.go", []byte(id), true)) != 0 {
		t.Error("a history-only value must be reported in the tree and allowed in history")
	}
}

func readFile(rel string) ([]byte, error) { return os.ReadFile(filepath.Join(repoRoot, rel)) }
