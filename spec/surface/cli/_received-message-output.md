# Received-message output (plain-text and JSON)

This file specifies how signal-cli 0.14.7 renders each received envelope:
- plain-text output from `receive`, and from `daemon` when it prints received messages to stdout;
- JSON output (`-o json`) from `receive` and `daemon`, which is also the payload of JSON-RPC `receive` notifications (see `jsonRpc.md` and `daemon.md`).

## Sources

- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/ReceiveMessageHandler.java (plain-text rendering)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/json/ (JsonReceiveMessageHandler, JsonMessageEnvelope, JsonError, JsonDataMessage, JsonEditMessage, JsonAttachment, JsonQuote, JsonQuotedAttachment, JsonMention, JsonReaction, JsonPayment, JsonPreview, JsonSticker, JsonRemoteDelete, JsonTextStyle, JsonGroupInfo, JsonStoryContext, JsonPinMessage, JsonUnpinMessage, JsonAdminDelete, JsonPollCreate, JsonPollVote, JsonPollTerminate, JsonSyncMessage, JsonSyncDataMessage, JsonSyncReadMessage, JsonSyncStoryMessage, JsonStoryMessage, JsonCallMessage, JsonReceiptMessage, JsonTypingMessage, JsonSharedContact, JsonContactName, JsonContactAvatar, JsonContactPhone, JsonContactEmail, JsonContactAddress)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/output/PlainTextWriterImpl.java, JsonWriterImpl.java (line and indentation rules)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/util/DateUtils.java, Hex.java, Util.java
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/lib/src/main/java/org/asamk/signal/manager/api/MessageEnvelope.java, RecipientAddress.java, GroupId.java, GroupIdV2.java, Color.java, TextStyle.java, and lib/.../manager/util/Utils.java (call id conversion)
- man page `man/signal-cli.1.adoc` section `receive`, and `man/signal-cli-jsonrpc.5.adoc` (examples) at tag v0.14.7
- Binary observation: none. Receiving needs a registered account, so this file is derived from source only. One fact was checked in `jshell` (JDK 25, the JVM the binary runs on): Java's HashMap iteration order for the keys `account`, `exception`, and `envelope`.

## Common formatting rules

### Formatted timestamp ("TS")
Many plain-text lines show a timestamp in this form:
`<millis> (<yyyy-MM-ddTHH:mm:ss.SSSZ>)`
- `<millis>` is the raw epoch-millisecond integer, in decimal.
- The part in parentheses is the same instant in UTC with exactly 3 millisecond digits, followed by the literal letter `Z` (UTC). Example: `1700000000000 (2023-11-14T22:13:20.000Z)`.
In this file, "TS(x)" means "x rendered in this form". "raw x" means the bare integer.

### Contact rendering ("CONTACT")
A recipient address renders as follows:
- First take its identifier: the phone number if known; otherwise the ACI UUID; otherwise the PNI; otherwise the username.
- If the local store has a non-empty contact name, or failing that a profile name, for the recipient, the result is `“NAME” IDENT`. The quotes are U+201C and U+201D, and one space separates them from the identifier.
- Otherwise the result is just `IDENT`.

### Group rendering ("GROUPINFO")
This is two lines at the current indentation:
- `Id: <base64 group id>`
- `Name: <group title>`, or `Name: <Unknown group>` if the group is not in the local store.
A title that is null renders as `null`.

### Indentation
- Each nesting level adds exactly 2 spaces.
- The indent is written only at the start of each logical line. If a value contains newlines (for example a multi-line body), the continuation lines are not indented.
- Lines end with the platform line separator (`\n` on Linux).
- Output is flushed after every line.
- Placeholder substitution is literal. A null value renders as `null`.
- Booleans render as `true` or `false`.

### Character encoding
stdout uses the console charset. Characters that the charset cannot encode become `?`. With an ASCII/POSIX locale, the curly quotes around names therefore become `?`.

### Call ids
In both plain text and JSON, call ids are rendered as the unsigned 64-bit decimal value of the wire id. In JSON this is a JSON number, not a string, and it can exceed 2^63.

