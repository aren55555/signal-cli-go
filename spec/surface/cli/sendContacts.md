# signal-cli sendContacts

## Sources

- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/commands/SendContactsCommand.java (tag v0.14.7)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/lib/src/main/java/org/asamk/signal/manager/helper/SyncHelper.java (contact sync behaviour)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/man/signal-cli.1.adoc section "sendContacts"
- Binary observation (signal-cli 0.14.7, empty scratch `--config <CONFIG>`), commands run:
  - `signal-cli --config <CONFIG> sendContacts --help`
  - `signal-cli --config <CONFIG> sendContacts` (exit 1, `No local users found, you first need to register or link an account`)
  - `signal-cli --config <CONFIG> sendContacts extra` (exit 1, parse error)
  - `signal-cli --config <CONFIG> -a +15555550123 sendContacts` (exit 1, `User +15555550123 is not registered.`)

## Summary

Help line: `Send a synchronization message with the local contacts list to all linked devices.`

Man page: "This command should only be used if this is the primary device."

## Invocation category & account requirement

Local single-account command; requires a registered local account (common account
errors, see index.md). Contacts the server (attachment upload + sync message to own devices).

## Output modes

plain-text (default) and json. No output on success in either mode.

## Arguments

No command-specific arguments besides `-h/--help`. Any extra token is a parse error.

## Constraints & validation

None beyond account resolution.

## Behavior

Builds a contacts-sync blob containing every locally stored contact (identifier: ACI and/or
phone number, plus name, avatar, etc.) and, if the account has a profile key, an entry for the
account itself. If the blob is non-empty it is uploaded as an attachment and sent as a
"contacts" sync message (marked complete) to the account's own linked devices. Nothing is
printed. The command does not check whether this device is the primary; it is the user's
responsibility (per man page).

## Plain-text output

None.

## JSON output

None.

## Errors

| condition | stream | exact message | exit |
|---|---|---|---|
| extra argument | stderr | `usage: signal-cli sendContacts [-h]` then `signal-cli: error: unrecognized arguments: '<arg>'` | 1 |
| no/unregistered local account | stderr | common account errors (index.md) | 1 |
| IO/network error | stderr | `SendContacts error: <io message>` | 3 |

## --help transcript

Command: `signal-cli --config <CONFIG> sendContacts --help` — exit code 0, stderr empty, stdout:

```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli sendContacts [-h]

named arguments:
  -h, --help             show this help message and exit
```

## Doc vs binary discrepancies

None found.
