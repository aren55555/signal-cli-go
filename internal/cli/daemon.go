// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/aren55555/signal-cli-go/internal/cli/argparse"
	"github.com/aren55555/signal-cli-go/internal/daemon"
	"github.com/aren55555/signal-cli-go/internal/errcode"
	"github.com/aren55555/signal-cli-go/internal/manager"
	"github.com/aren55555/signal-cli-go/internal/store"
)

// daemonState is a running daemon or jsonRpc session: the open accounts
// it serves and the JSON-RPC server (spec/surface/cli/daemon.md,
// jsonrpc/transports.md).
type daemonState struct {
	e    *env     // the process environment (stdout printer, stderr log)
	g    *globals // global options; account is the single account or ""
	srv  *daemon.Server
	opts receiveOpts
	// mode is on-start, on-connection or manual.
	mode string
	// printStdout prints received messages to stdout (daemon without
	// --no-receive-stdout).
	printStdout bool
	ctx         context.Context
	// cancel shuts the daemon down (single-account mode, after unregister).
	cancel context.CancelFunc
	// started is set once the initial accounts are open; later ones
	// are announced to subscribed connections.
	started bool

	mu       sync.Mutex
	accounts []*liveAccount
	links    map[string]*manager.PendingLink // startLink sessions by URI
}

// liveAccount is an account the daemon keeps open. mu is held by a
// command for its whole run and by the receive loop while it handles an
// envelope.
type liveAccount struct {
	mu        sync.Mutex
	acct      *store.Account
	number    string
	sess      *session // the shared authenticated connection; nil until needed
	receiving bool
}

func (d *daemonState) logf(format string, args ...any) {
	fmt.Fprintf(d.e.stderr, format+"\n", args...)
}

// account returns the open account with this number or ACI.
func (d *daemonState) account(id string) *liveAccount {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, la := range d.accounts {
		if la.number == id || (la.acct.Data.ACI != "" && la.acct.Data.ACI == id) {
			return la
		}
	}
	return nil
}

func (d *daemonState) byAccount(a *store.Account) *liveAccount {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, la := range d.accounts {
		if la.acct == a {
			return la
		}
	}
	return nil
}

// list is the server's account list, in load order.
func (d *daemonState) list() []daemon.Account {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]daemon.Account, 0, len(d.accounts))
	for _, la := range d.accounts {
		out = append(out, daemon.Account{Number: la.number, ACI: la.acct.Data.ACI})
	}
	return out
}

// loadAccounts opens every registered local account not open yet
// (multi-account mode), or the single account. New accounts start
// receiving in on-start mode.
func (d *daemonState) loadAccounts() error {
	if d.g.account == "" {
		if err := os.MkdirAll(filepath.Join(d.g.dataDir, "data"), 0o700); err != nil {
			return errcode.AccountsListRead.Wrap(err)
		}
	}
	root := d.g.root()
	entries, err := root.Accounts()
	if err != nil {
		return errcode.AccountsFileLoad.Wrap(err)
	}
	for _, en := range entries {
		if d.g.account != "" && en.Number != d.g.account && en.UUID != d.g.account {
			continue
		}
		if d.account(en.Number) != nil {
			continue
		}
		a, err := root.Load(en.Path)
		if err != nil || !a.Data.Registered {
			if a != nil {
				a.Close()
			}
			if d.g.account == "" {
				d.logf("WARN  MultiAccountManager - Ignoring %s: User is not registered. (NotRegisteredException)", en.Number)
			}
			continue
		}
		la := &liveAccount{acct: a, number: a.Data.Number}
		d.mu.Lock()
		d.accounts = append(d.accounts, la)
		d.mu.Unlock()
		if d.started {
			d.srv.AccountAdded(la.number)
		}
		if d.mode == "on-start" {
			d.startReceiving(la)
		}
	}
	return nil
}

