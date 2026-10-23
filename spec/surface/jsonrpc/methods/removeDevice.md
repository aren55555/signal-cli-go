# Method: removeDevice

## Sources

- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/commands/RemoveDeviceCommand.java (tag v0.14.7)
- https://github.com/AsamK/signal-cli/blob/v0.14.7/lib/src/main/java/org/asamk/signal/manager/Manager.java
- `index.md` conventions
- Not observed on the binary. Source reading only.

## Summary

Unlinks a linked device from the account (primary device only).

## CLI mapping

`signal-cli -a ACCOUNT removeDevice -d|--device-id|--deviceId DEVICE_ID`.

## Account requirement

Class **single**.

## Params

| JSON key(s) | type | required | default | CLI flag | notes |
|---|---|---|---|---|---|
| `device-id` / `deviceId` | integer (32-bit) | yes | none | `-d`, `--device-id`, `--deviceId` | must be a JSON integer that fits in 32 bits. A string, fractional or >2^31-1 value -> -32603 ` (ClassCastException)`; missing -> -32603 ` (NullPointerException)` |

## Result

`{}`.

## Errors

| code | message | trigger |
|---|---|---|
| -32602 | account selection errors | see `index.md` |
| -1 | `This command doesn't work on linked devices.` | account is not the primary device |
| -3 | `Error while removing device: <io message>` | network / server error (e.g. unknown device id) |
| -32603 | `<java message> (ClassCastException)` | wrong JSON type for `device-id` |
| -32603 | `<java message> (NullPointerException)` | `device-id` missing |
