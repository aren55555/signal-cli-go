// Command obs2scenarios turns the account-free invocations logged in
// spec/surface/cli/observations.md into difftest scenarios.
//
// Each observation heading has the form
//
//	### `signal-cli --config <CONFIG> ARGS...`
//
// ARGS never contain quotes, so they are split on whitespace. Entries with a
// "Setup:" line need files or environment variables and are skipped here;
// they are written by hand in testdata/golden/cli/scenarios.json. So are
// any arg lists that the hand-written file already covers.
//
// Usage (from the repo root):
//
//	go run ./tools/obs2scenarios > testdata/golden/cli/scenarios-observed.json
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"regexp"
	"strings"
)

type scenario struct {
	Name  string            `json:"name"`
	Args  []string          `json:"args"`
	Env   map[string]string `json:"env,omitempty"`
	Files map[string]string `json:"files,omitempty"`
	Note  string            `json:"note,omitempty"`
}

var heading = regexp.MustCompile("^### `signal-cli --config <CONFIG>(.*)`$")

func main() {
	obsPath := flag.String("observations", "spec/surface/cli/observations.md", "observations file")
	exclude := flag.String("exclude", "testdata/golden/cli/scenarios.json", "hand-written scenarios whose arg lists are skipped")
	flag.Parse()

	if err := run(*obsPath, *exclude); err != nil {
		fmt.Fprintln(os.Stderr, "obs2scenarios:", err)
		os.Exit(1)
	}
}

func run(obsPath, excludePath string) error {
	seen := map[string]bool{}
	if data, err := os.ReadFile(excludePath); err == nil {
		var manual []scenario
		if err := json.Unmarshal(data, &manual); err != nil {
			return fmt.Errorf("%s: %w", excludePath, err)
		}
		for _, sc := range manual {
			if len(sc.Env) > 0 || len(sc.Files) > 0 {
				continue // same args, different setup: not a duplicate
			}
			seen[strings.Join(sc.Args, "\x00")] = true
		}
	}

	f, err := os.Open(obsPath)
	if err != nil {
		return err
	}
	defer f.Close()

	type entry struct {
		ordinal  int
		args     []string
		hasSetup bool
	}
	var entries []entry
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		if m := heading.FindStringSubmatch(line); m != nil {
			args := []string{"--config", "{{CONFIG}}"}
			for _, a := range strings.Fields(m[1]) {
				args = append(args, strings.ReplaceAll(a, "<SCRATCH>", "{{SCRATCH}}"))
			}
			entries = append(entries, entry{ordinal: len(entries) + 1, args: args})
			continue
		}
		if strings.HasPrefix(line, "Setup:") && len(entries) > 0 {
			entries[len(entries)-1].hasSetup = true
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}

	var out []scenario
	skippedSetup, skippedDup := 0, 0
	for _, e := range entries {
		key := strings.Join(e.args, "\x00")
		switch {
		case e.hasSetup:
			skippedSetup++
		case seen[key]:
			skippedDup++
		default:
			seen[key] = true
			out = append(out, scenario{
				Name: fmt.Sprintf("obs%03d-%s", e.ordinal, slug(e.args[2:])),
				Args: e.args,
				Note: "from spec/surface/cli/observations.md",
			})
		}
	}
	fmt.Fprintf(os.Stderr, "obs2scenarios: %d headings, %d scenarios, %d skipped (setup), %d skipped (duplicate args)\n",
		len(entries), len(out), skippedSetup, skippedDup)

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(out)
}

var unsafe = regexp.MustCompile(`[^A-Za-z0-9.=-]+`) // no "+": a cut-off phone number would trip the secret scan

// slug makes a short, filename-safe label from the args.
func slug(args []string) string {
	s := strings.Trim(unsafe.ReplaceAllString(strings.Join(args, "_"), "_"), "_")
	if s == "" {
		s = "no-args"
	}
	if len(s) > 60 {
		s = s[:60]
	}
	return s
}
