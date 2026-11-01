// SPDX-License-Identifier: GPL-3.0-or-later

// Command live is the driver for live production runs (milestone 12, P7-P9).
// It runs one signal-cli-go or reference signal-cli command against
// production, behind the internal/livetest guard:
//
//	SIGNAL_CLI_GO_LIVE=1 go run ./tools/live ours|ref DATADIR TAG -- ARGS...
//
// ARGS may name the live numbers only as @N1, @N2, @N3 (lines 1-3 of
// /tmp/live-numbers); any other phone number is refused. DATADIR must be
// under .data/live/. The command always gets --service-environment live;
// any other environment is refused. Sends are paced across invocations
// (state in .data/live/pacer.json), rate limits are backed off and end the
// run when they repeat, and every number in the printed output is masked.
// The raw output goes to .data/live/cmd/TAG.{out,err} (gitignored).
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/aren55555/signal-cli-go/internal/livetest"
)

const root = ".data/live"

// sending subcommands count against the pacer and the message cap.
var sending = map[string]bool{
	"send": true, "sendReaction": true, "sendTyping": true, "sendReceipt": true, "sendStory": true,
	"updateGroup": true, "joinGroup": true, "quitGroup": true, "register": true, "verify": true,
	"remoteDelete": true, "sendSyncRequest": true, "updateProfile": true, "unregister": true,
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "live:", livetest.Mask(err.Error()))
		if errors.Is(err, livetest.ErrHardStop) {
			os.Exit(4)
		}
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) < 4 || args[3] != "--" {
		return errors.New("usage: live ours|ref DATADIR TAG -- ARGS...")
	}
	client, dir, tag, cmdArgs := args[0], args[1], args[2], args[4:]
	g, err := livetest.Open()
	if err != nil {
		return err
	}
	nums := g.Numbers()
	if len(nums) < 3 {
		return fmt.Errorf("%s needs three numbers (N1, N2, N3)", livetest.NumbersFile)
	}
	abs, _ := filepath.Abs(dir)
	rootAbs, _ := filepath.Abs(root)
	if !strings.HasPrefix(abs+"/", rootAbs+"/") {
		return fmt.Errorf("data directory %s is not under %s", dir, root)
	}
	expanded := make([]string, len(cmdArgs))
	sub := ""
	for i, a := range cmdArgs {
		a = strings.NewReplacer("@N1", nums[0], "@N2", nums[1], "@N3", nums[2]).Replace(a)
		if strings.HasPrefix(a, "--service-environment") || a == "live" && i > 0 && cmdArgs[i-1] == "--service-environment" {
			return errors.New("the environment is set by the driver (live); remove --service-environment")
		}
		// Global flags only: after the subcommand, -d is e.g. listGroups --detailed.
		if sub == "" && (a == "-d" || a == "--data-dir" || a == "-c" || a == "--config") {
			return errors.New("the data directory is set by the driver; remove " + a)
		}
		expanded[i] = a
		if sub == "" && !strings.HasPrefix(a, "-") && (i == 0 || !takesValue(cmdArgs[i-1])) {
			sub = a
		}
	}
	if err := g.CheckArgs(expanded); err != nil {
		return err
	}
	full := append([]string{"--service-environment", "live", "--config", dir}, expanded...)

	state, err := loadPacer()
	if err != nil {
		return err
	}
	if sending[sub] {
		if state.Sent >= livetest.DefaultPacing.MaxMessages {
			return livetest.ErrMessageCap
		}
		if wait := livetest.DefaultPacing.MinGap - time.Since(state.Last); wait > 0 {
			time.Sleep(wait)
		}
	}
	for attempt := 0; ; attempt++ {
		out, errOut, code, err := execute(client, full, tag)
		if err != nil {
			return err
		}
		if sending[sub] {
			state.Last, state.Sent = time.Now(), state.Sent+1
		}
		if code != 0 && livetest.RateLimited(string(out)+string(errOut)) {
			if attempt >= len(livetest.DefaultPacing.Backoff) {
				state.save()
				return livetest.ErrHardStop
			}
			wait := livetest.DefaultPacing.Backoff[attempt]
			fmt.Fprintf(os.Stderr, "live: rate limited (%s), waiting %v\n", tag, wait)
			state.save()
			time.Sleep(wait)
			continue
		}
		state.save()
		if code != 0 {
			os.Exit(code)
		}
		return nil
	}
}

