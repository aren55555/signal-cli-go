package argparse

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// Result holds parsed values; Sub is the chosen subcommand's result.
type Result struct {
	Values     map[string]any
	Command    string
	Sub        *Result
	Help       bool    // -h was reached: print HelpParser's help, exit 0
	HelpParser *Parser // the parser whose -h it was
	Version    bool    // --version was reached
}

// String returns a string value, or "".
func (r *Result) String(k string) string {
	s, _ := r.Values[k].(string)
	return s
}

// Int returns an integer value (Int32 or Count), or 0.
func (r *Result) Int(k string) int {
	switch v := r.Values[k].(type) {
	case int:
		return v
	case int64:
		return int(v)
	}
	return 0
}

// Int64 returns a 64-bit integer value and whether it was set.
func (r *Result) Int64(k string) (int64, bool) {
	v, ok := r.Values[k].(int64)
	return v, ok
}

// Float returns a double value, or 0.
func (r *Result) Float(k string) float64 {
	f, _ := r.Values[k].(float64)
	return f
}

// Bool returns a flag's value.
func (r *Result) Bool(k string) bool {
	b, _ := r.Values[k].(bool)
	return b
}

// Strings returns a multi-valued argument's values.
func (r *Result) Strings(k string) []string {
	s, _ := r.Values[k].([]string)
	return s
}

// Has reports whether k was given or has a default.
func (r *Result) Has(k string) bool { _, ok := r.Values[k]; return ok }

// Error is a parse error: Message belongs to Parser (whose usage is
// printed).
type Error struct {
	Parser          *Parser
	Message         string
	SuggestCommands bool     // an invalid subcommand: suggest commands
	Unrecognized    []string // for option suggestions
	BadCommand      string
}

func (e *Error) Error() string { return e.Message }

// DestOf is an argument's result key: Dest, else its first long name, else
// its first name, without dashes.
func DestOf(a *Arg) string {
	if a.Dest != "" {
		return a.Dest
	}
	for _, n := range a.Names {
		if strings.HasPrefix(n, "--") {
			return n[2:]
		}
	}
	return strings.TrimLeft(a.Names[0], "-")
}

func (a *Arg) isPositional() bool { return !strings.HasPrefix(a.Names[0], "-") }

func (a *Arg) takesValue() bool { return a.Action == Store }

// displayName is "-a/--account/…" for error messages.
func (a *Arg) displayName() string {
	if a.isPositional() {
		return a.Names[0]
	}
	return strings.Join(a.Names, "/")
}

var negativeNumber = regexp.MustCompile(`^-\d+$|^-\d*\.\d+$`)

// hasNegativeNumberOptions mirrors argparse's rule: if no option looks
// like a negative number, "-1" is a value, not an option.
func (p *Parser) hasNegativeNumberOptions() bool {
	for _, a := range p.Args {
		for _, n := range a.Names {
			if negativeNumber.MatchString(n) {
				return true
			}
		}
	}
	return false
}

func (p *Parser) looksLikeOption(s string) bool {
	if len(s) < 2 || s[0] != '-' {
		return false
	}
	if negativeNumber.MatchString(s) && !p.hasNegativeNumberOptions() {
		return false
	}
	return true
}

// Parse parses args (without the program name).
func (p *Parser) Parse(args []string) (*Result, error) {
	return p.parse(args)
}

func (p *Parser) defaults() map[string]any {
	v := map[string]any{}
	for _, a := range p.Args {
		switch {
		case a.Default != nil:
			v[DestOf(a)] = a.Default
		case a.Action == StoreTrue:
			v[DestOf(a)] = false
		case a.Action == Count:
			v[DestOf(a)] = 0
		}
	}
	return v
}

func (p *Parser) errorf(format string, args ...any) *Error {
	return &Error{Parser: p, Message: fmt.Sprintf(format, args...)}
}

