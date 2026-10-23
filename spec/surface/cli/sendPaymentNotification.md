# signal-cli sendPaymentNotification

## Sources

- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/commands/SendPaymentNotificationCommand.java (tag v0.14.7)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/util/CommandUtil.java
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/util/SendMessageResultUtils.java
- man page https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/man/signal-cli.1.adoc section `=== sendPaymentNotification`
- Binary commands run (`--config <CONFIG>` empty scratch dir):
  - `signal-cli --config <CONFIG> sendPaymentNotification --help`
  - `signal-cli --config <CONFIG> sendPaymentNotification`
  - `signal-cli --config <CONFIG> sendPaymentNotification --receipt abc`
  - `signal-cli --config <CONFIG> sendPaymentNotification +15555550199`
  - `signal-cli --config <CONFIG> sendPaymentNotification --receipt abc +15555550199 +15555550198`
- Shared result format: [_send-results.md](_send-results.md).

## Summary

Help line: `Send a payment notification.`

Sends a (MobileCoin) payment notification message carrying an opaque receipt
blob and optional note to exactly one recipient.

## Invocation category & account requirement

Local single-account command; requires a registered account (see index.md).
Contacts the Signal server.

## Output modes

`plain-text` (default) and `json`.

## Arguments

| Names | Arity | Value | Default | Required | Help text (verbatim) | Notes |
|---|---|---|---|---|---|---|
| `recipient` (positional) | exactly 1 | string | none | yes | Specify the recipient's phone number. | recipient syntax (number, UUID, `PNI:`, `u:`) |
| `--receipt` | 1 | RECEIPT | none | yes | The base64 encoded receipt blob. | standard base64 |
| `--note` | 1 | NOTE | none | no | Specify a note for the payment notification. | |
| `-h`, `--help` | 0 | | | | show this help message and exit | |

## Constraints & validation

After account loading:
1. Recipient parsed; invalid -> `Invalid phone number '<s>': <reason>` (exit 1).
2. Receipt decoded as standard base64. Invalid base64 is not handled cleanly: the
   reference fails with an internal error (Java stack trace on stderr, exit 2)
   -- derived from source, unverified.

## Behavior

Sends the payment notification (receipt bytes, note) to the recipient.

## Plain-text output

See [_send-results.md](_send-results.md).

## JSON output

See [_send-results.md](_send-results.md).

## Errors

| Condition | Stream | Exact message | Exit |
|---|---|---|---|
| missing recipient (parse) | stderr | usage + `signal-cli: error: too few arguments` (observed) | 1 |
| missing `--receipt` (parse) | stderr | usage + `signal-cli: error: argument --receipt is required` (observed) | 1 |
| more than one recipient (parse) | stderr | usage + `signal-cli: error: unrecognized arguments: '+15555550198'` (observed) | 1 |
| no account / not registered | stderr | see index.md | 1 |
| invalid recipient | stderr | `Invalid phone number '<s>': <reason>` | 1 |
| invalid base64 receipt | stderr | Java stack trace (unclean) | 2 |
| I/O error | stderr | `Failed to send message: <msg> (<ExceptionName>)` | 2 |
| send failed for the recipient | stderr | per [_send-results.md](_send-results.md) | 4/5/1 |

## --help transcript

Command: `signal-cli --config <CONFIG> sendPaymentNotification --help` -- exit code: 0, stderr empty. Stdout:

```text
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli sendPaymentNotification [-h] --receipt RECEIPT
                  [--note NOTE] recipient

positional arguments:
  recipient              Specify the recipient's phone number.

named arguments:
  -h, --help             show this help message and exit
  --receipt RECEIPT      The base64 encoded receipt blob.
  --note NOTE            Specify a note for the payment notification.
```

## Doc vs binary discrepancies

None found.
