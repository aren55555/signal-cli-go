// Package websocket is a client for the WebSocket protocol (RFC 6455),
// written for signal-cli-go's message pipe and provisioning sockets. It
// dials through an HTTP CONNECT proxy when one is given (the sandbox has
// no other route out) and takes the TLS configuration from the caller, so
// the transport layer decides which roots to trust.
//
// Only binary messages are exposed; Signal's sockets carry protobufs.
// Pings are answered automatically.
package websocket

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha1"
	"crypto/tls"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/aren55555/signal-cli-go/internal/service/tunnel"
)

var (
	// ErrProtocol reports a frame that breaks RFC 6455's rules.
	ErrProtocol = errors.New("websocket: protocol error")
	// ErrHandshake reports a failed opening handshake.
	ErrHandshake = errors.New("websocket: handshake failed")
	// ErrClosed reports a connection closed by either side.
	ErrClosed = errors.New("websocket: connection closed")
)

// CloseError is a close frame from the server; it matches ErrClosed.
// Code is 0 when the frame carried no status code.
type CloseError struct {
	Code   int
	Reason string
}

func (e *CloseError) Error() string {
	return fmt.Sprintf("websocket: closed by server with code %d %q", e.Code, e.Reason)
}

func (e *CloseError) Is(target error) bool { return target == ErrClosed }

type opcode byte

const (
	opContinuation opcode = 0x0
	opText         opcode = 0x1
	opBinary       opcode = 0x2
	opClose        opcode = 0x8
	opPing         opcode = 0x9
	opPong         opcode = 0xa
)

func (o opcode) control() bool { return o&0x8 != 0 }

// keyGUID is the GUID of RFC 6455 section 1.3.
const keyGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

func acceptKey(key string) string {
	h := sha1.Sum([]byte(key + keyGUID))
	return base64.StdEncoding.EncodeToString(h[:])
}

type frame struct {
	fin     bool
	op      opcode
	payload []byte
}

// writeFrame writes one frame, masked with mask if it is non-nil (client
// frames must be masked; RFC 6455 section 5.3).
func writeFrame(w io.Writer, fin bool, op opcode, payload []byte, mask *[4]byte) error {
	b0 := byte(op)
	if fin {
		b0 |= 0x80
	}
	hdr := []byte{b0, 0}
	n := len(payload)
	switch {
	case n < 126:
		hdr[1] = byte(n)
	case n <= 0xffff:
		hdr[1] = 126
		hdr = binary.BigEndian.AppendUint16(hdr, uint16(n))
	default:
		hdr[1] = 127
		hdr = binary.BigEndian.AppendUint64(hdr, uint64(n))
	}
	body := payload
	if mask != nil {
		hdr[1] |= 0x80
		hdr = append(hdr, mask[:]...)
		body = make([]byte, n)
		for i := range payload {
			body[i] = payload[i] ^ mask[i%4]
		}
	}
	if _, err := w.Write(hdr); err != nil {
		return err
	}
	_, err := w.Write(body)
	return err
}

// readFrame reads one frame. wantMasked is false on the client side
// (servers must not mask). limit bounds the payload size.
func readFrame(r *bufio.Reader, limit int64, wantMasked bool) (frame, error) {
	var h [2]byte
	if _, err := io.ReadFull(r, h[:]); err != nil {
		return frame{}, err
	}
	f := frame{fin: h[0]&0x80 != 0, op: opcode(h[0] & 0x0f)}
	if h[0]&0x70 != 0 {
		return frame{}, fmt.Errorf("%w: reserved bits set", ErrProtocol)
	}
	switch f.op {
	case opContinuation, opText, opBinary, opClose, opPing, opPong:
	default:
		return frame{}, fmt.Errorf("%w: opcode %#x", ErrProtocol, f.op)
	}
	if masked := h[1]&0x80 != 0; masked != wantMasked {
		return frame{}, fmt.Errorf("%w: masked=%v", ErrProtocol, masked)
	}
	n := int64(h[1] & 0x7f)
	switch n {
	case 126:
		var b [2]byte
		if _, err := io.ReadFull(r, b[:]); err != nil {
			return frame{}, unexpected(err)
		}
		n = int64(binary.BigEndian.Uint16(b[:]))
	case 127:
		var b [8]byte
		if _, err := io.ReadFull(r, b[:]); err != nil {
			return frame{}, unexpected(err)
		}
		u := binary.BigEndian.Uint64(b[:])
		if u>>63 != 0 {
			return frame{}, fmt.Errorf("%w: 64-bit length with the top bit set", ErrProtocol)
		}
		n = int64(u)
	}
	if f.op.control() && (!f.fin || n > 125) {
		return frame{}, fmt.Errorf("%w: control frame fragmented or over 125 bytes", ErrProtocol)
	}
	if n > limit {
		return frame{}, fmt.Errorf("%w: %d-byte frame over the %d-byte limit", ErrProtocol, n, limit)
	}
	var mask [4]byte
	if wantMasked {
		if _, err := io.ReadFull(r, mask[:]); err != nil {
			return frame{}, unexpected(err)
		}
	}
	f.payload = make([]byte, n)
	if _, err := io.ReadFull(r, f.payload); err != nil {
		return frame{}, unexpected(err)
	}
	if wantMasked {
		for i := range f.payload {
			f.payload[i] ^= mask[i%4]
		}
	}
	return f, nil
}

