# signal-cli receive

## Sources
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/commands/ReceiveCommand.java
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/util/CommandUtil.java (receive-config flags)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/App.java and Main.java (account resolution, exit codes)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/man/signal-cli.1.adoc, section `=== receive`
- Output format: see [_received-message-output.md](_received-message-output.md)
- Binary observations (signal-cli 0.14.7, `--config <CONFIG>`, LANG unset, account-free):
  - `signal-cli --config <CONFIG> receive --help`
  - `signal-cli --config <CONFIG> receive` (with no local accounts)
  - `signal-cli --config <CONFIG> receive -t abc`
  - `signal-cli --config <CONFIG> receive --max-messages 1.5`
  - `signal-cli --config <CONFIG> -a +15555550123 receive`
  - `signal-cli --config <CONFIG> -a +15555550123 -o json receive -t 1`

## Summary
Help text: "Query the server for new messages."

The command connects to the Signal service for one account. It receives pending and newly arriving envelopes, and prints each envelope as it is handled. It stops when one of these happens:
- the idle timeout expires, meaning no new message arrived within TIMEOUT seconds;
- `--max-messages` envelopes have been received;
- it is interrupted with SIGINT or SIGTERM. A shutdown hook stops receiving gracefully.

Attachments, avatars, and sticker packs are downloaded into the data directory unless the matching `--ignore-*` flag is given.

## Invocation category & account requirement
This is a single-account local command and needs a registered account.
- If `-a` is omitted, the single local account is used.
- If there are no local accounts, the command fails with `No local users found, you first need to register or link an account` (exit 1).
- If there are several, it fails with `Multiple users found, you need to specify an account (phone number) with -a` (exit 1).

## Output modes
- `plain-text` (the default) and `json` are both supported.
- Plain text uses the human-readable block format.
- JSON prints one JSON object per envelope, one per line.
- Both formats are specified in [_received-message-output.md](_received-message-output.md).

## Arguments

| names | arity | value type / metavar | default | required | help text (verbatim) | notes |
|---|---|---|---|---|---|---|
| `-h`, `--help` | 0 | – | – | no | `show this help message and exit` | Prints help on stdout and exits 0. |
| `-t`, `--timeout` | 1 | double / `TIMEOUT` | `3.0` | no | `Number of seconds to wait for new messages (negative values disable timeout)` | The value is a decimal number of seconds and is converted to whole milliseconds by truncation. A negative value means there is no timeout: the command waits indefinitely. The default is not shown in `--help`. |
| `--max-messages` | 1 | 32-bit int / `MAX_MESSAGES` | `-1` | no | `Maximum number of messages to receive, before returning.` | A negative value means there is no limit. |
| `--ignore-attachments` | 0 | flag | false | no | `Don’t download attachments of received messages.` | Uses U+2019. See the charset note below. |
| `--ignore-stories` | 0 | flag | false | no | `Don’t receive story messages from the server.` | |
| `--ignore-avatars` | 0 | flag | false | no | `Don't download avatars of received messages.` | |
| `--ignore-stickers` | 0 | flag | false | no | `Don't download sticker packs of received messages.` | |
| `--send-read-receipts` | 0 | flag | false | no | `Send read receipts for all incoming data messages (in addition to the default delivery receipts)` | |

There are no positionals and no mutually exclusive groups.

## Constraints & validation
- Parse-time errors exit 1 and print the subcommand usage plus an error line on stderr. The message text below is shown in normalized form. The binary re-wraps it to about 75 columns with padded spaces; the verbatim output is under "Observed invocations".
  - A `--timeout` value that is not a number gives: `argument -t/--timeout: could not convert 'abc' to number (double precision)`.
  - A `--max-messages` value that is not an integer gives: `argument --max-messages: could not convert '1.5' to integer (32 bits)`.
- Both orderings are accepted: `-t 0` stops as soon as the queue is drained, and a negative timeout waits indefinitely.

## Behavior
- The receive configuration is applied before receiving. It consists of: ignore attachments, ignore stories, ignore avatars, ignore stickers, and send read receipts.
- Envelopes are printed in the order they are handled. In JSON mode each envelope is flushed immediately as a line.
- Delivery receipts are sent automatically for incoming data messages. With `--send-read-receipts`, read receipts are sent as well.
- If this account is already receiving in the same process (only relevant inside the daemon), the command fails with `Receive command cannot be used if messages are already being received.`

## Plain-text output
See [_received-message-output.md](_received-message-output.md), "Plain-text rendering". Each envelope block ends with an empty line. If no messages are received, nothing is printed.

