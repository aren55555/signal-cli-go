# Method `getAvatar`

## Sources

- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/commands/GetAvatarCommand.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/json/JsonAttachmentData.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/lib/src/main/java/org/asamk/signal/manager/internal/ManagerImpl.java (avatar retrieval)
- Binary observation: none. Conventions: [index.md](index.md#parameter-conventions).

## Summary

Returns a stored avatar image, base64 encoded: a contact's (system contact)
avatar, a profile avatar (profile is fetched/refreshed first), or a group
avatar.

CLI equivalent: `signal-cli -a ACCOUNT getAvatar (-c R | -p R | -g GROUP_ID)`.

Account requirement: **single**.

## Params

Exactly one is expected; precedence if several are given: `contact`, then
`profile`, then `group-id`.

| JSON key(s) | Type | Req. | Default | CLI flag | Notes |
|---|---|---|---|---|---|
| `contact` | string | one of three | — | `-c/--contact` | Recipient (index.md rule 10). |
| `profile` | string | one of three | — | `-p/--profile` | Recipient. |
| `group-id` / `groupId` | string | one of three | — | `-g/--group-id` | Base64 group id. |

If none is given, a group avatar lookup with no group is attempted, which
fails with -32603 (unexpected exception).

## Result

`{ "data": "<standard base64 of the image bytes>" }`

## Errors

| Code | Message | Trigger |
|---|---|---|
| -1 | `Invalid phone number '<input>': <reason>` | bad contact/profile recipient |
| -1 | `Invalid group id: Failed to decode groupId (must be base64) "<id>": <reason>` | bad group id |
| -1 | `Could not find avatar` | no avatar stored |
| -1 | `The user <identifier> is not registered.` | recipient unregistered |
| -32603 | `An error occurred reading avatar (UnexpectedErrorException)` | other I/O error |
| -32603 | `<msg> (<ExceptionClass>)` | none of the three params given |
