package difftest

import (
	"context"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Recording runs the reference signal-cli and needs the sandbox:
//
//	just record-golden
var record = flag.Bool("record", false, "re-record golden transcripts from the reference signal-cli")

const (
	repoRoot     = "../.."
	cliScenarios = repoRoot + "/testdata/golden/cli/scenarios.json"
	cliGoldenDir = repoRoot + "/testdata/golden/cli"
	cliPending   = repoRoot + "/testdata/golden/cli/PENDING"
	oracleBin    = repoRoot + "/third_party/signal-cli-0.14.7/bin/signal-cli"
)

func TestCLIGolden(t *testing.T) {
	scs, err := LoadScenarios(cliScenarios)
	if err != nil {
		t.Fatal(err)
	}
	if *record {
		recordAll(t, scs)
		return
	}

	pending, err := LoadPending(cliPending)
	if err != nil {
		t.Fatal(err)
	}
	for name := range pending {
		if !hasScenario(scs, name) {
			t.Errorf("PENDING lists unknown scenario %q", name)
		}
	}

	bin := buildSignalCLIGo(t)
	for _, sc := range scs {
		t.Run(sc.Name, func(t *testing.T) {
			g, err := ReadGolden(GoldenPath(cliGoldenDir, sc))
			if err != nil {
				t.Fatalf("no golden transcript (run `just record-golden`): %v", err)
			}
			got, err := Run(context.Background(), bin, sc)
			if err != nil {
				t.Fatal(err)
			}
			d := Diff(g.Result, got)
			switch {
			case d != "" && pending[sc.Name]:
				t.Skipf("pending (listed in testdata/golden/cli/PENDING):\n%s", d)
			case d != "":
				t.Errorf("differs from signal-cli 0.14.7:\n%s", d)
			case pending[sc.Name]:
				t.Errorf("matches the reference now; remove it from testdata/golden/cli/PENDING")
			}
		})
	}
}

func hasScenario(scs []Scenario, name string) bool {
	for _, sc := range scs {
		if sc.Name == name {
			return true
		}
	}
	return false
}

func recordAll(t *testing.T, scs []Scenario) {
	if os.Getenv("SIGNAL_CLI_GO_SANDBOX") != "1" {
		t.Fatal("recording runs the reference binary and must happen inside the sandbox")
	}
	bin, err := filepath.Abs(oracleBin)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(bin); err != nil {
		t.Fatalf("reference signal-cli missing (run `just install-oracle`): %v", err)
	}
	ver, _ := exec.Command(bin, "--version").Output()
	for _, sc := range scs {
		res, err := Run(context.Background(), bin, sc)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(res.Stderr, "Missing required native library") {
			t.Fatalf("%s: the oracle could not load libsignal (sandbox problem, not real behavior):\n%s", sc.Name, res.Stderr)
		}
		g := Golden{Scenario: sc, Oracle: string(trimNL(ver)), Result: res}
		if err := WriteGolden(GoldenPath(cliGoldenDir, sc), g); err != nil {
			t.Fatal(err)
		}
		t.Logf("recorded %s (exit %d)", sc.Name, res.ExitCode)
	}
}

func trimNL(b []byte) []byte {
	for len(b) > 0 && (b[len(b)-1] == '\n' || b[len(b)-1] == '\r') {
		b = b[:len(b)-1]
	}
	return b
}

// buildSignalCLIGo compiles the binary under test. The output is named
// signal-cli so usage lines that echo argv[0] can match.
func buildSignalCLIGo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if d := os.Getenv("GOTMPDIR"); d != "" {
		var err error
		if dir, err = os.MkdirTemp(d, "difftest-bin-"); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { os.RemoveAll(dir) })
	}
	bin := filepath.Join(dir, "signal-cli")
	cmd := exec.Command("go", "build", "-o", bin, "../../cmd/signal-cli-go")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	return bin
}
