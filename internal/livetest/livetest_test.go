// SPDX-License-Identifier: GPL-3.0-or-later

package livetest

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

func env(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestOpenNeedsTheSwitchAndTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "live-numbers")
	os.WriteFile(path, []byte("# test accounts\n+12025550101\n\n+12025550102\n+12025550101\n"), 0o600)
	if _, err := open(env(nil), path); !errors.Is(err, ErrNotEnabled) {
		t.Errorf("without %s: %v", EnvVar, err)
	}
	if _, err := open(env(map[string]string{EnvVar: "true"}), path); !errors.Is(err, ErrNotEnabled) {
		t.Errorf("%s=true must not count: %v", EnvVar, err)
	}
	on := env(map[string]string{EnvVar: "1"})
	if _, err := open(on, path+".missing"); err == nil {
		t.Error("a missing numbers file was accepted")
	}
	g, err := open(on, path)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(g.Numbers(), ","); got != "+12025550101,+12025550102" {
		t.Errorf("numbers %s", got)
	}
	if g.Check("+12025550102") != nil || g.Check("+12025550103") == nil {
		t.Error("Check must accept listed numbers only")
	}
	if g.CheckArgs([]string{"-a", "+12025550101", "send", "-m", "hi", "+12025550102"}) != nil {
		t.Error("listed numbers refused")
	}
	if err := g.CheckArgs([]string{"-a", "+12025550101", "send", "+12025550199"}); err == nil || strings.Contains(err.Error(), "5550199") {
		t.Errorf("an unlisted recipient must be refused with the number masked: %v", err)
	}
	os.WriteFile(path, []byte("12025550101\n"), 0o600)
	if _, err := open(on, path); err == nil {
		t.Error("a non-E.164 line was accepted")
	}
	os.WriteFile(path, []byte("# nothing\n"), 0o600)
	if _, err := open(on, path); err == nil {
		t.Error("an empty numbers file was accepted")
	}
}

func TestMask(t *testing.T) {
	if got := Mask("to +12025550101 and +12025550102"); got != "to +*********01 and +*********02" {
		t.Errorf("Mask = %q", got)
	}
}

func fakePacer(p Pacing) (*Pacer, *time.Time, *[]time.Duration) {
	now := time.Unix(0, 0)
	var slept []time.Duration
	pc := NewPacer(p)
	pc.now = func() time.Time { return now }
	pc.sleep = func(d time.Duration) { slept = append(slept, d); now = now.Add(d) }
	return pc, &now, &slept
}

func TestPacerGapAndCap(t *testing.T) {
	pc, now, slept := fakePacer(Pacing{MinGap: 30 * time.Second, MaxMessages: 3})
	for i := 0; i < 3; i++ {
		if err := pc.BeforeSend(); err != nil {
			t.Fatal(err)
		}
		*now = now.Add(10 * time.Second)
	}
	if len(*slept) != 2 || (*slept)[0] != 20*time.Second {
		t.Errorf("waits %v, want two of 20s", *slept)
	}
	if err := pc.BeforeSend(); !errors.Is(err, ErrMessageCap) {
		t.Errorf("fourth send: %v", err)
	}
}

func TestPacerBackoffAndHardStop(t *testing.T) {
	pc, _, slept := fakePacer(Pacing{Backoff: []time.Duration{time.Minute, 2 * time.Minute}})
	if pc.OnRateLimit(0) != nil || pc.OnRateLimit(5*time.Minute) != nil {
		t.Fatal("back-offs refused")
	}
	if got := *slept; len(got) != 2 || got[0] != time.Minute || got[1] != 5*time.Minute {
		t.Errorf("waits %v (Retry-After wins when longer)", got)
	}
	if err := pc.OnRateLimit(0); !errors.Is(err, ErrHardStop) {
		t.Errorf("third limit: %v", err)
	}
	for _, s := range []string{"[429] Rate limit exceeded", "status 413", "428 Precondition Required", "proof required"} {
		if !RateLimited(s) {
			t.Errorf("%q not recognised as a limit", s)
		}
	}
	if RateLimited("sent 1793300000000") {
		t.Error("a timestamp counted as a limit")
	}
	if DefaultPacing.MinGap <= 5*time.Second || DefaultPacing.MaxMessages > 40 {
		t.Error("live pacing must stay well below the staging runs (5 s gaps, 40 sends an hour)")
	}
}

// TestStagingDriverCannotPickLive: the staging interop driver passes
// --service-environment staging on every command of both clients and never
// names the live environment, so it cannot reach production whatever the
// defaults are.
func TestStagingDriverCannotPickLive(t *testing.T) {
	b, err := os.ReadFile("../../tools/interop/run.sh")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	raw := s[strings.Index(s, "\nraw() {"):]
	raw = raw[:strings.Index(raw, "\n}\n")]
	ours := regexp.MustCompile(`"\$OURS" -v --service-environment staging `)
	ref := regexp.MustCompile(`just oracle "\$\(quote --service-environment staging `)
	if !ours.MatchString(raw) || !ref.MatchString(raw) {
		t.Errorf("raw() must pass --service-environment staging to both clients:\n%s", raw)
	}
	if regexp.MustCompile(`service-environment[ =]+["']?live|SIGNAL_CLI_GO_LIVE`).MatchString(s) {
		t.Error("the staging driver names the live environment")
	}
	// Every other way the driver starts a client goes through raw() or
	// passes staging itself (the link helpers).
	for _, line := range strings.Split(s, "\n") {
		if l := strings.TrimSpace(line); strings.HasPrefix(l, "#") || strings.Contains(l, "go build") {
			continue // comments and building the binary start no client
		}
		if strings.Contains(line, `"$OURS"`) || strings.Contains(line, "just oracle") {
			if !strings.Contains(line, "--service-environment staging") && !strings.Contains(line, "quote --service-environment staging") {
				t.Errorf("client started without staging: %s", strings.TrimSpace(line))
			}
		}
	}
}
