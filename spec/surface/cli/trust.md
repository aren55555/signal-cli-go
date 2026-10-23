# signal-cli trust

## Sources

- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/commands/TrustCommand.java (tag v0.14.7)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/util/CommandUtil.java
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/lib/src/main/java/org/asamk/signal/manager/api/IdentityVerificationCode.java
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/lib/src/main/java/org/asamk/signal/manager/internal/ManagerImpl.java (trust behaviour)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/man/signal-cli.1.adoc section "trust"
- Binary observation (signal-cli 0.14.7, `--config <CONFIG>` = empty scratch dir), commands run:
  - `signal-cli --config <CONFIG> trust --help`
  - `signal-cli --config <CONFIG> trust`
  - `signal-cli --config <CONFIG> trust -a -v 123 +15555550199`
  - `signal-cli --config <CONFIG> -a +15555550123 trust +15555550199`
  - `signal-cli --config <CONFIG> -a +15555550123 trust -a +15555550199`
  - `signal-cli --config <CONFIG> -a +15555550123 trust -v 123 +15555550199`

## Summary

Help line: `Set the trust level of a given number.`

Marks the identity key(s) of one recipient as trusted. Either all currently known keys of
the recipient are trusted unconditionally (`-a`), or the key matching a verified safety
number / fingerprint (`-v`) is marked as verified.

## Invocation category & account requirement

Local single-account command. Requires a registered local account (resolved via global
`-a`, or the only local account if exactly one exists). Account-resolution errors are the
common ones described in index.md (e.g. `No local users found, you first need to register
or link an account`, `User +15555550123 is not registered.`, exit 1). Does not contact the
server by itself.

## Output modes

plain-text (default) and json. Produces no output on success in either mode (json mode
prints nothing, not even `{}`).

## Arguments

| names | arity | value type / metavar | default | required | help text (verbatim) | notes |
|---|---|---|---|---|---|---|
| `recipient` (positional) | exactly 1 | string, RECIPIENT | — | yes | `Specify the phone number, for which to set the trust.` | Any recipient form (phone number, ACI UUID, `PNI:`-prefixed UUID, `u:`username). |
| `-a`, `--trust-all-known-keys` | 0 (flag) | boolean | false | no | `Trust all known keys of this user, only use this for testing.` | Mutually exclusive with `-v`. |
| `-v`, `--verified-safety-number`, `--verified-fingerprint` | 1 | string, VERIFIED_SAFETY_NUMBER | none | no (but see validation) | `Specify the safety number of the key, only use this option if you have verified the safety number.` | Mutually exclusive with `-a`. |

Mutual-exclusion group: { `-a/--trust-all-known-keys`, `-v/--verified-safety-number/--verified-fingerprint` } (not required at parse level).

## Constraints & validation

1. Parse-level: `recipient` missing -> `signal-cli: error: too few arguments` (exit 1). Both
   `-a` and `-v` -> mutex error (exit 1, see Errors).
2. After account load, the recipient string is parsed; an invalid phone number yields
   `Invalid phone number '<input>': <reason>` (exit 1).
3. If `-a` is not given and `-v` is not given:
   `You need to specify the fingerprint/safety number you have verified with -v SAFETY_NUMBER` (exit 1).
4. Safety-number format (`-v`): all ASCII spaces are removed first. Then:
   - length 66 characters: interpreted as the legacy hex fingerprint (case-insensitive hex; 33 bytes);
   - length 60 characters: interpreted as the numeric safety number shown in the apps;
   - any other length: interpreted as standard base64 of the scannable (QR-code) safety number bytes.
   If decoding fails (bad hex, bad base64):
   `Safety number has invalid format, either specify the old hex fingerprint or the new safety number` (exit 1).

## Behavior

- `-a`: every known identity key of the recipient is set to trusted (unverified-trusted).
  If nothing could be updated (e.g. no known identity for the recipient):
  `Failed to set the trust for this number, make sure the number is correct.` (exit 1).