func (d *daemonState) closeAccounts() {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, la := range d.accounts {
		la.mu.Lock()
		if la.sess != nil {
			la.sess.closeConns()
		}
		la.acct.Close()
		la.mu.Unlock()
	}
	d.accounts = nil
}

// forget drops an account whose local data was deleted.
func (d *daemonState) forget(la *liveAccount) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for i, x := range d.accounts {
		if x == la {
			d.accounts = append(d.accounts[:i:i], d.accounts[i+1:]...)
		}
	}
}

// sessionLocked returns the account's shared session, dialing it if
// needed; la.mu is held.
func (d *daemonState) sessionLocked(ctx context.Context, la *liveAccount) (*session, error) {
	if la.sess != nil && la.sess.chat.Err() == nil {
		return la.sess, nil
	}
	if la.sess != nil {
		la.sess.closeConns()
		la.sess = nil
	}
	g := *d.g
	g.account = la.number
	g.daemon = nil
	s, err := openSessionWith(context.WithoutCancel(ctx), d.e, &g, la.acct, true, !d.opts.ignoreStories)
	if err != nil {
		return nil, err
	}
	la.sess = s
	return s, nil
}

// startReceiving runs the account's receive loop in the background
// (pipe.md §7), reconnecting with a growing delay.
func (d *daemonState) startReceiving(la *liveAccount) {
	la.mu.Lock()
	if la.receiving {
		la.mu.Unlock()
		return
	}
	la.receiving = true
	la.mu.Unlock()
	go func() {
		ctx := d.ctx
		delay := time.Second
		for ctx.Err() == nil {
			la.mu.Lock()
			s, err := d.sessionLocked(ctx, la)
			la.mu.Unlock()
			if err != nil {
				d.logf("WARN  ReceiveHelper - Connection closed unexpectedly, reconnecting in %d s", int(delay.Seconds()))
				sleepCtx(ctx, delay)
				delay = min(2*delay, time.Minute)
				continue
			}
			n, _ := d.e.network(d.g)
			g := *d.g
			g.account = la.number
			r := newReceiver(d.e, &g, n, la.acct, s.chat, s.sender.Unauth, d.opts, func(rc *RenderContext, ev manager.Event) {
				d.publish(la, rc, ev)
			})
			r.Lock = &la.mu
			if err := r.RetryCache(ctx); err != nil {
				d.logf("WARN  ReceiveHelper - %v", err)
			}
			err = r.Run(ctx, s.chat, 0, 0)
			if ctx.Err() != nil {
				return
			}
			la.mu.Lock()
			if la.sess == s {
				la.sess.closeConns()
				la.sess = nil
			}
			la.mu.Unlock()
			if err != nil {
				d.logf("WARN  ReceiveHelper - Connection closed unexpectedly, reconnecting in %d s", int(delay.Seconds()))
			}
			sleepCtx(ctx, delay)
			delay = min(2*delay, time.Minute)
		}
	}()
}

func sleepCtx(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
	case <-ctx.Done():
	}
}

// publish delivers one received envelope to JSON-RPC subscribers, events
// streams and the stdout printer.
func (d *daemonState) publish(la *liveAccount, rc *RenderContext, ev manager.Event) {
	payload := RenderJSON(rc, ev)
	d.srv.Publish(false, la.number, payload)
	if d.printStdout {
		if d.g.json() {
			fmt.Fprintln(d.e.stdout, payload)
		} else {
			io.WriteString(d.e.stdout, RenderPlain(rc, ev))
		}
	}
}

// onSubscribe starts receiving when a client subscribes (receive modes
// on-connection and manual).
func (d *daemonState) onSubscribe(account string) {
	if d.mode == "on-start" {
		return
	}
	d.mu.Lock()
	las := append([]*liveAccount(nil), d.accounts...)
	d.mu.Unlock()
	for _, la := range las {
		if account == "" || la.number == account {
			d.startReceiving(la)
		}
	}
}

