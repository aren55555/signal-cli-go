# Method `sendPollCreate`

## Sources

- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/commands/SendPollCreateCommand.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/util/SendMessageResultUtils.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/util/CommandUtil.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/lib/src/main/java/org/asamk/signal/manager/Manager.java
- Binary observation: none. Conventions: [index.md](index.md); result: [send.md](send.md).

## Summary

Creates a poll and sends it to recipients/groups. CLI:
`signal-cli -a ACCOUNT sendPollCreate [RECIPIENT...] [-g GROUP...] -q QUESTION -o OPTION OPTION... [--no-multi]`.

Account requirement: **single**.

## Parameters

| JSON key(s) | Type | Required | Default | CLI flag | Notes |
|---|---|---|---|---|---|
| `recipient` / `recipients` | list of string | no* | none | positional | index.md rule 10. |
| `group-id` / `groupId` / `group-ids` / `groupIds` | list of string | no* | none | `-g`, `--group-id`, `--group` | index.md rule 11. |
| `username` / `usernames` | list of string | no* | none | `-u`, `--username` | |
| `note-to-self` / `noteToSelf` | bool | no* | false | `--note-to-self` | |
| `notify-self` / `notifySelf` | bool | no | false | `--notify-self` | |
| `question` | string | yes (not checked) | none | `-q`, `--question` | Poll question; not validated (missing -> internal error at send). |
| `no-multi` / `noMulti` | bool | no | false | `--no-multi` | When true only one option may be selected; default allows multiple. |
| `option` / `options` | list of string | yes | none | `-o`, `--option` (one or more) | 2 to 10 options; each non-empty and at most 100 characters (Java string length, i.e. UTF-16 code units). Missing entirely -> internal error -32603 (NullPointerException). Elements must be JSON strings (otherwise ClassCastException -32603). |

\* At least one recipient source required. Recipients are validated before options.

## Result

Send result object as in [send.md](send.md); its `timestamp` identifies the poll for later votes/termination. Same post-send failure rules.

## Errors

| Code | Message | Trigger |
|---|---|---|
| -1 | `Poll needs at least two options` | Fewer than 2 options. |
| -1 | `Poll cannot have more than 10 options` | More than 10. |
| -1 | `Poll options must not be empty` | An option is `""`. |
| -1 | `Poll option "<option>" exceeds the maximum length of 100 characters` | Option longer than 100. |
| -1 | `No recipients given` | No recipients. |
| -1 | `Invalid phone number '<input>': <reason>` / `Invalid group id: …` | Parse errors. |
| -1 | group error texts (send.md) | Group not found / not a member / not allowed. |
| -1 | `The user <identifier> is not registered.` | Unregistered recipient. |
| -1 / -4 / -5 | post-send failure | No successful recipient. |
| -32603 | `Failed to send message: <reason> (<ExceptionName>) (UnexpectedErrorException)` | I/O error; if the I/O text contains `No prekeys available` the form is `Failed to send message: <reason> (<ExceptionName>), maybe one of the devices of the recipient wasn't online for a while. (UnexpectedErrorException)`. |
| -32603 | `<text> (NullPointerException)` / `(ClassCastException)` | Missing options/question, type mismatch. |
