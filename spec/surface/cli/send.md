# signal-cli send

## Sources

- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/commands/SendCommand.java (tag v0.14.7)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/util/CommandUtil.java
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/util/SendMessageResultUtils.java
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/util/IOUtils.java
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/lib/src/main/java/org/asamk/signal/manager/api/TextStyle.java
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/lib/src/main/java/org/asamk/signal/manager/api/RecipientIdentifier.java
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/lib/src/main/java/org/asamk/signal/manager/Manager.java (declared failure kinds)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/lib/src/main/java/org/asamk/signal/manager/api/AttachmentInvalidException.java, InvalidStickerException.java
- man page https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/man/signal-cli.1.adoc section `=== send`
- Binary (`signal-cli` 0.14.7, `--config <CONFIG>` = empty scratch dir), commands run:
  - `signal-cli --config <CONFIG> send --help`
  - `signal-cli --config <CONFIG> send`
  - `signal-cli --config <CONFIG> send --bogus`
  - `signal-cli --config <CONFIG> send -m hi +15555550123` (no account)
  - `signal-cli --config <CONFIG> send -m hi --message-from-stdin +15555550123`
  - `signal-cli --config <CONFIG> send --quote-timestamp abc -m hi +15555550123`
  - `signal-cli --config <CONFIG> send --edit-timestamp 1.5 -m hi +15555550123`
  - `signal-cli --config <CONFIG> send --story-timestamp -m hi`
  - `signal-cli --config <CONFIG> send -m`
  - `signal-cli --config <CONFIG> -a +15555550123 send -m hi +15555550199`
  - `signal-cli --config <CONFIG> -a +15555550123 -o json send -m hi +15555550199`
- Shared result format: see [_send-results.md](_send-results.md).

## Summary

Help line: `Send a message to another user or group.`

Sends a data message (text and/or attachments, sticker, quote, mentions, text
styles, link preview, story reply) to any combination of individual
recipients, groups, usernames and/or self. Alternatively sends an edit of a
previously sent message (`--edit-timestamp`) or an end-session message (`-e`).

## Invocation category & account requirement

Local single-account command. Requires a registered local account: either `-a`
must be given, or exactly one account must exist in the data directory. Account
errors (no local users, not registered, invalid account) are described in
[index.md](index.md) and happen before any validation below. Contacts the Signal
server.

## Output modes

`plain-text` (default) and `json`.

## Arguments

