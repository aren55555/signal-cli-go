# signal-cli submitRateLimitChallenge

## Sources

- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/commands/SubmitRateLimitChallengeCommand.java (tag v0.14.7)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/Main.java (exit code 6 mapping)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/man/signal-cli.1.adoc section "submitRateLimitChallenge"
- Binary observation (signal-cli 0.14.7):
  - `signal-cli --config <CONFIG> submitRateLimitChallenge --help`
  - `signal-cli --config <CONFIG> submitRateLimitChallenge` (no args)
  - `signal-cli --config <CONFIG> submitRateLimitChallenge --challenge x`
  - `signal-cli --config <CONFIG> submitRateLimitChallenge --captcha x`
  - `signal-cli --config <CONFIG> -a +15555550123 submitRateLimitChallenge --challenge x --captcha y`

## Summary

Help line (verbatim): `Submit a captcha challenge to lift the rate limit. This command should only be necessary when sending fails with a proof required error.`

Submits a solved captcha together with the challenge token from a "proof required" send failure, so the server lifts the sending rate limit. Man page: captcha from https://signalcaptchas.org/challenge/generate.html (staging: https://signalcaptchas.org/staging/registration/generate.html); the captcha value starts with `signalcaptcha://`.

## Invocation category & account requirement

Local (single-account) command. Requires a registered account: the account given with `-a/--account` (phone number in E.164 form with leading `+`, or an ACI UUID), or, if `-a` is omitted, the single account present in the data directory. The account must exist locally and be registered, otherwise the command fails before doing anything (no network traffic). Also available as a JSON-RPC method of the same name in daemon/jsonRpc mode (not covered here). With `--dbus`/`--dbus-system` the command is forwarded over D-Bus instead (not covered here).

Contacts the server.

## Output modes

`plain-text` and `json`; default `plain-text`. Nothing printed on success.

## Arguments

| Names | Arity | Value type / metavar | Default | Required | Help text (verbatim) | Notes |
|---|---|---|---|---|---|---|
| `--challenge` | 1 | string, `CHALLENGE` | — | yes | `The challenge token taken from the proof required error.` | |
| `--captcha` | 1 | string, `CAPTCHA` | — | yes | `The captcha token from the solved captcha on the signal website.` | |

## Constraints & validation

- Missing `--captcha` → `signal-cli: error: argument --captcha is required`; missing `--challenge` → `signal-cli: error: argument --challenge is required`; both missing → the `--captcha` error is reported (observed).

## Behavior

Submits the recaptcha challenge response. Exit 0, no output.

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
| Server / I/O failure | stderr | `Submit challenge error: <detail>` | 3 |
| Captcha rejected by server | stderr | `Captcha rejected, it may be outdated, already used or solved from a different IP address.` | 6 |

This is the only command in this group producing exit code 6.

## --help transcript

Command: `signal-cli --config <CONFIG> submitRateLimitChallenge --help` — exit code 0, stderr empty, stdout:

```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli submitRateLimitChallenge [-h] --challenge CHALLENGE
                  --captcha CAPTCHA

named arguments:
  -h, --help             show this help message and exit
  --challenge CHALLENGE  The challenge token taken  from the proof required
                         error.
  --captcha CAPTCHA      The captcha token from  the  solved captcha on the
                         signal website.
```

The first 7 lines (the short usage of only `-h`, `-v`, `--log-file`, `--scrub-log`) are printed by the early logging-options pre-parse and precede every `--help` output; the command-specific help follows immediately with no blank line in between.

## Doc vs binary discrepancies

None found.
