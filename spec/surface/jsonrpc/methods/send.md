# Method `send`

## Sources

- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/commands/SendCommand.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/util/SendMessageResultUtils.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/util/CommandUtil.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/json/JsonSendMessageResult.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/json/JsonRecipientAddress.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/jsonrpc/SignalJsonRpcCommandHandler.java (error mapping)
- https://github.com/AsamK/signal-cli/blob/v0.14.7/lib/src/main/java/org/asamk/signal/manager/api/SendMessageResults.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/lib/src/main/java/org/asamk/signal/manager/api/SendMessageResult.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/lib/src/main/java/org/asamk/signal/manager/api/RecipientAddress.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/lib/src/main/java/org/asamk/signal/manager/api/TextStyle.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/lib/src/main/java/org/asamk/signal/manager/api/Manager.java (method signatures, which result kinds each send returns)
- Exception message texts: `lib/.../manager/api/GroupNotFoundException.java`, `NotAGroupMemberException.java`,
  `GroupSendingNotAllowedException.java`, `UnregisteredRecipientException.java`, `InvalidStickerException.java`,
  `AttachmentInvalidException.java` (all at v0.14.7)
- Man page https://github.com/AsamK/signal-cli/blob/v0.14.7/man/signal-cli-jsonrpc.5.adoc (examples)
- Binary observation: none for this method (requires a registered account). Parameter
  conventions referenced below are in [index.md](index.md).

## Summary

Sends a data message (text, attachments, mentions, styles, quote, sticker,
link preview, story reply) to one or more recipients and/or groups, or edits a
previously sent message, or sends an end-session message.

CLI equivalent: `signal-cli -a ACCOUNT send [RECIPIENT...] [options]`.

Account requirement: **single** (see index.md).

## Parameters

All keys follow index.md conventions (dash form, then camelCase form; list
params also accept the plural key). "List" means a JSON array of strings, or a
single string which is wrapped.

| JSON key(s) | Type | Required | Default | CLI flag | Notes |
|---|---|---|---|---|---|
| `recipient` / `recipients` | list of string | no* | none | positional `RECIPIENT...` | Each parsed per index.md rule 10 (UUID, `PNI:<uuid>`, `u:<username>`, phone number). |
| `group-id` / `groupId` / `group-ids` / `groupIds` | list of string | no* | none | `-g`, `--group-id`, `--group` | Base64 group id, index.md rule 11. The `group` alias is NOT accepted in JSON. |
| `username` / `usernames` | list of string | no* | none | `-u`, `--username` | Username or username link, passed verbatim. |
| `note-to-self` / `noteToSelf` | bool | no* | false | `--note-to-self` | Adds the account itself as a recipient. |
| `notify-self` / `notifySelf` | bool | no | false | `--notify-self` | If self is among recipients/groups, send a normal message instead of only a sync message. Ignored for edits. |
| `message` | string | no | `""` | `-m`, `--message` | Message body. |
| `message-from-stdin` / `messageFromStdin` | bool | no | false | `--message-from-stdin` | Reads the body from the **daemon process's** standard input until EOF (useless in practice over RPC; documented for parity). Read failure: code -1 `Failed to read message from stdin: <reason>`. |
| `attachment` / `attachments` | list of string | no | empty | `-a`, `--attachment` | Each is a file path on the daemon host, or an RFC 2397 data URI, optionally with a filename: `data:<MIME-TYPE>;filename=<FILENAME>;base64,<DATA>`. |
| `view-once` / `viewOnce` | bool | no | false | `--view-once` | |
| `end-session` / `endSession` | bool | no | false | `-e`, `--end-session`, `--endsession` | See "End-session mode" below. The `endsession` alias is NOT accepted. |
| `mention` / `mentions` | list of string | no | empty | `--mention` | Each `start:length:recipient` (start, length decimal digits; unit UTF-16 code units). Recipient parsed per rule 10. |
| `text-style` / `textStyle` / `text-styles` / `textStyles` | list of string | no | empty | `--text-style` | Each `start:length:STYLE`, STYLE exactly one of `NONE`, `BOLD`, `ITALIC`, `SPOILER`, `STRIKETHROUGH`, `MONOSPACE` (upper case, case-sensitive). |
| `quote-timestamp` / `quoteTimestamp` | long | no | none | `--quote-timestamp` | Presence enables the quote. Must be a JSON integer > 2^31-1 (index.md rule 6). |
| `quote-author` / `quoteAuthor` | string | if quote | none | `--quote-author` | Recipient per rule 10. |
| `quote-message` / `quoteMessage` | string | no | `""` | `--quote-message` | |
| `quote-mention` / `quoteMention` / `quote-mentions` / `quoteMentions` | list of string | no | empty | `--quote-mention` | Same syntax as `mention`. |
| `quote-text-style` / `quoteTextStyle` / plural forms | list of string | no | empty | `--quote-text-style` | Same syntax as `text-style`. |
| `quote-attachment` / `quoteAttachment` / plural forms | list of string | no | empty | `--quote-attachment` | Each `contentType[:filename[:previewFile]]` (content type: no colon; filename: no colon; preview file: rest). |
| `sticker` | string | no | none | `--sticker` | `packIdHex:stickerId`: pack id is lowercase hex digits (`[0-9a-f]`) of even length, sticker id decimal. |
| `preview-url` / `previewUrl` | string | no | none | `--preview-url` | Presence enables one link preview. URL should also appear in the body. |
| `preview-title` / `previewTitle` | string | no | `""` | `--preview-title` | |
| `preview-description` / `previewDescription` | string | no | `""` | `--preview-description` | |
| `preview-image` / `previewImage` | string | no | none | `--preview-image` | Image file path / data URI. |
| `story-timestamp` / `storyTimestamp` | long | no | none | `--story-timestamp` | Presence makes this a story reply. |
| `story-author` / `storyAuthor` | string | with story-timestamp | none | `--story-author` | Recipient per rule 10. |
| `edit-timestamp` / `editTimestamp` | long | no | none | `--edit-timestamp` | Presence turns the send into an edit of the message with that timestamp. |
| `no-urgent` / `noUrgent` | bool | no | false | `--no-urgent` | Sends without the urgent flag. |
| `voice-note` / `voiceNote` | bool | no | false | `--voice-note` | Marks audio attachments as voice notes. |