| Names | Arity | Value | Default | Required | Help text (verbatim) | Notes |
|---|---|---|---|---|---|---|
| `recipient` (positional) | 0..n | string | none | no | Specify the recipients' phone number. | Each value: phone number, ACI UUID, `PNI:<uuid>`, or `u:<username>` |
| `-g`, `--group-id`, `--group` | 0..n | GROUP_ID (base64) | none | no | Specify the recipient group ID. | Flag may be given with zero values |
| `-u`, `--username` | 0..n | USERNAME | none | no | Specify the recipient username or username link. | Value used verbatim as a username (no `u:` prefix needed) |
| `--note-to-self` | 0 | flag | false | no | Send the message to self. | |
| `--notify-self` | 0 | flag | false | no | If self is part of recipients/groups send a normal message, not a sync message. | |
| `-m`, `--message` | 1 | MESSAGE | none | no | Specify the message to be sent. | mutually exclusive with `--message-from-stdin` |
| `--message-from-stdin` | 0 | flag | false | no | Read the message from standard input. | mutually exclusive with `-m` |
| `-a`, `--attachment` | 0..n | ATTACHMENT | none | no | Add an attachment. Can be either a file path or a data URI. Data URI encoded attachments must follow the RFC 2397. Additionally a file name can be added, e.g. data:<MIME-TYPE>;filename=<FILENAME>;base64,<BASE64 ENCODED DATA>. | |
| `--view-once` | 0 | flag | false | no | Send the message as a view once message | |
| `-e`, `--end-session`, `--endsession` | 0 | flag | false | no | Clear session state and send end session message. | |
| `--mention` | 0..n | MENTION | none | no | Mention another group member (syntax: start:length:recipientNumber). Unit of start and length is UTF-16 code units, NOT Unicode code points. | |
| `--text-style` | 0..n | TEXT_STYLE | none | no | Style parts of the message text (syntax: start:length:STYLE). Unit of start and length is UTF-16 code units, NOT Unicode code points. | |
| `--quote-timestamp` | 1 | int64 | none | no | Specify the timestamp of a previous message with the recipient or group to add a quote to the new message. | |
| `--quote-author` | 1 | QUOTE_AUTHOR | none | no (but required when `--quote-timestamp` given) | Specify the number of the author of the original message. | |
| `--quote-message` | 1 | QUOTE_MESSAGE | none | no | Specify the message of the original message. | |
| `--quote-mention` | 0..n | QUOTE_MENTION | none | no | Quote with mention of another group member (syntax: start:length:recipientNumber) | |
| `--quote-attachment` | 0..n | QUOTE_ATTACHMENT | none | no | Specify the attachments of the original message (syntax: contentType[:filename[:previewFile]]), e.g. 'audio/aac' or 'image/png:test.png:/tmp/preview.jpg'. | |
| `--quote-text-style` | 0..n | QUOTE_TEXT_STYLE | none | no | Quote with style parts of the message text (syntax: start:length:STYLE) | |
| `--sticker` | 1 | STICKER | none | no | Send a sticker (syntax: stickerPackId:stickerId) | |
| `--preview-url` | 1 | PREVIEW_URL | none | no | Specify the url for the link preview (the same url must also appear in the message body). | |
| `--preview-title` | 1 | PREVIEW_TITLE | none | no | Specify the title for the link preview (mandatory). | Not enforced; defaults to empty string |
| `--preview-description` | 1 | PREVIEW_DESCRIPTION | none | no | Specify the description for the link preview (optional). | |
| `--preview-image` | 1 | PREVIEW_IMAGE | none | no | Specify the image file for the link preview (optional). | |
| `--story-timestamp` | 1 | int64 | none | no | Specify the timestamp of a story to reply to. | |
| `--story-author` | 1 | STORY_AUTHOR | none | no (effectively required with `--story-timestamp`) | Specify the number of the author of the story. | |
| `--edit-timestamp` | 1 | int64 | none | no | Specify the timestamp of a previous message with the recipient or group to send an edited message. | |
| `--no-urgent` | 0 | flag | false | no | Send the message without the urgent flag, so no push notification is triggered for the recipient. The message will still be delivered in real-time if the recipient's app is active. | |
| `--voice-note` | 0 | flag | false | no | Mark audio attachments as voice notes. Voice notes are displayed inline in Signal clients. | |
| `-h`, `--help` | 0 | | | | show this help message and exit | |

Mutually exclusive group: `-m/--message` vs `--message-from-stdin`.

Note: the subcommand's `-a` means `--attachment`; the global `-a` (account)
must come before the subcommand name.

Integer typing: int64 options reject non-integers at parse time with
`could not convert '<v>' to integer (64 bits)`.

## Constraints & validation

Performed in this order after the account is loaded (all exit 1 unless noted):

