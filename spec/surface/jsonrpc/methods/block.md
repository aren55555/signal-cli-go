# Method `block`

## Sources

- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/commands/BlockCommand.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/lib/src/main/java/org/asamk/signal/manager/internal/ManagerImpl.java (block semantics)
- Binary observation: none. Conventions: [index.md](index.md#parameter-conventions).

## Summary

Blocks contacts and/or groups. For each newly blocked item a "block" message
request response is synced to linked devices; the profile key may be rotated;
the blocked list is synced and storage-service sync is triggered.

CLI equivalent: `signal-cli -a ACCOUNT block [RECIPIENT ...] [-g GROUP_ID ...]`.

Account requirement: **single**.

## Params

| JSON key(s) | Type | Req. | Default | CLI flag | Notes |
|---|---|---|---|---|---|
| `recipient` / `recipients` | list of string | no | none | positional | Recipients (index.md rule 10). |
| `group-id` / `groupId` / `group-ids` / `groupIds` | list of string | no | none | `-g/--group-id/--group` | Base64 group ids. |

Giving neither is not an error (no-op, result `{}`). Recipients are processed
first, then groups. Already-blocked items are skipped.

## Result

`{}`.

## Errors

| Code | Message | Trigger |
|---|---|---|
| -1 | `Invalid phone number '<input>': <reason>` | bad recipient (checked before anything is done) |
| -1 | `This command doesn't work on linked devices.` | account is not the primary device |
| -1 | `The user <identifier> is not registered.` | recipient not a Signal user |
| -1 | `Invalid group id: Failed to decode groupId (must be base64) "<id>": <reason>` | bad group id (checked after recipients were already blocked) |
| -32603 | `Failed to sync block to linked devices: <msg> (UnexpectedErrorException)` | I/O error |
| (none) | — | unknown group id: only a warning is logged; result is still `{}` |
