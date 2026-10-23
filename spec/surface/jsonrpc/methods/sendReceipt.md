# Method `sendReceipt`

## Sources

- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/commands/SendReceiptCommand.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/util/SendMessageResultUtils.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/util/CommandUtil.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/lib/src/main/java/org/asamk/signal/manager/Manager.java
- Binary observation: none (needs a registered account). Conventions: [index.md](index.md); result: [send.md](send.md).

## Summary

Sends a read (default) or viewed receipt for one or more messages from one
sender. CLI: `signal-cli -a ACCOUNT sendReceipt RECIPIENT -t TIMESTAMP... [--type read|viewed]`.

Account requirement: **single**.

## Parameters

| JSON key(s) | Type | Required | Default | CLI flag | Notes |
|---|---|---|---|---|---|
| `recipient` | string (single) | yes | none | positional `RECIPIENT` | Single value read as a string (index.md rule 4; there is NO plural `recipients` fallback and an array would be stringified to `[a, b]` and fail as an invalid number). Parsed per index.md rule 10. Missing -> -32603 internal error. |
| `target-timestamp` / `targetTimestamp` / `target-timestamps` / `targetTimestamps` | list of long | yes (not checked) | none | `-t`, `--target-timestamp` (one or more) | Single number accepted (wrapped). Every element must be a JSON integer that is a 64-bit Long (i.e. > 2^31-1), otherwise an internal ClassCastException error. |
| `type` | string | no | `read` | `--type` | Exactly `read` or `viewed` (case-sensitive). |

## Result

The send result object as in [send.md](send.md), same post-send failure rules.

## Errors

| Code | Message | Trigger |
|---|---|---|
| -1 | `Invalid phone number '<input>': <reason>` | Bad recipient. |
| -1 | `Unknown receipt type: <type>` | `type` other than read/viewed. Checked after recipient parsing. |
| -1 / -4 / -5 | post-send failure (see send.md) | No successful recipient. |
| -32603 | Java exception text + ` (NullPointerException)` / ` (ClassCastException)` | Missing recipient or timestamps, non-Long timestamp elements. |
