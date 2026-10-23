// Package chat speaks Signal's chat-socket protocol (spec/service/pipe.md,
// transport.md §3.5): HTTP-like requests and responses framed as
// ChatWebSocketMessage protobufs, one per binary WebSocket message, in both
// directions.
package chat

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aren55555/signal-cli-go/internal/proto/signalpb"
	"google.golang.org/protobuf/proto"
)

// Errors.
var (
	// ErrClosed reports a connection closed by Close.
	ErrClosed = errors.New("chat: connection closed")
	// ErrKeepAlive reports a keep-alive that got no 2xx reply in time; the
	// socket has been torn down (pipe.md §5.1).
	ErrKeepAlive = errors.New("chat: keep-alive failed")
	// ErrProtocol reports a frame that is not a valid message.
	ErrProtocol = errors.New("chat: protocol error")
)

// Socket is a message-oriented transport: a WebSocket.
type Socket interface {
	Read() ([]byte, error)
	Write([]byte) error
	Close() error
}

// Config holds the application keep-alive settings. A zero
// KeepAliveInterval sends none (a socket without a keep-alive token).
type Config struct {
	KeepAliveInterval time.Duration // signal-cli: 30 s
	KeepAliveTimeout  time.Duration // signal-cli: 20 s
}

// KeepAlive is signal-cli's keep-alive schedule while `receive` runs.
var KeepAlive = Config{KeepAliveInterval: 30 * time.Second, KeepAliveTimeout: 20 * time.Second}

// Header is one request or response header.
type Header struct{ Name, Value string }

// Response is the server's answer to a request. A 4xx or 5xx status is a
// completed round trip, not an error.
type Response struct {
	Status  int
	Message string
	Headers []Header
	Body    []byte
}

// Header returns the value of the last header called name (compared
// case-insensitively), or "".
func (r *Response) Header(name string) string {
	v := ""
	for _, h := range r.Headers {
		if strings.EqualFold(h.Name, name) {
			v = h.Value
		}
	}
	return v
}

// Kind classifies a server request.
type Kind int

const (
	// KindEnvelope is PUT /api/v1/message: Body is a binary Envelope.
	KindEnvelope Kind = iota + 1
	// KindQueueEmpty is PUT /api/v1/queue/empty, sent once per connection
	// when the stored queue has been delivered. It is never answered.
	KindQueueEmpty
)

// ServerRequest is a request pushed by the server.
type ServerRequest struct {
	Kind            Kind
	Body            []byte
	ServerTimestamp uint64 // X-Signal-Timestamp in ms; 0 if absent
	id              uint64
	c               *Conn
}

// Ack acknowledges an envelope (200 "OK", nothing else), deleting it from
// the server's queue.
func (r *ServerRequest) Ack() error {
	if r.Kind != KindEnvelope {
		return errors.New("chat: only envelopes are acknowledged")
	}
	return r.c.write(&signalpb.ChatWebSocketMessage{
		Type:     signalpb.ChatWebSocketMessage_RESPONSE.Enum(),
		Response: &signalpb.ChatWebSocketResponse{Id: proto.Uint64(r.id), Status: proto.Uint32(200), Message: proto.String("OK")},
	})
}

// Conn is a chat connection over a Socket.
type Conn struct {
	s   Socket
	cfg Config
	wmu sync.Mutex

	mu      sync.Mutex
	err     error
	done    chan struct{}
	nextID  uint64
	pending map[uint64]chan *Response
	queue   []*ServerRequest
	wake    chan struct{}
}

// New starts the protocol on s; the Conn owns s from now on.
func New(s Socket, cfg Config) *Conn {
	c := &Conn{s: s, cfg: cfg, done: make(chan struct{}), pending: map[uint64]chan *Response{}, wake: make(chan struct{}, 1)}
	go c.readLoop()
	if cfg.KeepAliveInterval > 0 {
		go c.keepAlive()
	}
	return c
}

// fail ends the connection with err (the first cause wins). Done is
// closed only after the socket has been closed.
func (c *Conn) fail(err error) {
	c.mu.Lock()
	first := c.err == nil
	if first {
		c.err = err
	}
	c.mu.Unlock()
	if first {
		c.s.Close()
		close(c.done)
	}
}

