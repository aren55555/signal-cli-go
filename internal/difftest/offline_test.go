// SPDX-License-Identifier: GPL-3.0-or-later

package difftest

import (
	"context"
	"strings"
	"testing"
	"time"
)

// TestReplayIsOffline: replays of signal-cli-go run with every proxy
// variable pointed at a dead address and none of the caller's, so a
// scenario that tries the network fails at once instead of reaching
// Signal (milestone 12 P1). A networked command proves it end to end, in
// both service environments.
func TestReplayIsOffline(t *testing.T) {
	t.Setenv("HTTPS_PROXY", "http://real-proxy.example:3128")
	env := strings.Join(scenarioEnv(t.TempDir()), "\n")
	for _, k := range []string{"HTTPS_PROXY", "https_proxy", "ALL_PROXY", "all_proxy", "HTTP_PROXY", "http_proxy"} {
		if !strings.Contains(env, "\n"+k+"="+DeadProxy) && !strings.HasPrefix(env, k+"="+DeadProxy) {
			t.Errorf("%s is not the dead proxy", k)
		}
	}
	if strings.Contains(env, "real-proxy") {
		t.Error("the caller's proxy leaked into a replay")
	}
	bin := buildSignalCLIGo(t)
	for _, envFlag := range [][]string{{"--service-environment", "staging"}, {"--service-environment", "live"}, nil} {
		start := time.Now()
		args := append(append([]string{}, envFlag...), "-c", ConfigPlaceholder, "link", "-n", "offline-check")
		res, err := Run(context.Background(), bin, Scenario{Name: "offline", Args: args})
		if err != nil {
			t.Fatal(err)
		}
		if res.ExitCode == 0 || time.Since(start) > 20*time.Second {
			t.Errorf("%v: link exited %d after %v; want a quick failure", envFlag, res.ExitCode, time.Since(start))
		}
		out := res.Stdout + res.Stderr
		if strings.Contains(out, "sgnl://linkdevice") {
			t.Errorf("%v: link reached a provisioning server:\n%s", envFlag, out)
		}
	}
}
