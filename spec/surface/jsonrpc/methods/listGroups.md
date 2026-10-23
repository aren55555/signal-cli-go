# Method `listGroups`

## Sources

- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/commands/ListGroupsCommand.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/lib/src/main/java/org/asamk/signal/manager/api/Group.java, GroupMember.java, GroupPermission.java, RecipientAddress.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/lib/src/main/java/org/asamk/signal/manager/internal/ManagerImpl.java (group lookup)
- https://github.com/AsamK/signal-cli/blob/v0.14.7/lib/src/main/java/org/asamk/signal/manager/helper/GroupHelper.java (filtering by id)
- Binary observation: none. Conventions: [index.md](index.md#parameter-conventions).

## Summary

Lists the account's groups with full details. In JSON-RPC the result is always
the detailed form (the CLI `--detailed` flag only affects plain-text output).

CLI equivalent: `signal-cli -a ACCOUNT -o json listGroups [-d] [-g GROUP_ID ...]`.

Account requirement: **single**.

## Params

| JSON key(s) | Type | Req. | Default | CLI flag | Notes |
|---|---|---|---|---|---|
| `group-id` / `groupId` / `group-ids` / `groupIds` | list of string | no | all groups | `-g/--group-id` | Base64 ids. Unknown ids are silently omitted from the result. An empty list means all groups. |
| `detailed` | boolean | no | false | `-d/--detailed` | Ignored for JSON output (not even read). |

## Result

JSON array of group objects (possibly empty). Each object has exactly these
keys, all always present:

| Key | Type | Notes |
|---|---|---|
| `id` | string | base64 group id |
| `name` | string \| null | group title |
| `description` | string \| null | |
| `isMember` | boolean | self is a full member |
| `isBlocked` | boolean | |
| `messageExpirationTime` | integer | seconds; 0 = disabled |
| `members` | array of member objects | full members |
| `pendingMembers` | array of address objects | invited members |
| `requestingMembers` | array of address objects | join requests |
| `admins` | array of address objects | (deprecated) members with admin role |
| `banned` | array of address objects | |
| `permissionAddMember` | string | `EVERY_MEMBER` or `ONLY_ADMINS` |
| `permissionEditDetails` | string | `EVERY_MEMBER` or `ONLY_ADMINS` |
| `permissionSendMessage` | string | `EVERY_MEMBER` or `ONLY_ADMINS` |
| `groupInviteLink` | string \| null | `https://signal.group/#...` when the link is enabled, else null |

Member object: `{ "number": string|null, "uuid": string|null, "isAdmin": boolean }`
plus `labelEmoji` (string) and `label` (string), each **omitted when null**.

Address object: `{ "number": string|null, "uuid": string|null }` (both keys
always present).

Arrays derived from sets have no defined order.

## Errors

| Code | Message | Trigger |
|---|---|---|
| -1 | `Invalid group id: Failed to decode groupId (must be base64) "<id>": <reason>` | bad id |
| -32603 | ClassCastException text | non-array under plural key, non-boolean `detailed` is not read so never fails |
