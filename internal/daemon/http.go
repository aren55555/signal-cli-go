package daemon

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf16"

	"github.com/aren55555/signal-cli-go/internal/errcode"
	"github.com/aren55555/signal-cli-go/internal/jackson"
)

// HTTP serves /api/v1/rpc, /api/v1/events and /api/v1/check the way the
// JDK's built-in server and signal-cli's handler answer (transports.md
// §8). It writes responses byte for byte, so it does not use net/http.
type HTTP struct {
	S        *Server
	BindHost string // as given on the command line
	Port     int
	// KeepAlive is the interval of the events stream's comment line.
	KeepAlive time.Duration

	mu    sync.Mutex
	conns map[net.Conn]bool
	done  chan struct{}
}

var reasons = map[int]string{200: "OK", 201: "Created", 400: "Bad Request", 404: "Not Found", 405: "Method Not Allowed", 415: "Unsupported Media Type", 421: "", 500: "Internal Server Error"}

// Serve accepts connections until ln is closed.
func (h *HTTP) Serve(ctx context.Context, ln net.Listener) {
	h.mu.Lock()
	h.conns = map[net.Conn]bool{}
	h.done = make(chan struct{})
	h.mu.Unlock()
	for {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		h.mu.Lock()
		h.conns[c] = true
		h.mu.Unlock()
		go h.serveConn(ctx, c)
	}
}

// Close ends every open connection (events streams included).
func (h *HTTP) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.done != nil {
		select {
		case <-h.done:
		default:
			close(h.done)
		}
	}
	for c := range h.conns {
		c.Close()
	}
}

type httpRequest struct {
	method, target, proto string
	headers               [][2]string
	body                  []byte
}

func (r *httpRequest) header(name string) (string, bool) {
	for _, h := range r.headers {
		if strings.EqualFold(h[0], name) {
			return h[1], true
		}
	}
	return "", false
}

var errBadRequest = errors.New("bad request")

func readRequest(br *bufio.Reader) (*httpRequest, error) {
	line, err := br.ReadString('\n')
	if err != nil {
		return nil, err
	}
	line = strings.TrimRight(line, "\r\n")
	if line == "" { // tolerate a blank line between requests
		return readRequest(br)
	}
	parts := strings.SplitN(line, " ", 3)
	if len(parts) != 3 {
		return nil, errBadRequest
	}
	r := &httpRequest{method: parts[0], target: parts[1], proto: parts[2]}
	for {
		l, err := br.ReadString('\n')
		if err != nil {
			return nil, err
		}
		l = strings.TrimRight(l, "\r\n")
		if l == "" {
			break
		}
		name, value, ok := strings.Cut(l, ":")
		if !ok {
			return nil, errBadRequest
		}
		r.headers = append(r.headers, [2]string{strings.TrimSpace(name), strings.TrimSpace(value)})
	}
	if te, _ := r.header("Transfer-Encoding"); strings.EqualFold(te, "chunked") {
		for {
			sz, err := br.ReadString('\n')
			if err != nil {
				return nil, err
			}
			n, err := strconv.ParseInt(strings.TrimSpace(strings.SplitN(sz, ";", 2)[0]), 16, 64)
			if err != nil {
				return nil, errBadRequest
			}
			if n == 0 {
				for { // trailers
					t, err := br.ReadString('\n')
					if err != nil || strings.TrimRight(t, "\r\n") == "" {
						break
					}
				}
				break
			}
			chunk := make([]byte, n)
			if _, err := io.ReadFull(br, chunk); err != nil {
				return nil, err
			}
			r.body = append(r.body, chunk...)
			br.ReadString('\n')
		}
	} else if cl, ok := r.header("Content-Length"); ok {
		n, err := strconv.Atoi(cl)
		if err != nil || n < 0 {
			return nil, errBadRequest
		}
		r.body = make([]byte, n)
		if _, err := io.ReadFull(br, r.body); err != nil {
			return nil, err
		}
	}
	return r, nil
}

