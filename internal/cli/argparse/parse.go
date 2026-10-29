// SPDX-License-Identifier: GPL-3.0-or-later

package argparse

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/aren55555/signal-cli-go/internal/errcode"
)

// Result holds parsed values; Sub is the chosen subcommand's result.
type Result struct {
	Values     map[string]any
	Command    string
	Sub        *Result
	Help       bool    // help was shown for HelpParser
	HelpParser *Parser // the parser whose -h it was
	Version    bool    // --version was reached
}

// String returns a string value, or "".
func (r *Result) String(k string) string { s, _ := r.Values[k].(string); return s }

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
func (r *Result) Int64(k string) (int64, bool) { v, ok := r.Values[k].(int64); return v, ok }

// Float returns a double value, or 0.
func (r *Result) Float(k string) float64 { f, _ := r.Values[k].(float64); return f }

// Bool returns a flag's value.
func (r *Result) Bool(k string) bool { b, _ := r.Values[k].(bool); return b }

// Strings returns a multi-valued argument's values.
func (r *Result) Strings(k string) []string {
	switch v := r.Values[k].(type) {
	case []string:
		return v
	case []any:
		out := make([]string, len(v))
		for i, x := range v {
			out[i] = fmt.Sprint(x)
		}
		return out
	}
	return nil
}

// Has reports whether k has a non-nil value.
func (r *Result) Has(k string) bool { return r.Values[k] != nil }

// ErrorKind tells which errors may carry suggestions (§9).
type ErrorKind int

const (
	ErrPlain ErrorKind = iota
	ErrUnknownOption
	ErrUnknownCommand
)

// Error is a parse error: Message belongs to Parser (whose usage is
// printed).
type Error struct {
	Parser  *Parser
	Code    errcode.Code
	Message string
	Kind    ErrorKind
	Token   string // the unknown option or command
}

func (e *Error) Error() string { return e.Message }

// DestOf is an argument's result key (§0).
func DestOf(a *Arg) string {
	if a.Dest != "" {
		return a.Dest
	}
	if a.isPositional() {
		return strings.ReplaceAll(a.Names[0], "-", "_")
	}
	name := a.Names[0]
	for _, n := range a.Names {
		if strings.HasPrefix(n, "--") {
			name = n
			break
		}
	}
	return strings.ReplaceAll(strings.TrimLeft(name, "-"), "-", "_")
}

func (a *Arg) isPositional() bool { return !strings.HasPrefix(a.Names[0], "-") }

func (a *Arg) takesValue() bool { return a.Action == Store }

// textualName is "-a/--account/…" or a positional's name.
func (a *Arg) textualName() string {
	if a.isPositional() {
		return a.Names[0]
	}
	return strings.Join(a.Names, "/")
}

func (a *Arg) isList() bool {
	return a.Nargs == ZeroOrMore || a.Nargs == OneOrMore || a.Nargs == Exactly
}

func (a *Arg) minMax() (int, int) {
	switch a.Nargs {
	case Optional:
		return 0, 1
	case ZeroOrMore:
		return 0, math.MaxInt
	case OneOrMore:
		return 1, math.MaxInt
	case Exactly:
		return a.N, a.N
	}
	return 1, 1
}

func isFlagShaped(s string) bool { return strings.HasPrefix(s, "-") && strings.Trim(s, "-") != "" }

var negativeInt = regexp.MustCompile(`^-[0-9]+$`)

// isFlagToken is §7.1 (before "--").
func (p *Parser) isFlagToken(s string) bool {
	if !isFlagShaped(s) {
		return false
	}
	if negativeInt.MatchString(s) {
		for _, a := range p.Args {
			for _, n := range a.Names {
				if negativeInt.MatchString(n) {
					return true
				}
			}
		}
		return false
	}
	return true
}

