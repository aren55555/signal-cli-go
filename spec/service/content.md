# Content messages: Content, DataMessage, receipts, typing, padding

Spec role: prose and tables only. This file covers the **plaintext** that
goes inside a 1:1 Signal message: the `Content` protobuf, the messages a
plain text send, a receipt and a typing indicator need, the transport
padding applied before encryption, and exactly which fields signal-cli
0.14.7 sets so that our sends look the same on the wire.

Not covered here (see the referenced files):

- How the padded plaintext is encrypted: `spec/protocol/double-ratchet.md`,
  `spec/protocol/pqxdh.md`, and the sealed-sender wrapping in
  `spec/protocol/sealed-sender.md`.
- The REST/WebSocket request that carries the ciphertexts (the
  `OutgoingPushMessageList` JSON with its `online` and `urgent` keys,
  sealed-sender access headers, 401 fallback): `spec/service/send.md`.
- The `Envelope` wrapper the server delivers: `spec/protocol/capture.md`
  section 3; the receive loop and when receipts are triggered:
  `spec/service/pipe.md`.

## Sources

| Ref | What | URL @ version |
|-----|------|---------------|
| S1 | Wire-formatted `SignalService.proto` packaged inside the reference jar `/work/third_party/signal-cli-0.14.7/lib/signal-service-java-2.15.3_unofficial_151.jar` (jar sha256 `ce83b1a51983ddb3bd456609ea1957371db56048637309754f42a502a4d14b33`). Field numbers, types and enum values below come from it. Field *emission order* was checked with `javap -c` on the Wire adapter class `DataMessage$Companion$ADAPTER$1` in the same jar. | jar from the signal-cli 0.14.7 release |
| S2 | signal-service-java fork | https://github.com/Turasa/libsignal-service-java @ tag `v2.15.3_unofficial_151` = commit `66956cb4a0394d50cee3590209baf58312084a44`: `lib/libsignal-service/src/main/java/org/whispersystems/signalservice/internal/push/PushTransportDetails.java`, `.../api/SignalServiceMessageSender.java`, `.../api/crypto/{SignalServiceCipher,EnvelopeContent}.java`, `.../api/messages/{SignalServiceDataMessage,EnvelopeContentValidator}.kt`, `.../internal/util/Util.java`; test `lib/libsignal-service/src/test/java/org/whispersystems/signalservice/api/push/PushTransportDetailsTest.kt` |
| S3 | signal-cli | https://github.com/AsamK/signal-cli @ tag `v0.14.7` = commit `b01b6b370dc063599a1a2b9fde0f5ff4e2d78fe8`: `lib/src/main/java/org/asamk/signal/manager/{internal/ManagerImpl, helper/SendHelper, helper/IncomingMessageHandler, helper/UnidentifiedAccessHelper, actions/SendReceiptAction, api/Contact, config/ServiceConfig}.java`; `src/main/java/org/asamk/signal/commands/SendCommand.java` |
| S4 | libsignal | https://github.com/signalapp/libsignal @ tag `v0.99.1` = commit `97801d22dcf9f5bf714f7b8fa3212cdc973ae1c8`: `rust/protocol/src/protocol.rs` (`PlaintextContent`, `DecryptionErrorMessage`) |

