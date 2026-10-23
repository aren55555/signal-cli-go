# signal-cli setPin

## Sources

- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/commands/SetPinCommand.java (tag v0.14.7)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/man/signal-cli.1.adoc section "setPin"
- Binary observation (signal-cli 0.14.7):
  - `signal-cli --config <CONFIG> setPin --help`
  - `signal-cli --config <CONFIG> setPin` (missing positional)
  - `signal-cli --config <CONFIG> setPin a b`
  - `signal-cli --config <CONFIG> -a +15555550123 setPin 1234`

## Summary

Help line (verbatim): `Set a registration lock pin, to prevent others from registering this number.`

Sets (or replaces) the registration lock PIN for the account (stored with the secure value recovery service and enabled as registration lock). Primary device only.

## Invocation category & account requirement

Local (single-account) command. Requires a registered account: the account given with `-a/--account` (phone number in E.164 form with leading `+`, or an ACI UUID), or, if `-a` is omitted, the single account present in the data directory. The account must exist locally and be registered, otherwise the command fails before doing anything (no network traffic). Also available as a JSON-RPC method of the same name in daemon/jsonRpc mode (not covered here). With `--dbus`/`--dbus-system` the command is forwarded over D-Bus instead (not covered here).

## Output modes

`plain-text` and `json`; default `plain-text`. Nothing printed on success.

## Arguments

| Names | Arity | Value type / metavar | Default | Required | Help text (verbatim) | Notes |
|---|---|---|---|---|---|---|
| `pin` (positional) | exactly 1 | string | — | yes | `The registration lock PIN, that will be required for new registrations (resets after 7 days of inactivity)` | Man calls it REGISTRATION_LOCK_PIN. No format validation at the CLI layer. |

## Constraints & validation

Missing → `signal-cli: error: too few arguments`; extra positional → `signal-cli: error: unrecognized arguments: '<x>'` (both observed, exit 1).

## Behavior

Sets the PIN, exit 0, no output.

## Plain-text output

None.

## JSON output

None.

## Errors

| Condition | Stream | Exact message | Exit |
|---|---|---|---|
| `-o` output type not supported by the command (checked before any account handling; cannot occur for commands supporting both modes) | stderr | `Command doesn't support output type <plain-text\|json>` | 1 |
| No `-a` and zero local accounts in the data directory | stderr | `No local users found, you first need to register or link an account` | 1 |
| No `-a` and more than one local account | stderr | `Multiple users found, you need to specify an account (phone number) with -a` | 1 |
| `-a` value is neither a valid phone number (with country code) nor an ACI UUID | stderr | `Invalid account (phone number), make sure you include the country code.` | 1 |
| Account unknown locally or not registered | stderr | `User <ACCOUNT> is not registered.` (ACCOUNT echoed exactly as given / resolved) | 1 |
| Account check failed with an I/O problem | stderr | `Error while checking account <ACCOUNT>: <detail>` | 3 |
| Account check failed otherwise | stderr | `Error while checking account <ACCOUNT>: <detail>` | 2 |
| Account state file could not be loaded | stderr | `Error loading state file for user <ACCOUNT>: <detail> (<ErrorKind>)` | 2 |
| Argument parse error (unknown flag, missing required, bad value) | stderr | sub-command usage line(s) followed by `signal-cli: error: <reason>` (see index.md) | 1 |
| Server / I/O failure | stderr | `Set pin error: <detail>` | 3 |
| Linked (non-primary) device | stderr | `This command doesn't work on linked devices.` | 1 |

Observed: `-a +15555550123 setPin 1234` → `User +15555550123 is not registered.`, exit 1.

## --help transcript

Command: `signal-cli --config <CONFIG> setPin --help` — exit code 0, stderr empty, stdout:

```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli setPin [-h] pin

positional arguments:
  pin                    The registration lock PIN,  that  will be required
                         for new  registrations  (resets  after  7  days of
                         inactivity)

named arguments:
  -h, --help             show this help message and exit
```

The first 7 lines (the short usage of only `-h`, `-v`, `--log-file`, `--scrub-log`) are printed by the early logging-options pre-parse and precede every `--help` output; the command-specific help follows immediately with no blank line in between.

## Doc vs binary discrepancies

None (positional display name differs only: man REGISTRATION_LOCK_PIN vs binary `pin`).
