# signal-cli sendPinMessage

## Sources

- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/commands/SendPinMessageCommand.java (tag v0.14.7)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/util/CommandUtil.java
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/util/SendMessageResultUtils.java
- man page https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/man/signal-cli.1.adoc section `=== sendPinMessage`
- Binary commands run (`--config <CONFIG>` empty scratch dir):
  - `signal-cli --config <CONFIG> sendPinMessage --help`
  - `signal-cli --config <CONFIG> sendPinMessage`
  - `signal-cli --config <CONFIG> sendPinMessage -a +1`
  - `signal-cli --config <CONFIG> sendPinMessage -a +1 -t 1 -d x`
- Shared result format: [_send-results.md](_send-results.md).

## Summary

Help line: `Send pin message for a previously received or sent message.`

Pins a message (or story), identified by author + timestamp, in a 1:1 or group
conversation, for a duration or forever.

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
| `--note-to-self` | 0 | flag | false | no | Send the pin message to self. | |
| `--notify-self` | 0 | flag | false | no | If self is part of recipients/groups send a normal message, not a sync message. | |
| `-d`, `--pin-duration` | 1 | int32 | `-1` | no | Specify the pin duration in seconds. Use -1 for forever. | help does not print the default |
| `-a`, `--target-author` | 1 | TARGET_AUTHOR | none | yes | Specify the number of the author of the message to pin. | recipient syntax |
| `-t`, `--target-timestamp` | 1 | int64 | none | yes | Specify the timestamp of the message to pin. | |
| `--story` | 0 | flag | false | no | Pin a story instead of a normal message | |
| `-h`, `--help` | 0 | | | | show this help message and exit | |

## Constraints & validation

After account loading:
1. Recipients resolved; none -> `No recipients given` (exit 1).
2. Target author parsed as recipient; invalid -> `Invalid phone number '<s>': <reason>` (exit 1).
   (The implementation would fall back to the sole individual recipient when
   no target author is given, but on the CLI `-a` is mandatory so this path is
   unreachable; it matters only for JSON-RPC.)
3. No range check on duration.

## Behavior

Sends a pin message (duration, target author, target timestamp, story flag).

## Plain-text output

See [_send-results.md](_send-results.md).

## JSON output

See [_send-results.md](_send-results.md).

## Errors

| Condition | Stream | Exact message | Exit |
|---|---|---|---|
| missing `-a` (parse) | stderr | usage + `signal-cli: error: argument -a/--target-author is required` | 1 |
| missing `-t` (parse) | stderr | usage + `signal-cli: error: argument -t/--target-timestamp is required` (observed) | 1 |
| non-integer `-d` (parse) | stderr | usage + `signal-cli: error: argument  -d/--pin-duration:  could  not  convert 'x' to` / `integer (32 bits)` (observed) | 1 |
| no account / not registered | stderr | see index.md | 1 |
| recipient errors | stderr | see _send-results.md | 1 |
| group errors | stderr | `Group not found: <id>` etc. | 1 |
| unregistered recipient | stderr | `The user <identifier> is not registered.` | 1 |
| I/O error | stderr | `Failed to send message: <msg> (<ExceptionName>)` | 2 |
| all sends failed | stderr | per [_send-results.md](_send-results.md) | 4/5/1 |

## --help transcript

Command: `signal-cli --config <CONFIG> sendPinMessage --help` -- exit code: 0, stderr empty. Stdout:

```text
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli sendPinMessage [-h] [-g [GROUP_ID [GROUP_ID ...]]]
                  [-u [USERNAME [USERNAME ...]]] [--note-to-self]
                  [--notify-self] [-d PIN_DURATION] -a TARGET_AUTHOR
                  -t TARGET_TIMESTAMP [--story] [recipient [recipient ...]]

positional arguments:
  recipient              Specify the recipients' phone number.

named arguments:
  -h, --help             show this help message and exit
  -g [GROUP_ID [GROUP_ID ...]], --group-id [GROUP_ID [GROUP_ID ...]], --group [GROUP_ID [GROUP_ID ...]]
                         Specify the recipient group ID.
  -u [USERNAME [USERNAME ...]], --username [USERNAME [USERNAME ...]]
                         Specify the recipient username or username link.
  --note-to-self         Send the pin message to self.
  --notify-self          If  self  is  part  of  recipients/groups  send  a
                         normal message, not a sync message.
  -d PIN_DURATION, --pin-duration PIN_DURATION
                         Specify the pin duration  in  seconds.  Use -1 for
                         forever.
  -a TARGET_AUTHOR, --target-author TARGET_AUTHOR
                         Specify the number of  the  author  of the message
                         to pin.
  -t TARGET_TIMESTAMP, --target-timestamp TARGET_TIMESTAMP
                         Specify the timestamp of the message to pin.
  --story                Pin a story instead of a normal message
```

## Doc vs binary discrepancies

- Man page does not mention `--story` for sendPinMessage; the binary has it.
- Man page does not mark `-a`/`-t` as required; the binary requires both.
