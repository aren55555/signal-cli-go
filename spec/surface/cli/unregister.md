# signal-cli unregister

## Sources

- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/commands/UnregisterCommand.java (tag v0.14.7)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/App.java
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/man/signal-cli.1.adoc section "unregister"
- Binary observation (signal-cli 0.14.7):
  - `signal-cli --config <CONFIG> unregister --help`
  - `signal-cli --config <CONFIG> unregister` (no accounts)
  - `signal-cli --config <CONFIG> unregister --bogus`
  - `signal-cli --config <CONFIG> -a +15555550123 unregister`
  - `signal-cli --config <CONFIG> -a +15555550123 unregister --delete-account`

## Summary

Help line (verbatim): `Unregister the current device from the signal server.`

Man page: disables push support for this device, so it won't receive any more messages. If this is the primary device, other users can no longer send messages to this number. `updateAccount` undoes this. To remove a linked device use `removeDevice` from the primary. With `--delete-account` the account is deleted completely from the server (cannot be undone; user must be re-added to every group).

## Invocation category & account requirement

Local (single-account) command. Requires a registered account: the account given with `-a/--account` (phone number in E.164 form with leading `+`, or an ACI UUID), or, if `-a` is omitted, the single account present in the data directory. The account must exist locally and be registered, otherwise the command fails before doing anything (no network traffic). Also available as a JSON-RPC method of the same name in daemon/jsonRpc mode (not covered here). With `--dbus`/`--dbus-system` the command is forwarded over D-Bus instead (not covered here).

## Output modes

`plain-text` and `json`; default `plain-text`. Nothing printed on success.

## Arguments

| Names | Arity | Value type / metavar | Default | Required | Help text (verbatim) | Notes |
|---|---|---|---|---|---|---|
| `--delete-account` | 0 (flag) | — | false | no | `Delete account completely from server. CAUTION: Only do this if you won't use this number again!` | |

## Constraints & validation

None beyond argument parsing.

## Behavior

Without `--delete-account`: deactivates the account/device on the server (unregister). With it: deletes the account on the server. Exit 0, no output.

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
| Server / I/O failure | stderr | `Unregister error: <detail>` | 3 |

Observed: with `-a +15555550123` and no such account → `User +15555550123 is not registered.`, exit 1 (both with and without `--delete-account`).

## --help transcript

Command: `signal-cli --config <CONFIG> unregister --help` — exit code 0, stderr empty, stdout:

```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli unregister [-h] [--delete-account]

named arguments:
  -h, --help             show this help message and exit
  --delete-account       Delete account  completely  from  server. CAUTION:
                         Only do this if you won't use this number again!
```

The first 7 lines (the short usage of only `-h`, `-v`, `--log-file`, `--scrub-log`) are printed by the early logging-options pre-parse and precede every `--help` output; the command-specific help follows immediately with no blank line in between.

## Doc vs binary discrepancies

None found.
