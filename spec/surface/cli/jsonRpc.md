# signal-cli jsonRpc

## Sources
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/commands/JsonRpcDispatcherCommand.java
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/jsonrpc/SignalJsonRpcDispatcherHandler.java and JsonRpcReader.java (stdin loop, EOF handling, notifications)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/App.java (output-type check, account dispatch)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/man/signal-cli.1.adoc, section `=== jsonRpc`; https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/man/signal-cli-jsonrpc.5.adoc
- Received-message output: [_received-message-output.md](_received-message-output.md)
- Binary observations (account-free):
  - `signal-cli --config <CONFIG> jsonRpc --help`
  - `signal-cli --config <CONFIG> jsonRpc --receive-mode bogus`
  - `signal-cli --config <CONFIG> jsonRpc --no-receive-stdout`
  - `signal-cli --config <CONFIG> -a +15555550123 jsonRpc`
  - `signal-cli --config <CONFIG> -o plain-text jsonRpc` (recorded by the parent agent in observations.md)

## Summary
Help text: "Take commands from standard input as line-delimited JSON RPC while receiving messages."

The command reads JSON-RPC 2.0 requests from stdin, one JSON value per line, and writes responses and notifications to stdout, one JSON object per line. It also receives messages and pushes them as `receive` notifications.

The JSON-RPC protocol itself is specified separately: see `man/signal-cli-jsonrpc.5.adoc` and the repo's JSON-RPC spec.

## Invocation category & account requirement
- With `-a ACCOUNT`, the command runs in single-account mode, and the account must be registered. Otherwise it fails with `User <ACCOUNT> is not registered.` (exit 1, observed).
- Without `-a`, it runs in multi-account mode:
  - All registered local accounts are loaded. Zero accounts is allowed.
  - Requests that target an account need an `account` param.
  - The extra methods `register`, `verify`, `startLink`, and `finishLink` are available.

## Output modes
- **JSON only.** If `-o` is not given, JSON is used automatically.
- `-o plain-text jsonRpc` fails with `Command doesn't support output type plain-text` (exit 1). This check happens before any account resolution.

## Arguments

| names | arity | value type / metavar | default | required | help text (verbatim) | notes |
|---|---|---|---|---|---|---|
| `-h`, `--help` | 0 | – | – | no | `show this help message and exit` | |
| `--ignore-attachments` | 0 | flag | false | no | `Don’t download attachments of received messages.` | U+2019 |
| `--ignore-stories` | 0 | flag | false | no | `Don’t receive story messages from the server.` | U+2019 |
| `--ignore-avatars` | 0 | flag | false | no | `Don't download avatars of received messages.` | |
| `--ignore-stickers` | 0 | flag | false | no | `Don't download sticker packs of received messages.` | |
| `--send-read-receipts` | 0 | flag | false | no | `Send read receipts for all incoming data messages (in addition to the default delivery receipts)` | |
| `--receive-mode` | 1 | choice / `{on-start,on-connection,manual}` | `on-start` | no | `Specify when to start receiving messages.` | `on-connection` is accepted but behaves like `on-start`, because the stdin "connection" exists from the start. Only `manual` disables automatic receiving. |

There are no positionals. Unlike `daemon`, `jsonRpc` has no `--no-receive-stdout`; it is rejected as unrecognized (observed).

## Constraints & validation
- Parse errors exit 1 with usage on stderr.
- The output type must be JSON.

## Behavior
- After start-up (unless `--receive-mode manual`), the command subscribes to received messages for all loaded accounts. In multi-account mode this includes accounts added later.
- Each received envelope is written as a JSON-RPC notification:
  `{"jsonrpc":"2.0","method":"receive","params":{"envelope":{...},"account":"+NUMBER"}}`
  The params object is specified in [_received-message-output.md](_received-message-output.md).