## Plain-text rendering

Every envelope prints the lines below in this order. Indentation is level 0 unless stated otherwise.

1. `Envelope from: <SOURCE> (device: <sourceDevice>) to <selfNumber>`
   - `<SOURCE>` is CONTACT(source), or the literal `unknown source` if the envelope has no source address.
   - `<sourceDevice>` is an integer.
   - `<selfNumber>` is the account's own phone number.
2. `Timestamp: TS(envelope timestamp)`
3. `Server timestamps: received: TS(serverReceived) delivered: TS(serverDelivered)`
4. If the envelope used sealed sender: `Sent by unidentified/sealed sender`
5. If decrypting or handling the envelope raised an error:
   - If the error is an untrusted identity, three lines follow. `<self>` is the self number. `<id>` is the sender's identifier (phone number if known, else UUID).
     - `The user’s key is untrusted, either the user has reinstalled Signal or a third party sent this message.` (the apostrophe is U+2019)
     - `Use 'signal-cli -a <self> listIdentities -n <id>', verify the key and run 'signal-cli -a <self> trust -v "FINGER_PRINT" <id>' to mark it as trusted`
     - `If you don't care about security, use 'signal-cli -a <self> trust -a <id>' to trust it without verification`
   - Any other error: `Exception: <message> (<ExceptionSimpleClassName>)`
6. Data message, if present: the DATA block at level 0.
7. Edit message, if present:
   - `Edit: Target message timestamp: TS(targetSentTimestamp)` at level 0.
   - Then the DATA block at level 1.
8. Story message, if present: the STORY block at level 1. There is no header line.
9. Sync message, if present:
   - `Received a sync message` at level 0.
   - Then the SYNC block (the base level is 0).
10. Call message, if present:
    - `Received a call message` at level 0.
    - Then the CALL block at level 1.
11. Receipt message, if present:
    - `Received a receipt message` at level 0.
    - Then the RECEIPT block at level 1.
12. Typing message, if present:
    - `Received a typing message` at level 0.
    - Then the TYPING block at level 1.
13. An empty line. It is always printed and terminates the envelope.

Only one handler writes at a time, so an envelope's lines are never interleaved with another envelope's lines.

### DATA block (at level L)
Lines appear in this order. Each one appears only when its condition holds.
- `Message timestamp: TS(timestamp)`. Always printed.
- `=VIEW ONCE=` if the message is view-once.
- `Body: <body>` if there is a body. A long-text attachment, when present, replaces the body.
- If there is a group context:
  - `Group info:`
  - Then, at L+1: GROUPINFO, `Revision: <int>`, and `Type: UPDATE` or `Type: DELIVER`. UPDATE is used when the message is a group update.
- If there is a story context:
  - `Story reply:`
  - Then, at L+1: `Sender: CONTACT(author)` and `Sent timestamp: <raw millis>`. This timestamp is not formatted.
- If there is a group call update:
  - `Group call update:`
  - Then, at L+1: `Era id: <eraId>`.
- If there are previews: `Previews:`, then for each preview:
  - `- Preview` at L.
  - The PREVIEW block at L+1.
- If there are shared contacts: `Contacts:`, then for each contact:
  - `- Contact:` at L.
  - The SHAREDCONTACT block at L+1.
- If there is a sticker:
  - `Sticker:`
  - Then, at L+1: `Pack id: <lowercase hex of pack id, no separators>` and `Sticker id: <int>`.
- `Is end session` if the message is an end-session message.
- `Is Expiration update: true` if it is an expiration-timer update.
- `Expires in: <n> seconds` if expiresInSeconds > 0.
- `Profile key update` if it is a profile-key update.
- `With profile key` if the message carries a profile key.
- If there is a reaction:
  - `Reaction:`
  - Then, at L+1: `Emoji: <emoji>`, `Target author: CONTACT`, `Target timestamp: TS`, and `Is remove: <bool>`.
