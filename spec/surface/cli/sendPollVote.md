# signal-cli sendPollVote

## Sources

- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/commands/SendPollVoteCommand.java (tag v0.14.7)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/util/CommandUtil.java
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/util/SendMessageResultUtils.java
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/lib/src/main/java/org/asamk/signal/manager/api/RecipientIdentifier.java
- man page https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/man/signal-cli.1.adoc section `=== sendPollVote`
- Binary commands run (`--config <CONFIG>` empty scratch dir):
  - `signal-cli --config <CONFIG> sendPollVote --help`
  - `signal-cli --config <CONFIG> sendPollVote`
  - `signal-cli --config <CONFIG> sendPollVote --poll-timestamp 1`
  - `signal-cli --config <CONFIG> sendPollVote --vote-count 1`
  - `signal-cli --config <CONFIG> sendPollVote --poll-timestamp 1 --vote-count x`
  - `signal-cli --config <CONFIG> sendPollVote --poll-timestamp 1 --vote-count 1 -o a`
- Shared result format: [_send-results.md](_send-results.md).

## Summary

Help line: `Vote on a poll and send it to another user or group.`

Sends a vote for one or more option indexes of an existing poll, identified by
the poll author and poll timestamp.

## Invocation category & account requirement

Local single-account command; requires a registered account (see index.md).
Contacts the Signal server.

## Output modes

`plain-text` (default) and `json`.

## Arguments

| Names | Arity | Value | Default | Required | Help text (verbatim) | Notes |
|---|---|---|---|---|---|---|
| `recipient` (positional) | 0..n | string | none | no | Specify the recipients' phone number. | |
| `-g`, `--group-id`, `--group` | 0..n | GROUP_ID | none | no | Specify the recipient group ID. | |
| `-u`, `--username` | 0..n | USERNAME | none | no | Specify the recipient username or username link. | |
| `--note-to-self` | 0 | flag | false | no | Send the message to self | |
| `--notify-self` | 0 | flag | false | no | If self is part of recipients/groups send a normal message, not a sync message. | |
| `--poll-author` | 1 | POLL_AUTHOR | none | no (parser) but effectively required | Specify the number of the author of the poll message. | recipient syntax |
| `--poll-timestamp` | 1 | int64 | none | yes | Specify the timestamp of the original poll message. | |
| `-o`, `--option` | 0..n | int32 | none | no | The option indexes of the poll to vote for | each value must parse as a 32-bit integer |
| `--vote-count` | 1 | int32 | none | yes | Specify the number of this vote (increase by one for every time you vote). | |
| `-h`, `--help` | 0 | | | | show this help message and exit | |

## Constraints & validation

After account loading:
1. Recipients (see [_send-results.md](_send-results.md)); none -> `No recipients given` (exit 1).
2. `--poll-author` parsed as recipient; invalid -> `Invalid phone number '<s>': <reason>` (exit 1).
   If `--poll-author` is omitted, the reference fails with an internal
   null-pointer error (Java stack trace on stderr, exit 2) -- derived from
   source, unverified.
3. No validation of option indexes or vote count is done locally (no range
   checks, no single-choice enforcement). Behavior when `-o` is entirely absent
   is not determined from the CLI layer (the absent list is passed on as "no
   value").

## Behavior

Sends the vote (author, poll timestamp, option indexes, vote count) to the recipients.

## Plain-text output

See [_send-results.md](_send-results.md).

## JSON output

See [_send-results.md](_send-results.md).

## Errors

| Condition | Stream | Exact message | Exit |
|---|---|---|---|
| missing `--vote-count` (parse) | stderr | usage + `signal-cli: error: argument --vote-count is required` (observed) | 1 |
| missing `--poll-timestamp` (parse) | stderr | usage + `signal-cli: error: argument --poll-timestamp is required` (observed) | 1 |
| non-integer `--vote-count` (parse) | stderr | usage + `signal-cli: error: argument --vote-count: could  not convert 'x' to integer (32 bits)` (observed; the suffix wraps onto the next line at width 75) | 1 |
| non-integer `-o` value (parse) | stderr | usage + `signal-cli: error: argument -o/--option: could  not  convert 'a' to integer (32 bits)` (observed; the suffix wraps onto the next line at width 75) | 1 |
| no account / not registered | stderr | see index.md | 1 |
| recipient errors | stderr | see _send-results.md | 1 |
| group errors | stderr | `Group not found: <id>` etc. | 1 |
| unregistered recipient | stderr | `The user <identifier> is not registered.` | 1 |
| I/O error | stderr | `Failed to send message: <msg> (<ExceptionName>)` (+ prekeys suffix) | 2 |
| all sends failed | stderr | per [_send-results.md](_send-results.md) | 4/5/1 |

## --help transcript

Command: `signal-cli --config <CONFIG> sendPollVote --help` -- exit code: 0, stderr empty. Stdout:

```text
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli sendPollVote [-h] [-g [GROUP_ID [GROUP_ID ...]]]
                  [-u [USERNAME [USERNAME ...]]] [--note-to-self]
                  [--notify-self] [--poll-author POLL_AUTHOR]
                  --poll-timestamp POLL_TIMESTAMP
                  [-o [OPTION [OPTION ...]]] --vote-count VOTE_COUNT
                  [recipient [recipient ...]]

positional arguments:
  recipient              Specify the recipients' phone number.

named arguments:
  -h, --help             show this help message and exit
  -g [GROUP_ID [GROUP_ID ...]], --group-id [GROUP_ID [GROUP_ID ...]], --group [GROUP_ID [GROUP_ID ...]]
                         Specify the recipient group ID.
  -u [USERNAME [USERNAME ...]], --username [USERNAME [USERNAME ...]]
                         Specify the recipient username or username link.
  --note-to-self         Send the message to self
  --notify-self          If  self  is  part  of  recipients/groups  send  a
                         normal message, not a sync message.
  --poll-author POLL_AUTHOR
                         Specify the  number  of  the  author  of  the poll
                         message.
  --poll-timestamp POLL_TIMESTAMP
                         Specify  the  timestamp   of   the  original  poll
                         message.
  -o [OPTION [OPTION ...]], --option [OPTION [OPTION ...]]
                         The option indexes of the poll to vote for
  --vote-count VOTE_COUNT
                         Specify the number of  this  vote (increase by one
                         for every time you vote).
```

## Doc vs binary discrepancies

- Man page says "For none multi polls, only one option may be specified"; this is
  not enforced locally by the CLI.
- Man page describes `--poll-author` as "number or uuid"; the help says "number".
  Any recipient syntax is accepted.
