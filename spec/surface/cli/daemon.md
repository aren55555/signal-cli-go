# signal-cli daemon

## Sources
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/commands/DaemonCommand.java
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/commands/ReceiveMode.java
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/util/IOUtils.java (socket path default, address parsing, bind errors)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/DbusConfig.java (default bus name)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/App.java (multi-account vs single-account dispatch)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/man/signal-cli.1.adoc, section `=== daemon`; https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/man/signal-cli-jsonrpc.5.adoc (protocol summary)
- Received-message output: [_received-message-output.md](_received-message-output.md)
- Binary observations (account-free; no daemon was actually started):
  - `signal-cli --config <CONFIG> daemon --help`
  - `signal-cli --config <CONFIG> daemon --receive-mode bogus`
  - `signal-cli --config <CONFIG> -a +15555550123 daemon`
  - `signal-cli --config <CONFIG> -a +15555550123 daemon --http`

## Summary
Help text: "Run in daemon mode and provide a JSON-RPC or an experimental dbus interface."

The command is a long-running server. It exposes the JSON-RPC API on any combination of these transports: a UNIX socket, a TCP socket, HTTP, and D-Bus (user or system bus). It also accepts a listening socket inherited from the parent process, for example through systemd socket activation. Received messages are, by default, also printed to stdout in the selected output mode.

The JSON-RPC protocol is specified separately; see `man/signal-cli-jsonrpc.5.adoc` and the JSON-RPC spec in this repo. In brief:
- Requests are JSON-RPC 2.0. Each line is one request on the socket and TCP transports.
- Over `--http`, the endpoints are `POST /api/v1/rpc` (a single request or a batch), `GET /api/v1/events` (an SSE stream of received messages), and `GET /api/v1/check` (returns 200 when running).
- Method names equal CLI command names, and parameters are camelCase.
- Extra methods: `subscribeReceive`, `unsubscribeReceive`, `subscribeCallEvents`, `unsubscribeCallEvents`, and, in multi-account mode only, `startLink`, `finishLink`, and `register`/`verify`.

## Invocation category & account requirement
- With `-a ACCOUNT`, the daemon runs in single-account mode. The account must be registered; otherwise it fails with `User <ACCOUNT> is not registered.` (exit 1, observed).
  - The info log line is `Starting daemon in single-account mode for <NUMBER>`.
- Without `-a`, the daemon runs in multi-account mode. It loads all registered local accounts.
  - Zero accounts is allowed; accounts can be added later through `register` or `startLink`/`finishLink` over JSON-RPC.
  - Each JSON-RPC request must then carry an `account` param.
  - Unregistered accounts are skipped with a stderr warning of the form `WARN  MultiAccountManager - Ignoring <NUMBER>: User is not registered. (NotRegisteredException)`. This was observed with `listAccounts`, which uses the same loader.
  - The info log line is `Starting daemon in multi-account mode`.

## Output modes
- `plain-text` (the default) and `json` are both supported.
- The output mode controls only how received messages are printed to stdout. See [_received-message-output.md](_received-message-output.md); in JSON mode the output is one object per line.
- With `--no-receive-stdout`, nothing is printed for received messages.
- JSON-RPC responses go to the clients, not to stdout.

## Arguments

