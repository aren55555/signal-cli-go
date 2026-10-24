# Data-message features: quotes, reactions, edits, polls, PNI signatures and more

Spec role: prose and tables only. This file extends `content.md`. That
file covers the `Content` envelope, padding, receipts, typing and a plain
1:1 text send. This file covers every other DataMessage feature that
signal-cli 0.14.7 sends or understands. For each feature it gives the
wire shape, the exact values signal-cli puts in each field, the
send-side rules, and what signal-cli does when it receives the feature.
It also gives the wire result of every send-family CLI command.

Not covered here (cross-references only):

- attachment upload, `AttachmentPointer` contents, download, sticker-pack
  download and the long-text attachment's transport: the attachments spec
  (another agent; `spec/service/attachments.md` when it lands);
- sync messages (sent transcripts, read/viewed syncs, message-request-response
  syncs, verified syncs) and linked devices: the sync/linked-devices spec;
- `StoryMessage` itself, group sends, sender keys, `GroupContextV2`: the
  stories/sender-key spec. Where a command can target a group, this file
  only describes the DataMessage that is built;
- the request that carries ciphertexts, the sealed-sender decision and the
  response handling: `send.md`;
- CLI argument syntax, parse errors and output formats:
  `spec/surface/cli/<command>.md`, `_send-results.md`,
  `_received-message-output.md`.

## Sources

| Ref | What | URL @ version |
|-----|------|---------------|
| S1 | Wire-formatted `SignalService.proto` inside the reference jar `/work/third_party/signal-cli-0.14.7/lib/signal-service-java-2.15.3_unofficial_151.jar` (same jar as `content.md` S1). Field numbers, types and enum values. Cross-checked against the source `.proto` of S2, `lib/libsignal-service/src/main/protowire/SignalService.proto`; the two differ only in formatting, comments and the position of `oneof` blocks. | jar from the signal-cli 0.14.7 release |
| S2 | signal-service-java (Turasa fork) | https://github.com/Turasa/libsignal-service-java @ tag `v2.15.3_unofficial_151` = commit `66956cb4a0394d50cee3590209baf58312084a44`: `lib/libsignal-service/src/main/java/org/whispersystems/signalservice/api/SignalServiceMessageSender.java`, `.../api/messages/{SignalServiceDataMessage,SignalServiceContent,EnvelopeContentValidator}.kt`, `.../api/util/OptionalUtil.kt`; `core/models-jvm/src/main/java/org/signal/core/models/ServiceId.kt`; `core/util-jvm/src/main/java/org/signal/core/util/StringExtensions.kt` |
| S3 | signal-cli | https://github.com/AsamK/signal-cli @ tag `v0.14.7` = commit `b01b6b370dc063599a1a2b9fde0f5ff4e2d78fe8`: `lib/src/main/java/org/asamk/signal/manager/internal/ManagerImpl.java`; `.../manager/helper/{SendHelper,IncomingMessageHandler,ContactHelper,SyncHelper,IdentityHelper,RecipientHelper}.java`; `.../manager/actions/{RenewSessionAction,SendReceiptAction,SendProfileKeyAction}.java`; `.../manager/storage/recipients/{RecipientStore,RecipientAddress}.java`; `.../manager/storage/sendLog/MessageSendLogStore.java`; `.../manager/api/{Message,TextStyle,MessageEnvelope,Contact}.java`; `.../manager/config/ServiceConfig.java`; `.../manager/util/MimeUtils.java`; `.../manager/groups/GroupUtils.java`; `src/main/java/org/asamk/signal/commands/{Send,SendReaction,RemoteDelete,SendTyping,SendReceipt,SendPollCreate,SendPollVote,SendPollTerminate,SendPinMessage,SendUnpinMessage,SendAdminDelete,SendPaymentNotification,SendMessageRequestResponse,UpdateContact}Command.java` |
| S4 | libsignal | https://github.com/signalapp/libsignal @ tag `v0.99.1` = commit `97801d22dcf9f5bf714f7b8fa3212cdc973ae1c8`: `rust/protocol/src/identity_key.rs` (alternate-identity signature), `rust/core/src/curve.rs` (multipart signing), `java/client/src/test/java/org/signal/libsignal/protocol/IdentityKeyTest.java`, `node/ts/test/protocol/ProtocolTest.ts` (searched for fixed vectors) |
| S5 | Reference distribution, offline | `jshell` over `/work/third_party/signal-cli-0.14.7/lib/*.jar`, no network: hand-built messages serialized with the bundled Wire classes, to observe field presence and emission order (§1.1); `javap -c` on `SignalServiceDataMessage$PaymentNotification` and `$PollVote` (non-null parameter checks) |

No test-vector file accompanies this spec. Upstream has no fixed byte
values for any of these encodings. The PNI signature (§4) is XEdDSA with a
random nonce, and the upstream tests (S4) only check sign-then-verify
round trips.

## 1. Conventions shared by all features

### 1.1 Field presence and emission order

The general rules of `content.md` §1 apply (proto2, unpacked repeated
scalars). Two more facts were observed with S5:

- **Presence.** A field that signal-service-java assigns is on the wire,
  even when the value is the type default. This affects the fields listed
  in the table below, which are always emitted when their submessage
  exists. A receiver must not infer "unset" from a default value.
- **Emission order.** The Wire runtime writes the ordinary fields of a
  message in schema declaration order first, and the members of any
  `oneof` after them. Example: in `Content`, `senderKeyDistributionMessage`
  (7) and `pniSignatureMessage` (10) are written **before** the `oneof`
  member (`dataMessage`, `editMessage`, `receiptMessage`, ...). Observed:
  a Content with a READ receipt and a PNI signature serializes as tag 10
  then tag 5. **This corrects `content.md` §3**, which lists 7 and 10 last.
  Order does not matter to parsers; it only matters for byte-identical
  comparisons with captures.

| Submessage | Fields signal-cli always emits, even with default values |
|---|---|
| Quote | `text` (empty string if no quote text), `type` (0 = NORMAL) |
| BodyRange | `start` and `length` (also when 0); `style` = 0 (NONE) when the style is NONE |
| Reaction | `remove` (false for an add) |
| Preview | `title` and `description` (empty strings by default), `date` = 0 |
| PollCreate | `allowMultiple` |
| PinMessage | `pinDurationSeconds` (may be 0) or `pinDurationForever` = true |
| Payment.Notification | `note` (must be non-null, see §3.17) |
| DataMessage | `requiredProtocolVersion` (usually 0), `expireTimerVersion` (see `content.md` §4.1) |

