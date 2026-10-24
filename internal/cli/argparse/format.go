package argparse

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Usage, help and error output (spec/surface/cli/argparse.md §5, §6, §8).

// metavars is the argument's metavar list (§5.3).
func (a *Arg) metavars() []string {
	switch {
	case len(a.Metavars) > 0:
		return a.Metavars
	case len(a.Choices) > 0:
		return []string{"{" + strings.Join(a.Choices, ",") + "}"}
	case a.Kind == Boolean:
		return []string{"{true,false}"}
	case a.isPositional():
		return []string{DestOf(a)}
	}
	return []string{strings.ToUpper(DestOf(a))}
}

// valueForm is the nargs-dependent value text (§5.3).
func (a *Arg) valueForm() string {
	if !a.takesValue() {
		return ""
	}
	ms := a.metavars()
	m, m2 := ms[0], ms[0]
	if len(ms) > 1 {
		m2 = ms[1]
	}
	switch a.Nargs {
	case Optional:
		return "[" + m + "]"
	case ZeroOrMore:
		return "[" + m + " [" + m2 + " ...]]"
	case OneOrMore:
		return m + " [" + m2 + " ...]"
	case Exactly:
		parts := make([]string, a.N)
		for i := range parts {
			parts[i] = ms[min(i, len(ms)-1)]
		}
		return strings.Join(parts, " ")
	}
	return m
}

// syntax is the usage form, without brackets.
func (a *Arg) syntax() string {
	if a.isPositional() {
		return a.valueForm()
	}
	if v := a.valueForm(); v != "" {
		return a.Names[0] + " " + v
	}
	return a.Names[0]
}

func (a *Arg) usageItem() string {
	if a.isPositional() || a.Required {
		return a.syntax()
	}
	return "[" + a.syntax() + "]"
}

// title is the help row title (§6.2).
func (a *Arg) title() string {
	if a.isPositional() {
		return a.metavars()[0]
	}
	v := a.valueForm()
	parts := make([]string, len(a.Names))
	for i, n := range a.Names {
		parts[i] = n
		if v != "" {
			parts[i] += " " + v
		}
	}
	return strings.Join(parts, ", ")
}

func groupItems(g *Group) []string {
	var vis []*Arg
	for _, a := range g.Args {
		if !a.Hidden {
			vis = append(vis, a)
		}
	}
	switch {
	case len(vis) == 0:
		return nil
	case len(vis) == 1:
		if g.Required {
			return []string{vis[0].syntax()}
		}
		return []string{vis[0].usageItem()}
	}
	open, close := "[", "]"
	if g.Required {
		open, close = "(", ")"
	}
	items := []string{open + vis[0].syntax()}
	for i, a := range vis[1:] {
		item := a.syntax()
		if i == len(vis)-2 {
			item += close
		}
		items = append(items, "|", item)
	}
	return items
}

// usageItems lists the usage items after "usage: prog" (§5.2).
func (p *Parser) usageItems() []string {
	var items []string
	// Enclosing parsers, outermost first.
	var ancestors []*Parser
	for q := p.parent; q != nil; q = q.parent {
		ancestors = append([]*Parser{q}, ancestors...)
	}
	for _, q := range ancestors {
		if len(q.chain) > 0 {
			items = append(items, q.chain[len(q.chain)-1])
		}
		for _, a := range q.Args {
			if a.Required && !a.isPositional() && q.groupOf(a) == nil {
				items = append(items, a.syntax())
			}
		}
		for _, g := range q.Groups {
			if g.Required || (len(g.Args) == 1 && g.Args[0].Required) {
				items = append(items, groupItems(g)...)
			}
		}
		for _, a := range q.positionalArgs() {
			items = append(items, a.usageItem())
		}
	}
	if len(p.chain) > 0 {
		items = append(items, p.chain[len(p.chain)-1])
	}
	for _, a := range p.Args {
		if !a.isPositional() && !a.Hidden && p.groupOf(a) == nil {
			items = append(items, a.usageItem())
		}
	}
	for _, g := range p.Groups {
		items = append(items, groupItems(g)...)
	}
	for _, a := range p.positionalArgs() {
		if !a.Hidden {
			items = append(items, a.usageItem())
		}
	}
	if len(p.Commands) > 0 {
		items = append(items, p.choiceList(), "...")
	}
	return items
}

func (p *Parser) choiceList() string {
	names := make([]string, len(p.Commands))
	for i, c := range p.Commands {
		names[i] = c.Name
	}
	return "{" + strings.Join(names, ",") + "}"
}

// FormatUsage is the usage line(s) (§5).
func FormatUsage(p *Parser) string {
	w := p.width()
	prefix := "usage: " + p.Prog
	u := width(prefix)
	var b strings.Builder
	b.WriteString(prefix)
	indent := strings.Repeat(" ", u)
	count := u
	if u > 30 {
		b.WriteString("\n" + "      ")
		indent = "      "
		count = 12
	}
	for i, item := range p.usageItems() {
		iw := width(item)
		if i > 0 && count+1+iw > w {
			b.WriteString("\n" + indent + " " + item)
			count = width(indent) + 1 + iw
			continue
		}
		b.WriteString(" " + item)
		count += 1 + iw
	}
	b.WriteString("\n")
	return b.String()
}

