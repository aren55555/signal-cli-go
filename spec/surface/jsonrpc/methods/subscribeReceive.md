# Method `subscribeReceive`

## Sources

- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/jsonrpc/SignalJsonRpcDispatcherHandler.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/jsonrpc/SignalJsonRpcCommandHandler.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/http/HttpServerHandler.java (not offered over HTTP)
- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/commands/DaemonCommand.java (`--receive-mode`)
- https://github.com/AsamK/signal-cli/blob/v0.14.7/man/signal-cli-jsonrpc.5.adoc
- Binary observation: over HTTP the method returns -32601 (see `../observations.md`). Socket behavior with accounts not
  observed (needs an account).

## Summary

Starts delivering incoming messages to the current connection as `receive` notifications in the explicit
`{subscription, result}` form, and returns the new subscription id. No CLI equivalent (CLI `daemon`/`jsonRpc`
`--receive-mode manual` is the companion setting).

## Availability

Only on connection-oriented transports: UNIX socket, TCP, inherited socket, and the `jsonRpc` stdio command. Over HTTP
`POST /api/v1/rpc` it is unknown: -32601 `Method not implemented`.

## Account requirement

**single+multi**:
- Single-account mode: subscribes the daemon's account.
- Multi-account mode with `account` param: subscribes only that account (-32602 `Specified account does not exist` if
  unknown).
- Multi-account mode without `account`: subscribes all accounts loaded at that moment; accounts added later at runtime
  are automatically added to this subscription. Works with zero accounts (returns an id, delivers nothing).

## Params

None. `params` (object, array, or anything) is ignored apart from `account` in multi-account mode.

## Result

A JSON integer: the subscription id. Ids come from a single process-wide counter starting at 0 that is shared by all
connections, by receive and call-event subscriptions, and by the implicit per-connection subscription (which consumes an
id but is never reported). Example: first socket connection in default on-start mode -> implicit subscription takes 0, a
first `subscribeReceive` returns 1; a second connection's implicit subscription then takes 2, etc. With
`--receive-mode manual` there is no implicit subscription, so the first `subscribeReceive` in the process returns 0.

## Effects

- Each subscription delivers its own copy of every message: if the implicit subscription is active, a client calling
  `subscribeReceive` will get each message twice (bare and wrapped). Clients that use explicit subscriptions normally run
  the daemon with `--receive-mode manual`.
- Subscribing registers a continuous (strong) receiver for the account(s), which starts message reception if it was not
  running and makes the `receive` method fail with "already being received".
- Subscriptions end on `unsubscribeReceive`, when the connection closes, or when a notification write fails because the
  channel was closed.

## Errors

| code | message | when |
|---|---|---|
| -32601 | `Method not implemented` | over HTTP; |
| -32602 | `Specified account does not exist` | multi-account mode, unknown `account` |