func (h *HTTP) serveConn(ctx context.Context, c net.Conn) {
	defer func() {
		c.Close()
		h.mu.Lock()
		delete(h.conns, c)
		h.mu.Unlock()
	}()
	br := bufio.NewReader(c)
	for {
		r, err := readRequest(br)
		if err != nil {
			return
		}
		if !h.handle(ctx, c, br, r) {
			return
		}
		if v, _ := r.header("Connection"); strings.EqualFold(v, "close") || r.proto == "HTTP/1.0" {
			return
		}
	}
}

func httpDate() string { return time.Now().UTC().Format("Mon, 02 Jan 2006 15:04:05 GMT") }

// jdkPage is the JDK server's own error page; the connection closes.
func jdkPage(w io.Writer, code errcode.Code) {
	status := code.HTTPStatus()
	body := code.Template()
	fmt.Fprintf(w, "HTTP/1.1 %d %s\r\nContent-Length: %d\r\nContent-Type: text/html\r\nConnection: close\r\n\r\n%s", status, reasons[status], len(body), body)
}

// empty is a response without a body. A HEAD request gets no
// Content-length.
func empty(w io.Writer, r *httpRequest, status int) {
	fmt.Fprintf(w, "HTTP/1.1 %d %s\r\nDate: %s\r\n", status, reasons[status], httpDate())
	if r.method != "HEAD" {
		io.WriteString(w, "Content-length: 0\r\n")
	}
	io.WriteString(w, "\r\n")
}

func emptyFor(w io.Writer, r *httpRequest, code errcode.Code) { empty(w, r, code.HTTPStatus()) }

// handle answers one request; false closes the connection.
func (h *HTTP) handle(ctx context.Context, c net.Conn, br *bufio.Reader, r *httpRequest) bool {
	host, hasHost := r.header("Host")
	if !hasHost && r.proto == "HTTP/1.1" {
		jdkPage(c, errcode.HTTPMissingHost)
		return false
	}
	path, query, _ := strings.Cut(r.target, "?")
	if p, err := url.PathUnescape(path); err == nil {
		path = p
	}
	var endpoint string
	for _, e := range []string{"/api/v1/rpc", "/api/v1/events", "/api/v1/check"} {
		if strings.HasPrefix(path, e) {
			endpoint = e
		}
	}
	switch endpoint {
	case "":
		jdkPage(c, errcode.HTTPNoContext)
		return false
	case "/api/v1/check":
		switch {
		case path != endpoint:
			emptyFor(c, r, errcode.HTTPNotFound)
		case r.method != "GET":
			emptyFor(c, r, errcode.HTTPMethodNotAllowed)
		default:
			empty(c, r, 200)
		}
		return true
	}
	switch {
	case !h.hostAllowed(host):
		emptyFor(c, r, errcode.HTTPMisdirected)
		return true
	case path != endpoint:
		emptyFor(c, r, errcode.HTTPNotFound)
		return true
	}
	if endpoint == "/api/v1/events" {
		if r.method != "GET" {
			emptyFor(c, r, errcode.HTTPMethodNotAllowed)
			return true
		}
		return h.events(ctx, c, br, r, query)
	}
	if r.method != "POST" {
		emptyFor(c, r, errcode.HTTPMethodNotAllowed)
		return true
	}
	if ct, _ := r.header("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		emptyFor(c, r, errcode.HTTPUnsupportedMediaType)
		return true
	}
	resp := escapeSupplementary(h.rpc(ctx, string(r.body)))
	if resp == "" {
		empty(c, r, 201)
		return true
	}
	fmt.Fprintf(c, "HTTP/1.1 200 OK\r\nDate: %s\r\nContent-type: application/json\r\nContent-length: %d\r\n\r\n%s", httpDate(), len(resp), resp)
	return true
}

// rpc runs one HTTP body; "" means no response (201).
func (h *HTTP) rpc(ctx context.Context, body string) string {
	n, err := jackson.Parse(body, jackson.Bytes)
	switch {
	case errors.Is(err, errcode.JSONNestingDepth):
		return errorResponse(jackson.NewNull(), errcode.RPCInternal)
	case err != nil:
		return errorResponse(jackson.NewNull(), err)
	case n.Kind == jackson.Missing:
		return errorResponse(jackson.NewNull(), errcode.RPCUnexpectedType.New("MISSING"))
	}
	var out string
	var mu sync.Mutex
	var wg sync.WaitGroup
	spawn := func(f func()) {
		wg.Add(1)
		go func() { defer wg.Done(); f() }()
	}
	h.S.handleMessage(ctx, nil, n, true, spawn, func(s string) {
		mu.Lock()
		out = s
		mu.Unlock()
	})
	wg.Wait()
	return out
}

