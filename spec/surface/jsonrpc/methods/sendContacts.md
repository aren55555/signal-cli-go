# Method `sendContacts`

## Sources

- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/commands/SendContactsCommand.java
- Binary observation: none. Conventions: [index.md](index.md).

## Summary

Sends a contacts synchronization message with the local contact list to all
linked devices. CLI: `signal-cli -a ACCOUNT sendContacts`.

Account requirement: **single**.

## Parameters

None (all keys ignored; `params` must still be an object or absent, index.md rule 1).

## Result

Empty object `{}`.

## Errors

| Code | Message | Trigger |
|---|---|---|
| -3 | `SendContacts error: <reason>` | I/O error while building/sending the sync message. |
