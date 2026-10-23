# signal-cli 0.14.7 JSON-RPC: errors

## Sources

- Upstream signal-cli, tag v0.14.7 (https://github.com/AsamK/signal-cli/tree/v0.14.7):
  - `src/main/java/org/asamk/signal/jsonrpc/JsonRpcReader.java`, `JsonRpcResponse.java`,
    `SignalJsonRpcCommandHandler.java`, `SignalJsonRpcDispatcherHandler.java`
  - `src/main/java/org/asamk/signal/http/HttpServerHandler.java`
  - `src/main/java/org/asamk/signal/commands/exceptions/*.java`
  - `man/signal-cli-jsonrpc.5.adoc`
- Binary observation: every error string quoted below as "observed" was received from the
  zero-account 0.14.7 daemon. Exact requests are in `observations.md`.

## 1. Error object shape

Every error response has this shape:

    {"jsonrpc":"2.0","error":{"code":<int>,"message":<string>,"data":<null|object>},"id":<id or null>}

- **Key order is fixed.** The response keys are `jsonrpc`, then `error` (or `result`),
  then `id`. The error keys are `code`, `message`, `data`.
- **`data` is always present.** It is JSON `null` unless the method had already produced a
  result value before failing. In that case `data` is `{"response": <that result value>}`.
  For example, a send-type method whose sends all failed returns error -4, -5 or -1 with
  `data.response` holding the full send result object; see `methods/send.md`.
- **`id`.**
  - It echoes the request's `id` when the request object could be read far enough to find
    it. The id must be a string, number or null; an array or object id is not echoed.
  - It is `null` for parse errors, non-object input, an empty batch, and request objects
    whose `id` member itself was invalid.
- **Success responses** are `{"jsonrpc":"2.0","result":<value>,"id":<id>}`. `result` is
  never omitted: a method with no output returns `{}`.

## 2. Code table

| code | name | message (exact or pattern) | trigger |
|---|---|---|---|
| -32700 | Parse error | `Empty input line` | stream transports: a line that is empty or only whitespace |
| -32700 | Parse error | the JSON parser's message, e.g. `Unrecognized token 'not': was expecting (JSON String, Number, Array, Object or token 'null', 'true' or 'false')\n at [Source: REDACTED (`StreamReadFeature.INCLUDE_SOURCE_IN_LOCATION` disabled); line: 1, column: 1]` (observed) | input not parseable as JSON. Another observed example: `Unexpected end-of-input within/between Object entries\n at [Source: REDACTED (...); line: 1, column: 18]` |
| -32600 | Invalid Request | `invalid request` | empty batch `[]` (observed) |
| -32600 | Invalid Request | `unexpected type: <TYPE>` with TYPE one of `NULL`, `NUMBER`, `STRING`, `BOOLEAN`, `MISSING` | top-level JSON value is not an object or array (observed). `MISSING` happens only over HTTP, for an empty or whitespace body |
| -32600 | Invalid Request | `only jsonrpc version 2.0 is supported` | `jsonrpc` missing or not "2.0" (observed; also for `{}`) |
| -32600 | Invalid Request | `method field must be set` | `method` missing or null (observed) |
| -32600 | Invalid Request | deserializer message: `Unrecognized field "<name>" (class org.asamk.signal.jsonrpc.JsonRpcRequest), not marked as ignorable (4 known properties: "method", "id", "jsonrpc", "params")\n at [Source: UNKNOWN; byte offset: #UNKNOWN] (through reference chain: org.asamk.signal.jsonrpc.JsonRpcRequest["<name>"])` | an extra top-level member in a request (observed). In a batch, an element with `result`/`error` also hits this |
| -32600 | Invalid Request | deserializer message beginning `Problem deserializing property 'id' ...` / `Problem deserializing property 'params' ...` | `id` that is an array or object; `params` that is a scalar (observed) |
| -32600 | Invalid Request | deserializer message beginning `Cannot construct instance of `org.asamk.signal.jsonrpc.JsonRpcRequest`` or `Cannot deserialize value of type `org.asamk.signal.jsonrpc.JsonRpcRequest` from Array value` | a batch element that is not an object (observed) |
| -32600 | Invalid Request | deserializer message, e.g. ``Cannot deserialize value of type `java.util.LinkedHashMap<java.lang.String,java.lang.Object>` from Array value (token `JsonToken.START_ARRAY`)\n at [Source: UNKNOWN; byte offset: #UNKNOWN]`` | `params` cannot be bound to what the method expects, e.g. an array for a method that takes an object (observed with `version`), wrong types or unknown keys for the typed-param methods (`receive`, `register`, `verify`, `finishLink`). See the method files |
| -32601 | Method not found | `Method not implemented` | unknown method name; a CLI-only command (`daemon`, `jsonRpc`, `link`); a multi-account-only or registration method in single-account mode; the subscription methods over HTTP (observed) |
| -32602 | Invalid params | `Method requires valid account parameter` | multi-account mode, single-account method, `account` absent or null, and the number of loaded accounts is not exactly 1 (observed) |
| -32602 | Invalid params | `Specified account does not exist` | multi-account mode, non-null `account` that isn't a loaded account (observed) |
| -32602 | Invalid params | `Method requires account parameter` | registration method (`register`, `verify`, `deleteLocalAccountData`) with no `account` key |
| -32602 | Invalid params | `Failed to load the given account` | registration method whose account cannot be opened or locked |
| -32603 | Internal error | `<exception message> (<ExceptionSimpleName>)` | any unexpected exception inside a method: ClassCastException from a mistyped param, NullPointerException from a missing param, IllegalArgumentException from bad enum or base64 input. Observed: `Cannot invoke "com.fasterxml.jackson.databind.JsonNode.asInt()" because the return value of "com.fasterxml.jackson.databind.node.ObjectNode.get(String)" is null (NullPointerException)` for `unsubscribeReceive` with `{}` |
| -32603 | Internal error | `<message> (UnexpectedErrorException)` | a command reported an unexpected failure. The message is usually already of the form `Failed to ...: <reason> (<Cause>)`, so it ends with two parenthesised names, e.g. `Failed to send message: <reason> (IOException) (UnexpectedErrorException)` |
| -32603 | Internal error | `An internal server error has occurred.` | HTTP only: an exception escaped request processing entirely (`id` null, HTTP 200) |
| -1 | User error | per method, e.g. `Unknown subscription id`, `Missing subscription parameter with subscription id` (observed), `No recipients given`, `Invalid phone number '<x>': <reason>`, `Invalid group id: ...`, `Failed to send message` | invalid user input or a state problem reported by the command |
| -3 | I/O error | per method, e.g. `Failed to send message: ...`, `... (network)` | network or storage I/O failure reported by the command |
| -4 | Untrusted key | e.g. `Failed to send message due to untrusted identities` | all sends failed because of identity-key mismatch |
| -5 | Rate limit | e.g. `Failed to send message due to rate limiting`; registration flows use `Rate limit reached` plus an optional `\nNext attempt may be tried at <time>` | the server rate-limited the request |
| -6 | Captcha rejected | per method (registration / rate-limit challenge) | the server rejected the captcha |

The CLI uses exit code 2 for "unexpected error", but **code -2 is never produced over
JSON-RPC**: unexpected errors become -32603. Codes -1, -3, -4, -5 and -6 match the CLI exit
codes 1, 3, 4, 5 and 6 with the sign flipped.

Method-specific -1/-3/-4/-5/-6 messages are listed in each `methods/<method>.md`.

## 3. When no error is sent

- **Notification requests** (no `id` member): any error is discarded, and so is any success.
- **Response objects sent by the client** (top-level object with `result` or `error`): they
  are ignored with no output. This includes malformed ones.
- **Over HTTP**, when nothing is returned the status is 201 with an empty body.

## 4. Edge-case matrix (observed)

| Input (one line / HTTP body) | Output |
|---|---|
| `not json` | -32700 parser message, id null |
| empty line (stream) | -32700 `Empty input line`, id null |
| empty body (HTTP) | -32600 `unexpected type: MISSING`, id null |
| `[]` | -32600 `invalid request`, id null (single object, not an array) |
| `42` / `"str"` / `null` | -32600 `unexpected type: NUMBER` / `STRING` / `NULL` |
| `{}` | -32600 `only jsonrpc version 2.0 is supported`, id null |
| `{"jsonrpc":"2.0","id":5}` | -32600 `method field must be set`, id 5 |
| `{"method":"version","id":7}` | -32600 `only jsonrpc version 2.0 is supported`, id 7 |
| `{"jsonrpc":2.0,"method":"version","id":14}` | success (number coerced to "2.0") |
| `{"jsonrpc":"2.0","method":123,"id":12}` | -32601 `Method not implemented`, id 12 |
| `{"jsonrpc":"2.0","method":"version","id":null}` | success with `"id":null` |
| `{"jsonrpc":"2.0","method":"version","id":1.5}` | success with `"id":1.5` |
| `{"jsonrpc":"2.0","method":"version","id":[1]}` | -32600 Problem deserializing property 'id'..., id null |
| `{"jsonrpc":"2.0","method":"version","params":null,"id":8}` | success |
| `{"jsonrpc":"2.0","method":"version","params":[],"id":9}` | -32600 Cannot deserialize ... LinkedHashMap ... from Array value, id 9 |
| `{"jsonrpc":"2.0","method":"version","params":"x","id":10}` | -32600 Problem deserializing property 'params'..., id 10 |
| `{"jsonrpc":"2.0","method":"version","params":{"foo":1},"id":11}` | success (unknown param keys ignored) |
| `{"jsonrpc":"2.0","method":"version","id":13,"extra":true}` | -32600 Unrecognized field "extra" ..., id 13 |
| `{...valid id 15...} trailing` | success for id 15; trailing text ignored |
| `{"jsonrpc":"2.0","result":1,"id":1}` | nothing (HTTP 201) |
| `[1,2]` | array of two -32600 errors, id null |
| `[valid id1, "x", jsonrpc 1.0 id4, unknown-method id5]` | array: error for "x" (id null), error for id4, then results for id1 and id5 in completion order |
| `[{"jsonrpc":"2.0","result":1,"id":1}]` | array with -32600 Unrecognized field "result", id 1 |
| `[{"jsonrpc":"2.0","method":"version"}]` | nothing (HTTP 201) |
| `send` with no account, zero accounts | -32602 `Method requires valid account parameter` |
| `send` with `"account":"+15550000000"` (unknown) | -32602 `Specified account does not exist` |
| `send` with `"account":null` | -32602 `Method requires valid account parameter` |
| `version` with unknown `account` | -32602 `Specified account does not exist` |
| `listAccounts` with unknown `account` | success `[]` (account ignored) |
| `subscribeReceive` over HTTP | -32601 `Method not implemented` |
| `unsubscribeReceive` (stream, multi mode) without params | -1 `Missing subscription parameter with subscription id` |
| `unsubscribeReceive` `{"subscription":999}` | -1 `Unknown subscription id` |
| `unsubscribeReceive` `{}` | -32603 NullPointerException message (see table) |
| `unsubscribeCallEvents` `{"subscription":"abc"}` | treated as id 0, so -1 `Unknown subscription id` unless 0 is a call-event subscription on this connection |