// Err returns why the connection ended, or nil while it is up.
func (c *Conn) Err() error {
	select {
	case <-c.done:
	default:
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.err
}

// Done is closed when the connection ends.
func (c *Conn) Done() <-chan struct{} { return c.done }

// Close closes the connection.
func (c *Conn) Close() error {
	c.fail(ErrClosed)
	return nil
}

func (c *Conn) write(m *signalpb.ChatWebSocketMessage) error {
	b, err := proto.Marshal(m)
	if err != nil {
		return err
	}
	c.wmu.Lock()
	defer c.wmu.Unlock()
	if err := c.Err(); err != nil {
		return err
	}
	if err := c.s.Write(b); err != nil {
		c.fail(err)
		return err
	}
	return nil
}

// Request sends verb and path with headers and body and waits for the
// response or for ctx to end. Header names are sent in lower case; the
// body field is always present, empty for a nil body, as signal-cli
// sends it.
func (c *Conn) Request(ctx context.Context, verb, path string, headers []Header, body []byte) (*Response, error) {
	c.mu.Lock()
	if c.err != nil {
		err := c.err
		c.mu.Unlock()
		return nil, err
	}
	id := c.nextID
	c.nextID++
	ch := make(chan *Response, 1)
	c.pending[id] = ch
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
	}()

	req := &signalpb.ChatWebSocketRequest{
		Verb: proto.String(strings.ToUpper(verb)),
		Path: proto.String(path),
		Body: append([]byte{}, body...),
		Id:   proto.Uint64(id),
	}
	for _, h := range headers {
		req.Headers = append(req.Headers, strings.ToLower(h.Name)+": "+h.Value)
	}
	if err := c.write(&signalpb.ChatWebSocketMessage{Type: signalpb.ChatWebSocketMessage_REQUEST.Enum(), Request: req}); err != nil {
		return nil, err
	}
	select {
	case r := <-ch:
		return r, nil
	case <-c.done:
		return nil, c.Err()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Next returns the next server request, waiting until one arrives, the
// connection ends or ctx ends. Requests received before the connection
// ended are still returned.
func (c *Conn) Next(ctx context.Context) (*ServerRequest, error) {
	for {
		c.mu.Lock()
		if len(c.queue) > 0 {
			r := c.queue[0]
			c.queue = c.queue[1:]
			c.mu.Unlock()
			return r, nil
		}
		c.mu.Unlock()
		if err := c.Err(); err != nil {
			return nil, err
		}
		select {
		case <-c.wake:
		case <-c.done:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

func parseHeader(s string) Header {
	name, value, _ := strings.Cut(s, ":")
	return Header{Name: strings.TrimSpace(name), Value: strings.TrimSpace(value)}
}

// serverTimestamp applies pipe.md §4.1: the last X-Signal-Timestamp that
// parses as an unsigned decimal wins; none gives 0.
func serverTimestamp(headers []string) uint64 {
	var ts uint64
	for _, s := range headers {
		h := parseHeader(s)
		if !strings.EqualFold(h.Name, "x-signal-timestamp") {
			continue
		}
		if v, err := strconv.ParseUint(h.Value, 10, 64); err == nil {
			ts = v
		}
	}
	return ts
}

func (c *Conn) readLoop() {
	for {
		b, err := c.s.Read()
		if err != nil {
			c.fail(err)
			return
		}
		m := &signalpb.ChatWebSocketMessage{}
		if err := proto.Unmarshal(b, m); err != nil {
			c.fail(fmt.Errorf("%w: %v", ErrProtocol, err))
			return
		}
		switch m.GetType() {
		case signalpb.ChatWebSocketMessage_REQUEST:
			c.serverRequest(m.GetRequest())
		case signalpb.ChatWebSocketMessage_RESPONSE:
			r := m.GetResponse()
			resp := &Response{Status: int(r.GetStatus()), Message: r.GetMessage(), Body: r.GetBody()}
			for _, h := range r.GetHeaders() {
				resp.Headers = append(resp.Headers, parseHeader(h))
			}
			c.mu.Lock()
			ch := c.pending[r.GetId()]
			delete(c.pending, r.GetId())
			c.mu.Unlock()
			if ch != nil {
				ch <- resp
			}
		}
	}
}

// serverRequest queues envelope deliveries and queue-empty notices and
// drops anything else without a response (pipe.md §4).
func (c *Conn) serverRequest(q *signalpb.ChatWebSocketRequest) {
	if q == nil || q.GetVerb() != "PUT" {
		return
	}
	r := &ServerRequest{id: q.GetId(), c: c}
	switch q.GetPath() {
	case "/api/v1/message":
		r.Kind, r.Body, r.ServerTimestamp = KindEnvelope, q.GetBody(), serverTimestamp(q.GetHeaders())
	case "/api/v1/queue/empty":
		r.Kind = KindQueueEmpty
	default:
		return
	}
	c.mu.Lock()
	c.queue = append(c.queue, r)
	c.mu.Unlock()
	select {
	case c.wake <- struct{}{}:
	default:
	}
}

// keepAlive sends GET /v1/keepalive every interval and tears the
// connection down if one gets no 2xx reply within the timeout.
func (c *Conn) keepAlive() {
	t := time.NewTicker(c.cfg.KeepAliveInterval)
	defer t.Stop()
	for {
		select {
		case <-c.done:
			return
		case <-t.C:
		}
		ctx, cancel := context.WithTimeout(context.Background(), c.cfg.KeepAliveTimeout)
		r, err := c.Request(ctx, "GET", "/v1/keepalive", nil, nil)
		cancel()
		switch {
		case err != nil && c.Err() != nil:
			return // the connection ended for another reason
		case err != nil:
			c.fail(fmt.Errorf("%w: %v", ErrKeepAlive, err))
			return
		case r.Status < 200 || r.Status > 299:
			c.fail(fmt.Errorf("%w: status %d", ErrKeepAlive, r.Status))
			return
		}
	}
}
