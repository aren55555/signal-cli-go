// SPDX-License-Identifier: GPL-3.0-or-later

package daemon

import (
	"bufio"
	"context"
	"errors"
	"io"
	"strings"
	"sync"

	"github.com/aren55555/signal-cli-go/internal/errcode"
	"github.com/aren55555/signal-cli-go/internal/jackson"
)

// sub is one receive or call-event subscription of a connection.
type sub struct {
	id       int
	implicit bool
	account  string // "" covers every account
}

// Conn is one stream connection (a JSON-RPC session).
type Conn struct {
	s   *Server
	w   io.Writer
	wmu sync.Mutex

	mu       sync.Mutex
	closed   bool
	receive  []*sub
	call     []*sub
	inFlight sync.WaitGroup
}

// write sends one line; a failed write ends the connection's
// subscriptions.
func (c *Conn) write(line string) error {
	c.wmu.Lock()
	_, err := io.WriteString(c.w, line+"\n")
	c.wmu.Unlock()
	if err != nil {
		c.drop()
	}
	return err
}

// drop removes the connection's subscriptions.
func (c *Conn) drop() {
	c.mu.Lock()
	c.closed = true
	c.receive, c.call = nil, nil
	c.mu.Unlock()
	c.s.mu.Lock()
	delete(c.s.conns, c)
	c.s.mu.Unlock()
}

// Subscribe adds a receive (or call-event) subscription and returns its
// id from the process-wide counter.
func (c *Conn) Subscribe(callEvents bool, account string) int {
	sb := &sub{id: c.s.newSubID(), account: account}
	c.mu.Lock()
	if callEvents {
		c.call = append(c.call, sb)
	} else {
		c.receive = append(c.receive, sb)
	}
	c.mu.Unlock()
	return sb.id
}

// Unsubscribe removes subscription id; implicitOnly removes the
// connection's implicit subscription instead. It reports whether one
// was found.
func (c *Conn) Unsubscribe(callEvents bool, id int, implicitOnly bool) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	list := &c.receive
	if callEvents {
		list = &c.call
	}
	for i, sb := range *list {
		if (implicitOnly && sb.implicit) || (!implicitOnly && sb.id == id) {
			*list = append((*list)[:i:i], (*list)[i+1:]...)
			return true
		}
	}
	return false
}

// notify delivers a notification payload to the matching subscriptions.
func (c *Conn) notify(callEvents bool, account, payload string) {
	c.mu.Lock()
	list := c.receive
	method := "receive"
	if callEvents {
		list, method = c.call, "callEvent"
	}
	var lines []string
	for _, sb := range list {
		if sb.account != "" && sb.account != account {
			continue
		}
		if sb.implicit {
			lines = append(lines, `{"jsonrpc":"2.0","method":"`+method+`","params":`+payload+`}`)
		} else {
			lines = append(lines, `{"jsonrpc":"2.0","method":"`+method+`","params":{"subscription":`+itoa(sb.id)+`,"result":`+payload+`}}`)
		}
	}
	c.mu.Unlock()
	for _, l := range lines {
		if c.write(l) != nil {
			return
		}
	}
}

// listener is a non-JSON-RPC consumer of received messages (the
// events stream, the stdout printer).
type listener struct {
	accounts map[string]bool // nil: all
	fn       func(account, payload string)
}

// AddListener registers fn for received messages of accounts (nil: all
// accounts, including later ones). The returned function removes it.
func (s *Server) AddListener(accounts []string, fn func(account, payload string)) func() {
	l := &listener{fn: fn}
	if accounts != nil {
		l.accounts = map[string]bool{}
		for _, a := range accounts {
			l.accounts[a] = true
		}
	}
	s.mu.Lock()
	if s.listeners == nil {
		s.listeners = map[*listener]bool{}
	}
	s.listeners[l] = true
	s.mu.Unlock()
	return func() {
		s.mu.Lock()
		delete(s.listeners, l)
		s.mu.Unlock()
	}
}

