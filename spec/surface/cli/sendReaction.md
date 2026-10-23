# signal-cli sendReaction

## Sources

- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/commands/SendReactionCommand.java (tag v0.14.7)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/util/CommandUtil.java
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/util/SendMessageResultUtils.java
- man page https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/man/signal-cli.1.adoc section `=== sendReaction`
- Binary commands run (`--config <CONFIG>` empty scratch dir):
  - `signal-cli --config <CONFIG> sendReaction --help`
  - `signal-cli --config <CONFIG> sendReaction`
  - `signal-cli --config <CONFIG> sendReaction -a +1 -t 1`
  - `signal-cli --config <CONFIG> sendReaction -e x -a +1`
- Shared result format: [_send-results.md](_send-results.md).

## Summary

Help line: `Send reaction to a previously received or sent message.`

Adds (or with `-r` removes) an emoji reaction to a message or story identified
by author + timestamp.

## Invocation category & account requirement

Local single-account command; requires a registered account (see index.md).
Contacts the Signal server.

## Output modes

`plain-text` (default) and `json`.

## Arguments

| Names | Arity | Value | Default | Required | Help text (verbatim) | Notes |
|---|---|---|---|---|---|---|
| `-g`, `--group-id`, `--group` | 0..n | GROUP_ID | none | no | Specify the recipient group ID. | |
| `recipient` (positional) | 0..n | string | none | no | Specify the recipients' phone number. | |
| `-u`, `--username` | 0..n | USERNAME | none | no | Specify the recipient username or username link. | |
| `--note-to-self` | 0 | flag | false | no | Send the reaction to self. | |
| `--notify-self` | 0 | flag | false | no | If self is part of recipients/groups send a normal message, not a sync message. | |
| `-e`, `--emoji` | 1 | EMOJI | none | yes | Specify the emoji, should be a single unicode grapheme cluster. | not validated locally |
| `-a`, `--target-author` | 1 | TARGET_AUTHOR | none | yes | Specify the number of the author of the message to which to react. | recipient syntax |
| `-t`, `--target-timestamp` | 1 | int64 | none | yes | Specify the timestamp of the message to which to react. | |
| `-r`, `--remove` | 0 | flag | false | no | Remove a reaction. | |
| `--story` | 0 | flag | false | no | React to a story instead of a normal message | |
| `-h`, `--help` | 0 | | | | show this help message and exit | |

## Constraints & validation

After account loading: recipients resolved (none -> `No recipients given`),
target author parsed as recipient (`Invalid phone number '<s>': <reason>`). Exit 1.
The emoji is passed through unvalidated.

## Behavior

Sends a reaction (emoji, remove flag, target author, target timestamp, story flag).

## Plain-text output

See [_send-results.md](_send-results.md).

## JSON output

See [_send-results.md](_send-results.md).

## Errors

| Condition | Stream | Exact message | Exit |
|---|---|---|---|
| missing `-e` (parse) | stderr | usage + `signal-cli: error: argument -e/--emoji is required` (observed) | 1 |
| missing `-a` (parse) | stderr | usage + `signal-cli: error: argument -a/--target-author is required` | 1 |
| missing `-t` (parse) | stderr | usage + `signal-cli: error: argument -t/--target-timestamp is required` (observed) | 1 |
| no account / not registered | stderr | see index.md | 1 |
| recipient errors | stderr | see _send-results.md | 1 |
| group errors | stderr | `Group not found: <id>` etc. | 1 |
| unregistered recipient | stderr | `The user <identifier> is not registered.` | 1 |
| I/O error | stderr | `Failed to send message: <msg> (<ExceptionName>)` | 2 |
| all sends failed | stderr | per [_send-results.md](_send-results.md) | 4/5/1 |

Parse-time required-argument checks are reported one at a time. With all three
missing, the binary reports `argument -t/--target-timestamp is required`
(observed: `signal-cli --config <CONFIG> sendReaction`), i.e. the last-declared
missing required option is reported.

## --help transcript

Command: `signal-cli --config <CONFIG> sendReaction --help` -- exit code: 0, stderr empty. Stdout:

```text
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli sendReaction [-h] [-g [GROUP_ID [GROUP_ID ...]]]
                  [-u [USERNAME [USERNAME ...]]] [--note-to-self]
                  [--notify-self] -e EMOJI -a TARGET_AUTHOR
                  -t TARGET_TIMESTAMP [-r] [--story]
                  [recipient [recipient ...]]

positional arguments:
  recipient              Specify the recipients' phone number.

named arguments:
  -h, --help             show this help message and exit
  -g [GROUP_ID [GROUP_ID ...]], --group-id [GROUP_ID [GROUP_ID ...]], --group [GROUP_ID [GROUP_ID ...]]
                         Specify the recipient group ID.
  -u [USERNAME [USERNAME ...]], --username [USERNAME [USERNAME ...]]
                         Specify the recipient username or username link.
  --note-to-self         Send the reaction to self.
  --notify-self          If  self  is  part  of  recipients/groups  send  a
                         normal message, not a sync message.
  -e EMOJI, --emoji EMOJI
                         Specify the  emoji,  should  be  a  single unicode
                         grapheme cluster.
  -a TARGET_AUTHOR, --target-author TARGET_AUTHOR
                         Specify the number of  the  author  of the message
                         to which to react.
  -t TARGET_TIMESTAMP, --target-timestamp TARGET_TIMESTAMP
                         Specify the timestamp of  the  message to which to
                         react.
  -r, --remove           Remove a reaction.
  --story                React to a story instead of a normal message
```

## Doc vs binary discrepancies

- Man page does not mark `-e`, `-a`, `-t` as required; the binary requires them.