// hostAllowed applies the Host pinning (transports.md §8).
func (h *HTTP) hostAllowed(value string) bool {
	switch h.BindHost {
	case "0.0.0.0", "::", "[::]", "":
		return true
	}
	host, port := value, ""
	if strings.HasPrefix(value, "[") {
		if i := strings.Index(value, "]"); i > 0 {
			host = value[1:i]
			port = strings.TrimPrefix(value[i+1:], ":")
		}
	} else if strings.Count(value, ":") == 1 {
		host, port, _ = strings.Cut(value, ":")
	}
	host = strings.ToLower(host)
	bind := strings.ToLower(strings.Trim(h.BindHost, "[]"))
	if host != bind && host != "localhost" && host != "127.0.0.1" && host != "::1" {
		return false
	}
	if port != "" {
		if n, err := strconv.Atoi(port); err == nil && n != h.Port {
			return false
		}
	}
	return true
}

// queryParam returns the URL-decoded value of key, keeping a literal
// '+' (transports.md §8.2).
func queryParam(query, key string) (string, bool) {
	for _, kv := range strings.Split(query, "&") {
		k, v, _ := strings.Cut(kv, "=")
		if k != key {
			continue
		}
		if d, err := url.PathUnescape(v); err == nil {
			v = d
		}
		return v, true
	}
	return "", false
}

// events streams received messages as Server-Sent Events until the
// client goes away or the server shuts down.
func (h *HTTP) events(ctx context.Context, c net.Conn, br *bufio.Reader, r *httpRequest, query string) bool {
	var accounts []string
	if h.S.SingleAccount != "" {
		accounts = []string{h.S.SingleAccount}
	} else if a, ok := queryParam(query, "account"); ok && a != "" {
		acc, found := h.S.findAccount(a)
		if !found {
			emptyFor(c, r, errcode.HTTPUnknownAccount)
			return true
		}
		accounts = []string{acc.Number}
	} else {
		accounts = []string{}
		for _, a := range h.S.Accounts() {
			accounts = append(accounts, a.Number)
		}
	}
	fmt.Fprintf(c, "HTTP/1.1 200 OK\r\nDate: %s\r\nTransfer-encoding: chunked\r\nContent-type: text/event-stream\r\n\r\n", httpDate())
	var wmu sync.Mutex
	failed := make(chan struct{})
	var once sync.Once
	chunk := func(s string) {
		wmu.Lock()
		defer wmu.Unlock()
		if _, err := fmt.Fprintf(c, "%x\r\n%s\r\n", len(s), s); err != nil {
			once.Do(func() { close(failed) })
		}
	}
	remove := h.S.AddListener(accounts, func(account, payload string) {
		chunk("event:receive\ndata:" + payload + "\n\n")
	})
	defer remove()
	gone := make(chan struct{})
	go func() {
		io.Copy(io.Discard, br)
		close(gone)
	}()
	ka := h.KeepAlive
	if ka <= 0 {
		ka = 15 * time.Second
	}
	t := time.NewTicker(ka)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			chunk(":\n")
		case <-gone:
			return false
		case <-failed:
			return false
		case <-h.done:
			return false
		case <-ctx.Done():
			return false
		}
	}
}

// escapeSupplementary writes characters outside the BMP as escaped
// UTF-16 surrogate pairs (upper-case hex), as the reference's UTF-8
// JSON generator does for HTTP responses.
func escapeSupplementary(s string) string {
	if !strings.ContainsFunc(s, func(r rune) bool { return r > 0xFFFF }) {
		return s
	}
	var b strings.Builder
	for _, r := range s {
		if r > 0xFFFF {
			hi, lo := utf16.EncodeRune(r)
			fmt.Fprintf(&b, `\u%04X\u%04X`, hi, lo)
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
