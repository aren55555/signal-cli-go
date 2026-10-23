# Method `subscribeCallEvents`

## Sources

- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/jsonrpc/SignalJsonRpcDispatcherHandler.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/json/JsonCallEvent.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/lib/src/main/java/org/asamk/signal/manager/helper/CallManager.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/test/java/org/asamk/signal/jsonrpc/SubscribeCallEventsTest.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/docs/CALL_TUNNEL.md
- Binary observation: over HTTP the method returns -32601 (see `../observations.md`).

## Summary

Starts delivering voice-call state changes to the current connection as `callEvent` notifications
(`{"jsonrpc":"2.0","method":"callEvent","params":{"subscription":N,"result":{...}}}`; shape in
`../notifications.md` section 10). Returns the subscription id. No CLI equivalent.

## Availability

Socket/TCP/inherited socket/`jsonRpc` stdio only. Over HTTP: -32601 `Method not implemented`. Call events are never
subscribed automatically (independent of `--receive-mode`).

## Account requirement

**single+multi**: single-account mode -> the daemon's account; multi-account with `account` -> that account
(-32602 `Specified account does not exist` if unknown); multi-account without `account` -> all loaded accounts, and
accounts added later are attached automatically.

## Params

None (ignored, apart from `account` in multi-account mode).

## Result

JSON integer subscription id, drawn from the same process-wide counter as `subscribeReceive` (see that file).

## Effects

- While at least one call-event subscription exists for an account, incoming call offers are processed (state
  `RINGING_INCOMING`, event emitted). With no subscriber, incoming offers are silently ignored.
- Subscriptions end on `unsubscribeCallEvents` or connection close. (Upstream quirk: when a call-event notification
  write fails on a closed channel, the code attempts to drop a *receive* subscription with that id rather than the
  call-event subscription; harmless in practice because the connection teardown removes everything.)

## Errors

| code | message | when |
|---|---|---|
| -32601 | `Method not implemented` | HTTP |
| -32602 | `Specified account does not exist` | multi-account mode, unknown `account` |
