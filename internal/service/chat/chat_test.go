// SPDX-License-Identifier: GPL-3.0-or-later

package chat

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aren55555/signal-cli-go/internal/proto/signalpb"
	"google.golang.org/protobuf/proto"
)

// fakeSocket is the client end of an in-memory socket; the test plays
// the server through toClient and fromClient.
type fakeSocket struct {
	toClient   chan []byte
	fromClient chan []byte
	once       sync.Once
	closed     chan struct{}
}

func newFake() *fakeSocket {
	return &fakeSocket{toClient: make(chan []byte, 16), fromClient: make(chan []byte, 16), closed: make(chan struct{})}
}

func (f *fakeSocket) Read() ([]byte, error) {
	select {
	case b := <-f.toClient:
		return b, nil
	case <-f.closed:
		return nil, errSocketClosed
	}
}

func (f *fakeSocket) Write(b []byte) error {
	select {
	case <-f.closed:
		return errSocketClosed
	default:
	}
	f.fromClient <- append([]byte(nil), b...)
	return nil
}

func (f *fakeSocket) Close() error { f.once.Do(func() { close(f.closed) }); return nil }

var errSocketClosed = errors.New("fake socket closed")

func (f *fakeSocket) recv(t *testing.T) *signalpb.ChatWebSocketMessage {
	t.Helper()
	select {
	case b := <-f.fromClient:
		m := &signalpb.ChatWebSocketMessage{}
		if err := proto.Unmarshal(b, m); err != nil {
			t.Fatal(err)
		}
		return m
	case <-time.After(5 * time.Second):
		t.Fatal("nothing from the client")
		return nil
	}
}

func (f *fakeSocket) quiet(t *testing.T, d time.Duration) {
	t.Helper()
	select {
	case b := <-f.fromClient:
		t.Fatalf("unexpected frame from client: %x", b)
	case <-time.After(d):
	}
}

func (f *fakeSocket) send(t *testing.T, m *signalpb.ChatWebSocketMessage) {
	t.Helper()
	b, err := proto.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	f.toClient <- b
}

func serverRequest(id uint64, verb, path string, body []byte, headers ...string) *signalpb.ChatWebSocketMessage {
	return &signalpb.ChatWebSocketMessage{
		Type:    signalpb.ChatWebSocketMessage_REQUEST.Enum(),
		Request: &signalpb.ChatWebSocketRequest{Id: proto.Uint64(id), Verb: proto.String(verb), Path: proto.String(path), Body: body, Headers: headers},
	}
}

func response(id uint64, status uint32, body string, headers ...string) *signalpb.ChatWebSocketMessage {
	return &signalpb.ChatWebSocketMessage{
		Type:     signalpb.ChatWebSocketMessage_RESPONSE.Enum(),
		Response: &signalpb.ChatWebSocketResponse{Id: proto.Uint64(id), Status: proto.Uint32(status), Message: proto.String("OK"), Body: []byte(body), Headers: headers},
	}
}

var noKeepAlive = Config{}

// TestRequestWireForm: ids count from 0 per connection, headers are
// "<lower-case name>: <value>", the body field is always present (zero
// length for GET), and responses are matched by id even out of order
// (spec/service/pipe.md §3, transport.md §3.5).
func TestRequestWireForm(t *testing.T) {
	fs := newFake()
	c := New(fs, noKeepAlive)
	defer c.Close()
	ctx := context.Background()
	type res struct {
		r   *Response
		err error
	}
	r0, r1 := make(chan res, 1), make(chan res, 1)
	go func() {
		r, err := c.Request(ctx, "get", "/v2/keys?identity=aci", nil, nil)
		r0 <- res{r, err}
	}()
	m0 := fs.recv(t)
	go func() {
		r, err := c.Request(ctx, "PUT", "/v1/messages/x?story=false", []Header{{"Content-Type", "application/json"}}, []byte(`{}`))
		r1 <- res{r, err}
	}()
	m1 := fs.recv(t)

	q0, q1 := m0.GetRequest(), m1.GetRequest()
	if m0.GetType() != signalpb.ChatWebSocketMessage_REQUEST || q0.GetId() != 0 || q0.GetVerb() != "GET" || q0.GetPath() != "/v2/keys?identity=aci" {
		t.Errorf("first request %v", m0)
	}
	if q0.Body == nil || len(q0.Body) != 0 || len(q0.Headers) != 0 {
		t.Errorf("GET body %v (present: %v), headers %q", q0.Body, q0.Body != nil, q0.Headers)
	}
	if q1.GetId() != 1 || q1.GetVerb() != "PUT" || string(q1.Body) != "{}" || len(q1.Headers) != 1 || q1.Headers[0] != "content-type: application/json" {
		t.Errorf("second request %v", m1)
	}
	fs.send(t, response(1, 409, `{"missing":[]}`, "Content-Type:application/json", "x-a: b"))
	fs.send(t, response(0, 200, "keys"))
	a := <-r0
	b := <-r1
	if a.err != nil || a.r.Status != 200 || string(a.r.Body) != "keys" {
		t.Errorf("response 0: %+v, %v", a.r, a.err)
	}
	if b.err != nil || b.r.Status != 409 || b.r.Header("content-type") != "application/json" || b.r.Header("X-A") != "b" {
		t.Errorf("response 1: %+v, %v", b.r, b.err)
	}
}

