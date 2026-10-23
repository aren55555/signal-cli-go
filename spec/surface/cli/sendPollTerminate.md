# signal-cli sendPollTerminate

## Sources

- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/commands/SendPollTerminateCommand.java (tag v0.14.7)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/util/CommandUtil.java
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/util/SendMessageResultUtils.java
- man page https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/man/signal-cli.1.adoc section `=== sendPollTerminate`
- Binary commands run (`--config <CONFIG>` empty scratch dir):
  - `signal-cli --config <CONFIG> sendPollTerminate --help`
  - `signal-cli --config <CONFIG> sendPollTerminate`
  - `signal-cli --config <CONFIG> sendPollTerminate --poll-timestamp x`
- Shared result format: [_send-results.md](_send-results.md).

## Summary

Help line: `Terminate a poll and send it to another user or group.`

Closes a poll previously created by this account (identified by its timestamp).

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
| `--poll-timestamp` | 1 | int64 | none | yes | Specify the timestamp of the original poll message. | |
| `-h`, `--help` | 0 | | | | show this help message and exit | |

## Constraints & validation

After account loading: recipients resolved; none -> `No recipients given` (exit 1).

## Behavior

Sends a poll-terminate message referencing the poll timestamp.

## Plain-text output

See [_send-results.md](_send-results.md).

## JSON output

See [_send-results.md](_send-results.md).

## Errors

| Condition | Stream | Exact message | Exit |
|---|---|---|---|
| missing `--poll-timestamp` (parse) | stderr | usage + `signal-cli: error: argument --poll-timestamp is required` (observed) | 1 |
| non-integer `--poll-timestamp` (parse) | stderr | usage + `signal-cli: error: argument  --poll-timestamp:  could  not  convert  'x' to` / `integer (64 bits)` (observed) | 1 |
| no account / not registered | stderr | see index.md | 1 |
| recipient errors | stderr | see _send-results.md | 1 |
| group errors | stderr | `Group not found: <id>` etc. | 1 |
| unregistered recipient | stderr | `The user <identifier> is not registered.` | 1 |
| I/O error | stderr | `Failed to send message: <msg> (<ExceptionName>)` (+ prekeys suffix) | 2 |
| all sends failed | stderr | per [_send-results.md](_send-results.md) | 4/5/1 |

## --help transcript

Command: `signal-cli --config <CONFIG> sendPollTerminate --help` -- exit code: 0, stderr empty. Stdout:

```text
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli sendPollTerminate [-h] [-g [GROUP_ID [GROUP_ID ...]]]
                  [-u [USERNAME [USERNAME ...]]] [--note-to-self]
                  [--notify-self] --poll-timestamp POLL_TIMESTAMP
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
  --poll-timestamp POLL_TIMESTAMP
                         Specify  the  timestamp   of   the  original  poll
                         message.
```

## Doc vs binary discrepancies

None found.
