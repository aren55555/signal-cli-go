# signal-cli rejectCall

## Sources

- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/commands/RejectCallCommand.java (tag v0.14.7)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/lib/src/main/java/org/asamk/signal/manager/helper/CallManager.java (call state machine, error texts)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/lib/src/main/java/org/asamk/signal/manager/api/CallInfo.java (state names)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/lib/src/main/java/org/asamk/signal/manager/util/Utils.java (unsigned call id rendering)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/lib/src/main/java/org/asamk/signal/manager/internal/ManagerImpl.java
- Man page v0.14.7 (man/signal-cli.1.adoc) does not document the call commands.
- Binary observation (signal-cli 0.14.7, empty scratch `--config <CONFIG>`), commands run:
  - `signal-cli --config <CONFIG> rejectCall --help`
  - `signal-cli --config <CONFIG> rejectCall` (exit 1)
  - `signal-cli --config <CONFIG> rejectCall --call-id 1.5` (exit 1)
  - `signal-cli --config <CONFIG> -a +15555550123 rejectCall --call-id 1` (exit 1, `User +15555550123 is not registered.`)

## Summary

Help line: `Reject an incoming voice call.`

## Invocation category & account requirement

Local single-account command; requires a registered local account (common account errors,
see index.md).

## Output modes

plain-text (default) and json.

## Arguments

| names | arity | value type / metavar | default | required | help text (verbatim) | notes |
|---|---|---|---|---|---|---|
| `--call-id` | 1 | signed 64-bit integer, CALL_ID | — | yes | `The call ID to reject.` | negative values accepted; decimal only |

## Constraints & validation

- `--call-id` must parse as a signed 64-bit decimal integer. Any non-integer or out-of-range value
  (e.g. `abc`, `1.5`, `99999999999999999999`) gives
  `signal-cli: error: argument --call-id: could not convert '<v>' to integer (64 bits)`
  (the ` (64 bits)` suffix is always present; the line is justified/wrapped at 75 columns, so the
  suffix usually lands on a second line; see observations).
- (`No call ID given`, exit 1, exists but is unreachable from the CLI since the option is required.)

## Behavior

Requires an active call with that id (any state). Sends a "busy" call message to the peer device and ends the call locally (end reason `rejected`). The send result is not reported.

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

None.

## JSON output

None (nothing written).

## Errors

| condition | stream | exact message | exit |
|---|---|---|---|
| `--call-id` missing | stderr | `usage: signal-cli rejectCall [-h] --call-id CALL_ID` + `signal-cli: error: argument --call-id is required` | 1 |
| `--call-id` not an integer | stderr | usage + `signal-cli: error: argument --call-id: could not convert '<v>' to integer (64 bits)` (justified, wrapped at 75 columns) | 1 |
| `--call-id` out of 64-bit range | stderr | usage + `signal-cli: error: argument --call-id: could not convert '<v>' to integer (64 bits)` (justified, wrapped) | 1 |
| no/unregistered local account | stderr | common account errors (index.md) | 1 |
| no such active call | stderr | `Failed to reject call: No active call with id <callId rendered as UNSIGNED 64-bit decimal>` (e.g. `--call-id -5` -> `18446744073709551611`) | 3 |


## --help transcript

Command: `signal-cli --config <CONFIG> rejectCall --help` — exit code 0, stderr empty, stdout:

```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli rejectCall [-h] --call-id CALL_ID

named arguments:
  -h, --help             show this help message and exit
  --call-id CALL_ID      The call ID to reject.
```

## Doc vs binary discrepancies

Not documented in the man page at v0.14.7; binary is the only reference.