## JSON output
- One line per envelope: `{"envelope":{...},"account":"+NUMBER"}`. An `"exception":{"message":...,"type":...}` key comes first when handling the envelope failed.
- There is no surrounding array and no output when no messages arrive.
- The full key list is in [_received-message-output.md](_received-message-output.md).

This differs from the JSON-RPC `receive` method, which is used only when the daemon runs with `--receive-mode manual`. There the result is a single JSON array of these envelope objects. That method takes `timeout` and `maxMessages` params, with defaults 3.0 and -1.

## Errors

| condition | stream | exact message | exit code |
|---|---|---|---|
| Parse error (bad option or value) | stderr | usage + `signal-cli: error: ...` | 1 |
| No local accounts and no `-a` | stderr | `No local users found, you first need to register or link an account` | 1 |
| Several local accounts and no `-a` | stderr | `Multiple users found, you need to specify an account (phone number) with -a` | 1 |
| `-a` is not a valid E.164 number or UUID | stderr | `Invalid account (phone number), make sure you include the country code.` | 1 |
| Account unknown or not registered (observed) | stderr | `User +15555550123 is not registered.` | 1 |
| Account state file cannot be loaded | stderr | `Error loading state file for user <ACCOUNT>: <msg> (<ExceptionClass>)` | 2 |
| Account check failed due to I/O | stderr | `Error while checking account <ACCOUNT>: <msg>` | 3 |
| Network or I/O error while receiving | stderr | `Error while receiving messages: <msg>` | 3 |
| Already receiving | stderr | `Receive command cannot be used if messages are already being received.` | 1 |

## --help transcript
Command: `signal-cli --config <CONFIG> receive --help`

exit code: 0

stdout:
```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli receive [-h] [-t TIMEOUT] [--max-messages MAX_MESSAGES]
                  [--ignore-attachments] [--ignore-stories]
                  [--ignore-avatars] [--ignore-stickers]
                  [--send-read-receipts]

named arguments:
  -h, --help             show this help message and exit
  -t TIMEOUT, --timeout TIMEOUT
                         Number  of  seconds  to   wait  for  new  messages
                         (negative values disable timeout)
  --max-messages MAX_MESSAGES
                         Maximum number  of  messages  to  receive,  before
                         returning.
  --ignore-attachments   Don?t download attachments of received messages.
  --ignore-stories       Don?t receive story messages from the server.
  --ignore-avatars       Don't download avatars of received messages.
  --ignore-stickers      Don't download sticker packs of received messages.
  --send-read-receipts   Send read receipts for  all incoming data messages
                         (in addition to the default delivery receipts)
```

stderr:
```
```


Charset note: The transcript above was captured with LANG unset, so U+2019 (’) in "Don’t" appears as `?`. With a UTF-8 locale, for example `LANG=C.UTF-8`, the binary prints U+2019 (verified for `receive --help`).

## Observed invocations
### `signal-cli --config <CONFIG> receive`

exit code: 1

stdout:
```
```

stderr:
```
No local users found, you first need to register or link an account
```

### `signal-cli --config <CONFIG> receive -t abc`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli receive [-h] [-t TIMEOUT] [--max-messages MAX_MESSAGES]
                  [--ignore-attachments] [--ignore-stories]
                  [--ignore-avatars] [--ignore-stickers]
                  [--send-read-receipts]
signal-cli: error:  argument  -t/--timeout:  could  not  convert  'abc'  to
number (double precision)
```

### `signal-cli --config <CONFIG> receive --max-messages 1.5`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli receive [-h] [-t TIMEOUT] [--max-messages MAX_MESSAGES]
                  [--ignore-attachments] [--ignore-stories]
                  [--ignore-avatars] [--ignore-stickers]
                  [--send-read-receipts]
signal-cli: error: argument  --max-messages:  could  not  convert  '1.5' to
integer (32 bits)
```

### `signal-cli --config <CONFIG> -a +15555550123 receive`

exit code: 1

stdout:
```
```

stderr:
```
User +15555550123 is not registered.
```

### `signal-cli --config <CONFIG> -a +15555550123 -o json receive -t 1`

exit code: 1

stdout:
```
```

stderr:
```
User +15555550123 is not registered.
```


## Doc vs binary discrepancies
- The man page says "Default is 5 seconds" for `--timeout`. The binary default is **3.0 seconds**, and `--help` shows no default. The binary is authoritative.
- The man page says JSON output is "one json object per line". This is consistent with the binary.
