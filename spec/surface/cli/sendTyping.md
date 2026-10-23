# signal-cli sendTyping

## Sources

- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/commands/SendTypingCommand.java (tag v0.14.7)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/util/CommandUtil.java
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/util/SendMessageResultUtils.java
- man page https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/man/signal-cli.1.adoc section `=== sendTyping`
- Binary commands run (`--config <CONFIG>` empty scratch dir):
  - `signal-cli --config <CONFIG> sendTyping --help`
  - `signal-cli --config <CONFIG> sendTyping` (no account -> account error)
  - `signal-cli --config <CONFIG> sendTyping -u x`
  - `signal-cli --config <CONFIG> sendTyping -s extra --bogus`
- Shared result format: [_send-results.md](_send-results.md).

## Summary

Help line: `Send typing message to trigger a typing indicator for the recipient. Indicator will be shown for 15seconds unless a typing STOP message is sent first.`

Sends a typing-started (default) or typing-stopped (`-s`) indicator.

## Invocation category & account requirement

Local single-account command; requires a registered account (see index.md).
Contacts the Signal server.

## Output modes

`plain-text` (default) and `json`.

## Arguments

| Names | Arity | Value | Default | Required | Help text (verbatim) | Notes |
|---|---|---|---|---|---|---|
| `-g`, `--group-id`, `--group` | 0..n | GROUP_ID | none | no | Specify the recipient group ID. | |
| `recipient` (positional) | 0..n | string | none | no | Specify the recipients' phone number. | number, UUID, `PNI:`, `u:` |
| `-s`, `--stop` | 0 | flag | false | no | Send a typing STOP message. | |
| `-h`, `--help` | 0 | | | | show this help message and exit | |

No `-u/--username`, `--note-to-self` or `--notify-self` options.

## Constraints & validation

After account loading: individual recipients and groups resolved (invalid ->
messages as in [_send-results.md](_send-results.md)); none -> `No recipients given` (exit 1).

## Behavior

Sends a typing action START or STOP to all recipients.

## Plain-text output

See [_send-results.md](_send-results.md).

## JSON output

See [_send-results.md](_send-results.md).

## Errors

| Condition | Stream | Exact message | Exit |
|---|---|---|---|
| unknown option, e.g. `-u` (parse) | stderr | usage + `signal-cli: error: unrecognized arguments: '-u'` (observed; no suggestion shown) | 1 |
| unknown long option (parse) | stderr | usage + `signal-cli: error: unrecognized arguments: '--bogus'` + blank line + `Did you mean:` + tab + `--group` (observed) | 1 |
| no account / not registered | stderr | see index.md (`No local users found, you first need to register or link an account` observed) | 1 |
| no recipients / invalid recipient / group id | stderr | see _send-results.md | 1 |
| I/O error | stderr | `Failed to send message: <msg> (<ExceptionName>)` | **1** (not 2, unlike other send commands) |
| group not found / not member / not allowed | stderr | `Failed to send to group: <underlying message>` (prefixed, unlike other commands) | 1 |
| all sends failed | stderr | per [_send-results.md](_send-results.md) | 4/5/1 |

## --help transcript

Command: `signal-cli --config <CONFIG> sendTyping --help` -- exit code: 0, stderr empty. Stdout:

```text
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli sendTyping [-h] [-g [GROUP_ID [GROUP_ID ...]]] [-s]
                  [recipient [recipient ...]]

positional arguments:
  recipient              Specify the recipients' phone number.

named arguments:
  -h, --help             show this help message and exit
  -g [GROUP_ID [GROUP_ID ...]], --group-id [GROUP_ID [GROUP_ID ...]], --group [GROUP_ID [GROUP_ID ...]]
                         Specify the recipient group ID.
  -s, --stop             Send a typing STOP message.
```

## Doc vs binary discrepancies

None found.
