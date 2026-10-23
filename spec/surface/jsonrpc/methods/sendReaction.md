# Method `sendReaction`

## Sources

- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/commands/SendReactionCommand.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/util/SendMessageResultUtils.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/util/CommandUtil.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/json/JsonSendMessageResult.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/lib/src/main/java/org/asamk/signal/manager/Manager.java (result kind)
- Binary observation: none (needs a registered account). Conventions: [index.md](index.md). Result object and post-send failure rules: [send.md](send.md).

## Summary

Sends (or removes) an emoji reaction to a previously sent or received message,
or to a story. CLI: `signal-cli -a ACCOUNT sendReaction [RECIPIENT...] -e EMOJI -a AUTHOR -t TIMESTAMP [-r] [--story]`.

Account requirement: **single**.

## Parameters

| JSON key(s) | Type | Required | Default | CLI flag | Notes |
|---|---|---|---|---|---|
| `recipient` / `recipients` | list of string | no* | none | positional | index.md rule 10. |
| `group-id` / `groupId` / `group-ids` / `groupIds` | list of string | no* | none | `-g`, `--group-id`, `--group` | index.md rule 11. |
| `username` / `usernames` | list of string | no* | none | `-u`, `--username` | |
| `note-to-self` / `noteToSelf` | bool | no* | false | `--note-to-self` | |
| `notify-self` / `notifySelf` | bool | no | false | `--notify-self` | |
| `emoji` | string | yes (not checked) | none | `-e`, `--emoji` | Should be one grapheme cluster. Not validated by signal-cli; absence leads to an internal error at send time (-32603). |
| `target-author` / `targetAuthor` | string | conditionally | see notes | `-a`, `--target-author` | Author of the target message, index.md rule 10. If absent AND exactly one recipient was resolved AND it is an individual recipient, that recipient is used as the author. |
| `target-timestamp` / `targetTimestamp` | long | yes (not checked) | none | `-t`, `--target-timestamp` | Timestamp of the target message. Absent -> -32603 internal error. |
| `remove` | bool | no | false | `-r`, `--remove` | Remove instead of add. |
| `story` | bool | no | false | `--story` | Target is a story. |

\* At least one recipient source must be non-empty ("No recipients given").

Note on author defaulting: the defaulting applies when the single resolved
recipient is any individual identifier (UUID, PNI, phone number, or a
`username`/`u:` entry, which are all individual recipients); a single
group or note-to-self does not qualify (note-to-self is not an individual
recipient here, so `target-author` is then required).

The CLI marks emoji, target-author, target-timestamp as required; the JSON-RPC
path does not enforce this (CLI defaults/required checks do not apply over RPC).

## Result

The send result object (`timestamp`, `results`) as in [send.md](send.md),
with the same post-send failure rules (-1/-4/-5 with `data.response`).

## Errors

| Code | Message | Trigger |
|---|---|---|
| -1 | `No recipients given` | No recipients. |
| -1 | `Invalid phone number '<input>': <reason>` | Bad recipient or target author. |
| -1 | `Invalid group id: Failed to decode groupId (must be base64) "<input>": <reason>` | Bad group id. |
| -1 | `Group not found: …` / `User is not a member in group: …` / `User is not allowed to send message to group: …` | Group problems (texts as in send.md). |
| -1 | `The user <identifier> is not registered.` | Unregistered recipient. |
| -1 / -4 / -5 | post-send failure (see send.md) | No successful recipient. |
| -32603 | `Failed to send message: <reason> (<ExceptionName>) (UnexpectedErrorException)` | I/O error. |
| -32603 | Java exception text + ` (NullPointerException)` / ` (ClassCastException)` | Missing target-timestamp / target-author (when not defaultable) / emoji, or type mismatch (index.md rules 5, 6). |
