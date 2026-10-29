// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/aren55555/signal-cli-go/internal/cli/argparse"
	"github.com/aren55555/signal-cli-go/internal/daemon"
	"github.com/aren55555/signal-cli-go/internal/errcode"
	"github.com/aren55555/signal-cli-go/internal/jackson"
	"github.com/aren55555/signal-cli-go/internal/manager"
	"github.com/aren55555/signal-cli-go/internal/service/provision"
	"github.com/aren55555/signal-cli-go/internal/service/rest"
	"github.com/aren55555/signal-cli-go/internal/store"
)

// rpcClass is each method's account requirement (methods/index.md);
// unlisted methods need a single account.
var rpcClass = map[string]daemon.Class{
	"listAccounts":           daemon.Multi,
	"startLink":              daemon.Multi,
	"finishLink":             daemon.Multi,
	"register":               daemon.Registration,
	"verify":                 daemon.Registration,
	"deleteLocalAccountData": daemon.Registration,
	"version":                daemon.Both,
}

// methods is the JSON-RPC method table: every CLI command except
// daemon, jsonRpc and link, plus startLink, finishLink and the
// subscription methods.
func (d *daemonState) methods() map[string]*daemon.Method {
	m := daemon.SubscriptionMethods(d.onSubscribe)
	for _, c := range commandHelp {
		name := c[0]
		switch name {
		case "daemon", "jsonRpc", "link":
			continue
		}
		cls, ok := rpcClass[name]
		if !ok {
			cls = daemon.Single
		}
		m[name] = &daemon.Method{Class: cls, Run: d.namespaceMethod(name)}
	}
	m["version"].Run = func(ctx context.Context, c *daemon.Call) (string, error) {
		if _, err := daemon.NamespaceParams(c); err != nil {
			return "", err
		}
		return `{"version":` + jackson.Quote(Version) + `}`, nil
	}
	m["listAccounts"].Run = func(ctx context.Context, c *daemon.Call) (string, error) {
		if _, err := daemon.NamespaceParams(c); err != nil {
			return "", err
		}
		var items []string
		for _, a := range d.list() {
			items = append(items, `{"number":`+jackson.Quote(a.Number)+`}`)
		}
		return "[" + strings.Join(items, ",") + "]", nil
	}
	m["receive"].Run = d.rpcReceive
	m["register"].Run = d.rpcRegister
	m["verify"].Run = d.rpcVerify
	m["deleteLocalAccountData"].Run = d.rpcDeleteLocalAccountData
	d.callMethods(m)
	m["startLink"] = &daemon.Method{Class: daemon.Multi, Run: d.rpcStartLink}
	m["finishLink"] = &daemon.Method{Class: daemon.Multi, Run: d.rpcFinishLink}
	return m
}

// namespaceMethod runs a CLI command with its options read from the
// params object (methods/index.md "Parameter conventions").
func (d *daemonState) namespaceMethod(name string) func(context.Context, *daemon.Call) (string, error) {
	return func(ctx context.Context, c *daemon.Call) (string, error) {
		params, err := daemon.NamespaceParams(c)
		if err != nil {
			return "", err
		}
		res, err := rpcNamespace(commandParser(name), params, rpcIgnored[name]...)
		if err != nil {
			return "", err
		}
		var acct *store.Account
		if la := d.account(c.Account); la != nil {
			acct = la.acct
		}
		if err := rpcPrepare(name, acct, params, res); err != nil {
			return "", err
		}
		return d.runCommandRPC(ctx, c.Account, name, res)
	}
}

// runCommandRPC runs a command for account with JSON output captured as
// the result. A command that printed a result and then failed returns
// the result as the error's data.
func (d *daemonState) runCommandRPC(ctx context.Context, account, name string, res *argparse.Result) (string, error) {
	var out bytes.Buffer
	e := &env{stdout: &out, stderr: d.e.stderr, stdin: strings.NewReader(""), getenv: d.e.getenv}
	g := *d.g
	g.account = account
	g.output = "json"
	g.daemon = d
	err := runCommand(ctx, e, &g, name, res)
	result := strings.TrimSpace(out.String())
	if err == nil && name == "unregister" {
		d.unregistered(account)
	}
	if err != nil {
		ee := errcode.As(err)
		if result != "" {
			ee.Data = `{"response":` + result + `}`
		}
		return "", ee
	}
	return result, nil
}