| names | arity | value type / metavar | default | required | help text (verbatim) | notes |
|---|---|---|---|---|---|---|
| `-h`, `--help` | 0 | – | – | no | `show this help message and exit` | |
| `--dbus` | 0 | flag | false | no | `Expose a DBus interface on the user bus.` | This is separate from the global `--dbus`, which makes a client request instead. |
| `--dbus-system`, `--system` | 0 | flag | false | no | `Expose a DBus interface on the system bus.` | |
| `--bus-name` | 1 | string / `BUS_NAME` | `org.asamk.Signal`. If `/.flatpak-info` exists, the default is `org.asamk.SignalCli`. | no | `Specify the D-Bus bus name to connect to.` | Used for both D-Bus exports. This is separate from the global `--bus-name`. |
| `--socket` | 0 or 1 (optional value) | file path / `SOCKET` | Absent means not enabled. When given without a value, the path is `$XDG_RUNTIME_DIR/signal-cli/socket`, or `<java tmpdir>/signal-cli/socket` when `XDG_RUNTIME_DIR` is unset. | no | `Expose a JSON-RPC interface on a UNIX socket (default $XDG_RUNTIME_DIR/signal-cli/socket).` | Parent directories are created privately. The socket file is deleted on exit. |
| `--tcp` | 0 or 1 | `HOST:PORT` / `TCP` | When given without a value: `localhost:7583` | no | `Expose a JSON-RPC interface on a TCP socket (default localhost:7583).` | |
| `--http` | 0 or 1 | `HOST:PORT` / `HTTP` | When given without a value: `localhost:8080` | no | `Expose a JSON-RPC interface as http endpoint (default localhost:8080).` | |
| `--no-receive-stdout` | 0 | flag | false | no | `Don’t print received messages to stdout.` | U+2019 apostrophe. |
| `--receive-mode` | 1 | choice / `{on-start,on-connection,manual}` | `on-start` | no | `Specify when to start receiving messages.` | |
| `--ignore-attachments` | 0 | flag | false | no | `Don’t download attachments of received messages.` | |
| `--ignore-stories` | 0 | flag | false | no | `Don’t receive story messages from the server.` | |
| `--ignore-avatars` | 0 | flag | false | no | `Don't download avatars of received messages.` | |
| `--ignore-stickers` | 0 | flag | false | no | `Don't download sticker packs of received messages.` | |
| `--send-read-receipts` | 0 | flag | false | no | `Send read receipts for all incoming data messages (in addition to the default delivery receipts)` | |

There are no positionals and no mutually exclusive groups. `--dbus` and `--dbus-system` may both be given.

For the optional-value flags (`--socket`, `--tcp`, `--http`):
- `--tcp` alone uses the default value.
- `--tcp=HOST:PORT` and `--tcp HOST:PORT` set the value.
- The next token is consumed as the value unless it starts with `-`.

## Constraints & validation
- At least one channel is required: `--socket`, `--tcp`, `--http`, `--dbus`, `--dbus-system`, or an inherited server socket. Otherwise the command fails with `At least one channel parameter is required, e.g. --socket or --dbus.` (exit 1). This check runs after any channels that were requested have been set up.
- TCP and HTTP address parsing:
  - The address is split at the last `:`. A missing colon gives `Invalid tcp bind address (expected host:port): <value>` (exit 1).
  - A non-integer port gives `Invalid tcp port: <port>` (exit 1).
  - An unresolvable host gives `Invalid tcp bind address, invalid host: <host>` (exit 1).
- A bind failure for the socket or TCP transport gives `Failed to bind socket <address>: <msg>` (exit 3).
- An HTTP server start failure gives `Failed to initialize HTTP Server` (exit 3).
- A failure to use an inherited socket gives `Failed to use inherited socket` (exit 3).

## Behavior
- Startup order:
  1. Install the shutdown handler.
  2. Configure receiving on each account.
  3. Register the stdout printer for received messages.
  4. Set up channels in this order: inherited socket, `--socket`, `--tcp`, `--http`, `--dbus-system`, `--dbus`.
- The process then blocks until shutdown. Shutdown is triggered by SIGINT/SIGTERM or, in single-account mode, when the account's manager closes.
- On shutdown, all channels are closed and the exit status is 0.
- `--receive-mode`:
  - `on-start`: start receiving immediately for all accounts.
  - `on-connection`: start receiving only while at least one client is connected. The stdout printer is registered as a weak listener, so it does not keep receiving alive.
  - `manual`: clients must call `subscribeReceive` (on socket/TCP); messages are not pushed automatically.
- For D-Bus, any mode other than `on-start` means receiving is not started at startup.
- Received-message notifications to JSON-RPC clients have the method `receive`. The params are the envelope object `{"envelope":...,"account":...}`. For explicit subscriptions they are instead `{"subscription":N,"result":{...}}`.

## Plain-text output
The stdout stream shows received envelopes in the plain-text block format of [_received-message-output.md](_received-message-output.md). Logs go to stderr.

