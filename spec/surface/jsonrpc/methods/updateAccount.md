# Method: updateAccount

## Sources

- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/commands/UpdateAccountCommand.java (tag v0.14.7)
- https://github.com/AsamK/signal-cli/blob/v0.14.7/lib/src/main/java/org/asamk/signal/manager/internal/ManagerImpl.java (attribute update, username reservation rules)
- https://github.com/AsamK/signal-cli/blob/v0.14.7/lib/src/main/java/org/asamk/signal/manager/api/UsernameLinkUrl.java (link URL format)
- `index.md` conventions
- Not observed on the binary. Source reading only.

## Summary

Updates account attributes on the server (device name, unidentified-sender policy, phone-number
discoverability and sharing) and optionally sets or deletes the username.

## CLI mapping

`signal-cli -a ACCOUNT updateAccount [-n|--device-name NAME] [--unrestricted-unidentified-sender BOOL]
[--discoverable-by-number BOOL] [--number-sharing BOOL] [-u|--username NAME | --delete-username]`.
On the CLI the three tri-state options take `true`/`false` arguments; in JSON they are JSON booleans.

## Account requirement

Class **single**.

## Params

Namespace-style. All optional; absent/null means "leave unchanged".

| JSON key(s) | type | CLI flag | notes |
|---|---|---|---|
| `device-name` / `deviceName` | string | `-n`, `--device-name` | new name of **this** device |
| `unrestricted-unidentified-sender` / `unrestrictedUnidentifiedSender` | boolean | `--unrestricted-unidentified-sender` | allow sealed-sender messages from anyone |
| `discoverable-by-number` / `discoverableByNumber` | boolean | `--discoverable-by-number` | false = number unlisted |
| `number-sharing` / `numberSharing` | boolean | `--number-sharing` | true = share number with everybody, false = nobody |
| `username` | string | `-u`, `--username` | nickname to reserve. If it contains a `.` it is treated as an exact username (`nick.NN`), otherwise a nickname for which a numeric discriminator is chosen |
| `delete-username` / `deleteUsername` | boolean | `--delete-username` | delete the username |

Boolean params given non-boolean JSON -> -32603 `(ClassCastException)`.
The CLI's mutual exclusion of `username` and `delete-username` is **not enforced** in JSON-RPC:
if both are given, the username is set first and then deleted.

Processing order: attributes are always pushed to the server (even when no attribute param was
given) and the account identity is re-checked; then username set; then username delete.

## Result

- Without `username`: `{}`.
- With `username` (after success): object with keys (each omitted when null, never emitted as null):

| key | type | notes |
|---|---|---|
| `username` | string | full reserved username incl. discriminator, e.g. `name.42` |
| `usernameLink` | string | `https://signal.me/#eu/` + URL-safe unpadded base64 of 48 bytes (32-byte entropy + 16-byte server id UUID) |

If `delete-username` also ran after a successful set, the result object from the set is kept.

## Errors

| code | message | trigger |
|---|---|---|
| -32602 | account selection errors | see `index.md` |
| -3 | `UpdateAccount error: <io message>` | attribute update failed |
| -3 | `Failed to set username: <io message>` | network failure while setting username |
| -1 | `Invalid username: Username is malformed or already taken` | server rejected reservation |
| -1 | `Invalid username: <libsignal reason> (<exception simple name>)` | nickname invalid locally (too short, bad chars, ...) |
| -3 | `Failed to delete username: <io message>` | delete failed (if a username result was already produced it appears in `data.response`) |
| -32603 | `... (ClassCastException)` | wrong JSON type |
