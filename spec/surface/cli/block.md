# signal-cli block

## Sources

- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/commands/BlockCommand.java (tag v0.14.7)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/util/CommandUtil.java
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/lib/src/main/java/org/asamk/signal/manager/internal/ManagerImpl.java (blocking behaviour)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/man/signal-cli.1.adoc section "block"
- Binary observation (signal-cli 0.14.7, `--config <CONFIG>` = empty scratch dir), commands run:
  - `signal-cli --config <CONFIG> block --help`
  - `signal-cli --config <CONFIG> block` (exit 1, `No local users found, you first need to register or link an account`)
  - `signal-cli --config <CONFIG> -a +15555550123 block +15555550199` (exit 1, `User +15555550123 is not registered.`)
  - `signal-cli --config <CONFIG> -a +15555550123 block` (exit 1, `User +15555550123 is not registered.`)

## Summary

Help line: `Block the given contacts or groups (no messages will be received)`

Marks the given contacts and/or groups as blocked on this account, informs linked devices
and storage service.

## Invocation category & account requirement

Local single-account command; requires a registered local account (common account
resolution errors, see index.md). Contacts the server (sync messages to own linked devices,
possibly profile-key rotation and storage-service sync).

## Output modes

plain-text (default) and json. No output on success in either mode.

## Arguments

| names | arity | value type / metavar | default | required | help text (verbatim) | notes |
|---|---|---|---|---|---|---|
| `recipient` (positional) | 0..n (`*`) | string, RECIPIENT | empty list | no | `Contact number` | Any recipient form (phone number, ACI, `PNI:`UUID, `u:`username). |
| `-g`, `--group-id`, `--group` | 0..n (`*`) | string (base64 group id), GROUP_ID | none | no | `Group ID` | May be given with zero values. |

No mutual exclusion. Giving neither recipients nor groups is accepted and is a successful no-op (exit 0).

## Constraints & validation

- Each recipient string is parsed; invalid phone numbers fail with
  `Invalid phone number '<input>': <reason>` (exit 1) before anything is changed.
- Each group id must be valid base64 of a group id, else `Invalid group id: <reason>` (exit 1).
  Group ids are validated only after the contacts have already been processed.

## Behavior

1. Contacts: if the recipient list is non-empty, each recipient is resolved (unresolvable
   user -> `The user <identifier> is not registered.`, exit 1). For every recipient whose
   blocked state actually changes: the local contact is marked blocked and a message-request-response
   sync message of type "block" is sent to own linked devices. If a newly blocked contact shares no group with the account, the account's profile key is rotated. Then the full blocked list is synced to linked devices and a storage-service sync is scheduled.
2. Groups: same, per group id (blocked flag + message-request-response sync; blocking any group causes profile-key rotation), followed by blocked-list sync and storage sync.
   An unknown group id does NOT fail the command: a warning is logged to stderr
   (`WARN  BlockCommand - Unknown group id: <detail>` in default log format) and exit code stays 0.
3. The message `This command doesn't work on linked devices.` (exit 1) exists for a "not primary device" condition, but in v0.14.7 the underlying operation never raises that condition, so the command also runs on linked devices (source-derived; not observable without an account).

## Plain-text output

None.

## JSON output

None.

## Errors

| condition | stream | exact message | exit |
|---|---|---|---|
| no/unregistered local account | stderr | common account errors (index.md) | 1 |
| invalid recipient number | stderr | `Invalid phone number '<input>': <reason>` | 1 |
| recipient not registered | stderr | `The user <identifier> is not registered.` | 1 |
| invalid group id | stderr | `Invalid group id: <reason>` | 1 |
| linked (non-primary) device (unreachable in v0.14.7) | stderr | `This command doesn't work on linked devices.` | 1 |
| network/IO failure syncing | stderr | `Failed to sync block to linked devices: <io message>` | 2 |
| unknown group id | stderr (log) | warning `Unknown group id: <detail>` | 0 |

## --help transcript

Command: `signal-cli --config <CONFIG> block --help` — exit code 0, stderr empty, stdout:

```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli block [-h] [-g [GROUP_ID [GROUP_ID ...]]]
                  [recipient [recipient ...]]

positional arguments:
  recipient              Contact number

named arguments:
  -h, --help             show this help message and exit
  -g [GROUP_ID [GROUP_ID ...]], --group-id [GROUP_ID [GROUP_ID ...]], --group [GROUP_ID [GROUP_ID ...]]
                         Group ID
```

## Doc vs binary discrepancies

- Man page says the change is "only local but can be synchronized ... by using sendContacts".
  Source behaviour: the command itself already sends blocked-list and message-request sync
  messages to linked devices and triggers storage sync.
- Man page does not list the `--group` alias; binary accepts it.
- An IO failure maps to exit code 2 ("unexpected"), not 3.