- If there is a quote:
  - `Quote:`
  - Then, at L+1:
    - `Id: <raw id>`
    - `Author: CONTACT`
    - `Text: <text>`, if the quote has text.
    - If the quote has mentions: `Mentions:`, followed by one MENTION line per mention at the same level (L+1).
    - If the quote has attachments: `Attachments:`, then for each one `- Attachment:` at L+1, followed by the ATTACHMENT block at L+2.
- `Remote delete message: timestamp = <raw millis>` if the message is a remote delete.
- If there are mentions: `Mentions:`, followed by one MENTION line per mention at level L. These lines are not indented further.
- If there are text styles: `Text styles:`, followed by one line per style at level L: `- <STYLE>: <start> (length: <length>)`. STYLE is one of `NONE`, `BOLD`, `ITALIC`, `SPOILER`, `STRIKETHROUGH`, `MONOSPACE`.
- If there are attachments: `Attachments:`, then for each one `- Attachment:` at L, followed by the ATTACHMENT block at L+1.
- If there is a poll creation:
  - `Poll Create: "<question>" (multi)` or `(single)`. The double quotes are ASCII.
  - Then one line per option at L: `- <option>`.
- If there is a poll vote: `Poll Vote: "CONTACT(target author)" (TS(targetSentTimestamp)) selected <indexes joined with "," without spaces> (vote #<voteCount>)`
- If there is a poll termination: `Poll Terminate: TS(targetSentTimestamp)`
- If there is a pin message:
  - `Pin Message:`
  - Then, at L+1: `Target author: CONTACT` and `Target timestamp: TS`.
  - Then a duration line:
    - `Duration: forever` if the duration is -1.
    - `Duration: unspecified` if it is 0.
    - `Duration: <n> seconds` otherwise.
- If there is an unpin message:
  - `Unpin Message:`
  - Then, at L+1: `Target author: CONTACT` and `Target timestamp: TS`.
- If there is an admin delete:
  - `Admin Delete:`
  - Then, at L+1: `Target author: CONTACT` and `Target timestamp: TS`.
- Payment messages produce no plain-text lines.

MENTION line: `- CONTACT(recipient): <start> (length: <length>)`

### PREVIEW block (at level P)
- `Title: <title>`
- `Description: <description>`
- `Date: TS(date)`
- `Url: <url>`
- If the preview has an image: `Image:`, followed by the ATTACHMENT block at P+1.

### ATTACHMENT block (at level A)
Lines appear in this order. Each one appears only when its condition holds.
- `Content-Type: <contentType>`. Always printed.
- `Type: Pointer` if the attachment has a remote id; otherwise `Type: Stream`.
- `Id: <id>` if it has an id.
- `Upload timestamp: TS(uploadTimestamp)` if present.
- `Caption: <caption>` if present.
- `Filename: <fileName>` if present.
- A size line, if a size or an inline preview is present: `Size: <S><P>`
  - `<S>` is `<n> bytes`, or `<unavailable>` if no size is known.
  - `<P>` is ` (Preview is available: <k> bytes)` when inline preview bytes exist; otherwise it is empty.
- If there is a thumbnail: `Thumbnail:`, followed by a nested ATTACHMENT block at A+1.
- `Flags: <list>` if any flag is set. The list is a comma-plus-space-joined subset of `voice note`, `borderless`, `video gif`, in that order.
- `Dimensions: <w>x<h>` if a width or a height is present. A missing value prints as 0.
- `Stored plaintext in: <absolute file path>` if the attachment has a local file and that file exists.

### SHAREDCONTACT block (at level C)
- `Name:`. Then, at C+1, each of the following only if it is non-blank: `First name: <given>`, `Middle name: <middle>`, `Family name: <family>`, `Prefix name: <prefix>`, `Suffix name: <suffix>`, `Display name: <nickname>`.
- If there is an avatar:
  - `Avatar: (profile: <bool>)`
  - The ATTACHMENT block at C+1.
