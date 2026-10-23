# Method: startLink

## Sources

- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/commands/StartLinkCommand.java (tag v0.14.7)
- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/commands/JsonRpcCommand.java (no request type -> params ignored)
- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/jsonrpc/SignalJsonRpcCommandHandler.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/lib/src/main/java/org/asamk/signal/manager/internal/MultiAccountManagerImpl.java (pending-link bookkeeping)
- https://github.com/AsamK/signal-cli/blob/v0.14.7/lib/src/main/java/org/asamk/signal/manager/api/DeviceLinkUrl.java (URI format)
- https://github.com/AsamK/signal-cli/blob/v0.14.7/man/signal-cli-jsonrpc.5.adoc (section "startLink")
- Not observed on the binary (opens a provisioning connection to Signal servers). Source reading only.

## Summary

First step of linking this signal-cli instance as a new secondary device of an existing Signal
account. Opens a provisioning session with the server and returns the device-link URI that the
primary device must scan (QR code) or that another signal-cli passes to `addDevice`.

## CLI mapping

No direct CLI equivalent; the CLI `link` command performs startLink + finishLink in one go
(`link` itself is **not** a JSON-RPC method: -32601).

## Account requirement

Class **multi**: multi-account mode only; single-account mode -> -32601 `Method not implemented`.
Works with zero accounts.

## Params

None. The method declares no parameter type, so `params` is **completely ignored**: it may be
omitted, `null`, any object (including one containing `account`) or even an array.

## Result

| key | type | nullable | notes |
|---|---|---|---|
| `deviceLinkUri` | string | no | `sgnl://linkdevice?uuid=<url-encoded provisioning address>&pub_key=<url-encoded base64 of the 33-byte provisioning public key, padding '=' stripped>` |

Example (man page): `{"jsonrpc":"2.0","result":{"deviceLinkUri":"sgnl://linkdevice?uuid=X&pub_key=X"},"id":"5"}`.

The daemon remembers the pending provisioning session keyed by the exact URI string; it is
consumed by `finishLink` (each URI can be finished at most once). Multiple pending sessions may
coexist.

## Errors

| code | message | trigger |
|---|---|---|
| -32601 | `Method not implemented` | single-account mode |
| -1 | `Device link creation timed out, please try again.` | provisioning address not received in time |
| -3 | `Link request error: <io message>` | network failure |
| -32603 | `<message> (<exception simple name>)` | anything else |
