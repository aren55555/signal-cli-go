# Method `sendMessageRequestResponse`

## Sources

- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/commands/SendMessageRequestResponseCommand.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/commands/MessageRequestResponseType.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/util/CommandUtil.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/lib/src/main/java/org/asamk/signal/manager/Manager.java
- Binary observation: none. Conventions: [index.md](index.md).

## Summary

Accepts or deletes a message request (from a contact or group) and syncs the
decision to linked devices. CLI: `signal-cli -a ACCOUNT sendMessageRequestResponse [RECIPIENT...] [-g GROUP...] [-u USERNAME...] --type accept|delete`.

Account requirement: **single**.

## Parameters

| JSON key(s) | Type | Required | Default | CLI flag | Notes |
|---|---|---|---|---|---|
| `recipient` / `recipients` | list of string | no* | none | positional | index.md rule 10. |
| `group-id` / `groupId` / `group-ids` / `groupIds` | list of string | no* | none | `-g`, `--group-id`, `--group` | index.md rule 11. |
| `username` / `usernames` | list of string | no* | none | `-u`, `--username` | |
| `type` | string | yes | none | `--type` (`accept` / `delete`) | Case-insensitive over JSON-RPC: the value is upper-cased and must then equal `ACCEPT` or `DELETE`. Any other value -> -32603 `No enum constant org.asamk.signal.commands.MessageRequestResponseType.<UPPERCASED> (IllegalArgumentException)`. Missing -> -32603 (NullPointerException). Non-string JSON values -> -32603 (ClassCastException). The type is evaluated **before** recipients. |

\* At least one recipient source required. There is no note-to-self option.

## Result

Always the empty object `{}` on success (per-recipient results are not reported).

## Errors

| Code | Message | Trigger |
|---|---|---|
| -1 | `No recipients given` | No recipients. |
| -1 | `Invalid phone number '<input>': <reason>` / `Invalid group id: …` | Parse errors. |
| -32603 | `No enum constant org.asamk.signal.commands.MessageRequestResponseType.<VALUE> (IllegalArgumentException)` | Unknown type. The fully qualified Java type name is part of the upstream message; a reimplementation that wants byte-exact errors must reproduce it. |
| -32603 | `<text> (NullPointerException)` / `(ClassCastException)` | Missing/non-string type. |
