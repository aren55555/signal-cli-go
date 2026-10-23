# Method: listAccounts

## Sources

- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/commands/ListAccountsCommand.java (tag v0.14.7)
- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/commands/JsonRpcMultiLocalCommand.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/jsonrpc/SignalJsonRpcCommandHandler.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/lib/src/main/java/org/asamk/signal/manager/MultiAccountManager.java and
  https://github.com/AsamK/signal-cli/blob/v0.14.7/lib/src/main/java/org/asamk/signal/manager/internal/MultiAccountManagerImpl.java (which accounts are listed)
- Binary observation: zero-account multi-account daemon over HTTP (see `version.md` for the command line);
  transcripts in `../observations.md` (fork C section; ids 10-13).

## Summary

Lists the phone numbers of the accounts currently loaded by a multi-account daemon.

## CLI mapping

`signal-cli listAccounts` (plain text: one `Number: +NNN` line per account; json: same array as the RPC result).

## Account requirement

Class **multi**: exists only in multi-account mode (daemon / jsonRpc started without `-a`). In single-account
mode the method is unknown: -32601 `Method not implemented`.

The `account` key is **not** consumed for this method; it is just an ignored unknown key (observed:
`{"account":"+15550000000"}` with zero accounts still returns `[]`, no error).

## Params

None. `params` may be omitted, `null` or any object (unknown keys ignored). A JSON array is rejected (-32600).

## Result

JSON array, one element per loaded account, element shape:

| key | type | nullable | notes |
|---|---|---|---|
| `number` | string | practically no | the account's E.164 phone number |

- Zero accounts: `[]` (observed).
- The set is the accounts loaded at daemon start (registered local accounts) plus accounts added later in the
  same process (via `register`+`verify`, `finishLink`, or loaded on demand when a request named them via
  `account`); accounts whose manager was closed (e.g. after unregistering) disappear.
- Order is unspecified (an unordered set internally); clients must not depend on it.

## Errors

| code | message | trigger |
|---|---|---|
| -32600 | `` Cannot deserialize value of type `java.util.LinkedHashMap<java.lang.String,java.lang.Object>` from Array value (token `JsonToken.START_ARRAY`)\n at [Source: UNKNOWN; byte offset: #UNKNOWN] `` (the `\n` is a newline character in the JSON string) | `params` is an array (observed for `[]` and `[1,2]`) |
| -32601 | `Method not implemented` | single-account mode |
