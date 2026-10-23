# signal-cli listGroups

## Sources
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/commands/ListGroupsCommand.java
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/lib/src/main/java/org/asamk/signal/manager/api/Group.java, GroupMember.java, GroupPermission.java, RecipientAddress.java
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/man/signal-cli.1.adoc, section `=== listGroups`; https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/man/signal-cli-jsonrpc.5.adoc (example)
- Binary observations:
  - `signal-cli --config <CONFIG> listGroups --help`
  - `signal-cli --config <CONFIG> listGroups`
  - `signal-cli --config <CONFIG> listGroups -x`
  - `signal-cli --config <CONFIG> -a +15555550123 listGroups -d`

## Summary
Help text: "List group information including names, ids, active status, blocked status and members"

The command lists groups from the local store. It makes no network call.

## Invocation category & account requirement
This is a single-account local command and needs a registered account.

## Output modes
`plain-text` (the default) and `json`. JSON output is always detailed.

## Arguments

| names | arity | value type / metavar | default | required | help text (verbatim) | notes |
|---|---|---|---|---|---|---|
| `-h`, `--help` | 0 | – | – | no | `show this help message and exit` | |
| `-d`, `--detailed` | 0 | flag | false | no | `List the members and group invite links of each group. If output=json, then this is always set` | |
| `-g`, `--group-id` | 0..n | base64 / `GROUP_ID` | – | no | `Specify one or more group IDs to show.` | There is no `--group` alias here, unlike other commands. |

## Constraints & validation
An invalid group id gives `Invalid group id: Failed to decode groupId (must be base64) "<id>": <cause>` (exit 1).

## Behavior
- Without `-g`, or with `-g` and no values, all known groups are listed. Otherwise only the given groups are listed.
- The order follows the local store and is unspecified.

## Plain-text output
Each group is printed on one line.

Non-detailed format. Note the **two spaces** before `Active:`:
`Id: <base64> Name: <title>  Active: <isMember true|false> Blocked: <true|false>`

Detailed format (`-d`):
`Id: <base64> Name: <title> Description: <description> Active: <bool> Blocked: <bool> Members: <SET> Pending members: <SET> Requesting members: <SET> Banned: <SET> Message expiration: <EXP> Link: <LINK>`

- SET is rendered the way Java renders a collection: `[a, b, c]`, or `[]` when empty. The element order is unspecified because it is a hash set.
  - A member element is its identifier: the phone number if known, otherwise the UUID.
  - If the member is an admin, `{ADMIN}` is appended.
  - If the member has a label, `(<labelEmoji><label>)` is appended. A missing part is omitted, so you get `(emoji)`, `(text)`, or `(emojitext)`.
  - Pending, requesting, and banned elements are bare identifiers.
- EXP is `disabled` when the timer is 0, otherwise `<n>s`.
- LINK is the invite-link URL, or `-` when there is none.
- A null title or description prints as `null`.

## JSON output
A JSON array with one object per group. The keys appear in this order and are all always present:

| key | type |
|---|---|
| id | string (base64) |
| name | string or null |
| description | string or null |
| isMember | boolean |
| isBlocked | boolean |
| messageExpirationTime | integer (seconds) |
| members | array of `{number: string or null, uuid: string or null, isAdmin: boolean, labelEmoji?: string, label?: string}`. `labelEmoji` and `label` are omitted when null. |
| pendingMembers | array of `{number, uuid}` |
| requestingMembers | array of `{number, uuid}` |
| admins | array of `{number, uuid}`. Deprecated. These are the members with isAdmin. |
| banned | array of `{number, uuid}` |
| permissionAddMember | `"EVERY_MEMBER"` or `"ONLY_ADMINS"` |
| permissionEditDetails | `"EVERY_MEMBER"` or `"ONLY_ADMINS"` |
| permissionSendMessage | `"EVERY_MEMBER"` or `"ONLY_ADMINS"` |
| groupInviteLink | string or null |

- The order of elements inside each array is unspecified (hash set).
- With no groups, the output is `[]`.

## Errors

| condition | stream | exact message | exit code |
|---|---|---|---|
| Unknown option | stderr | usage + `signal-cli: error: unrecognized arguments: '-x'` | 1 |
| No account / not registered (observed) | stderr | `No local users found, you first need to register or link an account` / `User +15555550123 is not registered.` | 1 |
| Invalid group id | stderr | `Invalid group id: ...` | 1 |

## --help transcript
Command: `signal-cli --config <CONFIG> listGroups --help`

exit code: 0

stdout:
```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli listGroups [-h] [-d] [-g [GROUP_ID [GROUP_ID ...]]]

named arguments:
  -h, --help             show this help message and exit
  -d, --detailed         List the members and  group  invite  links of each
                         group. If output=json, then this is always set
  -g [GROUP_ID [GROUP_ID ...]], --group-id [GROUP_ID [GROUP_ID ...]]
                         Specify one or more group IDs to show.
```

stderr:
```
```


## Observed invocations
### `signal-cli --config <CONFIG> listGroups`

exit code: 1

stdout:
```
```

stderr:
```
No local users found, you first need to register or link an account
```

### `signal-cli --config <CONFIG> listGroups -x`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli listGroups [-h] [-d] [-g [GROUP_ID [GROUP_ID ...]]]
signal-cli: error: unrecognized arguments: '-x'
```

### `signal-cli --config <CONFIG> -a +15555550123 listGroups -d`

exit code: 1

stdout:
```
```

stderr:
```
User +15555550123 is not registered.
```


## Doc vs binary discrepancies
- The jsonrpc man page example shows `members` and `admins` as arrays of plain phone-number strings. It also omits `banned`, the permission keys, and `messageExpirationTime`. In 0.14.7, `members` contains objects, and the address arrays contain `{number, uuid}` objects. The binary is authoritative.
- The man page documents `-g, --group-id` as a filter, which matches the binary.
