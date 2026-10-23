# Method `listCalls`

## Sources

- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/commands/ListCallsCommand.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/lib/src/main/java/org/asamk/signal/manager/helper/CallManager.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/lib/src/main/java/org/asamk/signal/manager/api/CallInfo.java
- Binary observation: none (needs an account).

## Summary

Lists the account's active voice calls. CLI: `signal-cli -a ACCOUNT -o json listCalls`.

## Account requirement

**single**.

## Params

None (ignored).

## Result

JSON array (empty `[]` when no calls; order unspecified), one object per call, all keys always present in this order:

| key | type | notes |
|---|---|---|
| `callId` | int64 | signed 64-bit |
| `state` | string | `IDLE`, `RINGING_INCOMING`, `RINGING_OUTGOING`, `CONNECTING`, `CONNECTED`, `RECONNECTING`, `ENDED` (ended calls are normally already removed) |
| `number` | string or null | peer phone number if known |
| `uuid` | string or null | peer ACI if known |
| `isOutgoing` | boolean | |
| `inputDeviceName` | string or null | |
| `outputDeviceName` | string or null | |

Note the difference from the `callEvent` payload, which omits absent optional keys instead of emitting `null`.

## Errors

Only account selection errors (-32602, see `index.md`).
