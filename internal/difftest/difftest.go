// SPDX-License-Identifier: GPL-3.0-or-later

// Package difftest runs the same scenario against the reference signal-cli
// and signal-cli-go and compares what a user would see: stdout, stderr and
// the exit code.
//
// Reference runs are recorded once as golden transcripts under
// testdata/golden/ and replayed offline. Only fields that genuinely vary
// between runs are normalized; every rule is listed in normalizers below
// with the reason it exists.
package difftest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Placeholders in a scenario's args, env values, file paths and file
// contents. Each run gets a fresh, empty config directory and a separate
// scratch directory (for config files, log files and the like).
const (
	ConfigPlaceholder  = "{{CONFIG}}"
	ScratchPlaceholder = "{{SCRATCH}}"
)

// Scenario is one CLI invocation.
type Scenario struct {
	Name  string   `json:"name"`
	Args  []string `json:"args"`
	Stdin string   `json:"stdin,omitempty"`
	// Env adds to or overrides the base environment (see scenarioEnv).
	Env map[string]string `json:"env,omitempty"`
	// Files are written before the run: path -> content. Paths must start
	// with a placeholder.
	Files map[string]string `json:"files,omitempty"`
	// Note says what the scenario covers, for humans reading goldens.
	Note string `json:"note,omitempty"`
}

// Result is the user-visible outcome of a run, after normalization.
type Result struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exitCode"`
}

// Golden is a recorded reference run.
type Golden struct {
	Scenario Scenario `json:"scenario"`
	Oracle   string   `json:"oracle"`
	Result   Result   `json:"result"`
}

// Run executes bin with the scenario in fresh config and scratch
// directories and returns the normalized result.
func Run(ctx context.Context, bin string, sc Scenario) (Result, error) {
	cfg, err := os.MkdirTemp(scratchDir(), "difftest-config-")
	if err != nil {
		return Result{}, err
	}
	defer os.RemoveAll(cfg)
	scratch, err := os.MkdirTemp(scratchDir(), "difftest-scratch-")
	if err != nil {
		return Result{}, err
	}
	defer os.RemoveAll(scratch)

	expand := strings.NewReplacer(ConfigPlaceholder, cfg, ScratchPlaceholder, scratch).Replace
	for p, content := range sc.Files {
		if !strings.HasPrefix(p, ConfigPlaceholder+"/") && !strings.HasPrefix(p, ScratchPlaceholder+"/") {
			return Result{}, fmt.Errorf("%s: file path %q must start with a placeholder", sc.Name, p)
		}
		path := expand(p)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return Result{}, err
		}
		if err := os.WriteFile(path, []byte(expand(content)), 0o644); err != nil {
			return Result{}, err
		}
	}
	args := make([]string, len(sc.Args))
	for i, a := range sc.Args {
		args[i] = expand(a)
	}
	env := scenarioEnv(scratch)
	for k, v := range sc.Env {
		env = append(env, k+"="+expand(v)) // later entries win
	}

	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Stdin = strings.NewReader(sc.Stdin)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Env = env

	err = cmd.Run()
	code := 0
	var exitErr *exec.ExitError
	switch {
	case ctx.Err() != nil:
		return Result{}, fmt.Errorf("%s: timed out", sc.Name)
	case errors.As(err, &exitErr):
		code = exitErr.ExitCode()
	case err != nil:
		return Result{}, fmt.Errorf("%s: %w", sc.Name, err)
	}
	return Result{
		Stdout:   Normalize(stdout.String(), cfg, scratch),
		Stderr:   Normalize(stderr.String(), cfg, scratch),
		ExitCode: code,
	}, nil
}

// scratchDir is where per-run directories go. /tmp is noexec in the
// sandbox, which is fine for data, but GOTMPDIR keeps everything in one
// place when it is set.
func scratchDir() string {
	if d := os.Getenv("GOTMPDIR"); d != "" {
		return d
	}
	return os.TempDir()
}

// scenarioEnv is a minimal, stable environment so the operator's locale,
// config files and secrets never leak into a run. COLUMNS is left unset:
// with no terminal, signal-cli wraps help at 75 columns, which is what
// scripts and integrations see. Scenarios that test other widths set it.
// HOME points into the scratch dir so no per-user config file is read.
func scenarioEnv(scratch string) []string {
	env := []string{"LANG=C.UTF-8", "LC_ALL=C.UTF-8", "TZ=UTC", "TERM=dumb",
		"HOME=" + filepath.Join(scratch, "home")}
	for _, k := range []string{"PATH", "JAVA_HOME",
		"HTTP_PROXY", "HTTPS_PROXY", "http_proxy", "https_proxy", "NO_PROXY", "no_proxy"} {
		if v, ok := os.LookupEnv(k); ok {
			env = append(env, k+"="+v)
		}
	}
	return append(env, "JAVA_OPTS="+javaOpts())
}

