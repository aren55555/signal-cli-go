# Method `updateContact`

## Sources

- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/commands/UpdateContactCommand.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/util/CommandUtil.java
- Binary observation: none. Conventions: [index.md](index.md#parameter-conventions).

## Summary

Sets local contact details (system name, nickname, note) and/or the
disappearing-message timer for one recipient.

CLI equivalent: `signal-cli -a ACCOUNT updateContact RECIPIENT [...]`.

Account requirement: **single**.

## Params

| JSON key(s) | Type | Req. | Default | CLI flag | Notes |
|---|---|---|---|---|---|
| `recipient` | string | yes | — | positional `recipient` | Single recipient (index.md rule 10). Not a list (a list would be stringified and rejected as an invalid number). Missing -> -32603. |
| `name` | string | no | — | `-n/--name` | Used as given name only when `given-name` is absent; then family name defaults to `""` if `family-name` is also absent. |
| `given-name` / `givenName` | string | no | — | `--given-name` | |
| `family-name` / `familyName` | string | no | — | `--family-name` | |
| `nick-given-name` / `nickGivenName` | string | no | — | `--nick-given-name` | |
| `nick-family-name` / `nickFamilyName` | string | no | — | `--nick-family-name` | |
| `note` | string | no | — | `--note` | |
| `expiration` | integer (32-bit) | no | unchanged | `-e/--expiration` | Seconds. If present, the timer is set first (this sends an expiration-update message). |

The name/nickname/note update is always performed (with null meaning "no
change" for each field as interpreted by the account store).

## Result

`{}`.

## Errors

| Code | Message | Trigger |
|---|---|---|
| -1 | `Invalid phone number '<input>': <reason>` | bad recipient |
| -1 | `The user <identifier> is not registered.` | recipient not a Signal user |
| -3 | `Update contact error: <msg>` | I/O error (e.g. sending expiration update) |
| -32603 | `<msg> (<ExceptionClass>)` | missing recipient, non-integer `expiration` |
