// SPDX-License-Identifier: GPL-3.0-or-later

package argparse

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/aren55555/signal-cli-go/internal/testvec"
)

type argDef struct {
	Names       []string        `json:"names"`
	Action      string          `json:"action"`
	Type        string          `json:"type"`
	Nargs       json.RawMessage `json:"nargs"`
	Choices     []any           `json:"choices"`
	EnumValues  []string        `json:"enum_values"`
	EnumNames   []string        `json:"enum_names"`
	Metavar     json.RawMessage `json:"metavar"`
	Default     any             `json:"default"`
	DefaultText *string         `json:"default_text"`
	Dest        string          `json:"dest"`
	Required    bool            `json:"required"`
	Help        string          `json:"help"`
	MutexGroup  *int            `json:"mutex_group"`
}

type parserDef struct {
	Prog        string   `json:"prog"`
	Name        string   `json:"name"`
	Help        string   `json:"help"`
	Width       int      `json:"width"`
	DefaultHelp bool     `json:"default_help"`
	AddHelp     *bool    `json:"add_help"`
	Description string   `json:"description"`
	Epilog      string   `json:"epilog"`
	Arguments   []argDef `json:"arguments"`
	MutexGroups []struct {
		Required bool `json:"required"`
	} `json:"mutex_groups"`
	Subparsers *struct {
		Dest    string      `json:"dest"`
		Help    string      `json:"help"`
		Title   string      `json:"title"`
		Parsers []parserDef `json:"parsers"`
	} `json:"subparsers"`
}

func buildArg(t *testing.T, d argDef) *Arg {
	a := &Arg{Names: d.Names, Dest: d.Dest, Required: d.Required, Help: d.Help, Default: d.Default}
	if d.DefaultText != nil {
		a.DefaultText = *d.DefaultText
	}
	switch d.Action {
	case "store_true":
		a.Action = StoreTrue
	case "count":
		a.Action = Count
	}
	switch {
	case d.Type == "int":
		a.Kind = Int32
		if f, ok := d.Default.(float64); ok {
			a.Default = int(f)
		}
	case d.Type == "long":
		a.Kind = Int64
	case d.Type == "double":
		a.Kind = Double
	case d.Type == "boolean":
		a.Kind = Boolean
	case strings.HasPrefix(d.Type, "enum_string:"):
		a.Kind, a.Choices = Enum, d.EnumValues
	case strings.HasPrefix(d.Type, "enum_name:"):
		a.Kind, a.Choices, a.Display = EnumName, d.EnumNames, d.EnumValues
		if len(a.Display) == 0 { // the test enum displays its names in lower case, dashed
			for _, n := range d.EnumNames {
				a.Display = append(a.Display, strings.ReplaceAll(strings.ToLower(n), "_", "-"))
			}
		}
	}
	if d.Default != nil && a.DefaultText == "" {
		a.DefaultText = fmt.Sprint(d.Default)
	}
	for _, c := range d.Choices {
		switch x := c.(type) {
		case float64:
			a.Choices = append(a.Choices, fmt.Sprint(int64(x)))
		default:
			a.Choices = append(a.Choices, fmt.Sprint(x))
		}
	}
	if len(d.Nargs) > 0 {
		var s string
		var n int
		switch {
		case json.Unmarshal(d.Nargs, &s) == nil:
			a.Nargs = map[string]Nargs{"?": Optional, "*": ZeroOrMore, "+": OneOrMore}[s]
		case json.Unmarshal(d.Nargs, &n) == nil:
			a.Nargs, a.N = Exactly, n
		}
	}
	if len(d.Metavar) > 0 {
		var s string
		if json.Unmarshal(d.Metavar, &s) == nil {
			a.Metavars = []string{s}
		} else {
			json.Unmarshal(d.Metavar, &a.Metavars)
		}
	}
	return a
}

func buildParser(t *testing.T, d parserDef, prog string) *Parser {
	p := &Parser{Prog: prog, Width: d.Width, ShowDefaults: d.DefaultHelp, Description: d.Description, Epilog: d.Epilog}
	if d.AddHelp == nil || *d.AddHelp {
		p.Args = append(p.Args, &Arg{Names: []string{"-h", "--help"}, Action: Help, Help: "show this help message and exit"})
	}
	for range d.MutexGroups {
		p.Groups = append(p.Groups, &Group{})
	}
	for i, g := range d.MutexGroups {
		p.Groups[i].Required = g.Required
	}
	for _, ad := range d.Arguments {
		a := buildArg(t, ad)
		p.Args = append(p.Args, a)
		if ad.MutexGroup != nil {
			g := p.Groups[*ad.MutexGroup]
			g.Args = append(g.Args, a)
		}
	}
	if sp := d.Subparsers; sp != nil {
		p.CommandDest, p.CommandsTitle, p.CommandsHelp = sp.Dest, sp.Title, sp.Help
		for _, sd := range sp.Parsers {
			p.Commands = append(p.Commands, &Command{Name: sd.Name, Help: sd.Help, Parser: buildParser(t, sd, prog)})
		}
	}
	return p
}

