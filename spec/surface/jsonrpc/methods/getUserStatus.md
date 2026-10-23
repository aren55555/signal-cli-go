# Method `getUserStatus`

## Sources

- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/commands/GetUserStatusCommand.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/util/CommandUtil.java (rate limit message), util/DateUtils.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/lib/src/main/java/org/asamk/signal/manager/internal/ManagerImpl.java (number canonicalisation, username lookup)
- https://github.com/AsamK/signal-cli/blob/v0.14.7/lib/src/main/java/org/asamk/signal/manager/api/UserStatus.java, UsernameStatus.java
- Binary observation: none. Conventions: [index.md](index.md#parameter-conventions).

## Summary

Checks whether phone numbers and/or usernames are registered Signal users
(via contact discovery / username lookup on the server).

CLI equivalent: `signal-cli -a ACCOUNT -o json getUserStatus [NUMBER ...] [--username U ...]`.

Account requirement: **single**.

## Params

| JSON key(s) | Type | Req. | Default | CLI flag | Notes |
|---|---|---|---|---|---|
| `recipient` / `recipients` | list of string | no | none | positional | Phone numbers. Each is normalised to E.164 relative to the account number; invalid numbers are not an error (they simply report as unregistered with `number` null). Duplicates collapse. |
| `username` / `usernames` | list of string | no | none | `--username` | Usernames or username links. Unknown usernames report as unregistered. |

Both may be omitted (result `[]`, though a discovery request with an empty set
may still be issued).

## Result

JSON array; first one entry per distinct input number, then one entry per
distinct input username (order within each part unspecified):

| Key | Type | Notes |
|---|---|---|
| `recipient` | string | the input string exactly as given |
| `number` | string | number entries only: normalised E.164; **omitted** for username entries and when the input was not a valid number |
| `username` | string | username entries only (the input username); **omitted** otherwise |
| `uuid` | string \| null | ACI if registered, else null (always present) |
| `isRegistered` | boolean | `uuid != null` |

## Errors

| Code | Message | Trigger |
|---|---|---|
| -5 | `Rate limit reached` optionally followed by `\nNext attempt may be tried at <ms> (<yyyy-MM-dd'T'HH:mm:ss.SSSX in UTC>)` | discovery service rate limit / resource exhausted |
| -3 | `Unable to check if users are registered: <msg> (<ExceptionClass>)` | I/O error in number or username lookup |
| -32603 | `<msg> (<ExceptionClass>)` | non-array under plural key; non-string list elements |
