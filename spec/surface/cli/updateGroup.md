# signal-cli updateGroup

## Sources
- Shared send-result formats: [_send-results.md](_send-results.md)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/commands/UpdateGroupCommand.java
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/util/CommandUtil.java (group-id and recipient parsing)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/util/SendMessageResultUtils.java
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/lib/src/main/java/org/asamk/signal/manager/api/GroupId.java (and GroupIdFormatException, for the invalid-id message)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/man/signal-cli.1.adoc, section `=== updateGroup`
- Binary observations:
  - `signal-cli --config <CONFIG> updateGroup --help`
  - `signal-cli --config <CONFIG> updateGroup`
  - `signal-cli --config <CONFIG> updateGroup --link bogus`
  - `signal-cli --config <CONFIG> updateGroup --set-permission-add-member x`
  - `signal-cli --config <CONFIG> updateGroup -e x`
  - `signal-cli --config <CONFIG> -a +15555550123 updateGroup -n test`
- `jshell` (JDK 25) was used to compute the HashMap key order of the JSON result.

## Summary
Help text: "Create or update a group."

- Without `-g`, the command creates a new Groups-v2 group with a random id. It then applies any remaining changes.
- With `-g`, it updates the existing group.
- If the user is only a pending (invited) member, updating the group accepts the invitation (man page).

## Invocation category & account requirement
This is a single-account local command and needs a registered account.

## Output modes
`plain-text` (the default) and `json`.

## Arguments

| names | arity | value type / metavar | default | required | help text (verbatim) | notes |
|---|---|---|---|---|---|---|
| `-h`, `--help` | 0 | – | – | no | `show this help message and exit` | |
| `-g`, `--group-id`, `--group` | 1 | base64 string / `GROUP_ID` | – | no | `Specify the group ID.` | If absent, a new group is created. |
| `-n`, `--name` | 1 | string / `NAME` | – | no | `Specify the new group name.` | |
| `-d`, `--description` | 1 | string / `DESCRIPTION` | – | no | `Specify the new group description.` | |
| `-a`, `--avatar` | 1 | file path / `AVATAR` | – | no | `Specify a new group avatar image file` | |
| `-m`, `--member` | 0..n | recipients / `MEMBER` | – | no | `Specify one or more members to add to the group` | |
| `-r`, `--remove-member` | 0..n | recipients / `REMOVE_MEMBER` | – | no | `Specify one or more members to remove from the group` | |
| `--admin` | 0..n | recipients / `ADMIN` | – | no | `Specify one or more members to make a group admin` | |
| `--remove-admin` | 0..n | recipients / `REMOVE_ADMIN` | – | no | `Specify one or more members to remove group admin privileges` | |
| `--ban` | 0..n | recipients / `BAN` | – | no | `Specify one or more members to ban from joining the group` | |
| `--unban` | 0..n | recipients / `UNBAN` | – | no | `Specify one or more members to remove from the ban list` | |
| `--reset-link` | 0 | flag | false | no | `Reset group link and create new link password` | |
| `--link` | 1 | choice `{enabled,enabled-with-approval,disabled}` | – | no | `Set group link state, with or without admin approval` | Through JSON-RPC, the camelCase alias `enabledWithApproval` is also accepted. |
| `--set-permission-add-member` | 1 | choice `{every-member,only-admins}` | – | no | `Set permission to add new group members` | Through JSON-RPC, `everyMember` and `onlyAdmins` are also accepted. |
| `--set-permission-edit-details` | 1 | choice `{every-member,only-admins}` | – | no | `Set permission to edit group details` | Same aliases as above. |
| `--set-permission-send-messages` | 1 | choice `{every-member,only-admins}` | – | no | `Set permission to send messages` | `only-admins` turns the group into an announcement group. |
| `-e`, `--expiration` | 1 | 32-bit int / `EXPIRATION` | – | no | `Set expiration time of messages (seconds)` | 0 disables expiration. |
| `--member-label-emoji` | 1 | string / `MEMBER_LABEL_EMOJI` | – | no | `Specify the emoji for the member label.` | |
| `--member-label` | 1 | string / `MEMBER_LABEL` | – | no | `Specify the string for the member label.` | |

- Multi-value options (`0..n`) accept zero or more following tokens. They consume tokens until the next one that starts with `-`.
- A recipient is a phone number, an ACI UUID, `PNI:<uuid>`, or `u:<username>`.
- There are no positionals and no mutually exclusive groups.
- Note that `-a` here means `--avatar`. The global `-a` (account) must come before the subcommand.

## Constraints & validation
- Parse errors exit 1:
  - an invalid choice for `--link` or a `--set-permission-*` option;
  - a non-integer `-e`.
