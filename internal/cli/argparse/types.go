// SPDX-License-Identifier: GPL-3.0-or-later

// Package argparse reproduces the argument parsing, help and error text
// of argparse4j 0.9.0 as signal-cli configures it
// (spec/surface/cli/argparse.md, refining spec/surface/cli/index.md).
package argparse

// Action is what an argument does when it appears.
type Action int

const (
	Store     Action = iota // takes values per Nargs
	StoreTrue               // flag
	Count                   // flag, counts occurrences (-vvv)
	Help                    // prints help and exits 0
	Version                 // prints the version and exits 0
)

// Nargs is how many values an argument takes.
type Nargs int

const (
	One        Nargs = iota // exactly one, stored as a single value
	Optional                // '?': zero or one
	ZeroOrMore              // '*'
	OneOrMore               // '+'
	Exactly                 // exactly N values (list); N in Arg.N
)

// Kind is the value type, which decides conversion and its error text
// (§10).
type Kind int

const (
	String   Kind = iota
	Int32         // "could not convert '%s' to integer (32 bits)"
	Int64         // "... (64 bits)"
	Double        // "... to  number (double precision)"
	Enum          // display-string enum: "could not convert '%s' (choose from {a,b})"
	EnumName      // constant-name enum, same message
	Boolean       // shown as {true,false}; any value; true iff "true" ignoring case
)

// Arg is one option or positional.
type Arg struct {
	Names       []string // "-a", "--account", …; a positional has one name without dashes
	Dest        string   // explicit dest; else derived (§0)
	Metavars    []string // explicit metavar(s)
	Action      Action
	Nargs       Nargs
	N           int // for Exactly
	Kind        Kind
	Choices     []string // for Enum/EnumName: the values; else a choices restriction
	Required    bool
	Help        string
	Default     any      // stored default
	Const       any      // stored when an Optional option is given without a value
	DefaultText string   // help's "(default: X)"; derived from Default if empty
	Display     []string // EnumName: display strings matching Choices, stored as the value
	Hidden      bool
}

// Group is a mutually exclusive group.
type Group struct {
	Args     []*Arg
	Required bool
}

// Command is a subcommand.
type Command struct {
	Name   string
	Help   string // "" means no row in the subcommand list
	Parser *Parser
}

// Parser is one argument parser (top level or a subcommand).
type Parser struct {
	Prog         string // the program name, also for subcommand parsers
	Description  string
	Epilog       string
	Args         []*Arg   // in declaration order
	Groups       []*Group // their args are also in Args
	ShowDefaults bool     // "(default: X)" annotations in help

	Commands      []*Command
	CommandDest   string
	CommandsTitle string // subparsers title ("subcommands")
	CommandsHelp  string // help of the choice-list row
	CommandsDesc  string // subparsers description
	VersionText   string
	Width         int // W; 0 means 75
	parent        *Parser
	chain         []string // command names from the top, for usage
}

func (p *Parser) width() int {
	for q := p; q != nil; q = q.parent {
		if q.Width > 0 {
			return q.Width
		}
	}
	return 75
}

// Link sets parent pointers and command chains; call once after building.
func (p *Parser) Link() {
	for _, c := range p.Commands {
		c.Parser.parent = p
		c.Parser.chain = append(append([]string{}, p.chain...), c.Name)
		if c.Parser.Prog == "" {
			c.Parser.Prog = p.Prog
		}
		c.Parser.Link()
	}
}