func takesValue(flag string) bool {
	switch flag {
	case "-a", "--account", "-o", "--output", "--log-file", "--trust-new-identities":
		return true
	}
	return false
}

// execute runs one client: our binary, or the reference through `just
// oracle` (the only sanctioned way to run it). Output streams line by
// line, raw to .data/live/cmd/TAG.{out,err} and masked to the terminal,
// so long-running commands (link, daemon) can be followed while they run.
func execute(client string, args []string, tag string) ([]byte, []byte, int, error) {
	var cmd *exec.Cmd
	switch client {
	case "ours":
		cmd = exec.Command(filepath.Join(os.Getenv("HOME"), ".cache", "sgo"), args...)
	case "ref":
		var q []string
		for _, a := range args {
			q = append(q, "'"+strings.ReplaceAll(a, "'", `'\''`)+"'")
		}
		cmd = exec.Command("just", "oracle", strings.Join(q, " "))
		cmd.Env = append(os.Environ(), "LANG=C.UTF-8")
	default:
		return nil, nil, 0, errors.New("client must be ours or ref")
	}
	d := filepath.Join(root, "cmd")
	if err := os.MkdirAll(d, 0o700); err != nil {
		return nil, nil, 0, err
	}
	var out, errOut bytes.Buffer
	streams := []struct {
		buf  *bytes.Buffer
		ext  string
		term io.Writer
		dst  *io.Writer
	}{{&out, ".out", os.Stdout, &cmd.Stdout}, {&errOut, ".err", os.Stderr, &cmd.Stderr}}
	for _, s := range streams {
		f, err := os.OpenFile(filepath.Join(d, tag+s.ext), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
		if err != nil {
			return nil, nil, 0, err
		}
		defer f.Close()
		w := &lines{w: s.term}
		defer w.flush()
		*s.dst = io.MultiWriter(s.buf, f, w)
	}
	err := cmd.Run()
	code := 0
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		code = ee.ExitCode()
	} else if err != nil {
		return nil, nil, 0, err
	}
	return out.Bytes(), errOut.Bytes(), code, nil
}

// lines writes complete lines to w with every phone number masked, and
// turns the `just oracle` echo of the command into a comment.
type lines struct {
	w   io.Writer
	buf []byte
}

func (l *lines) Write(p []byte) (int, error) {
	l.buf = append(l.buf, p...)
	for {
		i := bytes.IndexByte(l.buf, '\n')
		if i < 0 {
			return len(p), nil
		}
		l.emit(l.buf[:i+1])
		l.buf = l.buf[i+1:]
	}
}

func (l *lines) emit(b []byte) {
	s := string(b)
	if strings.HasPrefix(s, "JAVA_OPTS=") {
		s = "# " + s
	}
	io.WriteString(l.w, livetest.Mask(s))
}

func (l *lines) flush() {
	if len(l.buf) > 0 {
		l.emit(l.buf)
		l.buf = nil
	}
}

// pacer is the pacing state shared by the invocations of one run.
type pacer struct {
	Last time.Time `json:"last"`
	Sent int       `json:"sent"`
}

func loadPacer() (*pacer, error) {
	p := &pacer{}
	b, err := os.ReadFile(filepath.Join(root, "pacer.json"))
	if errors.Is(err, os.ErrNotExist) {
		return p, nil
	}
	if err != nil {
		return nil, err
	}
	return p, json.Unmarshal(b, p)
}

func (p *pacer) save() {
	b, _ := json.Marshal(p)
	os.MkdirAll(root, 0o700)
	os.WriteFile(filepath.Join(root, "pacer.json"), b, 0o600)
}