// convert applies an argument's type to a value.
func (p *Parser) convert(a *Arg, s string) (any, error) {
	switch a.Kind {
	case Int32:
		n, err := strconv.ParseInt(s, 10, 32)
		if err != nil {
			return nil, p.errorf("argument %s: could not convert '%s' to integer (32 bits)", a.displayName(), s)
		}
		return int(n), nil
	case Int64:
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return nil, p.errorf("argument %s: could not convert '%s' to integer (64 bits)", a.displayName(), s)
		}
		return n, nil
	case Double:
		f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
		if err != nil || math.IsInf(f, 0) && !strings.Contains(strings.ToLower(s), "inf") {
			return nil, p.errorf("argument %s: could not convert '%s' to number (double precision)", a.displayName(), s)
		}
		return f, nil
	case Enum, Choice:
		for _, c := range a.Choices {
			if c == s {
				return s, nil
			}
		}
		if a.Kind == Enum {
			return nil, p.errorf("argument %s: could not convert '%s' (choose from {%s})", a.displayName(), s, strings.Join(a.Choices, ","))
		}
		return nil, p.errorf("argument %s: invalid choice: '%s' (choose from {%s})", a.displayName(), s, strings.Join(a.Choices, ","))
	}
	return s, nil
}

// lookupLong resolves "--name" exactly or by unique prefix.
func (p *Parser) lookupLong(name string) (*Arg, string, error) {
	type match struct {
		a *Arg
		n string
	}
	var prefix []match
	for _, a := range p.Args {
		for _, n := range a.Names {
			if !strings.HasPrefix(n, "--") {
				continue
			}
			if n == name {
				return a, n, nil
			}
			if strings.HasPrefix(n, name) {
				prefix = append(prefix, match{a, n})
			}
		}
	}
	distinct := map[*Arg]bool{}
	for _, m := range prefix {
		distinct[m.a] = true
	}
	switch {
	case len(distinct) == 1:
		return prefix[0].a, prefix[0].n, nil
	case len(distinct) > 1:
		var names []string
		for _, m := range prefix {
			names = append(names, m.n)
		}
		return nil, "", p.errorf("ambiguous option: %s could match %s", name, strings.Join(names, ", "))
	}
	return nil, "", nil
}

func (p *Parser) lookupShort(name string) *Arg {
	for _, a := range p.Args {
		for _, n := range a.Names {
			if n == name {
				return a
			}
		}
	}
	return nil
}

func (p *Parser) groupOf(a *Arg) *Group {
	for _, g := range p.Groups {
		for _, x := range g.Args {
			if x == a {
				return g
			}
		}
	}
	return nil
}

