package difftest

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// RPCScenario is one exchange with a running daemon.
//
// For Transport "http" the harness builds a raw HTTP/1.1 request from
// Method, Path, Headers and Body. It writes the request bytes itself
// because net/http would rewrite header casing and always add Host, and
// the reference server's handling of exactly those details is under test.
//
// For "tcp" and "socket" the harness writes Send verbatim on a new
// connection and collects every line that comes back until the
// connection has been quiet for a moment.
type RPCScenario struct {
	Name      string `json:"name"`
	Transport string `json:"transport"` // http, tcp, socket
	Note      string `json:"note,omitempty"`

	Method string `json:"method,omitempty"`
	Path   string `json:"path,omitempty"`
	// Headers are sent in order, as written. {{HOST}} expands to
	// 127.0.0.1:<port> and {{PORT}} to the HTTP port. A Host header is
	// added first unless NoHost is set or Headers already has one.
	Headers []string `json:"headers,omitempty"`
	NoHost  bool     `json:"noHost,omitempty"`
	Body    *string  `json:"body,omitempty"`
	// StreamFor reads a streaming body (SSE) for this many milliseconds.
	StreamFor int `json:"streamForMs,omitempty"`

	Send string `json:"send,omitempty"`
}

// RPCResult is what came back, normalized.
type RPCResult struct {
	Status   string   `json:"status,omitempty"`
	Headers  []string `json:"headers,omitempty"`
	Body     string   `json:"body,omitempty"`
	Messages []string `json:"messages,omitempty"`
}

// RPCGolden is a recorded reference exchange.
type RPCGolden struct {
	Scenario RPCScenario `json:"scenario"`
	Oracle   string      `json:"oracle"`
	Result   RPCResult   `json:"result"`
}

// Daemon is a running `signal-cli daemon` serving HTTP, TCP and a UNIX
// socket, with an empty config directory (zero accounts, multi-account
// mode).
type Daemon struct {
	HTTPAddr, TCPAddr, Socket string
	cmd                       *exec.Cmd
	stderr                    *lockedBuffer
	cfg, scratch              string
}

