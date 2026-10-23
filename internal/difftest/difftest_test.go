package difftest

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNormalize(t *testing.T) {
	cases := []struct{ in, cfg, want string }{
		{"data in /x/cfg-1/data", "/x/cfg-1", "data in <CONFIG>/data"},
		{"log at /s/scratch-9/lf.txt", "", "log at <SCRATCH>/lf.txt"},
		{"at 2026-10-23T05:46:01.123Z ok", "", "at <TIMESTAMP> ok"},
		{"at 2026-10-23T05:46:01+02:00", "", "at <TIMESTAMP>"},
		{"version 0.14.7 unchanged", "", "version 0.14.7 unchanged"},
	}
	for _, c := range cases {
		if got := Normalize(c.in, c.cfg, "/s/scratch-9"); got != c.want {
			t.Errorf("Normalize(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestDiff(t *testing.T) {
	a := Result{Stdout: "x\ny\n", Stderr: "", ExitCode: 1}
	if d := Diff(a, a); d != "" {
		t.Errorf("equal results differ: %s", d)
	}
	for _, b := range []Result{
		{Stdout: "x\nz\n", ExitCode: 1},
		{Stdout: "x\ny\n", ExitCode: 2},
		{Stdout: "x\ny\nextra", ExitCode: 1},
		{Stdout: "x\ny\n", Stderr: "warn", ExitCode: 1},
	} {
		if Diff(a, b) == "" {
			t.Errorf("Diff(%+v, %+v) reported no difference", a, b)
		}
	}
}

func TestLoadScenariosRejectsBadNames(t *testing.T) {
	for _, body := range []string{
		`[{"name":"a"},{"name":"a"}]`,
		`[{"name":"../x"}]`,
		`[{"name":""}]`,
	} {
		p := filepath.Join(t.TempDir(), "s.json")
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadScenarios(p); err == nil {
			t.Errorf("accepted %s", body)
		}
	}
}

func TestGoldenRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "g.json")
	g := Golden{Scenario: Scenario{Name: "n", Args: []string{"--version"}}, Oracle: "signal-cli 0.14.7",
		Result: Result{Stdout: "signal-cli 0.14.7\n"}}
	if err := WriteGolden(p, g); err != nil {
		t.Fatal(err)
	}
	got, err := ReadGolden(p)
	if err != nil {
		t.Fatal(err)
	}
	if got.Result != g.Result || got.Scenario.Name != g.Scenario.Name {
		t.Errorf("round trip: got %+v", got)
	}
}

func TestRPCDiffIgnoresOrderWhereTheReferenceDoes(t *testing.T) {
	a := RPCResult{
		Status:   "HTTP/1.1 200 OK",
		Headers:  []string{"Content-type: application/json"},
		Body:     `[{"jsonrpc":"2.0","result":{"version":"0.14.7"},"id":1},{"jsonrpc":"2.0","error":{"code":-32601,"message":"Method not implemented","data":null},"id":5}]`,
		Messages: []string{`{"jsonrpc":"2.0","result":4,"id":6}`, `{"jsonrpc":"2.0","result":[],"id":16}`},
	}
	same := RPCResult{
		Status:  a.Status,
		Headers: a.Headers,
		// batch elements reordered, keys reordered
		Body:     `[{"id":5,"jsonrpc":"2.0","error":{"data":null,"message":"Method not implemented","code":-32601}},{"jsonrpc":"2.0","result":{"version":"0.14.7"},"id":1}]`,
		Messages: []string{`{"jsonrpc":"2.0","result":[],"id":16}`, `{"id":6,"jsonrpc":"2.0","result":4}`},
	}
	if d := RPCDiff(a, same); d != "" {
		t.Errorf("reordering reported as a difference:\n%s", d)
	}

	for name, b := range map[string]RPCResult{
		"status":         {Status: "HTTP/1.1 201 Created", Headers: a.Headers, Body: a.Body, Messages: a.Messages},
		"header casing":  {Status: a.Status, Headers: []string{"Content-Type: application/json"}, Body: a.Body, Messages: a.Messages},
		"batch element":  {Status: a.Status, Headers: a.Headers, Body: `[{"jsonrpc":"2.0","result":{"version":"0.14.7"},"id":1}]`, Messages: a.Messages},
		"nested order":   {Status: a.Status, Headers: a.Headers, Body: a.Body, Messages: []string{`{"jsonrpc":"2.0","result":4,"id":6}`, `{"jsonrpc":"2.0","result":[1],"id":16}`}},
		"extra message":  {Status: a.Status, Headers: a.Headers, Body: a.Body, Messages: append([]string{`{}`}, a.Messages...)},
		"number vs text": {Status: a.Status, Headers: a.Headers, Body: a.Body, Messages: []string{`{"jsonrpc":"2.0","result":"4","id":6}`, a.Messages[1]}},
	} {
		if RPCDiff(a, b) == "" {
			t.Errorf("%s: difference not reported", name)
		}
	}
}

func TestDechunkPartial(t *testing.T) {
	in := []byte("6\r\n:\nabc\n\r\n4\r\ndata\r\n3\r\nxy")
	if got := string(dechunkPartial(in)); got != ":\nabc\ndataxy" {
		t.Errorf("dechunkPartial = %q", got)
	}
}
