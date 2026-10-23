# Method `sendPinMessage`

## Sources

- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/commands/SendPinMessageCommand.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/util/SendMessageResultUtils.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/util/CommandUtil.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/lib/src/main/java/org/asamk/signal/manager/Manager.java
- Binary observation: none. Conventions: [index.md](index.md); result: [send.md](send.md).

## Summary

Pins a previously sent or received message (or story) in a conversation, for a
duration or forever. CLI: `signal-cli -a ACCOUNT sendPinMessage [RECIPIENT...] [-g GROUP...] [-u USERNAME...] [-d SECONDS] -a AUTHOR -t TIMESTAMP [--story]`.

Account requirement: **single**.

## Parameters

| JSON key(s) | Type | Required | Default | CLI flag | Notes |
|---|---|---|---|---|---|
| `recipient` / `recipients` | list of string | no* | none | positional | index.md rule 10. |
| `group-id` / `groupId` / `group-ids` / `groupIds` | list of string | no* | none | `-g`, `--group-id`, `--group` | |
| `username` / `usernames` | list of string | no* | none | `-u`, `--username` | |
| `note-to-self` / `noteToSelf` | bool | no* | false | `--note-to-self` | |
| `notify-self` / `notifySelf` | bool | no | false | `--notify-self` | |
| `pin-duration` / `pinDuration` | int (32-bit) | **effectively yes over JSON-RPC** | CLI default -1; **no default over RPC** | `-d`, `--pin-duration` | Seconds; -1 = forever. The CLI default of -1 is not applied on the JSON-RPC path: omitting it causes an internal error (-32603, NullPointerException). Values above 2^31-1 fail with ClassCastException (-32603). Clients should always send it (e.g. `-1`). |
| `target-author` / `targetAuthor` | string | conditionally | single recipient | `-a`, `--target-author` | Author of the target message. If absent and exactly one individual recipient was resolved, that recipient is used; otherwise required (absent -> -32603). |
| `target-timestamp` / `targetTimestamp` | long | yes (not checked) | none | `-t`, `--target-timestamp` | Missing -> -32603. |
| `story` | bool | no | false | `--story` | Target is a story. |

\* At least one recipient source required.

## Result

Send result object as in [send.md](send.md), same post-send failure rules.

## Errors

| Code | Message | Trigger |
|---|---|---|
| -1 | `No recipients given` | No recipients. |
| -1 | `Invalid phone number '<input>': <reason>` / `Invalid group id: …` | Parse errors (recipients, target author). |
| -1 | group error texts (send.md) | Group not found / not a member / not allowed. |
| -1 | `The user <identifier> is not registered.` | Unregistered recipient. |
| -1 / -4 / -5 | post-send failure | No successful recipient. |
| -32603 | `Failed to send message: <reason> (<ExceptionName>) (UnexpectedErrorException)` | I/O error. |
| -32603 | `<text> (NullPointerException)` / `(ClassCastException)` | Missing pin-duration, target-timestamp or non-defaultable target-author; type mismatch. |
