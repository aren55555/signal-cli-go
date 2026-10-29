// SPDX-License-Identifier: GPL-3.0-or-later

// Package daemon serves signal-cli's JSON-RPC API over line-delimited
// streams (stdin/stdout, UNIX sockets, TCP) and HTTP
// (spec/surface/jsonrpc/transports.md, errors.md). Methods are supplied
// by the caller; the package owns request validation, account
// selection, subscriptions and the transports.
package daemon

import (
	"context"
	"errors"
	"io"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/aren55555/signal-cli-go/internal/errcode"
	"github.com/aren55555/signal-cli-go/internal/jackson"
)

// Class is a method's account requirement (methods/index.md).
type Class int

const (
	Single       Class = iota // needs one account
	Multi                     // works on the account set; multi-account mode only
	Both                      // one account if given, else all
	Registration              // multi-account mode only; account key mandatory
)

// Method is one JSON-RPC method.
type Method struct {
	Class Class
	// NoHTTP methods (the subscription methods) do not exist over HTTP.
	NoHTTP bool
	// Inline methods run in the reading goroutine, so their effects
	// (subscription ids) follow the input order.
	Inline bool
	// Run executes the call. Its result is encoded JSON; "" means {}.
	// An error that already produced a result carries it as
	// errcode.Error.Data.
	Run func(ctx context.Context, c *Call) (string, error)
}

// Call is one method invocation.
type Call struct {
	Method string
	// Account is the selected account's number; "" when the method runs
	// for all accounts (or none was needed).
	Account string
	// Params is the params object or array with "account" removed when
	// it was consumed; nil when absent or null.
	Params *jackson.Node
	Conn   *Conn // the stream connection; nil over HTTP
	Server *Server
}

// Account is a local account the daemon serves.
type Account struct {
	Number string
	ACI    string
}

// Server dispatches JSON-RPC requests.
type Server struct {
	Methods map[string]*Method
	// Accounts lists the local accounts (read on each request, so
	// accounts added at runtime are seen).
	Accounts func() []Account
	// SingleAccount is the account number in single-account mode; ""
	// means multi-account mode.
	SingleAccount string
	// AutoSubscribe gives each stream connection the implicit receive
	// subscription (receive modes on-start and on-connection).
	AutoSubscribe bool
	// Log receives diagnostics (signal-cli's stderr log lines).
	Log io.Writer

	nextSub atomic.Int64

	mu        sync.Mutex
	conns     map[*Conn]bool
	listeners map[*listener]bool
}

func (s *Server) logf(line string) {
	if s.Log != nil {
		io.WriteString(s.Log, line+"\n")
	}
}

// newSubID takes the next id from the process-wide counter.
func (s *Server) newSubID() int { return int(s.nextSub.Add(1) - 1) }

func (s *Server) findAccount(id string) (Account, bool) {
	for _, a := range s.Accounts() {
		if a.Number == id || (a.ACI != "" && a.ACI == id) {
			return a, true
		}
	}
	return Account{}, false
}

// request is a validated JSON-RPC request.
type request struct {
	method string
	params *jackson.Node // object or array; nil if absent or null
	id     *jackson.Node // nil: notification
}

const (
	nodePkg        = "com.fasterxml.jackson.databind.node."
	valueNodeClass = nodePkg + "ValueNode"
	containerClass = nodePkg + "ContainerNode"
)

// parseRequest binds an object to JsonRpcRequest as Jackson does:
// members in document order, the first problem wins.
func parseRequest(obj *jackson.Node) (*request, error) {
	req := &request{}
	var version *string
	var method *string
	for i, k := range obj.Keys {
		v := obj.Vals[i]
		switch k {
		case "jsonrpc", "method":
			if v.IsContainer() {
				return nil, MemberTypeError("java.lang.String", v, "org.asamk.signal.jsonrpc.JsonRpcRequest", k)
			}
			var p *string
			if v.Kind != jackson.Null {
				t := v.AsText()
				p = &t
			}
			if k == "jsonrpc" {
				version = p
			} else {
				method = p
			}
		case "params":
			switch {
			case v.Kind == jackson.Null:
				req.params = nil
			case v.IsContainer():
				req.params = v
			default:
				return nil, errcode.RPCPropertyType.New("params", containerClass+"<java.lang.Object>", nodePkg+v.NodeClass(), containerClass, "params", nodePkg+v.NodeClass(), "params")
			}
		case "id":
			if v.IsContainer() {
				return nil, errcode.RPCPropertyType.New("id", valueNodeClass, nodePkg+v.NodeClass(), valueNodeClass, "id", nodePkg+v.NodeClass(), "id")
			}
			req.id = v
		default:
			return nil, errcode.RPCUnrecognizedField.New(k, "org.asamk.signal.jsonrpc.JsonRpcRequest", 4, `"method", "id", "jsonrpc", "params"`, "org.asamk.signal.jsonrpc.JsonRpcRequest", k)
		}
	}
	if version == nil || *version != "2.0" {
		return req, errcode.RPCVersion
	}
	if method == nil {
		return req, errcode.RPCMethodMissing
	}
	req.method = *method
	return req, nil
}

