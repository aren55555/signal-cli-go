// SPDX-License-Identifier: GPL-3.0-or-later

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

	"github.com/aren55555/signal-cli-go/internal/errcode"
	"github.com/aren55555/signal-cli-go/internal/fingerprint"
	"github.com/aren55555/signal-cli-go/internal/groups"
	"github.com/aren55555/signal-cli-go/internal/manager"
	"github.com/aren55555/signal-cli-go/internal/protocol"
	"github.com/aren55555/signal-cli-go/internal/qr"
	"github.com/aren55555/signal-cli-go/internal/service/agent"
	"github.com/aren55555/signal-cli-go/internal/service/cdn"
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
	// daemon is set while a command runs inside daemon or jsonRpc: the
	// account is then shared and already open.
	daemon *daemonState
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
		return nil, errcode.LiveUnsupported
	}
	n := &network{userAgent: e.getenv(agent.EnvOverride)}
	for _, k := range []string{"HTTPS_PROXY", "https_proxy", "ALL_PROXY", "all_proxy"} {
		if v := e.getenv(k); v != "" {
			u, err := url.Parse(v)
			if err == nil && u.Host == "" {
				u, err = url.Parse("http://" + v)
			}
			if err != nil {
				return nil, errcode.InvalidProxy.Wrap(err, err)
			}
			n.proxy = u
			break
		}
	}
	return n, nil
}

// cdn is the attachment CDN client.
func (n *network) cdn() *cdn.Client {
	return cdn.New(cdn.Config{Proxy: n.proxy, UserAgentOverride: n.userAgent})
}

func (n *network) dialChat(ctx context.Context, a *store.Account, keepAlive bool) (*chat.Conn, error) {
	return n.dialChatStories(ctx, a, keepAlive, true)
}

// dialChatStories is dialChat with x-signal-receive-stories set (pipe.md §2).
func (n *network) dialChatStories(ctx context.Context, a *store.Account, keepAlive, stories bool) (*chat.Conn, error) {
	ep := chat.Endpoint{Proxy: n.proxy, UserAgentOverride: n.userAgent, ReceiveStories: stories}
	if a != nil {
		ep.Credentials = &chat.Credentials{ACI: a.Data.ACI, DeviceID: a.Data.DeviceID, Password: a.Data.Password}
	}
	if keepAlive {
		ep.KeepAlive = chat.KeepAlive
	}
	return chat.Dial(ctx, ep)
}

