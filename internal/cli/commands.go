package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aren55555/signal-cli-go/internal/manager"
	"github.com/aren55555/signal-cli-go/internal/protocol"
	"github.com/aren55555/signal-cli-go/internal/service/agent"
	"github.com/aren55555/signal-cli-go/internal/service/chat"
	"github.com/aren55555/signal-cli-go/internal/service/provision"
	"github.com/aren55555/signal-cli-go/internal/service/rest"
	"github.com/aren55555/signal-cli-go/internal/store"
)

// env is the process environment a command runs in.
type env struct {
	stdout, stderr io.Writer
	stdin          io.Reader
	getenv         func(string) string
}

// globals are the parsed global options (index.md §2).
type globals struct {
	verbose        int
	dataDir        string
	account        string
	output         string // "plain-text" or "json"
	serviceEnv     string // "live" or "staging"
	trustNew       string
	disableSendLog bool
}

func (g *globals) json() bool { return g.output == "json" }

func (g *globals) root() *store.Root { return store.Open(filepath.Join(g.dataDir, "data")) }

// network is how commands reach Signal.
type network struct {
	proxy     *url.URL
	userAgent string
}

func (e *env) network(g *globals) (*network, error) {
	if g.serviceEnv != "staging" {
		// The brief forbids production; only staging endpoints exist here.
		return nil, &cliError{msg: "The live service environment is not supported by signal-cli-go yet; use --service-environment staging", code: 1}
	}
	n := &network{userAgent: e.getenv(agent.EnvOverride)}
	for _, k := range []string{"HTTPS_PROXY", "https_proxy", "ALL_PROXY", "all_proxy"} {
		if v := e.getenv(k); v != "" {
			u, err := url.Parse(v)
			if err == nil && u.Host == "" {
				u, err = url.Parse("http://" + v)
			}
			if err != nil {
				return nil, &cliError{msg: "Invalid proxy setting: " + err.Error(), code: 1}
			}
			n.proxy = u
			break
		}
	}
	return n, nil
}

func (n *network) dialChat(ctx context.Context, a *store.Account, keepAlive bool) (*chat.Conn, error) {
	ep := chat.Endpoint{Proxy: n.proxy, UserAgentOverride: n.userAgent, ReceiveStories: true}
	if a != nil {
		ep.Credentials = &chat.Credentials{ACI: a.Data.ACI, DeviceID: a.Data.DeviceID, Password: a.Data.Password}
	}
	if keepAlive {
		ep.KeepAlive = chat.KeepAlive
	}
	return chat.Dial(ctx, ep)
}

// ioError is exit code 3 ("Server or IO error").
func ioError(format string, args ...any) error {
	return &cliError{msg: fmt.Sprintf(format, args...), code: 3}
}

// checkAccount runs the start-up account check on the authenticated
// socket (api.md §4.3).
func checkAccount(ctx context.Context, a *store.Account, c *chat.Conn, account string) error {
	err := manager.CheckAccount(ctx, a, c, time.Now())
	switch {
	case errors.Is(err, manager.ErrNotRegistered):
		return userError("User " + account + " is not registered.")
	case err != nil:
		return ioError("Error while checking account %s: %v", account, err)
	}
	return nil
}

// runLink is `link [-n NAME]` (spec/surface/cli/link.md).
func runLink(ctx context.Context, e *env, g *globals, name string) error {
	n, err := e.network(g)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(g.dataDir, "data"), 0o700); err != nil {
		return ioError("Failed to read local accounts list")
	}
	l := &manager.Linker{
		Root: g.root(),
		REST: rest.New(rest.Config{Proxy: n.proxy, UserAgentOverride: n.userAgent}),
		OpenSocket: func(ctx context.Context) (*provision.Session, error) {
			return provision.Open(ctx, provision.Endpoint{Proxy: n.proxy, UserAgentOverride: n.userAgent})
		},
		DeviceName: name,
		OnURL:      func(u string) { fmt.Fprintln(e.stdout, u) },
		PostLink: func(ctx context.Context, a *store.Account) error {
			c, err := n.dialChat(ctx, a, false)
			if err != nil {
				return err
			}
			defer c.Close()
			return manager.CheckAccount(ctx, a, c, time.Now())
		},
	}
	ctx, cancel := context.WithTimeout(ctx, 150*time.Second)
	defer cancel()
	a, err := l.Link(ctx)
	switch {
	case errors.Is(err, manager.ErrUserExists):
		return userError("User " + "already exists")
	case a == nil && err != nil:
		return ioError("Link request error: %v", err)
	}
	defer a.Close()
	if err != nil {
		fmt.Fprintf(e.stderr, "WARN  ProvisioningManagerImpl - %v\n", err)
	}
	fmt.Fprintf(e.stdout, "Associated with: %s\n", a.Data.Number)
	return nil
}

// sendOpts are the send options signal-cli-go handles so far.
type sendOpts struct {
	recipients []string
	message    string
	hasMessage bool
	fromStdin  bool
	noteToSelf bool
	noUrgent   bool
}

