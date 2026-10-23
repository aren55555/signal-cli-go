# signal-cli listCalls

## Sources

- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/commands/ListCallsCommand.java (tag v0.14.7)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/output/PlainTextWriterImpl.java (indentation)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/lib/src/main/java/org/asamk/signal/manager/helper/CallManager.java
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/lib/src/main/java/org/asamk/signal/manager/api/CallInfo.java
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/lib/src/main/java/org/asamk/signal/manager/api/RecipientAddress.java (recipient rendering)
- Man page v0.14.7 does not document this command.
- Binary observation (signal-cli 0.14.7, empty scratch `--config <CONFIG>`), commands run:
  - `signal-cli --config <CONFIG> listCalls --help`
  - `signal-cli --config <CONFIG> listCalls` (exit 1, `No local users found, you first need to register or link an account`)
  - `signal-cli --config <CONFIG> listCalls extra` (exit 1, `unrecognized arguments: 'extra'`)
  - `signal-cli --config <CONFIG> -a +15555550123 listCalls` (exit 1, `User +15555550123 is not registered.`)

## Summary

Help line: `List active voice calls.`

## Invocation category & account requirement

Local single-account command; requires a registered local account (common account errors,
see index.md). Purely local (in-process state).

## Output modes

plain-text (default) and json.

## Arguments

No command-specific arguments besides `-h/--help`.

## Constraints & validation

None.

## Behavior

Lists the calls currently tracked by this process. Because call state lives only in process
memory, a one-shot CLI invocation always has no calls: plain-text prints `No active calls.`
and json prints `[]`. Non-empty results are only possible via daemon/jsonRpc. Call states:
`IDLE`, `RINGING_INCOMING`, `RINGING_OUTGOING`, `CONNECTING`, `CONNECTED`, `RECONNECTING`, `ENDED`.

## Plain-text output

No calls: exactly one line `No active calls.`

Otherwise, per call (indent = two spaces):

```
- Call <callId, signed decimal>:
  State: <state>
  Recipient: <recipient rendering>
  Direction: <outgoing|incoming>
  Input device: <name>      (line omitted when unknown)
  Output device: <name>     (line omitted when unknown)
```

`<recipient rendering>` is the platform's default record rendering of the recipient address,
i.e. `RecipientAddress[aci=Optional[<uuid>], pni=Optional.empty, number=Optional[<+E164>], username=Optional.empty]`
(each component either `Optional[<value>]` or `Optional.empty`). This is an upstream
artefact; a reimplementation that wants byte-identical output must reproduce it.

## JSON output

One line: JSON array (`[]` when empty). Each element, keys in this order:

| key | type | notes |
|---|---|---|
| `callId` | integer (signed 64-bit) | |
| `state` | string | state name |
| `number` | string or null | recipient phone number |
| `uuid` | string or null | recipient ACI |
| `isOutgoing` | boolean | |
| `inputDeviceName` | string or null | |
| `outputDeviceName` | string or null | |

Null values are emitted, not omitted.

## Errors

| condition | stream | exact message | exit |
|---|---|---|---|
| extra argument | stderr | `usage: signal-cli listCalls [-h]` + `signal-cli: error: unrecognized arguments: '<arg>'` | 1 |
| no/unregistered local account | stderr | common account errors (index.md) | 1 |

## --help transcript

Command: `signal-cli --config <CONFIG> listCalls --help` — exit code 0, stderr empty, stdout:

```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli listCalls [-h]

named arguments:
  -h, --help             show this help message and exit
```

## Doc vs binary discrepancies

Not documented in the man page at v0.14.7.
