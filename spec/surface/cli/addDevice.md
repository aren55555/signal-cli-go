# signal-cli addDevice

## Sources

- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/commands/AddDeviceCommand.java (tag v0.14.7)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/man/signal-cli.1.adoc section "addDevice"
- Binary observation (signal-cli 0.14.7):
  - `signal-cli --config <CONFIG> addDevice --help`
  - `signal-cli --config <CONFIG> addDevice` (missing required)
  - `signal-cli --config <CONFIG> addDevice --uri` (missing value)
  - `signal-cli --config <CONFIG> -a +15555550123 addDevice --uri 'sgnl://linkdevice?uuid=x&pub_key=y'`

## Summary

Help line (verbatim): `Link another device to this device. Only works, if this is the primary device.`

Approves a new secondary device: given the device-link URI shown by the new device (e.g. by `signal-cli link`), sends it the provisioning data.

## Invocation category & account requirement

Local (single-account) command. Requires a registered account: the account given with `-a/--account` (phone number in E.164 form with leading `+`, or an ACI UUID), or, if `-a` is omitted, the single account present in the data directory. The account must exist locally and be registered, otherwise the command fails before doing anything (no network traffic). Also available as a JSON-RPC method of the same name in daemon/jsonRpc mode (not covered here). With `--dbus`/`--dbus-system` the command is forwarded over D-Bus instead (not covered here).

Primary device only.

## Output modes

`plain-text` and `json`; default `plain-text`. Nothing printed on success.

## Arguments

| Names | Arity | Value type / metavar | Default | Required | Help text (verbatim) | Notes |
|---|---|---|---|---|---|---|
| `--uri` | 1 | string, `URI` | — | yes | `Specify the uri contained in the QR code shown by the new device.` | Full URI like `sgnl://linkdevice?uuid=...&pub_key=...` (formerly `tsdevice:/?uuid=...`); quote it in shells. |

## Constraints & validation

- Missing `--uri` → `signal-cli: error: argument --uri is required`; `--uri` without value → `signal-cli: error: argument --uri: expected one argument` (observed).
- The value must be a syntactically valid URI, and must be a valid device link (contains the device identifier and public key), checked after account loading.

## Behavior

Parse URI → validate as device link → provision the new device. Exit 0, no output.

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
| Value is not a syntactically valid URI | stderr | `Device link uri has invalid format: <URI-syntax detail>` | 1 |
| URI is not a valid device link | stderr | `Invalid device link` (an INFO log line is emitted only at higher verbosity) | 1 |
| Too many linked devices | stderr | `Account has too many linked devices already` | 1 |
| Linked (non-primary) device | stderr | `This command doesn't work on linked devices.` | 1 |
| Network / I/O error | stderr | first an error log line (default log format, e.g. `ERROR AddDeviceCommand - Add device link failed: <detail>`), then `Add device link failed` | 3 |

Observed: `-a +15555550123 addDevice --uri 'sgnl://linkdevice?uuid=x&pub_key=y'` → `User +15555550123 is not registered.`, exit 1.

## --help transcript

Command: `signal-cli --config <CONFIG> addDevice --help` — exit code 0, stderr empty, stdout:

```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli addDevice [-h] --uri URI

named arguments:
  -h, --help             show this help message and exit
  --uri URI              Specify the uri contained in  the QR code shown by
                         the new device.
```

The first 7 lines (the short usage of only `-h`, `-v`, `--log-file`, `--scrub-log`) are printed by the early logging-options pre-parse and precede every `--help` output; the command-specific help follows immediately with no blank line in between.

## Doc vs binary discrepancies

None found.