// StartDaemon launches bin as a daemon and waits until its HTTP port
// accepts connections.
func StartDaemon(bin string) (*Daemon, error) {
	cfg, err := os.MkdirTemp(scratchDir(), "difftest-config-")
	if err != nil {
		return nil, err
	}
	scratch, err := os.MkdirTemp(scratchDir(), "difftest-scratch-")
	if err != nil {
		return nil, err
	}
	d := &Daemon{cfg: cfg, scratch: scratch, Socket: filepath.Join(scratch, "rpc.sock"), stderr: &lockedBuffer{}}
	if d.HTTPAddr, err = freePort(); err != nil {
		return nil, err
	}
	if d.TCPAddr, err = freePort(); err != nil {
		return nil, err
	}
	d.cmd = exec.Command(bin, "--config", cfg, "daemon",
		"--http", d.HTTPAddr, "--tcp", d.TCPAddr, "--socket", d.Socket)
	d.cmd.Env = scenarioEnv(scratch)
	d.cmd.Stdout = io.Discard
	d.cmd.Stderr = d.stderr
	if err := d.cmd.Start(); err != nil {
		return nil, err
	}
	exited := make(chan error, 1)
	go func() { exited <- d.cmd.Wait() }()

	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case err := <-exited:
			d.cleanupDirs()
			return nil, fmt.Errorf("daemon exited during start-up (%v); stderr:\n%s", err, d.stderr.String())
		default:
		}
		if ready(d.HTTPAddr) && ready(d.TCPAddr) {
			if _, err := os.Stat(d.Socket); err == nil {
				return d, nil
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	d.Stop()
	return nil, fmt.Errorf("daemon not ready after 60s; stderr:\n%s", d.stderr.String())
}

func ready(addr string) bool {
	c, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
	if err != nil {
		return false
	}
	c.Close()
	return true
}

func freePort() (string, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	defer l.Close()
	return l.Addr().String(), nil
}

// Stop terminates the daemon and removes its directories.
func (d *Daemon) Stop() {
	if d.cmd != nil && d.cmd.Process != nil {
		_ = d.cmd.Process.Signal(os.Interrupt)
		done := make(chan struct{})
		go func() { _, _ = d.cmd.Process.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			_ = d.cmd.Process.Kill()
		}
	}
	d.cleanupDirs()
}

func (d *Daemon) cleanupDirs() {
	os.RemoveAll(d.cfg)
	os.RemoveAll(d.scratch)
}

// normalize hides the per-run ports and paths.
func (d *Daemon) normalize(s string) string {
	for _, addr := range []string{d.HTTPAddr, d.TCPAddr} {
		_, port, _ := net.SplitHostPort(addr)
		s = strings.ReplaceAll(s, ":"+port, ":<PORT>")
	}
	s = strings.ReplaceAll(s, d.Socket, "<SOCKET>")
	return Normalize(s, d.cfg, d.scratch)
}

// Exchange runs one scenario against the daemon.
func (d *Daemon) Exchange(ctx context.Context, sc RPCScenario) (RPCResult, error) {
	switch sc.Transport {
	case "http":
		return d.httpExchange(sc)
	case "tcp":
		return d.streamExchange("tcp", d.TCPAddr, sc)
	case "socket":
		return d.streamExchange("unix", d.Socket, sc)
	}
	return RPCResult{}, fmt.Errorf("%s: unknown transport %q", sc.Name, sc.Transport)
}

func (d *Daemon) httpExchange(sc RPCScenario) (RPCResult, error) {
	var req bytes.Buffer
	fmt.Fprintf(&req, "%s %s HTTP/1.1\r\n", sc.Method, sc.Path)
	hasHost := false
	for _, h := range sc.Headers {
		if strings.HasPrefix(strings.ToLower(h), "host:") {
			hasHost = true
		}
	}
	if !sc.NoHost && !hasHost {
		fmt.Fprintf(&req, "Host: %s\r\n", d.HTTPAddr)
	}
	for _, h := range sc.Headers {
		_, port, _ := net.SplitHostPort(d.HTTPAddr)
		h = strings.NewReplacer("{{HOST}}", d.HTTPAddr, "{{PORT}}", port).Replace(h)
		fmt.Fprintf(&req, "%s\r\n", h)
	}
	if sc.Body != nil {
		fmt.Fprintf(&req, "Content-Length: %d\r\n", len(*sc.Body))
	}
	req.WriteString("\r\n")
	if sc.Body != nil {
		req.WriteString(*sc.Body)
	}

	conn, err := net.DialTimeout("tcp", d.HTTPAddr, 5*time.Second)
	if err != nil {
		return RPCResult{}, err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(30 * time.Second))
	if _, err := conn.Write(req.Bytes()); err != nil {
		return RPCResult{}, err
	}

	r := bufio.NewReader(conn)
	status, err := r.ReadString('\n')
	if err != nil {
		return RPCResult{}, fmt.Errorf("%s: reading status line: %w", sc.Name, err)
	}
	res := RPCResult{Status: strings.TrimRight(status, "\r\n")}
	length, chunked := -1, false
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return RPCResult{}, fmt.Errorf("%s: reading headers: %w", sc.Name, err)
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		name, value, _ := strings.Cut(line, ":")
		value = strings.TrimSpace(value)
		switch strings.ToLower(name) {
		case "date":
			continue // differs on every request
		case "content-length":
			length, _ = strconv.Atoi(value)
		case "transfer-encoding":
			chunked = strings.EqualFold(value, "chunked")
		}
		res.Headers = append(res.Headers, line)
	}

	var body []byte
	switch {
	case sc.StreamFor > 0:
		_ = conn.SetReadDeadline(time.Now().Add(time.Duration(sc.StreamFor) * time.Millisecond))
		body, _ = io.ReadAll(r)
		if chunked {
			body = dechunkPartial(body)
		}
	case length >= 0:
		body = make([]byte, length)
		if _, err := io.ReadFull(r, body); err != nil {
			return RPCResult{}, fmt.Errorf("%s: reading body: %w", sc.Name, err)
		}
	case chunked:
		raw, _ := io.ReadAll(r)
		body = dechunkPartial(raw)
	default:
		_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		body, _ = io.ReadAll(r)
	}
	res.Body = d.normalize(string(body))
	res.Status = d.normalize(res.Status)
	for i := range res.Headers {
		res.Headers[i] = d.normalize(res.Headers[i])
	}
	return res, nil
}

