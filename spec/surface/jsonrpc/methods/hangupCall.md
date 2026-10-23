# Method `hangupCall`

## Sources

- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/commands/HangupCallCommand.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/lib/src/main/java/org/asamk/signal/manager/helper/CallManager.java
- Binary observation: none (needs an account).

## Summary

Ends an active call (any state). CLI: `signal-cli -a ACCOUNT hangupCall --call-id ID`.

## Account requirement

**single**.

## Params

| JSON key(s) | type | required | CLI | notes |
|---|---|---|---|---|
| `call-id` / `callId` | JSON number | yes | `--call-id` | Same lenient numeric handling as `acceptCall` |

## Result

`{}`.

## Effects

The call is removed, a `callEvent` with state `ENDED` and reason `local_hangup` is emitted, a Signal hangup message
(type normal) is sent to the peer, and the media helper is told to hang up and terminated.

## Errors

| code | message | when |
|---|---|---|
| -1 | `No call ID given` | key missing or not a JSON number |
| -3 | `Failed to hang up call: No active call with id <unsigned id>` | no such active call |
| -32602 | account selection errors | see `index.md` |
