# Method `unblock`

## Sources

- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/commands/UnblockCommand.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/lib/src/main/java/org/asamk/signal/manager/internal/ManagerImpl.java
- Binary observation: none. Conventions: [index.md](index.md#parameter-conventions).

## Summary

Inverse of [block](block.md): unblocks contacts and/or groups, syncing an
"unblock and accept" message request response for each changed item, then the
blocked list and storage service. No profile-key rotation.

CLI equivalent: `signal-cli -a ACCOUNT unblock [RECIPIENT ...] [-g GROUP_ID ...]`.

Account requirement: **single**.

## Params

Identical to [block](block.md#params): `recipient`/`recipients` and
`group-id`/`groupId`/`group-ids`/`groupIds`, both optional lists.

## Result

`{}`.

## Errors

Same as [block](block.md#errors), except the I/O error message is
`Failed to sync unblock to linked devices: <msg> (UnexpectedErrorException)`
(code -32603). Unknown group ids are only logged.
