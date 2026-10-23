package difftest

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const (
	rpcDir       = repoRoot + "/testdata/golden/rpc"
	rpcScenarios = rpcDir + "/scenarios.json"
	rpcGoldenDir = rpcDir + "/recorded"
	rpcPending   = rpcDir + "/PENDING"
)

func loadRPCScenarios(t *testing.T) []RPCScenario {
	t.Helper()
	data, err := os.ReadFile(rpcScenarios)
	if err != nil {
		t.Fatal(err)
	}
	var scs []RPCScenario
	if err := json.Unmarshal(data, &scs); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, sc := range scs {
		if sc.Name == "" || strings.ContainsAny(sc.Name, `/\ `) || seen[sc.Name] {
			t.Fatalf("bad or duplicate scenario name %q", sc.Name)
		}
		seen[sc.Name] = true
	}
	return scs
}

// TestRPCGolden replays JSON-RPC exchanges against one signal-cli-go
// daemon. Scenarios run one at a time in file order: subscription ids
// come from a counter shared by all connections, so order is part of the
// recorded behavior. For the same reason recording always covers the
// whole file.
func TestRPCGolden(t *testing.T) {
	scs := loadRPCScenarios(t)
	if *record {
		recordRPC(t, scs)
		return
	}
	pending, err := LoadPending(rpcPending)
	if err != nil {
		t.Fatal(err)
	}
	for name := range pending {
		found := false
		for _, sc := range scs {
			found = found || sc.Name == name
		}
		if !found {
			t.Errorf("PENDING lists unknown scenario %q", name)
		}
	}
	files, _ := filepath.Glob(rpcGoldenDir + "/*.json")
	for _, f := range files {
		name := strings.TrimSuffix(filepath.Base(f), ".json")
		if !seen(scs, name) {
			t.Errorf("stale golden %s", f)
		}
	}

	d, err := StartDaemon(buildSignalCLIGo(t))
	if err != nil {
		if len(pending) == len(scs) {
			t.Skipf("signal-cli-go daemon does not start yet; all %d scenarios are pending: %v", len(scs), firstLine(err.Error()))
		}
		t.Fatal(err)
	}
	defer d.Stop()

	for _, sc := range scs {
		t.Run(sc.Name, func(t *testing.T) {
			var g RPCGolden
			data, err := os.ReadFile(filepath.Join(rpcGoldenDir, sc.Name+".json"))
			if err == nil {
				err = json.Unmarshal(data, &g)
			}
			if err != nil {
				t.Fatalf("no golden transcript (run `just record-golden`): %v", err)
			}
			got, err := d.Exchange(context.Background(), sc)
			if err != nil {
				t.Fatal(err)
			}
			diff := RPCDiff(g.Result, got)
			switch {
			case diff != "" && pending[sc.Name]:
				t.Skipf("pending (listed in testdata/golden/rpc/PENDING):\n%s", diff)
			case diff != "":
				t.Errorf("differs from signal-cli 0.14.7:\n%s", diff)
			case pending[sc.Name]:
				t.Errorf("matches the reference now; remove it from testdata/golden/rpc/PENDING")
			}
		})
	}
}

func seen(scs []RPCScenario, name string) bool {
	for _, sc := range scs {
		if sc.Name == name {
			return true
		}
	}
	return false
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return line
}

func recordRPC(t *testing.T, scs []RPCScenario) {
	if os.Getenv("SIGNAL_CLI_GO_SANDBOX") != "1" {
		t.Fatal("recording runs the reference binary and must happen inside the sandbox")
	}
	if *recordOnly != "" {
		t.Log("-record-only is ignored for JSON-RPC: scenarios share one daemon and are recorded together")
	}
	bin, err := filepath.Abs(oracleBin)
	if err != nil {
		t.Fatal(err)
	}
	ver, _ := exec.Command(bin, "--version").Output()
	d, err := StartDaemon(bin)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Stop()
	if strings.Contains(d.stderr.String(), "Missing required native library") {
		t.Fatalf("the oracle could not load libsignal (sandbox problem):\n%s", d.stderr.String())
	}
	if err := os.MkdirAll(rpcGoldenDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, sc := range scs {
		res, err := d.Exchange(context.Background(), sc)
		if err != nil {
			t.Fatalf("%s: %v", sc.Name, err)
		}
		g := RPCGolden{Scenario: sc, Oracle: string(trimNL(ver)), Result: res}
		data, err := json.MarshalIndent(g, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := writeFileAtomic(filepath.Join(rpcGoldenDir, sc.Name+".json"), append(data, '\n')); err != nil {
			t.Fatal(err)
		}
		t.Logf("recorded %s", sc.Name)
	}
}
