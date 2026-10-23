# signal-cli finishChangeNumber

## Sources

- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/commands/FinishChangeNumberCommand.java (tag v0.14.7)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/Main.java (logging pre-parse)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/man/signal-cli.1.adoc section "finishChangeNumber"
- Binary observation (signal-cli 0.14.7):
  - `signal-cli --config <CONFIG> finishChangeNumber --help`
  - `signal-cli --config <CONFIG> finishChangeNumber` (no args)
  - `signal-cli --config <CONFIG> finishChangeNumber +15555550124`
  - `signal-cli --config <CONFIG> finishChangeNumber -v 123`
  - `signal-cli --config <CONFIG> -a +15555550123 finishChangeNumber -v 123 +15555550124`

## Summary

Help line (verbatim): `Verify the new number using the code received via SMS or voice.`

Completes a number change started with `startChangeNumber` by submitting the code received on the new number (and the registration lock PIN if the new number has one). Primary device only.

## Invocation category & account requirement

Local (single-account) command. Requires a registered account: the account given with `-a/--account` (phone number in E.164 form with leading `+`, or an ACI UUID), or, if `-a` is omitted, the single account present in the data directory. The account must exist locally and be registered, otherwise the command fails before doing anything (no network traffic). Also available as a JSON-RPC method of the same name in daemon/jsonRpc mode (not covered here). With `--dbus`/`--dbus-system` the command is forwarded over D-Bus instead (not covered here).

## Output modes

`plain-text` and `json`; default `plain-text`. Nothing printed on success.

## Arguments

| Names | Arity | Value type / metavar | Default | Required | Help text (verbatim) | Notes |
|---|---|---|---|---|---|---|
| `number` (positional) | exactly 1 | string | — | yes | `The new phone number in E164 format.` | |
| `-v`, `--verification-code` | 1 | string, `VERIFICATION_CODE` | — | yes | `The verification code you received via sms or voice call.` | Quirk: the short form `-v` is ALSO counted by the early logging pre-parse as global `--verbose`, so using `-v CODE` turns on debug logging on stderr (observed). `--verification-code` avoids this. |
| `-p`, `--pin` | 1 | string, `PIN` | none | no | `The registration lock PIN, that was set by the user (Optional)` | |

## Constraints & validation

- Missing `-v` (observed, `finishChangeNumber +15555550124`): `signal-cli: error: argument -v/--verification-code is required`, exit 1.
- Missing positional (observed, `finishChangeNumber -v 123`): `signal-cli: error: too few arguments`, exit 1 — preceded on stderr by a DEBUG log line because of the `-v` quirk, e.g. `<ISO timestamp> [main] DEBUG org.asamk.signal.util.IOUtils - XDG_RUNTIME_DIR not set, falling back to temp dir` (log line content is environment-dependent).
- When both are missing (observed, `finishChangeNumber` alone, no `-v` so no debug line) the reported error is `signal-cli: error: too few arguments` — missing positionals are reported before missing required options.

## Behavior

Submits the code for the new number; on success the local account switches to the new number. Exit 0, no output.

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
| New number registration-locked, no/insufficient PIN | stderr | `Verification failed! This number is locked with a pin. Hours remaining until reset: <H>` / `Use '--pin PIN_CODE' to specify the registration lock PIN` | 1 |
| Wrong PIN | stderr | `Verification failed! Invalid pin, tries remaining: <N>` | 1 |
| PIN data deleted on server | stderr | `Account is pin locked, but pin data has been deleted on the server.` | 1 |
| Linked (non-primary) device | stderr | `This command doesn't work on linked devices.` | 1 |
| Network / I/O error | stderr | `Failed to change number: <detail> (<ErrorKind>)` | 3 |

Observed: `-a +15555550123 finishChangeNumber -v 123 +15555550124` → stderr two DEBUG lines (`... DEBUG org.asamk.signal.util.IOUtils - XDG_RUNTIME_DIR not set, falling back to temp dir`, `... DEBUG org.asamk.signal.App - Starting signal-cli 0.14.7`) then `User +15555550123 is not registered.`, exit 1.

## --help transcript

Command: `signal-cli --config <CONFIG> finishChangeNumber --help` — exit code 0, stderr empty, stdout:

```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli finishChangeNumber [-h] -v VERIFICATION_CODE [-p PIN]
                  number

positional arguments:
  number                 The new phone number in E164 format.

named arguments:
  -h, --help             show this help message and exit
  -v VERIFICATION_CODE, --verification-code VERIFICATION_CODE
                         The verification  code  you  received  via  sms or
                         voice call.
  -p PIN, --pin PIN      The registration lock  PIN,  that  was  set by the
                         user (Optional)
```

The first 7 lines (the short usage of only `-h`, `-v`, `--log-file`, `--scrub-log`) are printed by the early logging-options pre-parse and precede every `--help` output; the command-specific help follows immediately with no blank line in between.

## Doc vs binary discrepancies

- Man page lists `-v, --verification-code` without a metavar; binary requires a value (`VERIFICATION_CODE`).
- Man calls the positional RECIPIENT; binary shows `number`.
