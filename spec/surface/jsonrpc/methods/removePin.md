# Method: removePin

## Sources

- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/commands/RemovePinCommand.java (tag v0.14.7)
- `index.md` conventions
- Not observed on the binary. Source reading only.

## Summary

Removes the registration-lock PIN (disables registration lock and deletes the SVR data). Primary only.

## CLI mapping

`signal-cli -a ACCOUNT removePin`.

## Account requirement

Class **single**.

## Params

None (object with any keys accepted; array -> -32600).

## Result

`{}`.

## Errors

| code | message | trigger |
|---|---|---|
| -32602 | account selection errors | see `index.md` |
| -1 | `This command doesn't work on linked devices.` | linked device |
| -3 | `Remove pin error: <io message>` | server failure |
