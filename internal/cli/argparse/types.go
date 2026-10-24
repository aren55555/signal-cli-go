// Package argparse reproduces the argument parsing, help and error text
// of argparse4j 0.9.0 as signal-cli configures it
// (spec/surface/cli/index.md §1, §4, §5; spec/surface/cli/argparse.md).
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
	One        Nargs = iota // exactly one
	Optional                // '?': zero or one
	ZeroOrMore              // '*'
	OneOrMore               // '+'
)

// Kind is the value type, which decides the conversion error text.
type Kind int

const (
	String     Kind = iota
	Int32           // "could not convert '%s' to integer (32 bits)"
	Int64           // "... (64 bits)"
	Double          // "... to number (double precision)"
	Enum            // choices, "could not convert '%s' (choose from {a,b})"
	Choice          // choices, "invalid choice: '%s' (choose from {a,b})"
	BoolChoice      // shown as {true,false}, any value accepted
)

// Arg is one option or positional.
type Arg struct {
	Names    []string // "-a", "--account", …; a positional has one name without dashes
	Dest     string   // key in the result; defaults from the first long name
	Metavar  string   // defaults to Dest upper-cased
	Action   Action
	Nargs    Nargs
	Kind     Kind
	Choices  []string
	Required bool
	Help     string
	Default  any // shown as "(default: X)" in top-level help when non-nil
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
	Prog        string // "signal-cli" or "signal-cli send"
	Description string
	Epilog      string
	Args        []*Arg   // in declaration order
	Groups      []*Group // their args are also in Args
	Commands    []*Command
	CommandDest string
}
