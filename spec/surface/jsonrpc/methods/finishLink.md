# Method: finishLink

## Sources

- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/commands/FinishLinkCommand.java (tag v0.14.7)
- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/jsonrpc/SignalJsonRpcCommandHandler.java (multi command dispatch; `account` is not stripped for multi-only methods; Jackson errors -> -32600)
- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/util/Util.java (object mapper configuration: default Jackson settings, unknown properties fail)
- https://github.com/AsamK/signal-cli/blob/v0.14.7/lib/src/main/java/org/asamk/signal/manager/internal/MultiAccountManagerImpl.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/lib/src/main/java/org/asamk/signal/manager/api/UserAlreadyExistsException.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/man/signal-cli-jsonrpc.5.adoc (section "finishLink"; its RESPONSE example is wrong, see Result)
- Not observed on the binary. Source reading only.

## Summary

Second step of linking: waits for the primary device to approve the link started by `startLink`,
then creates the new local account (as a linked device) and adds it to the daemon. Blocks until
the primary responds or the provisioning wait times out.

## CLI mapping

Second half of the CLI `link [-n NAME]` command.

## Account requirement

Class **multi** (multi-account mode only; single-account -> -32601). Important quirk: because
this method is multi-only, the daemon does **not** strip an `account` key from its params; an
`account` key is therefore an unknown property and fails with -32600 (see Params).

## Params

Typed (strict) object; exact key names only; unknown keys (including `account`) -> -32600 with a
Jackson `Unrecognized field "<name>" (class ...), not marked as ignorable (2 known properties:
"deviceLinkUri", "deviceName"])` message plus location/reference-chain suffix.

| JSON key | type | required | default | CLI | notes |
|---|---|---|---|---|---|
| `deviceLinkUri` | string | yes | none | (from link flow) | must be byte-for-byte the URI returned by a previous `startLink` on this daemon |
| `deviceName` | string | no | `"cli"` | `-n`, `--name` of `link` | name of the new linked device as shown on the primary |

If `params` is omitted or `null`, the parameter object is absent and the method fails with
-32603 (a NullPointerException-derived message ending in ` (NullPointerException)`); clients
should always send an object.

## Result

| key | type | nullable | notes |
|---|---|---|---|
| `number` | string | no | phone number (E.164) of the account that was linked |

e.g. `{"jsonrpc":"2.0","result":{"number":"+12025550100"},"id":"6"}`.
(The man page example showing `{"deviceLinkUri":...}` as the result is incorrect for 0.14.7.)

After success the new account is part of the daemon's account set (shows in `listAccounts`;
multi-account receive subscriptions and SSE streams opened without `account` pick it up).

## Errors

| code | message | trigger |
|---|---|---|
| -32601 | `Method not implemented` | single-account mode |
| -32600 | Jackson message | unknown key (incl. `account`) or non-coercible type |
| -1 | `Missing deviceLinkUri.` | `deviceLinkUri` absent or null |
| -1 | `Invalid device link uri.` | string is not a syntactically valid URI |
| -1 | `Unknown device link uri.` | no pending `startLink` session with exactly this URI (never started, already finished, or different daemon) |
| -1 | `Link request timed out, please try again.` | primary did not answer in time |
| -1 | `The user <number> already exists` + `\n` + `Delete "<path to account file>" before trying again.` | a local account for that number already exists |
| -3 | `Link request error: <io message> (<exception simple name>)` | I/O failure |
| -32603 | `<message> (<exception simple name>)` | anything else, e.g. params omitted |
