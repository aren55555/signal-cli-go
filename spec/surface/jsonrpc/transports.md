# signal-cli 0.14.7 JSON-RPC: daemon flags and transports

## Sources

- Upstream signal-cli, tag v0.14.7 (https://github.com/AsamK/signal-cli/tree/v0.14.7):
  - `src/main/java/org/asamk/signal/commands/DaemonCommand.java`
  - `src/main/java/org/asamk/signal/commands/JsonRpcDispatcherCommand.java`
  - `src/main/java/org/asamk/signal/commands/ReceiveMode.java`
  - `src/main/java/org/asamk/signal/jsonrpc/` (all files: reader, request/response, dispatcher, command handler, socket handler)
  - `src/main/java/org/asamk/signal/http/HttpServerHandler.java`, `http/ServerSentEventSender.java`
  - `src/main/java/org/asamk/signal/output/JsonWriterImpl.java`, `util/IOUtils.java`, `util/Util.java`, `util/CommandUtil.java`
  - `src/main/java/org/asamk/signal/json/JsonReceiveMessageHandler.java`
  - `man/signal-cli-jsonrpc.5.adoc`, `man/signal-cli.1.adoc`
- Binary observation (signal-cli 0.14.7, zero accounts, multi-account mode). Exact commands and
  raw bytes are in `observations.md`. Commands used:
  - `signal-cli --config <CONFIG> daemon --help`, `signal-cli --config <CONFIG> jsonRpc --help`
  - `signal-cli --config <CONFIG> daemon --http 127.0.0.1:<PORT> --tcp 127.0.0.1:<PORT> --socket <PATH>`
  - `signal-cli --config <CONFIG> daemon --receive-mode manual --tcp 127.0.0.1:<PORT> --http 0.0.0.0:<PORT>`
  - `... | signal-cli --config <CONFIG> jsonRpc`
  - HTTP probed with curl, TCP with bash `/dev/tcp`, UNIX socket with a perl IO::Socket::UNIX one-liner.

Not observable without an account: single-account mode, real `receive` notifications,
D-Bus. Those parts come from the source only.

## 1. Entry points

JSON-RPC is served by two CLI commands:

- `signal-cli [-a ACCOUNT] jsonRpc [flags]`: one JSON-RPC session over stdin and stdout.
  It supports only JSON output: with `-o plain-text` it prints
  `Command doesn't support output type plain-text` to stderr and exits with status 1.
  At end of stdin it finishes the requests already in flight and exits with status 0.
- `signal-cli [-a ACCOUNT] daemon [flags]`: a long-running server with any combination of
  UNIX socket, TCP, HTTP and D-Bus. D-Bus is out of scope for this project, but its flags
  are listed here.

**Single-account vs multi-account mode.** With the global `-a ACCOUNT` option the process
runs in single-account mode for that account. If the account doesn't exist locally, the
command fails at start-up: stderr `User +15550000000 is not registered.`, exit 1.
Without `-a` it runs in multi-account mode and serves every local account, including zero
accounts. Start-up logs `INFO  DaemonCommand - Starting daemon in multi-account mode` or
`Starting daemon in single-account mode for <number>`.

Multi-account mode has extra methods: `listAccounts`, `startLink`, `finishLink`,
`register`, `verify`, `deleteLocalAccountData`. Accounts that are added at runtime, through
`finishLink` or `verify`, are picked up by the live daemon. See `methods/index.md` for the
account-requirement classes.

## 2. `daemon` flags (exact names)

| Flag | Argument | Default when flag given without value | Meaning |
|---|---|---|---|
| `--dbus` | none | | D-Bus on the session bus (out of scope) |
| `--dbus-system`, alias `--system` | none | | D-Bus on the system bus (out of scope) |
| `--bus-name BUS_NAME` | string | default `org.asamk.Signal` (`org.asamk.SignalCli` when running as a Flatpak) | D-Bus name (out of scope) |
| `--socket [SOCKET]` | optional path | `$XDG_RUNTIME_DIR/signal-cli/socket`; if XDG_RUNTIME_DIR is unset, `<java tmpdir>/signal-cli/socket` | JSON-RPC over a UNIX stream socket |
| `--tcp [TCP]` | optional `host:port` | `localhost:7583` | JSON-RPC over TCP |
| `--http [HTTP]` | optional `host:port` | `localhost:8080` | JSON-RPC over HTTP plus SSE |
| `--no-receive-stdout` | none | | don't print received messages to the daemon's stdout |
| `--receive-mode {on-start,on-connection,manual}` | choice | `on-start` | when receiving starts (see section 7) |
| `--ignore-attachments` | none | | don't download attachments of received messages |
| `--ignore-stories` | none | | don't receive story messages |
| `--ignore-avatars` | none | | don't download avatars |
| `--ignore-stickers` | none | | don't download sticker packs |
| `--send-read-receipts` | none | | send read receipts for all incoming data messages (delivery receipts are always sent) |

The `jsonRpc` command has the same flags from `--ignore-attachments` through
`--send-read-receipts`, plus `--receive-mode`. It has no transport flags.

The help text is shown verbatim in `observations.md` and in `daemon --help`.

**Address parsing** (`--tcp`, `--http`). The value is split at the **last** `:`.
- No colon: `Invalid tcp bind address (expected host:port): <value>`, exit 1.
- Port not an integer: `Invalid tcp port: <port>`, exit 1.
- Host doesn't resolve: `Invalid tcp bind address, invalid host: <host>`, exit 1.

These messages also apply to `--http`.

**Bind failures**, all exit 3:
- TCP or UNIX socket: `Failed to bind socket <address>: <OS message>`. Examples are
  `Failed to bind socket /127.0.0.1:7583: Address already in use`, and a UNIX socket path
  that already exists gives `Address already in use`.
- HTTP: `Failed to initialize HTTP Server`.

**No transport.** If no transport flag is given and the process has no inherited listening
socket: `At least one channel parameter is required, e.g. --socket or --dbus.`, exit 1.

**Socket activation.** If the process inherits a listening server socket (systemd socket
activation, `System.inheritedChannel`), JSON-RPC is served on it the same way as `--socket`.
This is in addition to any flags.

**UNIX socket details.**
- The parent directory is created with owner-only permissions if it is missing.
- The socket file is created with the process umask (observed `srwxr-xr-x`).
- The socket file is deleted on normal shutdown (SIGTERM). A stale file prevents start-up.

**Stdout.** Unless `--no-receive-stdout` is given, each received message is also printed to
the daemon's stdout in the global output format:
- `-o json`: one object per line, the same object as the notification payload in
  `notifications.md`.
- Plain text: the human-readable receive format.

With `--receive-mode` other than `on-start`, this stdout listener is "weak": it does not
cause receiving to start by itself.

## 3. Stream transports: stdio, UNIX socket, TCP

All three use the same framing and dispatcher.

**Framing**
- Input is UTF-8 text, read **one line at a time**. A line ends at LF, CR or CRLF (Java
  `readLine`).
- Each line must hold exactly one complete JSON value: an object or an array (a batch).
  A request split across lines fails: each fragment gets its own error (see observations,
  section 5).
- After the first JSON value on a line, anything else is **ignored**. With
  `{...id:4} {...id:5}` on one line, only id 4 is handled.
- Output is one JSON value per line, followed by the platform line separator (LF on
  Linux). Writes are serialised, so a line is never interleaved with another.
- stdio uses the console charset for stdin; sockets use UTF-8.

**Concurrency and ordering**
- Each line is parsed when it is read. Parse and validation errors are written
  immediately.
- Valid requests run **concurrently**, so responses can come back in a different order
  from the requests. Clients must match responses by `id`.
- Notifications (`receive`, `callEvent`) are written on the same stream, interleaved with
  responses.

**Per-line handling.** Each line is handled by the first rule that applies. Exact messages
are in `errors.md`.
1. Blank or whitespace-only line: error -32700 `Empty input line`, `id: null`.
2. Not parseable as JSON: error -32700 with the parser message, `id: null`.
3. JSON `null`, number, string or boolean: -32600 `unexpected type: <NODE TYPE>`, `id: null`.
   `<NODE TYPE>` is `NULL`, `NUMBER`, `STRING` or `BOOLEAN`.
4. Empty array `[]`: -32600 `invalid request`, `id: null`.
5. Non-empty array: a batch (section 5).
6. Object that has a `result` key or an `error` key: treated as a JSON-RPC response from the
   client. It is matched against nothing and silently dropped, with no output.
7. Any other object: a request (section 4).

**Connection lifecycle**
- Every accepted socket connection is an independent JSON-RPC session with its own
  subscriptions.
- When the client closes its side, or a notification write fails because the socket is
  closed, all of that connection's subscriptions are removed.
- When the daemon shuts down, it closes the listening sockets and all client connections.
- In single-account mode, a connection's input loop is interrupted when the account's
  manager closes, which ends the session.

## 4. Request object rules (all transports)

- **Top-level keys.** Allowed keys are exactly `jsonrpc`, `method`, `params` and `id`. Any
  other key causes -32600 with the deserializer's "Unrecognized field" message, and the
  response echoes the request's `id`. This is strict: `{"jsonrpc":"2.0","method":"version","id":13,"extra":true}` is rejected.
- **`jsonrpc`.** Must equal the string `"2.0"` after coercion. A number such as `2.0` is
  accepted because it coerces to the text `"2.0"`. If it is missing or different:
  `only jsonrpc version 2.0 is supported`. This check runs before the `method` check, so
  `{}` gets this error.
- **`method`.** If it is missing or `null`: `method field must be set`. Scalar values are
  coerced to text, so `123` becomes method `"123"`, which gives `Method not implemented`.
  Method names are case-sensitive and use the camelCase command name.
- **`params`.**
  - May be omitted, or be an object or an array.
  - `null` is treated as omitted.
  - A scalar (string, number or boolean) causes -32600 "Problem deserializing property
    'params' ...".
- **`id`.**
  - May be a string, a number (integer or fractional, echoed back as given, e.g. `1.5`),
    or `null`.
  - An **explicit `"id": null` is a request, not a notification**: it gets a response with
    `"id":null`.
  - Only a **missing** `id` makes the request a notification.
  - An array or object `id` causes -32600 "Problem deserializing property 'id' ...", with
    `id: null` in the response.
- **Notifications** (no `id`) run normally, but their result and any error are discarded;
  nothing is written.

## 5. Batches

- A non-empty JSON array is a batch.
- Each element is validated as a request using the rules in section 4. An element that is
  not an object gets -32600 with a deserializer message such as
  `Cannot construct instance of ...` or `Cannot deserialize value of type ... from Array value`,
  with `id: null`.
- Inside a batch, an object with a `result` or `error` key is **not** treated as a response.
  It fails as an unrecognized field, with -32600 and its `id`.
- Validation errors for elements are collected first, in element order. Valid elements
  then run concurrently, and their responses are appended in completion order. Response
  order is therefore **not** the element order.
- Responses are written as **one JSON array**: one line on stream transports, one body over
  HTTP.
- If every element was a notification, nothing is written. Over HTTP this gives status 201.
- Empty batch `[]`: a single, non-array error object -32600 `invalid request` with `id: null`.

## 6. Account selection (multi-account mode)

Before the command runs:
- **single-account methods**: if `params.account` is present and not null, it is looked up
  by number or ACI and then **removed** from params. An unknown account gives -32602
  `Specified account does not exist`. If the key is absent or null:
  - exactly one local account: that account is used;
  - otherwise: -32602 `Method requires valid account parameter`.
- **multi-account methods** (`listAccounts`, `startLink`, `finishLink`): `account` is left in
  params and ignored. Observed: `listAccounts` with `{"account":"+15550000000"}` returns `[]`.
- **methods that support both** (`version`, `subscribeReceive`, `unsubscribeReceive`,
  `subscribeCallEvents`, `unsubscribeCallEvents`):
  - non-null `account`: it must exist (otherwise -32602 `Specified account does not exist`),
    and the method runs scoped to that account;
  - otherwise: it runs across all accounts.
- **registration methods** (`register`, `verify`, `deleteLocalAccountData`):
  - the `account` key must be present (`params.has("account")`); if absent, -32602
    `Method requires account parameter`. Its text value is used, so a JSON null becomes the
    string "null";
  - if the account cannot be opened, or is locked by another process even after the daemon
    closes its own instance and retries: -32602 `Failed to load the given account`.

In single-account mode:
- `account` is not consumed. Namespace-style methods ignore it.
- Methods with typed parameter objects reject it as an unknown property (-32600).
- Multi-account-only and registration methods are -32601 `Method not implemented`.

A method name that isn't a JSON-RPC command gives -32601 `Method not implemented`. This
includes unknown names and the CLI-only commands `daemon`, `jsonRpc` and `link`. Over HTTP
it also includes the four subscription methods (section 8).

## 7. Receiving and notifications on stream transports

`--receive-mode` controls whether a connection is subscribed automatically:
- `on-start` (default): message receiving starts when the daemon starts. Each new
  connection is **automatically subscribed** to every account, and to accounts added later.
- `on-connection`: receiving starts when the first listener attaches. Connections are still
  automatically subscribed, as with `on-start`.
- `manual`: connections are **not** subscribed automatically. The client calls
  `subscribeReceive`.

Notifications are JSON-RPC requests with no `id`. Two forms exist:
- From an **automatic** subscription:
  `{"jsonrpc":"2.0","method":"receive","params":<payload>}`.
  `<payload>` is the object `{"account": "<self number>", "envelope": {...}}`, plus
  `"exception": {...}` when decryption or handling failed.
- From an **explicit** `subscribeReceive` subscription:
  `{"jsonrpc":"2.0","method":"receive","params":{"subscription":<int>,"result":<payload>}}`.
- Call events go only to explicit `subscribeCallEvents` subscribers:
  `{"jsonrpc":"2.0","method":"callEvent","params":{"subscription":<int>,"result":{...}}}`.

Full shapes, including the exact key order of the payload object, are in `notifications.md`.

**Subscription ids**
- Ids are integers from **one process-wide counter**, starting at 0. They are not
  per-connection.
- Each automatic subscription uses up one id when the connection is set up, so on an
  `on-start` daemon the first connection's explicit subscription gets 1, the next
  connection's automatic subscription 2, and so on. Observed in `observations.md`.
- Receive and call-event subscriptions share the counter.

**Subscription methods**
- `subscribeReceive` returns the new id. In multi-account mode it covers all current
  accounts and accounts added later. With `account` it covers only that account. A second
  subscription on the same connection delivers each message once per subscription.
- `unsubscribeReceive`:
  - takes `{"subscription": <int>}` or `[<int>]`;
  - success: result `{}`;
  - unknown id: -1 `Unknown subscription id`. Only subscriptions of the same connection can
    be found;
  - no params: in multi-account mode -1 `Missing subscription parameter with subscription id`;
    in single-account mode it removes the automatic subscription;
  - the automatic subscription's id can also be passed explicitly.

  Details and the -32603 edge cases (e.g. `{}` with no `subscription` key) are in the
  method files.

## 8. HTTP transport (`--http`)

Implemented on the JDK built-in HTTP server, HTTP/1.1, keep-alive. There is no
authentication and no TLS. Requests are handled concurrently.

The server has three contexts. Each one matches by **path prefix**, and the handler then
requires an exact path:

| Path | Method | Purpose |
|---|---|---|
| `/api/v1/rpc` | POST | JSON-RPC (single or batch) |
| `/api/v1/events` | GET | Server-Sent Events stream of received messages |
| `/api/v1/check` | GET | liveness check |

**Paths outside the three contexts.** Examples are `/`, `/api/v1/` and `/api/v1/unknown`.
The JDK server answers `404 Not Found` with headers `Content-Type: text/html`,
`Content-Length: 50` and `Connection: close`, and body
`<h1>404 Not Found</h1>No context found for request`.

**Paths inside a context but not exact.** Examples are `/api/v1/rpc/`, `/api/v1/rpcx` and
`/api/v1/events/x`. The answer is `404` with an empty body (`Content-length: 0`). The
query string is ignored for path matching.

**Host header pinning** (only on `/api/v1/rpc` and `/api/v1/events`)
- If the bind address is the wildcard (`0.0.0.0` or `::`), any Host header is accepted.
- Otherwise the Host header's host part must be one of these, case-insensitive:
  - the bind host string as given (lower-cased), `localhost`, `127.0.0.1`, `::1`.

  IPv6 hosts are written in brackets: `[::1]:port`.
- If the Host header has a numeric port, it must equal the bound port.
- A failed check returns status **421** with an empty body. The status line has no reason
  phrase: `HTTP/1.1 421 `.
- A request with no Host header at all is rejected earlier by the JDK server: `400 Bad Request`,
  `Content-Type: text/html`, body `<h1>400 Bad Request</h1>Missing Host header`,
  `Connection: close`.

For `/api/v1/rpc` and `/api/v1/events` the checks run in this order: Host, then exact
path (404), then method (405). Other responses, such as 405, have an empty body and
`Content-length: 0`. They have no `Allow` header.

### 8.1 `POST /api/v1/rpc`

- **Method.** Anything other than `POST`, including GET, PUT and HEAD, gets `405`.
- **Content-Type.**
  - The `Content-Type` request header must exist and **start with** the lowercase string
    `application/json`, compared case-sensitively.
  - Accepted: `application/json`, `application/json; charset=utf-8`, `application/json-rpc`.
  - Rejected with `415`, empty body: a missing header, `text/plain`, `Application/JSON`.
- **Body.** The body is parsed as **one JSON value**, and any bytes after it are ignored.
  Newlines don't matter, and there is no line framing.
  - An empty or whitespace-only body gives -32600 `unexpected type: MISSING`. This differs
    from stream transports, which return -32700 `Empty input line`.
  - Other parsing rules are as in sections 3 to 6.
- **Response.**
  - If a JSON-RPC response exists (one object, or an array for a batch), the status is
    `200`, with header `Content-type: application/json`, an exact `Content-length`, and a
    compact JSON body with no trailing newline.
  - JSON-RPC errors are also returned with HTTP 200.
  - If nothing is to be returned, the status is **`201 Created`** with an empty body
    (`Content-length: 0`) and no Content-Type. This happens for:
    - a notification (no `id`);
    - a batch made only of notifications;
    - a client-sent response object.
  - If an unexpected exception escapes request processing, the status is `200` with body
    `{"jsonrpc":"2.0","error":{"code":-32603,"message":"An internal server error has occurred.","data":null},"id":null}`.
- **Account selection.** As in section 6.
- **No receive notifications.** HTTP never pushes receive notifications on this endpoint.
  The methods `subscribeReceive`, `unsubscribeReceive`, `subscribeCallEvents` and
  `unsubscribeCallEvents` **do not exist over HTTP** and return -32601
  `Method not implemented`. Use `/api/v1/events` instead. The `receive` method (manual
  polling) is available.

### 8.2 `GET /api/v1/events` (SSE)

- **Method.** Anything other than `GET`, including HEAD and POST, gets `405`.
- **Query parameter `account`.**
  - In multi-account mode, `?account=<number or ACI>` restricts the stream to that
    account. The value is URL-decoded, and a literal `+` in the raw query is kept as `+`
    (not turned into a space), so both `?account=+4912345` and `?account=%2B4912345` work.
  - If the parameter is absent or empty, the stream covers all accounts that exist **at
    connect time**. Accounts added later are not included.
  - An unknown account gives `400` with an empty body.
  - In single-account mode the parameter is ignored.
- **Success response.**
  - Status `200`, `Content-type: text/event-stream`, `Transfer-encoding: chunked`. The
    headers are flushed immediately.
  - There is no initial event. The stream stays open until the client disconnects or the
    daemon shuts down.
- **Event format.** Each received message produces:

      event:receive\n
      data:<payload JSON on one line>\n
      \n

  - There is no `id:` field and no space after the colons.
  - `<payload>` is the same `{"account":..., "envelope":..., "exception"?:...}` object as
    in the automatic-subscription notification `params`, with no JSON-RPC wrapper.
- **Keep-alive.** About every 15 s with no other traffic, the comment line `:\n` (colon,
  LF) is sent. Observed at about 15 s after connect.
- **Disconnect.** If a write fails, the subscription is removed and the stream closes.
- **Errors.** An unexpected server error before streaming starts gives status `500` with
  an empty body.

### 8.3 `GET /api/v1/check`

- `GET`: `200` with an empty body (`Content-length: 0`). The query string is ignored.
- Any other method gets `405`.
- A non-exact path gets `404`.
- **There is no Host-header check on this endpoint.**

## 9. Shutdown

On SIGTERM or SIGINT the daemon:
1. logs `INFO  Shutdown - Received SIGTERM signal, shutting down ...`;
2. closes the HTTP server, with up to 2 s grace for in-flight exchanges, which ends SSE
   streams;
3. closes the listening sockets and client connections;
4. removes the UNIX socket file.

In single-account mode the daemon also shuts down when the account's manager closes, for
example after the account is unregistered.