Test-vector data: `testdata/vectors/content_padding.json` (padded-length
assertions from S2's test, plus derived boundary cases).

## 1. Encoding conventions

- All messages are **proto2**. A field is on the wire if and only if the
  sender set it, even when its value equals the type default. This matters:
  several fields below are deliberately sent with value 0 (for example
  `requiredProtocolVersion = 0`, `ReceiptMessage.type = DELIVERY (0)`,
  `TypingMessage.action = STARTED (0)`), and receivers reject some messages
  whose enum field is *absent* (section 7).
- Repeated scalar fields are **not packed** (proto2 default; S1 declares no
  `packed` option). Example: each `ReceiptMessage.timestamp` element is its
  own tag-2 varint. Receivers should accept both encodings.
- The Wire runtime used by signal-service-java writes fields in the order
  they are **declared** in the schema, not in tag order. The tables below
  list fields in declaration order, so a byte-identical encoder emits them
  top to bottom, skipping unset ones. Byte identity is not needed for
  interoperability; the padding (section 2) hides small length differences
  anyway.

## 2. Transport padding of the Content plaintext

Every `Content` that is encrypted with a session cipher (Double Ratchet
`SignalMessage`/`PreKeySignalMessage`, whether or not the result is then
wrapped in sealed sender) is padded first (S2 `PushTransportDetails`,
`EnvelopeContent.Encrypted`). Sender-key group messages use the same
helper; they are out of scope here.

### Padding on send

Let `L` be the length in bytes of the serialized `Content`.

1. Compute the padded length `P = ceil((L + 2) / 80) * 80 - 1`.
   Equivalently: the smallest multiple of 80 that is at least `L + 2`,
   minus one.
2. The padded plaintext is `P` bytes: the `L` content bytes, then one byte
   **0x80**, then `P - L - 1` bytes of **0x00**.

The block size is **80** bytes. The "minus one" exists because the Double
Ratchet message cipher (AES-256-CBC with PKCS#7, see
`spec/protocol/double-ratchet.md`) then adds its own padding: since every
`P` is 15 mod 16 (79, 159, 239, ...), PKCS#7 adds exactly one byte, and the
CBC ciphertext is a multiple of 80 bytes as well.

Examples (from S2's test and the formula):

| Serialized Content length `L` | Padded length `P` | Zero bytes after 0x80 |
|---|---|---|
| 0 | 79 | 78 |
| 77 | 79 | 1 |
| 78 | 79 | 0 |
| 79 | 159 | 79 |
| 157 | 159 | 1 |
| 158 | 159 | 0 |
| 159 | 239 | 79 |
| 238 | 239 | 0 |
| 239 | 319 | 79 |

Put differently: `P` is the smallest value of the form 80k − 1 that can hold the content plus the 0x80 terminator. At `L = 78`, `158`, `238`, ... the terminator is the last byte and there are no zero bytes; one more content byte moves to the next block.

The padding is all-deterministic: no random bytes.

### Unpadding on receive

After the session cipher (and, for sealed sender, the sealed-sender layer)
returns the plaintext, the receiver strips padding (S2
`SignalServiceCipher`, `PushTransportDetails`):

- Scan backwards from the last byte. Skip bytes equal to 0x00. The first
  byte that is not 0x00:
  - if it is 0x80, the content is everything before that byte;
  - otherwise the padding is malformed. signal-service-java then logs a
    warning and uses the **whole, unstripped** buffer as the Content bytes
    (it does not reject the message).
- If the buffer is empty or consists only of 0x00 bytes, the result is an
  empty Content (which then fails validation, section 7).

So a receiver must accept any amount of 0x00 padding, including none, and
need not check the block size.

### Interaction with sealed sender and libsignal

- Padding is applied **inside** the session-cipher plaintext. Sealed
  sender (`spec/protocol/sealed-sender.md`) wraps the already-encrypted
  `CiphertextMessage` in an `UnidentifiedSenderMessageContent`, and adds no
  further content padding of its own.
- libsignal itself never adds or removes this padding for normal messages;
  it is purely an application-layer convention.
- **Exception, plaintext content:** a `DecryptionErrorMessage` is sent
  unencrypted as libsignal `PlaintextContent` (envelope type
  PLAINTEXT_CONTENT 8, or inside sealed sender with content hint IMPLICIT).
  Its serialized form is built by libsignal (S4) as: one byte **0xC0**
  (identifier), then a serialized `Content` whose only field is
  `decryptionErrorMessage` (field 8), then one byte **0x80**. There are no
  zero bytes and no 80-byte blocks. On receipt, the `PlaintextContent`
  body (everything after the 0xC0 byte) goes through the same unpadding
  rule above, which removes the trailing 0x80.

## 3. `Content` (top-level plaintext)

The core message is a `oneof` named `content`; exactly one of fields 1–6,
8, 9, 11 should be set. Fields 7 and 10 sit outside the oneof and may
accompany any of them.

| # | Name | Type | Meaning |
|---|------|------|---------|
| 1 | dataMessage | DataMessage (section 4) | A normal message: text, attachments, reactions, expiry-timer updates, profile-key updates, and so on. |
| 2 | syncMessage | SyncMessage | Messages between one's own devices (section 8). |
| 3 | callMessage | CallMessage | Calling signaling. Out of scope. |
| 4 | nullMessage | NullMessage (section 6) | Content-free message used to advance a session or hide traffic. |
| 5 | receiptMessage | ReceiptMessage (section 5.1) | Delivery, read, or viewed receipts. |
| 6 | typingMessage | TypingMessage (section 5.2) | Typing indicator. |
| 8 | decryptionErrorMessage | bytes | A serialized `DecryptionErrorMessage` (section 5.3), i.e. a nested protobuf carried as opaque bytes. Only ever sent as libsignal `PlaintextContent`, never encrypted. |
| 9 | storyMessage | StoryMessage | Stories. Out of scope. |
| 11 | editMessage | EditMessage | Edit of an earlier DataMessage: field 1 `targetSentTimestamp` uint64, field 2 `dataMessage` DataMessage. |
| 7 | senderKeyDistributionMessage | bytes | Serialized libsignal SenderKeyDistributionMessage (`spec/protocol/sender-keys.md`). Outside the oneof. |
| 10 | pniSignatureMessage | PniSignatureMessage | Outside the oneof. Field 1 `pni` bytes (the 16-byte raw PNI UUID), field 2 `signature` bytes (signature by the PNI identity key over the ACI identity public key). See section 4.4. |

Declaration (emission) order: 1, 2, 3, 4, 5, 6, 8, 9, 11, then 7, 10.

## 4. `DataMessage`

### 4.1 Field table

Declaration order is the table order.

| # | Name | Type | Meaning |
|---|------|------|---------|
| 1 | body | string | Message text, UTF-8. signal-service-java refuses to send a body over 2048 UTF-8 bytes; signal-cli cuts text at 2000 bytes and sends the full text as a long-text attachment (out of scope). An empty string is not sent: the field is then omitted. |
| 2 | attachments | repeated AttachmentPointer | Attachments. Out of scope. |
| 3 | group | GroupContext | Legacy group v1 context. Not used. |
| 15 | groupV2 | GroupContextV2 | Group v2 context (masterKey 1, revision 2, groupChange 3). Absent for 1:1. |
| 4 | flags | uint32 | Bit set of `Flags` (4.2). Absent on a normal message. |
| 5 | expireTimer | uint32 | Disappearing-message timer in seconds. Absent (not 0) when the conversation has no timer. |
| 23 | expireTimerVersion | uint32 | Version counter of the conversation's timer setting, used to resolve concurrent timer changes; higher wins. Always sent by signal-cli (default 1). |
| 6 | profileKey | bytes | The sender's 32-byte profile key (section 4.4). |
| 7 | timestamp | uint64 | Sent timestamp, milliseconds since the Unix epoch. **Must equal the envelope's `clientTimestamp`** (the `timestamp` of the send request); receivers reject a DataMessage where it is missing or different (section 7). This is the message's identity for receipts, quotes, reactions and deletes. |
| 8 | quote | Quote | Reply quote. Out of scope. |
| 9 | contact | repeated Contact | Shared contacts. Out of scope. |
| 10 | preview | repeated Preview | Link previews. Out of scope. |
| 11 | sticker | Sticker | Out of scope. |
| 12 | requiredProtocolVersion | uint32 | Minimum `ProtocolVersion` (4.3) the receiver must support to render the message. Receivers whose CURRENT is lower treat the message as unsupported. signal-cli always sends it, normally as 0. |
| 14 | isViewOnce | bool | Set (true) only for view-once messages; otherwise absent. |
| 16 | reaction | Reaction | Emoji reaction. Out of scope. |
| 17 | delete | Delete | Remote delete (field 1 `targetSentTimestamp`). Out of scope. |
| 18 | bodyRanges | repeated BodyRange | Mentions and text styles. Out of scope. |
| 19 | groupCallUpdate | GroupCallUpdate | Out of scope. |
| 20 | payment | Payment | Out of scope. |
| 21 | storyContext | StoryContext | Story reply context. Out of scope. |
| 22 | giftBadge | GiftBadge | Out of scope. |
| 24 | pollCreate | PollCreate | Out of scope. |
| 25 | pollTerminate | PollTerminate | Out of scope. |
| 26 | pollVote | PollVote | Out of scope. |
| 27 | pinMessage | PinMessage | Out of scope. |
| 28 | unpinMessage | UnpinMessage | Out of scope. |
| 29 | adminDelete | AdminDelete | Out of scope. |

Field 13 is unused in S1.

### 4.2 `DataMessage.Flags` (values for field 4)

| Value | Name | Meaning |
|---|---|---|
| 2 | EXPIRATION_TIMER_UPDATE | The message announces a new `expireTimer` (and `expireTimerVersion`) for the conversation. Usually has no body. |
| 4 | PROFILE_KEY_UPDATE | The message exists only to deliver `profileKey`. |
| 8 | FORWARD | Forwarded message marker. signal-cli does not set it. |

signal-service-java assigns the flags field rather than OR-ing, so it never
sends more than one flag at a time (PROFILE_KEY_UPDATE wins if both were
requested).

### 4.3 `DataMessage.ProtocolVersion` (values for field 12)

| Value | Name |
|---|---|
| 0 | INITIAL |
| 1 | MESSAGE_TIMERS |
| 2 | VIEW_ONCE |
| 3 | VIEW_ONCE_VIDEO |
| 4 | REACTIONS |
| 5 | CDN_SELECTOR_ATTACHMENTS |
| 6 | MENTIONS |
| 7 | PAYMENTS |
| 8 | POLLS |
| 8 | CURRENT (alias of POLLS) |

signal-service-java starts every outgoing DataMessage at 0 and raises it to
the maximum of: 6 if the message or its quote carries mentions, 3 for a
view-once message, 4 for a reaction, 7 for a payment, 8 for any poll field.
A plain text message stays at **0**, and 0 is sent explicitly.

On receive, a DataMessage whose `requiredProtocolVersion` exceeds the
receiver's CURRENT (8 for 0.14.7) is dropped as unsupported (S2
`EnvelopeContentValidator`; signal-cli logs a warning and outputs nothing
for it).

### 4.4 Profile key and PNI signature: when signal-cli includes them

`profileKey` (field 6, 32 bytes, the account's zkgroup `ProfileKey`
serialization):

- On every 1:1 DataMessage send (`send`, and anything else that goes
  through the same single-recipient path: reactions, remote deletes, edits,
  and so on), signal-cli includes its own profile key **unless the
  recipient is blocked** (S3 `SendHelper`). Before sending it also marks the
  recipient's contact entry as "profile sharing enabled" and not hidden.
  There is no other condition: it is included whether or not the recipient
  already has it.
- A dedicated profile-key update is a DataMessage with only `flags = 4`
  (PROFILE_KEY_UPDATE), `profileKey`, `timestamp`,
  `expireTimerVersion = 1` (the builder default) and
  `requiredProtocolVersion = 0`; no body, no `expireTimer`. It is sent with
  content hint IMPLICIT, `urgent = false`. signal-cli sends one when it
  receives a message from a contact that was **not** sealed sender, if it is
  the primary device and the contact is not blocked and has profile
  sharing enabled (S3 `IncomingMessageHandler`). This is how a peer that
  could not yet use sealed sender gets the key needed for it.
- Receipts, typing messages and null messages carry no profile key.
- On receive, a `profileKey` whose length is not exactly 32 is ignored; a
  valid one is stored as the sender's profile key (and, if the sender is
  the account itself, as the account's own key).

`pniSignatureMessage` (Content field 10) is added to a 1:1 DataMessage only
when signal-cli's recipient store says the recipient "needs a PNI
signature", i.e. after a conversation that started at the account's PNI;
for an ordinary ACI-to-ACI conversation it is absent. Details of when that
flag is set and cleared are **not specified here** (see
`spec/service/send.md` if needed).

## 5. Receipts, typing, and decryption errors

### 5.1 `ReceiptMessage` (Content field 5)

| # | Name | Type | Meaning |
|---|------|------|---------|
| 1 | type | enum Type | Kind of receipt. Must be present, even for DELIVERY (0). |
| 2 | timestamp | repeated uint64 | The `DataMessage.timestamp` values (sent timestamps) of the messages being acknowledged. Unpacked encoding. |

`ReceiptMessage.Type`: DELIVERY = 0, READ = 1, VIEWED = 2.

As sent by signal-cli (S2, S3):

- The Content contains only `receiptMessage`; no padding field, no profile
  key.
- The envelope timestamp of a receipt is the wall-clock time when it is sent
  (not related to the acknowledged messages).
- Send parameters: content hint IMPLICIT, `online = false`,
  `urgent = false`; sealed sender is used when available (section 9).
- **Delivery receipts** are generated automatically while receiving: one
  DELIVERY receipt per sender, listing the timestamps of all that sender's
  DataMessages (or edit messages) in the processed batch, **only for
  messages that arrived via sealed sender** (the receiver's
  "needs receipt" bit is true only for envelope type UNIDENTIFIED_SENDER
  without a visible source). For a non-sealed message signal-cli sends no
  delivery receipt and instead may send its profile key (section 4.4).
- **Read receipts**: sent when `receive --send-read-receipts` is used
  (same batching, type READ), or explicitly by `sendReceipt --type read`
  (one receipt with the given timestamps). An explicit READ or VIEWED
  receipt is skipped if the account's read-receipts configuration is
  explicitly false; DELIVERY is never suppressed this way.

### 5.2 `TypingMessage` (Content field 6)

| # | Name | Type | Meaning |
|---|------|------|---------|
| 1 | timestamp | uint64 | Must equal the envelope's `clientTimestamp`; receivers reject it otherwise. |
| 2 | action | enum Action | Must be present, even for STARTED (0). |
| 3 | groupId | bytes | Group identifier for group typing; absent for 1:1. |

`TypingMessage.Action`: STARTED = 0, STOPPED = 1.

As sent by signal-cli: the Content holds only `typingMessage`; the
timestamp comes from the same monotonic clock as message timestamps
(current milliseconds, bumped so that two consecutive sends never share a
value); content hint IMPLICIT; `online = true` (the server should deliver it
only to connected devices and not queue it); `urgent = false`; sealed sender
when available.

### 5.3 `DecryptionErrorMessage` (carried in Content field 8)

The nested message, serialized and placed as the bytes of Content field 8:

| # | Name | Type | Meaning |
|---|------|------|---------|
| 1 | ratchetKey | bytes | The sender ratchet public key (33-byte serialized form, 0x05 prefix) taken from the `SignalMessage` that failed to decrypt (for a `PreKeySignalMessage`, from its embedded `SignalMessage`). Absent when the failed message was a sender-key message. |
| 2 | timestamp | uint64 | The envelope timestamp of the message that failed. |
| 3 | deviceId | uint32 | The device ID of the sender of the failed message. |

libsignal (S4) encodes the three fields in tag order. The whole Content is
then wrapped as `PlaintextContent` (section 2, 0xC0 prefix and 0x80
suffix) and sent with envelope type PLAINTEXT_CONTENT, or inside sealed
sender (content hint IMPLICIT, the original group ID if any),
`urgent = false`. When a retry receipt is sent, what the original sender
does with it, and the resend log are described in `spec/service/pipe.md`.

A received PLAINTEXT_CONTENT message is valid only if its Content has
`decryptionErrorMessage` and none of the other content fields.

## 6. `NullMessage` (Content field 4)

| # | Name | Type | Meaning |
|---|------|------|---------|
| 1 | padding | bytes | Opaque filler, ignored by the receiver. |

How signal-service-java fills `padding` when it sends a null message (S2):
it builds a throwaway `DataMessage` whose only field is `body`, set to the
**standard base64 (with `=` padding)** of a random byte string whose length
is uniform in 1..140; it serializes that DataMessage and uses the bytes as
`padding`. The resulting Content (only `nullMessage` set) is then padded and
encrypted as in section 2. The envelope timestamp is the current wall-clock
time; content hint IMPLICIT; `urgent = false`; sealed sender when available.
The reason for the DataMessage shape is to make a null message look like a
short text message in size; a receiver must not parse `padding`.

signal-cli 0.14.7 sends null messages only in specific situations (for
example after a session reset or as part of identity verification sync),
not for plain text sends. Those triggers are outside this file.

## 7. Receive-side validation (what our sends must satisfy)

signal-cli runs every decrypted Content through the validator of S2
before handling it; an invalid Content is logged and silently dropped (no
output, no receipt). The rules relevant to 1:1 text, receipts and typing:

- The Content must have at least one of the fields of section 3 set
  ("Content is empty" otherwise).
- DataMessage: `timestamp` present and equal to the envelope
  `clientTimestamp`; `requiredProtocolVersion` absent or at most 8.
- EditMessage: `dataMessage` and `targetSentTimestamp` present, and the
  inner DataMessage timestamp present.
- ReceiptMessage: `type` present.
- TypingMessage: `timestamp` present and equal to the envelope
  `clientTimestamp`; `action` present.
- NullMessage: always valid.
- SyncMessage: only accepted when the source is the account's own ACI.
- If `pniSignatureMessage` is present, its `pni` must be a valid 16-byte
  PNI and `signature` non-empty.
- If `senderKeyDistributionMessage` is present, it must parse.

## 8. `SyncMessage` and `SyncMessage.Sent`

Content field 2. Relevant top-level fields of `SyncMessage`: 1 `sent`
(Sent), 5 repeated `read`, 8 `padding` bytes; the other members of its
`content` oneof (contacts 2, groups 3, request 4, blocked 6, verified 7,
configuration 9, viewOnceOpen 11, fetchLatest 12, keys 13,
messageRequestResponse 14, outgoingPayment 15, pniChangeNumber 18,
callEvent 19, callLinkUpdate 20, callLogEvent 21, deleteForMe 22,
deviceNameChange 23, attachmentBackfillRequest 24,
attachmentBackfillResponse 25, usernameChange 26) are out of scope. When
signal-service-java builds any SyncMessage it sets `padding` to 1..512
random bytes.

`SyncMessage.Sent` (the "sent transcript" sent to one's own other devices):

| # | Name | Type | Meaning |
|---|------|------|---------|
| 1 | destinationE164 | string | Recipient phone number (legacy). |
| 7 | destinationServiceId | string | Recipient service ID, string form. |
| 2 | timestamp | uint64 | Sent timestamp of the original message. |
| 3 | message | DataMessage | The DataMessage that was sent. |
| 4 | expirationStartTimestamp | uint64 | When the disappearing timer started. |
| 5 | unidentifiedStatus | repeated UnidentifiedDeliveryStatus | Per recipient: 3 `destinationServiceId` string, 2 `unidentified` bool, 5 `destinationPniIdentityKey` bytes, 6 `destinationServiceIdBinary` bytes. |
| 6 | isRecipientUpdate | bool | Default false. |
| 8 | storyMessage | StoryMessage | |
| 9 | storyMessageRecipients | repeated StoryMessageRecipient | |
| 10 | editMessage | EditMessage | |
| 12 | destinationServiceIdBinary | bytes | Recipient service ID, binary form (16 bytes for an ACI; 17 bytes, one prefix byte plus 16, for a PNI). |

Field 11 is reserved.

**A single-device account needs none of this on the send side.**
signal-cli 0.14.7 sends a sent transcript after a 1:1 send only if the
server's send response says `needsSync` (the account has other devices),
and its own sync-message path returns immediately when the account is not
multi-device. A "note to self" send on a single-device account therefore
produces **no network message at all**: it is reported as a success
locally. On the receive side, a single-device account will not receive
SyncMessages (they are only sent by the account's own other devices), and
any SyncMessage from a different source is dropped by validation.

## 9. Exactly what signal-cli 0.14.7 sends for a plain 1:1 text message

For `send -m TEXT RECIPIENT` (no attachments, mentions, styles, quote,
preview or sticker) to a recipient that is not the account itself (S3
`ManagerImpl`, `SendHelper`; S2 `SignalServiceMessageSender`):

**Timestamp.** One timestamp `T` per send command: the current time in
milliseconds, made strictly greater than the previous message timestamp
used by this process (it waits 1 ms if the clock has not advanced). `T` is
used as `DataMessage.timestamp`, as the request's message `timestamp`
(hence the receiver's envelope `clientTimestamp`), and is the timestamp
printed/returned by `send`. With several recipients, all get the same `T`.

**Content.** Only field 1 `dataMessage`, plus field 10
`pniSignatureMessage` in the PNI case of section 4.4. No
`senderKeyDistributionMessage`.

**DataMessage fields, in emission order:**

| # | Field | Value |
|---|---|---|
| 1 | body | The text (omitted if empty). |
| 5 | expireTimer | The contact's current disappearing timer in seconds, **only if non-zero**; absent otherwise. |
| 23 | expireTimerVersion | The contact's stored timer version; **1** for a new contact or one whose timer was never changed. Always present. |
| 6 | profileKey | Own 32-byte profile key, unless the recipient is blocked. |
| 7 | timestamp | `T`. |
| 12 | requiredProtocolVersion | **0**, explicitly present. |

Not set: attachments, groupV2, flags, isViewOnce (absent rather than
false), bodyRanges and all the out-of-scope fields.

So a typical first message is roughly: tag 1 body, tag 23 = 1, tag 6
(32 bytes), tag 7 = T, tag 12 = 0, nested as Content field 1. For a short
body this serializes to well under 78 bytes and pads to 79 (section 2).

**Send parameters** (carried by the request; encoding in
`spec/service/send.md`):

- content hint **RESENDABLE** (only visible inside sealed sender);
- `online = false`;
- `urgent = true` by default; `false` when `send --no-urgent` is given;
- **sealed sender** is used when all of the following hold (S3
  `UnidentifiedAccessHelper`): signal-cli has the recipient's profile, and
  the recipient's unidentified-access mode is UNRESTRICTED, or ENABLED and
  signal-cli knows the recipient's profile key; and a sender certificate
  could be obtained (the phone-number-privacy certificate unless phone
  number sharing is EVERYBODY, or CONTACTS and the recipient is a contact;
  certificates are reused until one day before expiry). Otherwise the
  message is sent unsealed (authenticated). If a sealed send is rejected
  with 401, signal-service-java retries unsealed. Details belong to
  `spec/service/send.md`.

The same DataMessage is also stored in signal-cli's resend log (to answer
retry receipts) unless `--disable-send-log` is set.

## 10. Gaps and uncertainties

- **When "needs PNI signature" is set and cleared** for a recipient was not
  traced; only its effect (Content field 10 added) is specified.
- **Null-message triggers** in signal-cli 0.14.7 were not enumerated.
- The exact handling of a DataMessage whose body exceeds 2000 bytes
  (long-text attachment) is out of scope.
- Upstream has **no fixed byte-level test vectors** for Content or
  DataMessage encodings; the only fixed assertions are the padded lengths
  in S2's test, recorded in `testdata/vectors/content_padding.json`. No
  vectors were invented beyond lengths derived directly from the formula.
- Whether the server or the receiving clients care about Wire's
  declaration-order field emission: almost certainly not (protobuf parsers
  accept any order); noted only for byte-identity comparisons with
  captures.
