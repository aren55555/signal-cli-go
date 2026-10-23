# signal-cli sendAdminDelete

## Sources

- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/commands/SendAdminDeleteCommand.java (tag v0.14.7)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/util/CommandUtil.java
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/util/SendMessageResultUtils.java
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/lib/src/main/java/org/asamk/signal/manager/Manager.java
- man page https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/man/signal-cli.1.adoc section `=== sendAdminDelete`
- Binary commands run (`--config <CONFIG>` empty scratch dir):
  - `signal-cli --config <CONFIG> sendAdminDelete --help`
  - `signal-cli --config <CONFIG> sendAdminDelete`
  - `signal-cli --config <CONFIG> sendAdminDelete -t 1`
  - `signal-cli --config <CONFIG> sendAdminDelete -a +15555550123`
  - `signal-cli --config <CONFIG> sendAdminDelete -a +15555550123 -t 1 -g`
  - `signal-cli --config <CONFIG> sendAdminDelete -a +15555550123 -t x -g abc`
  - `signal-cli --config <CONFIG> sendAdminDelete -a +15555550123 -t 1`
- Shared result format: [_send-results.md](_send-results.md).

## Summary

Help line: `Send admin delete message for a previously received or sent message.`

As a group admin, deletes a message (or story) authored by someone (identified
by author + timestamp) for all members of the given group(s). Group recipients only.

## Invocation category & account requirement

Local single-account command; requires a registered account (see index.md).
Contacts the Signal server.

## Output modes

`plain-text` (default) and `json`.

## Arguments

| Names | Arity | Value | Default | Required | Help text (verbatim) | Notes |
|---|---|---|---|---|---|---|
| `-g`, `--group-id`, `--group` | 1..n | GROUP_ID | none | not at parse time; enforced after | Specify the recipient group ID. | If the flag is given it needs at least one value |
| `--notify-self` | 0 | flag | false | no | If self is part of recipients/groups send a normal message, not a sync message. | |
| `-a`, `--target-author` | 1 | TARGET_AUTHOR | none | yes | Specify the number of the author of the message to admin delete. | recipient syntax (number, UUID, `PNI:`, `u:`) |
| `-t`, `--target-timestamp` | 1 | int64 | none | yes | Specify the timestamp of the message to admin delete. | |
| `--story` | 0 | flag | false | no | Admin delete a story instead of a normal message | |
| `-h`, `--help` | 0 | | | | show this help message and exit | |

No positional arguments.

## Constraints & validation

Order (after account loading):
1. Group ids decoded (`Invalid group id: Failed to decode groupId (must be base64) "<id>": <cause>`, exit 1).
2. No group given -> `Admin delete requires group IDs` (exit 1).
3. Target author parsed as recipient (`Invalid phone number '<s>': <reason>`, exit 1).

## Behavior

Sends an admin-delete for (author, timestamp, story flag) to each group.

## Plain-text output

See [_send-results.md](_send-results.md) (failure block, then timestamp line).

## JSON output

See [_send-results.md](_send-results.md); result entries carry `groupId`.

## Errors

| Condition | Stream | Exact message | Exit |
|---|---|---|---|
| missing `-a` (parse) | stderr | usage + `signal-cli: error: argument -a/--target-author is required` (observed) | 1 |
| missing `-t` (parse) | stderr | usage + `signal-cli: error: argument -t/--target-timestamp is required` (observed) | 1 |
| `-g` without value (parse) | stderr | usage + `signal-cli: error: argument -g/--group-id/--group: expected 1 argument(s)` (observed) | 1 |
| non-integer `-t` (parse) | stderr | usage + `signal-cli: error: argument  -t/--target-timestamp:  could  not convert 'x'` / `to integer (64 bits)` (observed) | 1 |
| no account | stderr | `No local users found, you first need to register or link an account` (observed) | 1 |
| no group | stderr | `Admin delete requires group IDs` | 1 |
| invalid group id / author | stderr | see above | 1 |
| group not found / not member / not allowed | stderr | `Group not found: <id>` / `User is not a member in group: <name> (<id>)` / `User is not allowed to send message to group: <name> (<id>)` | 1 |
| unregistered target | stderr | `The user <identifier> is not registered.` | 1 |
| I/O error | stderr | `Failed to send message: <msg> (<ExceptionName>)` | 2 |
| all sends failed | stderr (after stdout result) | per [_send-results.md](_send-results.md) | 4/5/1 |

## --help transcript

Command: `signal-cli --config <CONFIG> sendAdminDelete --help` -- exit code: 0, stderr empty. Stdout:

```text
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli sendAdminDelete [-h] [-g GROUP_ID [GROUP_ID ...]]
                  [--notify-self] -a TARGET_AUTHOR -t TARGET_TIMESTAMP
                  [--story]

named arguments:
  -h, --help             show this help message and exit
  -g GROUP_ID [GROUP_ID ...], --group-id GROUP_ID [GROUP_ID ...], --group GROUP_ID [GROUP_ID ...]
                         Specify the recipient group ID.
  --notify-self          If  self  is  part  of  recipients/groups  send  a
                         normal message, not a sync message.
  -a TARGET_AUTHOR, --target-author TARGET_AUTHOR
                         Specify the number of  the  author  of the message
                         to admin delete.
  -t TARGET_TIMESTAMP, --target-timestamp TARGET_TIMESTAMP
                         Specify the  timestamp  of  the  message  to admin
                         delete.
  --story                Admin delete a story instead of a normal message
```

## Doc vs binary discrepancies

- Man page says `-g` is "(required)"; the parser does not mark it required, the
  check happens after account loading with message `Admin delete requires group IDs`.
