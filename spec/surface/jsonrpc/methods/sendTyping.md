# Method `sendTyping`

## Sources

- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/commands/SendTypingCommand.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/util/SendMessageResultUtils.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/util/CommandUtil.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/lib/src/main/java/org/asamk/signal/manager/Manager.java
- Binary observation: none. Conventions: [index.md](index.md); result: [send.md](send.md).

## Summary

Sends a typing indicator START (default) or STOP to recipients and/or groups.
Recipients show the indicator for ~15 s unless STOP arrives. CLI:
`signal-cli -a ACCOUNT sendTyping [RECIPIENT...] [-g GROUP...] [-s]`.

Account requirement: **single**.

## Parameters

| JSON key(s) | Type | Required | Default | CLI flag | Notes |
|---|---|---|---|---|---|
| `recipient` / `recipients` | list of string | no* | none | positional | index.md rule 10. |
| `group-id` / `groupId` / `group-ids` / `groupIds` | list of string | no* | none | `-g`, `--group-id`, `--group` | index.md rule 11. |
| `stop` | bool | no | false | `-s`, `--stop` | Send STOP instead of START. |

\* At least one recipient or group required. No `username`, `note-to-self` or `notify-self` support.

## Result

Send result object as in [send.md](send.md), same post-send failure rules.

## Errors

| Code | Message | Trigger |
|---|---|---|
| -1 | `No recipients given` | No recipients and no groups. |
| -1 | `Invalid phone number '<input>': <reason>` / `Invalid group id: …` | Parse errors. |
| -1 | `Failed to send message: <reason> (<ExceptionName>)` | I/O error while sending. Note: here an I/O error is a **user error (-1)**, not -32603, and has no `(UnexpectedErrorException)` suffix. |
| -1 | `Failed to send to group: <group error text>` | Group not found / not a member / sending not allowed (texts as in send.md, prefixed). |
| -1 / -4 / -5 | post-send failure | No successful recipient. |
| -32603 | `<text> (ClassCastException)` | Type mismatch (e.g. `stop` not a boolean). |
