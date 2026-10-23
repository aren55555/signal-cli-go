# Method: submitRateLimitChallenge

## Sources

- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/commands/SubmitRateLimitChallengeCommand.java (tag v0.14.7)
- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/jsonrpc/SignalJsonRpcCommandHandler.java (code -6)
- `index.md` conventions
- Not observed on the binary. Source reading only.

## Summary

Submits a solved captcha for a "proof required" rate-limit challenge (the challenge token comes
from a failed send result), lifting the sending rate limit.

## CLI mapping

`signal-cli -a ACCOUNT submitRateLimitChallenge --challenge TOKEN --captcha CAPTCHA`.

## Account requirement

Class **single**.

## Params

| JSON key | type | required | CLI flag | notes |
|---|---|---|---|---|
| `challenge` | string | yes (CLI) | `--challenge` | challenge token from the proof-required send failure |
| `captcha` | string | yes (CLI) | `--captcha` | captcha token (`signalcaptcha://...`); the prefix handling is done by the account layer |

Presence is not validated by the JSON-RPC layer; missing values are passed on and fail downstream
(typically -3 or -32603).

## Result

`{}`.

## Errors

| code | message | trigger |
|---|---|---|
| -32602 | account selection errors | see `index.md` |
| -6 | `Captcha rejected, it may be outdated, already used or solved from a different IP address.` | server rejected the captcha |
| -3 | `Submit challenge error: <io message>` | network failure |
| -32603 | `<message> (<simple name>)` | other |