// rpcKey is the params key of an option: its primary long name without
// dashes, or a positional's name.
func rpcKey(a *argparse.Arg) string {
	for _, n := range a.Names {
		if strings.HasPrefix(n, "--") {
			return n[2:]
		}
	}
	return strings.TrimLeft(a.Names[0], "-")
}

// camelCase is the alternative key: dash-separated parts joined, every
// part after the first capitalized.
func camelCase(k string) string {
	parts := strings.Split(k, "-")
	var b strings.Builder
	b.WriteString(parts[0])
	for _, p := range parts[1:] {
		if p == "" {
			continue
		}
		b.WriteString(strings.ToUpper(p[:1]) + strings.ToLower(p[1:]))
	}
	return b.String()
}

// lookup finds key, then its camelCase form; null counts as absent.
func lookup(params *jackson.Node, key string) *jackson.Node {
	if v := params.Get(key); v != nil && v.Kind != jackson.Null {
		return v
	}
	if v := params.Get(camelCase(key)); v != nil && v.Kind != jackson.Null {
		return v
	}
	return nil
}

func classCast(from, to string) error { return errcode.RPCClassCast.New(from, to, from, to) }

// rpcNamespace reads a command's options from params the way signal-cli's
// JSON-RPC namespace does: values keep their JSON types, and a value of
// the wrong type fails as Java's casts do.
// rpcIgnored are options a method never reads over JSON-RPC.
var rpcIgnored = map[string][]string{"listGroups": {"detailed"}}

func rpcNamespace(p *argparse.Parser, params *jackson.Node, ignored ...string) (*argparse.Result, error) {
	r := &argparse.Result{Values: map[string]any{}} // no CLI defaults over JSON-RPC
	for _, a := range p.Args {
		if a.Action == argparse.Help || slices.Contains(ignored, rpcKey(a)) {
			continue
		}
		key, dest := rpcKey(a), argparse.DestOf(a)
		v := lookup(params, key)
		if a.IsList() {
			var elems []*jackson.Node
			switch {
			case v != nil && v.Kind == jackson.Array:
				elems = v.Elems
			case v != nil:
				elems = []*jackson.Node{v}
			default:
				pv := lookup(params, key+"s")
				if pv == nil {
					continue
				}
				if pv.Kind != jackson.Array {
					return nil, classCast(pv.JavaClass(), "java.util.List")
				}
				elems = pv.Elems
			}
			vals := make([]any, len(elems))
			strs := make([]string, len(elems))
			allStrings := true
			for i, el := range elems {
				if a.Kind == argparse.String && el.Kind != jackson.String {
					return nil, classCast(el.JavaClass(), "java.lang.String")
				}
				x, err := rpcValue(a, el)
				if err != nil {
					return nil, err
				}
				vals[i] = x
				s, ok := x.(string)
				strs[i], allStrings = s, allStrings && ok
			}
			if allStrings {
				r.Values[dest] = strs
			} else {
				r.Values[dest] = vals
			}
			continue
		}
		if v == nil {
			continue
		}
		x, err := rpcValue(a, v)
		if err != nil {
			return nil, err
		}
		r.Values[dest] = x
	}
	return r, nil
}

