# signal-cli getUserStatus

## Sources

- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/commands/GetUserStatusCommand.java (tag v0.14.7)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/util/CommandUtil.java (rate-limit message)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/lib/src/main/java/org/asamk/signal/manager/internal/ManagerImpl.java (status lookup)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/lib/src/main/java/org/asamk/signal/manager/api/UserStatus.java
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/lib/src/main/java/org/asamk/signal/manager/api/UsernameStatus.java
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/man/signal-cli.1.adoc section "getUserStatus"
- Binary observation (signal-cli 0.14.7, empty scratch `--config <CONFIG>`), commands run:
  - `signal-cli --config <CONFIG> getUserStatus --help`
  - `signal-cli --config <CONFIG> getUserStatus` (exit 1, `No local users found, you first need to register or link an account`)
  - `signal-cli --config <CONFIG> getUserStatus --username` (exit 1, same; zero usernames is accepted by the parser)
  - `signal-cli --config <CONFIG> -a +15555550123 getUserStatus +15555550199` (exit 1, `User +15555550123 is not registered.`)
  - `signal-cli --config <CONFIG> -a +15555550123 getUserStatus --username u.01` (exit 1, same)

## Summary

Help line: `Check if the specified phone number/s have been registered`

Looks up whether phone numbers (via contact discovery) and/or usernames are registered
Signal users.

## Invocation category & account requirement

Local single-account command; requires a registered local account (common account errors,
see index.md). Contacts the server (contact discovery service, username lookup, profiles).

## Output modes

plain-text (default) and json.

## Arguments

| names | arity | value type / metavar | default | required | help text (verbatim) | notes |
|---|---|---|---|---|---|---|
| `recipient` (positional) | 0..n (`*`) | string (phone number), RECIPIENT | empty | no | `Phone number` | |
| `--username` | 0..n (`*`) | string, USERNAME | none | no | `Specify the recipient username or username link.` | username (e.g. `name.01`) or username link |

Neither is required; with no inputs the command succeeds with empty output (`[]` in json).
Duplicate inputs are collapsed (each distinct input string reported once).

## Constraints & validation

- Phone numbers are canonicalized to E.164 using the local account's number for country
  context (e.g. national-format input). Inputs that cannot be parsed as a phone number are not
  an error: they are reported as not registered (and, in JSON, without a `number` key).
- Usernames that cannot be resolved are reported as not registered (no error).

## Behavior

1. All canonicalized phone numbers are looked up in one contact-discovery request. For
   registered ones, the cached/fetched profile determines whether the user allows
   unrestricted sealed-sender access.
2. Each username (or username link) is resolved individually.
3. Output: first all phone-number results, then all username results. Order within each
   group is unspecified (hash order) — a reimplementation may choose any order, tests should
   compare as sets.

## Plain-text output

One line per input:

`<input as given>: <true|false>` optionally followed by ` (unrestricted sealed sender)` when
the user is registered and has unrestricted unidentified access enabled.

`true` means registered (a service UUID was found).

## JSON output

One line: JSON array; one object per input, keys in this order:

| key | type | presence | meaning |
|---|---|---|---|
| `recipient` | string | always | the input string as given (number or username) |
| `number` | string | phone-number entries only, and only if canonicalization succeeded (key omitted otherwise) | canonical E.164 number |
| `username` | string | username entries only (omitted for numbers) | the username as given |
| `uuid` | string or null | always (null when not registered) | ACI UUID |
| `isRegistered` | boolean | always | true iff `uuid` is non-null |

The "unrestricted sealed sender" flag is NOT included in JSON.

Example shape: `[{"recipient":"+15555550199","number":"+15555550199","uuid":null,"isRegistered":false}]`

## Errors

| condition | stream | exact message | exit |
|---|---|---|---|
| no/unregistered local account | stderr | common account errors (index.md) | 1 |
| contact discovery rate limited | stderr | `Rate limit reached` optionally followed by a newline and `Next attempt may be tried at <epochMillis> (<UTC time as yyyy-MM-ddTHH:mm:ss.SSSZ>)`, e.g. `Next attempt may be tried at 1700000000000 (2023-11-14T22:13:20.000Z)`; the time is now + server retry-after | 5 |
| network/IO error (numbers or usernames) | stderr | `Unable to check if users are registered: <io message> (<exception simple name>)` | 3 |

## --help transcript

Command: `signal-cli --config <CONFIG> getUserStatus --help` — exit code 0, stderr empty, stdout:

```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli getUserStatus [-h] [--username [USERNAME [USERNAME ...]]]
                  [recipient [recipient ...]]

positional arguments:
  recipient              Phone number

named arguments:
  -h, --help             show this help message and exit
  --username [USERNAME [USERNAME ...]]
                         Specify the recipient username or username link.
```

## Doc vs binary discrepancies

- Positional is shown as `recipient` in help; man page shows `[RECIPIENT ...]` "numbers to check". Only phone numbers are meaningful here (UUIDs / `u:` forms are treated as unparsable numbers -> not registered).
