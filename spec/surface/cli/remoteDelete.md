# signal-cli remoteDelete

## Sources

- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/commands/RemoteDeleteCommand.java (tag v0.14.7)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/util/CommandUtil.java
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/util/SendMessageResultUtils.java
- man page https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/man/signal-cli.1.adoc section `=== remoteDelete`
- Binary commands run (`--config <CONFIG>` empty scratch dir):
  - `signal-cli --config <CONFIG> remoteDelete --help`
  - `signal-cli --config <CONFIG> remoteDelete`
  - `signal-cli --config <CONFIG> remoteDelete -t abc`
  - `signal-cli --config <CONFIG> -o json remoteDelete -t 1 +15555550199`
- Shared result format: [_send-results.md](_send-results.md).

## Summary

Help line: `Remotely delete a previously sent message.`

Sends a "delete for everyone" for one of this account's own previously sent
messages, identified by its timestamp.

## Invocation category & account requirement

Local single-account command; requires a registered account (see index.md).
Contacts the Signal server.

## Output modes

`plain-text` (default) and `json`.

## Arguments

| Names | Arity | Value | Default | Required | Help text (verbatim) | Notes |
|---|---|---|---|---|---|---|
| `-t`, `--target-timestamp` | 1 | int64 | none | yes | Specify the timestamp of the message to delete. | |
| `-g`, `--group-id`, `--group` | 0..n | GROUP_ID | none | no | Specify the recipient group ID. | |
| `recipient` (positional) | 0..n | string | none | no | Specify the recipients' phone number. | |
| `-u`, `--username` | 0..n | USERNAME | none | no | Specify the recipient username or username link. | |
| `--note-to-self` | 0 | flag | false | no | (no help text; the help output shows the bare flag) | |
| `-h`, `--help` | 0 | | | | show this help message and exit | |

No `--notify-self` option.

## Constraints & validation

After account loading: recipients resolved; none -> `No recipients given` (exit 1).

## Behavior

Sends a remote-delete for the target timestamp to the recipients.

## Plain-text output

See [_send-results.md](_send-results.md).

## JSON output

See [_send-results.md](_send-results.md).

## Errors

| Condition | Stream | Exact message | Exit |
|---|---|---|---|
| missing `-t` (parse) | stderr | usage + `signal-cli: error: argument -t/--target-timestamp is required` (observed) | 1 |
| non-integer `-t` (parse) | stderr | usage + `signal-cli: error: argument -t/--target-timestamp:  could not convert 'abc'` / `to integer (64 bits)` (observed) | 1 |
| no account | stderr | `No local users found, you first need to register or link an account` (observed, also with `-o json`) | 1 |
| not registered | stderr | `User <account> is not registered.` | 1 |
| recipient errors | stderr | see _send-results.md | 1 |
| group errors | stderr | `Group not found: <id>` etc. | 1 |
| I/O error | stderr | `Failed to send message: <msg> (<ExceptionName>)` | 2 |
| all sends failed | stderr | per [_send-results.md](_send-results.md) | 4/5/1 |

## --help transcript

Command: `signal-cli --config <CONFIG> remoteDelete --help` -- exit code: 0, stderr empty. Stdout:

```text
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli remoteDelete [-h] -t TARGET_TIMESTAMP
                  [-g [GROUP_ID [GROUP_ID ...]]]
                  [-u [USERNAME [USERNAME ...]]] [--note-to-self]
                  [recipient [recipient ...]]

positional arguments:
  recipient              Specify the recipients' phone number.

named arguments:
  -h, --help             show this help message and exit
  -t TARGET_TIMESTAMP, --target-timestamp TARGET_TIMESTAMP
                         Specify the timestamp of the message to delete.
  -g [GROUP_ID [GROUP_ID ...]], --group-id [GROUP_ID [GROUP_ID ...]], --group [GROUP_ID [GROUP_ID ...]]
                         Specify the recipient group ID.
  -u [USERNAME [USERNAME ...]], --username [USERNAME [USERNAME ...]]
                         Specify the recipient username or username link.
  --note-to-self
```

## Doc vs binary discrepancies

- Man page does not list `--note-to-self` for remoteDelete; the binary has it.