// failedID is the id of a request that failed validation (reported even
// without an id): the bound id, else the raw one.
func failedID(obj *jackson.Node, req *request) *jackson.Node {
	if req != nil {
		if req.id == nil {
			return jackson.NewNull()
		}
		return req.id
	}
	return idOf(obj)
}

// idOf is the id echoed in an error for a request that failed to bind:
// the raw id member if it is a value node.
func idOf(obj *jackson.Node) *jackson.Node {
	if id := obj.Get("id"); id != nil && !id.IsContainer() {
		return id
	}
	return jackson.NewNull()
}

func encodeID(id *jackson.Node) string {
	if id == nil {
		return "null"
	}
	return id.Encode()
}

func resultResponse(id *jackson.Node, result string) string {
	if result == "" {
		result = "{}"
	}
	return `{"jsonrpc":"2.0","result":` + result + `,"id":` + encodeID(id) + `}`
}

func errorResponse(id *jackson.Node, err error) string {
	e := errcode.As(err)
	data := e.Data
	if data == "" {
		data = "null"
	}
	return `{"jsonrpc":"2.0","error":{"code":` + itoa(e.Code.RPCCode()) + `,"message":` + jackson.Quote(errcode.RPCMessage(e)) + `,"data":` + data + `},"id":` + encodeID(id) + `}`
}

