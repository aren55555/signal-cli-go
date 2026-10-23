# Method `sendSyncRequest`

## Sources

- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/commands/SendSyncRequestCommand.java
- Man page https://github.com/AsamK/signal-cli/blob/v0.14.7/man/signal-cli-jsonrpc.5.adoc (example `{"jsonrpc":"2.0","method":"sendSyncRequest","id":9}` -> `{"jsonrpc":"2.0","result":{},"id":9}`)
- Binary observation: none. Conventions: [index.md](index.md).

## Summary

From a linked (secondary) device, asks the primary device to send all sync data
(groups, contacts, blocked list, configuration, keys). CLI:
`signal-cli -a ACCOUNT sendSyncRequest`.

Account requirement: **single**.

## Parameters

None (all keys ignored; `params` must be an object or absent, index.md rule 1).

## Result

Empty object `{}`.

## Errors

| Code | Message | Trigger |
|---|---|---|
| -3 | `Request sync data error: <reason>` | I/O error while sending the request. |