func unexpected(err error) error {
	if errors.Is(err, io.EOF) {
		return io.ErrUnexpectedEOF
	}
	return err
}

// Options configure Dial.
type Options struct {
	Header     http.Header // extra handshake headers (authorization, user agent)
	Proxy      *url.URL    // HTTP CONNECT proxy; nil dials directly
	TLSConfig  *tls.Config // for wss; ServerName is set from the URL
	MaxMessage int64       // largest message accepted; 0 means 16 MiB
}

// Conn is a client WebSocket connection. Read and Write may be used from
// different goroutines; neither may be used concurrently with itself.
// Close may be called at any time, including during a Read.
type Conn struct {
	r          *bufio.Reader
	w          io.Writer
	closeFn    func() error
	maxMessage int64

	wmu    sync.Mutex
	closed bool

	readMu   sync.Mutex // held while reading frames, by Read or by Close
	doneOnce sync.Once
	done     chan struct{} // closed when the reader sees the end of the connection
}

// readDone returns the channel closed once a reader has seen the server's
// close frame or the end of the transport.
func (c *Conn) readDone() chan struct{} {
	c.doneOnce.Do(func() { c.done = make(chan struct{}) })
	return c.done
}

func (c *Conn) markDone() {
	ch := c.readDone()
	select {
	case <-ch:
	default:
		close(ch)
	}
}

// EnvProxy returns the proxy for rawURL from the environment; see
// tunnel.EnvProxy.
func EnvProxy(rawURL string) (*url.URL, error) { return tunnel.EnvProxy(rawURL) }

// Dial opens a WebSocket connection to a ws:// or wss:// URL.
func Dial(ctx context.Context, rawURL string, opts Options) (*Conn, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	secure := u.Scheme == "wss"
	if !secure && u.Scheme != "ws" {
		return nil, fmt.Errorf("websocket: unsupported scheme %q", u.Scheme)
	}
	host := u.Host
	if u.Port() == "" {
		host = net.JoinHostPort(u.Hostname(), map[bool]string{true: "443", false: "80"}[secure])
	}

	raw, err := tunnel.Dial(ctx, host, opts.Proxy)
	if err != nil {
		return nil, err
	}
	ok := false
	defer func() {
		if !ok {
			raw.Close()
		}
	}()
	if d, has := ctx.Deadline(); has {
		raw.SetDeadline(d)
	}
	conn := raw
	if secure {
		cfg := &tls.Config{}
		if opts.TLSConfig != nil {
			cfg = opts.TLSConfig.Clone()
		}
		cfg.ServerName = u.Hostname()
		tc := tls.Client(raw, cfg)
		if err := tc.HandshakeContext(ctx); err != nil {
			return nil, fmt.Errorf("websocket: TLS: %w", err)
		}
		conn = tc
	}

	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, err
	}
	key := base64.StdEncoding.EncodeToString(nonce[:])
	req := &http.Request{
		Method: http.MethodGet,
		URL:    &url.URL{Path: u.Path, RawQuery: u.RawQuery},
		Host:   u.Host,
		Header: http.Header{},
	}
	for k, vs := range opts.Header {
		req.Header[k] = append([]string(nil), vs...)
	}
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Sec-WebSocket-Key", key)
	req.Header.Set("Sec-WebSocket-Version", "13")
	if err := req.Write(conn); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrHandshake, err)
	}
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrHandshake, err)
	}
	if resp.StatusCode != http.StatusSwitchingProtocols {
		return nil, fmt.Errorf("%w: status %s", ErrHandshake, resp.Status)
	}
	if !strings.EqualFold(resp.Header.Get("Upgrade"), "websocket") ||
		!strings.EqualFold(resp.Header.Get("Connection"), "Upgrade") ||
		resp.Header.Get("Sec-WebSocket-Accept") != acceptKey(key) {
		return nil, fmt.Errorf("%w: bad upgrade response", ErrHandshake)
	}
	conn.SetDeadline(time.Time{})
	ok = true
	limit := opts.MaxMessage
	if limit == 0 {
		limit = 16 << 20
	}
	return &Conn{r: br, w: conn, closeFn: conn.Close, maxMessage: limit}, nil
}