- `Organisation: <organization>` if present. Note the British spelling.
- If there are phones: `Phone details:`. For each phone:
  - `- Phone:`
  - Then, at C+1: `Number: <value>`, `Type: <HOME|WORK|MOBILE|CUSTOM>`, and `Label: <label>` if the label is non-blank.
- If there are emails: `Email details:`. For each email:
  - `- Email:`
  - Then, at C+1: `Address: <value>`, `Type: <HOME|WORK|MOBILE|CUSTOM>`, and `Label: <label>` if non-blank.
- If there are addresses: `Address details:`. For each address:
  - `- Address:`
  - Then, at C+1: `Type: <HOME|WORK|CUSTOM>`.
  - Then each of the following, only if non-blank: `Label:`, `Street:`, `Pobox:`, `Neighbourhood:`, `City:`, `Region:`, `Postcode:`, `Country:`. Each label is followed by one space and the value.

### STORY block (at level S)
- `Story: with replies: <bool>`
- If the story belongs to a group: `Group info:`, followed by GROUPINFO at S+1.
- If there is a text attachment:
  - `Body: <text, or empty string>`.
  - If the text attachment has a preview: `Preview:`, followed by the PREVIEW block at S+1.
- If there is a file attachment: `Attachments:`, followed by the ATTACHMENT block at S+1. There is no `- Attachment:` line here.

### TYPING block (at level T)
- `Action: STARTED` or `Action: STOPPED`
- `Timestamp: TS(timestamp)`
- If the typing event is in a group: `Group Info:` (capital I), followed by GROUPINFO at T+1.

### RECEIPT block (at level R)
- `When: TS(when)`
- One of the following lines: `Is delivery receipt`, `Is read receipt`, or `Is viewed receipt`. For an unknown receipt type, no line is printed.
- `Timestamps:`
- One line per timestamp: `- TS(timestamp)`.

### CALL block (at level K)
Lines appear in this order. Each one appears only when present.
- `Destination device id: <int>`
- `Destination group id: <G>`. Quirk: `<G>` is not base64. It is the JVM's default object text for the group id: `org.asamk.signal.manager.api.GroupIdV2@<hex>` for a 32-byte id, or `...GroupIdV1@<hex>` for a 16-byte id. `<hex>` is the lowercase hex, without leading zeros, of a 32-bit hash. The hash is computed as Java's array hash over the id bytes: start with 1, then for each signed byte b, hash = 31*hash + b, using 32-bit overflow. Negative hashes are shown as unsigned 32-bit values.
- `Timestamp: TS(timestamp)`
- `Answer message: <callId>, opaque length: <n>)`. Quirk: the line ends with an unbalanced `)`.
- `Busy message: <callId>`
- `Hangup message: <callId>`, followed by `- type: <NORMAL|ACCEPTED|DECLINED|BUSY|NEED_PERMISSION>` and `- device id: <int>`, all three at level K.
- `Ice update messages:`, followed by one line per update: `- <callId>, opaque length: <n>`.
- `Offer message: <callId>, opaque length: <n>`. The offer type is not printed.
- `Opaque message: size <n>, urgency: <DROPPABLE|HANDLE_IMMEDIATELY>`

### SYNC block (base level 0)
Lines appear in this order. Each one appears only when present.
- Contacts sync: `Received complete sync contacts:` or `Received partial sync contacts:`. Nothing follows the colon.
- Groups sync: `Received sync groups.`
- Read list:
  - `Received sync read messages list`
  - Then one line per entry at level 0: `- From: CONTACT(sender) Message timestamp: TS(timestamp)`.
- Viewed list:
  - `Received sync viewed messages list`
  - Then the same line format as the read list.
- Sent transcript:
  - `Received sync sent message`.
  - Then, at level 1:
    - `To: <X>`. X is CONTACT(destination) if there is a destination. Otherwise X is the recipients rendered with CONTACT and joined by `, `. If neither exists, X is `<unknown>`.
    - `Timestamp: TS(timestamp)`.
    - `Expiration started at: TS(expirationStartTimestamp)`, only if the value is > 0.
    - The DATA block of the sent message at level 1, if there is one.
    - The STORY block at level 1, if the transcript is a story.
  - Edit-message transcripts are not printed in plain text.