## JSON output
With `-o json`, received envelopes are printed as JSON Lines, using the same object as `receive -o json`.

## Errors

| condition | stream | exact message | exit code |
|---|---|---|---|
| Parse error (for example a bad `--receive-mode`) | stderr | usage + `signal-cli: error: argument --receive-mode: could not convert 'bogus' (choose from {on-start,on-connection,manual})` (re-wrapped; see Observed) | 1 |
| `-a` given but account not registered (observed) | stderr | `User +15555550123 is not registered.` | 1 |
| `-a` invalid | stderr | `Invalid account (phone number), make sure you include the country code.` | 1 |
| No channel | stderr | `At least one channel parameter is required, e.g. --socket or --dbus.` | 1 |
| Bad TCP/HTTP address | stderr | see Constraints | 1 |
| Bind failure | stderr | `Failed to bind socket <addr>: <msg>` | 3 |
| HTTP init failure | stderr | `Failed to initialize HTTP Server` | 3 |
| Multi-account: accounts file unreadable | stderr | `Failed to load local accounts file` | 3 |

## --help transcript
Command: `signal-cli --config <CONFIG> daemon --help`

exit code: 0

stdout:
```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli daemon [-h] [--dbus] [--dbus-system]
                  [--bus-name BUS_NAME] [--socket [SOCKET]] [--tcp [TCP]]
                  [--http [HTTP]] [--no-receive-stdout]
                  [--receive-mode {on-start,on-connection,manual}]
                  [--ignore-attachments] [--ignore-stories]
                  [--ignore-avatars] [--ignore-stickers]
                  [--send-read-receipts]

named arguments:
  -h, --help             show this help message and exit
  --dbus                 Expose a DBus interface on the user bus.
  --dbus-system, --system
                         Expose a DBus interface on the system bus.
  --bus-name BUS_NAME    Specify the D-Bus bus name to connect to.
  --socket [SOCKET]      Expose a  JSON-RPC  interface  on  a  UNIX  socket
                         (default $XDG_RUNTIME_DIR/signal-cli/socket).
  --tcp [TCP]            Expose  a  JSON-RPC  interface  on  a  TCP  socket
                         (default localhost:7583).
  --http [HTTP]          Expose  a  JSON-RPC  interface  as  http  endpoint
                         (default localhost:8080).
  --no-receive-stdout    Don?t print received messages to stdout.
  --receive-mode {on-start,on-connection,manual}
                         Specify when to start receiving messages.
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


Charset note: With LANG unset, "Don’t" (U+2019) is printed as `Don?t`. With a UTF-8 locale, U+2019 is printed.

## Observed invocations
### `signal-cli --config <CONFIG> daemon --receive-mode bogus`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli daemon [-h] [--dbus] [--dbus-system]
                  [--bus-name BUS_NAME] [--socket [SOCKET]] [--tcp [TCP]]
                  [--http [HTTP]] [--no-receive-stdout]
                  [--receive-mode {on-start,on-connection,manual}]
                  [--ignore-attachments] [--ignore-stories]
                  [--ignore-avatars] [--ignore-stickers]
                  [--send-read-receipts]
signal-cli: error:  argument  --receive-mode:  could  not  convert  'bogus'
(choose from {on-start,on-connection,manual})
```

### `signal-cli --config <CONFIG> -a +15555550123 daemon`

exit code: 1

stdout:
```
```

stderr:
```
User +15555550123 is not registered.
```

### `signal-cli --config <CONFIG> -a +15555550123 daemon --http`

exit code: 1

stdout:
```
```

stderr:
```
User +15555550123 is not registered.
```


## Doc vs binary discrepancies
- The man page describes `--dbus-system` but not its alias `--system`. The binary accepts `--system`.
- For the `--receive-mode` values, the man page lists "(on-start, on-connection, manual)". This matches the binary.
- The man page wording is "Export"/"Claim another D-Bus bus name than the default". The binary help text is as recorded above.
- The "At least one channel parameter is required" case was not run against the binary: running `daemon` without `-a` was deliberately avoided. Its message is taken from source.
