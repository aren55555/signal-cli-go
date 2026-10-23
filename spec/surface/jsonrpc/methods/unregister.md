# Method: unregister

## Sources

- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/commands/UnregisterCommand.java (tag v0.14.7)
- https://github.com/AsamK/signal-cli/blob/v0.14.7/lib/src/main/java/org/asamk/signal/manager/Manager.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/lib/src/main/java/org/asamk/signal/manager/internal/ManagerImpl.java, lib/.../manager/helper/AccountHelper.java
- `index.md` conventions
- Not observed on the binary. Source reading only.

## Summary

Unregisters this device from the Signal server (primary: disables the account's push/registration;
linked device: removes itself), or with `deleteAccount` deletes the whole account on the server.

## CLI mapping

`signal-cli -a ACCOUNT unregister [--delete-account]`.

## Account requirement

Class **single**.

## Params

| JSON key(s) | type | required | default | CLI flag | notes |
|---|---|---|---|---|---|
| `delete-account` / `deleteAccount` | boolean | no | false | `--delete-account` | non-boolean -> -32603 ClassCastException |

## Result

`{}`. Primary device without `deleteAccount`: the server push token is cleared so the number stops
receiving messages. Linked device: this device stops receiving. With `deleteAccount`: the
registration-lock PIN is removed (failure only logged) and the account is deleted on the server.
Afterwards the account's local state is marked unregistered; in a single-account daemon the
account manager closes, which shuts the daemon down (the daemon exits when its single account
closes). In multi-account mode the account is removed from the daemon's set.

## Errors

| code | message | trigger |
|---|---|---|
| -32602 | account selection errors | see `index.md` |
| -3 | `Unregister error: <io message>` | network / server failure |
| -32603 | `... (ClassCastException)` | wrong type |