// TestEnvelopeDelivery: PUT /api/v1/message is delivered with its
// timestamp and acked with 200 "OK" and nothing else; queue/empty is
// delivered but never answered; other requests are dropped without a
// response (pipe.md §4).
func TestEnvelopeDelivery(t *testing.T) {
	fs := newFake()
	c := New(fs, noKeepAlive)
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	fs.send(t, serverRequest(7, "PUT", "/v1/unknown", nil))
	fs.send(t, serverRequest(41, "PUT", "/api/v1/message", []byte{1, 2, 3}, "X-Signal-Timestamp: 12", "x-signal-timestamp:  1700000000123 ", "X-SIGNAL-TIMESTAMP: junk"))
	fs.send(t, serverRequest(42, "PUT", "/api/v1/message", nil))
	fs.send(t, serverRequest(43, "PUT", "/api/v1/queue/empty", nil))

	r, err := c.Next(ctx)
	if err != nil || r.Kind != KindEnvelope || string(r.Body) != "\x01\x02\x03" || r.ServerTimestamp != 1700000000123 {
		t.Fatalf("first delivery %+v, %v", r, err)
	}
	if err := r.Ack(); err != nil {
		t.Fatal(err)
	}
	ack := fs.recv(t)
	resp := ack.GetResponse()
	if ack.GetType() != signalpb.ChatWebSocketMessage_RESPONSE || ack.Request != nil || resp.GetId() != 41 || resp.GetStatus() != 200 || resp.GetMessage() != "OK" || resp.Body != nil || resp.Headers != nil {
		t.Errorf("ack %v", ack)
	}
	r, err = c.Next(ctx)
	if err != nil || r.Kind != KindEnvelope || len(r.Body) != 0 || r.ServerTimestamp != 0 {
		t.Fatalf("empty envelope %+v, %v", r, err)
	}
	r, err = c.Next(ctx)
	if err != nil || r.Kind != KindQueueEmpty {
		t.Fatalf("queue empty %+v, %v", r, err)
	}
	if err := r.Ack(); err == nil {
		t.Error("acking queue/empty should be refused")
	}
	fs.quiet(t, 50*time.Millisecond) // no response to the unknown request or queue/empty
}

// TestKeepAlive: GET /v1/keepalive every interval, the first one interval
// after connecting; a 2xx keeps the socket, a missing reply within the
// timeout, or a 4xx, tears it down (pipe.md §5.1).
func TestKeepAlive(t *testing.T) {
	cfg := Config{KeepAliveInterval: 40 * time.Millisecond, KeepAliveTimeout: 60 * time.Millisecond}
	for _, tc := range []struct {
		name   string
		status uint32 // 0: never answer the second keep-alive
	}{{"no reply", 0}, {"4xx", 404}} {
		t.Run(tc.name, func(t *testing.T) {
			fs := newFake()
			start := time.Now()
			c := New(fs, cfg)
			defer c.Close()
			m := fs.recv(t)
			if d := time.Since(start); d < 35*time.Millisecond {
				t.Errorf("first keep-alive after %v", d)
			}
			q := m.GetRequest()
			if q.GetVerb() != "GET" || q.GetPath() != "/v1/keepalive" || len(q.Headers) != 0 || q.Body == nil {
				t.Errorf("keep-alive %v", m)
			}
			fs.send(t, response(q.GetId(), 200, ""))
			m = fs.recv(t)
			if m.GetRequest().GetPath() != "/v1/keepalive" || m.GetRequest().GetId() != q.GetId()+1 {
				t.Errorf("second keep-alive %v", m)
			}
			if tc.status != 0 {
				fs.send(t, response(m.GetRequest().GetId(), tc.status, ""))
			}
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_, err := c.Next(ctx)
			if !errors.Is(err, ErrKeepAlive) || (tc.status != 0) != strings.Contains(err.Error(), "status 404") {
				t.Errorf("Next after failed keep-alive: %v", err)
			}
			select {
			case <-fs.closed:
			default:
				t.Error("socket left open")
			}
		})
	}
}

// TestCloseFailsPending: when the socket dies, outstanding requests and
// Next fail with the socket's error.
func TestCloseFailsPending(t *testing.T) {
	fs := newFake()
	c := New(fs, noKeepAlive)
	errc := make(chan error, 1)
	go func() { _, err := c.Request(context.Background(), "GET", "/v1/x", nil, nil); errc <- err }()
	fs.recv(t)
	fs.Close()
	if err := <-errc; !errors.Is(err, errSocketClosed) {
		t.Errorf("pending request: %v", err)
	}
	if _, err := c.Next(context.Background()); !errors.Is(err, errSocketClosed) {
		t.Errorf("Next: %v", err)
	}
	if _, err := c.Request(context.Background(), "GET", "/v1/x", nil, nil); !errors.Is(err, errSocketClosed) {
		t.Errorf("request after close: %v", err)
	}
}

// TestRequestContext: a request whose context ends fails without
// killing the connection; its late response is ignored.
func TestRequestContext(t *testing.T) {
	fs := newFake()
	c := New(fs, noKeepAlive)
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, err := c.Request(ctx, "GET", "/v1/slow", nil, nil); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("slow request: %v", err)
	}
	fs.recv(t)
	fs.send(t, response(0, 200, "late"))
	done := make(chan error, 1)
	go func() { _, err := c.Request(context.Background(), "GET", "/v1/next", nil, nil); done <- err }()
	if m := fs.recv(t); m.GetRequest().GetId() != 1 {
		t.Errorf("next id %d", m.GetRequest().GetId())
	}
	fs.send(t, response(1, 200, ""))
	if err := <-done; err != nil {
		t.Error(err)
	}
}