Observed serializations (S5) that show both rules. The hex was produced
by the bundled serializer from hand-built messages with dummy values.

| Message (dummy values) | Bytes, hex | Field order seen |
|---|---|---|
| Quote: id 5, text "", one QuotedAttachment (contentType only), one mention range (0,1), one style range (1,2,NONE), type NORMAL, 16 zero bytes as author | `0805 1a00 220b0a09696d6167652f706e67 32160800 1001 2a10<16×00> 3206 0801 1002 2000 3800 4210<16×00>` | 1, 3, 4, 6, 6, 7, 8 |
| Reaction: emoji "x", remove false, target 7, author 16 zero bytes | `0a0178 1000 2807 3210<16×00>` | 1, 2, 5, 6 |
| Preview: url "u", title "", description "", date 0 | `0a0175 1200 2200 2800` | 1, 2, 4, 5 |
| PinMessage forever | `0a0101 1003 2001` | 1, 2, then oneof 4 |
| Payment notification, note "n", receipt 0x09 | `0a08 1201 6e 0a03 0a0109` | Notification: 2 (note), then oneof 1 |

### 1.2 Service IDs in "...AciBinary" fields

The proto offers string and binary forms for author/mention identifiers
(for example Quote `authorAci` 5 and `authorAciBinary` 8). signal-cli 0.14.7
**only ever sets the binary form**. Its value is the binary service ID of
the recipient that signal-cli resolved:

- 16 raw UUID bytes when signal-cli knows the person's ACI (the normal
  case);
- 17 bytes, a 0x01 prefix plus the 16 PNI bytes, when it only knows a PNI;
- 16 zero bytes when it knows neither (only a phone number whose lookup
  failed).

Consequence on receivers that use this validator (signal-cli itself and
other signal-service-java clients, §7.1): the binary field must parse as
an **ACI** that is not all-zero. A 17-byte PNI form or zero bytes
therefore makes the *whole* message invalid when it is a quote author, a
mention, a reaction author, a story-context author, a poll-vote author,
or a pin/unpin/admin-delete author. A compatible sender should reproduce
signal-cli's choice of value. It should still be aware that a PNI-only
target produces a message the other side drops.

On receive, signal-cli accepts either form. The binary form wins when
both are present; when the binary form is present but does not parse as
an ACI, the string form is tried next.

### 1.3 The common send path for data-message commands

All commands that send a DataMessage (`send`, `sendReaction`,
`remoteDelete`, `sendPollCreate`, `sendPollVote`, `sendPollTerminate`,
`sendPinMessage`, `sendUnpinMessage`, `sendAdminDelete`,
`sendPaymentNotification`, and `updateContact --expiration`) share this
path (S3 `ManagerImpl`, `SendHelper`):

1. **One timestamp `T` per command**, from the monotonic message clock
   (`content.md` §9). It is `DataMessage.timestamp` (also the inner
   DataMessage of an edit), the request timestamp, and the timestamp
   printed by the command. All recipients get the same `T`.
2. **Per recipient**, in the iteration order of the recipient set:
   - *The account itself* (`--note-to-self`, or a recipient that matches
     the own address):
     - *without* `--notify-self` (the default, and always for edits,
       remote deletes, payments and timer updates): no message to the
       recipient. A sent transcript is sent to the own linked devices
       (sync spec). On a single-device account nothing goes on the wire,
       and the result is a success with no devices. The transcript's
       DataMessage carries the own contact's timer and timer version
       (version 1 if there is no own contact entry) and **no profile key**.
     - *with* `--notify-self`: sent as a normal 1:1 message to the own
       account, through the next bullet.
   - *A single other recipient*: the 1:1 path below.
   - *A group*: the group path (stories/sender-key spec). The group's
     timer goes into `expireTimer`. `expireTimerVersion` keeps its default
     of 1, and no profile key is added.
3. **1:1 enrichment** (`SendHelper`):
   - The recipient's contact entry is created or updated to "profile
     sharing enabled, not hidden".
   - `expireTimer` gets the contact's timer (emitted only if > 0), and
     `expireTimerVersion` the contact's timer version (1 for a new
     contact).
   - `profileKey` gets the own 32-byte profile key, unless the contact is
     blocked.
4. **1:1 send parameters**:
   - content hint RESENDABLE;
   - `online = false`;
   - `urgent = true`. Only `send --no-urgent` gives false; every other
     command in this list always sends urgent;
   - sealed sender per `send.md` §3, with the 401 fallbacks and the sticky
     downgrade of `send.md` §3.3–3.4.
5. **PNI signature**: Content field 10 is added if the recipient's
   "needs PNI signature" flag is set (§4). Edits never carry it.
6. **Send log**: the Content is stored for retry-receipt answers (unless
   `--disable-send-log`).
7. **Sent transcript** to the own devices when the server reports
   `needsSync` (sync spec).

### 1.4 `requiredProtocolVersion`

signal-service-java starts at 0 and raises the value to the maximum of
the applicable entries. This **corrects `content.md` §4.3**, which says
"8 for any poll field": only poll *creation* raises it.

| Content of the DataMessage | Raised to |
|---|---|
| at least one mention in the body (§2.1) | 6 (MENTIONS) |
| at least one mention in the quote | 6 |
| `isViewOnce` set | 3 (VIEW_ONCE_VIDEO) |
| `reaction` set | 4 (REACTIONS) |
| `payment` set (notification or activation) | 7 (PAYMENTS) |
| `pollCreate` set | 8 (POLLS) |

Text styles, quotes without mentions, previews, stickers, deletes, story
contexts, poll votes and terminations, pins, unpins and admin deletes do
not raise it. The field is always emitted.

## 2. Wire shapes

Field numbers and types from S1. Each table is in declaration order; the
`oneof` members (if any) are emitted last (§1.1). "Set by signal-cli"
describes outgoing messages; "—" means signal-cli never sets the field
(receive only).

### 2.1 `BodyRange` (DataMessage field 18, Quote field 6)