// javaString renders a default like Java's toString() (§6.3).
func javaString(v any) string {
	switch x := v.(type) {
	case bool:
		return strconv.FormatBool(x)
	case float64:
		return javaDoubleString(x)
	case []string:
		return "[" + strings.Join(x, ", ") + "]"
	}
	return fmt.Sprint(v)
}

func javaDoubleString(f float64) string {
	switch {
	case math.IsNaN(f):
		return "NaN"
	case math.IsInf(f, 1):
		return "Infinity"
	case math.IsInf(f, -1):
		return "-Infinity"
	case f == 0:
		if math.Signbit(f) {
			return "-0.0"
		}
		return "0.0"
	}
	a := math.Abs(f)
	if a >= 1e-3 && a < 1e7 {
		s := strconv.FormatFloat(f, 'f', -1, 64)
		if !strings.Contains(s, ".") {
			s += ".0"
		}
		return s
	}
	s := strconv.FormatFloat(f, 'E', -1, 64)
	mant, exp, _ := strings.Cut(s, "E")
	if !strings.Contains(mant, ".") {
		mant += ".0"
	}
	e, _ := strconv.Atoi(exp)
	return mant + "E" + strconv.Itoa(e)
}

func (p *Parser) rowHelp(a *Arg) string {
	h := a.Help
	def := a.Default
	switch {
	case def != nil:
	case a.Action == StoreTrue:
		def = false
	case a.Action == Count:
		def = 0
	}
	if p.ShowDefaults && (def != nil || a.DefaultText != "") {
		d := a.DefaultText
		if d == "" {
			d = javaString(def)
		}
		if h != "" {
			h += " "
		}
		h += "(default: " + d + ")"
	}
	return h
}

// FormatHelp is the help screen (§6).
func FormatHelp(p *Parser) string {
	w := p.width()
	var b strings.Builder
	b.WriteString(FormatUsage(p))
	if p.Description != "" {
		b.WriteString("\n" + Wrap(p.Description, w, 0, "", "") + "\n")
	}
	pos := p.positionalArgs()
	untitled := len(p.Commands) > 0 && p.CommandsTitle == "" && p.CommandsDesc == ""
	if len(pos) > 0 || untitled {
		b.WriteString("\npositional arguments:\n")
		for _, a := range pos {
			if !a.Hidden {
				b.WriteString(helpRow(a.title(), p.rowHelp(a), w))
			}
		}
		if untitled {
			p.commandRows(&b)
		}
	}
	var flags []*Arg
	for _, a := range p.Args {
		if !a.isPositional() && !a.Hidden {
			flags = append(flags, a)
		}
	}
	if len(flags) > 0 {
		b.WriteString("\nnamed arguments:\n")
		for _, a := range flags {
			b.WriteString(helpRow(a.title(), p.rowHelp(a), w))
		}
	}
	if len(p.Commands) > 0 && !untitled {
		title := p.CommandsTitle
		if title == "" {
			title = "subcommands"
		}
		b.WriteString("\n" + title + ":\n")
		if p.CommandsDesc != "" {
			b.WriteString("  " + Wrap(p.CommandsDesc, w, 2, "", "  ") + "\n\n")
		}
		p.commandRows(&b)
	}
	if p.Epilog != "" {
		b.WriteString("\n" + Wrap(p.Epilog, w, 0, "", "") + "\n")
	}
	return b.String()
}

func (p *Parser) commandRows(b *strings.Builder) {
	w := p.width()
	b.WriteString(helpRow(p.choiceList(), p.CommandsHelp, w))
	for _, c := range p.Commands {
		if c.Help != "" {
			b.WriteString(helpRow("  "+c.Name, c.Help, w))
		}
	}
}

func body(s string) string { return strings.TrimLeft(s, "-") }

// FormatError is the usage, the justified error line and any
// suggestions (§8, §9).
func FormatError(e *Error) string {
	p := e.Parser
	out := FormatUsage(p) + Wrap(p.Prog+": error: "+e.Message+"\n", p.width(), 0, "", "")
	switch e.Kind {
	case ErrUnknownOption:
		if width(body(e.Token)) >= 2 {
			var cands []string
			for _, a := range p.Args {
				for _, n := range a.Names {
					if strings.HasPrefix(n, "-") && width(body(n)) >= 2 {
						cands = append(cands, n)
					}
				}
			}
			out += suggest(e.Token, cands, body)
		}
	case ErrUnknownCommand:
		var cands []string
		for _, c := range p.Commands {
			cands = append(cands, c.Name)
		}
		out += suggest(e.Token, cands, func(s string) string { return s })
	}
	return out
}