// rpcValue converts one params value for option a.
func rpcValue(a *argparse.Arg, v *jackson.Node) (any, error) {
	switch {
	case a.Action == argparse.StoreTrue || a.Kind == argparse.Boolean:
		if v.Kind != jackson.Bool {
			return nil, classCast(v.JavaClass(), "java.lang.Boolean")
		}
		return v.B, nil
	case a.Kind == argparse.Int32:
		if v.Kind != jackson.Int {
			return nil, classCast(v.JavaClass(), "java.lang.Integer")
		}
		return int(v.I), nil
	case a.Kind == argparse.Int64:
		if v.Kind != jackson.Long {
			return nil, classCast(v.JavaClass(), "java.lang.Long")
		}
		return v.I, nil
	case a.Kind == argparse.Double:
		if v.Kind != jackson.Double {
			return nil, classCast(v.JavaClass(), "java.lang.Double")
		}
		return v.F, nil
	}
	return v.JavaString(), nil
}

// typedField is a member of a typed params class.
type typedField struct{ name, kind string } // kind: String, Boolean, Integer, Double

// bindTyped binds params strictly to a typed params class (receive,
// register, verify, finishLink): exact keys only, Jackson's coercions.
func bindTyped(class string, fields []typedField, params *jackson.Node) (map[string]any, error) {
	out := map[string]any{}
	if params == nil {
		return out, nil
	}
	if params.Kind == jackson.Array {
		return nil, errcode.RPCDeserializeArray.New(class)
	}
	quoted := make([]string, len(fields))
	for i, f := range fields {
		quoted[i] = `"` + f.name + `"`
	}
	for i, k := range params.Keys {
		v := params.Vals[i]
		var f *typedField
		for j := range fields {
			if fields[j].name == k {
				f = &fields[j]
			}
		}
		if f == nil {
			return nil, errcode.RPCUnrecognizedField.New(k, class, len(fields), strings.Join(quoted, ", "), class, k)
		}
		if v.Kind == jackson.Null {
			delete(out, k)
			continue
		}
		x, err := coerce(f.kind, v, class, k)
		if err != nil {
			return nil, err
		}
		if x != nil {
			out[k] = x
		}
	}
	return out, nil
}

// coerce applies Jackson's default scalar coercions for a member of
// the given Java type.
func coerce(kind string, v *jackson.Node, class, member string) (any, error) {
	target := "java.lang." + kind
	badString := func(why string) error {
		return errcode.RPCDeserializeString.New(target, `"`+v.S+`"`, why, class, member)
	}
	switch kind {
	case "String":
		if v.IsContainer() {
			return nil, daemon.MemberTypeError(target, v, class, member)
		}
		return v.AsText(), nil
	case "Boolean":
		switch v.Kind {
		case jackson.Bool:
			return v.B, nil
		case jackson.Int, jackson.Long, jackson.BigInt:
			return v.AsText() != "0", nil
		case jackson.String:
			switch strings.TrimSpace(strings.ToLower(v.S)) {
			case "true":
				return true, nil
			case "false":
				return false, nil
			case "":
				return nil, nil
			}
			return nil, badString(`only "true" or "false" recognized`)
		}
	case "Integer":
		switch v.Kind {
		case jackson.Int:
			return int(v.I), nil
		case jackson.Long, jackson.BigInt:
			return nil, errcode.RPCNumberRange.New(v.AsText(), class, member)
		case jackson.Double:
			return int(v.AsInt()), nil
		case jackson.String:
			s := strings.TrimSpace(v.S)
			if s == "" {
				return nil, nil
			}
			if n, err := strconv.ParseInt(s, 10, 32); err == nil {
				return int(n), nil
			}
			return nil, badString("not a valid `Integer` value")
		}
	case "Double":
		switch v.Kind {
		case jackson.Int, jackson.Long, jackson.Double:
			f, _ := strconv.ParseFloat(v.AsText(), 64)
			return f, nil
		case jackson.BigInt:
			f, _ := strconv.ParseFloat(v.Big.String(), 64)
			return f, nil
		case jackson.String:
			s := strings.TrimSpace(v.S)
			if s == "" {
				return nil, nil
			}
			if f, err := strconv.ParseFloat(s, 64); err == nil {
				return f, nil
			}
			return nil, badString("not a valid `Double` value")
		}
	}
	return nil, daemon.MemberTypeError(target, v, class, member)
}

