package argparse

import "strings"

// FormatHelp, FormatUsage and FormatError render text as argparse4j does
// (spec/surface/cli/argparse.md). Placeholder layout until that spec
// lands: correct content, unjustified.

// FormatError is the usage block, "signal-cli: error: <message>" and any
// suggestions.
func FormatError(e *Error, width int) string {
	return FormatUsage(e.Parser, width) + "signal-cli: error: " + e.Message + "\n"
}

// FormatUsage is the usage block.
func FormatUsage(p *Parser, width int) string {
	return "usage: " + p.Prog + "\n"
}

// FormatHelp is the help of p; top marks the top-level parser.
func FormatHelp(p *Parser, width int, top bool) string {
	var b strings.Builder
	b.WriteString(FormatUsage(p, width))
	return b.String()
}