- Blocked list:
  - `Received sync message with block list`
  - `Blocked:`
  - One line per blocked recipient at level 0: `- <identifier>`. The identifier is the phone number if known, otherwise the UUID. CONTACT rendering is not used here.
  - Then one line per blocked group: `- <base64 group id>`.
- View-once open:
  - `Received sync message with view once open message:`
  - Then, at level 1: `Sender: CONTACT` and `Timestamp: TS`.
- Message request response:
  - `Received message request response:`
  - Then, at level 1: `Type: <UNKNOWN|ACCEPT|DELETE|BLOCK|BLOCK_AND_DELETE|UNBLOCK_AND_ACCEPT|SPAM|BLOCK_AND_SPAM>`.
  - If the response is for a group: `For group:` at level 0, followed by GROUPINFO at level 1.
  - If the response is for a person: `For Person: CONTACT` at level 1.

## JSON rendering

Each received envelope produces one JSON object, written as a single line followed by a newline and then flushed. In CLI `receive -o json` and `daemon -o json` mode, the output is therefore a stream of JSON Lines.

The JSON serializer is used with default settings. In the tables below:
- "always" means the key is always present and its value may be `null`.
- "omit-null" means the key is absent when the value is null.
- "omit-empty" means the key is absent when the value is null or an empty list.
- Within an object, keys appear in the order the tables list them.
- Absent strings/numbers inside "always" keys are `null`.
- Byte arrays are serialized as base64 strings.

### Top-level object
This object has up to 3 keys. Their order is fixed: `exception` (if present), then `envelope`, then `account`. This order was confirmed in jshell for these three keys.

| key | type | presence | meaning |
|---|---|---|---|
| exception | object `{message: string or null, type: string}` | only if handling raised an error | `type` is the simple class name of the error (for example `UntrustedIdentityException`, `ProtocolInvalidMessageException`). |
| envelope | object (Envelope) | present whenever an envelope exists, which is the normal case | |
| account | string | always | The account's own phone number. |

So a typical line looks like `{"envelope":{...},"account":"+15555550123"}`.

### Envelope

| key | type | presence | notes |
|---|---|---|---|
| source | string or null | always | Deprecated. Phone number if known, otherwise UUID. |
| sourceNumber | string or null | always | |
| sourceUuid | string or null | always | ACI. |
| sourceName | string or null | always | Contact name or profile name. |
| sourceDevice | integer or null | always | |
| timestamp | integer | always | |
| serverReceivedTimestamp | integer | always | |
| serverDeliveredTimestamp | integer | always | |
| dataMessage | DataMessage | omit-null | |
| editMessage | EditMessage | omit-null | |
| storyMessage | StoryMessage | omit-null | |
| syncMessage | SyncMessage | omit-null | |
| callMessage | CallMessage | omit-null | |
| receiptMessage | ReceiptMessage | omit-null | |
| typingMessage | TypingMessage | omit-null | |

The source fields are null when the envelope has no source address. One exception: if the error is an untrusted identity, the sender and sender device are taken from the error.

### DataMessage