func (c *Conn) writeControl(op opcode, payload []byte) error {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	return c.writeLocked(op, payload)
}

func (c *Conn) writeLocked(op opcode, payload []byte) error {
	var mask [4]byte
	if _, err := rand.Read(mask[:]); err != nil {
		return err
	}
	return writeFrame(c.w, true, op, payload, &mask)
}

// Write sends one binary message.
func (c *Conn) Write(msg []byte) error {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	if c.closed {
		return ErrClosed
	}
	return c.writeLocked(opBinary, msg)
}

// Read returns the next data message, answering pings on the way. A close
// from the server is answered and reported as a *CloseError (matching
// ErrClosed).
func (c *Conn) Read() ([]byte, error) {
	c.readMu.Lock()
	defer c.readMu.Unlock()
	var msg []byte
	inMessage := false
	for {
		f, err := readFrame(c.r, c.maxMessage, false)
		if err != nil {
			c.wmu.Lock()
			closed := c.closed
			c.wmu.Unlock()
			if closed {
				c.markDone()
				return nil, ErrClosed
			}
			return nil, err
		}
		switch f.op {
		case opPing:
			if err := c.writeControl(opPong, f.payload); err != nil {
				return nil, err
			}
			continue
		case opPong:
			continue
		case opClose:
			c.wmu.Lock()
			if !c.closed {
				c.closed = true
				_ = c.writeLocked(opClose, f.payload)
			}
			c.wmu.Unlock()
			c.markDone()
			c.closeFn()
			ce := &CloseError{}
			if len(f.payload) >= 2 {
				ce.Code = int(binary.BigEndian.Uint16(f.payload))
				ce.Reason = string(f.payload[2:])
			}
			return nil, ce
		case opText, opBinary:
			if inMessage {
				return nil, fmt.Errorf("%w: new message inside a fragmented one", ErrProtocol)
			}
			inMessage, msg = true, f.payload
		case opContinuation:
			if !inMessage {
				return nil, fmt.Errorf("%w: continuation without a message", ErrProtocol)
			}
			if int64(len(msg)+len(f.payload)) > c.maxMessage {
				return nil, fmt.Errorf("%w: message over the size limit", ErrProtocol)
			}
			msg = append(msg, f.payload...)
		}
		if f.fin {
			return msg, nil
		}
	}
}

// Close sends a normal-closure frame, waits briefly for the server's
// close, and closes the connection. If a Read is in progress, that Read
// sees the server's close; otherwise Close reads for it.
func (c *Conn) Close() error {
	c.wmu.Lock()
	if c.closed {
		c.wmu.Unlock()
		return nil
	}
	c.closed = true
	err := c.writeLocked(opClose, []byte{0x03, 0xe8}) // 1000: normal closure
	c.wmu.Unlock()
	if err == nil {
		if c.readMu.TryLock() {
			// No reader: read until the server's close. Closing the
			// transport below unblocks this if it never comes.
			go func() {
				defer c.readMu.Unlock()
				for {
					f, rerr := readFrame(c.r, c.maxMessage, false)
					if rerr != nil || f.op == opClose {
						c.markDone()
						return
					}
				}
			}()
		}
		select {
		case <-c.readDone():
		case <-time.After(2 * time.Second):
		}
	}
	if cerr := c.closeFn(); err == nil {
		err = cerr
	}
	return err
}
