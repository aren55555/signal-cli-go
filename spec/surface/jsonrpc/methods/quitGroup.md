# Method `quitGroup`

## Sources

- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/commands/QuitGroupCommand.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/util/SendMessageResultUtils.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/json/JsonSendMessageResult.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/lib/src/main/java/org/asamk/signal/manager/api/LastGroupAdminException.java, GroupNotFoundException.java
- Binary observation: none (requires an account). Conventions: [index.md](index.md#parameter-conventions).

## Summary

Leaves a group (sends a group update removing self), optionally appointing new
admins first, and optionally deleting the local group data afterwards.

CLI equivalent: `signal-cli -a ACCOUNT quitGroup -g GROUP_ID [--delete] [--admin ...]`.

Account requirement: **single**.

## Params

| JSON key(s) | Type | Req. | Default | CLI flag | Notes |
|---|---|---|---|---|---|
| `group-id` / `groupId` | string | yes (logically) | — | `-g/--group-id/--group` (required on CLI) | Base64 group id. Single value, not a list. If missing, the operation runs with no group and fails with an internal error (-32603). |
| `delete` | boolean | no | false | `--delete` | After quitting, delete the local group data. |
| `admin` / `admins` | list of string | no | none | `--admin` | Members to promote to admin before leaving; needed if self is the last admin. |

## Result

- If the leave succeeded: `{ "timestamp": <int64 ms>, "results": [SendMessageResult...] }`
  (SendMessageResult shape as in [updateGroup.md](updateGroup.md#result); no
  per-item `groupId`). Send failures do not cause an error.
- If self was already not a member: nothing is written (informational log
  only), then `delete` is still honored; result is `{}`.

## Errors

| Code | Message | Trigger |
|---|---|---|
| -1 | `Invalid group id: Failed to decode groupId (must be base64) "<id>": <reason>` | bad group id |
| -1 | `Invalid phone number '<input>': <reason>` | bad `admin` entry |
| -1 | `Failed to send to group: Group not found: <base64 id>` | unknown group |
| -1 | `You need to specify a new admin with --admin: User is last admin in group: <name> (<base64 id>)` | self is last admin and no `admin` given |
| -1 | `The user <identifier> is not registered.` | an `admin` recipient is unregistered |
| -3 | `Failed to send message: <msg> (<ExceptionClass>)` | I/O error |
| -32603 | `<msg> (<ExceptionClass>)` | missing group id / wrong JSON types |