| key | type | presence |
|---|---|---|
| timestamp | integer | always |
| message | string or null | always. The body, or the long-text attachment body when one exists. |
| expiresInSeconds | integer | always (0 if none) |
| isExpirationUpdate | boolean | always |
| viewOnce | boolean | always |
| reaction | Reaction | omit-null |
| quote | Quote | omit-null |
| payment | `{note: string, receipt: base64 string}` | omit-null |
| mentions | Mention[] | omit-null. Null when there are no mentions, so the key is never `[]`. |
| previews | Preview[] | omit-null (never empty) |
| attachments | Attachment[] | omit-null (never empty) |
| sticker | `{packId: lowercase hex string, stickerId: integer}` | omit-null |
| remoteDelete | `{timestamp: integer}` | omit-null |
| contacts | SharedContact[] | omit-null (never empty) |
| pollCreate | `{question: string, allowMultiple: boolean, options: string[]}` | omit-null |
| pollVote | `{author (deprecated), authorNumber, authorUuid: string or null, targetSentTimestamp: integer, optionIndexes: integer[], voteCount: integer}` | omit-null |
| pollTerminate | `{targetSentTimestamp: integer}` | omit-null |
| textStyles | `{style: string, start: integer, length: integer}`[] | omit-null (never empty). `style` takes the enum names listed in the plain-text section. |
| groupInfo | `{groupId: base64, groupName: string, revision: integer, type: "UPDATE" or "DELIVER"}` | omit-null |
| storyContext | `{authorNumber: string or null, authorUuid: string or null, sentTimestamp: integer}` | omit-null |
| pinMessage | `{targetAuthor (deprecated), targetAuthorNumber, targetAuthorUuid, targetSentTimestamp: integer, pinDurationSeconds: integer}` | omit-null |
| unpinMessage | `{targetAuthor (deprecated), targetAuthorNumber, targetAuthorUuid, targetSentTimestamp}` | omit-null |
| adminDelete | `{targetAuthor (deprecated), targetAuthorNumber, targetAuthorUuid, targetSentTimestamp}` | omit-null |

Notes:
- The man-page example of a notification shows `"mentions":[],"attachments":[],"contacts":[]`. That example is outdated. In 0.14.7 these keys are omitted when there are no items. The binary is authoritative.
- In groupInfo, `groupName` comes from the local group store. Source review suggests that a group missing from the store would make the lookup fail with an internal error. This was not observed.
- Wherever a key named `author` or `targetAuthor` appears (deprecated), its value is the phone number if known, otherwise the UUID. The `...Number` and `...Uuid` keys are strings or null.

### EditMessage
`{targetSentTimestamp: integer, dataMessage: DataMessage}`. Both keys are always present.

### Reaction
`{emoji: string, targetAuthor: string (deprecated), targetAuthorNumber: string or null, targetAuthorUuid: string or null, targetSentTimestamp: integer, isRemove: boolean}`. All keys are always present.

### Quote

| key | type | presence |
|---|---|---|
| id | integer | always |
| author | string | always (deprecated) |
| authorNumber | string or null | always |
| authorUuid | string or null | always |
| text | string or null | always |
| mentions | Mention[] | omit-null (never empty) |
| attachments | QuotedAttachment[] | always. Here an empty list `[]` is emitted. |
| textStyles | TextStyle[] | omit-null (never empty) |

QuotedAttachment is `{contentType: string, filename: string or null, thumbnail: Attachment}`. The `thumbnail` key is omit-null.

### Mention
`{name: string (deprecated, the identifier), number: string or null, uuid: string or null, start: integer, length: integer}`. All keys are always present.

### Attachment
`{contentType: string, filename: string or null, id: string or null, size: integer or null, width: integer or null, height: integer or null, caption: string or null, uploadTimestamp: integer or null}`.
- All 8 keys are always present, in that order.
- There are no voice-note or flags fields.
- There is no local file path. To get the stored file, use `getAttachment` with `id`.

### Preview
`{url: string, title: string, description: string, image: Attachment or null}`. All keys are always present, and `image` can be `null`.

### SharedContact

| key | type | presence |
|---|---|---|
| name | `{nickname, given, family, prefix, suffix, middle}` | always. Each value is a string or null, and blank strings become null. All 6 keys are always present. |
| avatar | `{attachment: Attachment, isProfile: boolean}` | omit-null |
| phone | `{value: string, type: "HOME" or "WORK" or "MOBILE" or "CUSTOM", label: string or null}`[] | omit-null (never empty) |
| email | same shape as phone | omit-null (never empty) |
| address | `{type: "HOME" or "WORK" or "CUSTOM", label, street, pobox, neighborhood, city, region, postcode, country}`[] | omit-null (never empty). In each item, every key is always present and blank strings become null. |
| organization | string or null | always |

### StoryMessage