// checkAccount runs the start-up account check on the authenticated
// socket (api.md §4.3).
func checkAccount(ctx context.Context, a *store.Account, c *chat.Conn, account string) error {
	err := manager.CheckAccount(ctx, a, c, time.Now())
	switch {
	case errors.Is(err, manager.ErrNotRegistered):
		return errcode.UserNotRegistered.Wrap(err, account)
	case err != nil:
		return errcode.AccountCheckFailed.Wrap(err, account, err)
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
		return errcode.AccountsListRead.Wrap(err)
	}
	l := &manager.Linker{
		Root: g.root(),
		REST: rest.New(rest.Config{Proxy: n.proxy, UserAgentOverride: n.userAgent}),
		OpenSocket: func(ctx context.Context) (*provision.Session, error) {
			return provision.Open(ctx, provision.Endpoint{Proxy: n.proxy, UserAgentOverride: n.userAgent})
		},
		DeviceName: name,
		OnURL: func(u string) {
			if isTerminal(e.stdout) {
				if c, err := qr.Encode([]byte(u)); err == nil {
					fmt.Fprintln(e.stdout, "\x1b[37;40m")
					for _, l := range c.Lines() {
						fmt.Fprintln(e.stdout, l)
					}
					fmt.Fprintln(e.stdout, "\x1b[39;49m")
				}
			}
			fmt.Fprintln(e.stdout, u)
		},
		PostLink: func(ctx context.Context, a *store.Account) error {
			c, err := n.dialChat(ctx, a, false)
			if err != nil {
				return err
			}
			defer c.Close()
			if err := manager.CheckAccount(ctx, a, c, time.Now()); err != nil {
				return err
			}
			// The post-link sync requests (sync.md §7).
			s := &manager.Sender{Acct: a, Chat: c}
			if err := s.SendSyncRequests(ctx); err != nil {
				fmt.Fprintln(e.stderr, "WARN  ProvisioningManagerImpl - Failed to request sync messages from linked device, data can be requested again with `sendSyncRequest`.")
			}
			return nil
		},
	}
	ctx, cancel := context.WithTimeout(ctx, 150*time.Second)
	defer cancel()
	a, err := l.Link(ctx)
	switch {
	case errors.Is(err, manager.ErrUserExists):
		return errcode.LinkUserExists.Wrap(err)
	case a == nil && err != nil:
		return errcode.LinkRequestFailed.WrapText(err)
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

// session is a loaded, checked account with its sockets, for the send
// family.
type session struct {
	acct   *store.Account
	sender *manager.Sender
	chat   *chat.Conn // the authenticated socket (gRPC calls)
	groups *manager.Groups
	close  func()
	// closeConns closes the sockets only (a daemon session that died).
	closeConns func()
}

// openSession runs the start-up account check and opens both sockets
// (the unauthenticated one may fail: sends then go unsealed). Inside the
// daemon it returns the account's shared session, which the command
// must not close.
func openSession(ctx context.Context, e *env, g *globals, a *store.Account) (*session, error) {
	if g.daemon != nil {
		la := g.daemon.byAccount(a)
		if la == nil {
			return nil, errcode.UserNotRegistered.New(a.Data.Number)
		}
		s, err := g.daemon.sessionLocked(ctx, la)
		if err != nil {
			return nil, err
		}
		shared := *s
		shared.close = func() {}
		return &shared, nil
	}
	return openSessionWith(ctx, e, g, a, false, true)
}

// openSessionWith is openSession with the socket options: keep-alives
// and whether stories are received.
func openSessionWith(ctx context.Context, e *env, g *globals, a *store.Account, keepAlive, stories bool) (*session, error) {
	n, err := e.network(g)
	if err != nil {
		return nil, err
	}
	c, err := n.dialChatStories(ctx, a, keepAlive, stories)
	if err != nil {
		return nil, errcode.AccountCheckFailed.Wrap(err, a.Data.Number, err)
	}
	if err := checkAccount(ctx, a, c, a.Data.Number); err != nil {
		c.Close()
		return nil, err
	}
	s := &session{acct: a, sender: g.withProfiles(e, &manager.Sender{Acct: a, Chat: c, CDN: n.cdn()}), chat: c}
	closers := []func() error{c.Close}
	if u, err := n.dialChat(ctx, nil, false); err == nil {
		s.sender.Unauth = u
		closers = append(closers, u.Close)
	}
	s.sender.DialCDSI = n.cdsiDialer()
	s.sender.DialSVR2 = n.svr2Dialer()
	s.groups = n.groupsManager(g, a, c, s.sender)
	s.closeConns = func() {
		for _, f := range closers {
			f()
		}
	}
	s.close = func() {
		// The scheduled full CDSI refresh runs before the account closes
		// (cdsi.md §6.1).
		rctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		s.sender.RefreshRecipients(rctx)
		cancel()
		for _, f := range closers {
			f()
		}
	}
	return s, nil
}

// resolveAll resolves recipients, dropping duplicates.
func resolveAll(a *store.Account, recipients []string) ([]resolved, error) {
	var out []resolved
	seen := map[string]bool{}
	for _, r := range recipients {
		t, err := resolveRecipient(a, r)
		var uu *unresolvedUsername
		if errors.As(err, &uu) {
			t, err = resolved{key: r, username: uu.name}, nil
		}
		if err != nil {
			return nil, err
		}
		if !seen[t.key] {
			seen[t.key] = true
			out = append(out, t)
		}
	}
	return out, nil
}

// finishSend prints the results and maps them to the exit code.
func finishSend(e *env, g *globals, ts uint64, results []sendResult) error {
	out, code, msg := renderSendResults(g.json(), ts, results)
	io.WriteString(e.stdout, out)
	if code != 0 {
		return sendFailure(code, msg)
	}
	return nil
}

// runSend is `send` for text messages to individual recipients.
func runSend(ctx context.Context, e *env, g *globals, o sendOpts) error {
	a, release, err := useAccount(g)
	if err != nil {
		return err
	}
	defer release()
	if len(o.recipients) == 0 && !o.noteToSelf {
		return errcode.NoRecipients
	}
	targets, err := resolveAll(a, o.recipients)
	if err != nil {
		return err
	}
	text := o.message
	if o.fromStdin {
		b, err := io.ReadAll(e.stdin)
		if err != nil {
			return errcode.StdinReadFailed.WrapText(err)
		}
		text = string(b)
	}
	if text == "" {
		return errcode.EmptyMessage
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	s, err := openSession(ctx, e, g, a)
	if err != nil {
		return err
	}
	defer s.close()
	var results []sendResult
	var ts uint64
	for _, t := range targets {
		var err error
		if t.aci == "" {
			results = append(results, unregisteredResult(t))
			continue
		}
		ts, err = s.sender.SendTextAt(ctx, t.aci, text, ts, !o.noUrgent)
		results = append(results, resultFor(t.key, t.aci, err))
	}
	return finishSend(e, g, ts, results)
}

// runSendTyping is `sendTyping [-s] RECIPIENT…`.
func runSendTyping(ctx context.Context, e *env, g *globals, recipients, groupArgs []string, stop bool) error {
	a, release, err := useAccount(g)
	if err != nil {
		return err
	}
	defer release()
	if len(recipients) == 0 && len(groupArgs) == 0 {
		return errcode.NoRecipients
	}
	var groupIDs [][]byte
	for _, id := range groupArgs {
		gid, err := parseGroupArg(id)
		if err != nil {
			return err
		}
		groupIDs = append(groupIDs, gid)
	}
	targets, err := resolveAll(a, recipients)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	s, err := openSession(ctx, e, g, a)
	if err != nil {
		return err
	}
	defer s.close()
	var results []sendResult
	var ts uint64
	for _, t := range targets {
		var err error
		if t.aci == "" {
			results = append(results, unregisteredResult(t))
			continue
		}
		ts, err = s.sender.SendTyping(ctx, t.aci, stop, ts)
		results = append(results, resultFor(t.key, t.aci, err))
	}
	for _, id := range groupIDs {
		gts, res, err := s.groups.SendTyping(ctx, id, stop)
		if err != nil {
			if ge := groupSendError(s, id, err); errcode.ExitCode(ge) == 1 {
				return errcode.GroupSendFailed.Wrap(err, ge)
			}
			return errcode.SendIOFailedUser.WrapText(err)
		}
		ts = gts
		results = append(results, groupResults(a, groups.FormatGroupID(id), res)...)
	}
	return finishSend(e, g, ts, results)
}

// runSendReceipt is `sendReceipt RECIPIENT -t TS… [--type read|viewed]`.
func runSendReceipt(ctx context.Context, e *env, g *globals, recipient string, timestamps []uint64, kind string) error {
	a, release, err := useAccount(g)
	if err != nil {
		return err
	}
	defer release()
	t, err := resolveRecipient(a, recipient)
	if err != nil {
		return err
	}
	if kind != "" && kind != "read" && kind != "viewed" {
		return errcode.ReceiptTypeUnknown.New(kind)
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	s, err := openSession(ctx, e, g, a)
	if err != nil {
		return err
	}
	defer s.close()
	rk := manager.ReceiptRead
	if kind == "viewed" {
		rk = manager.ReceiptViewed
	}
	// An explicit read or viewed receipt is skipped when read receipts are
	// configured off (content.md §5.1); the sync goes out either way
	// (sync.md §6.8).
	var ts uint64
	var results []sendResult
	if rr := a.Data.Configuration.ReadReceipts; rr == nil || *rr {
		ts, err = s.sender.SendReceiptAt(ctx, t.aci, rk, timestamps)
		results = append(results, resultFor(t.key, t.aci, err))
	}
	if err := s.sender.SendReadSync(ctx, t.aci, kind == "viewed", timestamps); err != nil {
		fmt.Fprintf(e.stderr, "WARN  SendHelper - Failed to send read sync: %v\n", err)
	}
	return finishSend(e, g, ts, results)
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

type resolved struct {
	key, aci string
	group    []byte // set for a group recipient
	// username is set for a username the server does not know: the send
	// reports it as an unregistered user (usernames.md §7.2 step 4).
	username string
}

// resolveRecipient interprets a RECIPIENT (index.md §10). Phone numbers
// need CDSI, which signal-cli-go does not have yet, unless the number is
// already known locally.
func resolveRecipient(a *store.Account, s string) (resolved, error) {
	switch {
	case uuidRe.MatchString(s):
		return resolved{key: strings.ToLower(s), aci: strings.ToLower(s)}, nil
	case strings.HasPrefix(s, "PNI:"):
		if !uuidRe.MatchString(s[4:]) {
			return resolved{}, errcode.InvalidPhoneNumber.New(s, "Invalid PNI")
		}
		pni := "PNI:" + strings.ToLower(s[4:])
		return resolved{key: s, aci: pni}, nil
	case strings.HasPrefix(s, "u:"):
		name := s[2:]
		if usernameResolver != nil {
			if aci, ok := usernameResolver(a, name); ok {
				return resolved{key: s, aci: aci}, nil
			}
		}
		return resolved{}, &unresolvedUsername{name: name}
	case strings.Contains(s, "@"):
		return resolved{}, errcode.InvalidPhoneNumber.New(s, "Possible attempt to use email address.")
	}
	digits, err := canonicalNumber(a, s)
	if err != nil {
		return resolved{}, err
	}
	if digits == a.Data.Number {
		return resolved{key: digits, aci: a.Data.ACI}, nil
	}
	if k, ok := a.FindByNumber(digits); ok {
		if ad := a.AddressOf(k); ad.ACI != "" {
			return resolved{key: digits, aci: ad.ACI}, nil
		} else if ad.PNI != "" {
			return resolved{key: digits, aci: "PNI:" + ad.PNI}, nil
		}
	}
	if numberResolver != nil {
		if k, ok := numberResolver(a, digits); ok {
			return resolved{key: digits, aci: k}, nil
		}
	}
	return resolved{}, errcode.RecipientNotRegistered.New(digits)
}

// receiveOpts are receive's options.
type receiveOpts struct {
	timeout           time.Duration // <= 0: none
	maxMessages       int
	sendReadReceipts  bool
	ignoreStories     bool
	ignoreAttachments bool
	ignoreAvatars     bool
	ignoreStickers    bool
}

// runReceive is `receive` (spec/surface/cli/receive.md).
func runReceive(ctx context.Context, e *env, g *globals, o receiveOpts) error {
	a, release, err := useAccount(g)
	if err != nil {
		return err
	}
	defer release()
	n, err := e.network(g)
	if err != nil {
		return err
	}
	c, err := n.dialChatStories(ctx, a, true, !o.ignoreStories)
	if err != nil {
		return errcode.AccountCheckFailed.Wrap(err, a.Data.Number, err)
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
	r := newReceiver(e, g, n, a, c, unauth, o, func(rc *RenderContext, ev manager.Event) {
		if g.json() {
			fmt.Fprintln(e.stdout, RenderJSON(rc, ev))
		} else {
			io.WriteString(e.stdout, RenderPlain(rc, ev))
		}
	})
	if err := r.RetryCache(ctx); err != nil {
		return errcode.ReceiveFailed.WrapText(err)
	}
	if err := r.Run(ctx, c, o.timeout, o.maxMessages); err != nil {
		return errcode.ReceiveFailed.WrapText(err)
	}
	return nil
}

// newReceiver builds the receive loop for account a on connection c
// (pipe.md §7); onEvent gets each handled envelope and its render
// context.
func newReceiver(e *env, g *globals, n *network, a *store.Account, c *chat.Conn, unauth manager.Requester, o receiveOpts, onEvent func(*RenderContext, manager.Event)) *manager.Receiver {
	cd := n.cdn()
	var nameSender *manager.Sender // set below; names may need a lazy profile read
	rc := &RenderContext{Self: a.Data.Number, SelfACI: a.Data.ACI, AttachmentsDir: strings.TrimRight(g.dataDir, "/") + "/attachments", Lookup: func(aci string) (string, string) {
		return a.Recipients[aci].Number, senderName(a, nameSender, aci)
	}, Group: func(id []byte) (string, bool, bool) {
		rec := a.Group(id)
		if rec == nil {
			return "", false, false
		}
		st := manager.StateOf(rec)
		if st == nil {
			return "", true, true
		}
		return st.Title, false, true
	}}
	sender := g.withProfiles(e, &manager.Sender{Acct: a, Chat: c, Unauth: unauth, CDN: cd})
	nameSender = sender
	gm := n.groupsManager(g, a, c, sender)
	gm.IgnoreAvatars = o.ignoreAvatars
	gm.Log = func(f string, args ...any) { fmt.Fprintf(e.stderr, "WARN  GroupHelper - "+f+"\n", args...) }
	r := &manager.Receiver{
		Acct: a,
		Decryptor: &manager.Decryptor{Acct: a, TrustNew: g.trustNew,
			Debug: func(f string, args ...any) { fmt.Fprintf(e.stderr, "DEBUG ReceiveHelper - "+f+"\n", args...) }},
		Sender: sender,
		Groups: gm,
		Storage: &manager.Storage{Sender: sender, Groups: gm, REST: gm.Svc.REST,
			Log: func(f string, args ...any) { fmt.Fprintf(e.stderr, "WARN  StorageHelper - "+f+"\n", args...) }},
		Stickers: &manager.StickerStore{Dir: filepath.Join(g.dataDir, "stickers"), CDN: cd, Ignore: o.ignoreStickers,
			Log: func(f string, args ...any) { fmt.Fprintf(e.stderr, "WARN  StickerHelper - "+f+"\n", args...) }},
		Attachments: &manager.AttachmentStore{Dir: filepath.Join(g.dataDir, "attachments"), CDN: cd, Ignore: o.ignoreAttachments, AvatarsDir: filepath.Join(g.dataDir, "avatars"), IgnoreAvatars: o.ignoreAvatars,
			Log: func(f string, args ...any) { fmt.Fprintf(e.stderr, "WARN  AttachmentHelper - "+f+"\n", args...) }},
		SendReadReceipts: o.sendReadReceipts,
		Log:              func(s string) { fmt.Fprintln(e.stderr, "WARN  ReceiveHelper - "+s) },
		OnEvent:          func(ev manager.Event) { onEvent(rc, ev) },
	}
	return r
}

// runSyncCommand is sendSyncRequest and sendContacts (sync.md §8.5,
// §8.6): exit 0, no output; failures are logged.
func runSyncCommand(ctx context.Context, e *env, g *globals, contacts bool) error {
	return withAccount(g, func(a *store.Account) error {
		ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		s, err := openSession(ctx, e, g, a)
		if err != nil {
			return err
		}
		defer s.close()
		if contacts {
			err = s.sender.SendContacts(ctx)
		} else {
			err = s.sender.SendSyncRequests(ctx)
			defer s.syncStorage(ctx, e)
		}
		if err != nil {
			fmt.Fprintf(e.stderr, "WARN  SyncHelper - %v\n", err)
		}
		return nil
	})
}

// runTrust is `trust` (spec/surface/cli/trust.md): no output on success.
func runTrust(ctx context.Context, e *env, g *globals, recipient string, all bool, safetyNumber string, hasSafetyNumber bool) error {
	return withAccount(g, func(a *store.Account) error {
		t, err := resolveRecipient(a, recipient)
		if err != nil {
			return err
		}
		var key []byte
		if !all {
			if !hasSafetyNumber {
				return errcode.TrustNeedsSafetyNumber
			}
			code, err := fingerprint.ParseVerificationCode(safetyNumber)
			if err != nil {
				return errcode.SafetyNumberFormat.Wrap(err)
			}
			// A numeric or scannable code that does not match leaves key
			// nil, which TrustVerifiedKey reports as "not changed".
			if k, ok := manager.VerifyCode(a, t.aci, code); ok || code.Kind == fingerprint.KindHex {
				key = k
			}
		}
		ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		sender := &manager.Sender{Acct: a}
		// The verified sync needs the network; trusting itself does not.
		if a.IsMultiDevice() {
			if s, err := openSession(ctx, e, g, a); err == nil {
				defer s.close()
				sender = s.sender
			}
		}
		if all {
			err = sender.TrustAllKnownKeys(ctx, t.aci)
			if errors.Is(err, manager.ErrTrustNotChanged) {
				return errcode.TrustAllFailed.Wrap(err)
			}
		} else {
			err = sender.TrustVerifiedKey(ctx, t.aci, key)
			if errors.Is(err, manager.ErrTrustNotChanged) {
				return errcode.TrustVerifiedFailed.Wrap(err)
			}
		}
		return err
	})
}

// withProfiles sets what profile fetches need on s (profiles.md §5).
func (g *globals) withProfiles(e *env, s *manager.Sender) *manager.Sender {
	s.AvatarsDir = filepath.Join(g.dataDir, "avatars")
	s.TrustNew = g.trustNew
	s.DisableSendLog = g.disableSendLog
	s.Log = func(f string, args ...any) { fmt.Fprintf(e.stderr, "WARN  ProfileHelper - "+f+"\n", args...) }
	s.Debug = func(f string, args ...any) { fmt.Fprintf(e.stderr, "DEBUG SendHelper - "+f+"\n", args...) }
	return s
}

// canonicalNumber is a phone-number argument in E.164 form.
func canonicalNumber(a *store.Account, s string) (string, error) {
	digits := strings.Map(func(r rune) rune {
		if r == '+' || (r >= '0' && r <= '9') {
			return r
		}
		return -1
	}, s)
	if digits == "" {
		return "", errcode.InvalidPhoneNumber.New(s, "No valid characters found.")
	}
	if !strings.HasPrefix(digits, "+") {
		digits = "+" + digits
	}
	return digits, nil
}

// syncStorage runs one storage-service sync (storage.md §6.1); failures
// are logged, never fatal.
func (s *session) syncStorage(ctx context.Context, e *env) {
	st := &manager.Storage{Sender: s.sender, Groups: s.groups, REST: s.groups.Svc.REST,
		Log: func(f string, args ...any) { fmt.Fprintf(e.stderr, "WARN  StorageHelper - "+f+"\n", args...) }}
	if err := st.Sync(ctx); err != nil {
		fmt.Fprintf(e.stderr, "WARN  SyncStorageJob - Failed to sync storage data: %v\n", err)
	}
}

// unresolvedUsername is a u:<name> recipient the server does not know;
// commands with one recipient report it as not registered, sends as an
// unregistered user result.
type unresolvedUsername struct{ name string }

func (e *unresolvedUsername) Error() string { return errcode.Message(e.Unwrap()) }

func (e *unresolvedUsername) Unwrap() error { return errcode.RecipientNotRegistered.New(e.name) }

// unregisteredResult is the send result for an unresolved username.
func unregisteredResult(t resolved) sendResult {
	return sendResult{key: t.key, username: t.username, kind: resultUnregistered}
}
