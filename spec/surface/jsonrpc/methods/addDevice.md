# Method: addDevice

## Sources

- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/commands/AddDeviceCommand.java (tag v0.14.7)
- https://github.com/AsamK/signal-cli/blob/v0.14.7/lib/src/main/java/org/asamk/signal/manager/api/DeviceLinkUrl.java (URI parsing rules)
- https://github.com/AsamK/signal-cli/blob/v0.14.7/lib/src/main/java/org/asamk/signal/manager/Manager.java
- `index.md` conventions (namespace params, account selection)
- Not observed on the binary (needs a registered account). Source reading only.

## Summary

On a primary device, approves a new linked device given the `sgnl://linkdevice?...` URI the new
device displays (e.g. the result of another instance's `startLink`).

## CLI mapping

`signal-cli -a ACCOUNT addDevice --uri URI`.

## Account requirement

Class **single** (see `index.md`).

## Params

Namespace-style (dash/camelCase keys, unknown keys ignored, array params -> -32600).

| JSON key | type | required | default | CLI flag | notes |
|---|---|---|---|---|---|
| `uri` | string | yes | none | `--uri` | any JSON scalar is stringified |

URI validation, in order:
1. Must parse as a URI, else -1 `Device link uri has invalid format: <URI parser message>`.
2. Must have a non-empty raw query string, else -32603 `Invalid device link uri (RuntimeException)`.
3. Query must contain non-empty `uuid` and `pub_key` parameters, else -1 `Invalid device link`.
   (The underlying reason text "Invalid device link uri" is not surfaced; the user-facing message
   is just `Invalid device link`.)
4. `pub_key` must decode as standard base64 (padding optional), else -1 `Invalid device link`.
5. Decoded bytes must be a valid EC public key (33 bytes, type byte 0x05), else -1 `Invalid device link`.

Scheme/host are not checked. Query values are percent-decoded.

## Result

`{}` on success.

## Errors

| code | message | trigger |
|---|---|---|
| -32602 | `Method requires valid account parameter` / `Specified account does not exist` | account selection (multi-account mode) |
| -1 | `Device link uri has invalid format: <reason>` | unparsable URI |
| -1 | `Invalid device link` | missing uuid/pub_key, bad base64, bad key |
| -1 | `Account has too many linked devices already` | server device limit |
| -1 | `This command doesn't work on linked devices.` | the account is itself a linked (non-primary) device |
| -3 | `Add device link failed` | network/I/O failure (the I/O detail is only logged) |
| -32603 | `Invalid device link uri (RuntimeException)` | URI without query |
| -32603 | message ending ` (NullPointerException)` | `uri` missing/null |