- With `manual`, a client sends `subscribeReceive`, which returns an integer subscription id. Ids come from a single process-wide counter that starts at 0 and is shared by receive and call-event subscriptions. Notifications then carry `"params":{"subscription":<id>,"result":{...}}`. `unsubscribeReceive` takes `subscription`.
- `subscribeCallEvents` and `unsubscribeCallEvents` produce `callEvent` notifications with the same subscription/result wrapping.
- Requests are processed concurrently, so responses may come back in a different order from the requests. Clients must match responses by `id`.
- A line that is not valid JSON, or is not a valid JSON-RPC message, produces an error response with `"id":null` and is then skipped. An empty or whitespace-only line also produces a parse-error response, with the message `Empty input line`. The error codes are defined in the JSON-RPC spec.
- **EOF on stdin ends the command.** When stdin is closed, the read loop stops, subscriptions are removed, and the process exits with status 0.
- SIGINT/SIGTERM also interrupt the loop and shut down.
- In single-account mode, the loop also stops if the account's manager closes.

## Plain-text output
Not supported.

## JSON output
stdout carries only JSON-RPC response objects and notification objects, each on its own line. Logs go to stderr.

## Errors

| condition | stream | exact message | exit code |
|---|---|---|---|
| `-o plain-text` | stderr | `Command doesn't support output type plain-text` | 1 |
| Parse error (bad `--receive-mode`, unknown flag) | stderr | usage + `signal-cli: error: ...` (see Observed) | 1 |
| `-a` not registered (observed) | stderr | `User +15555550123 is not registered.` | 1 |
| `-a` invalid | stderr | `Invalid account (phone number), make sure you include the country code.` | 1 |
| Multi-account: accounts file unreadable | stderr | `Failed to load local accounts file` | 3 |

## --help transcript
Command: `signal-cli --config <CONFIG> jsonRpc --help`

exit code: 0

stdout:
```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli jsonRpc [-h] [--ignore-attachments] [--ignore-stories]
                  [--ignore-avatars] [--ignore-stickers]
                  [--send-read-receipts]
                  [--receive-mode {on-start,on-connection,manual}]

named arguments:
  -h, --help             show this help message and exit
  --ignore-attachments   Don?t download attachments of received messages.
  --ignore-stories       Don?t receive story messages from the server.
  --ignore-avatars       Don't download avatars of received messages.
  --ignore-stickers      Don't download sticker packs of received messages.
  --send-read-receipts   Send read receipts for  all incoming data messages
                         (in addition to the default delivery receipts)
  --receive-mode {on-start,on-connection,manual}
                         Specify when to start receiving messages.
```

stderr:
```
```


Charset note: With LANG unset, U+2019 is printed as `?`. With a UTF-8 locale it is printed as-is.

## Observed invocations
### `signal-cli --config <CONFIG> -o plain-text jsonRpc`

exit code: 1

stdout:
```
```

stderr:
```
Command doesn't support output type plain-text
```

### `signal-cli --config <CONFIG> jsonRpc --receive-mode bogus`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli jsonRpc [-h] [--ignore-attachments] [--ignore-stories]
                  [--ignore-avatars] [--ignore-stickers]
                  [--send-read-receipts]
                  [--receive-mode {on-start,on-connection,manual}]
signal-cli: error:  argument  --receive-mode:  could  not  convert  'bogus'
(choose from {on-start,on-connection,manual})
```

### `signal-cli --config <CONFIG> -a +15555550123 jsonRpc`

exit code: 1

stdout:
```
```

stderr:
```
User +15555550123 is not registered.
```

### `signal-cli --config <CONFIG> jsonRpc --no-receive-stdout`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli jsonRpc [-h] [--ignore-attachments] [--ignore-stories]
                  [--ignore-avatars] [--ignore-stickers]
                  [--send-read-receipts]
                  [--receive-mode {on-start,on-connection,manual}]
signal-cli: error: unrecognized arguments: '--no-receive-stdout'
```


## Doc vs binary discrepancies
- The man page says `--receive-mode` takes "(on-start, manual)". The binary accepts and lists `{on-start,on-connection,manual}`. The binary is authoritative.
- The jsonrpc man page example notification omits `account` and the server timestamps. The binary includes them; see [_received-message-output.md](_received-message-output.md).
