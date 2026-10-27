package errcode

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite docs/error-codes.md")

const (
	repoRoot  = "../.."
	tablePath = repoRoot + "/docs/error-codes.md"
)

func TestTable(t *testing.T) {
	want := Markdown()
	if *update {
		if err := os.MkdirAll(filepath.Dir(tablePath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(tablePath, []byte(want), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	got, err := os.ReadFile(tablePath)
	if err != nil || string(got) != want {
		t.Fatalf("%s is stale; run go test ./internal/errcode -run TestTable -update", tablePath)
	}
}

func TestCatalogue(t *testing.T) {
	names := map[string]bool{}
	type key struct {
		kind Kind
		exit int
		rpc  int
		http int
		msg  string
	}
	seen := map[key]Code{}
	for _, c := range All() {
		i := c.info()
		if names[i.name] {
			t.Errorf("duplicate name %s", i.name)
		}
		names[i.name] = true
		if i.cause == "" {
			t.Errorf("%s has no cause", c)
		}
		if i.exit == 0 && i.rpc == 0 && i.http == 0 {
			t.Errorf("%s is reachable nowhere", c)
		}
		k := key{i.kind, i.exit, i.rpc, i.http, i.msg}
		if prev, ok := seen[k]; ok {
			t.Errorf("%s and %s are the same error", prev, c)
		}
		seen[k] = c
	}
	if len(All()) != int(numCodes)-1 {
		t.Errorf("All() has %d codes, want %d: a constant lacks a table entry", len(All()), int(numCodes)-1)
	}
}

func TestWrapping(t *testing.T) {
	cause := errors.New("connection reset")
	err := fmt.Errorf("context: %w", UpdateAccountFailed.WrapText(cause))
	if !errors.Is(err, UpdateAccountFailed) || errors.Is(err, UnregisterFailed) {
		t.Error("errors.Is does not match the code")
	}
	if !errors.Is(err, cause) {
		t.Error("errors.Is does not reach the cause")
	}
	var e *Error
	if !errors.As(err, &e) || e.Code != UpdateAccountFailed {
		t.Error("errors.As does not find *Error")
	}
	if got := Message(err); got != "UpdateAccount error: connection reset" {
		t.Errorf("Message = %q", got)
	}
	if got, want := err.Error(), "context: UpdateAccountFailed: the account attribute update failed: connection reset"; got != want {
		t.Errorf("Error() = %q, want %q (not the reference text)", got, want)
	}
	if ExitCode(err) != 3 || RPCCode(err) != -3 {
		t.Errorf("exit %d rpc %d", ExitCode(err), RPCCode(err))
	}
	if got := RPCMessage(SendIOFailed.New("boom")); got != "Failed to send message: boom (IOException) (UnexpectedErrorException)" {
		t.Errorf("unexpected-kind RPC message = %q", got)
	}
	if got := Message(IllegalArgumentCrash.New("odd-length pack id")); got != "Exception in thread \"main\" java.lang.IllegalArgumentException: odd-length pack id" {
		t.Errorf("crash CLI message = %q", got)
	}
	if got := RPCMessage(IllegalArgumentCrash.New("odd-length pack id")); got != "odd-length pack id (IllegalArgumentException)" {
		t.Errorf("crash RPC message = %q", got)
	}
	if got := Message(errors.New("plain")); got != "plain" || ExitCode(errors.New("x")) != 2 {
		t.Errorf("uncoded error: %q", got)
	}
}

// goldenError is one error a recorded transcript shows.
type goldenError struct {
	file    string
	surface string // rpc, http, cli, argparse
	num     int    // rpc code, HTTP status or exit code
	msg     string
}

func loadGoldenErrors(t *testing.T) []goldenError {
	var out []goldenError
	rpcFiles, _ := filepath.Glob(repoRoot + "/testdata/golden/rpc/recorded/*.json")
	cliFiles, _ := filepath.Glob(repoRoot + "/testdata/golden/cli/recorded/*.json")
	if len(rpcFiles) < 100 || len(cliFiles) < 100 {
		t.Fatalf("goldens not found (%d rpc, %d cli)", len(rpcFiles), len(cliFiles))
	}
	for _, f := range rpcFiles {
		var g struct {
			Result struct {
				Status   string
				Body     string
				Messages []string
			}
		}
		readJSON(t, f, &g)
		name := filepath.Base(f)
		if fs := strings.Fields(g.Result.Status); len(fs) >= 2 && fs[1] >= "400" {
			var n int
			fmt.Sscan(fs[1], &n)
			out = append(out, goldenError{name, "http", n, g.Result.Body})
			continue
		}
		for _, s := range append([]string{g.Result.Body}, g.Result.Messages...) {
			var v any
			if json.Unmarshal([]byte(s), &v) != nil {
				continue
			}
			items, ok := v.([]any)
			if !ok {
				items = []any{v}
			}
			for _, it := range items {
				m, _ := it.(map[string]any)
				e, ok := m["error"].(map[string]any)
				if !ok {
					continue
				}
				out = append(out, goldenError{name, "rpc", int(e["code"].(float64)), e["message"].(string)})
			}
		}
	}
	argpErr := regexp.MustCompile(`signal-cli:\s+error:`)
	logLine := regexp.MustCompile(`^((INFO|WARN|DEBUG|ERROR|TRACE) |<TIMESTAMP> \[)`)
	for _, f := range cliFiles {
		var g struct {
			Result struct {
				Stderr   string
				ExitCode int
			}
		}
		readJSON(t, f, &g)
		if g.Result.ExitCode == 0 {
			continue
		}
		name := filepath.Base(f)
		s := g.Result.Stderr
		if loc := argpErr.FindStringIndex(s); loc != nil {
			s = s[loc[1]:]
			s, _, _ = strings.Cut(s, "Did you mean")
			out = append(out, goldenError{name, "argparse", g.Result.ExitCode, strings.Join(strings.Fields(s), "")})
			continue
		}
		var lines []string
		for _, l := range strings.Split(strings.TrimRight(s, "\n"), "\n") {
			if !logLine.MatchString(l) {
				lines = append(lines, l)
			}
		}
		out = append(out, goldenError{name, "cli", g.Result.ExitCode, strings.Join(lines, "\n")})
	}
	return out
}

func readJSON(t *testing.T, f string, v any) {
	t.Helper()
	data, err := os.ReadFile(f)
	if err == nil {
		err = json.Unmarshal(data, v)
	}
	if err != nil {
		t.Fatalf("%s: %v", f, err)
	}
}

// pattern turns a message template into an anchored regexp; strip
// removes all whitespace first (argparse wraps and justifies its text).
func pattern(tmpl string, strip bool) *regexp.Regexp {
	if strip {
		tmpl = strings.Join(strings.Fields(tmpl), "")
	}
	q := regexp.QuoteMeta(tmpl)
	q = strings.ReplaceAll(q, "%s", "(.*)")
	q = strings.ReplaceAll(q, "%d", "(-?[0-9]+)")
	return regexp.MustCompile("(?s)^" + q + "$")
}

func matches(ge goldenError) []Code {
	var out []Code
	for _, c := range All() {
		if c == Internal {
			continue
		}
		i := c.info()
		ok := false
		switch ge.surface {
		case "rpc":
			ok = i.rpc == ge.num && i.kind != KindHTTP && pattern(c.RPCTemplate(), false).MatchString(ge.msg)
		case "http":
			ok = i.kind == KindHTTP && i.http == ge.num && i.msg == ge.msg
		case "argparse":
			ok = i.kind == KindArgparse && i.exit == ge.num && pattern(i.msg, true).MatchString(ge.msg)
		case "cli":
			ok = i.kind != KindArgparse && i.exit == ge.num && pattern(i.msg, false).MatchString(ge.msg)
		}
		if ok {
			out = append(out, c)
		}
	}
	return out
}

// TestGoldenErrors checks that every error in the recorded goldens maps
// to exactly one code, and that a code is marked Observed exactly when
// some golden exercises it.
func TestGoldenErrors(t *testing.T) {
	exercised := map[Code][]string{}
	for _, ge := range loadGoldenErrors(t) {
		ms := matches(ge)
		switch len(ms) {
		case 1:
			exercised[ms[0]] = append(exercised[ms[0]], ge.file)
		case 0:
			t.Errorf("%s: no code for %s %d %q", ge.file, ge.surface, ge.num, ge.msg)
		default:
			t.Errorf("%s: %s %d %q matches several codes: %v", ge.file, ge.surface, ge.num, ge.msg, ms)
		}
	}
	var stale []string
	for _, c := range All() {
		files := exercised[c]
		switch {
		case c.Source() == Observed && len(files) == 0:
			stale = append(stale, fmt.Sprintf("%s is marked golden but no golden shows it", c))
		case c.Source() != Observed && len(files) > 0:
			stale = append(stale, fmt.Sprintf("%s is marked %s but %s shows it", c, c.Source(), files[0]))
		}
	}
	sort.Strings(stale)
	for _, s := range stale {
		t.Error(s)
	}
}
