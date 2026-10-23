# Method `unsubscribeReceive`

## Sources

- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/jsonrpc/SignalJsonRpcDispatcherHandler.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/jsonrpc/SignalJsonRpcCommandHandler.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/man/signal-cli-jsonrpc.5.adoc
- Binary observation: over HTTP the method returns -32601 (see `../observations.md`). Socket paths not observed (most
  require an account); exact NullPointerException texts below are therefore derived, not observed.

## Summary

Cancels a receive subscription. No CLI equivalent.

## Availability

Socket/TCP/inherited socket/`jsonRpc` stdio only. Over HTTP: -32601 `Method not implemented`.

## Account requirement

**single+multi** (in multi-account mode, if `account` is given it is stripped and the single-account rules below apply
with that account; it must exist, else -32602 `Specified account does not exist`).

## Params

`params` may be an object, an array, or absent/`null` (all three are accepted — this method is not namespace-based).

| form | meaning |
|---|---|
| object with key `subscription` | subscription id = that value converted leniently to an integer (number -> integer part; numeric string -> its value; non-numeric string, `null`, object, array -> 0; `true` -> 1, `false` -> 0) |
| array | subscription id = first element, converted the same way |
| absent or `null` | "no id" |

Kebab/camelCase variants do not apply (only `subscription`). Ids are global, so a connection can cancel another
connection's subscription if it knows its id.

Edge cases:
- Object **without** `subscription` (including `{}`, and including the case where the only key was `account` in
  multi-account mode) or an **empty array**: an internal null dereference -> -32603 with message
  `Cannot invoke "com.fasterxml.jackson.databind.JsonNode.asInt()" because the return value of "<container>.get(...)" is null (NullPointerException)`
  (derived text; Java helpful-NPE format).

## Behavior

- With an id: removes that subscription (all its accounts). Unknown id -> -1 `Unknown subscription id`.
- Without an id:
  - single-account mode: removes the subscription
    with the lowest id among those consisting of exactly one handler for this account — normally the connection's
    implicit subscription if it exists, otherwise the oldest single-account explicit one. If none exists, nothing happens
    (still success).
  - multi-account mode: -1 `Missing subscription parameter with subscription id`.

## Result

`{}` on success.

## Errors

| code | message | when |
|---|---|---|
| -1 | `Unknown subscription id` | id not currently subscribed (also for id 0 produced by lenient conversion) |
| -1 | `Missing subscription parameter with subscription id` | multi-account mode, params absent/null |
| -32601 | `Method not implemented` | HTTP |
| -32602 | `Specified account does not exist` | multi-account mode, unknown `account` |
| -32603 | NullPointerException text (see above) | object without `subscription`, or empty array |