- `-v`: the recipient's current identity key is checked against the given code (fingerprint,
  numeric safety number or scannable safety number). On match it is marked verified. On
  mismatch / no identity:
  `Failed to set the trust for this number, make sure the number and the fingerprint/safety number are correct.` (exit 1).
- If the recipient is a username / identifier that cannot be resolved to a registered user:
  `The user <identifier> is not registered.` (exit 1).
- When trust was changed while the account is actively receiving (daemon), failed messages are
  scheduled for retry (no user-visible output).

## Plain-text output

None on success.

## JSON output

None on success (nothing written to stdout).

## Errors

| condition | stream | exact message | exit |
|---|---|---|---|
| missing recipient | stderr | usage line + `signal-cli: error: too few arguments` | 1 |
| `-a` together with `-v` | stderr | usage line + `signal-cli: error: argument -v/--verified-safety-number/--verified-fingerprint: not allowed with argument -a/--trust-all-known-keys` (wrapped/justified to 75 cols as argparse does; see transcript) | 1 |
| no/unregistered local account | stderr | common account errors (index.md) | 1 |
| invalid recipient number | stderr | `Invalid phone number '<input>': <reason>` | 1 |
| neither `-a` nor `-v` | stderr | `You need to specify the fingerprint/safety number you have verified with -v SAFETY_NUMBER` | 1 |
| unparsable safety number | stderr | `Safety number has invalid format, either specify the old hex fingerprint or the new safety number` | 1 |
| `-a` and nothing updated | stderr | `Failed to set the trust for this number, make sure the number is correct.` | 1 |
| `-v` and no match | stderr | `Failed to set the trust for this number, make sure the number and the fingerprint/safety number are correct.` | 1 |
| recipient not registered | stderr | `The user <identifier> is not registered.` | 1 |

Observed transcript of the mutex error (exact bytes on stderr; note the first line is a
debug log line, see quirk below):

```
2026-10-23T06:12:27.929Z [main] DEBUG org.asamk.signal.util.IOUtils - XDG_RUNTIME_DIR not set, falling back to temp dir
usage: signal-cli trust [-h] [-a | -v VERIFIED_SAFETY_NUMBER] recipient
signal-cli:   error:    argument    -v/--verified-safety-number/--verified-
fingerprint: not allowed with argument -a/--trust-all-known-keys
```

### Quirk: subcommand `-v` also raises the global log level

The early logging pre-parser scans the *whole* argument list for `-v`/`--verbose`
(ignoring unknown arguments), so `trust -v <SAFETY_NUMBER> ...` also turns on verbose
(DEBUG) logging to stderr, exactly as if the global `-v` had been given. Observed with
`-a +15555550123 trust -v 123 +15555550199`: stderr contained
`<timestamp> [main] DEBUG org.asamk.signal.util.IOUtils - XDG_RUNTIME_DIR not set, falling back to temp dir`,
`<timestamp> [main] DEBUG org.asamk.signal.App - Starting signal-cli 0.14.7`, then
`User +15555550123 is not registered.` (exit 1). The long form `--verified-safety-number`
does not trigger this (only the token `-v` / `--verbose` does).

## --help transcript

Command: `signal-cli --config <CONFIG> trust --help` — exit code 0, stderr empty, stdout:

```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli trust [-h] [-a | -v VERIFIED_SAFETY_NUMBER] recipient

positional arguments:
  recipient              Specify the phone  number,  for  which  to set the
                         trust.

named arguments:
  -h, --help             show this help message and exit
  -a, --trust-all-known-keys
                         Trust all known keys of  this  user, only use this
                         for testing.
  -v VERIFIED_SAFETY_NUMBER, --verified-safety-number VERIFIED_SAFETY_NUMBER, --verified-fingerprint VERIFIED_SAFETY_NUMBER
                         Specify the safety  number  of  the  key, only use
                         this  option  if  you  have  verified  the  safety
                         number.
```

## Doc vs binary discrepancies

- Man page describes the positional as RECIPIENT ("Specify the recipient"); `--help` says
  "phone number". Any recipient form is accepted (binary authoritative: the argument is
  parsed with the generic recipient parser).
- Man page does not list the `--verified-fingerprint` alias; binary accepts it.