func itoa(n int) string {
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
		if n == 0 {
			break
		}
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

// handleMessage processes one parsed top-level value. emit is called at
// most once with the response line; viaHTTP selects HTTP's method set.
// Validation errors are reported before any request runs.
func (s *Server) handleMessage(ctx context.Context, conn *Conn, n *jackson.Node, viaHTTP bool, spawn func(func()), emit func(string)) {
	switch n.Kind {
	case jackson.Object:
		if n.Has("result") || n.Has("error") {
			return // a response from the client: ignored
		}
		req, err := parseRequest(n)
		if err != nil {
			emit(errorResponse(failedID(n, req), err))
			return
		}
		run := func() {
			if resp, ok := s.run(ctx, conn, req, viaHTTP); ok {
				emit(resp)
			}
		}
		if m := s.Methods[req.method]; m != nil && m.Inline {
			run()
		} else {
			spawn(run)
		}
	case jackson.Array:
		if len(n.Elems) == 0 {
			emit(errorResponse(jackson.NewNull(), errcode.RPCInvalidRequest))
			return
		}
		s.batch(ctx, conn, n.Elems, viaHTTP, spawn, emit)
	default:
		emit(errorResponse(jackson.NewNull(), errcode.RPCUnexpectedType.New(n.TypeName())))
	}
}

// batch validates every element first, then runs the valid ones
// concurrently; responses are one array, validation errors first and
// results in completion order.
func (s *Server) batch(ctx context.Context, conn *Conn, elems []*jackson.Node, viaHTTP bool, spawn func(func()), emit func(string)) {
	var out []string
	var reqs []*request
	for _, e := range elems {
		switch e.Kind {
		case jackson.Null:
			return // the reference drops a batch with a null element
		case jackson.Object:
			req, err := parseRequest(e)
			if err != nil {
				out = append(out, errorResponse(failedID(e, req), err))
				continue
			}
			reqs = append(reqs, req)
		case jackson.Array:
			out = append(out, errorResponse(jackson.NewNull(), errcode.RPCDeserializeArray.New("org.asamk.signal.jsonrpc.JsonRpcRequest")))
		default:
			out = append(out, errorResponse(jackson.NewNull(), batchScalarError(e)))
		}
	}
	if len(reqs) == 0 {
		if len(out) > 0 {
			emit("[" + strings.Join(out, ",") + "]")
		}
		return
	}
	spawn(func() {
		var mu sync.Mutex
		var wg sync.WaitGroup
		for _, r := range reqs {
			wg.Add(1)
			go func(r *request) {
				defer wg.Done()
				if resp, ok := s.run(ctx, conn, r, viaHTTP); ok {
					mu.Lock()
					out = append(out, resp)
					mu.Unlock()
				}
			}(r)
		}
		wg.Wait()
		if len(out) > 0 {
			emit("[" + strings.Join(out, ",") + "]")
		}
	})
}

func batchScalarError(e *jackson.Node) error {
	switch e.Kind {
	case jackson.String:
		return errcode.RPCBatchElementScalar.New("String", "String", "'"+e.S+"'")
	case jackson.Bool:
		return errcode.RPCBatchElementScalar.New("boolean/Boolean", "boolean", e.AsText())
	case jackson.Int:
		return errcode.RPCBatchElementScalar.New("int/Int", "Number", e.AsText())
	case jackson.Long:
		return errcode.RPCBatchElementScalar.New("long/Long", "Number", e.AsText())
	case jackson.BigInt:
		return errcode.RPCBatchElementScalar.New("BigInteger", "Number", e.AsText())
	}
	return errcode.RPCBatchElementScalar.New("double/Double", "Number", e.AsText())
}

// run executes a validated request; ok is false for notifications.
func (s *Server) run(ctx context.Context, conn *Conn, req *request, viaHTTP bool) (string, bool) {
	result, err := s.dispatch(ctx, conn, req, viaHTTP)
	if req.id == nil {
		return "", false
	}
	if err != nil {
		return errorResponse(req.id, err), true
	}
	return resultResponse(req.id, result), true
}

func (s *Server) dispatch(ctx context.Context, conn *Conn, req *request, viaHTTP bool) (result string, err error) {
	defer func() {
		if r := recover(); r != nil {
			result, err = "", errcode.Internal.New(panicText(r))
		}
	}()
	m := s.Methods[req.method]
	if m == nil || (viaHTTP && m.NoHTTP) || (s.SingleAccount != "" && (m.Class == Multi || m.Class == Registration)) {
		return "", errcode.RPCMethodNotImplemented
	}
	c := &Call{Method: req.method, Params: req.params, Conn: conn, Server: s}
	if s.SingleAccount != "" {
		if m.Class != Multi {
			c.Account = s.SingleAccount
		}
		return m.Run(ctx, c)
	}
	var accountNode *jackson.Node
	if req.params != nil && req.params.Kind == jackson.Object {
		accountNode = req.params.Get("account")
	}
	switch m.Class {
	case Single, Both:
		if accountNode != nil && accountNode.Kind != jackson.Null {
			a, ok := s.findAccount(accountNode.AsText())
			if !ok {
				return "", errcode.RPCAccountNotFound
			}
			c.Account = a.Number
			req.params.Remove("account")
		} else if m.Class == Single {
			accounts := s.Accounts()
			if len(accounts) != 1 {
				return "", errcode.RPCAccountRequired
			}
			c.Account = accounts[0].Number
			if accountNode != nil {
				req.params.Remove("account")
			}
		}
	case Registration:
		if accountNode == nil {
			return "", errcode.RPCAccountParamMissing
		}
		c.Account = accountNode.AsText()
		req.params.Remove("account")
	}
	return m.Run(ctx, c)
}

func panicText(r any) string {
	if e, ok := r.(error); ok {
		return e.Error()
	}
	if s, ok := r.(string); ok {
		return s
	}
	return "panic"
}

// NamespaceParams is the params object of a namespace-style method:
// absent or null params are an empty object; an array cannot be bound.
func NamespaceParams(c *Call) (*jackson.Node, error) {
	switch {
	case c.Params == nil:
		return jackson.NewObject(), nil
	case c.Params.Kind == jackson.Array:
		return nil, errcode.RPCDeserializeArray.New("java.util.LinkedHashMap<java.lang.String,java.lang.Object>")
	}
	return c.Params, nil
}

// accountsSorted is a stable copy for snapshots.
func accountsSorted(as []Account) []Account {
	out := append([]Account(nil), as...)
	sort.Slice(out, func(i, j int) bool { return out[i].Number < out[j].Number })
	return out
}

var errClosed = errors.New("daemon: connection closed")

// MemberTypeError is Jackson's error for a member of class owner whose
// JSON value cannot become target.
func MemberTypeError(target string, v *jackson.Node, owner, member string) error {
	var what, tok string
	switch v.Kind {
	case jackson.Object:
		what, tok = "Object", "START_OBJECT"
	case jackson.Array:
		what, tok = "Array", "START_ARRAY"
	case jackson.Double:
		what, tok = "Floating-point", "VALUE_NUMBER_FLOAT"
	case jackson.Int, jackson.Long, jackson.BigInt:
		what, tok = "Integer", "VALUE_NUMBER_INT"
	case jackson.Bool:
		what, tok = "Boolean", "VALUE_FALSE"
		if v.B {
			tok = "VALUE_TRUE"
		}
	default:
		what, tok = "String", "VALUE_STRING"
	}
	return errcode.RPCDeserializeMember.New(target, what, tok, owner, member)
}
