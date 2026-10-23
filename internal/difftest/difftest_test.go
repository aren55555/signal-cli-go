package difftest

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNormalize(t *testing.T) {
	cases := []struct{ in, cfg, want string }{
		{"data in /x/cfg-1/data", "/x/cfg-1", "data in <CONFIG>/data"},
		{"at 2026-10-23T05:46:01.123Z ok", "", "at <TIMESTAMP> ok"},
		{"at 2026-10-23T05:46:01+02:00", "", "at <TIMESTAMP>"},
		{"version 0.14.7 unchanged", "", "version 0.14.7 unchanged"},
	}
	for _, c := range cases {
		if got := Normalize(c.in, c.cfg); got != c.want {
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
