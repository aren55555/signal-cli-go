# Method `sendPollVote`

## Sources

- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/commands/SendPollVoteCommand.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/util/SendMessageResultUtils.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/util/CommandUtil.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/lib/src/main/java/org/asamk/signal/manager/Manager.java
- Binary observation: none. Conventions: [index.md](index.md); result: [send.md](send.md).

## Summary

Votes on a poll. CLI: `signal-cli -a ACCOUNT sendPollVote [RECIPIENT...] [-g GROUP...] --poll-author AUTHOR --poll-timestamp TS [-o INDEX...] --vote-count N`.

Account requirement: **single**.

## Parameters

| JSON key(s) | Type | Required | Default | CLI flag | Notes |
|---|---|---|---|---|---|
| `recipient` / `recipients` | list of string | no* | none | positional | index.md rule 10. |
| `group-id` / `groupId` / `group-ids` / `groupIds` | list of string | no* | none | `-g`, `--group-id`, `--group` | index.md rule 11. |
| `username` / `usernames` | list of string | no* | none | `-u`, `--username` | |
| `note-to-self` / `noteToSelf` | bool | no* | false | `--note-to-self` | |
| `notify-self` / `notifySelf` | bool | no | false | `--notify-self` | |
| `poll-author` / `pollAuthor` | string | yes (effectively) | none | `--poll-author` | Author of the poll message, index.md rule 10. No defaulting to the recipient: missing -> internal error -32603. |
| `poll-timestamp` / `pollTimestamp` | long | yes (not checked) | none | `--poll-timestamp` | Timestamp of the poll message. Missing -> -32603. |
| `option` / `options` | list of int | no | none | `-o`, `--option` (zero or more) | Zero-based option indexes voted for. Elements must be JSON integers within 32-bit range. Omitting it hands an absent list to the send layer; the outcome was not determined from the command source (expected to be an internal error, -32603). Clients should always send an array. |
| `vote-count` / `voteCount` | int (32-bit) | yes (not checked) | none | `--vote-count` | Sequence number of this vote, increase by one for every re-vote. Missing -> -32603 (NullPointerException). |

\* At least one recipient source required.

## Result

Send result object as in [send.md](send.md), same post-send failure rules.

## Errors

| Code | Message | Trigger |
|---|---|---|
| -1 | `No recipients given` | No recipients. |
| -1 | `Invalid phone number '<input>': <reason>` / `Invalid group id: …` | Parse errors. |
| -1 | group error texts (send.md) | Group not found / not a member / not allowed. |
| -1 | `The user <identifier> is not registered.` | Unregistered recipient. |
| -1 / -4 / -5 | post-send failure | No successful recipient. |
| -32603 | `Failed to send message: <reason> (<ExceptionName>) (UnexpectedErrorException)` | I/O error; if the I/O text contains `No prekeys available` the form is `Failed to send message: <reason> (<ExceptionName>), maybe one of the devices of the recipient wasn't online for a while. (UnexpectedErrorException)`. |
| -32603 | `<text> (NullPointerException)` / `(ClassCastException)` | Missing poll-author / poll-timestamp / vote-count, wrong numeric widths. |