| key | type | presence |
|---|---|---|
| allowsReplies | boolean | always |
| groupId | base64 string | omit-null |
| fileAttachment | Attachment | omit-null |
| textAttachment | TextAttachment | omit-null |

TextAttachment:

| key | type | presence |
|---|---|---|
| text | string or null | always |
| style | `DEFAULT`, `REGULAR`, `BOLD`, `SERIF`, `SCRIPT`, or `CONDENSED` | omit-null |
| textForegroundColor | color string | omit-null |
| textBackgroundColor | color string | omit-null |
| preview | Preview | omit-null |
| backgroundGradient | `{startColor, endColor, colors: string[], positions: number[] (float), angle: integer or null}` | omit-null |
| backgroundColor | color string | omit-null |

- A color string is `#` followed by 8 lowercase hex digits in the order alpha, red, green, blue (`#aarrggbb`).
- In backgroundGradient, `startColor` and `endColor` are set only for a "legacy" gradient: exactly 2 colors with positions [0, 1]. Otherwise both are null. `colors` and `positions` are always present.

### SyncMessage
An object whose keys are all omit-null:

| key | type | when present |
|---|---|---|
| sentMessage | SyncDataMessage | A sent transcript that is not a story. |
| sentStoryMessage | SyncStoryMessage | A sent transcript that is a story. |
| blockedNumbers | string[] | A blocked-list sync. Each entry is the phone number if known, otherwise the UUID. |
| blockedGroupIds | base64 string[] | A blocked-list sync. |
| readMessages | `{sender (deprecated), senderNumber, senderUuid, timestamp: integer}`[] | A read sync with at least one entry. |
| type | `"CONTACTS_SYNC"` or `"GROUPS_SYNC"` | A contacts sync takes precedence. `REQUEST_SYNC` is defined but never produced. |

- Viewed-sync, view-once-open, and message-request-response syncs have no JSON representation.
- A sync message that only carries those, or any other unhandled sync, serializes as `"syncMessage":{}`.

SyncDataMessage:
- It starts with `destination` (deprecated), `destinationNumber`, and `destinationUuid`. These are always present, as a string or null, and are null when there is no single destination.
- Then `editMessage` (EditMessage), which is omit-null.
- Then all keys of a DataMessage, flattened ("unwrapped") into the same object: `timestamp`, `message`, `expiresInSeconds`, and so on. If the transcript has no data message, none of these keys appear.

SyncStoryMessage:
- `destinationNumber` and `destinationUuid`, each always present as a string or null.
- Then the StoryMessage keys, flattened into the same object.

### CallMessage
All keys are omit-null, except `iceUpdateMessages`, which is omit-empty.

| key | type |
|---|---|
| offerMessage | `{id: unsigned integer, type: "AUDIO_CALL" or "VIDEO_CALL", opaque: base64}` |
| answerMessage | `{id, opaque: base64}` |
| busyMessage | `{id}` |
| hangupMessage | `{id, type: "NORMAL" or "ACCEPTED" or "DECLINED" or "BUSY" or "NEED_PERMISSION", deviceId: integer}` |
| iceUpdateMessages | `{id, opaque: base64}`[] |

Opaque call messages, destination device id, group id, and call timestamp are not represented in JSON. An envelope with only such content yields `"callMessage":{}`.

### ReceiptMessage
`{when: integer, isDelivery: boolean, isRead: boolean, isViewed: boolean, timestamps: integer[]}`. All keys are always present.

### TypingMessage
`{action: "STARTED" or "STOPPED", timestamp: integer, groupId: base64 string}`. The `groupId` key is omit-null.

## Doc vs binary discrepancies
- The man-page jsonrpc example shows empty arrays (`mentions`, `attachments`, `contacts`) in dataMessage. In 0.14.7 those keys are omitted when empty.
- The man-page example omits `serverReceivedTimestamp`, `serverDeliveredTimestamp`, `expiresInSeconds`, `isExpirationUpdate`, and `account`. All of these are present in 0.14.7.
