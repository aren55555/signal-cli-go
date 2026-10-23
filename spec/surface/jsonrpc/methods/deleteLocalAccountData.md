# Method: deleteLocalAccountData

## Sources

- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/commands/DeleteLocalAccountDataCommand.java (tag v0.14.7)
- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/commands/JsonRpcNamespace.java (key lookup)
- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/jsonrpc/SignalJsonRpcCommandHandler.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/lib/src/main/java/org/asamk/signal/manager/internal/MultiAccountManagerImpl.java, lib/.../SignalAccountFiles.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/lib/src/main/java/org/asamk/signal/manager/internal/RegistrationManagerImpl.java
- Not observed on the binary. Source reading only.

## Summary

Deletes all local data (account file, database, keys, attachments cache) for an account. Refuses
while the account is still registered unless forced. Does not contact the server.

## CLI mapping

`signal-cli -a ACCOUNT deleteLocalAccountData [--ignore-registered]`.

## Account requirement

Class **registration** (multi-account mode only, mandatory `account` key, see `register.md`).
Note the side effect of the registration class: if no local account exists for the given number,
one is **created** first and then deleted again by this method (the net effect for an unknown
number is no local data, result `{}`).

## Params

Namespace-style (see `index.md` conventions: dash or camelCase keys, unknown keys ignored, array
params rejected with -32600).

| JSON key(s) | type | required | default | CLI flag | notes |
|---|---|---|---|---|---|
| `ignore-registered` / `ignoreRegistered` | boolean | no | false | `--ignore-registered` | delete even though the account is still registered. Non-boolean -> -32603 ClassCastException |

## Result

`{}` on success. The account's manager is closed; the account disappears from `listAccounts`.

## Errors

| code | message | trigger |
|---|---|---|
| -32601 | `Method not implemented` | single-account mode |
| -32602 | `Method requires account parameter` / `Failed to load the given account` | as for `register` |
| -1 | `Not deleting account, it is still registered. Use --ignore-registered to delete it anyway.` | account registered and flag not true |
| -3 | `Deletion error: <io message>` | file system failure |
| -32603 | `<message> (ClassCastException)` | `ignore-registered` not a JSON boolean |