// JVMTmpDir is where the reference signal-cli may extract its libsignal JNI
// library. /tmp is noexec in the sandbox, and a library mapped from there
// fails to load ("Missing required native library dependency").
func JVMTmpDir() string {
	if h, err := os.UserHomeDir(); err == nil {
		return filepath.Join(h, ".cache", "jvmtmp")
	}
	return filepath.Join(scratchDir(), "jvmtmp")
}

func javaOpts() string {
	opts := os.Getenv("JAVA_OPTS")
	if !strings.Contains(opts, "-Djava.io.tmpdir=") {
		dir := JVMTmpDir()
		_ = os.MkdirAll(dir, 0o700)
		opts = strings.TrimSpace(opts + " -Djava.io.tmpdir=" + dir)
	}
	return opts
}

type normalizer struct {
	why string
	re  *regexp.Regexp
	to  string
}

// normalizers apply in order to stdout and stderr. Keep this list short;
// each entry hides a potential difference, so each must be justified.
var normalizers = []normalizer{
	{
		why: "ISO-8601 timestamps differ on every run",
		re:  regexp.MustCompile(`\b\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:?\d{2})\b`),
		to:  "<TIMESTAMP>",
	},
}

// Normalize replaces the per-run config and scratch dirs with <CONFIG> and
// <SCRATCH>, then applies the documented normalizers.
func Normalize(s, configDir, scratchDir string) string {
	if configDir != "" {
		s = strings.ReplaceAll(s, configDir, "<CONFIG>")
	}
	if scratchDir != "" {
		s = strings.ReplaceAll(s, scratchDir, "<SCRATCH>")
	}
	for _, n := range normalizers {
		s = n.re.ReplaceAllString(s, n.to)
	}
	return s
}

// LoadScenarios reads a JSON array of scenarios.
func LoadScenarios(path string) ([]Scenario, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var scs []Scenario
	if err := json.Unmarshal(data, &scs); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	seen := map[string]bool{}
	for _, sc := range scs {
		if sc.Name == "" || strings.ContainsAny(sc.Name, `/\ `) {
			return nil, fmt.Errorf("%s: bad scenario name %q", path, sc.Name)
		}
		if seen[sc.Name] {
			return nil, fmt.Errorf("%s: duplicate scenario %q", path, sc.Name)
		}
		seen[sc.Name] = true
	}
	return scs, nil
}

// LoadPending reads a list of scenario names, one per line, that are
// known not to match yet. Blank lines and # comments are ignored.
func LoadPending(path string) (map[string]bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]bool{}, nil
	}
	if err != nil {
		return nil, err
	}
	pending := map[string]bool{}
	for _, line := range strings.Split(string(data), "\n") {
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}
		if line = strings.TrimSpace(line); line != "" {
			pending[line] = true
		}
	}
	return pending, nil
}

// GoldenPath is where a scenario's golden transcript lives.
func GoldenPath(dir string, sc Scenario) string {
	return filepath.Join(dir, sc.Name+".json")
}

// WriteGolden writes g crash-safely.
func WriteGolden(path string, g Golden) error {
	data, err := json.MarshalIndent(g, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(path, append(data, '\n'))
}

// writeFileAtomic writes via a temp file, fsync and rename.
func writeFileAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".golden-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// ReadGolden reads a golden transcript.
func ReadGolden(path string) (Golden, error) {
	var g Golden
	data, err := os.ReadFile(path)
	if err != nil {
		return g, err
	}
	err = json.Unmarshal(data, &g)
	return g, err
}

// Diff describes how got differs from want, or returns "" if they match.
func Diff(want, got Result) string {
	var b strings.Builder
	if want.ExitCode != got.ExitCode {
		fmt.Fprintf(&b, "exit code: want %d, got %d\n", want.ExitCode, got.ExitCode)
	}
	diffStream(&b, "stdout", want.Stdout, got.Stdout)
	diffStream(&b, "stderr", want.Stderr, got.Stderr)
	return b.String()
}

func diffStream(b *strings.Builder, name, want, got string) {
	if want == got {
		return
	}
	wl, gl := strings.Split(want, "\n"), strings.Split(got, "\n")
	for i := 0; i < len(wl) || i < len(gl); i++ {
		var w, g string
		if i < len(wl) {
			w = wl[i]
		}
		if i < len(gl) {
			g = gl[i]
		}
		if w != g || i >= len(wl) || i >= len(gl) {
			fmt.Fprintf(b, "%s: first difference at line %d\n  want: %q\n  got:  %q\n", name, i+1, w, g)
			return
		}
	}
}
