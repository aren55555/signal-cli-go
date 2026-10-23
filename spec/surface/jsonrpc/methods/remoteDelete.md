# Method `remoteDelete`

## Sources

- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/commands/RemoteDeleteCommand.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/util/SendMessageResultUtils.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/util/CommandUtil.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/lib/src/main/java/org/asamk/signal/manager/Manager.java
- Binary observation: none. Conventions: [index.md](index.md); result: [send.md](send.md).

## Summary

Deletes a message previously sent by this account, for everyone ("delete for
everyone"). CLI: `signal-cli -a ACCOUNT remoteDelete -t TIMESTAMP [RECIPIENT...] [-g GROUP...] [-u USERNAME...] [--note-to-self]`.

Account requirement: **single**.

## Parameters

| JSON key(s) | Type | Required | Default | CLI flag | Notes |
|---|---|---|---|---|---|
| `target-timestamp` / `targetTimestamp` | long | yes (not checked) | none | `-t`, `--target-timestamp` | Timestamp of own message to delete. Missing -> -32603 (checked after recipients). |
| `recipient` / `recipients` | list of string | no* | none | positional | |
| `group-id` / `groupId` / `group-ids` / `groupIds` | list of string | no* | none | `-g`, `--group-id`, `--group` | |
| `username` / `usernames` | list of string | no* | none | `-u`, `--username` | |
| `note-to-self` / `noteToSelf` | bool | no* | false | `--note-to-self` | |

\* At least one recipient source required. No `notify-self` parameter.

## Result

Send result object as in [send.md](send.md), same post-send failure rules.

## Errors

| Code | Message | Trigger |
|---|---|---|
| -1 | `No recipients given` | No recipients. |
| -1 | `Invalid phone number '<input>': <reason>` / `Invalid group id: …` | Parse errors. |
| -1 | group error texts (as in send.md, unprefixed) | Group not found / not a member / not allowed. |
| -1 / -4 / -5 | post-send failure | No successful recipient. |
| -32603 | `Failed to send message: <reason> (<ExceptionName>) (UnexpectedErrorException)` | I/O error. |
| -32603 | `<text> (NullPointerException)` / `(ClassCastException)` | Missing or wrongly typed timestamp, other type mismatch. |
