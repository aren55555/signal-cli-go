# signal-cli sendReceipt

## Sources

- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/commands/SendReceiptCommand.java (tag v0.14.7)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/util/CommandUtil.java
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/util/SendMessageResultUtils.java
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/lib/src/main/java/org/asamk/signal/manager/Manager.java
- man page https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/man/signal-cli.1.adoc section `=== sendReceipt`
- Binary commands run (`--config <CONFIG>` empty scratch dir):
  - `signal-cli --config <CONFIG> sendReceipt --help`
  - `signal-cli --config <CONFIG> sendReceipt`
  - `signal-cli --config <CONFIG> sendReceipt +15555550199`
  - `signal-cli --config <CONFIG> sendReceipt -t 1`
  - `signal-cli --config <CONFIG> sendReceipt -t 1 --type foo +15555550199`
  - `signal-cli --config <CONFIG> sendReceipt -t 1 2 x +15555550199`
  - `signal-cli --config <CONFIG> sendReceipt -t 1 2 +15555550199`
  - `signal-cli --config <CONFIG> sendReceipt +15555550199 -t 1 2`
- Shared result format: [_send-results.md](_send-results.md).

## Summary

Help line: `Send a read or viewed receipt to a previously received message.`

Sends a read (default) or viewed receipt for one or more message timestamps to
the original sender.

## Invocation category & account requirement

Local single-account command; requires a registered account (see index.md).
Contacts the Signal server.

## Output modes

`plain-text` (default) and `json`.

## Arguments

| Names | Arity | Value | Default | Required | Help text (verbatim) | Notes |
|---|---|---|---|---|---|---|
| `recipient` (positional) | exactly 1 | string | none | yes | Specify the sender's phone number. | recipient syntax (number, UUID, `PNI:`, `u:`) |
| `-t`, `--target-timestamp` | 1..n | int64 | none | yes | Specify the timestamp of the messages for which a receipt should be sent. | |
| `--type` | 1 | `{read,viewed}` | none (treated as `read`) | no | Specify the receipt type (default is read receipt). | case-sensitive; uses the "invalid choice" error form |
| `-h`, `--help` | 0 | | | | show this help message and exit | |

Because `-t` takes one or more values, a positional recipient placed directly
after the `-t` values is consumed as another timestamp (a `+`-prefixed phone
number parses as an int64), and parsing then fails with `too few arguments`
(observed: `sendReceipt -t 1 2 +15555550199`). Placing the recipient before
`-t` works (observed: `sendReceipt +15555550199 -t 1 2` reaches the account check).

## Constraints & validation

After account loading: recipient parsed (`Invalid phone number '<s>': <reason>`,
exit 1). Unknown type cannot occur from the CLI (parser restricts choices).

## Behavior

Sends one receipt message of the given type listing all target timestamps.

## Plain-text output

See [_send-results.md](_send-results.md).

## JSON output

See [_send-results.md](_send-results.md).

## Errors

| Condition | Stream | Exact message | Exit |
|---|---|---|---|
| missing recipient (parse) | stderr | usage + `signal-cli: error: too few arguments` (observed, also when only `-t 1` given) | 1 |
| missing `-t` (parse) | stderr | usage + `signal-cli: error: argument -t/--target-timestamp is required` (observed) | 1 |
| bad `--type` (parse) | stderr | usage + `signal-cli: error: argument  --type:  invalid  choice:  'foo'  (choose from` / `{read,viewed})` (observed) | 1 |
| non-integer timestamp (parse) | stderr | usage + `signal-cli: error: argument  -t/--target-timestamp:  could  not convert 'x'` / `to integer (64 bits)` (observed) | 1 |
| no account / not registered | stderr | see index.md | 1 |
| invalid recipient | stderr | `Invalid phone number '<s>': <reason>` | 1 |
| send failed | stderr | per [_send-results.md](_send-results.md) | 4/5/1 |

## --help transcript

Command: `signal-cli --config <CONFIG> sendReceipt --help` -- exit code: 0, stderr empty. Stdout:

```text
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli sendReceipt [-h]
                  -t TARGET_TIMESTAMP [TARGET_TIMESTAMP ...]
                  [--type {read,viewed}] recipient

positional arguments:
  recipient              Specify the sender's phone number.

named arguments:
  -h, --help             show this help message and exit
  -t TARGET_TIMESTAMP [TARGET_TIMESTAMP ...], --target-timestamp TARGET_TIMESTAMP [TARGET_TIMESTAMP ...]
                         Specify the timestamp of the  messages for which a
                         receipt should be sent.
  --type {read,viewed}   Specify  the  receipt   type   (default   is  read
                         receipt).
```

## Doc vs binary discrepancies

- Man page lists `-u/--username` for sendReceipt; the binary has no such option
  (the recipient positional accepts `u:<name>` instead).
- Man page help for `-t` says "Specify the timestamp of the message to which to
  react." (copy-paste); binary help text differs.