// dechunkPartial decodes HTTP chunked framing, tolerating a stream that
// was cut off mid-chunk (SSE is read for a fixed time).
func dechunkPartial(b []byte) []byte {
	var out []byte
	for len(b) > 0 {
		i := bytes.Index(b, []byte("\r\n"))
		if i < 0 {
			break
		}
		n, err := strconv.ParseInt(strings.TrimSpace(strings.SplitN(string(b[:i]), ";", 2)[0]), 16, 64)
		if err != nil || n == 0 {
			break
		}
		b = b[i+2:]
		if int64(len(b)) < n {
			out = append(out, b...)
			break
		}
		out = append(out, b[:n]...)
		b = bytes.TrimPrefix(b[n:], []byte("\r\n"))
	}
	return out
}

// quiet is how long a stream connection must be silent before the
// harness stops reading. Long enough for the JVM to answer a burst.
const quiet = 1500 * time.Millisecond

func (d *Daemon) streamExchange(network, addr string, sc RPCScenario) (RPCResult, error) {
	conn, err := net.DialTimeout(network, addr, 5*time.Second)
	if err != nil {
		return RPCResult{}, err
	}
	defer conn.Close()
	if _, err := io.WriteString(conn, sc.Send); err != nil {
		return RPCResult{}, err
	}
	var res RPCResult
	r := bufio.NewReader(conn)
	end := time.Now().Add(30 * time.Second)
	for time.Now().Before(end) {
		_ = conn.SetReadDeadline(time.Now().Add(quiet))
		line, err := r.ReadString('\n')
		if line != "" {
			res.Messages = append(res.Messages, d.normalize(strings.TrimRight(line, "\r\n")))
		}
		var ne net.Error
		if errors.As(err, &ne) && ne.Timeout() {
			break
		}
		if err != nil {
			break
		}
	}
	return res, nil
}

// RPCDiff compares results. JSON bodies and messages are compared as
// values, so key order does not matter. Top-level batch arrays and the
// set of stream messages are compared as multisets: the reference runs
// requests concurrently and answers in completion order.
func RPCDiff(want, got RPCResult) string {
	var b strings.Builder
	if want.Status != got.Status {
		fmt.Fprintf(&b, "status: want %q, got %q\n", want.Status, got.Status)
	}
	if !reflect.DeepEqual(want.Headers, got.Headers) {
		fmt.Fprintf(&b, "headers:\n  want %q\n  got  %q\n", want.Headers, got.Headers)
	}
	if !sameBody(want.Body, got.Body) {
		fmt.Fprintf(&b, "body:\n  want %q\n  got  %q\n", want.Body, got.Body)
	}
	if !sameMessages(want.Messages, got.Messages) {
		fmt.Fprintf(&b, "messages:\n  want %q\n  got  %q\n", want.Messages, got.Messages)
	}
	return b.String()
}

func sameBody(a, b string) bool {
	if a == b {
		return true
	}
	va, errA := decodeJSON(a)
	vb, errB := decodeJSON(b)
	if errA != nil || errB != nil {
		return false
	}
	if la, ok := va.([]any); ok {
		lb, ok := vb.([]any)
		return ok && sameMultiset(la, lb)
	}
	return reflect.DeepEqual(va, vb)
}

func sameMessages(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	va, vb := make([]any, len(a)), make([]any, len(b))
	for i := range a {
		va[i] = jsonOrString(a[i])
		vb[i] = jsonOrString(b[i])
	}
	return sameMultiset(va, vb)
}

func jsonOrString(s string) any {
	if v, err := decodeJSON(s); err == nil {
		return v
	}
	return s
}

func decodeJSON(s string) (any, error) {
	dec := json.NewDecoder(strings.NewReader(s))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	if dec.More() {
		return nil, errors.New("trailing data")
	}
	return v, nil
}

// sameMultiset reports whether a and b hold the same values, ignoring
// order. Values are compared through a canonical encoding (encoding/json
// sorts map keys).
func sameMultiset(a, b []any) bool {
	if len(a) != len(b) {
		return false
	}
	canon := func(vs []any) []string {
		out := make([]string, len(vs))
		for i, v := range vs {
			enc, _ := json.Marshal(v)
			out[i] = string(enc)
		}
		sort.Strings(out)
		return out
	}
	return reflect.DeepEqual(canon(a), canon(b))
}

type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}