\* At least one of `recipient`, `group-id`, `username`, `note-to-self` must yield
a recipient, otherwise error "No recipients given".

`message` and `message-from-stdin` are mutually exclusive on the CLI; over
JSON-RPC there is no mutual exclusion check: if `message-from-stdin` is true the
stdin text replaces `message`.

### Validation order (observable through which error wins)

1. Recipients collected and parsed (invalid phone number / group id errors).
   Empty set -> "No recipients given".
2. If `end-session` is true: only individual recipients are kept; if none ->
   "No recipients given"; the end-session message is sent and the method
   finishes (all other params ignored).
3. `sticker` syntax.
4. Message text (stdin if requested, else `message`, else empty).
5. `mention` syntax, then `text-style` syntax.
6. Quote: if `quote-timestamp` present: missing `quote-author` -> error; then
   quote mentions, quote text styles, quote attachments parsed. (Note: quote
   author is parsed after quote mentions/styles/attachments.)
7. Preview fields.
8. Story reply: `story-author` parsed only if `story-timestamp` given.
9. Empty check: body empty AND no attachments AND no sticker AND no quote ->
   error.
10. Send (or edit, if `edit-timestamp` present).

## Result

### Normal / edit send

A JSON object (the "send result object", shared by all send-type methods):

| Key | Type | Nullable | Meaning |
|---|---|---|---|
| `timestamp` | integer (int64) | no | Timestamp (ms since epoch) of the sent message; for edits the timestamp of the new edit message. |
| `results` | array of per-recipient result objects | no | One entry per delivery target (per individual recipient; for groups one per group member sent to). May be empty. Order unspecified. |

Key order of `timestamp` and `results` inside the object is not fixed upstream
(built from an immutable two-entry map whose iteration order varies between JVM
runs); clients must not rely on it.

Per-recipient result object:

| Key | Type | Presence | Meaning |
|---|---|---|---|
| `recipientAddress` | object | always | `{"uuid": string\|null, "number": string\|null, "username": string\|null}`; all three keys always present, null when unknown. `uuid` is the ACI. |
| `groupId` | string | only when the target was a group | Base64 group id. Omitted (not null) otherwise. |
| `type` | string | always | One of `SUCCESS`, `NETWORK_FAILURE`, `UNREGISTERED_FAILURE`, `IDENTITY_FAILURE`, `RATE_LIMIT_FAILURE`, `INVALID_PRE_KEY_FAILURE`. Precedence when several apply: SUCCESS, RATE_LIMIT_FAILURE (also used for proof-required/captcha failures), NETWORK_FAILURE, UNREGISTERED_FAILURE, INVALID_PRE_KEY_FAILURE, otherwise IDENTITY_FAILURE. |
| `token` | string | only for proof-required failures | Challenge token to pass to `submitRateLimitChallenge`. |
| `retryAfterSeconds` | integer | only when a rate-limit/proof-required retry-after is known | Retry-after in seconds, rounded up from milliseconds. |

