package argparse

import (
	"testing"
)

func testParser() *Parser {
	dbus := &Arg{Names: []string{"--dbus"}, Action: StoreTrue}
	dbusSystem := &Arg{Names: []string{"--dbus-system"}, Action: StoreTrue}
	msg := &Arg{Names: []string{"-m", "--message"}, Dest: "message"}
	stdin := &Arg{Names: []string{"--message-from-stdin"}, Action: StoreTrue}
	send := &Parser{Prog: "signal-cli send", Args: []*Arg{
		{Names: []string{"-h", "--help"}, Action: Help},
		{Names: []string{"recipient"}, Nargs: ZeroOrMore},
		{Names: []string{"-g", "--group-id", "--group"}, Dest: "group-id", Nargs: ZeroOrMore},
		msg, stdin,
		{Names: []string{"--quote-timestamp"}, Kind: Int64},
	}, Groups: []*Group{{Args: []*Arg{msg, stdin}}}}
	receive := &Parser{Prog: "signal-cli receive", Args: []*Arg{
		{Names: []string{"-h", "--help"}, Action: Help},
		{Names: []string{"-t", "--timeout"}, Kind: Double, Default: 3.0},
		{Names: []string{"--max-messages"}, Kind: Int32, Default: -1},
	}}
	return &Parser{Prog: "signal-cli", CommandDest: "command", Args: []*Arg{
		{Names: []string{"-h", "--help"}, Action: Help},
		{Names: []string{"--version"}, Action: Version},
		{Names: []string{"-v", "--verbose"}, Action: Count, Default: 0},
		{Names: []string{"-d", "--data-dir", "-c", "--config"}, Dest: "data-dir"},
		{Names: []string{"-a", "--account", "-u", "--username"}, Dest: "account"},
		dbus, dbusSystem,
		{Names: []string{"-o", "--output"}, Kind: Enum, Choices: []string{"plain-text", "json"}},
		{Names: []string{"--disable-send-log"}, Action: StoreTrue},
	}, Groups: []*Group{{Args: []*Arg{dbus, dbusSystem}}},
		Commands: []*Command{{Name: "link", Parser: &Parser{Prog: "signal-cli link", Args: []*Arg{{Names: []string{"-h", "--help"}, Action: Help}, {Names: []string{"-n", "--name"}, Dest: "name"}}}},
			{Name: "receive", Parser: receive}, {Name: "send", Parser: send}, {Name: "trust", Parser: &Parser{Prog: "signal-cli trust"}}, {Name: "version", Parser: &Parser{Prog: "signal-cli version", Args: []*Arg{{Names: []string{"-h", "--help"}, Action: Help}}}}}}
}

func TestParseOK(t *testing.T) {
	p := testParser()
	r, err := p.Parse([]string{"--config", "/x", "-vvv", "-ojson", "--acc=+15555550100", "send", "-m", "hi", "a", "b", "-g", "G1", "G2"})
	if err != nil {
		t.Fatal(err)
	}
	if r.String("data-dir") != "/x" || r.Int("verbose") != 3 || r.String("output") != "json" || r.String("account") != "+15555550100" || r.Command != "send" {
		t.Errorf("globals %+v", r)
	}
	s := r.Sub
	if s.String("message") != "hi" || len(s.Strings("recipient")) != 2 || len(s.Strings("group-id")) != 2 {
		t.Errorf("send %+v", s)
	}
	r, err = p.Parse([]string{"receive", "-t", "-1", "--max-messages", "5"})
	if err != nil || r.Sub.Float("timeout") != -1 || r.Sub.Int("max-messages") != 5 {
		t.Errorf("receive: %+v %v", r, err)
	}
	r, _ = p.Parse([]string{"receive"})
	if r.Sub.Float("timeout") != 3.0 || r.Sub.Int("max-messages") != -1 {
		t.Errorf("defaults %+v", r.Sub)
	}
	// The last occurrence wins.
	r, _ = p.Parse([]string{"-a", "+1", "--account", "+2", "version"})
	if r.String("account") != "+2" {
		t.Errorf("last wins: %q", r.String("account"))
	}
	// Help short-circuits; later arguments are not validated.
	r, err = p.Parse([]string{"version", "-h", "--bogus"})
	if err != nil || !r.Help || r.HelpParser.Prog != "signal-cli version" {
		t.Errorf("help: %+v %v", r, err)
	}
	r, err = p.Parse([]string{"--version", "bogus"})
	if err != nil || !r.Version {
		t.Errorf("version: %v", err)
	}
}

func TestParseErrors(t *testing.T) {
	p := testParser()
	for _, tc := range []struct {
		args []string
		prog string
		msg  string
	}{
		{[]string{}, "signal-cli", "too few arguments"},
		{[]string{"--d", "version"}, "signal-cli", "ambiguous option: --d could match --data-dir, --dbus, --dbus-system, --disable-send-log"},
		{[]string{"-o", "xml", "version"}, "signal-cli", "argument -o/--output: could not convert 'xml' (choose from {plain-text,json})"},
		{[]string{"-a"}, "signal-cli", "argument -a/--account/-u/--username: expected one argument"},
		{[]string{"--dbus", "--dbus-system", "version"}, "signal-cli", "argument --dbus-system: not allowed with argument --dbus"},
		{[]string{"--", "version"}, "signal-cli", "unrecognized arguments: 'version'"},
		{[]string{"--bogus", "version"}, "signal-cli", "unrecognized arguments: '--bogus'"},
		{[]string{"link", "extra"}, "signal-cli link", "unrecognized arguments: 'extra'"},
		{[]string{"link", "-n"}, "signal-cli link", "argument -n/--name: expected one argument"},
		{[]string{"send", "-m"}, "signal-cli send", "argument -m/--message: expected one argument"},
		{[]string{"send", "-m", "hi", "--message-from-stdin", "x"}, "signal-cli send", "argument --message-from-stdin: not allowed with argument -m/--message"},
		{[]string{"send", "--quote-timestamp", "abc"}, "signal-cli send", "argument --quote-timestamp: could not convert 'abc' to integer (64 bits)"},
		{[]string{"receive", "-t", "abc"}, "signal-cli receive", "argument -t/--timeout: could not convert 'abc' to number (double precision)"},
		{[]string{"receive", "--max-messages", "1.5"}, "signal-cli receive", "argument --max-messages: could not convert '1.5' to integer (32 bits)"},
		{[]string{"-h", "-o", "xml"}, "", ""}, // help wins: no error
	} {
		r, err := p.Parse(tc.args)
		if tc.msg == "" {
			if err != nil || !r.Help {
				t.Errorf("%v: %v", tc.args, err)
			}
			continue
		}
		pe, ok := err.(*Error)
		if !ok || pe.Message != tc.msg || pe.Parser.Prog != tc.prog {
			t.Errorf("%v: got %v (%T), want %q from %q", tc.args, err, err, tc.msg, tc.prog)
		}
	}
	// Errors before -h win.
	if _, err := p.Parse([]string{"-o", "xml", "-h"}); err == nil {
		t.Error("-o xml -h succeeded")
	}
	// Unknown subcommand.
	_, err := p.Parse([]string{"bogus"})
	if pe, ok := err.(*Error); !ok || pe.Message != "invalid choice: 'bogus' (choose from 'link', 'receive', 'send', 'trust', 'version')" || !pe.SuggestCommands {
		t.Errorf("bogus: %v", err)
	}
}