func newDaemonState(ctx context.Context, e *env, g *globals, s *argparse.Result) *daemonState {
	d := &daemonState{e: e, g: g, ctx: ctx, mode: s.String("receive-mode"), links: map[string]*manager.PendingLink{},
		opts: receiveOpts{ignoreAttachments: s.Bool("ignore_attachments"), ignoreStories: s.Bool("ignore_stories"),
			ignoreAvatars: s.Bool("ignore_avatars"), ignoreStickers: s.Bool("ignore_stickers"), sendReadReceipts: s.Bool("send_read_receipts")}}
	if d.mode == "" {
		d.mode = "on-start"
	}
	d.srv = &daemon.Server{Accounts: d.list, SingleAccount: g.account, AutoSubscribe: d.mode != "manual", Log: e.stderr}
	d.srv.Methods = d.methods()
	return d
}

// start opens the accounts; in single-account mode the account must
// exist and be registered.
func (d *daemonState) start() error {
	if d.g.account != "" {
		a, err := selectAccount(d.g.root(), d.g.account)
		if err != nil {
			return err
		}
		a.Close()
		d.g.account = a.Data.Number
		d.srv.SingleAccount = a.Data.Number
	}
	err := d.loadAccounts()
	d.started = true
	return err
}

// runJSONRPC is `jsonRpc`: one JSON-RPC session over stdin and stdout
// (jsonRpc.md, transports.md §3).
func runJSONRPC(ctx context.Context, e *env, g *globals, s *argparse.Result) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	d := newDaemonState(ctx, e, g, s)
	d.cancel = cancel
	if err := d.start(); err != nil {
		return err
	}
	defer d.closeAccounts()
	numberResolver = d.resolver()
	done := make(chan error, 1)
	go func() { done <- d.srv.ServeStream(ctx, e.stdin, e.stdout) }()
	var err error
	select {
	case err = <-done:
	case <-ctx.Done():
	}
	var crash *daemon.ErrReaderCrashed
	if errors.As(err, &crash) {
		return crash.Err
	}
	return nil
}

// runDaemon is `daemon` (daemon.md).
func runDaemon(parent context.Context, e *env, g *globals, s *argparse.Result) error {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	d := newDaemonState(ctx, e, g, s)
	d.cancel = cancel
	d.printStdout = !s.Bool("no_receive_stdout")
	if err := d.start(); err != nil {
		return err
	}
	defer d.closeAccounts()
	if g.account != "" {
		d.logf("INFO  DaemonCommand - Starting daemon in single-account mode for %s", g.account)
	} else {
		d.logf("INFO  DaemonCommand - Starting daemon in multi-account mode")
	}
	numberResolver = d.resolver()
	var closers []func()
	defer func() {
		for i := len(closers) - 1; i >= 0; i-- {
			closers[i]()
		}
	}()
	channels := 0
	serveStreams := func(ln net.Listener) {
		go func() {
			for {
				c, err := ln.Accept()
				if err != nil {
					return
				}
				go func() {
					defer c.Close()
					d.srv.ServeStream(ctx, c, c)
				}()
			}
		}()
	}
	if s.Has("socket") {
		path := s.String("socket")
		if path == "" {
			path = defaultSocketPath(e.getenv)
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return errcode.DaemonBindFailed.Wrap(err, path, javaIOMessage(err))
		}
		ln, err := net.Listen("unix", path)
		if err != nil {
			return errcode.DaemonBindFailed.Wrap(err, path, javaIOMessage(err))
		}
		closers = append(closers, func() { ln.Close() })
		serveStreams(ln)
		channels++
	}
	if s.Has("tcp") {
		addr, shown, err := bindAddress(s.String("tcp"))
		if err != nil {
			return err
		}
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			return errcode.DaemonBindFailed.Wrap(err, shown, javaIOMessage(err))
		}
		closers = append(closers, func() { ln.Close() })
		serveStreams(ln)
		channels++
	}
	if s.Has("http") {
		addr, shown, err := bindAddress(s.String("http"))
		if err != nil {
			return err
		}
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			return errcode.DaemonHTTPInit.Wrap(err)
		}
		host, portStr, _ := net.SplitHostPort(addr)
		port, _ := strconv.Atoi(portStr)
		h := &daemon.HTTP{S: d.srv, BindHost: host, Port: port}
		closers = append(closers, func() { ln.Close(); h.Close() })
		go h.Serve(ctx, ln)
		d.logf("INFO  HttpServerHandler - Started HTTP server on %s", shown)
		switch host {
		case "0.0.0.0", "::", "":
			d.logf("WARN  HttpServerHandler - HTTP server has no authentication and listens on all interfaces")
		default:
			d.logf("WARN  HttpServerHandler - HTTP server has no authentication; Host header is pinned to [localhost, ::1, 127.0.0.1]")
		}
		channels++
	}
	if channels == 0 {
		return errcode.DaemonNoChannel
	}
	<-ctx.Done()
	if parent.Err() != nil {
		d.logf("INFO  Shutdown - Received SIGTERM signal, shutting down ...")
	}
	return nil
}

