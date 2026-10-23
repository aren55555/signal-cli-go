# Method `removeContact`

## Sources

- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/commands/RemoveContactCommand.java
- Binary observation: none. Conventions: [index.md](index.md#parameter-conventions).

## Summary

Removes a contact's details, hides it, or forgets all data about it.

CLI equivalent: `signal-cli -a ACCOUNT removeContact RECIPIENT [--hide | --forget]`.

Account requirement: **single**.

## Params

| JSON key(s) | Type | Req. | Default | CLI flag | Notes |
|---|---|---|---|---|---|
| `recipient` | string | yes | — | positional | Single recipient. Missing -> -32603. |
| `hide` | boolean | no | false | `--hide` | Hide in contact list, keep data. |
| `forget` | boolean | no | false | `--forget` | Delete all data incl. identity keys and sessions. |

The CLI makes `--hide`/`--forget` mutually exclusive; JSON-RPC does not check:
if both are true, `hide` wins. If neither, the contact entry (names etc.) is
deleted.

Type quirk: these flags are compared by identity with the boolean `true`;
non-boolean JSON values cause -32603 (ClassCastException).

## Result

`{}`.

## Errors

| Code | Message | Trigger |
|---|---|---|
| -1 | `Invalid phone number '<input>': <reason>` | bad recipient |
| -32603 | `<msg> (<ExceptionClass>)` | missing recipient, wrong types, unexpected failures |
