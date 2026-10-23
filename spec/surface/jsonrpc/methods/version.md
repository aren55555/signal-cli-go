# Method: version

## Sources

- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/commands/VersionCommand.java (tag v0.14.7)
- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/jsonrpc/SignalJsonRpcCommandHandler.java (account selection, error mapping)
- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/commands/JsonRpcLocalCommand.java, JsonRpcMultiLocalCommand.java
- Binary observation: zero-account multi-account daemon
  (`signal-cli --config <CONFIG> daemon --http 127.0.0.1:<PORT> ...`), requests sent with
  `curl -s -i -X POST -H 'Content-Type: application/json' --data-binary BODY http://127.0.0.1:<PORT>/api/v1/rpc`.
  Transcripts: `../observations.md` (fork C section; ids 1-8, 14).

## Summary

Returns the signal-cli version. Needs no account and never contacts the network.

## CLI mapping

`signal-cli version` (plain text prints `signal-cli 0.14.7`; `-o json` prints the same object as the JSON-RPC result).

## Account requirement

Class **single+multi** (see `index.md`):

- Single-account daemon: always runs.
- Multi-account daemon, no `account` (absent or JSON `null`): runs in the global context; works with zero accounts.
- Multi-account daemon with a non-null `account`: the account must exist (be loadable),
  otherwise error -32602 `Specified account does not exist` (observed with zero accounts, both for a phone
  number and for a UUID string). When it exists the result is identical.

## Params

None. `params` may be omitted, `null`, or any JSON object; unknown keys are ignored (observed `{"foo":1}` succeeds).
A JSON array as `params` is rejected (namespace method rule 1 in `index.md`).

## Result

A JSON object with exactly one key:

| key | type | nullable | notes |
|---|---|---|---|
| `version` | string | no | `"0.14.7"` for this release (falls back to `"unknown"` only if the build carries no version) |

Observed: `{"jsonrpc":"2.0","result":{"version":"0.14.7"},"id":1}`.

## Errors

| code | message | trigger |
|---|---|---|
| -32600 | `` Cannot deserialize value of type `java.util.LinkedHashMap<java.lang.String,java.lang.Object>` from Array value (token `JsonToken.START_ARRAY`)\n at [Source: UNKNOWN; byte offset: #UNKNOWN] `` (the `\n` is a newline character in the JSON string) | `params` is a JSON array (observed, `data` null) |
| -32600 | Jackson message beginning `Problem deserializing property 'params'` | `params` is a JSON scalar such as a string (observed; generic request-level error, see `../errors.md`) |
| -32602 | `Specified account does not exist` | multi-account mode, non-null `account` naming no loadable account (observed) |
