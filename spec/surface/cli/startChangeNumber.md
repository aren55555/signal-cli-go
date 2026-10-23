# signal-cli startChangeNumber

## Sources

- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/commands/StartChangeNumberCommand.java (tag v0.14.7)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/util/CommandUtil.java, .../util/DateUtils.java
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/man/signal-cli.1.adoc section "startChangeNumber"
- Binary observation (signal-cli 0.14.7):
  - `signal-cli --config <CONFIG> startChangeNumber --help`
  - `signal-cli --config <CONFIG> startChangeNumber` (missing positional)
  - `signal-cli --config <CONFIG> startChangeNumber a b`
  - `signal-cli --config <CONFIG> -a +15555550123 startChangeNumber +15555550124`

## Summary

Help line (verbatim): `Change account to a new phone number with SMS or voice verification.`

Starts moving the account to a new phone number: requests a verification code for the new number by SMS or voice. Complete with `finishChangeNumber`. Only works on the primary device.

## Invocation category & account requirement

Local (single-account) command. Requires a registered account: the account given with `-a/--account` (phone number in E.164 form with leading `+`, or an ACI UUID), or, if `-a` is omitted, the single account present in the data directory. The account must exist locally and be registered, otherwise the command fails before doing anything (no network traffic). Also available as a JSON-RPC method of the same name in daemon/jsonRpc mode (not covered here). With `--dbus`/`--dbus-system` the command is forwarded over D-Bus instead (not covered here).

## Output modes

`plain-text` and `json`; default `plain-text`. Nothing printed on success.

## Arguments

| Names | Arity | Value type / metavar | Default | Required | Help text (verbatim) | Notes |
|---|---|---|---|---|---|---|
| `number` (positional) | exactly 1 | string | — | yes | `The new phone number in E164 format.` | Man calls it RECIPIENT. |
| `-v`, `--voice` | 0 (flag) | — | false | no | `The verification should be done over voice, not SMS.` | Quirk: `-v` is also counted as global `--verbose` by the early logging pre-parse (debug logs on stderr). |
| `--captcha` | 1 | string, `CAPTCHA` | none | no | `The captcha token, required if change number failed with a captcha required error.` | |

## Constraints & validation

Exactly one positional; missing → `signal-cli: error: too few arguments`; extra → `signal-cli: error: unrecognized arguments: '<x>'`.

## Behavior

Requests verification code for the new number. Exit 0, no output.

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
| Rate limited | stderr | `Rate limit reached` + optional line `Next attempt may be tried at <ms> (<ISO-8601 UTC with millis and Z>)` | 5 |
| Captcha required, none given | stderr | `Captcha required for verification, use --captcha CAPTCHA` / `To get the token, go to https://signalcaptchas.org/registration/generate.html` / `After solving the captcha, right-click on the "Open Signal" link and copy the link.` (+ optional `Next Captcha may be provided at <ms> (<ISO>)`) | 1 |
| Captcha required, one was given | stderr | `Invalid captcha given.` (+ optional `Next Captcha may be provided at ...`) | 1 |
| New number not normalized | stderr | `Failed to change number: <detail>` | 1 |
| Run on a linked (non-primary) device | stderr | `This command doesn't work on linked devices.` | 1 |
| Network / I/O error | stderr | `Failed to change number: <detail> (<ErrorKind>)` | 3 |
| Verification method not available | stderr | `Failed to register: <detail>` (note: says "register"), with `--voice` additionally `: Before requesting voice verification you need to request SMS verification and wait a minute.` appended | 1 |

Observed: `-a +15555550123 startChangeNumber +15555550124` → `User +15555550123 is not registered.`, exit 1.

## --help transcript

Command: `signal-cli --config <CONFIG> startChangeNumber --help` — exit code 0, stderr empty, stdout:

```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli startChangeNumber [-h] [-v] [--captcha CAPTCHA] number

positional arguments:
  number                 The new phone number in E164 format.

named arguments:
  -h, --help             show this help message and exit
  -v, --voice            The verification should  be  done  over voice, not
                         SMS.
  --captcha CAPTCHA      The  captcha  token,  required  if  change  number
                         failed with a captcha required error.
```

The first 7 lines (the short usage of only `-h`, `-v`, `--log-file`, `--scrub-log`) are printed by the early logging-options pre-parse and precede every `--help` output; the command-specific help follows immediately with no blank line in between.

## Doc vs binary discrepancies

- Man page calls the positional RECIPIENT; binary usage shows `number`.