1. Recipients are collected from positional recipients, `-g`, `-u` and
   `--note-to-self`. Parsing errors and "No recipients given" as in
   [_send-results.md](_send-results.md#common-recipient-parsing-errors-all-exit-1-stderr).
   Duplicate recipients collapse (set semantics).
2. If `-e/--end-session` is given: only the individual (non-group, non-self)
   recipients are kept; if none remain -> `No recipients given`. An end-session
   message is sent to them; all other options (including `-m`) are ignored. No
   output at all on success (exit 0). I/O failure -> `Failed to send message: <msg> (<ExceptionName>)`, exit 2.
3. `--sticker` must match: lowercase hex pack id of even length, `:`, decimal
   sticker id (uppercase hex rejected). Otherwise
   `Invalid sticker syntax (<value>) expected 'stickerPackId:stickerId'`.
4. Message text: `--message-from-stdin` reads ALL of stdin as text in the
   console/default charset, verbatim (no trimming; trailing newline kept). Read
   failure -> `Failed to read message from stdin: <msg>`. Without either option
   the text is empty.
5. `--mention` values must match `<digits>:<digits>:<recipient>`; otherwise
   `Invalid mention syntax (<value>) expected 'start:length:recipientNumber'`.
   The recipient part is parsed like a recipient (number/UUID/`PNI:`/`u:`);
   errors as `Invalid phone number '<s>': <reason>`.
6. `--text-style` values must match `<digits>:<digits>:<STYLE>`; otherwise
   `Invalid textStyle syntax (<value>) expected 'start:length:STYLE'`. STYLE is
   case-sensitive, one of `NONE`, `BOLD`, `ITALIC`, `SPOILER`, `STRIKETHROUGH`,
   `MONOSPACE`; otherwise `Invalid style: <STYLE>`.
7. Quote (only when `--quote-timestamp` given): `--quote-author` missing ->
   `Quote author parameter is missing`. Quote mentions and quote text styles are
   validated as in 5 and 6. Each `--quote-attachment` must be
   `contentType[:filename[:previewFile]]` (content type and filename contain no
   colon; the preview file may) else
   `Invalid attachment syntax (<value>) expected 'contentType[:filename[:previewFile]]'`.
   Quote text defaults to empty. Quote options without `--quote-timestamp` are ignored.
8. Link preview only when `--preview-url` given; title and description default
   to empty strings, image optional. Other preview options without the URL are ignored.
9. Story reply only when `--story-timestamp` given; author taken from
   `--story-author`. If `--story-author` is missing, the reference crashes with
   an internal null-pointer error (Java stack trace on stderr, exit 2) -- not a
   clean message (derived from source, unverified).
10. Empty check: if text is empty AND no attachments AND no sticker AND no
    quote -> `Sending empty message is not allowed, either a message, attachment or sticker must be given.`
    (A preview or story reply alone does not satisfy this.)
11. Numeric overflow of mention/style start or length (more digits than fit a
    32-bit int) is an unhandled internal error (stack trace, exit 2).

## Behavior

- If `--edit-timestamp` is given, the message is sent as an edit of the
  previously sent message with that timestamp; `--notify-self` is not applied
  for edits. Otherwise a normal message is sent, with `--notify-self` honoured.
- The urgent flag is set unless `--no-urgent`.
- Attachments are file paths or RFC 2397 data URIs with optional `filename=`
  parameter.
- `--view-once` and `--voice-note` set the corresponding attachment/message flags.

## Plain-text output

See [_send-results.md](_send-results.md): optional failure block, then the
message timestamp on one line. `-e` prints nothing.

## JSON output

See [_send-results.md](_send-results.md): `{"timestamp":<int64>,"results":[...]}`.
`-e` prints nothing.

## Errors

| Condition | Stream | Exact message | Exit |
|---|---|---|---|
| `-m` and `--message-from-stdin` together (parse) | stderr | usage + `signal-cli:  error:  argument   --message-from-stdin:   not   allowed  with` / `argument -m/--message` (wrapped, observed) | 1 |
| non-integer for int64 option (parse) | stderr | usage + `signal-cli: error: argument --quote-timestamp:  could  not convert 'abc' to` / `integer (64 bits)` (observed; analogous for `--edit-timestamp`, `--story-timestamp`) | 1 |
| option missing its value (parse) | stderr | usage + `signal-cli: error: argument -m/--message: expected one argument` (observed) | 1 |
| unknown option (parse) | stderr | usage + `signal-cli: error: unrecognized arguments: '--bogus'` + blank line + `Did you mean:` + tab + suggestion (observed) | 1 |
| no account / not registered | stderr | see index.md (`No local users found, you first need to register or link an account`, `User +15555550123 is not registered.` observed) | 1 |
| no recipients | stderr | `No recipients given` | 1 |
| invalid recipient / group id | stderr | see _send-results.md | 1 |
| bad sticker syntax | stderr | `Invalid sticker syntax (<v>) expected 'stickerPackId:stickerId'` | 1 |
| stdin read failure | stderr | `Failed to read message from stdin: <msg>` | 1 |
| bad mention | stderr | `Invalid mention syntax (<v>) expected 'start:length:recipientNumber'` | 1 |
| bad text style | stderr | `Invalid textStyle syntax (<v>) expected 'start:length:STYLE'` / `Invalid style: <S>` | 1 |
| quote without author | stderr | `Quote author parameter is missing` | 1 |
| bad quote attachment | stderr | `Invalid attachment syntax (<v>) expected 'contentType[:filename[:previewFile]]'` | 1 |
| empty message | stderr | `Sending empty message is not allowed, either a message, attachment or sticker must be given.` | 1 |
| group not found / not member / not allowed | stderr | `Group not found: <id>` etc. (see _send-results.md) | 1 |
| unregistered recipient (detected before sending) | stderr | `The user <identifier> is not registered.` | 1 |
| sticker unknown/invalid | stderr | `Failed to send sticker: <msg>` | 1 |
| attachment invalid or I/O error | stderr | `Failed to send message: <msg> (<ExceptionName>)`, with `, maybe one of the devices of the recipient wasn't online for a while.` appended when msg contains `No prekeys available` | 2 |
| all recipients failed | stdout result, then stderr | `Failed to send message due to untrusted identities` / `Failed to send message due to rate limiting` / `Failed to send message` | 4 / 5 / 1 |
| end-session I/O error | stderr | `Failed to send message: <msg> (<ExceptionName>)` | 2 |

Full observed stderr transcripts are in observations.md (parse errors print the
subcommand usage block first, then the `signal-cli: error:` line(s)).

## --help transcript

Command: `signal-cli --config <CONFIG> send --help` -- exit code: 0, stderr empty. Stdout:

```text
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli send [-h] [-g [GROUP_ID [GROUP_ID ...]]]
                  [-u [USERNAME [USERNAME ...]]] [--note-to-self]
                  [--notify-self] [-a [ATTACHMENT [ATTACHMENT ...]]]
                  [--view-once] [-e] [--mention [MENTION [MENTION ...]]]
                  [--text-style [TEXT_STYLE [TEXT_STYLE ...]]]
                  [--quote-timestamp QUOTE_TIMESTAMP]
                  [--quote-author QUOTE_AUTHOR]
                  [--quote-message QUOTE_MESSAGE]
                  [--quote-mention [QUOTE_MENTION [QUOTE_MENTION ...]]]
                  [--quote-attachment [QUOTE_ATTACHMENT [QUOTE_ATTACHMENT ...]]]
                  [--quote-text-style [QUOTE_TEXT_STYLE [QUOTE_TEXT_STYLE ...]]]
                  [--sticker STICKER] [--preview-url PREVIEW_URL]
                  [--preview-title PREVIEW_TITLE]
                  [--preview-description PREVIEW_DESCRIPTION]
                  [--preview-image PREVIEW_IMAGE]
                  [--story-timestamp STORY_TIMESTAMP]
                  [--story-author STORY_AUTHOR]
                  [--edit-timestamp EDIT_TIMESTAMP] [--no-urgent]
                  [--voice-note] [-m MESSAGE | --message-from-stdin]
                  [recipient [recipient ...]]

positional arguments:
  recipient              Specify the recipients' phone number.

named arguments:
  -h, --help             show this help message and exit
  -g [GROUP_ID [GROUP_ID ...]], --group-id [GROUP_ID [GROUP_ID ...]], --group [GROUP_ID [GROUP_ID ...]]
                         Specify the recipient group ID.
  -u [USERNAME [USERNAME ...]], --username [USERNAME [USERNAME ...]]
                         Specify the recipient username or username link.
  --note-to-self         Send the message to self.
  --notify-self          If  self  is  part  of  recipients/groups  send  a
                         normal message, not a sync message.
  -m MESSAGE, --message MESSAGE
                         Specify the message to be sent.
  --message-from-stdin   Read the message from standard input.
  -a [ATTACHMENT [ATTACHMENT ...]], --attachment [ATTACHMENT [ATTACHMENT ...]]
                         Add an attachment. Can be either  a file path or a
                         data  URI.  Data  URI   encoded  attachments  must
                         follow the RFC 2397. Additionally  a file name can
                         be       added,       e.g.       data:<MIME-TYPE>;
                         filename=<FILENAME>;base64,<BASE64 ENCODED DATA>.
  --view-once            Send the message as a view once message
  -e, --end-session, --endsession
                         Clear session state and send end session message.
  --mention [MENTION [MENTION ...]]
                         Mention  another  group   member  (syntax:  start:
                         length:recipientNumber). Unit of  start and length
                         is UTF-16 code units, NOT Unicode code points.
  --text-style [TEXT_STYLE [TEXT_STYLE ...]]
                         Style parts of  the  message  text (syntax: start:
                         length:STYLE). Unit of start  and length is UTF-16
                         code units, NOT Unicode code points.
  --quote-timestamp QUOTE_TIMESTAMP
                         Specify the timestamp of  a  previous message with
                         the recipient or group to  add  a quote to the new
                         message.
  --quote-author QUOTE_AUTHOR
                         Specify the number of  the  author of the original
                         message.
  --quote-message QUOTE_MESSAGE
                         Specify the message of the original message.
  --quote-mention [QUOTE_MENTION [QUOTE_MENTION ...]]
                         Quote  with  mention   of   another  group  member
                         (syntax: start:length:recipientNumber)
  --quote-attachment [QUOTE_ATTACHMENT [QUOTE_ATTACHMENT ...]]
                         Specify the attachments  of  the  original message
                         (syntax: contentType[:filename[:previewFile]]), e.
                         g.     'audio/aac'     or     'image/png:test.png:
                         /tmp/preview.jpg'.
  --quote-text-style [QUOTE_TEXT_STYLE [QUOTE_TEXT_STYLE ...]]
                         Quote  with  style  parts   of  the  message  text
                         (syntax: start:length:STYLE)
  --sticker STICKER      Send a sticker (syntax: stickerPackId:stickerId)
  --preview-url PREVIEW_URL
                         Specify the url  for  the  link  preview (the same
                         url must also appear in the message body).
  --preview-title PREVIEW_TITLE
                         Specify   the   title   for   the   link   preview
                         (mandatory).
  --preview-description PREVIEW_DESCRIPTION
                         Specify  the  description  for  the  link  preview
                         (optional).
  --preview-image PREVIEW_IMAGE
                         Specify  the  image  file  for  the  link  preview
                         (optional).
  --story-timestamp STORY_TIMESTAMP
                         Specify the timestamp of a story to reply to.
  --story-author STORY_AUTHOR
                         Specify the number of the author of the story.
  --edit-timestamp EDIT_TIMESTAMP
                         Specify the timestamp of  a  previous message with
                         the recipient or group to send an edited message.
  --no-urgent            Send the message without  the  urgent  flag, so no
                         push notification is triggered  for the recipient.
                         The message will still  be  delivered in real-time
                         if the recipient's app is active.
  --voice-note           Mark  audio  attachments  as  voice  notes.  Voice
                         notes are displayed inline in Signal clients.
```

## Doc vs binary discrepancies

- Man page does not list `--voice-note` (binary has it).
- Man page lists STYLE choices as BOLD, ITALIC, SPOILER, STRIKETHROUGH,
  MONOSPACE; the binary additionally accepts `NONE`.
- `--preview-title` is described as "(mandatory)" but is not enforced.
- Man page shows `-g`, `-u` as single-valued; binary accepts zero or more values each.
