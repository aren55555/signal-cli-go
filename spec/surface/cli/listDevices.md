# signal-cli listDevices

## Sources

- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/commands/ListDevicesCommand.java (tag v0.14.7)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/util/DateUtils.java (timestamp rendering)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/output/PlainTextWriterImpl.java (2-space indentation)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/man/signal-cli.1.adoc section "listDevices"
- Binary observation (signal-cli 0.14.7):
  - `signal-cli --config <CONFIG> listDevices --help`
  - `signal-cli --config <CONFIG> listDevices` (no accounts)
  - `signal-cli --config <CONFIG> listDevices extra`
  - `signal-cli --config <CONFIG> -a +15555550123 listDevices`

## Summary

Help line (verbatim): `Show a list of linked devices.`

Fetches the account's device list from the server (includes the primary and this device) and prints it.

## Invocation category & account requirement

Local (single-account) command. Requires a registered account: the account given with `-a/--account` (phone number in E.164 form with leading `+`, or an ACI UUID), or, if `-a` is omitted, the single account present in the data directory. The account must exist locally and be registered, otherwise the command fails before doing anything (no network traffic). Also available as a JSON-RPC method of the same name in daemon/jsonRpc mode (not covered here). With `--dbus`/`--dbus-system` the command is forwarded over D-Bus instead (not covered here).

Contacts the server.

## Output modes

`plain-text` and `json`; default `plain-text`.

## Arguments

None besides `-h/--help`.

## Constraints & validation

Extra arguments → `unrecognized arguments` parse error.

## Behavior

Retrieves devices and prints them in server order.

## Plain-text output

For each device, a block:
```
- Device <ID>[ (this device)]:
  Name: <NAME>
  Created: <CREATED>
  Last seen: <LASTSEEN>
```
- ` (this device)` is appended only for the device running signal-cli.
- Indentation is exactly 2 spaces.
- `<NAME>` is the decrypted device name; if the device has no name the literal `null` is printed.
- `<CREATED>` / `<LASTSEEN>` timestamp format: `<epoch-millis> (<yyyy-MM-dd>T<HH:mm:ss.SSS>Z)`, the parenthesised part being the same instant in UTC, e.g. `1700000000000 (2023-11-14T22:13:20.000Z)`.
- No blank lines between blocks.

## JSON output

One line: JSON array of objects, in this key order:
- `id` — integer (device id)
- `name` — string or `null` (key always present)
- `createdTimestamp` — integer, epoch milliseconds
- `lastSeenTimestamp` — integer, epoch milliseconds

There is no "this device" marker in JSON.

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
| Server / I/O failure | stderr | `Failed to get linked devices: <detail>` | 3 |

Observed: no accounts → `No local users found, you first need to register or link an account`, exit 1; `-a +15555550123 listDevices` → `User +15555550123 is not registered.`, exit 1.

## --help transcript

Command: `signal-cli --config <CONFIG> listDevices --help` — exit code 0, stderr empty, stdout:

```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli listDevices [-h]

named arguments:
  -h, --help             show this help message and exit
```

The first 7 lines (the short usage of only `-h`, `-v`, `--log-file`, `--scrub-log`) are printed by the early logging-options pre-parse and precede every `--help` output; the command-specific help follows immediately with no blank line in between.

## Doc vs binary discrepancies

None found.
