# signal-cli updateConfiguration

## Sources

- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/commands/UpdateConfigurationCommand.java (tag v0.14.7)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/man/signal-cli.1.adoc section "updateConfiguration"
- Binary observation (signal-cli 0.14.7):
  - `signal-cli --config <CONFIG> updateConfiguration --help`
  - `signal-cli --config <CONFIG> updateConfiguration` (no accounts)
  - `signal-cli --config <CONFIG> updateConfiguration --read-receipts maybe` (no accounts)
  - `signal-cli --config <CONFIG> -a +15555550123 updateConfiguration --read-receipts True`
  - `signal-cli --config <CONFIG> -a +15555550123 updateConfiguration`

## Summary

Help line (verbatim): `Update signal configs and sync them to linked devices.`

Changes account-wide configuration flags (read receipts, unidentified delivery indicators, typing indicators, link previews), stores them and syncs them to linked devices. Only works on the primary device.

## Invocation category & account requirement

Local (single-account) command. Requires a registered account: the account given with `-a/--account` (phone number in E.164 form with leading `+`, or an ACI UUID), or, if `-a` is omitted, the single account present in the data directory. The account must exist locally and be registered, otherwise the command fails before doing anything (no network traffic). Also available as a JSON-RPC method of the same name in daemon/jsonRpc mode (not covered here). With `--dbus`/`--dbus-system` the command is forwarded over D-Bus instead (not covered here).

## Output modes

`plain-text` and `json`; default `plain-text`. Nothing printed on success.

## Arguments

| Names | Arity | Value type / metavar | Default | Required | Help text (verbatim) | Notes |
|---|---|---|---|---|---|---|
| `--read-receipts` | 1 | boolean `{true,false}` | unset (unchanged) | no | `Indicates if Signal should send read receipts.` | |
| `--unidentified-delivery-indicators` | 1 | boolean `{true,false}` | unset | no | `Indicates if Signal should show unidentified delivery indicators.` | |
| `--typing-indicators` | 1 | boolean `{true,false}` | unset | no | `Indicates if Signal should send/show typing indicators.` | |
| `--link-previews` | 1 | boolean `{true,false}` | unset | no | `Indicates if Signal should generate link previews.` | |

Boolean note: help shows `{true,false}` but any string is accepted by the parser (observed: `maybe`, `True` pass parsing). Interpreted as true iff equal to `true` ignoring case, otherwise false (inferred). Missing value → `signal-cli: error: argument <opt>: expected one argument`.

## Constraints & validation

Primary device only (checked when executing). Calling with no options is allowed (sends unchanged configuration / sync).

## Behavior

Updates only the given settings, keeps others; syncs configuration to linked devices. Exit 0, no output.

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
| Linked (non-primary) device | stderr | `This command doesn't work on linked devices.` | 1 |

Other failures (e.g. I/O while syncing) are not mapped by this command; an unexpected exception results in a stack trace on stderr and exit 2 (generic handling, see index.md).

Observed: `updateConfiguration --read-receipts maybe` (no accounts) → `No local users found, you first need to register or link an account`, exit 1.

## --help transcript

Command: `signal-cli --config <CONFIG> updateConfiguration --help` — exit code 0, stderr empty, stdout:

```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli updateConfiguration [-h] [--read-receipts {true,false}]
                  [--unidentified-delivery-indicators {true,false}]
                  [--typing-indicators {true,false}]
                  [--link-previews {true,false}]

named arguments:
  -h, --help             show this help message and exit
  --read-receipts {true,false}
                         Indicates if Signal should send read receipts.
  --unidentified-delivery-indicators {true,false}
                         Indicates  if  Signal   should  show  unidentified
                         delivery indicators.
  --typing-indicators {true,false}
                         Indicates  if  Signal   should   send/show  typing
                         indicators.
  --link-previews {true,false}
                         Indicates if Signal should generate link previews.
```

The first 7 lines (the short usage of only `-h`, `-v`, `--log-file`, `--scrub-log`) are printed by the early logging-options pre-parse and precede every `--help` output; the command-specific help follows immediately with no blank line in between.

## Doc vs binary discrepancies

- Docs show `{true,false}`; binary accepts any string.
