# Method: updateConfiguration

## Sources

- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/commands/UpdateConfigurationCommand.java (tag v0.14.7)
- https://github.com/AsamK/signal-cli/blob/v0.14.7/lib/src/main/java/org/asamk/signal/manager/api/Configuration.java
- `index.md` conventions
- Not observed on the binary. Source reading only.

## Summary

Updates the account's local configuration flags and syncs them to linked devices (primary only).

## CLI mapping

`signal-cli -a ACCOUNT updateConfiguration [--read-receipts BOOL] [--unidentified-delivery-indicators BOOL]
[--typing-indicators BOOL] [--link-previews BOOL]`.

## Account requirement

Class **single**.

## Params

All optional booleans; absent/null = unchanged; non-boolean -> -32603 `(ClassCastException)`.

| JSON key(s) | CLI flag |
|---|---|
| `read-receipts` / `readReceipts` | `--read-receipts` |
| `unidentified-delivery-indicators` / `unidentifiedDeliveryIndicators` | `--unidentified-delivery-indicators` |
| `typing-indicators` / `typingIndicators` | `--typing-indicators` |
| `link-previews` / `linkPreviews` | `--link-previews` |

## Result

`{}`.

## Errors

| code | message | trigger |
|---|---|---|
| -32602 | account selection errors | see `index.md` |
| -1 | `This command doesn't work on linked devices.` | account is a linked device |
| -32603 | `... (ClassCastException)` / other `<message> (<simple name>)` | wrong type; unexpected failure (e.g. sync send failure) |