const commandsPkg = "org.asamk.signal.commands."

// rpcReceive is `receive` over JSON-RPC (methods/receive.md): messages
// polled for the account, as an array; refused while the daemon is
// already receiving for it.
func (d *daemonState) rpcReceive(ctx context.Context, c *daemon.Call) (string, error) {
	p, err := bindTyped(commandsPkg+"ReceiveCommand$ReceiveParams", []typedField{{"maxMessages", "Integer"}, {"timeout", "Double"}}, c.Params)
	if err != nil {
		return "", err
	}
	la := d.account(c.Account)
	if la == nil {
		return "", errcode.RPCAccountNotFound
	}
	timeout, maxMessages := 3.0, -1
	if t, ok := p["timeout"].(float64); ok {
		timeout = t
	}
	if m, ok := p["maxMessages"].(int); ok {
		maxMessages = m
	}
	la.mu.Lock()
	defer la.mu.Unlock()
	if la.receiving {
		return "", errcode.ReceiveBusy
	}
	s, err := d.sessionLocked(ctx, la)
	if err != nil {
		return "", err
	}
	n, err := d.e.network(d.g)
	if err != nil {
		return "", err
	}
	g := *d.g
	g.account = la.number
	var items []string
	r := newReceiver(d.e, &g, n, la.acct, s.chat, s.sender.Unauth, d.opts, func(rc *RenderContext, ev manager.Event) {
		items = append(items, RenderJSON(rc, ev))
	})
	wait := time.Duration(timeout * float64(time.Second)).Truncate(time.Millisecond)
	if timeout < 0 {
		wait = 0
	}
	if err := r.RetryCache(ctx); err != nil {
		return "", errcode.ReceiveFailed.WrapText(err)
	}
	if err := r.Run(ctx, s.chat, wait, maxMessages); err != nil {
		return "", errcode.ReceiveFailed.WrapText(err)
	}
	return "[" + strings.Join(items, ",") + "]", nil
}

func (d *daemonState) rpcRegister(ctx context.Context, c *daemon.Call) (string, error) {
	p, err := bindTyped(commandsPkg+"RegisterCommand$RegistrationParams", []typedField{{"voice", "Boolean"}, {"reregister", "Boolean"}, {"captcha", "String"}}, c.Params)
	if err != nil {
		return "", err
	}
	res := &argparse.Result{Values: map[string]any{}}
	for k, v := range p {
		res.Values[k] = v
	}
	return d.runRegistration(ctx, c.Account, "register", res)
}

func (d *daemonState) rpcVerify(ctx context.Context, c *daemon.Call) (string, error) {
	p, err := bindTyped(commandsPkg+"VerifyCommand$VerifyParams", []typedField{{"verificationCode", "String"}, {"pin", "String"}}, c.Params)
	if err != nil {
		return "", err
	}
	res := &argparse.Result{Values: map[string]any{"verification_code": p["verificationCode"], "pin": p["pin"]}}
	out, err := d.runRegistration(ctx, c.Account, "verify", res)
	if err == nil {
		d.loadAccounts()
	}
	return out, err
}

// runRegistration runs register or verify for an account that is not
// one of the daemon's open accounts.
func (d *daemonState) runRegistration(ctx context.Context, account, name string, res *argparse.Result) (string, error) {
	if la := d.account(account); la != nil {
		// The account is open in the daemon: close it so the
		// registration can lock it.
		la.mu.Lock()
		if la.sess != nil {
			la.sess.closeConns()
			la.sess = nil
		}
		la.acct.Close()
		la.mu.Unlock()
		d.forget(la)
		defer d.loadAccounts()
	}
	var out bytes.Buffer
	e := &env{stdout: &out, stderr: d.e.stderr, stdin: strings.NewReader(""), getenv: d.e.getenv}
	g := *d.g
	g.account = account
	g.output = "json"
	if err := runCommand(ctx, e, &g, name, res); err != nil {
		return "", err
	}
	return strings.TrimSpace(out.String()), nil
}

