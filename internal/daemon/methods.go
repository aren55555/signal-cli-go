// SPDX-License-Identifier: GPL-3.0-or-later

package daemon

import (
	"context"
	"strconv"

	"github.com/aren55555/signal-cli-go/internal/errcode"
	"github.com/aren55555/signal-cli-go/internal/jackson"
)

// SubscriptionMethods are the JSON-RPC-only methods subscribeReceive,
// unsubscribeReceive, subscribeCallEvents and unsubscribeCallEvents
// (methods/subscribeReceive.md, unsubscribeReceive.md and the call-event
// pair). OnSubscribe, if set, runs when a receive subscription is added
// (receive mode on-connection starts receiving then).
func SubscriptionMethods(onSubscribe func(account string)) map[string]*Method {
	subscribe := func(callEvents bool) *Method {
		return &Method{Class: Both, NoHTTP: true, Inline: true, Run: func(ctx context.Context, c *Call) (string, error) {
			id := c.Conn.Subscribe(callEvents, c.Account)
			if !callEvents && onSubscribe != nil {
				onSubscribe(c.Account)
			}
			return strconv.Itoa(id), nil
		}}
	}
	unsubscribe := func(callEvents bool) *Method {
		return &Method{Class: Both, NoHTTP: true, Inline: true, Run: func(ctx context.Context, c *Call) (string, error) {
			var id int32
			switch {
			case c.Params == nil:
				if c.Server.SingleAccount == "" {
					return "", errcode.RPCSubscriptionMissing
				}
				if !c.Conn.Unsubscribe(callEvents, 0, true) {
					return "", errcode.RPCSubscriptionUnknown
				}
				return "", nil
			case c.Params.Kind == jackson.Object:
				n := c.Params.Get("subscription")
				if n == nil {
					return "", errcode.RPCNullPointer.New("com.fasterxml.jackson.databind.JsonNode.asInt()", `the return value of "com.fasterxml.jackson.databind.node.ObjectNode.get(String)"`)
				}
				id = n.AsInt()
			default:
				if len(c.Params.Elems) == 0 {
					return "", errcode.RPCNullPointer.New("com.fasterxml.jackson.databind.JsonNode.asInt()", `the return value of "com.fasterxml.jackson.databind.node.ArrayNode.get(int)"`)
				}
				id = c.Params.Elems[0].AsInt()
			}
			if !c.Conn.Unsubscribe(callEvents, int(id), false) {
				return "", errcode.RPCSubscriptionUnknown
			}
			return "", nil
		}}
	}
	return map[string]*Method{
		"subscribeReceive":      subscribe(false),
		"unsubscribeReceive":    unsubscribe(false),
		"subscribeCallEvents":   subscribe(true),
		"unsubscribeCallEvents": unsubscribe(true),
	}
}
