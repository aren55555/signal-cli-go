# signal-cli updateDevice

## Sources

- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/commands/UpdateDeviceCommand.java (tag v0.14.7)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/man/signal-cli.1.adoc section "updateDevice"
- Binary observation (signal-cli 0.14.7):
  - `signal-cli --config <CONFIG> updateDevice --help`
  - `signal-cli --config <CONFIG> updateDevice` (no args)
  - `signal-cli --config <CONFIG> updateDevice -d 2`, `... updateDevice -n x`, `... updateDevice -d x -n y`
  - `signal-cli --config <CONFIG> -a +15555550123 updateDevice -d 2 -n y`

## Summary

Help line (verbatim): `Update a linked device.`

Renames a linked device (sets its encrypted device name on the server). Primary device only.

## Invocation category & account requirement

Local (single-account) command. Requires a registered account: the account given with `-a/--account` (phone number in E.164 form with leading `+`, or an ACI UUID), or, if `-a` is omitted, the single account present in the data directory. The account must exist locally and be registered, otherwise the command fails before doing anything (no network traffic). Also available as a JSON-RPC method of the same name in daemon/jsonRpc mode (not covered here). With `--dbus`/`--dbus-system` the command is forwarded over D-Bus instead (not covered here).

Contacts the server.

## Output modes

`plain-text` and `json`; default `plain-text`. Nothing printed on success.

## Arguments

| Names | Arity | Value type / metavar | Default | Required | Help text (verbatim) | Notes |
|---|---|---|---|---|---|---|
| `-d`, `--device-id`, `--deviceId` | 1 | 32-bit signed integer, `DEVICE_ID` | — | yes | `Specify the device you want to update. Use listDevices to see the deviceIds.` | |
| `-n`, `--device-name` | 1 | string, `DEVICE_NAME` | — | yes | `Specify a name to describe the given device.` | |

## Constraints & validation

- Missing `-n` → `signal-cli: error: argument -n/--device-name is required`; missing `-d` → `signal-cli: error: argument -d/--device-id/--deviceId is required` (both observed). When both are missing (observed, `updateDevice` with no arguments) the error names `-n/--device-name` (`signal-cli: error: argument -n/--device-name is required`), i.e. not the first-declared option.
- Non-integer `-d` → `signal-cli: error: argument -d/--device-id/--deviceId: could not convert '<value>' to integer (32 bits)` (observed).

## Behavior

Updates the device name. Exit 0, no output.

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
| Server / I/O failure | stderr | `Error while updating device: <detail>` | 3 |

## --help transcript

Command: `signal-cli --config <CONFIG> updateDevice --help` — exit code 0, stderr empty, stdout:

```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli updateDevice [-h] -d DEVICE_ID -n DEVICE_NAME

named arguments:
  -h, --help             show this help message and exit
  -d DEVICE_ID, --device-id DEVICE_ID, --deviceId DEVICE_ID
                         Specify  the  device  you   want  to  update.  Use
                         listDevices to see the deviceIds.
  -n DEVICE_NAME, --device-name DEVICE_NAME
                         Specify a name to describe the given device.
```

The first 7 lines (the short usage of only `-h`, `-v`, `--log-file`, `--scrub-log`) are printed by the early logging-options pre-parse and precede every `--help` output; the command-specific help follows immediately with no blank line in between.

## Doc vs binary discrepancies

- Man page does not mention the `--deviceId` alias.
