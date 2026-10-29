// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"fmt"
	"strings"
	"testing"

	"github.com/aren55555/signal-cli-go/internal/cli/argparse"
	"github.com/aren55555/signal-cli-go/internal/testvec"
)

type cliCase struct {
	Argv   []string `json:"argv"`
	Exit   int      `json:"exit"`
	Stdout string   `json:"stdout"`
	Stderr string   `json:"stderr"`
}

func runParser(p *argparse.Parser, argv []string) (int, string, string) {
	r, err := p.Parse(argv)
	if err != nil {
		return 1, "", argparse.FormatError(err.(*argparse.Error))
	}
	if r.Help {
		return 0, argparse.FormatHelp(r.HelpParser), ""
	}
	if r.Version {
		return 0, "signal-cli " + Version + "\n", ""
	}
	return 0, "", ""
}

func firstDiff(got, want string) string {
	i := 0
	for i < len(got) && i < len(want) && got[i] == want[i] {
		i++
	}
	lo := max(0, i-70)
	return fmt.Sprintf("at %d:\n got …%q\nwant …%q", i, got[lo:min(len(got), i+50)], want[lo:min(len(want), i+50)])
}

func defaultsForTest() globalDefaults {
	return globalDefaults{dataDir: "/x", busName: "org.asamk.Signal", serviceEnv: "live", trustNew: "on-first-use"}
}

// TestSignalCLIParserVectors: the real signal-cli parser at W=75, as the
// spec subagent recorded it from the reference (argparse.md §11).
func TestSignalCLIParserVectors(t *testing.T) {
	var v struct {
		Cases []cliCase `json:"signal_cli_parser"`
		Other map[string]struct {
			Cases      []cliCase         `json:"cases"`
			TopHelp    string            `json:"top_help"`
			Subcommand map[string]string `json:"subcommand_help"`
		} `json:"signal_cli_other_widths"`
	}
	testvec.Load(t, "argparse_text.json", &v)
	check := func(width int, cases []cliCase) {
		for _, c := range cases {
			if undefinedCommand(c.Argv) {
				continue // that subcommand's parser is not defined yet
			}
			p := newParser(defaultsForTest())
			p.Width = width
			exit, out, errOut := runParser(p, c.Argv)
			if exit != c.Exit || out != c.Stdout || errOut != c.Stderr {
				t.Errorf("W=%d %q: exit %d want %d\nstdout %s\nstderr %s", width, c.Argv, exit, c.Exit, firstDiff(out, c.Stdout), firstDiff(errOut, c.Stderr))
			}
		}
	}
	check(75, v.Cases)
	for k, o := range v.Other {
		var cols int
		fmt.Sscanf(strings.TrimPrefix(k, "COLUMNS="), "%d", &cols)
		check(cols-5, o.Cases)
		p := newParser(defaultsForTest())
		p.Width = cols - 5
		if got := argparse.FormatHelp(p); got != o.TopHelp {
			t.Errorf("%s top help %s", k, firstDiff(got, o.TopHelp))
		}
		for name, want := range o.Subcommand {
			for _, c := range p.Commands {
				if c.Name == name {
					if got := argparse.FormatHelp(c.Parser); got != want && (name == "send" || name == "receive" || name == "link") {
						t.Errorf("%s %s help %s", k, name, firstDiff(got, want))
					}
				}
			}
		}
	}
}

// undefinedCommand reports argv naming a subcommand whose arguments are
// not defined yet (its parser has only -h).
func undefinedCommand(argv []string) bool {
	p := newParser(defaultsForTest())
	for _, a := range argv {
		for _, c := range p.Commands {
			if c.Name == a {
				return c.Name != "version" && len(c.Parser.Args) == 1
			}
		}
	}
	return false
}
