package cli

import (
	"context"
	"fmt"
	"math/big"
	"strconv"

	"github.com/aren55555/signal-cli-go/internal/daemon"
	"github.com/aren55555/signal-cli-go/internal/errcode"
	"github.com/aren55555/signal-cli-go/internal/jackson"
	"github.com/aren55555/signal-cli-go/internal/store"
)

// Voice calls are deferred (PLAN.md milestone 9b): signal-cli-go never
// has an active call, so the call commands behave as the reference does
// with none (spec/surface/cli/listCalls.md, acceptCall.md, hangupCall.md,
// rejectCall.md) and startCall is refused.

// runListCalls is `listCalls`.
func runListCalls(e *env, g *globals) error {
	return withAccount(g, func(*store.Account) error {
		if g.json() {
			fmt.Fprintln(e.stdout, "[]")
		} else {
			fmt.Fprintln(e.stdout, "No active calls.")
		}
		return nil
	})
}

// callNotFound is each call command's "no active call" error; the id
// is printed as an unsigned 64-bit number.
var callNotFound = map[string]errcode.Code{
	"acceptCall": errcode.CallNotFound,
	"hangupCall": errcode.HangupCallNotFound,
	"rejectCall": errcode.RejectCallNotFound,
}

// runCallAction is acceptCall, hangupCall and rejectCall.
func runCallAction(e *env, g *globals, cmd string, id int64, hasID bool) error {
	return withAccount(g, func(*store.Account) error {
		if !hasID {
			return errcode.NoCallID
		}
		return callNotFound[cmd].New(strconv.FormatUint(uint64(id), 10))
	})
}

// rpcCallID reads callId the way the reference's call methods do: any
// JSON number, truncated to 64 bits; anything else is "no call ID".
func rpcCallID(c *daemon.Call) (int64, bool, error) {
	params, err := daemon.NamespaceParams(c)
	if err != nil {
		return 0, false, err
	}
	v := lookup(params, "call-id")
	switch {
	case v == nil || !v.IsNumber():
		return 0, false, nil
	case v.Kind == jackson.BigInt:
		return new(big.Int).And(v.Big, new(big.Int).SetUint64(^uint64(0))).Int64(), true, nil
	}
	return v.AsLong(), true, nil
}

// callMethods replaces the call methods' parameter handling.
func (d *daemonState) callMethods(m map[string]*daemon.Method) {
	for _, name := range []string{"acceptCall", "hangupCall", "rejectCall"} {
		name := name
		m[name].Run = func(ctx context.Context, c *daemon.Call) (string, error) {
			id, ok, err := rpcCallID(c)
			if err != nil {
				return "", err
			}
			g := *d.g
			g.account, g.daemon = c.Account, d
			return "", runCallAction(d.e, &g, name, id, ok)
		}
	}
}

// runStartCall is `startCall`, refused: voice calls are deferred.
func runStartCall(g *globals) error {
	return withAccount(g, func(*store.Account) error { return errcode.CallsUnsupported })
}
