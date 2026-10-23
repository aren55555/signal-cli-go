# Method `joinGroup`

## Sources

- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/commands/JoinGroupCommand.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/lib/src/main/java/org/asamk/signal/manager/api/GroupInviteLinkUrl.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/util/SendMessageResultUtils.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/json/JsonSendMessageResult.java
- Binary observation: none. Conventions: [index.md](index.md#parameter-conventions).

## Summary

Joins a group via a group invitation link (or requests to join, when the link
requires admin approval).

CLI equivalent: `signal-cli -a ACCOUNT joinGroup --uri URI`.

Account requirement: **single**.

## Params

| JSON key(s) | Type | Req. | Default | CLI flag | Notes |
|---|---|---|---|---|---|
| `uri` | string | yes | — | `--uri` (required) | Group invite link. Accepted when the URI scheme is `https` or `sgnl` (case-insensitive) and the host is `signal.group` (case-insensitive); path must be empty or `/`; the fragment is base64 of the group-invite-link protobuf (contents V1: master key + invite password). If missing (`null`), an internal error -32603 results. |

## Result

`{ "timestamp": <int64>, "results": [SendMessageResult...], "groupId": "<base64>" }`
when the account became a full member; additionally `"onlyRequested": true`
when only a join request was sent (admin approval required). Key order is not
significant. SendMessageResult shape: see [updateGroup.md](updateGroup.md#result)
(no per-item `groupId`).

## Errors

| Code | Message | Trigger |
|---|---|---|
| -1 | `Link is not a signal group invitation link` | scheme/host not matching, or URI unparseable |
| -1 | `Group link is invalid: <reason>` | path present (`No path was expected in uri`), missing fragment (`No reference was in the uri`), or undecodable fragment (reason is the wrapped decoder exception's string form) |
| -1 | `Group link was created with an incompatible version: Url contains no known group link content` | link has no V1 contents |
| -1 | `Group link is not valid: <reason>` | server says the link is inactive/invalid |
| -1 | `Pending admin approval: <reason>` | a join request is already pending |
| -3 | `Failed to send message: <msg> (<ExceptionClass>)` | I/O error |
| -32603 | `<msg> (<ExceptionClass>)` | missing `uri`, unexpected failures |
