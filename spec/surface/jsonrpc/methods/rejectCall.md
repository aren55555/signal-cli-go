# Method `rejectCall`

## Sources

- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/commands/RejectCallCommand.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/lib/src/main/java/org/asamk/signal/manager/helper/CallManager.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/lib/src/main/java/org/asamk/signal/manager/internal/ManagerImpl.java
- Binary observation: none (needs an account).

## Summary

Rejects an active (typically incoming, ringing) call by sending a "busy" call message to the peer.
CLI: `signal-cli -a ACCOUNT rejectCall --call-id ID`.

## Account requirement

**single**.

## Params

| JSON key(s) | type | required | CLI | notes |
|---|---|---|---|---|
| `call-id` / `callId` | JSON number | yes | `--call-id` | Same lenient numeric handling as `acceptCall` |

## Result

`{}` (the busy message's send result is not reported, and a failed busy send is not an error).

## Effects

Busy message sent to the peer device, call removed, `callEvent` with state `ENDED`, reason `rejected`. No hangup message
is sent in addition.

## Errors

| code | message | when |
|---|---|---|
| -1 | `No call ID given` | key missing or not a JSON number |
| -3 | `Failed to reject call: No active call with id <unsigned id>` | no such active call |
| -32602 | account selection errors | see `index.md` |