| # | Name | Type | Set by signal-cli |
|---|------|------|---|
| 1 | start | uint32 | always: start offset in **UTF-16 code units** of the body (or quote text) |
| 2 | length | uint32 | always: length in UTF-16 code units |
| oneof `associatedValue`: | | | |
| 3 | mentionAci | string | — |
| 4 | style | enum Style | for a text-style range |
| 5 | mentionAciBinary | bytes | for a mention: the binary service ID (§1.2) |

`BodyRange.Style`: NONE 0, BOLD 1, ITALIC 2, SPOILER 3, STRIKETHROUGH 4,
MONOSPACE 5.

UTF-16 offsets: a character outside the Basic Multilingual Plane (for
example most emoji) counts as 2 units, and every BMP character counts as
1. signal-cli takes `start` and `length` from the command line unchanged
and does not check them against the text. Ordering in the outgoing list:
first all mention ranges, in command-line order, then all style ranges,
in command-line order. The same holds for the Quote's `bodyRanges`.

### 2.2 `DataMessage.Quote` (field 8)

| # | Name | Type | Set by signal-cli |
|---|------|------|---|
| 1 | id | uint64 | the quoted message's sent timestamp (`--quote-timestamp`) |
| 5 | authorAci | string | — |
| 3 | text | string | `--quote-message`, or `""` (emitted) |
| 4 | attachments | repeated QuotedAttachment | one per `--quote-attachment`, in order |
| 6 | bodyRanges | repeated BodyRange | quote mentions, then quote text styles (§2.1) |
| 7 | type | enum Type | always NORMAL (0), emitted |
| 8 | authorAciBinary | bytes | binary service ID of `--quote-author` (§1.2) |

Field 2 (formerly `authorE164`) is reserved. `Quote.Type`: NORMAL 0,
GIFT_BADGE 1, POLL 2.

`QuotedAttachment`:

| # | Name | Type | Set by signal-cli |
|---|------|------|---|
| 1 | contentType | string | always (the given content type, verbatim) |
| 2 | fileName | string | only if a filename was given |
| 3 | thumbnail | AttachmentPointer | only if a preview file was given: that file, uploaded as a normal attachment (attachments spec) |

### 2.3 `DataMessage.Reaction` (field 16)

| # | Name | Type | Set by signal-cli |
|---|------|------|---|
| 1 | emoji | string | the `-e` value, unvalidated |
| 2 | remove | bool | always: true with `-r`, else false |
| 4 | targetAuthorAci | string | — |
| 5 | targetSentTimestamp | uint64 | `-t` |
| 6 | targetAuthorAciBinary | bytes | binary service ID of the target author (§1.2) |

Field 3 (formerly `targetAuthorE164`) is reserved.

### 2.4 `DataMessage.Delete` (field 17)

| # | Name | Type | Set by signal-cli |
|---|------|------|---|
| 1 | targetSentTimestamp | uint64 | the timestamp of the own message to delete |

### 2.5 `Preview` (DataMessage field 10, repeated; top-level message)

| # | Name | Type | Set by signal-cli |
|---|------|------|---|
| 1 | url | string | `--preview-url` |
| 2 | title | string | `--preview-title`, default `""` (emitted) |
| 3 | image | AttachmentPointer | only with `--preview-image` (uploaded as a normal attachment) |
| 4 | description | string | `--preview-description`, default `""` (emitted) |
| 5 | date | uint64 | always 0 (emitted) |

signal-cli sends at most one preview. It does not check that the URL
occurs in the body, although the help text asks for it. Official clients
may ignore a preview whose URL is not in the body; that was not verified.

### 2.6 `DataMessage.Sticker` (field 11)

| # | Name | Type | Set by signal-cli |
|---|------|------|---|
| 1 | packId | bytes | the pack ID (16 bytes for real packs; signal-cli passes the hex-decoded `--sticker` pack ID) |
| 2 | packKey | bytes | the pack key from the local sticker store |
| 3 | stickerId | uint32 | the sticker ID |
| 4 | data | AttachmentPointer | the sticker image, **re-uploaded** as an attachment from the local pack copy |
| 5 | emoji | string | the sticker's emoji from the pack manifest, only if the manifest has one |

How the pack, its manifest and the local sticker file are obtained, and
how the image is uploaded, belong to the attachments spec. See §3.6 for
the preconditions.

### 2.7 `DataMessage.Contact` (field 9, repeated; shared contacts)

signal-cli 0.14.7 has **no way to send** shared contacts (no CLI option,
no API path). On receive it parses and reports them.

| # | Name | Type |
|---|------|------|
| 1 | name | Name: givenName 1, familyName 2, prefix 3, suffix 4, middleName 5, nickname 7 (all strings; 6 reserved, formerly displayName) |
| 3 | number | repeated Phone: value 1 string, type 2 enum (HOME 1, MOBILE 2, WORK 3, CUSTOM 4), label 3 string |
| 4 | email | repeated Email: same fields and enum as Phone |
| 5 | address | repeated PostalAddress: type 1 enum (HOME 1, WORK 2, CUSTOM 3), label 2, street 3, pobox 4, neighborhood 5, city 6, region 7, postcode 8, country 9 (strings) |
| 6 | avatar | Avatar: avatar 1 AttachmentPointer, isProfile 2 bool |
| 7 | organization | string |

On receive, a missing phone, email or address `type` is read as HOME, and
a missing `isProfile` as false.

### 2.8 `DataMessage.StoryContext` (field 21)

| # | Name | Type | Set by signal-cli |
|---|------|------|---|
| 1 | authorAci | string | — |
| 2 | sentTimestamp | uint64 | the story's sent timestamp |
| 3 | authorAciBinary | bytes | binary service ID of the story author (§1.2) |

### 2.9 `EditMessage` (Content field 11)

| # | Name | Type | Set by signal-cli |
|---|------|------|---|
| 1 | targetSentTimestamp | uint64 | `--edit-timestamp`: the sent timestamp of the **original** message |
| 2 | dataMessage | DataMessage | a complete new DataMessage whose `timestamp` is the new `T` |

When editing an edit, signal-cli uses whatever timestamp the user gives.
It keeps no history, so it cannot map an edit back to the original.

### 2.10 `DataMessage.Payment` (field 20)

| # | Name | Type |
|---|------|------|
| oneof `Item`: | | |
| 1 | notification | Notification |
| 2 | activation | Activation: type 1 enum (REQUEST 0, ACTIVATED 1) |

