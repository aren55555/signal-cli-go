# Method: listDevices

## Sources

- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/commands/ListDevicesCommand.java (tag v0.14.7)
- https://github.com/AsamK/signal-cli/blob/v0.14.7/lib/src/main/java/org/asamk/signal/manager/api/Device.java
- `index.md` conventions
- Not observed on the binary (needs a registered account; fetches from server). Source reading only.

## Summary

Lists the devices registered for the account (fetched from the server), including this device.

## CLI mapping

`signal-cli -a ACCOUNT listDevices`.

## Account requirement

Class **single**.

## Params

None (any object accepted, unknown keys ignored; array params -> -32600).

## Result

JSON array; one object per device, in server order:

| key | type | nullable | notes |
|---|---|---|---|
| `id` | integer | no | device id (1 = primary) |
| `name` | string | **yes** (`null` emitted, key always present) | decrypted device name; null if none |
| `createdTimestamp` | integer | no | epoch milliseconds |
| `lastSeenTimestamp` | integer | no | epoch milliseconds (server granularity is a day) |

There is no "this device" marker in JSON output (only the plain-text CLI output shows
`(this device)`).

## Errors

| code | message | trigger |
|---|---|---|
| -32602 | account selection errors | see `index.md` |
| -3 | `Failed to get linked devices: <io message>` | network failure |
| -32603 | `<message> (<exception simple name>)` | other |
