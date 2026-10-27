// Package errcode is the catalogue of every error signal-cli-go's CLI and
// JSON-RPC daemon can report. Each Code carries the JSON-RPC error code,
// the CLI exit code, the reference's exact message template and a short
// cause. Errors travel as *Error values that wrap their underlying
// cause; the verbatim reference text is rendered only at the output
// edge, by Message and RPCMessage.
//
// The generated table is docs/error-codes.md (go test ./internal/errcode
// -run TestTable -update).
package errcode

import (
	"errors"
	"fmt"
	"strings"
)

// Code is one distinct error. It implements error so that
// errors.Is(err, errcode.X) works on any *Error carrying X.
type Code int

// Kind groups codes by how they surface.
type Kind int

const (
	KindProtocol   Kind = iota // JSON-RPC framing and validation (daemon only)
	KindHTTP                   // an HTTP status from the daemon's HTTP server
	KindArgparse               // command-line parse error (CLI only, exit 1)
	KindStartup                // CLI start-up failure before any command runs
	KindUser                   // UserErrorException: exit 1, JSON-RPC -1
	KindUnexpected             // UnexpectedErrorException: exit 2, JSON-RPC -32603
	KindIO                     // IOErrorException: exit 3, JSON-RPC -3
	KindUntrusted              // UntrustedKeyErrorException: exit 4, JSON-RPC -4
	KindRateLimit              // RateLimitErrorException: exit 5, JSON-RPC -5
	KindCaptcha                // CaptchaRejectedErrorException: exit 6, JSON-RPC -6
	KindCrash                  // an uncaught Java exception: exit 2, JSON-RPC -32603
)

var kindNames = [...]string{"protocol", "http", "argparse", "startup", "user", "unexpected", "io", "untrusted", "rate-limit", "captcha", "crash"}

func (k Kind) String() string { return kindNames[k] }

// Source says where a code's message is known from.
type Source int

const (
	// Observed codes appear in a recorded golden transcript.
	Observed Source = iota
	// Spec codes are known from spec/ only; no golden exercises them.
	Spec
	// Deviation codes are signal-cli-go's own: the reference has no
	// such error (an unimplemented feature, a refused input).
	Deviation
)

var sourceNames = [...]string{"golden", "spec", "deviation"}

func (s Source) String() string { return sourceNames[s] }

type info struct {
	name   string
	kind   Kind
	rpc    int    // JSON-RPC error code; 0 when not reachable over JSON-RPC
	exit   int    // CLI exit code; 0 when not reachable on the CLI
	http   int    // HTTP status for KindHTTP
	msg    string // reference message template (CLI stderr, HTTP body)
	rpcMsg string // JSON-RPC message template when it differs from msg
	cause  string
	source Source
}

func (c Code) info() *info {
	if c <= 0 || int(c) >= len(table) || table[c].name == "" {
		return &info{name: fmt.Sprintf("Code(%d)", int(c)), kind: KindUnexpected, exit: 2, rpc: -32603, msg: "%s", cause: "unknown error code"}
	}
	return &table[c]
}

// String is the code's Go name.
func (c Code) String() string { return c.info().name }

// Error makes a bare Code usable as an error and as an errors.Is target.
func (c Code) Error() string { return c.info().name + ": " + c.info().cause }

func (c Code) Kind() Kind      { return c.info().kind }
func (c Code) RPCCode() int    { return c.info().rpc }
func (c Code) ExitCode() int   { return c.info().exit }
func (c Code) HTTPStatus() int { return c.info().http }
func (c Code) Cause() string   { return c.info().cause }
func (c Code) Source() Source  { return c.info().source }

// Template is the reference's message template (fmt verbs for the
// variable parts) as printed by the CLI or sent as an HTTP body.
func (c Code) Template() string { return c.info().msg }

// RPCTemplate is the message template used in JSON-RPC error objects.
func (c Code) RPCTemplate() string {
	i := c.info()
	switch {
	case i.rpcMsg != "":
		return i.rpcMsg
	case i.kind == KindUnexpected:
		return i.msg + " (UnexpectedErrorException)"
	}
	return i.msg
}

// All enumerates every code in declaration order.
func All() []Code {
	var out []Code
	for c := Code(1); int(c) < len(table); c++ {
		if table[c].name != "" {
			out = append(out, c)
		}
	}
	return out
}

// New makes an error for c with the template's arguments.
func (c Code) New(args ...any) *Error { return &Error{Code: c, Args: args} }

