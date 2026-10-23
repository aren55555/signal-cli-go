# Method `receive`

## Sources

- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/commands/ReceiveCommand.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/jsonrpc/SignalJsonRpcCommandHandler.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/json/JsonReceiveMessageHandler.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/lib/src/main/java/org/asamk/signal/manager/internal/ManagerImpl.java (already-receiving rule)
- https://github.com/AsamK/signal-cli/blob/v0.14.7/man/signal-cli.1.adoc (CLI `receive`)
- Binary observation: none (needs a registered account). Jackson binding behavior stated below follows Jackson 2.x
  defaults for a plain ObjectMapper (jackson-databind 2.22.1 is the bundled version); items marked "unverified" were not
  observed.

## Summary

Synchronously fetches pending messages for one account and returns them as an array. Equivalent of CLI
`signal-cli -a ACCOUNT -o json receive`.

## Account requirement

**single** (see `index.md`). In multi-account mode give `account` unless exactly one account is loaded.

## Params

Unlike namespace methods, params are bound strictly to a fixed set of keys (camelCase only).

| JSON key | type | required | default | CLI flag | notes |
|---|---|---|---|---|---|
| `timeout` | number (seconds, fractional allowed) | no | `3.0` | `-t`, `--timeout` | Negative = no timeout (block until `maxMessages` reached or forever). Converted to milliseconds by truncation. `null` = default |
| `maxMessages` | integer | no | `-1` | `--max-messages` | Negative = unlimited. `null` = default |

- Kebab keys (`max-messages`) are **not** accepted.
- **Unknown keys are rejected**: any other key (e.g. `ignoreAttachments`, `max-messages`) fails with -32600 and a
  Jackson "Unrecognized field \"<key>\" (class ...), not marked as ignorable (2 known properties: \"timeout\",
  \"maxMessages\"])" message (followed by Jackson's location/reference-chain text).
  - Consequence: in **single-account** daemon mode, sending `account` in `receive` params fails this way (the key is
    only stripped in multi-account mode).
- The CLI flags `--ignore-attachments`, `--ignore-stories`, `--ignore-avatars`, `--ignore-stickers`,
  `--send-read-receipts` are **not** available via JSON-RPC; the daemon's own settings apply.
- `params` as an array fails with -32600 (cannot bind array to the params object).
- Type coercion follows Jackson defaults (unverified): numeric strings such as `"5"` are accepted for both keys;
  a fractional `maxMessages` is truncated; booleans/objects fail with -32600.

## Result

JSON array (possibly empty `[]`) of message wrapper objects, in reception order, exactly as specified in
`../notifications.md` section 1 (`{"exception"?, "envelope", "account"}`). The method returns after the timeout elapses
with no new message, or after `maxMessages` items.

## Errors

| code | message | when |
|---|---|---|
| -1 | `Receive command cannot be used if messages are already being received.` | The account currently has any continuous receiver: daemon started with default `--receive-mode on-start`; any socket/TCP connection with its implicit subscription (on-connection mode) or an explicit `subscribeReceive`; an open HTTP SSE stream; or another `receive` call in progress |
| -3 | `Error while receiving messages: <detail>` | Network/IO failure while receiving |
| -32600 | Jackson binding message | unknown key, wrong type, array params |
| -32602 | `Method requires valid account parameter` / `Specified account does not exist` | multi-account account selection |
| -32603 | `<message> (<ExceptionKind>)` | unexpected failure |

So in practice `receive` is only usable with a daemon in `--receive-mode manual` (or `on-connection` with no
connected socket clients / SSE streams), or with `jsonRpc --receive-mode manual`.

## Notes

- During a `receive` call, other (weak) listeners such as the daemon's stdout printer still see the messages.
- Over HTTP the POST blocks for the whole receive duration.