Example: `{"timestamp":1700000000000,"results":[{"recipientAddress":{"uuid":"…","number":"+4915…","username":null},"type":"SUCCESS"}]}`

The man page's examples abbreviate the result as `{"timestamp":999}`; the real
result always also carries `results`.

### End-session mode

Result is the empty object `{}`.

### Overall failure after sending

After writing the send result object, the method inspects the per-recipient
results. If **no** entry is SUCCESS and the list is non-empty, it fails (the
already produced result is attached as error `data`: `{"response": <send result object>}`):

- all entries are identity failures -> code **-4**, message `Failed to send message due to untrusted identities`
- else all entries are rate-limit failures (incl. proof required) -> code **-5**, message `Failed to send message due to rate limiting`
- else -> code **-1**, message `Failed to send message`

An empty `results` list counts as success.

## Errors

Codes per ../errors.md. `data` is `null` unless stated.

| Code | Message | Trigger |
|---|---|---|
| -1 | `No recipients given` | No recipients resolved; or end-session with no individual recipients. |
| -1 | `Invalid phone number '<input>': <reason>` | Unparseable recipient / mention / quote-author / story-author. |
| -1 | `Invalid group id: Failed to decode groupId (must be base64) "<input>": <reason>` | Bad group id. |
| -1 | `Invalid sticker syntax (<input>) expected 'stickerPackId:stickerId'` | `sticker` malformed or odd-length hex. |
| -1 | `Invalid mention syntax (<input>) expected 'start:length:recipientNumber'` | Bad `mention`/`quote-mention` entry. |
| -1 | `Invalid textStyle syntax (<input>) expected 'start:length:STYLE'` | Bad `text-style`/`quote-text-style` entry. |
| -1 | `Invalid style: <STYLE>` | Unknown STYLE word. |
| -1 | `Quote author parameter is missing` | `quote-timestamp` without `quote-author`. |
| -1 | `Invalid attachment syntax (<input>) expected 'contentType[:filename[:previewFile]]'` | Bad quote attachment. |
| -1 | `Sending empty message is not allowed, either a message, attachment or sticker must be given.` | Nothing to send. |
| -1 | `Group not found: <groupIdBase64>` | Unknown group. |
| -1 | `User is not a member in group: <groupName> (<groupIdBase64>)` | Not a member. |
| -1 | `User is not allowed to send message to group: <groupName> (<groupIdBase64>)` | Announcement-only group etc. |
| -1 | `The user <identifier> is not registered.` | Recipient unregistered (identifier: ACI, else number, else PNI, else username). |
| -1 | `Failed to send sticker: <reason>` | Sticker not installed/invalid. |
| -1 | `Failed to read message from stdin: <reason>` | stdin read failure. |
| -1 / -4 / -5 | see "Overall failure after sending" | No successful recipient; `data` = `{"response": {...}}`. |
| -32603 | `Failed to send message: <reason> (<ExceptionName>) (UnexpectedErrorException)` | Attachment invalid (reason `<attachment>: <detail>`, exception name `AttachmentInvalidException`) or I/O error during send / end-session. If the I/O message contains `No prekeys available` the message is `Failed to send message: <reason> (<ExceptionName>), maybe one of the devices of the recipient wasn't online for a while. (UnexpectedErrorException)`. |
| -32603 | `<java exception text> (ClassCastException)` | Type mismatch per index.md rules 3, 5, 6 (e.g. bool given as string, small integer for a timestamp, non-string list elements). |
| -32603 | `<text> (NullPointerException)` or similar | Internal failure, e.g. `story-timestamp` without `story-author`. Exact text is JVM-generated; match the code and the parenthesised suffix only. |
| -32602 | `Method requires valid account parameter` / `Specified account does not exist` | Account selection (multi-account mode). |
| -32600 | Jackson deserialization text | `params` is an array. |