func (p *Parser) parse(args []string) (*Result, error) {
	r := &Result{Values: p.defaults()}
	seenInGroup := map[*Group]*Arg{}
	given := map[*Arg]bool{}
	var positionals, unrecognized []string
	optionsDone := false

	// apply records an option's value(s), with the mutual-exclusion check.
	apply := func(a *Arg, vals []string) error {
		if g := p.groupOf(a); g != nil {
			if prev, ok := seenInGroup[g]; ok && prev != a {
				return p.errorf("argument %s: not allowed with argument %s", a.displayName(), prev.displayName())
			}
			seenInGroup[g] = a
		}
		given[a] = true
		dest := DestOf(a)
		switch a.Action {
		case StoreTrue:
			r.Values[dest] = true
		case Count:
			r.Values[dest] = r.Int(dest) + 1
		case Store:
			if a.Nargs == ZeroOrMore || a.Nargs == OneOrMore {
				out := []string{}
				for _, v := range vals {
					cv, err := p.convert(a, v)
					if err != nil {
						return err
					}
					out = append(out, fmt.Sprint(cv))
				}
				r.Values[dest] = out
				return nil
			}
			if len(vals) == 0 {
				r.Values[dest] = nil
				return nil
			}
			cv, err := p.convert(a, vals[0])
			if err != nil {
				return err
			}
			r.Values[dest] = cv
		}
		return nil
	}

	for i := 0; i < len(args); i++ {
		s := args[i]
		if !optionsDone && s == "--" {
			optionsDone = true
			continue
		}
		if optionsDone || !p.looksLikeOption(s) {
			if len(p.Commands) > 0 && r.Command == "" {
				if optionsDone {
					unrecognized = append(unrecognized, s)
					continue
				}
				var cmd *Command
				for _, c := range p.Commands {
					if c.Name == s {
						cmd = c
					}
				}
				if cmd == nil {
					var names []string
					for _, c := range p.Commands {
						names = append(names, "'"+c.Name+"'")
					}
					e := p.errorf("invalid choice: '%s' (choose from %s)", s, strings.Join(names, ", "))
					e.SuggestCommands, e.BadCommand = true, s
					return nil, e
				}
				r.Command = cmd.Name
				sub, err := cmd.Parser.parse(args[i+1:])
				if err != nil {
					return nil, err
				}
				r.Sub = sub
				if sub.Help || sub.Version {
					r.Help, r.HelpParser, r.Version = sub.Help, sub.HelpParser, sub.Version
				}
				if len(unrecognized) > 0 {
					return nil, p.unrecognizedError(unrecognized)
				}
				return r, nil
			}
			positionals = append(positionals, s)
			continue
		}

		// An option: find it and the value attached to it, if any.
		var a *Arg
		var attached string
		hasAttached := false
		if strings.HasPrefix(s, "--") {
			name, val, eq := strings.Cut(s, "=")
			var err error
			if a, _, err = p.lookupLong(name); err != nil {
				return nil, err
			}
			attached, hasAttached = val, eq
		} else {
			a = p.lookupShort(s[:2])
			if a != nil && len(s) > 2 {
				if a.takesValue() {
					attached, hasAttached = s[2:], true
					if attached != "" && attached[0] == '=' {
						attached = attached[1:]
					}
				} else {
					// A cluster of flags: -vvv.
					for _, c := range s[1:] {
						f := p.lookupShort("-" + string(c))
						if f == nil || f.takesValue() {
							a = nil
							break
						}
					}
					if a != nil {
						for _, c := range s[1:] {
							f := p.lookupShort("-" + string(c))
							if f.Action == Help {
								r.Help, r.HelpParser = true, p
								return r, nil
							}
							if err := apply(f, nil); err != nil {
								return nil, err
							}
						}
						continue
					}
				}
			}
		}
		if a == nil {
			unrecognized = append(unrecognized, s)
			continue
		}
		switch a.Action {
		case Help:
			r.Help, r.HelpParser = true, p
			return r, nil
		case Version:
			r.Version = true
			return r, nil
		case StoreTrue, Count:
			if err := apply(a, nil); err != nil {
				return nil, err
			}
			continue
		}
		var vals []string
		switch {
		case hasAttached:
			vals = []string{attached}
		case a.Nargs == ZeroOrMore || a.Nargs == OneOrMore || a.Nargs == Optional:
			for i+1 < len(args) && !p.looksLikeOption(args[i+1]) && args[i+1] != "--" {
				i++
				vals = append(vals, args[i])
				if a.Nargs == Optional {
					break
				}
			}
			if a.Nargs == OneOrMore && len(vals) == 0 {
				return nil, p.errorf("argument %s: expected at least one argument", a.displayName())
			}
		default:
			if i+1 >= len(args) || p.looksLikeOption(args[i+1]) {
				return nil, p.errorf("argument %s: expected one argument", a.displayName())
			}
			i++
			vals = []string{args[i]}
		}
		if err := apply(a, vals); err != nil {
			return nil, err
		}
	}

	// Positionals, in declaration order.
	for _, a := range p.Args {
		if !a.isPositional() {
			continue
		}
		switch a.Nargs {
		case ZeroOrMore, OneOrMore:
			if err := apply(a, positionals); err != nil {
				return nil, err
			}
			if a.Nargs == OneOrMore && len(positionals) == 0 {
				return nil, p.errorf("too few arguments")
			}
			positionals = nil
		case Optional:
			if len(positionals) > 0 {
				if err := apply(a, positionals[:1]); err != nil {
					return nil, err
				}
				positionals = positionals[1:]
			}
		default:
			if len(positionals) == 0 {
				return nil, p.errorf("too few arguments")
			}
			if err := apply(a, positionals[:1]); err != nil {
				return nil, err
			}
			positionals = positionals[1:]
		}
	}
	unrecognized = append(unrecognized, positionals...)
	if len(unrecognized) > 0 {
		return nil, p.unrecognizedError(unrecognized)
	}
	if len(p.Commands) > 0 && r.Command == "" {
		return nil, p.errorf("too few arguments")
	}
	for _, a := range p.Args {
		if a.Required && !given[a] {
			return nil, p.errorf("argument %s is required", a.displayName())
		}
	}
	for _, g := range p.Groups {
		if g.Required && seenInGroup[g] == nil {
			var names []string
			for _, a := range g.Args {
				names = append(names, a.displayName())
			}
			return nil, p.errorf("one of the arguments %s is required", strings.Join(names, " "))
		}
	}
	return r, nil
}

func (p *Parser) unrecognizedError(args []string) *Error {
	quoted := make([]string, len(args))
	for i, a := range args {
		quoted[i] = "'" + a + "'"
	}
	e := p.errorf("unrecognized arguments: %s", strings.Join(quoted, " "))
	e.Unrecognized = args
	return e
}
