// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Logging (spec/surface/cli/logging.md). Commands write their log lines
// to stderr in the simple form "LEVEL Name - message"; logWriter turns
// each such line into a log event and renders it per the verbosity, the
// log file and --scrub-log. Other stderr output (command errors, parser
// messages) passes through untouched.

// logSettings are the early pass's results merged with the config file
// (logging.md §1).
type logSettings struct {
	verbose int
	logFile string
	scrub   bool
}

// earlyLogging reads -v, --log-file and --scrub-log anywhere on the line
// (index.md §8). A failing pass (a --log-file without a value) leaves
// the defaults.
func earlyLogging(args []string, d globalDefaults) logSettings {
	s := logSettings{verbose: d.verbose, logFile: d.logFile, scrub: d.scrubLog}
	out := s
	for i := 0; i < len(args); i++ {
		tok := args[i]
		if tok == "--" {
			break
		}
		if !strings.HasPrefix(tok, "-") || strings.Trim(tok, "-") == "" || negativeInteger(tok) {
			continue
		}
		part, value, attached := strings.Cut(tok, "=")
		switch {
		case part == "-v" || (strings.HasPrefix(part, "--v") && strings.HasPrefix("--verbose", part)):
			out.verbose++
		case strings.HasPrefix(part, "--l") && strings.HasPrefix("--log-file", part):
			if !attached {
				if i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") {
					return s
				}
				i++
				value = args[i]
			}
			out.logFile = value
		case strings.HasPrefix(part, "--sc") && strings.HasPrefix("--scrub-log", part):
			out.scrub = true
		case !strings.HasPrefix(tok, "--"):
			for _, c := range tok[1:] {
				if c != 'v' {
					break
				}
				out.verbose++
			}
		}
	}
	return out
}

// loggerNames maps the short names our log lines use to the reference's
// full logger names (logging.md §4.1).
var loggerNames = map[string]string{
	"ReceiveHelper":                  "org.asamk.signal.manager.helper.ReceiveHelper",
	"ProvisioningManagerImpl":        "org.asamk.signal.manager.internal.ProvisioningManagerImpl",
	"SyncStorageJob":                 "org.asamk.signal.manager.jobs.SyncStorageJob",
	"SyncHelper":                     "org.asamk.signal.manager.helper.SyncHelper",
	"StorageHelper":                  "org.asamk.signal.manager.helper.StorageHelper",
	"MultiAccountManager":            "org.asamk.signal.manager.MultiAccountManager",
	"HttpServerHandler":              "org.asamk.signal.http.HttpServerHandler",
	"DaemonCommand":                  "org.asamk.signal.commands.DaemonCommand",
	"AddStickerPackCommand":          "org.asamk.signal.commands.AddStickerPackCommand",
	"StickerHelper":                  "org.asamk.signal.manager.helper.StickerHelper",
	"SendHelper":                     "org.asamk.signal.manager.helper.SendHelper",
	"ProfileHelper":                  "org.asamk.signal.manager.helper.ProfileHelper",
	"PinHelper":                      "org.asamk.signal.manager.helper.PinHelper",
	"ManagerImpl":                    "org.asamk.signal.manager.internal.ManagerImpl",
	"GroupHelper":                    "org.asamk.signal.manager.helper.GroupHelper",
	"BlockCommand":                   "org.asamk.signal.commands.BlockCommand",
	"AttachmentHelper":               "org.asamk.signal.manager.helper.AttachmentHelper",
	"AccountHelper":                  "org.asamk.signal.manager.helper.AccountHelper",
	"Shutdown":                       "org.asamk.signal.Shutdown",
	"RegistrationManagerImpl":        "org.asamk.signal.manager.internal.RegistrationManagerImpl",
	"AddDeviceCommand":               "org.asamk.signal.commands.AddDeviceCommand",
	"IOUtils":                        "org.asamk.signal.util.IOUtils",
	"App":                            "org.asamk.signal.App",
	"SignalJsonRpcDispatcherHandler": "org.asamk.signal.jsonrpc.SignalJsonRpcDispatcherHandler",
}

var levelRank = map[string]int{"TRACE": 0, "DEBUG": 1, "INFO": 2, "WARN": 3, "ERROR": 4}