// defaultSocketPath is $XDG_RUNTIME_DIR/signal-cli/socket, else the
// temp dir's signal-cli/socket.
func defaultSocketPath(getenv func(string) string) string {
	if x := getenv("XDG_RUNTIME_DIR"); x != "" {
		return filepath.Join(x, "signal-cli", "socket")
	}
	return filepath.Join(os.TempDir(), "signal-cli", "socket")
}

// bindAddress parses HOST:PORT (split at the last colon) and returns the
// listen address and Java's InetSocketAddress rendering for messages.
func bindAddress(v string) (addr, shown string, err error) {
	i := strings.LastIndex(v, ":")
	if i < 0 {
		return "", "", errcode.DaemonBadAddress.New(v)
	}
	host, portStr := v[:i], v[i+1:]
	port, perr := strconv.Atoi(portStr)
	if perr != nil {
		return "", "", errcode.DaemonBadPort.New(portStr)
	}
	ips, lerr := net.LookupIP(strings.Trim(host, "[]"))
	if lerr != nil || len(ips) == 0 {
		return "", "", errcode.DaemonBadHost.Wrap(lerr, host)
	}
	ip := ips[0]
	for _, x := range ips {
		if x.To4() != nil {
			ip = x
			break
		}
	}
	name := ""
	if net.ParseIP(strings.Trim(host, "[]")) == nil {
		name = host
	}
	return net.JoinHostPort(ip.String(), strconv.Itoa(port)), name + "/" + ip.String() + ":" + strconv.Itoa(port), nil
}

// javaIOMessage is the JDK's text for a socket error.
func javaIOMessage(err error) string {
	var errno syscall.Errno
	if errors.As(err, &errno) {
		s := errno.Error()
		if s != "" {
			return strings.ToUpper(s[:1]) + s[1:]
		}
	}
	return err.Error()
}

// resolver is numberResolver for the daemon: CDSI over the account's
// shared connection (the caller holds the account).
func (d *daemonState) resolver() func(a *store.Account, number string) (string, bool) {
	return func(a *store.Account, number string) (string, bool) {
		la := d.byAccount(a)
		if la == nil || la.sess == nil {
			return "", false
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		return la.sess.sender.ResolveNumber(ctx, number)
	}
}

// unregistered closes an account after a successful unregister: the
// single-account daemon shuts down, a multi-account daemon drops it
// (methods/unregister.md).
func (d *daemonState) unregistered(number string) {
	if d.g.account != "" {
		// Shut down once the unregister response has been written.
		time.AfterFunc(200*time.Millisecond, d.cancel)
		return
	}
	la := d.account(number)
	if la == nil {
		return
	}
	la.mu.Lock()
	if la.sess != nil {
		la.sess.closeConns()
		la.sess = nil
	}
	la.acct.Close()
	la.mu.Unlock()
	d.forget(la)
}
