# signal-cli register

## Sources

- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/commands/RegisterCommand.java (tag v0.14.7)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/util/CommandUtil.java (captcha / rate-limit message texts)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/util/DateUtils.java (timestamp rendering)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/App.java (account resolution for registration commands)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/man/signal-cli.1.adoc section "register", "Options"
- Binary observation (signal-cli 0.14.7, JVM; `--config` pointing at an empty scratch dir):
  - `signal-cli --config <CONFIG> register --help`
  - `signal-cli --config <CONFIG> register` (no accounts)
  - `signal-cli --config <CONFIG> register --bogus`, `... register extra`
  - `signal-cli --config <CONFIG> -a 12345 register`, `... -a 0b1c3e2a-1111-2222-3333-444455556666 register`
  - `signal-cli --config <CONFIG> -a +15555550123 -o json register` (run once by the coordinating agent; the network attempt was blocked by the sandbox egress proxy, see Errors)

## Summary

Help line (verbatim): `Register a phone number with SMS or voice verification.`

Requests a verification code for the account's phone number from the Signal server, by SMS (default) or voice call. Completion happens with `verify`. Per the man page: if the account is only deactivated (unregistered without `--delete-account`) and local data still exists, register just reactivates it without needing SMS verification; an account deleted with `unregister --delete-account` cannot be reactivated. If the account is already registered, nothing is re-requested unless `--reregister` is given.

## Invocation category & account requirement

Registration command. Needs an account phone number but NOT a registered account:
- `-a/--account` must be a valid phone number including the country calling code (leading `+`). An ACI UUID is rejected.
- If `-a` is omitted, the single local account in the data directory is used (zero → error, more than one → error; see Errors).
- Local state for the number is created in the data directory if it does not exist yet (observed: a failed `register` attempt leaves an `accounts.json` entry with `"uuid": null` plus an account directory; that entry is afterwards reported as not registered).
- Contacts the Signal server.

## Output modes

Supports `plain-text` and `json`; default `plain-text`. On success nothing is printed to stdout in either mode (exit 0).

## Arguments

| Names | Arity | Value type / metavar | Default | Required | Help text (verbatim) | Notes |
|---|---|---|---|---|---|---|
| `-v`, `--voice` | 0 (flag) | — | false | no | `The verification should be done over voice, not SMS.` | Man page: voice only works if an SMS verification has been attempted before. Quirk: a literal `-v` anywhere on the command line is also counted by the early logging pre-parse as global `--verbose` (see index.md), so `register -v` also turns on debug logging on stderr. `--voice` does not have this effect. |
| `--captcha` | 1 | string, metavar `CAPTCHA` | none | no | `The captcha token, required if registration failed with a captcha required error.` | Man page: token from https://signalcaptchas.org/registration/generate.html (staging: https://signalcaptchas.org/staging/registration/generate.html); the value is the "Open Signal" link (`signalcaptcha://...`). |
| `--reregister` | 0 (flag) | — | false | no | `Register even if account is already registered` | (no trailing period in help) |

No positional arguments.

## Constraints & validation

- No mutually exclusive groups.
- Account validation as above (phone number only).

## Behavior

1. Resolve account (see category). 2. Load or create the local registration state. 3. Ask the server to send a verification code via SMS or voice (with captcha if given). If the account is already registered and `--reregister` is not given, registration is not re-requested (behavior inside the manager library; not observable without an account). 4. Exit 0 silently.

## Plain-text output

None on success.

## JSON output

None on success (the CLI prints nothing; the JSON-RPC method of the same name returns an empty result, out of scope here).

## Errors

| Condition | Stream | Exact message | Exit |
|---|---|---|---|
| `-o` output type not supported by the command (checked before any account handling; cannot occur for commands supporting both modes) | stderr | `Command doesn't support output type <plain-text\|json>` | 1 |
| No `-a` and zero local accounts | stderr | `No local users found, you first need to register or link an account` | 1 |
| No `-a` and more than one local account | stderr | `Multiple users found, you need to specify an account (phone number) with -a` | 1 |
| Account is not a valid phone number with country code (an ACI UUID is NOT accepted here) | stderr | `Invalid account (phone number), make sure you include the country code.` | 1 |
| Local state for the number could not be loaded or created | stderr | `Error loading or creating state file: <detail> (<ErrorKind>)` | 2 |
| Argument parse error | stderr | sub-command usage line(s) followed by `signal-cli: error: <reason>` | 1 |
| Server rate limit | stderr | `Rate limit reached`; if the server supplied a retry-after, a second line `Next attempt may be tried at <ms> (<yyyy-MM-ddTHH:mm:ss.SSSZ>)` where `<ms>` is now+retry-after in epoch milliseconds and the parenthesised part is the same instant in UTC ISO-8601 with milliseconds and `Z` | 5 |
| Captcha required, no `--captcha` given | stderr | three lines: `Captcha required for verification, use --captcha CAPTCHA` / `To get the token, go to https://signalcaptchas.org/registration/generate.html` / `After solving the captcha, right-click on the "Open Signal" link and copy the link.`; optionally a 4th line `Next Captcha may be provided at <ms> (<ISO UTC>)` if the server gave a wait time > 0 | 1 |
| Captcha required although `--captcha` was given | stderr | `Invalid captcha given.` (optionally followed by the `Next Captcha may be provided at ...` line) | 1 |
| Server reports the number is not normalized | stderr | `Failed to register: <detail>` | 1 |
| Requested verification method not available | stderr | `Failed to register: <detail>`; with `--voice` the text `: Before requesting voice verification you need to request SMS verification and wait a minute.` is appended | 1 |
| Network / server I/O failure | stderr | `Failed to register: <detail> (<ErrorKind>)` — observed in the sandbox (egress blocked): `Failed to register: java.io.IOException: Unexpected response code for CONNECT: 403 (PushNetworkException)` | 3 |

Observed: `signal-cli --config <CONFIG> register` with an empty data dir → stderr `No local users found, you first need to register or link an account`, exit 1. `-a 12345 register` and `-a <uuid> register` → `Invalid account (phone number), make sure you include the country code.`, exit 1.

## --help transcript

Command: `signal-cli --config <CONFIG> register --help` — exit code 0, stderr empty, stdout:

```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli register [-h] [-v] [--captcha CAPTCHA] [--reregister]

named arguments:
  -h, --help             show this help message and exit
  -v, --voice            The verification should  be  done  over voice, not
                         SMS.
  --captcha CAPTCHA      The  captcha  token,   required   if  registration
                         failed with a captcha required error.
  --reregister           Register even if account is already registered
```

The first 7 lines (the short usage of only `-h`, `-v`, `--log-file`, `--scrub-log`) are printed by the early logging-options pre-parse and precede every `--help` output; the command-specific help follows immediately with no blank line in between.

## Doc vs binary discrepancies

- Man page lists `--reregister` help as "Register even if account is already registered." The binary help has no trailing period.
- The man page does not mention that `-v` also raises the global verbosity (binary behavior; authoritative).
