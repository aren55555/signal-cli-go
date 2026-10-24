package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/aren55555/signal-cli-go/internal/cli/argparse"
)

// Version is the signal-cli version being reproduced.
const Version = "0.14.7"

// loggingHelp is the early logging parser's help, printed before every
// help output (index.md §4).
const loggingHelp = "usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]\n\n" +
	"named arguments:\n" +
	"  -h, --help             show this help message and exit\n" +
	"  -v, --verbose\n" +
	"  --log-file LOG_FILE\n" +
	"  --scrub-log\n"

// plainOnly and jsonOnly are the commands with a single output mode
// (index.md §3).
var (
	plainOnly = map[string]bool{"link": true}
	jsonOnly  = map[string]bool{"jsonRpc": true}
)

// Main runs signal-cli-go with args (without the program name) and
// returns the exit code.
func Main(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer, getenv func(string) string) int {
	e := &env{stdout: stdout, stderr: stderr, stdin: stdin, getenv: getenv}
	width := 75
	if c, err := strconv.Atoi(getenv("COLUMNS")); err == nil && c > 0 {
		width = c
	}
	d := globalDefaults{dataDir: defaultDataDir(getenv), busName: "org.asamk.Signal", serviceEnv: "live", trustNew: "on-first-use"}
	if _, err := os.Stat("/.flatpak-info"); err == nil {
		d.busName = "org.asamk.SignalCli"
	}
	if err := applyConfig(&d, getenv); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	p := newParser(d)
	r, err := p.Parse(args)
	if err != nil {
		var pe *argparse.Error
		if errors.As(err, &pe) {
			io.WriteString(stderr, argparse.FormatError(pe, width))
			return 1
		}
		fmt.Fprintln(stderr, err)
		return 1
	}
	switch {
	case r.Help:
		io.WriteString(stdout, loggingHelp)
		io.WriteString(stdout, argparse.FormatHelp(r.HelpParser, width, r.HelpParser == p))
		return 0
	case r.Version:
		fmt.Fprintf(stdout, "signal-cli %s\n", Version)
		return 0
	}
	g := &globals{
		verbose:        r.Int("verbose"),
		dataDir:        r.String("data-dir"),
		account:        r.String("account"),
		output:         r.String("output"),
		serviceEnv:     r.String("service-environment"),
		trustNew:       r.String("trust-new-identities"),
		disableSendLog: r.Bool("disable-send-log"),
	}
	if g.dataDir == "" {
		g.dataDir = d.dataDir
	}
	cmd := r.Command
	if g.output == "" {
		g.output = d.output
	}
	if g.output == "" {
		g.output = "plain-text"
		if jsonOnly[cmd] {
			g.output = "json"
		}
	}
	if (plainOnly[cmd] && g.output != "plain-text") || (jsonOnly[cmd] && g.output != "json") {
		fmt.Fprintf(stderr, "Command doesn't support output type %s\n", g.output)
		return 1
	}
	if cmd == "link" && g.account != "" {
		fmt.Fprintln(stderr, "You cannot specify a account (phone number) when linking")
		return 1
	}
	if r.Bool("dbus") || r.Bool("dbus-system") {
		// D-Bus is out of scope; behave as the reference does without a bus.
		detail := "DISPLAY variable not set"
		if r.Bool("dbus-system") {
			detail = "MachineId file can not be found"
		}
		fmt.Fprintf(stderr, "Dbus client failed: Cannot Resolve Session Bus Address: %s\n", detail)
		return 2
	}
	var runErr error
	s := r.Sub
	switch cmd {
	case "version":
		if g.json() {
			fmt.Fprintf(stdout, "{\"version\":%q}\n", Version)
		} else {
			fmt.Fprintf(stdout, "signal-cli %s\n", Version)
		}
	case "link":
		name := s.String("name")
		if name == "" {
			name = "cli"
		}
		runErr = runLink(ctx, e, g, name)
	case "send":
		runErr = runSend(ctx, e, g, sendOpts{
			recipients: s.Strings("recipient"),
			message:    s.String("message"),
			hasMessage: s.Has("message"),
			fromStdin:  s.Bool("message-from-stdin"),
			noteToSelf: s.Bool("note-to-self"),
			noUrgent:   s.Bool("no-urgent"),
		})
	case "receive":
		t := s.Float("timeout")
		timeout := time.Duration(t * float64(time.Second)).Truncate(time.Millisecond)
		if t < 0 {
			timeout = 0
		}
		runErr = runReceive(ctx, e, g, receiveOpts{timeout: timeout, maxMessages: s.Int("max-messages"),
			sendReadReceipts: s.Bool("send-read-receipts"), ignoreStories: s.Bool("ignore-stories")})
	default:
		// Not implemented yet: report like an unexpected error.
		fmt.Fprintf(stderr, "Command %s is not implemented in signal-cli-go yet\n", cmd)
		return 2
	}
	var ce *cliError
	switch {
	case runErr == nil:
		return 0
	case errors.As(runErr, &ce):
		fmt.Fprintln(stderr, ce.msg)
		return ce.code
	default:
		fmt.Fprintln(stderr, runErr)
		return 2
	}
}

// defaultDataDir is $XDG_DATA_HOME/signal-cli, else
// $HOME/.local/share/signal-cli (index.md §2).
func defaultDataDir(getenv func(string) string) string {
	if x := getenv("XDG_DATA_HOME"); x != "" {
		return filepath.Join(x, "signal-cli")
	}
	return filepath.Join(getenv("HOME"), ".local", "share", "signal-cli")
}
