# Method `startCall`

## Sources

- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/commands/StartCallCommand.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/lib/src/main/java/org/asamk/signal/manager/helper/CallManager.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/lib/src/main/java/org/asamk/signal/manager/api/CallInfo.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/lib/src/main/java/org/asamk/signal/manager/api/UnregisteredRecipientException.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/docs/CALL_TUNNEL.md
- Binary observation: none (needs an account).

## Summary

Starts an outgoing 1:1 voice call. CLI: `signal-cli -a ACCOUNT startCall RECIPIENT`. Requires the external
`signal-call-tunnel` helper binary for media (located via env `SIGNAL_CALL_TUNNEL_BIN`, then
`<install dir>/bin/signal-call-tunnel`, then `PATH`).

## Account requirement

**single** (see `index.md`).

## Params (namespace rules from `index.md` apply)

| JSON key(s) | type | required | CLI | notes |
|---|---|---|---|---|
| `recipient` (or plural `recipients`, array) | string, or array of strings | yes | positional `RECIPIENT` (exactly one) | Single string is wrapped into a list. Only the **first** element is used; extras are ignored. Recipient forms per `index.md` rule 10 |

## Result

Object, all keys always present, in this order:

| key | type | notes |
|---|---|---|
| `callId` | int64 | signed 64-bit; randomly generated |
| `state` | string | state at return, normally `RINGING_OUTGOING` (could already be `ENDED` if the media helper could not be started) |
| `inputDeviceName` | string or null | null until the media helper reported its device names |
| `outputDeviceName` | string or null | same |
| `codec` | string | constant `"opus"` |
| `sampleRate` | integer | constant `48000` |
| `channels` | integer | constant `1` |
| `ptimeMs` | integer | constant `20` |

Subscribed connections also receive a `callEvent` with state `RINGING_OUTGOING`. If not answered within the ring
timeout the call ends with reason `ring_timeout`.

## Errors

| code | message | when |
|---|---|---|
| -1 | `No recipient given` | `recipient`/`recipients` missing or empty array |
| -1 | `Invalid phone number '<input>': <reason>` | unparsable recipient |
| -1 | `Recipient not registered: Unregistered user: <identifier>` | recipient not registered with Signal |
| -3 | `Failed to start call: <detail>` | IO failure |
| -32602 | account selection errors | see `index.md` |
| -32603 | `<message> (<ExceptionKind>)` | unexpected failures, e.g. plural key holding a non-array (ClassCastException) |
