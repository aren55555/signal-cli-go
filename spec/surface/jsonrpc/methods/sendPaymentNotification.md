# Method `sendPaymentNotification`

## Sources

- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/commands/SendPaymentNotificationCommand.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/util/SendMessageResultUtils.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/util/CommandUtil.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/lib/src/main/java/org/asamk/signal/manager/Manager.java
- Binary observation: none. Conventions: [index.md](index.md); result: [send.md](send.md).

## Summary

Sends a (MobileCoin) payment notification carrying an opaque receipt blob and
optional note to one recipient. CLI: `signal-cli -a ACCOUNT sendPaymentNotification RECIPIENT --receipt BASE64 [--note NOTE]`.

Account requirement: **single**.

## Parameters

| JSON key(s) | Type | Required | Default | CLI flag | Notes |
|---|---|---|---|---|---|
| `recipient` | string (single) | yes | none | positional `RECIPIENT` | One recipient, index.md rule 10. Single value; no plural fallback; an array is stringified and fails as an invalid number. Missing -> -32603. |
| `receipt` | string | yes | none | `--receipt` | Standard base64 (RFC 4648 basic alphabet; trailing `=` padding optional; URL-safe alphabet, whitespace and line breaks rejected). Invalid base64 -> -32603 `<decoder text> (IllegalArgumentException)`. Missing -> -32603 (NullPointerException). |
| `note` | string | no | none | `--note` | Free-text note. |

## Result

Send result object as in [send.md](send.md), same post-send failure rules.

## Errors

| Code | Message | Trigger |
|---|---|---|
| -1 | `Invalid phone number '<input>': <reason>` | Bad recipient. |
| -1 / -4 / -5 | post-send failure | Not successful. |
| -32603 | `Failed to send message: <reason> (<ExceptionName>) (UnexpectedErrorException)` | I/O error. |
| -32603 | `<text> (IllegalArgumentException)` | Receipt not valid base64. |
| -32603 | `<text> (NullPointerException)` | Missing recipient or receipt. |