// Wrap makes an error for c caused by err.
func (c Code) Wrap(err error, args ...any) *Error { return &Error{Code: c, Args: args, Err: err} }

// WrapText is Wrap for templates whose only argument is the cause's own
// text (e.g. "UpdateAccount error: %s").
func (c Code) WrapText(err error) *Error { return &Error{Code: c, Args: []any{err}, Err: err} }

// Error is a Code with its message arguments, its underlying cause and,
// for JSON-RPC, optional error data.
type Error struct {
	Code Code
	Args []any
	Err  error
	// Data is the JSON-RPC error data (already-encoded JSON); empty
	// means null.
	Data string
}

// Error describes the failure for logs; it is not the reference text.
func (e *Error) Error() string {
	s := e.Code.String() + ": " + e.Code.Cause()
	if e.Err != nil {
		s += ": " + e.Err.Error()
	}
	return s
}

func (e *Error) Unwrap() error { return e.Err }

// Is matches the bare Code.
func (e *Error) Is(target error) bool {
	c, ok := target.(Code)
	return ok && c == e.Code
}

// WithData attaches JSON-RPC error data.
func (e *Error) WithData(data string) *Error { e.Data = data; return e }

// render fills a template. An *Error argument renders as its CLI
// message (a nested reference message, e.g. "Failed to send to group:
// Group not found: ...").
func render(tmpl string, args []any) string {
	if len(args) == 0 {
		return tmpl
	}
	vs := make([]any, len(args))
	for i, a := range args {
		if e, ok := a.(*Error); ok {
			a = render(e.Code.Template(), e.Args)
		}
		vs[i] = a
	}
	return fmt.Sprintf(tmpl, vs...)
}

// Message is the CLI text (or HTTP body) for err. Errors that carry no
// code render as Internal.
func Message(err error) string {
	e := As(err)
	return render(e.Code.Template(), e.Args)
}

// RPCMessage is the JSON-RPC error message for err.
func RPCMessage(err error) string {
	e := As(err)
	return render(e.Code.RPCTemplate(), e.Args)
}

// ExitCode is the CLI exit status for err (0 for nil).
func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	return As(err).Code.ExitCode()
}

// RPCCode is the JSON-RPC error code for err.
func RPCCode(err error) int { return As(err).Code.RPCCode() }

// As finds the *Error in err's chain; an error without one becomes
// Internal wrapping it, and a bare Code an *Error without arguments.
func As(err error) *Error {
	var e *Error
	if errors.As(err, &e) {
		return e
	}
	var c Code
	if errors.As(err, &c) {
		return &Error{Code: c}
	}
	return Internal.WrapText(err)
}

// Markdown renders the generated table of all codes.
func Markdown() string {
	var b strings.Builder
	b.WriteString("# signal-cli-go error codes\n\n")
	b.WriteString("Generated from `internal/errcode` by `go test ./internal/errcode -run TestTable -update`; do not edit.\n\n")
	b.WriteString("Every error the CLI or the JSON-RPC daemon reports is one of these codes. ")
	b.WriteString("`exit` is the CLI exit status, `rpc` the JSON-RPC error code and `http` the HTTP status (blank when the error cannot occur there). ")
	b.WriteString("Messages are the reference's exact text with Go `fmt` verbs for the variable parts; ")
	b.WriteString("a JSON-RPC message is shown separately when it differs. ")
	b.WriteString("`source` is `golden` when a recorded transcript shows the message, `spec` when it is known from `spec/` only, ")
	b.WriteString("and `deviation` for signal-cli-go's own errors.\n\n")
	b.WriteString("| code | kind | exit | rpc | http | message | cause | source |\n|---|---|---|---|---|---|---|---|\n")
	num := func(n int) string {
		if n == 0 {
			return ""
		}
		return fmt.Sprint(n)
	}
	cell := func(s string) string {
		s = strings.ReplaceAll(s, "\n", `\n`)
		s = strings.ReplaceAll(s, "|", `\|`)
		return "`` " + s + " ``"
	}
	for _, c := range All() {
		i := c.info()
		msg := cell(i.msg)
		if i.rpc != 0 && i.exit != 0 && c.RPCTemplate() != i.msg {
			msg += "<br>rpc: " + cell(c.RPCTemplate())
		} else if i.exit == 0 && i.rpc != 0 {
			msg = cell(c.RPCTemplate())
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s | %s | %s |\n", i.name, i.kind, num(i.exit), num(i.rpc), num(i.http), msg, i.cause, i.source)
	}
	return b.String()
}
