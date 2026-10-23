# Method `getAttachment`

## Sources

- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/commands/GetAttachmentCommand.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/json/JsonAttachmentData.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/lib/src/main/java/org/asamk/signal/manager/internal/ManagerImpl.java
- Binary observation: none. Conventions: [index.md](index.md#parameter-conventions).

## Summary

Returns an already-downloaded attachment file, base64 encoded. The attachment
id is the `id` reported in received-message attachment objects (the stored
file name).

CLI equivalent: `signal-cli -a ACCOUNT getAttachment --id ID (--recipient R | -g GROUP_ID)`.

Account requirement: **single**.

## Params

| JSON key(s) | Type | Req. | Default | CLI flag | Notes |
|---|---|---|---|---|---|
| `id` | string | yes | — | `--id` | Attachment id. |
| `recipient` | string | no | — | `--recipient` | Required by the CLI (mutually exclusive with group-id) but **not read** at all; JSON-RPC callers may omit it. |
| `group-id` / `groupId` | string | no | — | `-g/--group-id` | Same: not read. |

## Result

`{ "data": "<standard base64 of the file bytes>" }`

## Errors

| Code | Message | Trigger |
|---|---|---|
| -1 | `Missing attachment id parameter` | `id` absent or null |
| -1 | `Could not find attachment with ID: <id>` | no such stored file |
| -32603 | `An error occurred reading attachment: <id> (UnexpectedErrorException)` | other I/O error |
