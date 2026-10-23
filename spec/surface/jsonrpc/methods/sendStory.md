# Method `sendStory`

## Sources

- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/commands/SendStoryCommand.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/util/SendMessageResultUtils.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/util/CommandUtil.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/lib/src/main/java/org/asamk/signal/manager/Manager.java
- Binary observation: none. Conventions: [index.md](index.md); result: [send.md](send.md).

## Summary

Posts an image/video story to "My Story", or to a group's story. CLI:
`signal-cli -a ACCOUNT sendStory -a ATTACHMENT [--no-replies] [-g GROUP]`.

Account requirement: **single**.

## Parameters

| JSON key(s) | Type | Required | Default | CLI flag | Notes |
|---|---|---|---|---|---|
| `attachment` | string (single) | yes | none | `-a`, `--attachment` | File path on the daemon host of the image or video. Single value: no plural form, an array is stringified (index.md rule 4) and will then fail as a bad path. |
| `no-replies` / `noReplies` | bool | no | false | `--no-replies` | Disallow replies. |
| `group-id` / `groupId` | string (single) | no | none (My Story) | `-g`, `--group-id` | One base64 group id (index.md rule 11). No plural form. |

## Result

Send result object as in [send.md](send.md), same post-send failure rules.

## Errors

| Code | Message | Trigger |
|---|---|---|
| -1 | `An attachment is required for sending a story.` | `attachment` missing or empty string. |
| -1 | `Invalid group id: Failed to decode groupId (must be base64) "<input>": <reason>` | Bad group id. |
| -1 | `Group not found: …` / `User is not a member in group: …` | Group problems. |
| -1 / -4 / -5 | post-send failure | No successful recipient. |
| -32603 | `Failed to send story: <reason> (<ExceptionName>) (UnexpectedErrorException)` | Invalid attachment (`AttachmentInvalidException`) or I/O error (`IOException` etc.). |
| -32603 | `<text> (ClassCastException)` | `no-replies` not a boolean. |
