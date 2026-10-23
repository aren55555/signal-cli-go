# signal-cli verify

## Sources

- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/commands/VerifyCommand.java (tag v0.14.7)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/App.java (registration-command account resolution)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/man/signal-cli.1.adoc section "verify"
- Binary observation (signal-cli 0.14.7):
  - `signal-cli --config <CONFIG> verify --help`
  - `signal-cli --config <CONFIG> verify` (missing positional)
  - `signal-cli --config <CONFIG> verify 1 2`
  - `signal-cli --config <CONFIG> -a 12345 verify 123456`

## Summary

Help line (verbatim): `Verify the number using the code received via SMS or voice.`

Completes a registration started with `register` by submitting the verification code to the server, optionally with the registration-lock PIN. On success the local account becomes registered.

## Invocation category & account requirement

Registration command: account must be a phone number with country code (ACI UUID rejected); if `-a` omitted, the single local account is used. The account does not need to be registered yet (it normally is pending verification). Contacts the Signal server.

## Output modes

`plain-text` and `json`; default `plain-text`. Nothing printed on success in either mode (exit 0).

## Arguments

| Names | Arity | Value type / metavar | Default | Required | Help text (verbatim) | Notes |
|---|---|---|---|---|---|---|
| `verification_code` (positional) | exactly 1 | string | — | yes | `The verification code you received via sms or voice call.` | Shown as `verification_code` in usage. Man page calls it VERIFICATIONCODE. |
| `-p`, `--pin` | 1 | string, metavar `PIN` | none | no | `The registration lock PIN, that was set by the user (Optional)` | Only needed if a registration lock PIN is set. |

## Constraints & validation

- Exactly one positional; a second positional is a parse error (`unrecognized arguments: '<x>'`).

## Behavior

Submits code (+ PIN) for the account; on success stores the registered state locally. Exit 0, no output.

## Plain-text output

None.

## JSON output

None.

## Errors

| Condition | Stream | Exact message | Exit |
|---|---|---|---|
| `-o` output type not supported by the command (checked before any account handling; cannot occur for commands supporting both modes) | stderr | `Command doesn't support output type <plain-text\|json>` | 1 |
| No `-a` and zero local accounts | stderr | `No local users found, you first need to register or link an account` | 1 |
| No `-a` and more than one local account | stderr | `Multiple users found, you need to specify an account (phone number) with -a` | 1 |
| Account is not a valid phone number with country code (an ACI UUID is NOT accepted here) | stderr | `Invalid account (phone number), make sure you include the country code.` | 1 |
| Local state for the number could not be loaded or created | stderr | `Error loading or creating state file: <detail> (<ErrorKind>)` | 2 |
| Argument parse error | stderr | sub-command usage line(s) followed by `signal-cli: error: <reason>` | 1 |
| Number is registration-locked and no/insufficient PIN | stderr | two lines: `Verification failed! This number is locked with a pin. Hours remaining until reset: <H>` / `Use '--pin PIN_CODE' to specify the registration lock PIN` — `<H>` is the remaining lock time in whole hours (integer division of milliseconds by 3,600,000) | 1 |
| Wrong PIN | stderr | `Verification failed! Invalid pin, tries remaining: <N>` | 1 |
| Account PIN-locked but PIN data deleted on server | stderr | `Account is pin locked, but pin data has been deleted on the server.` | 1 |
| Other server / I/O failure (includes wrong code) | stderr | `Verify error: <detail>` | 3 |

Observed: `verify` → stderr `usage: signal-cli verify [-h] [-p PIN] verification_code` / `signal-cli: error: too few arguments`, exit 1. `verify 1 2` → `signal-cli: error: unrecognized arguments: '2'`, exit 1. `-a 12345 verify 123456` → `Invalid account (phone number), make sure you include the country code.`, exit 1.

## --help transcript

Command: `signal-cli --config <CONFIG> verify --help` — exit code 0, stderr empty, stdout:

```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli verify [-h] [-p PIN] verification_code

positional arguments:
  verification_code      The verification  code  you  received  via  sms or
                         voice call.

named arguments:
  -h, --help             show this help message and exit
  -p PIN, --pin PIN      The registration lock  PIN,  that  was  set by the
                         user (Optional)
```

The first 7 lines (the short usage of only `-h`, `-v`, `--log-file`, `--scrub-log`) are printed by the early logging-options pre-parse and precede every `--help` output; the command-specific help follows immediately with no blank line in between.

## Doc vs binary discrepancies

- Man page names the positional VERIFICATIONCODE; binary usage shows `verification_code`. Only the display differs.
