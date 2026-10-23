# signal-cli sendSyncRequest

## Sources

- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/commands/SendSyncRequestCommand.java (tag v0.14.7)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/lib/src/main/java/org/asamk/signal/manager/helper/SyncHelper.java
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/lib/src/main/java/org/asamk/signal/manager/internal/ManagerImpl.java
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/man/signal-cli.1.adoc section "sendSyncRequest"
- Binary observation (signal-cli 0.14.7, empty scratch `--config <CONFIG>`), commands run:
  - `signal-cli --config <CONFIG> sendSyncRequest --help`
  - `signal-cli --config <CONFIG> sendSyncRequest` (exit 1, `No local users found, you first need to register or link an account`)
  - `signal-cli --config <CONFIG> sendSyncRequest extra` (exit 1, parse error)
  - `signal-cli --config <CONFIG> -a +15555550123 sendSyncRequest` (exit 1, `User +15555550123 is not registered.`)

## Summary

Help line: `Send a synchronization request message to primary device (for group, contacts, ...).`

Man page: "The primary device will respond with synchronization messages with full contact
and group lists." (Responses arrive later and are processed by `receive`/daemon.)

## Invocation category & account requirement

Local single-account command; requires a registered local account (common account errors,
see index.md). Contacts the server.

## Output modes

plain-text (default) and json. No output on success in either mode.

## Arguments

No command-specific arguments besides `-h/--help`. Any extra token is a parse error.

## Constraints & validation

None beyond account resolution.

## Behavior

Sends five sync-request messages to the account's own devices, in this order of request
type: GROUPS, CONTACTS, BLOCKED, CONFIGURATION, KEYS. Then schedules a storage-service sync.
Individual send failures are not reported to the user. Nothing is printed.

## Plain-text output

None.

## JSON output

None.

## Errors

| condition | stream | exact message | exit |
|---|---|---|---|
| extra argument | stderr | `usage: signal-cli sendSyncRequest [-h]` then `signal-cli: error: unrecognized arguments: '<arg>'` | 1 |
| no/unregistered local account | stderr | common account errors (index.md) | 1 |
| IO error (declared; the v0.14.7 operation does not actually raise it) | stderr | `Request sync data error: <io message>` | 3 |

## --help transcript

Command: `signal-cli --config <CONFIG> sendSyncRequest --help` — exit code 0, stderr empty, stdout:

```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli sendSyncRequest [-h]

named arguments:
  -h, --help             show this help message and exit
```

## Doc vs binary discrepancies

None found.