func (p *Parser) defaults() map[string]any {
	v := map[string]any{}
	for _, a := range p.Args {
		if a.Action == Help || a.Action == Version {
			continue
		}
		d := a.Default
		switch {
		case d != nil:
		case a.Action == StoreTrue:
			d = false
		case a.Action == Count:
			d = 0
		case a.isPositional() && a.Nargs == ZeroOrMore:
			d = []string{}
		}
		v[DestOf(a)] = d
	}
	return v
}

func (p *Parser) errorf(code errcode.Code, args ...any) *Error {
	return &Error{Parser: p, Code: code, Message: errcode.Message(code.New(args...))}
}

// argError is an error whose template starts "argument %s: ".
func (p *Parser) argError(a *Arg, code errcode.Code, args ...any) *Error {
	return p.errorf(code, append([]any{a.textualName()}, args...)...)
}

// parseInteger is §10's integer syntax: optional sign, decimal digits
// (any Unicode decimal digit).
func parseInteger(s string, bits int) (int64, bool) {
	var b strings.Builder
	for i, r := range s {
		switch {
		case i == 0 && (r == '+' || r == '-'):
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case unicode.IsDigit(r):
			// The digit's value within its block of ten.
			start := r
			for start > 0 && unicode.IsDigit(start-1) {
				start--
			}
			b.WriteRune('0' + (r-start)%10)
		default:
			return 0, false
		}
	}
	n, err := strconv.ParseInt(b.String(), 10, bits)
	return n, err == nil
}

var javaDouble = regexp.MustCompile(`^[+-]?(NaN|Infinity|((\d+\.?\d*|\.\d+)([eE][+-]?\d+)?)|(0[xX]([0-9a-fA-F]+\.?[0-9a-fA-F]*|\.[0-9a-fA-F]+)[pP][+-]?\d+))[fFdD]?$`)

// parseDouble is Java's double syntax (§10).
func parseDouble(s string) (float64, bool) {
	t := strings.TrimFunc(s, func(r rune) bool { return r <= 0x20 })
	if !javaDouble.MatchString(t) {
		return 0, false
	}
	t = strings.TrimRight(t, "fFdD")
	if strings.HasSuffix(t, "NaN") {
		return math.NaN(), true
	}
	if strings.HasSuffix(t, "Infinity") {
		if strings.HasPrefix(t, "-") {
			return math.Inf(-1), true
		}
		return math.Inf(1), true
	}
	if strings.ContainsAny(t, "xX") && !strings.Contains(t, ".") {
		// Go needs a mantissa point for hex floats only optionally; fine.
	}
	f, err := strconv.ParseFloat(t, 64)
	if err != nil {
		if ne, ok := err.(*strconv.NumError); ok && ne.Err == strconv.ErrRange {
			return f, true
		}
		return 0, false
	}
	return f, true
}

func inChoices(a *Arg, s string) bool {
	for _, c := range a.Choices {
		if c == s {
			return true
		}
	}
	return false
}

// convert applies an argument's type to a value (§10).
func (p *Parser) convert(a *Arg, s string) (any, error) {
	choices := "{" + strings.Join(a.Choices, ",") + "}"
	var v any = s
	switch a.Kind {
	case Int32, Int64:
		bits := 32
		if a.Kind == Int64 {
			bits = 64
		}
		n, ok := parseInteger(s, bits)
		if !ok {
			return nil, p.argError(a, errcode.ArgNotInteger, s, bits)
		}
		if a.Kind == Int32 {
			v = int(n)
		} else {
			v = n
		}
		if len(a.Choices) > 0 && !inChoices(a, strconv.FormatInt(n, 10)) {
			return nil, p.argError(a, errcode.ArgInvalidChoice, s, choices)
		}
		return v, nil
	case Double:
		f, ok := parseDouble(s)
		if !ok {
			return nil, p.argError(a, errcode.ArgNotDouble, s)
		}
		return f, nil
	case Enum, EnumName:
		for i, c := range a.Choices {
			if c == s {
				if a.Kind == EnumName && i < len(a.Display) {
					return a.Display[i], nil
				}
				return s, nil
			}
		}
		return nil, p.argError(a, errcode.ArgConvertChoice, s, choices)
	case Boolean:
		return strings.EqualFold(s, "true"), nil
	}
	if len(a.Choices) > 0 && !inChoices(a, s) {
		return nil, p.argError(a, errcode.ArgInvalidChoice, s, choices)
	}
	return v, nil
}

