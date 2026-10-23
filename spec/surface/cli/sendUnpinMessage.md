# signal-cli sendUnpinMessage

## Sources

- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/commands/SendUnpinMessageCommand.java (tag v0.14.7)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/util/CommandUtil.java
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/util/SendMessageResultUtils.java
- man page https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/man/signal-cli.1.adoc section `=== sendUnpinMessage`
- Binary commands run (`--config <CONFIG>` empty scratch dir):
  - `signal-cli --config <CONFIG> sendUnpinMessage --help`
  - `signal-cli --config <CONFIG> sendUnpinMessage`
  - `signal-cli --config <CONFIG> sendUnpinMessage -t 1`
  - `signal-cli --config <CONFIG> sendUnpinMessage -a +1`
- Shared result format: [_send-results.md](_send-results.md).

## Summary

Help line: `Send unpin message for a previously received or sent message.`

Unpins a previously pinned message (or story) identified by author + timestamp.

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
| `--note-to-self` | 0 | flag | false | no | Send the unpin message to self. | |
| `--notify-self` | 0 | flag | false | no | If self is part of recipients/groups send a normal message, not a sync message. | |
| `-a`, `--target-author` | 1 | TARGET_AUTHOR | none | yes | Specify the number of the author of the message to unpin. | recipient syntax |
| `-t`, `--target-timestamp` | 1 | int64 | none | yes | Specify the timestamp of the message to unpin. | |
| `--story` | 0 | flag | false | no | Unpin a story instead of a normal message | |
| `-h`, `--help` | 0 | | | | show this help message and exit | |

## Constraints & validation

Same as [sendPinMessage](sendPinMessage.md): recipients (none -> `No recipients given`),
target author parsed as recipient. Exit 1 for these.

## Behavior

Sends an unpin message (target author, target timestamp, story flag).

## Plain-text output

See [_send-results.md](_send-results.md).

## JSON output

See [_send-results.md](_send-results.md).

## Errors

| Condition | Stream | Exact message | Exit |
|---|---|---|---|
| missing `-a` (parse) | stderr | usage + `signal-cli: error: argument -a/--target-author is required` (observed) | 1 |
| missing `-t` (parse) | stderr | usage + `signal-cli: error: argument -t/--target-timestamp is required` (observed) | 1 |
| no account / not registered | stderr | see index.md | 1 |
| recipient errors | stderr | see _send-results.md | 1 |
| group errors | stderr | `Group not found: <id>` etc. | 1 |
| unregistered recipient | stderr | `The user <identifier> is not registered.` | 1 |
| I/O error | stderr | `Failed to send message: <msg> (<ExceptionName>)` | 2 |
| all sends failed | stderr | per [_send-results.md](_send-results.md) | 4/5/1 |

## --help transcript

Command: `signal-cli --config <CONFIG> sendUnpinMessage --help` -- exit code: 0, stderr empty. Stdout:

```text
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli sendUnpinMessage [-h] [-g [GROUP_ID [GROUP_ID ...]]]
                  [-u [USERNAME [USERNAME ...]]] [--note-to-self]
                  [--notify-self] -a TARGET_AUTHOR -t TARGET_TIMESTAMP
                  [--story] [recipient [recipient ...]]

positional arguments:
  recipient              Specify the recipients' phone number.

named arguments:
  -h, --help             show this help message and exit
  -g [GROUP_ID [GROUP_ID ...]], --group-id [GROUP_ID [GROUP_ID ...]], --group [GROUP_ID [GROUP_ID ...]]
                         Specify the recipient group ID.
  -u [USERNAME [USERNAME ...]], --username [USERNAME [USERNAME ...]]
                         Specify the recipient username or username link.
  --note-to-self         Send the unpin message to self.
  --notify-self          If  self  is  part  of  recipients/groups  send  a
                         normal message, not a sync message.
  -a TARGET_AUTHOR, --target-author TARGET_AUTHOR
                         Specify the number of  the  author  of the message
                         to unpin.
  -t TARGET_TIMESTAMP, --target-timestamp TARGET_TIMESTAMP
                         Specify the timestamp of the message to unpin.
  --story                Unpin a story instead of a normal message
```

## Doc vs binary discrepancies

- Man page does not mention `--story`; binary has it.
- Man page does not mark `-a`/`-t` as required; the binary requires both.
