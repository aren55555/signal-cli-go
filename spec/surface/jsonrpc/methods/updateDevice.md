# Method: updateDevice

## Sources

- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/commands/UpdateDeviceCommand.java (tag v0.14.7)
- https://github.com/AsamK/signal-cli/blob/v0.14.7/lib/src/main/java/org/asamk/signal/manager/Manager.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/lib/src/main/java/org/asamk/signal/manager/internal/ManagerImpl.java, lib/.../manager/helper/AccountHelper.java
- `index.md` conventions
- Not observed on the binary. Source reading only.

## Summary

Renames a device of the account. Renaming the device this signal-cli instance runs as is allowed
from any device (primary or linked); renaming any other device requires the primary device.

## CLI mapping

`signal-cli -a ACCOUNT updateDevice -d|--device-id|--deviceId DEVICE_ID -n|--device-name NAME`.

## Account requirement

Class **single**.

## Params

| JSON key(s) | type | required | default | CLI flag | notes |
|---|---|---|---|---|---|
| `device-id` / `deviceId` | integer (32-bit) | yes | none | `-d`, `--device-id`, `--deviceId` | same strictness as `removeDevice` |
| `device-name` / `deviceName` | string | yes (by CLI) | none | `-n`, `--device-name` | stringified; not checked for presence in JSON-RPC (a missing name is passed on as null and the outcome is decided by the account layer) |

## Result

`{}`.

## Errors

| code | message | trigger |
|---|---|---|
| -32602 | account selection errors | see `index.md` |
| -1 | `This command doesn't work on linked devices.` | target is another device and this account is not the primary |
| -3 | `Error while updating device: <io message>` | network / server error |
| -32603 | `... (ClassCastException)` / `... (NullPointerException)` | bad / missing `device-id`; possibly missing name |
