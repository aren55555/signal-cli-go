# signal-cli joinGroup

## Sources
- Shared send-result formats: [_send-results.md](_send-results.md)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/commands/JoinGroupCommand.java
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/util/SendMessageResultUtils.java and json/JsonSendMessageResult.java (result rendering)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/lib/src/main/java/org/asamk/signal/manager/api/GroupInviteLinkUrl.java (link parsing errors)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/man/signal-cli.1.adoc, section `=== joinGroup`
- Binary observations:
  - `signal-cli --config <CONFIG> joinGroup --help`
  - `signal-cli --config <CONFIG> joinGroup`
  - `signal-cli --config <CONFIG> joinGroup --uri`
  - `signal-cli --config <CONFIG> -a +15555550123 joinGroup --uri https://signal.group/#x`
- `jshell` (JDK 25) was used to confirm that the key order of the JSON result map varies between JVM runs.

## Summary
Help text: "Join a group via an invitation link."

The command joins a Groups-v2 group using an invite link. If the group requires admin approval, a join request is sent instead.

## Invocation category & account requirement
This is a single-account local command and needs a registered account. The usual `-a` resolution applies (see index.md).

## Output modes
`plain-text` (the default) and `json`.

## Arguments

| names | arity | value type / metavar | default | required | help text (verbatim) | notes |
|---|---|---|---|---|---|---|
| `-h`, `--help` | 0 | – | – | no | `show this help message and exit` | |
| `--uri` | 1 | string / `URI` | – | **yes** | `Specify the uri with the group invitation link.` | Form: `https://signal.group/#<base64url>`. The `sgnl` scheme with host `signal.group` is also accepted. |

## Constraints & validation
- A missing `--uri` gives the parse error `argument --uri is required` (exit 1).
- The link is validated after the account loads:
  - The scheme is not `https` or `sgnl` (case-insensitive), the host is not `signal.group`, or the URI is unparseable: `Link is not a signal group invitation link` (exit 1).
  - The URI has a path other than `/` or empty: `Group link is invalid: No path was expected in uri` (exit 1).
  - The fragment is missing or empty: `Group link is invalid: No reference was in the uri` (exit 1).
  - The fragment fails to decode: `Group link is invalid: <decoder message>` (exit 1).
  - The decoded link has no v1 contents: `Group link was created with an incompatible version: Url contains no known group link content` (exit 1).

## Behavior
- Fetches the group's join information from the server, then either joins the group or requests to join it.
- Sends the group update to the members.

## Plain-text output
1. One of these lines:
   - `Joined group "<base64 groupId>"` if the account is now a full member.
   - `Requested to join group "<base64 groupId>"` if only a join request is pending. The double quotes are ASCII.
2. If any recipient failed:
   - `Failed to send (some) messages:`
   - Then one line per failure. The failure formats are shared with `send`:
     - `Network failure for "<id>"`
     - `Rate limit failure for "<id>"`, optionally followed by `, retry after <n> seconds`
     - `Unregistered user "<id>"`
     - `Untrusted Identity for "<id>"`
     - A multi-line CAPTCHA proof-required text.
3. The send timestamp, as a bare integer, on its own line.

## JSON output
A single object:
- `timestamp`: integer.
- `results`: an array of SendMessageResult objects.
- `groupId`: string (base64).
- `onlyRequested`: `true`. This key is present only when the join is still pending admin approval; otherwise it is absent.

**Key order is not stable.** The object is built from an immutable map whose iteration order is randomized per JVM run, so key order can change between runs. Clients must not depend on it.

SendMessageResult has these keys:
- `recipientAddress`: `{uuid, number, username}`, each a string or null.
- `groupId`: omit-null. Absent here.
- `type`: one of `SUCCESS`, `NETWORK_FAILURE`, `UNREGISTERED_FAILURE`, `IDENTITY_FAILURE`, `RATE_LIMIT_FAILURE`, `INVALID_PRE_KEY_FAILURE`.
- `token`: omit-null. Present for proof-required failures.
- `retryAfterSeconds`: omit-null.

## Errors

| condition | stream | exact message | exit code |
|---|---|---|---|
| `--uri` missing or has no value | stderr | usage + `signal-cli: error: argument --uri is required` / `... argument --uri: expected one argument` | 1 |
| No account / invalid account / not registered | stderr | see index.md (observed: `User +15555550123 is not registered.`) | 1 |
| Not a group link | stderr | `Link is not a signal group invitation link` | 1 |
| Malformed link | stderr | `Group link is invalid: <detail>` | 1 |
| Unknown link version | stderr | `Group link was created with an incompatible version: <detail>` | 1 |
| Link inactive or reset on the server | stderr | `Group link is not valid: <detail>` | 1 |
| Pending admin approval error | stderr | `Pending admin approval: <detail>` | 1 |
| Network or I/O error | stderr | `Failed to send message: <msg> (<ExceptionClass>)` | 3 |

A failure to send to individual members does **not** change the exit code: it stays 0.

## --help transcript
Command: `signal-cli --config <CONFIG> joinGroup --help`

exit code: 0

stdout:
```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli joinGroup [-h] --uri URI

named arguments:
  -h, --help             show this help message and exit
  --uri URI              Specify the uri with the group invitation link.
```

stderr:
```
```


## Observed invocations
### `signal-cli --config <CONFIG> joinGroup`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli joinGroup [-h] --uri URI
signal-cli: error: argument --uri is required
```

### `signal-cli --config <CONFIG> joinGroup --uri`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli joinGroup [-h] --uri URI
signal-cli: error: argument --uri: expected one argument
```

### `signal-cli --config <CONFIG> -a +15555550123 joinGroup --uri https://signal.group/#x`

exit code: 1

stdout:
```
```

stderr:
```
User +15555550123 is not registered.
```


## Doc vs binary discrepancies
None found. The man page only says that the URI "starts with `https://signal.group/#`".
