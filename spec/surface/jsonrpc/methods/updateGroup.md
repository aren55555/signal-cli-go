# Method `updateGroup`

## Sources

- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/commands/UpdateGroupCommand.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/util/SendMessageResultUtils.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/json/JsonSendMessageResult.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/json/JsonRecipientAddress.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/util/CommandUtil.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/lib/src/main/java/org/asamk/signal/manager/api/ (GroupNotFoundException, NotAGroupMemberException, GroupSendingNotAllowedException, AttachmentInvalidException, UnregisteredRecipientException, RecipientAddress, SendMessageResult)
- Binary observation: none (requires a registered account). Shared conventions: [index.md](index.md#parameter-conventions).

## Summary

Creates a new group (when no group id is given) or updates an existing group:
name, description, avatar, members, admins, bans, group link, permissions,
disappearing-message timer, and the caller's own member label.

CLI equivalent: `signal-cli -a ACCOUNT updateGroup [...]`.

Account requirement: **single**.

## Params

| JSON key(s) | Type | Req. | Default | CLI flag | Notes |
|---|---|---|---|---|---|
| `group-id` / `groupId` | string | no | absent = create a new group | `-g/--group-id/--group` | Base64 group id (index.md rule 11). Not a list. |
| `name` | string | no | unchanged | `-n/--name` | Group title. For a new group it is the initial title. |
| `description` | string | no | unchanged | `-d/--description` | |
| `avatar` | string | no | unchanged | `-a/--avatar` | Local file path (on the daemon host) of an image. |
| `member` / `members` | list of string | no | none | `-m/--member` | Recipients to add (index.md rule 10). For a new group these are the initial members. |
| `remove-member` / `removeMember` / `remove-members` / `removeMembers` | list of string | no | none | `-r/--remove-member` | Recipients to remove. |
| `admin` / `admins` | list of string | no | none | `--admin` | Members to promote to admin. |
| `remove-admin` / `removeAdmin` / `remove-admins` / `removeAdmins` | list of string | no | none | `--remove-admin` | Admins to demote. |
| `ban` / `bans` | list of string | no | none | `--ban` | Recipients to ban. |
| `unban` / `unbans` | list of string | no | none | `--unban` | Recipients to unban. |
| `reset-link` / `resetLink` | boolean | no | false | `--reset-link` | Reset group link password. |
| `link` | string | no | unchanged | `--link` | One of `enabled`, `enabled-with-approval` (also `enabledWithApproval`), `disabled`. Case-sensitive. |
| `set-permission-add-member` / `setPermissionAddMember` | string | no | unchanged | `--set-permission-add-member` | `every-member` (also `everyMember`) or `only-admins` (also `onlyAdmins`). |
| `set-permission-edit-details` / `setPermissionEditDetails` | string | no | unchanged | `--set-permission-edit-details` | same values |
| `set-permission-send-messages` / `setPermissionSendMessages` | string | no | unchanged | `--set-permission-send-messages` | same values; `only-admins` makes it an announcement group, `every-member` turns that off. |
| `expiration` | integer (32-bit) | no | unchanged | `-e/--expiration` | Disappearing-message timer in seconds. Must be a JSON integer that fits in 32 bits (index.md rule 6). |
| `member-label-emoji` / `memberLabelEmoji` | string | no | unchanged | `--member-label-emoji` | Emoji of the caller's own member label. |
| `member-label` / `memberLabel` | string | no | unchanged | `--member-label` | Text of the caller's own member label. |

Note: the CLI aliases `--group`, short flags, and plural forms other than the
`dest+"s"` rule are not JSON keys. Note the plural for `ban` is `bans` and for
`unban` is `unbans` (mechanical rule, not English).

Validation order: group id is decoded first; then every recipient list is
parsed (invalid number -> -1); then `link` and the three permission values
are validated; then the operation runs.

Behavior: when no group id is given, a new group is created with `name`,
`member` and `avatar`, then the remaining update fields (description,
admins, link, permissions, expiration, labels, etc.) are applied to the newly
created group. When a group id is given, all provided fields are applied to it.

## Result

A JSON object. Keys present depend on what happened:

| Key | Type | Present when |
|---|---|---|
| `timestamp` | integer (ms epoch, 64-bit) | at least one group update message was sent (creation and/or update produced send results) |
| `results` | array of SendMessageResult objects | same condition as `timestamp` |
| `groupId` | string (base64) | only when a new group was created |

If an existing group was updated and nothing needed to be sent, the result is
`{}`. When creating and updating both produced results, `timestamp` is the
update's timestamp and `results` is the concatenation of the creation results
followed by the update results.

SendMessageResult object (group form; no `groupId` key is emitted in the
items for this method):

| Key | Type | Nullability |
|---|---|---|
| `recipientAddress` | object `{ "uuid": string\|null, "number": string\|null, "username": string\|null }` (all three keys always present) | never null |
| `type` | string enum: `SUCCESS`, `NETWORK_FAILURE`, `UNREGISTERED_FAILURE`, `IDENTITY_FAILURE`, `RATE_LIMIT_FAILURE`, `INVALID_PRE_KEY_FAILURE` | never null |
| `token` | string | omitted unless a proof-required (captcha) failure; then the challenge token |
| `retryAfterSeconds` | integer | omitted unless a rate-limit/proof-required failure carries a retry-after; value is ceil(ms/1000) |

Type precedence when several flags apply: success, then rate-limit (which
includes proof-required), then network, unregistered, invalid-prekey, and
finally identity failure as the fallback.

Unlike `send`, partial or total send failure does **not** produce an error for
this method; failures are visible only in `results`.

## Errors

| Code | Message | Trigger |
|---|---|---|
| -1 | `Invalid group id: Failed to decode groupId (must be base64) "<id>": <reason>` | bad `group-id` |
| -1 | `Invalid phone number '<input>': <reason>` | bad entry in any recipient list |
| -1 | `Invalid group link state: <value>` | `link` not one of the accepted values |
| -1 | `Invalid group permission: <value>` | permission value not accepted |
| -1 | `Failed to add avatar attachment for group": <reason>` | avatar file invalid/unreadable (note the stray `"` after `group` is part of the message) |
| -1 | `Group not found: <base64 id>` | unknown group id |
| -1 | `User is not a member in group: <name> (<base64 id>)` | not a member |
| -1 | `User is not allowed to send message to group: <name> (<base64 id>)` | sending not allowed |
| -1 | `The user <identifier> is not registered.` | a given member is not a Signal user; identifier is ACI if known, else number, else PNI, else username |
| -32603 | `Failed to send message: <msg> (<ExceptionClass>) (UnexpectedErrorException)` | I/O error while talking to the server |
| -32603 | ClassCastException text + ` (ClassCastException)` | wrong JSON type for a boolean/int param or non-array under a plural key |
| -32602 / -32601 | see ../errors.md | account selection failures |