- After the account loads:
  - An invalid group id gives `Invalid group id: Failed to decode groupId (must be base64) "<id>": <cause>` (exit 1). `<cause>` is a base64 decoder message, or `Invalid group id of size <n>` when the decoded length is neither 16 nor 32 bytes.
  - An invalid recipient gives `Invalid phone number '<s>': <reason>` (exit 1).

## Behavior
1. If `-g` is absent, create a group using the name, the members, and the avatar. Report `Created new group`. The name, members, and avatar are then not applied again.
2. Apply every other given change in one update: description, member removals, admins, bans, link reset/state, permissions, expiration, and member label.
3. Combine the send results of the creation step and the update step.

Running with `-g` and no change options is valid. For a pending member it accepts the invitation; otherwise it may produce no send at all.

## Plain-text output
1. `Created new group: "<base64 groupId>"`, only when a group was created.
2. If any send result exists:
   - On failures, `Failed to send (some) messages:` followed by one error line per failure (formats as in `send`).
   - Then the timestamp as a bare integer.
3. If no change was sent and no group was created, nothing is printed.

## JSON output
A single object with optional keys:
- `timestamp`: integer, present if any message was sent.
- `results`: an array of SendMessageResult objects, present together with `timestamp`.
- `groupId`: base64 string, present only when a new group was created.

When all three are present, the serialization order (Java HashMap order, computed in jshell; not observed with the binary) is `groupId`, `results`, `timestamp`. When nothing applies, the output is `{}`.

## Errors

| condition | stream | exact message | exit code |
|---|---|---|---|
| Parse errors | stderr | usage + `signal-cli: error: argument --link: invalid choice: 'bogus' (choose from {enabled,enabled-with-approval,disabled})` etc. (re-wrapped; see Observed) | 1 |
| No account / not registered (observed) | stderr | `User +15555550123 is not registered.` | 1 |
| Invalid group id | stderr | `Invalid group id: Failed to decode groupId (must be base64) "<id>": <cause>` | 1 |
| Invalid recipient | stderr | `Invalid phone number '<s>': <reason>` | 1 |
| Avatar file invalid | stderr | `Failed to add avatar attachment for group": <msg>` (sic: stray `"`) | 1 |
| Group not found / not a member / sending not allowed | stderr | `<underlying message>` (passed through unchanged) | 1 |
| A member is not registered | stderr | `The user <identifier> is not registered.` | 1 |
| Network or I/O error | stderr | `Failed to send message: <msg> (<ExceptionClass>)` | **2** (unlike most commands, I/O here maps to exit code 2) |

## --help transcript
Command: `signal-cli --config <CONFIG> updateGroup --help`

exit code: 0

stdout:
```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli updateGroup [-h] [-g GROUP_ID] [-n NAME] [-d DESCRIPTION]
                  [-a AVATAR] [-m [MEMBER [MEMBER ...]]]
                  [-r [REMOVE_MEMBER [REMOVE_MEMBER ...]]]
                  [--admin [ADMIN [ADMIN ...]]]
                  [--remove-admin [REMOVE_ADMIN [REMOVE_ADMIN ...]]]
                  [--ban [BAN [BAN ...]]] [--unban [UNBAN [UNBAN ...]]]
                  [--reset-link]
                  [--link {enabled,enabled-with-approval,disabled}]
                  [--set-permission-add-member {every-member,only-admins}]
                  [--set-permission-edit-details {every-member,only-admins}]
                  [--set-permission-send-messages {every-member,only-admins}]
                  [-e EXPIRATION] [--member-label-emoji MEMBER_LABEL_EMOJI]
                  [--member-label MEMBER_LABEL]

named arguments:
  -h, --help             show this help message and exit
  -g GROUP_ID, --group-id GROUP_ID, --group GROUP_ID
                         Specify the group ID.
  -n NAME, --name NAME   Specify the new group name.
  -d DESCRIPTION, --description DESCRIPTION
                         Specify the new group description.
  -a AVATAR, --avatar AVATAR
                         Specify a new group avatar image file
  -m [MEMBER [MEMBER ...]], --member [MEMBER [MEMBER ...]]
                         Specify one or more members to add to the group
  -r [REMOVE_MEMBER [REMOVE_MEMBER ...]], --remove-member [REMOVE_MEMBER [REMOVE_MEMBER ...]]
                         Specify one or  more  members  to  remove from the
                         group
  --admin [ADMIN [ADMIN ...]]
                         Specify one or more members to make a group admin
  --remove-admin [REMOVE_ADMIN [REMOVE_ADMIN ...]]
                         Specify one or more members  to remove group admin
                         privileges
  --ban [BAN [BAN ...]]  Specify one or more  members  to  ban from joining
                         the group
  --unban [UNBAN [UNBAN ...]]
                         Specify one or  more  members  to  remove from the
                         ban list
  --reset-link           Reset group link and create new link password
  --link {enabled,enabled-with-approval,disabled}
                         Set  group  link  state,  with  or  without  admin
                         approval
  --set-permission-add-member {every-member,only-admins}
                         Set permission to add new group members
  --set-permission-edit-details {every-member,only-admins}
                         Set permission to edit group details
  --set-permission-send-messages {every-member,only-admins}
                         Set permission to send messages
  -e EXPIRATION, --expiration EXPIRATION
                         Set expiration time of messages (seconds)
  --member-label-emoji MEMBER_LABEL_EMOJI
                         Specify the emoji for the member label.
  --member-label MEMBER_LABEL
                         Specify the string for the member label.
```