// runSend is `send` for text messages to individual recipients.
func runSend(ctx context.Context, e *env, g *globals, o sendOpts) error {
	a, err := selectAccount(g.root(), g.account)
	if err != nil {
		return err
	}
	defer a.Close()
	if len(o.recipients) == 0 && !o.noteToSelf {
		return userError("No recipients given")
	}
	type target struct{ key, aci string }
	var targets []target
	seen := map[string]bool{}
	for _, r := range o.recipients {
		t, err := resolveRecipient(a, r)
		if err != nil {
			return err
		}
		if !seen[t.key] {
			seen[t.key] = true
			targets = append(targets, target(t))
		}
	}
	text := o.message
	if o.fromStdin {
		b, err := io.ReadAll(e.stdin)
		if err != nil {
			return userError("Failed to read message from stdin: " + err.Error())
		}
		text = string(b)
	}
	if text == "" {
		return userError("Sending empty message is not allowed, either a message, attachment or sticker must be given.")
	}
	n, err := e.network(g)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	c, err := n.dialChat(ctx, a, false)
	if err != nil {
		return ioError("Error while checking account %s: %v", a.Data.Number, err)
	}
	defer c.Close()
	if err := checkAccount(ctx, a, c, a.Data.Number); err != nil {
		return err
	}
	u, err := n.dialChat(ctx, nil, false)
	if err != nil {
		u = nil // sealed sender unavailable; sends go unsealed
	} else {
		defer u.Close()
	}
	s := &manager.Sender{Acct: a, Chat: c}
	if u != nil {
		s.Unauth = u
	}
	var results []sendResult
	var ts uint64
	for _, t := range targets {
		var err error
		ts, err = s.SendTextAt(ctx, t.aci, text, ts, !o.noUrgent)
		results = append(results, resultFor(t.key, t.aci, err))
	}
	out, code, msg := renderSendResults(g.json(), ts, results)
	io.WriteString(e.stdout, out)
	if code != 0 {
		return &cliError{msg: msg, code: code}
	}
	return nil
}

func resultFor(key, aci string, err error) sendResult {
	r := sendResult{key: key, aci: aci, kind: resultSuccess}
	var se *manager.SendStatusError
	switch {
	case err == nil:
	case errors.Is(err, manager.ErrUnregistered):
		r.kind = resultUnregistered
	case errors.Is(err, protocol.ErrUntrustedIdentity):
		r.kind = resultUntrusted
	case errors.As(err, &se) && (se.Status == 413 || se.Status == 429):
		r.kind = resultRateLimit
	default:
		r.kind = resultNetwork
	}
	return r
}

type resolved struct{ key, aci string }

// resolveRecipient interprets a RECIPIENT (index.md §10). Phone numbers
// need CDSI, which signal-cli-go does not have yet, unless the number is
// already known locally.
func resolveRecipient(a *store.Account, s string) (resolved, error) {
	switch {
	case uuidRe.MatchString(s):
		return resolved{key: strings.ToLower(s), aci: strings.ToLower(s)}, nil
	case strings.HasPrefix(s, "PNI:"):
		if !uuidRe.MatchString(s[4:]) {
			return resolved{}, userError("Invalid phone number '" + s + "': Invalid PNI")
		}
		return resolved{}, userError("Sending to a PNI is not supported by signal-cli-go yet")
	case strings.HasPrefix(s, "u:"):
		return resolved{}, userError("Sending to a username is not supported by signal-cli-go yet")
	case strings.Contains(s, "@"):
		return resolved{}, userError("Invalid phone number '" + s + "': Possible attempt to use email address.")
	}
	digits := strings.Map(func(r rune) rune {
		if r == '+' || (r >= '0' && r <= '9') {
			return r
		}
		return -1
	}, s)
	if digits == "" {
		return resolved{}, userError("Invalid phone number '" + s + "': No valid characters found.")
	}
	if !strings.HasPrefix(digits, "+") {
		digits = "+" + digits
	}
	if digits == a.Data.Number {
		return resolved{key: digits, aci: a.Data.ACI}, nil
	}
	for aci, rc := range a.Recipients {
		if rc.Number == digits {
			return resolved{key: digits, aci: aci}, nil
		}
	}
	return resolved{}, ioError("Failed to send message: Unable to resolve %s: phone number lookup (CDSI) is not implemented in signal-cli-go yet (UnsupportedOperationException)", digits)
}

// receiveOpts are receive's options.
type receiveOpts struct {
	timeout          time.Duration // <= 0: none
	maxMessages      int
	sendReadReceipts bool
	ignoreStories    bool
}

// runReceive is `receive` (spec/surface/cli/receive.md).
func runReceive(ctx context.Context, e *env, g *globals, o receiveOpts) error {
	a, err := selectAccount(g.root(), g.account)
	if err != nil {
		return err
	}
	defer a.Close()
	n, err := e.network(g)
	if err != nil {
		return err
	}
	c, err := n.dialChat(ctx, a, true)
	if err != nil {
		return ioError("Error while checking account %s: %v", a.Data.Number, err)
	}
	defer c.Close()
	if err := checkAccount(ctx, a, c, a.Data.Number); err != nil {
		return err
	}
	var unauth manager.Requester
	if u, err := n.dialChat(ctx, nil, false); err == nil {
		defer u.Close()
		unauth = u
	}
	rc := &RenderContext{Self: a.Data.Number, SelfACI: a.Data.ACI, Lookup: func(aci string) (string, string) {
		return a.Recipients[aci].Number, ""
	}}
	r := &manager.Receiver{
		Acct:             a,
		Decryptor:        &manager.Decryptor{Acct: a},
		Sender:           &manager.Sender{Acct: a, Chat: c, Unauth: unauth},
		SendReadReceipts: o.sendReadReceipts,
		Log:              func(s string) { fmt.Fprintln(e.stderr, "WARN  ReceiveHelper - "+s) },
		OnEvent: func(ev manager.Event) {
			if g.json() {
				fmt.Fprintln(e.stdout, RenderJSON(rc, ev))
			} else {
				io.WriteString(e.stdout, RenderPlain(rc, ev))
			}
		},
	}
	if err := r.RetryCache(ctx); err != nil {
		return ioError("Error while receiving messages: %v", err)
	}
	if err := r.Run(ctx, c, o.timeout, o.maxMessages); err != nil {
		return ioError("Error while receiving messages: %v", err)
	}
	return nil
}
