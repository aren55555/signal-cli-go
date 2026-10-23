# Method `sendAdminDelete`

## Sources

- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/commands/SendAdminDeleteCommand.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/util/SendMessageResultUtils.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/util/CommandUtil.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/lib/src/main/java/org/asamk/signal/manager/Manager.java
- Binary observation: none. Conventions: [index.md](index.md); result: [send.md](send.md).

## Summary

As a group admin, deletes another member's message (or story) in one or more
groups. CLI: `signal-cli -a ACCOUNT sendAdminDelete -g GROUP... -a AUTHOR -t TIMESTAMP [--notify-self] [--story]`.

Account requirement: **single**.

## Parameters

| JSON key(s) | Type | Required | Default | CLI flag | Notes |
|---|---|---|---|---|---|
| `group-id` / `groupId` / `group-ids` / `groupIds` | list of string | yes | none | `-g`, `--group-id`, `--group` (one or more) | index.md rule 11. Only groups are targets; there are no individual recipient params. |
| `notify-self` / `notifySelf` | bool | no | false | `--notify-self` | |
| `target-author` / `targetAuthor` | string | yes (not checked) | none | `-a`, `--target-author` | Author of the message to delete, index.md rule 10. No defaulting. Missing -> -32603. |
| `target-timestamp` / `targetTimestamp` | long | yes (not checked) | none | `-t`, `--target-timestamp` | Missing -> -32603 at send time. |
| `story` | bool | no | false | `--story` | Delete a story instead of a message. |

## Result

Send result object as in [send.md](send.md) (entries carry `groupId`), same post-send failure rules.

## Errors

| Code | Message | Trigger |
|---|---|---|
| -1 | `Invalid group id: Failed to decode groupId (must be base64) "<input>": <reason>` | Bad group id. |
| -1 | `Admin delete requires group IDs` | No group ids given. |
| -1 | `Invalid phone number '<input>': <reason>` | Bad target author. |
| -1 | group error texts (send.md) | Group not found / not a member / not allowed. |
| -1 | `The user <identifier> is not registered.` | Unregistered target author. |
| -1 / -4 / -5 | post-send failure | No successful recipient. |
| -32603 | `Failed to send message: <reason> (<ExceptionName>) (UnexpectedErrorException)` | I/O error. |
| -32603 | `<text> (NullPointerException)` / `(ClassCastException)` | Missing author/timestamp, type mismatch. |
