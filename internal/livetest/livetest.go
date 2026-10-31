// SPDX-License-Identifier: GPL-3.0-or-later

// Package livetest is the safety guard for anything that can reach Signal's
// production servers (milestone 12, PLAN.md). A live test or driver runs
// only when SIGNAL_CLI_GO_LIVE=1, takes its phone numbers only from
// /tmp/live-numbers (tmpfs, one per line, written by the operator) and
// refuses every other number, and paces itself through a Pacer: far below
// the staging rate, exponential backoff on 413/428/429, a hard stop after
// repeated limits, and a cap on messages per run.
//
// The operator's real account is never in that file and must never be
// touched by a test; the guard cannot know that number, so the file is the
// whole allowlist.
package livetest

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	// EnvVar must be "1" for anything live to run.
	EnvVar = "SIGNAL_CLI_GO_LIVE"
	// NumbersFile is the only source of live numbers.
	NumbersFile = "/tmp/live-numbers"
)

var e164 = regexp.MustCompile(`^\+[1-9][0-9]{7,14}$`)

// ErrNotEnabled: SIGNAL_CLI_GO_LIVE is not "1".
var ErrNotEnabled = errors.New("livetest: " + EnvVar + "=1 is not set; live runs are disabled")

// Guard is an opened live session: the allowed numbers and the pacer.
type Guard struct {
	numbers []string
	allowed map[string]bool
	Pacer   *Pacer
}

// Open checks the switch and reads the numbers file. Drivers call it
// before doing anything; tests use Require.
func Open() (*Guard, error) { return open(os.Getenv, NumbersFile) }

func open(getenv func(string) string, path string) (*Guard, error) {
	if getenv(EnvVar) != "1" {
		return nil, ErrNotEnabled
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("livetest: %w (the operator writes the live numbers there)", err)
	}
	defer f.Close()
	g := &Guard{allowed: map[string]bool{}, Pacer: NewPacer(DefaultPacing)}
	sc := bufio.NewScanner(f)
	for line := 1; sc.Scan(); line++ {
		n := strings.TrimSpace(sc.Text())
		if n == "" || strings.HasPrefix(n, "#") {
			continue
		}
		if !e164.MatchString(n) {
			return nil, fmt.Errorf("livetest: %s line %d is not an E.164 number", path, line)
		}
		if !g.allowed[n] {
			g.allowed[n] = true
			g.numbers = append(g.numbers, n)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(g.numbers) == 0 {
		return nil, fmt.Errorf("livetest: %s lists no numbers", path)
	}
	return g, nil
}

// Require is Open for tests: it skips the test when live runs are off and
// fails it when the numbers file is unusable.
func Require(t testing.TB) *Guard {
	t.Helper()
	g, err := Open()
	if errors.Is(err, ErrNotEnabled) {
		t.Skip(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	return g
}

// Numbers are the allowed live numbers, in file order.
func (g *Guard) Numbers() []string { return append([]string(nil), g.numbers...) }

// Check refuses any number that is not in the numbers file.
func (g *Guard) Check(numbers ...string) error {
	for _, n := range numbers {
		if !g.allowed[n] {
			return fmt.Errorf("livetest: %s is not in %s; refusing it", mask(n), NumbersFile)
		}
	}
	return nil
}

// CheckArgs refuses a command line that names a phone number outside the
// numbers file anywhere in its arguments.
func (g *Guard) CheckArgs(args []string) error {
	for _, a := range args {
		for _, m := range anyNumber.FindAllString(a, -1) {
			if err := g.Check(m); err != nil {
				return err
			}
		}
	}
	return nil
}

var anyNumber = regexp.MustCompile(`\+[1-9][0-9]{7,14}`)

// Mask hides all but the last two digits of every phone number in s, for
// logs and error messages.
func Mask(s string) string { return anyNumber.ReplaceAllStringFunc(s, mask) }

func mask(n string) string {
	if len(n) < 4 {
		return "+***"
	}
	return "+" + strings.Repeat("*", len(n)-3) + n[len(n)-2:]
}

// Pacing are the limits of one live run.
type Pacing struct {
	MinGap      time.Duration   // between two requests that send
	MaxMessages int             // per run
	Backoff     []time.Duration // waits after successive rate limits
}

// DefaultPacing is well below what the staging runs used (5 s gaps, 40
// sends an hour): one send every 30 s, at most 25 per run, and four
// back-offs before giving up.
var DefaultPacing = Pacing{
	MinGap:      30 * time.Second,
	MaxMessages: 25,
	Backoff:     []time.Duration{2 * time.Minute, 4 * time.Minute, 8 * time.Minute, 16 * time.Minute},
}

// Errors of the pacer.
var (
	ErrMessageCap = errors.New("livetest: message cap for this run reached")
	ErrHardStop   = errors.New("livetest: rate limited repeatedly; stopping the run")
)

// Pacer spaces live requests and handles rate limits.
type Pacer struct {
	p       Pacing
	mu      sync.Mutex
	last    time.Time
	sent    int
	limited int
	now     func() time.Time
	sleep   func(time.Duration)
}

// NewPacer returns a pacer with the given limits.
func NewPacer(p Pacing) *Pacer { return &Pacer{p: p, now: time.Now, sleep: time.Sleep} }

// BeforeSend waits out the minimum gap and counts the message; it fails
// once the cap is reached.
func (p *Pacer) BeforeSend() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.sent >= p.p.MaxMessages {
		return ErrMessageCap
	}
	if !p.last.IsZero() {
		if wait := p.p.MinGap - p.now().Sub(p.last); wait > 0 {
			p.sleep(wait)
		}
	}
	p.last = p.now()
	p.sent++
	return nil
}

// RateLimited reports whether a command's output shows a rate limit or a
// challenge: HTTP 413, 428 or 429.
func RateLimited(output string) bool { return limitRe.MatchString(output) }

var limitRe = regexp.MustCompile(`(?i)\b(413|428|429)\b|rate.?limit|proof.?required|captcha`)

// OnRateLimit waits the next back-off (or Retry-After, if longer), and
// returns ErrHardStop once the back-offs are used up.
func (p *Pacer) OnRateLimit(retryAfter time.Duration) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.limited >= len(p.p.Backoff) {
		return ErrHardStop
	}
	wait := p.p.Backoff[p.limited]
	if retryAfter > wait {
		wait = retryAfter
	}
	p.limited++
	p.sleep(wait)
	return nil
}