stderr:
```
```


## Observed invocations
### `signal-cli --config <CONFIG> updateGroup`

exit code: 1

stdout:
```
```

stderr:
```
No local users found, you first need to register or link an account
```

### `signal-cli --config <CONFIG> updateGroup --link bogus`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli updateGroup [-h] [-g GROUP_ID] [-n NAME] [-d DESCRIPTION]
                  [-a AVATAR] [-m [MEMBER [MEMBER ...]]]
                  [-r [REMOVE_MEMBER [REMOVE_MEMBER ...]]]
                  [--admin [ADMIN [ADMIN ...]]]
                  [--remove-admin [REMOVE_ADMIN [REMOVE_ADMIN ...]]]
                  [--ban [BAN [BAN ...]]] [--unban [UNBAN [UNBAN ...]]]
                  [--reset-link]
                  [--link {enabled,enabled-with-approval,disabled}]
                  [--set-permission-add-member {every-member,only-admins}]
                  [--set-permission-edit-details {every-member,only-admins}]
                  [--set-permission-send-messages {every-member,only-admins}]
                  [-e EXPIRATION] [--member-label-emoji MEMBER_LABEL_EMOJI]
                  [--member-label MEMBER_LABEL]
signal-cli: error: argument --link:  invalid  choice:  'bogus' (choose from
{enabled,enabled-with-approval,disabled})
```

### `signal-cli --config <CONFIG> updateGroup --set-permission-add-member x`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli updateGroup [-h] [-g GROUP_ID] [-n NAME] [-d DESCRIPTION]
                  [-a AVATAR] [-m [MEMBER [MEMBER ...]]]
                  [-r [REMOVE_MEMBER [REMOVE_MEMBER ...]]]
                  [--admin [ADMIN [ADMIN ...]]]
                  [--remove-admin [REMOVE_ADMIN [REMOVE_ADMIN ...]]]
                  [--ban [BAN [BAN ...]]] [--unban [UNBAN [UNBAN ...]]]
                  [--reset-link]
                  [--link {enabled,enabled-with-approval,disabled}]
                  [--set-permission-add-member {every-member,only-admins}]
                  [--set-permission-edit-details {every-member,only-admins}]
                  [--set-permission-send-messages {every-member,only-admins}]
                  [-e EXPIRATION] [--member-label-emoji MEMBER_LABEL_EMOJI]
                  [--member-label MEMBER_LABEL]
signal-cli: error:  argument  --set-permission-add-member:  invalid choice:
'x' (choose from {every-member,only-admins})
```

### `signal-cli --config <CONFIG> updateGroup -e x`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli updateGroup [-h] [-g GROUP_ID] [-n NAME] [-d DESCRIPTION]
                  [-a AVATAR] [-m [MEMBER [MEMBER ...]]]
                  [-r [REMOVE_MEMBER [REMOVE_MEMBER ...]]]
                  [--admin [ADMIN [ADMIN ...]]]
                  [--remove-admin [REMOVE_ADMIN [REMOVE_ADMIN ...]]]
                  [--ban [BAN [BAN ...]]] [--unban [UNBAN [UNBAN ...]]]
                  [--reset-link]
                  [--link {enabled,enabled-with-approval,disabled}]
                  [--set-permission-add-member {every-member,only-admins}]
                  [--set-permission-edit-details {every-member,only-admins}]
                  [--set-permission-send-messages {every-member,only-admins}]
                  [-e EXPIRATION] [--member-label-emoji MEMBER_LABEL_EMOJI]
                  [--member-label MEMBER_LABEL]
signal-cli: error:  argument  -e/--expiration:  could  not  convert  'x' to
integer (32 bits)
```

### `signal-cli --config <CONFIG> -a +15555550123 updateGroup -n test`

exit code: 1

stdout:
```
```

stderr:
```
User +15555550123 is not registered.
```


## Doc vs binary discrepancies
- The man page says `-g` is the "recipient group ID in base64 encoding". The binary help says only "Specify the group ID."
- Network errors exit 2, not 3. This comes from source and is not covered by the man page's exit-code list.