// attrs flattens a result into argparse4j's namespace.
func attrs(p *Parser, r *Result) map[string]any {
	out := map[string]any{}
	for k, v := range r.Values {
		out[k] = v
	}
	if r.Sub != nil {
		if p.CommandDest != "" {
			out[p.CommandDest] = r.Command
		}
		for _, c := range p.Commands {
			if c.Name == r.Command {
				for k, v := range attrs(c.Parser, r.Sub) {
					out[k] = v
				}
			}
		}
	}
	return out
}

func normalize(v any) any {
	b, _ := json.Marshal(sanitize(v))
	var out any
	json.Unmarshal(b, &out)
	return out
}

func sanitize(v any) any {
	switch x := v.(type) {
	case float64:
		switch {
		case math.IsNaN(x):
			return "NaN"
		case math.IsInf(x, 1):
			return "Infinity"
		case math.IsInf(x, -1):
			return "-Infinity"
		}
	case map[string]any:
		m := map[string]any{}
		for k, e := range x {
			m[k] = sanitize(e)
		}
		return m
	case []any:
		l := make([]any, len(x))
		for i, e := range x {
			l[i] = sanitize(e)
		}
		return l
	}
	return v
}

// Run emulates signal-cli's handling of a parse: help, error or success.
func Run(p *Parser, argv []string) (exit int, stdout, stderr string, r *Result) {
	r, err := p.Parse(argv)
	if err != nil {
		return 1, "", FormatError(err.(*Error)), nil
	}
	if r.Help {
		return 0, FormatHelp(r.HelpParser), "", r
	}
	return 0, "", "", r
}

func TestSyntheticParsers(t *testing.T) {
	var v struct {
		Parsers []struct {
			ID         string    `json:"id"`
			Definition parserDef `json:"definition"`
			Usage      string    `json:"usage"`
			Help       string    `json:"help"`
			Cases      []struct {
				Argv   []string       `json:"argv"`
				Exit   int            `json:"exit"`
				Stdout string         `json:"stdout"`
				Stderr string         `json:"stderr"`
				Attrs  map[string]any `json:"attrs"`
			} `json:"cases"`
		} `json:"parsers"`
	}
	testvec.Load(t, "argparse_text.json", &v)
	total := 0
	for _, pv := range v.Parsers {
		p := buildParser(t, pv.Definition, pv.Definition.Prog)
		p.Link()
		if got := FormatUsage(p); got != pv.Usage {
			t.Errorf("%s usage %s", pv.ID, firstDiff(got, pv.Usage))
		}
		if got := FormatHelp(p); got != pv.Help {
			t.Errorf("%s help %s", pv.ID, firstDiff(got, pv.Help))
		}
		for _, c := range pv.Cases {
			total++
			exit, out, errOut, r := Run(p, c.Argv)
			if exit != c.Exit || out != c.Stdout || errOut != c.Stderr {
				t.Errorf("%s %q: exit %d want %d\nstdout %s\nstderr %s", pv.ID, c.Argv, exit, c.Exit, firstDiff(out, c.Stdout), firstDiff(errOut, c.Stderr))
				continue
			}
			if c.Attrs != nil && r != nil && !r.Help {
				got := normalize(attrs(p, r))
				if want := normalize(c.Attrs); !reflect.DeepEqual(got, want) {
					t.Errorf("%s %q attrs:\n got %v\nwant %v", pv.ID, c.Argv, got, want)
				}
			}
		}
	}
	if total != 169 {
		t.Errorf("%d cases", total)
	}
}

// firstDiff shows where got and want diverge.
func firstDiff(got, want string) string {
	i := 0
	for i < len(got) && i < len(want) && got[i] == want[i] {
		i++
	}
	lo := max(0, i-60)
	return fmt.Sprintf("at %d:\n got …%q\nwant …%q", i, got[lo:min(len(got), i+60)], want[lo:min(len(want), i+60)])
}