func (d *daemonState) rpcDeleteLocalAccountData(ctx context.Context, c *daemon.Call) (string, error) {
	params, err := daemon.NamespaceParams(c)
	if err != nil {
		return "", err
	}
	res, err := rpcNamespace(commandParser("deleteLocalAccountData"), params)
	if err != nil {
		return "", err
	}
	return d.runRegistration(ctx, c.Account, "deleteLocalAccountData", res)
}

// rpcStartLink starts provisioning a new linked account and returns its
// link URI (methods/startLink.md); finishLink completes it.
func (d *daemonState) rpcStartLink(ctx context.Context, c *daemon.Call) (string, error) {
	l, err := d.linker()
	if err != nil {
		return "", err
	}
	sctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	p, uri, err := l.Start(sctx)
	switch {
	case errors.Is(err, context.DeadlineExceeded) || errors.Is(err, provision.ErrNoAddress):
		return "", errcode.DeviceLinkTimedOut.Wrap(err)
	case err != nil:
		return "", errcode.LinkRequestFailed.WrapText(err)
	}
	d.mu.Lock()
	d.links[uri] = p
	d.mu.Unlock()
	return `{"deviceLinkUri":` + jackson.Quote(uri) + `}`, nil
}

func (d *daemonState) rpcFinishLink(ctx context.Context, c *daemon.Call) (string, error) {
	const class = commandsPkg + "FinishLinkCommand$FinishLinkParams"
	if c.Params == nil {
		return "", errcode.RPCNullPointer.New(class+".deviceLinkUri()", `"request"`)
	}
	p, err := bindTyped(class, []typedField{{"deviceName", "String"}, {"deviceLinkUri", "String"}}, c.Params)
	if err != nil {
		return "", err
	}
	uri, ok := p["deviceLinkUri"].(string)
	if !ok {
		return "", errcode.MissingDeviceLinkURI
	}
	if strings.ContainsAny(uri, " \"<>\\^`{|}") {
		return "", errcode.InvalidDeviceLinkURI
	}
	d.mu.Lock()
	pl := d.links[uri]
	delete(d.links, uri)
	d.mu.Unlock()
	if pl == nil {
		return "", errcode.UnknownDeviceLinkURI
	}
	name, _ := p["deviceName"].(string)
	fctx, cancel := context.WithTimeout(ctx, 150*time.Second)
	defer cancel()
	a, err := pl.Finish(fctx, name)
	switch {
	case errors.Is(err, manager.ErrUserExists):
		return "", errcode.LinkUserExists.Wrap(err)
	case errors.Is(err, context.DeadlineExceeded) || errors.Is(err, provision.ErrNoMessage):
		return "", errcode.LinkTimedOut.Wrap(err)
	case a == nil && err != nil:
		return "", errcode.LinkRequestFailed.WrapText(err)
	}
	number := a.Data.Number
	a.Close()
	d.loadAccounts()
	return `{"number":` + jackson.Quote(number) + `}`, nil
}

// linker is the provisioning flow for startLink/finishLink, with the
// post-link steps of `link`.
func (d *daemonState) linker() (*manager.Linker, error) {
	n, err := d.e.network(d.g)
	if err != nil {
		return nil, err
	}
	return &manager.Linker{
		Root: d.g.root(),
		REST: rest.New(rest.Config{Proxy: n.proxy, UserAgentOverride: n.userAgent}),
		OpenSocket: func(ctx context.Context) (*provision.Session, error) {
			return provision.Open(ctx, provision.Endpoint{Proxy: n.proxy, UserAgentOverride: n.userAgent})
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
			s := &manager.Sender{Acct: a, Chat: c}
			if err := s.SendSyncRequests(ctx); err != nil {
				d.logf("WARN  ProvisioningManagerImpl - Failed to request sync messages from linked device, data can be requested again with `sendSyncRequest`.")
			}
			return nil
		},
	}, nil
}