func (p *Parser) flagArg(name string) *Arg {
	for _, a := range p.Args {
		if a.isPositional() {
			continue
		}
		for _, n := range a.Names {
			if n == name {
				return a
			}
		}
	}
	return nil
}

// resolve finds the argument for a flag part (§7.2 steps 1-5); nil, nil
// means unknown.
func (p *Parser) resolve(part string) (*Arg, error) {
	if a := p.flagArg(part); a != nil {
		return a, nil
	}
	var cands []string
	owner := map[string]*Arg{}
	for _, a := range p.Args {
		if a.isPositional() {
			continue
		}
		for _, n := range a.Names {
			if strings.HasPrefix(n, part) {
				cands = append(cands, n)
				owner[n] = a
			}
		}
	}
	if len(cands) == 0 {
		return nil, nil
	}
	if !strings.HasPrefix(part, "--") && len(part) > 2 && p.flagArg(part[:2]) != nil {
		cands = append(cands, part[:2])
	}
	if len(cands) == 1 {
		return owner[cands[0]], nil
	}
	sort.Slice(cands, func(i, j int) bool { return lessUTF16(cands[i], cands[j]) })
	return nil, p.errorf(errcode.ArgAmbiguousOption, part, strings.Join(cands, ", "))
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

// parseState is one parser's run (§7.2).
type parseState struct {
	p           *Parser
	r           *Result
	used        map[*Arg]bool
	groupUser   map[*Group]*Arg
	positionals []string
	afterDashes bool
	deferred    *Error
}

// Parse parses args (without the program name).
func (p *Parser) Parse(args []string) (*Result, error) {
	r, deferred, err := p.parse(args)
	if err != nil {
		return nil, err
	}
	if deferred != nil && !r.Help && !r.Version {
		return nil, deferred
	}
	return r, nil
}

func (p *Parser) positionalArgs() []*Arg {
	var out []*Arg
	for _, a := range p.Args {
		if a.isPositional() {
			out = append(out, a)
		}
	}
	return out
}

func (s *parseState) positionalCapacity() int {
	total := 0
	for _, a := range s.p.positionalArgs() {
		_, mx := a.minMax()
		if mx == math.MaxInt {
			return math.MaxInt
		}
		total += mx
	}
	return total
}

func (p *Parser) parse(args []string) (*Result, *Error, error) {
	s := &parseState{p: p, r: &Result{Values: p.defaults()}, used: map[*Arg]bool{}, groupUser: map[*Group]*Arg{}}
	for i := 0; i < len(args); i++ {
		tok := args[i]
		if !s.afterDashes && tok == "--" {
			s.afterDashes = true
			continue
		}
		if !s.afterDashes && p.isFlagToken(tok) {
			consumed, stop, err := s.flag(args, i)
			if err != nil {
				return nil, nil, err
			}
			if stop {
				return s.r, nil, nil
			}
			i += consumed
			continue
		}
		// A positional token.
		if len(s.positionals) < s.positionalCapacity() {
			s.positionals = append(s.positionals, tok)
			continue
		}
		if !s.afterDashes && len(p.Commands) > 0 {
			cmd, err := p.chooseCommand(tok)
			if err != nil {
				return nil, nil, err
			}
			if err := s.finish(); err != nil {
				return nil, nil, err
			}
			s.r.Command = cmd.Name
			sub, subDeferred, err := cmd.Parser.parse(args[i+1:])
			if err != nil {
				return nil, nil, err
			}
			s.r.Sub = sub
			s.r.Help, s.r.HelpParser, s.r.Version = sub.Help, sub.HelpParser, sub.Version
			if s.deferred == nil {
				s.deferred = subDeferred
			}
			return s.r, s.deferred, nil
		}
		return nil, nil, p.errorf(errcode.ArgUnrecognized, strings.Join(args[i:], " "))
	}
	if len(p.Commands) > 0 {
		return nil, nil, p.errorf(errcode.ArgTooFew)
	}
	if err := s.finish(); err != nil {
		return nil, nil, err
	}
	return s.r, s.deferred, nil
}

func (p *Parser) chooseCommand(tok string) (*Command, error) {
	for _, c := range p.Commands {
		if c.Name == tok {
			return c, nil
		}
	}
	var matches []string
	var found *Command
	if tok != "" {
		for _, c := range p.Commands {
			if strings.HasPrefix(c.Name, tok) {
				matches = append(matches, c.Name)
				found = c
			}
		}
	}
	switch {
	case len(matches) == 1:
		return found, nil
	case len(matches) > 1:
		sort.Slice(matches, func(i, j int) bool { return lessUTF16(matches[i], matches[j]) })
		return nil, p.errorf(errcode.ArgAmbiguousCommand, tok, strings.Join(matches, ", "))
	}
	names := make([]string, len(p.Commands))
	for i, c := range p.Commands {
		names[i] = "'" + c.Name + "'"
	}
	e := p.errorf(errcode.ArgInvalidCommand, tok, strings.Join(names, ", "))
	e.Kind, e.Token = ErrUnknownCommand, tok
	return nil, e
}

// select runs the mutual-exclusion check for a selected flag (§7.5).
func (s *parseState) selectArg(a *Arg) error {
	if g := s.p.groupOf(a); g != nil {
		if prev := s.groupUser[g]; prev != nil && prev != a {
			return s.p.argError(a, errcode.ArgNotAllowedWith, prev.textualName())
		}
		s.groupUser[g] = a
	}
	s.used[a] = true
	return nil
}

// runValueless runs a flag that takes no value; stop is true for help
// and version.
func (s *parseState) runValueless(a *Arg) (stop bool) {
	dest := DestOf(a)
	switch a.Action {
	case Help:
		s.r.Help, s.r.HelpParser = true, s.p
		return true
	case Version:
		s.r.Version = true
		return true
	case StoreTrue:
		s.r.Values[dest] = true
	case Count:
		s.r.Values[dest] = s.r.Int(dest) + 1
	}
	return false
}

// flag handles a flag token at args[i]; it returns how many extra tokens
// were consumed and whether parsing stops (help or version).
func (s *parseState) flag(args []string, i int) (int, bool, error) {
	p := s.p
	tok := args[i]
	part, attached, hasAttached := strings.Cut(tok, "=")
	a, err := p.resolve(part)
	if err != nil {
		return 0, false, err
	}
	if a == nil {
		// Concatenated short flags (§7.2 "Unknown flag").
		if !strings.HasPrefix(tok, "--") {
			rs := []rune(tok[1:])
			for k, c := range rs {
				f := p.flagArg("-" + string(c))
				if f == nil {
					break
				}
				if err := s.selectArg(f); err != nil {
					return 0, false, err
				}
				if !f.takesValue() {
					if s.runValueless(f) {
						return 0, true, nil
					}
					if k == len(rs)-1 {
						return 0, false, nil
					}
					continue
				}
				rest := string(rs[k+1:])
				return s.values(f, args, i, rest, rest != "")
			}
		}
		e := p.errorf(errcode.ArgUnrecognized, tok)
		e.Kind, e.Token = ErrUnknownOption, tok
		return 0, false, e
	}
	if err := s.selectArg(a); err != nil {
		return 0, false, err
	}
	if !a.takesValue() {
		if hasAttached {
			return 0, false, p.argError(a, errcode.ArgIgnoredValue, attached)
		}
		return 0, s.runValueless(a), nil
	}
	return s.values(a, args, i, attached, hasAttached)
}

// values reads a value-taking option's values (§7.3).
func (s *parseState) values(a *Arg, args []string, i int, attached string, hasAttached bool) (int, bool, error) {
	p := s.p
	dest := DestOf(a)
	isValue := func(k int) bool { return k < len(args) && args[k] != "--" && !p.isFlagToken(args[k]) }
	if !a.isList() {
		var val string
		consumed := 0
		switch {
		case hasAttached:
			val = attached
		case isValue(i + 1):
			val, consumed = args[i+1], 1
		case a.Nargs == Optional:
			s.r.Values[dest] = a.Const
			return 0, false, nil
		default:
			return 0, false, p.argError(a, errcode.ArgExpectedOne)
		}
		v, err := p.convert(a, val)
		if err != nil {
			return 0, false, err
		}
		s.r.Values[dest] = v
		return consumed, false, nil
	}
	mn, mx := a.minMax()
	var raw []string
	consumed := 0
	if hasAttached {
		raw = []string{attached}
	} else {
		for k := i + 1; len(raw) < mx && isValue(k); k++ {
			raw = append(raw, args[k])
			consumed++
		}
	}
	if len(raw) < mn {
		return 0, false, p.argError(a, errcode.ArgExpectedN, mn)
	}
	out := []any{}
	for _, x := range raw {
		v, err := p.convert(a, x)
		if err != nil {
			return 0, false, err
		}
		out = append(out, v)
	}
	s.r.Values[dest] = listValue(out)
	return consumed, false, nil
}

// listValue stores string lists as []string for convenience.
func listValue(vs []any) any {
	ss := make([]string, 0, len(vs))
	for _, v := range vs {
		x, ok := v.(string)
		if !ok {
			return vs
		}
		ss = append(ss, x)
	}
	return ss
}

// finish distributes positionals (§7.4) and records the deferred
// required checks (§7.5).
func (s *parseState) finish() error {
	p := s.p
	pos := p.positionalArgs()
	toks := s.positionals
	minRest := func(from int) int {
		n := 0
		for _, a := range pos[from:] {
			mn, _ := a.minMax()
			n += mn
		}
		return n
	}
	if minRest(0) > len(toks) {
		return p.errorf(errcode.ArgTooFew)
	}
	for k, a := range pos {
		mn, mx := a.minMax()
		avail := len(toks) - minRest(k+1)
		take := 0
		switch a.Nargs {
		case One:
			take = 1
		case Optional:
			if avail > 0 {
				take = 1
			}
		default:
			take = min(avail, mx)
			if take < mn {
				take = mn
			}
		}
		if take == 0 && a.isList() {
			toks = toks[take:]
			continue
		}
		vals := toks[:take]
		toks = toks[take:]
		if a.isList() {
			out := []any{}
			for _, x := range vals {
				v, err := p.convert(a, x)
				if err != nil {
					return err
				}
				out = append(out, v)
			}
			s.r.Values[DestOf(a)] = listValue(out)
		} else if take == 1 {
			v, err := p.convert(a, vals[0])
			if err != nil {
				return err
			}
			s.r.Values[DestOf(a)] = v
		}
	}
	if s.deferred != nil {
		return nil
	}
	for _, a := range p.Args {
		if a.Required && !a.isPositional() && !s.used[a] && s.p.groupOf(a) == nil {
			s.deferred = p.errorf(errcode.ArgRequired, a.textualName())
		}
	}
	if s.deferred == nil {
		for _, g := range p.Groups {
			if g.Required && s.groupUser[g] == nil {
				var b strings.Builder
				for _, a := range g.Args {
					b.WriteString(a.textualName() + " ")
				}
				s.deferred = p.errorf(errcode.ArgOneOfRequired, strings.TrimSuffix(b.String(), " "))
			}
		}
	}
	return nil
}

// Defaults are the parser's stored defaults (a fresh map).
func (p *Parser) Defaults() map[string]any { return p.defaults() }

// IsList reports whether the argument takes a list of values.
func (a *Arg) IsList() bool { return a.isList() }