// Publish delivers a received message (or, with callEvents, a call
// event) for account to every subscriber. payload is the encoded
// notification object.
func (s *Server) Publish(callEvents bool, account, payload string) {
	s.mu.Lock()
	conns := make([]*Conn, 0, len(s.conns))
	for c := range s.conns {
		conns = append(conns, c)
	}
	var ls []*listener
	if !callEvents {
		for l := range s.listeners {
			if l.accounts == nil || l.accounts[account] {
				ls = append(ls, l)
			}
		}
	}
	s.mu.Unlock()
	for _, c := range conns {
		c.notify(callEvents, account, payload)
	}
	for _, l := range ls {
		l.fn(account, payload)
	}
}

// HasReceivers reports whether any connection or listener would get
// received messages of account.
func (s *Server) HasReceivers(account string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for c := range s.conns {
		c.mu.Lock()
		for _, sb := range c.receive {
			if sb.account == "" || sb.account == account {
				c.mu.Unlock()
				return true
			}
		}
		c.mu.Unlock()
	}
	for l := range s.listeners {
		if l.accounts == nil || l.accounts[account] {
			return true
		}
	}
	return false
}

// ErrReaderCrashed ends a stream whose reader thread would have died
// in the reference (input nested too deeply); for stdio the process
// exits with status 2 after printing Message.
type ErrReaderCrashed struct{ Err error }

func (e *ErrReaderCrashed) Error() string { return errcode.Message(e.Err) }
func (e *ErrReaderCrashed) Unwrap() error { return e.Err }

// ServeStream runs a JSON-RPC session over line-delimited text. It
// returns when the input ends and the requests in flight are done.
func (s *Server) ServeStream(ctx context.Context, r io.Reader, w io.Writer) error {
	c := &Conn{s: s, w: w}
	s.mu.Lock()
	if s.conns == nil {
		s.conns = map[*Conn]bool{}
	}
	s.conns[c] = true
	s.mu.Unlock()
	if s.AutoSubscribe {
		c.receive = append(c.receive, &sub{id: s.newSubID(), implicit: true})
	}
	defer c.drop()
	lr := &lineReader{r: bufio.NewReader(r)}
	spawn := func(f func()) {
		c.inFlight.Add(1)
		go func() {
			defer c.inFlight.Done()
			f()
		}()
	}
	emit := func(line string) { c.write(line) }
	for {
		line, err := lr.next()
		if err != nil {
			c.inFlight.Wait()
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		if strings.TrimLeft(line, "\x00\x01\x02\x03\x04\x05\x06\x07\x08\t\n\x0b\x0c\r\x0e\x0f\x10\x11\x12\x13\x14\x15\x16\x17\x18\x19\x1a\x1b\x1c\x1d\x1e\x1f ") == "" {
			emit(errorResponse(jackson.NewNull(), errcode.RPCEmptyLine))
			continue
		}
		n, err := jackson.Parse(line, jackson.Chars)
		if err != nil {
			if errors.Is(err, errcode.JSONNestingDepth) {
				c.inFlight.Wait()
				return &ErrReaderCrashed{Err: err}
			}
			emit(errorResponse(jackson.NewNull(), err))
			continue
		}
		s.handleMessage(ctx, c, n, false, spawn, emit)
	}
}

// lineReader splits input at LF, CR or CRLF like Java's
// BufferedReader.readLine; a CR returns the line at once.
type lineReader struct {
	r      *bufio.Reader
	skipLF bool
}

func (l *lineReader) next() (string, error) {
	var b []byte
	for {
		c, err := l.r.ReadByte()
		if err != nil {
			if len(b) > 0 {
				return toValidUTF8(b), nil
			}
			return "", err
		}
		if l.skipLF {
			l.skipLF = false
			if c == '\n' {
				continue
			}
		}
		switch c {
		case '\n':
			return toValidUTF8(b), nil
		case '\r':
			l.skipLF = true
			return toValidUTF8(b), nil
		}
		b = append(b, c)
	}
}

func toValidUTF8(b []byte) string { return strings.ToValidUTF8(string(b), "�") }