`Notification`: note 2 string; oneof `Transaction` { mobileCoin 1 =
MobileCoin { receipt 1 bytes } }. Field 1003 reserved. `Payment` has
reserved 1002 and 1003. `Payment.Amount` (mobileCoin { picoMob 1 uint64 })
is defined but never sent or read by signal-cli.

signal-cli sends only notifications (`sendPaymentNotification`). It
never sends activations or requests.

### 2.11 Polls

| Message (DataMessage field) | # | Name | Type | Set by signal-cli |
|---|---|---|---|---|
| PollCreate (24) | 1 | question | string | `-q` |
| | 2 | allowMultiple | bool | always: false with `--no-multi`, else true |
| | 3 | options | repeated string | the `-o` values, in order |
| PollTerminate (25) | 1 | targetSentTimestamp | uint64 | `--poll-timestamp` (the PollCreate message's `T`) |
| PollVote (26) | 1 | targetAuthorAciBinary | bytes | binary service ID of `--poll-author` (§1.2) |
| | 2 | targetSentTimestamp | uint64 | `--poll-timestamp` |
| | 3 | optionIndexes | repeated uint32 | the `-o` indexes, in order, unpacked |
| | 4 | voteCount | uint32 | `--vote-count` |

`voteCount` is the voter's own counter. The help asks the user to
increase it with every vote, and signal-cli does not track it.

### 2.12 Pins and admin delete

| Message (DataMessage field) | # | Name | Type | Set by signal-cli |
|---|---|---|---|---|
| PinMessage (27) | 1 | targetAuthorAciBinary | bytes | binary service ID of the target author |
| | 2 | targetSentTimestamp | uint64 | target timestamp |
| | oneof `pinDuration`: 3 | pinDurationSeconds | uint32 | when `-d` ≥ 0: that value (0 is allowed and emitted) |
| | oneof `pinDuration`: 4 | pinDurationForever | bool | when `-d` < 0 (default −1): true |
| UnpinMessage (28) | 1 | targetAuthorAciBinary | bytes | as above |
| | 2 | targetSentTimestamp | uint64 | as above |
| AdminDelete (29) | 1 | targetAuthorAciBinary | bytes | as above |
| | 2 | targetSentTimestamp | uint64 | as above |

### 2.13 Receive-only DataMessage parts

| Field | Shape | signal-cli |
|---|---|---|
| groupCallUpdate (19) | eraId 1 string | reported (plain text only); never sent by these commands |
| giftBadge (22) | receiptCredentialPresentation 1 bytes | validated (§7.1), otherwise ignored; never reported |
| flags bit 1 (legacy END_SESSION, reserved in S1) | — | ignored. signal-cli never resets a session because of it; "is end session" is always reported false |

### 2.14 `PniSignatureMessage` (Content field 10)

| # | Name | Type | Value |
|---|------|------|---|
| 1 | pni | bytes | the account's own PNI as **16 raw UUID bytes** (no prefix byte) |
| 2 | signature | bytes | 64-byte XEdDSA signature (§4.1) |

## 3. What signal-cli sends, per feature

### 3.1 Body and long text

- The CLI text (`-m`, stdin, or empty) goes to `body`. An empty text
  leaves `body` absent.
- **Long text.** If the text's UTF-8 encoding is longer than **2000
  bytes**:
  1. `body` gets the longest prefix of whole characters (Unicode code
     points; a surrogate pair is never split) whose UTF-8 encoding is at
     most 2000 bytes.
  2. The **entire** text, not just the remainder, is UTF-8 encoded and
     uploaded as an attachment with content type `text/x-signal-plain`.
     The attachment has no file name, and its size is the full UTF-8
     length. Upload and pointer details are in the attachments spec.
  3. This attachment is the **first** element of `attachments`. Any
     `-a` attachments follow it in their given order. `--voice-note`
     does not apply to it.
- Mention and style ranges are **not** adjusted. They keep referring to
  offsets in the full text, so they may point past the end of the
  truncated `body`. Receivers allow out-of-bounds ranges when a
  long-text attachment is present (§7.1).
- The 2048-byte body limit of `content.md` §4.1 can therefore never
  trigger for CLI sends. That check also exists only for plain data
  messages, not for edits.
- Long-text handling is identical for edits (§3.9).

### 3.2 Mentions (`--mention start:length:recipient`)

- Each mention becomes a BodyRange with `start`, `length` and
  `mentionAciBinary` (§2.1, §1.2).
- The recipient is resolved like any recipient. An unknown number
  triggers a lookup. An unregistered number aborts the command with
  "The user … is not registered." before anything is sent.
- `requiredProtocolVersion` becomes at least 6.
- Mentions are allowed in 1:1 messages. Receivers accept them and only log
  a warning.

### 3.3 Text styles (`--text-style start:length:STYLE`)

- Each style becomes a BodyRange with `start`, `length` and `style`
  (NONE is sent as the value 0, emitted).
- Style ranges follow all mention ranges in the list.
- No protocol-version bump.

### 3.4 Quote (`--quote-*`)

- Built only when `--quote-timestamp` is given; field values in §2.2.
- The author is resolved like a recipient. An unregistered author aborts
  the command, as for mentions.
- Quote mentions raise `requiredProtocolVersion` to 6.
- A preview file for a quoted attachment is uploaded as a full attachment
  at send time (attachments spec). signal-cli does not resize it or
  generate a thumbnail.
- signal-cli does not check that the quoted message exists or that the
  text matches.

### 3.5 Link preview

At most one Preview, filled as in §2.5. The optional image is uploaded
first.

### 3.6 Sticker (`--sticker packIdHex:stickerId`)

Preconditions, checked in this order. Failure aborts with "Failed to send
sticker: <reason>" (exit 1):

1. The pack ID must be in the local sticker store (installed or seen in
   an earlier incoming sticker). Otherwise: `Sticker pack not found`.
2. The pack manifest is loaded, fetching it if needed (attachments spec).
   If `stickerId` ≥ the number of stickers in the manifest:
   `Sticker id not part of this pack`.
3. The sticker image must exist in the local pack copy. Otherwise:
   `Missing local sticker file`.

Fields as in §2.6. A sticker message may also carry a body, attachments
and the other features. signal-cli does not forbid combining them.

### 3.7 Story reply (`--story-timestamp`, `--story-author`)

- Adds `storyContext` with the story author's binary service ID and the
  story timestamp (§2.8). Everything else is a normal DataMessage (body,
  quote, and so on).
- For a 1:1 recipient it goes through the normal 1:1 path, with request
  parameter `story=false`.
- A story reply alone (no text, attachment, sticker or quote) is refused
  by the empty-message check (`spec/surface/cli/send.md`).
- Story *reactions* are `sendReaction --story` (§3.11).

### 3.8 View-once (`--view-once`)

- Sets `isViewOnce = true` and raises `requiredProtocolVersion` to at
  least 3.
- Without the flag, `isViewOnce` is absent (never false).
- signal-cli does not restrict which attachments may be view-once, or
  require one.
- The sent transcript to the own devices drops the attachments of a
  view-once message (sync spec).

### 3.9 Edit (`send --edit-timestamp TS`)

- signal-cli builds a **brand-new DataMessage** from the command-line
  options, exactly as for a normal `send`: body or long text,
  attachments, mentions, styles, quote, sticker, preview, story context,
  view-once.
- **Nothing is carried over from the original message.** signal-cli
  stores no message history. A user who wants to keep the quote,
  attachments or styles must pass them again.
- The new DataMessage gets its own fresh timestamp `T`, the contact's
  timer and timer version, and the profile key (§1.3). It is wrapped as
  `Content.editMessage` with `targetSentTimestamp = TS` (§2.9). There is
  no `Content.dataMessage`.
- Differences from a normal send:
  - no PNI signature, even if the recipient needs one;
  - `--notify-self` is ignored, so a self recipient always gets only a
    sent transcript (whose `editMessage` holds the edit);
  - no 2048-byte body check.
- Content hint RESENDABLE, `online = false`, urgent unless `--no-urgent`.
- **Limits:** signal-cli enforces none. It does not check the age of the
  target, the number of earlier edits, or that the target was sent by this
  account. Limits that official clients apply on receive (time window,
  edit count) are not part of this source set.

### 3.10 Expiration timer

- **Carried on every 1:1 data message** (§1.3): `expireTimer` = the
  contact's timer in seconds, emitted only if > 0; `expireTimerVersion` =
  the contact's version, always emitted.
- **Timer update** (`updateContact RECIPIENT --expiration SECONDS`):
  1. The local contact entry gets the new timer. Its version is
     incremented by 1 (capped at the largest 32-bit signed integer), or
     set to 1 if the contact did not exist. If the timer equals the stored
     one, nothing is stored and the version stays the same.
  2. A DataMessage is sent through the common path (§1.3) with
     `flags = 2` (EXPIRATION_TIMER_UPDATE) and no body. It carries the new
     `expireTimer` (absent when the new value is 0, i.e. timer disabled),
     the stored `expireTimerVersion`, the profile key, `T` and
     `requiredProtocolVersion = 0`.
  3. A storage-service sync follows (out of scope).
- Group timers are changed through group updates (stories/sender-key
  spec).
- Receive rules: §7.2.

### 3.11 Reaction (`sendReaction`)

- A DataMessage whose only feature is `reaction` (§2.3), plus the common
  enrichment (§1.3). `requiredProtocolVersion` is 4.
- With `--story`, a `storyContext` is added whose author is the reaction's
  target author and whose `sentTimestamp` is the target timestamp.
- Removing a reaction uses the same emoji with `remove = true`. signal-cli
  does not remember which emoji was sent.
- The target author is the `-a` value, required on the CLI. Over JSON-RPC,
  where it may be omitted, the single recipient is used when the
  recipient set has exactly one individual entry.

### 3.12 Remote delete (`remoteDelete`)

- A DataMessage with only `delete` (§2.4) plus the common enrichment.
- `--notify-self` does not exist here, so a self recipient gets only a
  transcript.
- **Local bookkeeping before sending:** for each recipient, signal-cli
  removes the send-log entries for `targetSentTimestamp` belonging to that
  recipient, or to that group for a group recipient. Afterwards a retry
  receipt for the deleted message can no longer cause it to be resent.
  - For an individual recipient given by phone number or username, the
    lookup uses the resolved service ID.
  - If the lookup fails as unregistered, the cleanup is skipped silently.
    The send itself then reports "unregistered".

### 3.13 Polls

- **`sendPollCreate`:** a DataMessage with only `pollCreate`;
  `requiredProtocolVersion` 8. Local checks before sending: 2–10 options,
  none empty, none longer than 100 UTF-16 code units. The question is
  **not** checked locally. Receivers drop the whole message if the
  question is blank or longer than 200 UTF-16 code units (§7.1).
- **`sendPollVote`:** a DataMessage with only `pollVote`;
  `requiredProtocolVersion` stays 0. The option indexes are not checked
  locally. Receivers drop the message if more than 10 indexes are given.
  Without any `-o`, the reference crashes before sending (null list; see
  `spec/surface/cli/sendPollVote.md`).
- **`sendPollTerminate`:** a DataMessage with only `pollTerminate`;
  version 0.

signal-cli keeps no poll state. It does not check that the poll exists,
that the voter is allowed to vote, or that the terminator created the
poll.

### 3.14 Pin and unpin

- **`sendPinMessage`:** a DataMessage with only `pinMessage` (§2.12).
- **`sendUnpinMessage`:** a DataMessage with only `unpinMessage`.
- With `--story`, both add a `storyContext` built from the target author
  and target timestamp, as in §3.11.
- The target author defaults to the single recipient when it is omitted
  (JSON-RPC) and the recipient set has exactly one individual entry.
- Version 0.

### 3.15 Admin delete (`sendAdminDelete`)

- **Groups only.** Without a group ID the command fails: "Admin delete
  requires group IDs".
- The DataMessage has `adminDelete` (§2.12) and goes through the group
  path. With `--story`, a `storyContext` is added as in §3.11. Version 0.
- signal-cli does not check locally that it is an admin. Receivers drop
  an admin delete from a non-admin (§7.1).

### 3.16 End session (`send -e`)

signal-cli 0.14.7 **sends nothing** for an end session. For each
individual recipient (groups and unregistered recipients are skipped
silently), it archives all local sessions with that person. That
covers both its ACI and PNI addresses, in both the account's ACI and PNI
session stores. The next message to that person then starts a new session
(PreKey message). The command prints nothing. The legacy END_SESSION flag
(value 1) is reserved in S1 and never set. This corrects
`spec/surface/cli/send.md`, which says "an end-session message is sent".

### 3.17 Payment notification (`sendPaymentNotification`)

- A 1:1 DataMessage with only `payment.notification`: `note` = the
  `--note` text, `mobileCoin.receipt` = the base64-decoded `--receipt`
  bytes. signal-cli does not interpret the receipt.
- `requiredProtocolVersion` is 7. The common enrichment applies; self is
  sent a transcript only.
- Without `--note`, the reference crashes before sending: the note must
  not be null (S5 shows a non-null check on that constructor parameter;
  see `spec/surface/cli/sendPaymentNotification.md`).

## 4. PNI signature

### 4.1 Construction

The signature proves that the account's PNI identity key vouches for its
ACI identity key (S2 `createPniSignatureMessage`, S4 `identity_key.rs`):

- **Signing key:** the account's **PNI** identity private key.
- **Signed message:** the concatenation, 85 bytes in total, of
  1. 32 bytes of `0xFF`;
  2. the 20 ASCII bytes `Signal_PNI_Signature`;
  3. the account's **ACI** identity public key in its 33-byte serialized
     form (`0x05` followed by the 32-byte Curve25519 key).
- **Algorithm:** XEdDSA over Curve25519 (`spec/crypto/xeddsa.md`), with
  64 fresh random bytes as the nonce input. The signature is 64 bytes and
  differs on every call. libsignal signs the three parts as one message;
  "multipart" here means only that they are concatenated.
- **Message:** `PniSignatureMessage{ pni = own PNI as 16 raw bytes,
  signature }` (§2.14).

Verification by a receiver uses the claimed PNI's identity public key
on the same 85-byte message. It is not symmetric: swapping the two keys
fails.

### 4.2 When signal-cli includes it (send side)

signal-cli keeps a per-recipient boolean **"needs PNI signature"** in its
recipient store (default false). When the flag is true for the target
recipient, Content field 10 is added to these 1:1 sends:

- data messages through the common path (§1.3): `send` (but not
  `send --edit-timestamp`), reactions, remote deletes, polls, pins,
  unpins, payment notifications and timer updates;
- profile-key update messages (`content.md` §4.4);
- receipts (automatic delivery or read receipts, and `sendReceipt`).

It is **never** added to edits, typing messages, null messages, retry
receipts (plaintext content), call messages, sync messages, or group
(sender-key) sends. Resends answering a retry receipt reuse the logged
Content as it was first sent.

### 4.3 When the flag is set and cleared

The flag is updated for every incoming envelope whose content passes
decryption and validation, is not from a blocked contact, and is not a
group message from an unauthorized sender. It is updated from the
envelope's **destination service ID**, before any automatic reply is sent:

- destination = the account's **PNI** → the flag for the sender is set to
  **true**;
- destination = the account's **ACI** → the flag is set to **false**.

Consequences:

- Someone who found the account by phone number and wrote to its PNI
  gets PNI signatures on everything signal-cli sends back. That includes
  the profile-key update triggered by their message (messages to a PNI are
  normally unsealed, so the automatic reply is a profile-key update rather
  than a delivery receipt), and every reply and receipt.
- The flag is cleared by the first message from that sender addressed
  to the ACI. In practice that happens once the peer has verified the
  signature and learned the ACI.
- It is not affected by server delivery receipts (no content), by
  envelopes that fail to decrypt or validate, or by contents that resolve
  to nothing, such as a bare NullMessage (§7.1).

### 4.4 Receive side

When an incoming Content carries a `pniSignatureMessage`:

1. **Validation** (§7.1): `pni` must parse as a PNI (16 raw bytes, or 17
   with the PNI prefix) that is not all-zero, and `signature` must be
   present. A present but empty signature passes validation. This refines
   `content.md` §7, which says "non-empty". On failure the whole Content
   is dropped silently.
2. **Verification** needs:
   - a known sender address (from the envelope for unsealed messages, or
     the sealed-sender certificate);
   - an identity key already stored for the sender's ACI and one for the
     claimed PNI;
   - the ACI and PNI must differ.

   The PNI identity key must verify the signature over the stored ACI
   identity key (§4.1).
3. **On success** signal-cli treats the ACI, the PNI and the sender's
   phone number (if known) as one trusted recipient. It merges recipient
   entries if needed and marks the recipient "PNI signature verified",
   which rotates its storage ID.
4. If verification is skipped or fails, the sender is resolved normally;
   a failure is only logged at debug level. In both cases the rest of the
   Content is handled normally.
5. A Content with both an `editMessage` and a `pniSignatureMessage` is
   treated as a PNI-signature-only content: the edit is **lost**,
   neither handled nor reported. This is a quirk of S2's content
   conversion, which checks for a PNI signature before an edit. A Content
   with a PNI signature and a dataMessage, receipt, typing, sync, call,
   story or decryption-error message is handled normally.

## 5. Null messages: when signal-cli sends them

The construction is in `content.md` §6: padding is a serialized
DataMessage holding a random base64 body. Content hint IMPLICIT,
`urgent = false`, `online = false`, no PNI signature. The triggers in
0.14.7 are:

1. **Session renewal after an undecryptable message.** An incoming
   envelope can fail with an invalid message, invalid key, invalid key ID
   or missing session. When the sender is not blocked and both sender and
   destination are valid service IDs, signal-cli queues one renewal per
   sender. The renewal archives the sessions with the sender in the
   store of the destination identity (ACI or PNI). It then sends a null
   message to the sender with the normal sealed-sender decision
   (`send.md` §3). That send has no session, so it fetches a bundle and
   produces a PreKey message, which re-establishes the session. A retry
   receipt is also sent (`pipe.md`), but not when the failed message came
   from this very device.
2. **Session renewal after a retry receipt.** An incoming
   DecryptionErrorMessage addressed to this device can carry a ratchet key
   equal to the current one of the session with that sender device. If
   the send log has **no** matching entries, a renewal as in trigger 1
   follows (archive plus null message). If it has entries, the sessions
   are archived and those entries resent instead, with no null message.
3. **Identity verification sync** (`trust` on a multi-device account).
   When the trust level of a contact's key changes, signal-cli sends a
   Verified sync to its own devices. That send first sends a null message
   to the **contact**: unsealed, with timestamp = current wall-clock time.
   If the response says `needsSync`, it then sends the sync. The sync's
   `Verified.nullMessage` field holds the same NullMessage bytes. On a
   single-device account nothing is sent, so no null message either.

Null messages are never sent for plain text sends, end sessions (§3.16)
or session setup. On receive, a Content holding only `nullMessage` (with
or without a sender-key distribution message) produces no output and no
side effects. Not even the needs-PNI-signature flag changes.

## 6. Wire result of each send-family command

Columns:

- **Content**: the top-level shape for a 1:1 recipient. "DM" means
  `Content.dataMessage`, and "+PNI" means field 10 when the flag of §4.2
  is set.
- **Sealed**: "per §3" means sealed when `send.md` §3 allows it.
- **Transcript**: sent to own devices only when `needsSync` (or the
  account is multi-device) and the recipient is not self.

Group recipients go through the group path; not repeated here.

| Command | Content (1:1) | DataMessage features | Hint / urgent / online | Sealed | Transcript | Local bookkeeping |
|---|---|---|---|---|---|---|
| `send` | DM (+PNI) | §3.1–3.8 as given; enrichment §1.3 | RESENDABLE / true, false with `--no-urgent` / false | per §3 | yes (DM; view-once attachments stripped) | contact set to profile-sharing and not hidden; send log; sticky downgrade |
| `send --edit-timestamp` | editMessage (never +PNI) | §3.9 | RESENDABLE / as above / false | per §3 | yes (editMessage) | as `send` |
| `send -e` | nothing sent | — | — | — | — | archives all sessions with each individual recipient (§3.16) |
| `sendReaction` | DM (+PNI) | reaction (+storyContext with `--story`); version 4 | RESENDABLE / true / false | per §3 | yes | as `send` |
| `remoteDelete` | DM (+PNI) | delete | RESENDABLE / true / false | per §3 | yes | send-log entries for the target removed (§3.12) |
| `sendPollCreate` | DM (+PNI) | pollCreate; version 8 | RESENDABLE / true / false | per §3 | yes | as `send` |
| `sendPollVote` | DM (+PNI) | pollVote | same | per §3 | yes | as `send` |
| `sendPollTerminate` | DM (+PNI) | pollTerminate | same | per §3 | yes | as `send` |
| `sendPinMessage` | DM (+PNI) | pinMessage (+storyContext) | same | per §3 | yes | as `send` |
| `sendUnpinMessage` | DM (+PNI) | unpinMessage (+storyContext) | same | per §3 | yes | as `send` |
| `sendAdminDelete` | group path only | adminDelete (+storyContext) | RESENDABLE / true | group path | group path | send log |
| `sendPaymentNotification` | DM (+PNI) | payment.notification; version 7 | RESENDABLE / true / false | per §3 | yes | as `send` |
| `updateContact --expiration` | DM (+PNI) | flags 2, expireTimer, version (§3.10) | RESENDABLE / true / false | per §3 | yes | contact timer and version stored first; storage sync |
| `sendTyping` | typingMessage only | — | IMPLICIT / false / **true** | per §3 | no | none; one timestamp `T` from the message clock for all recipients |
| `sendReceipt` | receiptMessage (+PNI) | — | IMPLICIT / false / false | per §3 | read/viewed sync to own devices if the sender's ACI is known (sync spec) | envelope timestamp `T` from the message clock (automatic receipts use wall-clock time); send log (IMPLICIT, not urgent); skipped if read receipts are configured off (`content.md` §5.1) |
| `sendMessageRequestResponse` | **nothing sent to the other party** | — | — | — | MessageRequestResponse sync to own devices only (sync spec); nothing on a single-device account | ACCEPT turns the contact's profile sharing on, DELETE turns it off (individual recipients with a known service ID; recipients without one are skipped) |

Common to all DM rows: profile key unless the contact is blocked,
contact timer and version, `T` as `DataMessage.timestamp`, and
`requiredProtocolVersion` emitted (§1.4).

## 7. Receive side

### 7.1 Validation (extends `content.md` §7)

Every decrypted Content goes through three stages. What happens to an
offending message depends on the stage:

- **(V) validator:** the message is dropped silently. There is no output,
  no receipt, no side effect, only a log warning.
- **(X) conversion error:** the envelope is still reported, with an
  exception (`Exception: <msg> (InvalidMessageStructureException)` in
  plain text; `exception` in JSON). There are no side effects and no
  receipt.
- **(F) field dropped:** the message is handled and reported without that
  one feature.

Applies to DataMessages and, where noted, the inner DataMessage of an
EditMessage.

| Rule | Stage |
|---|---|
| `requiredProtocolVersion` > 8 | V (logged as unsupported) |
| quote present and its author does not parse as a non-zero ACI | V |
| quote body range has a mention ID that does not parse as a non-zero ACI | V |
| quote style range is missing `start` or `length` | V |
| quote body range has a negative bound or start + length > UTF-16 length of the quote text (missing start/length count as 0) | V |
| shared-contact avatar pointer has neither CDN ID nor CDN key | V |
| preview image pointer has neither CDN ID nor CDN key | V (also for edits) |
| body range mention ID invalid as above | V (also for edits) |
| style range is missing `start` or `length` | V (also for edits) |
| body range out of bounds of the body's UTF-16 length; **allowed** when any attachment has content type `text/x-signal-plain` | V (also for edits) |
| sticker present with a missing or invalid `data` pointer | V |
| reaction without `targetSentTimestamp`, or with an author that is not a valid non-zero ACI | V |
| delete without `targetSentTimestamp` | V |
| storyContext author invalid, or `sentTimestamp` missing | V |
| giftBadge without a presentation, or with an unparsable one | V |
| any attachment pointer with neither CDN ID nor CDN key | V (also for edits) |
| pollCreate: question missing, blank or longer than 200 UTF-16 units; fewer than 2 options; any option longer than 100 units; `allowMultiple` missing | V |
| pollTerminate without `targetSentTimestamp` | V |
| pollVote: author binary not a valid non-zero ACI, timestamp missing, `voteCount` missing, or more than 10 indexes | V |
| pinMessage: author invalid, timestamp missing, or neither `pinDurationSeconds` nor `pinDurationForever = true` | V |
| unpinMessage, adminDelete: author invalid or timestamp missing | V |
| EditMessage without `dataMessage` or `targetSentTimestamp`, inner timestamp missing or ≠ envelope timestamp, or inner version > 8 | V |
| envelope flagged `story` but the Content has none of: SKDM, storyMessage, a DataMessage with storyContext **and** groupV2, or a DataMessage with `delete` | V |
| payment with neither notification nor activation; notification without `mobileCoin.receipt`; activation without `type` | X |
| quote author missing | F (the quote is dropped; the validator already rejects an invalid author, so this only matters in theory) |
| reaction without `emoji` | F |
| sticker missing packId, packKey or stickerId (with valid data) | F |
| pollCreate, pollVote, pin, unpin or admin delete missing a required part | F (but V catches every case first) |
| body range with neither a valid mention ID nor a style | F (ignored) |
| mentions or styles while `body` is absent | F (both lists empty) |

Group-level checks, which act after validation, before handling:

- A DataMessage carrying `adminDelete` in a group is dropped silently
  when the group is unknown or the sender is not an admin. Without a
  group context, an admin delete is reported like any other message.
- Messages from blocked contacts or blocked groups, and group messages
  from non-members, are dropped silently (stories/sender-key spec).
- In announcement-only groups, non-admins may not send body, attachments,
  quote, previews, mentions or sticker.

### 7.2 Storage side effects

signal-cli **keeps no message history**. Reactions, remote deletes,
admin deletes, edits, pins, unpins, polls, votes, view-once and story
replies are **only reported** (`_received-message-output.md`). Nothing
local is changed, looked up or checked. For example, a reaction to an
unknown message, a delete of a message never received, or an edit of
someone else's message is reported the same way as any other.

The local effects of an accepted DataMessage are below. They apply to
`Content.dataMessage`, to the inner DataMessage of an `editMessage`, and
to the DataMessage of a sent transcript.

| Trigger | Effect |
|---|---|
| any data or edit message | the "needs PNI signature" flag (§4.3); a DELIVERY receipt if the message arrived sealed, or else a profile-key update to the sender (primary device, contact not blocked, profile sharing on) (`content.md` §4.4, §5.1); a READ receipt with `receive --send-read-receipts` |
| `profileKey` present | stored as the sender's profile key if exactly 32 bytes (`content.md` §4.4) |
| `isExpirationUpdate` (flag bit 2) **or a non-empty body**, in a 1:1 conversation | the conversation partner's contact timer is updated by version: see the list below this table |
| attachments, preview images, quote thumbnails, shared-contact avatars | downloaded unless `--ignore-attachments` (attachments spec) |
| a `text/x-signal-plain` attachment (pointer) | its content is read as UTF-8 and **replaces** the reported body. With `--ignore-attachments`, it is still fetched into memory for this purpose |
| `sticker` | the pack (ID, key) is added to the sticker store if unknown; a pack download is queued unless `--ignore-stickers` (attachments spec) |
| mentions, quote authors, reaction, poll, pin and admin-delete authors, story-context authors | the referenced ACIs are entered in the recipient store as recipients (no contact entry) when they are first rendered |
| groupV2 context | group handling (stories/sender-key spec) |
| giftBadge, payment, groupCallUpdate | nothing |

The timer update in the third row:

- the partner is the sender, or the destination for a sent transcript;
- the incoming timer is `expireTimer`, 0 if absent;
- the incoming version is `expireTimerVersion`, 1 if absent;
- with no contact entry for the partner, one is created with that timer
  and version;
- with an entry, nothing changes if the stored timer equals the incoming
  one, or if the stored version is ≥ the incoming version; otherwise both
  are replaced.

So a plain message without a timer from a known contact never clears a
timer: its default version of 1 never exceeds the stored one. In groups,
a v2 group's timer comes from the group state, not from messages.

### 7.3 What is reported

Field-to-output mapping is in `_received-message-output.md`. Points that
depend on this file:

- Mentions are reported from body ranges whose mention ID parses as an
  ACI. Text styles are reported from ranges with a style. Both are only
  reported when a body is present.
- A long-text attachment's content replaces `message`/`Body:`.
- Payment is reported only for notifications (`note`, `receipt`);
  activations produce no payment output.
- A pin is reported with `pinDurationSeconds` −1 when the duration is
  forever, 0 when no duration is given, and the seconds value otherwise.
- "Is end session" is always false (§2.13).
- A quote is reported only if its author is a valid ACI (always true
  after validation).

## 8. Corrections to other specs

| Spec | Statement | Correction |
|---|---|---|
| `content.md` §3 | "Declaration (emission) order: 1, …, 11, then 7, 10" | Fields 7 and 10 are emitted **before** the `oneof` member (§1.1). |
| `content.md` §4.3 | "8 for any poll field" | Only `pollCreate` raises it to 8; votes and terminations stay at their other values (§1.4). |
| `content.md` §5.1 | receipt envelope timestamp is wall-clock time | True for automatic receipts; `sendReceipt` uses the monotonic message clock value `T` (§6). |
| `content.md` §7 | PNI signature must be non-empty | It must be present; empty passes validation (§4.4). |
| `content.md` §10 | "needs PNI signature" and null-message triggers are open | Specified in §4.3 and §5. |
| `spec/surface/cli/send.md` | `-e`: "An end-session message is sent to them" | Nothing is sent; sessions are archived locally (§3.16). |

## Gaps

- **Unpaired UTF-16 surrogates** in a text longer than 2000 bytes:
  behavior of the long-text split was not traced. The UTF-8 encoder would
  stop at the malformed character.
- **Official-client behavior** on receive (edit time window and count,
  preview URL must occur in the body, PNI-only mentions) is outside this
  source set. Only signal-service-java's validator and signal-cli's
  handling are specified.
- **Shared-contact avatar edge case**: a `Contact.avatar` present with
  its inner `avatar` pointer absent passes the validator. Conversion then
  probably fails with a conversion error (stage X), but this was not
  confirmed.
- The exact bytes of the long-text, sticker, preview-image and
  quote-thumbnail attachment pointers belong to the attachments spec.
- Group sends (`sendAdminDelete`, and every command with `-g`) are only
  described up to the DataMessage. The fan-out, sender-key use, transcript
  and send-log details are in the stories/sender-key spec.
- No fixed test vectors exist upstream for any encoding in this file
  (§Sources). The hex in §1.1 shows serializer behavior on dummy values;
  it is not an interoperability vector.
