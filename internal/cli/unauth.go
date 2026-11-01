// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"context"
	"sync"
	"time"

	"github.com/aren55555/signal-cli-go/internal/service/chat"
)

// unauthIdle is how long the unauthenticated socket stays open after its
// last request (pipe.md §5.2: disconnect timeout 30000 ms).
const unauthIdle = 30 * time.Second

// unauthConn is what unauthSocket needs from a chat connection.
type unauthConn interface {
	Request(ctx context.Context, verb, path string, headers []chat.Header, body []byte) (*chat.Response, error)
	Err() error
	Close() error
}

// unauthSocket is the unauthenticated chat socket as signal-cli runs it
// (pipe.md §5.3): opened when a request needs it, closed 30 s after its
// last use, and opened again by the next request. A long-running daemon
// or receive therefore never sends on a socket the server closed (the
// production server closes idle sockets: 1001 "Connection Idle Timeout").
type unauthSocket struct {
	dial func(context.Context) (unauthConn, error)
	idle time.Duration

	mu       sync.Mutex
	c        unauthConn
	inflight int
	timer    *time.Timer
	closed   bool
}

// newUnauthSocket dials the first connection now, so that a client
// without one keeps falling back to unsealed sends; nil if that fails.
func newUnauthSocket(ctx context.Context, dial func(context.Context) (unauthConn, error)) *unauthSocket {
	c, err := dial(ctx)
	if err != nil {
		return nil
	}
	u := &unauthSocket{dial: dial, idle: unauthIdle, c: c}
	u.mu.Lock()
	u.armLocked()
	u.mu.Unlock()
	return u
}

// Request sends one request, dialing a new connection if there is none
// or the last one ended.
func (u *unauthSocket) Request(ctx context.Context, verb, path string, headers []chat.Header, body []byte) (*chat.Response, error) {
	u.mu.Lock()
	if u.closed {
		u.mu.Unlock()
		return nil, chat.ErrClosed
	}
	if u.c == nil || u.c.Err() != nil {
		if u.c != nil {
			u.c.Close()
		}
		c, err := u.dial(ctx)
		if err != nil {
			u.c = nil
			u.mu.Unlock()
			return nil, err
		}
		u.c = c
	}
	c := u.c
	u.inflight++
	if u.timer != nil {
		u.timer.Stop()
	}
	u.mu.Unlock()

	r, err := c.Request(ctx, verb, path, headers, body)

	u.mu.Lock()
	u.inflight--
	u.armLocked()
	u.mu.Unlock()
	return r, err
}

// armLocked starts the idle timer when no request is in flight.
func (u *unauthSocket) armLocked() {
	if u.inflight > 0 || u.closed || u.c == nil {
		return
	}
	c := u.c
	if u.timer != nil {
		u.timer.Stop()
	}
	u.timer = time.AfterFunc(u.idle, func() {
		u.mu.Lock()
		defer u.mu.Unlock()
		if u.c == c && u.inflight == 0 {
			c.Close()
			u.c = nil
		}
	})
}

// Close closes the socket for good.
func (u *unauthSocket) Close() error {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.closed = true
	if u.timer != nil {
		u.timer.Stop()
	}
	if u.c != nil {
		u.c.Close()
		u.c = nil
	}
	return nil
}
