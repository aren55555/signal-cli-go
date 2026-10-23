# signal-cli quitGroup

## Sources
- Shared send-result formats: [_send-results.md](_send-results.md)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/commands/QuitGroupCommand.java
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/util/SendMessageResultUtils.java (group-results output)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/util/CommandUtil.java
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/man/signal-cli.1.adoc, section `=== quitGroup`
- Binary observations:
  - `signal-cli --config <CONFIG> quitGroup --help`
  - `signal-cli --config <CONFIG> quitGroup`
  - `signal-cli --config <CONFIG> -a +15555550123 quitGroup -g abc`

## Summary
Help text: "Send a quit group message to all group members and remove self from member list."

- If the user is only a pending member, quitting declines the invitation (man page).
- `--delete` additionally removes all local data for the group.

## Invocation category & account requirement
This is a single-account local command and needs a registered account.

## Output modes
`plain-text` (the default) and `json`.

## Arguments

| names | arity | value type / metavar | default | required | help text (verbatim) | notes |
|---|---|---|---|---|---|---|
| `-h`, `--help` | 0 | – | – | no | `show this help message and exit` | |
| `-g`, `--group-id`, `--group` | 1 | base64 / `GROUP_ID` | – | **yes** | `Specify the recipient group ID.` | |
| `--delete` | 0 | flag | false | no | `Delete local group data completely after quitting group.` | |
| `--admin` | 0..n | recipients / `ADMIN` | – | no | `Specify one or more members to make a group admin, required if you're currently the only admin.` | |

## Constraints & validation
- A missing `-g` gives the parse error `argument -g/--group-id/--group is required` (exit 1, observed).
- An invalid group id gives `Invalid group id: Failed to decode groupId (must be base64) "<id>": <cause>` (exit 1). This check runs after the account loads.
- An invalid `--admin` recipient gives `Invalid phone number '<s>': <reason>` (exit 1).

## Behavior
1. Promote the `--admin` members, if any, and leave the group, sending the update to the members.
2. If the account is not a member of the group, nothing is sent and no output is printed. Only an info log line, "User is not a group member", appears, and it is shown only with `-v`. Processing continues.
3. With `--delete`, local group data is deleted afterwards. This happens even when the account was not a member.

## Plain-text output
This applies only when the quit message was sent.
- If any recipient failed: `Failed to send (some) messages:`, followed by one line per error. The formats are as in `send`, for example `Network failure for "<id>"`.
- Then the timestamp as a bare integer.

## JSON output
`{"timestamp": <integer>, "results": [SendMessageResult...]}`
- In each SendMessageResult, the `groupId` key is absent.
- Key order is not stable: the object is built from an immutable map whose iteration order varies between JVM runs.
- When the account was not a member, nothing is printed.

## Errors

| condition | stream | exact message | exit code |
|---|---|---|---|
| `-g` missing | stderr | usage + `signal-cli: error: argument -g/--group-id/--group is required` | 1 |
| No account / not registered (observed) | stderr | `User +15555550123 is not registered.` | 1 |
| Invalid group id | stderr | `Invalid group id: Failed to decode groupId (must be base64) "<id>": <cause>` | 1 |
| Group not found locally | stderr | `Failed to send to group: <msg>` | 1 |
| Last admin and no `--admin` | stderr | `You need to specify a new admin with --admin: <msg>` | 1 |
| An `--admin` member is unregistered | stderr | `The user <identifier> is not registered.` | 1 |
| Network or I/O error | stderr | `Failed to send message: <msg> (<ExceptionClass>)` | 3 |

Failures to send to individual members do not change the exit code: it stays 0.

## --help transcript
Command: `signal-cli --config <CONFIG> quitGroup --help`

exit code: 0

stdout:
```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli quitGroup [-h] -g GROUP_ID [--delete]
                  [--admin [ADMIN [ADMIN ...]]]

named arguments:
  -h, --help             show this help message and exit
  -g GROUP_ID, --group-id GROUP_ID, --group GROUP_ID
                         Specify the recipient group ID.
  --delete               Delete local group data  completely after quitting
                         group.
  --admin [ADMIN [ADMIN ...]]
                         Specify one  or  more  members  to  make  a  group
                         admin,  required  if  you're  currently  the  only
                         admin.
```

stderr:
```
```


## Observed invocations
### `signal-cli --config <CONFIG> quitGroup`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli quitGroup [-h] -g GROUP_ID [--delete]
                  [--admin [ADMIN [ADMIN ...]]]
signal-cli: error: argument -g/--group-id/--group is required
```

### `signal-cli --config <CONFIG> quitGroup`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli quitGroup [-h] -g GROUP_ID [--delete]
                  [--admin [ADMIN [ADMIN ...]]]
signal-cli: error: argument -g/--group-id/--group is required
```

### `signal-cli --config <CONFIG> -a +15555550123 quitGroup -g abc`

exit code: 1

stdout:
```
```

stderr:
```
User +15555550123 is not registered.
```


## Doc vs binary discrepancies
The man page does not document `--admin`. The binary has it.