// abbreviateLogger shortens a logger name to 36 characters (§3).
func abbreviateLogger(name string) string {
	const target = 36
	last := strings.LastIndexByte(name, '.')
	if len(name) < target || last < 0 {
		return name
	}
	tail, prefix := name[last:], name[:last]
	trim := len(prefix) - max(0, target-len(tail))
	segs := strings.Split(prefix, ".")
	var b strings.Builder
	removed := 0
	for i, seg := range segs {
		if i > 0 {
			if removed >= trim {
				b.WriteString("." + strings.Join(segs[i:], ".") + tail)
				return b.String()
			}
			b.WriteByte('.')
		}
		if seg != "" {
			b.WriteByte(seg[0])
			removed += len(seg) - 1
		}
	}
	b.WriteString(tail)
	return b.String()
}

// logger renders log events.
type logger struct {
	s       logSettings
	console io.Writer
	file    io.Writer
	mu      sync.Mutex
	now     func() time.Time
}

func newLogger(s logSettings, console io.Writer) *logger {
	l := &logger{s: s, console: console, now: time.Now}
	if s.logFile != "" {
		path, _ := filepath.Abs(s.logFile)
		os.MkdirAll(filepath.Dir(path), 0o755)
		f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			fmt.Fprintf(console, "Failed to open log file %s: %v\n", s.logFile, err)
		} else {
			l.file = f
		}
	}
	return l
}

// enabled applies the logger levels (§5.1); every name we log under is
// an org.asamk logger.
func (l *logger) enabled(level, name string) bool {
	min := "INFO"
	if !strings.HasPrefix(name, "org.asamk") {
		min = "WARN"
	}
	switch {
	case l.s.verbose == 1:
		min = map[bool]string{true: "DEBUG", false: "INFO"}[strings.HasPrefix(name, "org.asamk")]
	case l.s.verbose == 2:
		min = map[bool]string{true: "TRACE", false: "INFO"}[strings.HasPrefix(name, "org.asamk")]
	case l.s.verbose >= 3:
		min = "TRACE"
	}
	return levelRank[level] >= levelRank[min]
}

// log writes one event under its full logger name.
func (l *logger) log(level, name, msg string) {
	if !l.enabled(level, name) {
		return
	}
	short := name[strings.LastIndexByte(name, '.')+1:]
	simple := fmt.Sprintf("%-5s %s - %s\n", level, short, msg)
	detailed := fmt.Sprintf("%s [main] %-5s %s - %s\n", l.now().Format("2006-01-02T15:04:05.000Z0700"), level, abbreviateLogger(name), msg)
	if l.s.scrub {
		simple, detailed = scrubLog(simple), scrubLog(detailed)
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	switch {
	case l.s.logFile != "":
		if levelRank[level] >= levelRank["WARN"] || (level == "INFO" && strings.HasPrefix(name, "org.asamk")) {
			io.WriteString(l.console, simple)
		}
		if l.file != nil {
			io.WriteString(l.file, detailed)
		}
	case l.s.verbose >= 1:
		io.WriteString(l.console, detailed)
	default:
		io.WriteString(l.console, simple)
	}
}

// logLine is a simple-format line as commands write it.
var logLine = regexp.MustCompile(`^(TRACE|DEBUG|INFO|WARN|ERROR) +([A-Za-z0-9$_]+) - (.*)$`)

// logWriter is stderr for commands: log lines become events, the rest
// passes through.
type logWriter struct {
	l   *logger
	out io.Writer
	mu  sync.Mutex
	buf []byte
}

func (w *logWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf = append(w.buf, p...)
	for {
		i := bytes.IndexByte(w.buf, '\n')
		if i < 0 {
			break
		}
		line := string(w.buf[:i])
		w.buf = w.buf[i+1:]
		if m := logLine.FindStringSubmatch(line); m != nil {
			name, ok := loggerNames[m[2]]
			if !ok {
				name = "org.asamk.signal." + m[2]
			}
			w.l.log(m[1], name, m[3])
			continue
		}
		if _, err := io.WriteString(w.out, line+"\n"); err != nil {
			return len(p), err
		}
	}
	return len(p), nil
}

// flush writes a trailing partial line.
func (w *logWriter) flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.buf) > 0 {
		w.out.Write(w.buf)
		w.buf = nil
	}
}
