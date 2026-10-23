# Method `unsubscribeCallEvents`

## Sources

- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/jsonrpc/SignalJsonRpcDispatcherHandler.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/test/java/org/asamk/signal/jsonrpc/SubscribeCallEventsTest.java
- Binary observation: over HTTP the method returns -32601 (see `../observations.md`).

## Summary

Cancels a call-event subscription created by `subscribeCallEvents`. No CLI equivalent.

## Availability

Socket/TCP/inherited socket/`jsonRpc` stdio only. Over HTTP: -32601 `Method not implemented`.

## Account requirement

**single+multi**; the `account` key (multi-account mode) is stripped and only validated (-32602
`Specified account does not exist` if unknown). Behavior is otherwise identical in both modes.

## Params

Same forms and lenient id conversion as `unsubscribeReceive`: object `{"subscription": <id>}`, or array `[<id>]`, or
absent/`null`. An object without `subscription` (including `{}` or one that only contained `account`) or an empty array
causes -32603 with a Java NullPointerException message (see `unsubscribeReceive.md`).

## Behavior

- Id given and it is an active call-event subscription: removed. Only call-event ids are looked up here (a receive
  subscription id is "unknown").
- Id unknown: -1 `Unknown subscription id`.
- No id (params absent/null), in **both** single- and multi-account mode: -1
  `Missing subscription parameter with subscription id`.

## Result

`{}` on success.

## Errors

| code | message | when |
|---|---|---|
| -1 | `Missing subscription parameter with subscription id` | params absent/null |
| -1 | `Unknown subscription id` | id not an active call-event subscription |
| -32601 | `Method not implemented` | HTTP |
| -32602 | `Specified account does not exist` | multi-account mode, unknown `account` |
| -32603 | NullPointerException text | object without `subscription`, empty array |
