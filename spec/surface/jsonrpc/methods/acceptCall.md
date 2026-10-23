# Method `acceptCall`

## Sources

- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/commands/AcceptCallCommand.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/lib/src/main/java/org/asamk/signal/manager/helper/CallManager.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/lib/src/main/java/org/asamk/signal/manager/util/Utils.java
- Binary observation: none (needs an account).

## Summary

Accepts an incoming call that is ringing. CLI: `signal-cli -a ACCOUNT acceptCall --call-id ID`.

## Account requirement

**single**.

## Params (namespace rules from `index.md` apply)

| JSON key(s) | type | required | CLI | notes |
|---|---|---|---|---|
| `call-id` / `callId` | JSON number | yes | `--call-id` | Any JSON number is accepted (no Integer/Long strictness here): reduced to a 64-bit value by two's-complement truncation, so both the signed id (from `callEvent`/`listCalls`) and the unsigned id (from received `callMessage`) work; fractional values are truncated. A string (even numeric), boolean, or missing value -> -1 `No call ID given` |

## Result

Same object shape as `startCall` (`callId`, `state`, `inputDeviceName`, `outputDeviceName`, `codec` `"opus"`,
`sampleRate` 48000, `channels` 1, `ptimeMs` 20). `state` is `CONNECTING`. A `callEvent` with state `CONNECTING` is also
emitted. The actual accept towards the peer is deferred until the media helper reports it is ringing.

## Errors

| code | message | when |
|---|---|---|
| -1 | `No call ID given` | key missing or not a JSON number |
| -3 | `Failed to accept call: No active call with id <unsigned id>` | no such active call (id printed as unsigned decimal) |
| -3 | `Failed to accept call: Call <signed id> is not in RINGING_INCOMING state (current: <STATE>)` | call exists but is not ringing-incoming (id printed signed) |
| -32602 | account selection errors | see `index.md` |
