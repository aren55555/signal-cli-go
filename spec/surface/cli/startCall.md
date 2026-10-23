# signal-cli startCall

## Sources

- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/commands/StartCallCommand.java (tag v0.14.7)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/lib/src/main/java/org/asamk/signal/manager/helper/CallManager.java (call state machine, error texts)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/lib/src/main/java/org/asamk/signal/manager/api/CallInfo.java (state names)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/lib/src/main/java/org/asamk/signal/manager/util/Utils.java (unsigned call id rendering)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/lib/src/main/java/org/asamk/signal/manager/internal/ManagerImpl.java
- Man page v0.14.7 (man/signal-cli.1.adoc) does not document the call commands.
- Binary observation (signal-cli 0.14.7, empty scratch `--config <CONFIG>`), commands run:
  - `signal-cli --config <CONFIG> startCall --help`
  - `signal-cli --config <CONFIG> startCall` (exit 1)
  - `signal-cli --config <CONFIG> startCall +15555550199 +15555550198` (exit 1)
  - `signal-cli --config <CONFIG> -a +15555550123 startCall +15555550199` (exit 1, `User +15555550123 is not registered.`)

## Summary

Help line: `Start an outgoing voice call.`

Creates a new outgoing 1:1 voice call (random call id), sends the call offer and prints the
call info.

## Invocation category & account requirement

Local single-account command; requires a registered local account (common account errors,
see index.md). Contacts the server (TURN info, call signalling).

## Output modes

plain-text (default) and json.

## Arguments

| names | arity | value type / metavar | default | required | help text (verbatim) | notes |
|---|---|---|---|---|---|---|
| `recipient` (positional) | exactly 1 | string, RECIPIENT | — | yes | `Specify the recipient's phone number or UUID.` | any recipient form; a second positional is an "unrecognized arguments" error |

## Constraints & validation

- Invalid phone number -> `Invalid phone number '<input>': <reason>` (exit 1).
- (`No recipient given`, exit 1, exists for an empty recipient list; unreachable from the CLI
  because the parser requires exactly one.)

## Behavior

Resolves the recipient, generates a random 64-bit call id, registers the call in state
`RINGING_OUTGOING`, starts the media helper, fetches TURN servers (failure only logged), sends
the offer, and schedules a ring timeout. Prints the call info immediately (does not wait for
the callee). If the media helper cannot be started the call is ended internally (logged), but
the command still prints the info and exits 0.

### Call lifetime quirk

Active calls live only in the memory of the running signal-cli process. A one-shot CLI
invocation starts with no active calls and ends them when the process exits. Hence the call
commands are only meaningful through `daemon`/`jsonRpc` (same method names over JSON-RPC);
from a plain one-shot CLI invocation on a registered account, `acceptCall`, `hangupCall` and
`rejectCall` always fail with `No active call with id ...` (exit 3) and `listCalls` always
reports no calls. Media is handled by an external helper process (located via the
`SIGNAL_CALL_TUNNEL_BIN` environment variable or next to the installation); that subsystem is
out of scope for this CLI-surface spec.

Call states (enum names, printed verbatim): `IDLE`, `RINGING_INCOMING`, `RINGING_OUTGOING`,
`CONNECTING`, `CONNECTED`, `RECONNECTING`, `ENDED`.

Call ids are 64-bit values; the CLI prints and parses them as **signed** 64-bit decimal
integers (negative values occur), while some error messages render them as **unsigned**
decimal (see Errors).

## Plain-text output

Exactly five lines (the indentation is two literal spaces):

```
Call started:
  Call ID: <callId, signed decimal>
  State: <state>
  Input device: <name or null>
  Output device: <name or null>
```

Device names are not known at start, so they are typically printed as the literal `null`.

## JSON output

One line, object with keys in this order:

| key | type | value |
|---|---|---|
| `callId` | integer (signed 64-bit) | |
| `state` | string | state name, normally `RINGING_OUTGOING` |
| `inputDeviceName` | string or null | |
| `outputDeviceName` | string or null | |
| `codec` | string | always `"opus"` |
| `sampleRate` | integer | always `48000` |
| `channels` | integer | always `1` |
| `ptimeMs` | integer | always `20` |

## Errors

| condition | stream | exact message | exit |
|---|---|---|---|
| missing recipient | stderr | `usage: signal-cli startCall [-h] recipient` + `signal-cli: error: too few arguments` | 1 |
| more than one recipient | stderr | usage + `signal-cli: error: unrecognized arguments: '<extra>'` | 1 |
| no/unregistered local account | stderr | common account errors (index.md) | 1 |
| invalid recipient | stderr | `Invalid phone number '<input>': <reason>` | 1 |
| recipient not registered | stderr | `Recipient not registered: <detail>` | 1 |
| IO/network error | stderr | `Failed to start call: <io message>` | 3 |

## --help transcript

Command: `signal-cli --config <CONFIG> startCall --help` — exit code 0, stderr empty, stdout:

```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli startCall [-h] recipient

positional arguments:
  recipient              Specify the recipient's phone number or UUID.

named arguments:
  -h, --help             show this help message and exit
```

## Doc vs binary discrepancies

Not documented in the man page at v0.14.7; binary is the only reference.
