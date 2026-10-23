# Method: setPin

## Sources

- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/commands/SetPinCommand.java (tag v0.14.7)
- https://github.com/AsamK/signal-cli/blob/v0.14.7/lib/src/main/java/org/asamk/signal/manager/helper/AccountHelper.java (what setting the PIN does)
- `index.md` conventions
- Not observed on the binary. Source reading only.

## Summary

Sets the registration-lock PIN: stores the PIN-derived master key in Secure Value Recovery,
enables registration lock on the server and updates account attributes. Primary device only.

## CLI mapping

`signal-cli -a ACCOUNT setPin PIN` (positional).

## Account requirement

Class **single**.

## Params

| JSON key | type | required | CLI | notes |
|---|---|---|---|---|
| `pin` | string | yes | positional `PIN` | any scalar is stringified (a JSON number `1234` becomes `"1234"`; leading zeros are lost that way, so send a string). Missing/null -> -32603 `null (NullPointerException)` |

## Result

`{}`.

## Errors

| code | message | trigger |
|---|---|---|
| -32602 | account selection errors | see `index.md` |
| -1 | `This command doesn't work on linked devices.` | linked device |
| -3 | `Set pin error: <io message>` | SVR / server failure |
| -32603 | `null (NullPointerException)` | `pin` missing |
