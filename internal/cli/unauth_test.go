// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/aren55555/signal-cli-go/internal/service/chat"
)

type fakeUnauth struct {
	mu      sync.Mutex
	err     error
	hold    chan struct{} // if set, Request waits for it
	entered chan struct{} // if set, Request signals it on entry
}

func (f *fakeUnauth) Request(ctx context.Context, verb, path string, _ []chat.Header, _ []byte) (*chat.Response, error) {
	if f.entered != nil {
		f.entered <- struct{}{}
	}
	if f.hold != nil {
		<-f.hold
	}
	if err := f.Err(); err != nil {
		return nil, err
	}
	return &chat.Response{Status: 200}, nil
}

func (f *fakeUnauth) Err() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.err
}

func (f *fakeUnauth) Close() error { f.end(chat.ErrClosed); return nil }

func (f *fakeUnauth) end(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err == nil {
		f.err = err
	}
}

type dialer struct {
	mu    sync.Mutex
	conns []*fakeUnauth
	fail  bool
	next  func() *fakeUnauth
}

func (d *dialer) dial(context.Context) (unauthConn, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.fail {
		return nil, errors.New("dial failed")
	}
	c := &fakeUnauth{}
	if d.next != nil {
		c = d.next()
	}
	d.conns = append(d.conns, c)
	return c, nil
}

func (d *dialer) count() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.conns)
}

func (d *dialer) last() *fakeUnauth {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.conns[len(d.conns)-1]
}

func request(t *testing.T, u *unauthSocket) {
	t.Helper()
	if r, err := u.Request(context.Background(), "PUT", "/v1/messages/x", nil, nil); err != nil || r.Status != 200 {
		t.Fatalf("request: %v %v", r, err)
	}
}

// TestUnauthRedialsAfterServerClose: production closes an idle
// unauthenticated socket (1001 "Connection Idle Timeout"); the next send
// must go out on a new connection, not fail on the dead one.
func TestUnauthRedialsAfterServerClose(t *testing.T) {
	d := &dialer{}
	u := newUnauthSocket(context.Background(), d.dial)
	defer u.Close()
	request(t, u)
	d.last().end(errors.New(`websocket: closed by server with code 1001 "Connection Idle Timeout"`))
	request(t, u)
	request(t, u)
	if n := d.count(); n != 2 {
		t.Errorf("dialed %d connections, want 2", n)
	}
}

// TestUnauthClosesWhenIdle: like signal-cli, the socket closes 30 s after
// its last request and the next request opens a new one.
func TestUnauthClosesWhenIdle(t *testing.T) {
	d := &dialer{}
	u := newUnauthSocket(context.Background(), d.dial)
	defer u.Close()
	if u.idle != 30*time.Second {
		t.Errorf("idle timeout %v, want 30s (pipe.md §5.2)", u.idle)
	}
	u.mu.Lock()
	u.idle = 20 * time.Millisecond
	u.armLocked()
	u.mu.Unlock()
	request(t, u)
	first := d.last()
	deadline := time.Now().Add(2 * time.Second)
	for first.Err() == nil && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if first.Err() == nil {
		t.Fatal("idle socket was not closed")
	}
	request(t, u)
	if n := d.count(); n != 2 {
		t.Errorf("dialed %d connections, want 2", n)
	}
}

// TestUnauthKeepsBusySocket: a request that runs longer than the idle
// timeout keeps its connection.
func TestUnauthKeepsBusySocket(t *testing.T) {
	hold, entered := make(chan struct{}), make(chan struct{}, 1)
	d := &dialer{}
	u := newUnauthSocket(context.Background(), d.dial)
	defer u.Close()
	d.last().hold, d.last().entered = hold, entered
	u.mu.Lock()
	u.idle = 10 * time.Millisecond
	u.armLocked()
	u.mu.Unlock()
	done := make(chan error, 1)
	go func() {
		_, err := u.Request(context.Background(), "GET", "/v1/profile/x", nil, nil)
		done <- err
	}()
	<-entered
	time.Sleep(50 * time.Millisecond)
	close(hold)
	if err := <-done; err != nil {
		t.Fatalf("busy request failed: %v", err)
	}
}

func TestUnauthFirstDialAndClose(t *testing.T) {
	if u := newUnauthSocket(context.Background(), (&dialer{fail: true}).dial); u != nil {
		t.Error("a failed first dial must give no socket (unsealed fallback)")
	}
	d := &dialer{}
	u := newUnauthSocket(context.Background(), d.dial)
	u.Close()
	if _, err := u.Request(context.Background(), "GET", "/", nil, nil); !errors.Is(err, chat.ErrClosed) {
		t.Errorf("request after Close: %v", err)
	}
	if d.count() != 1 || d.last().Err() == nil {
		t.Error("Close must close the connection and not dial again")
	}
}
