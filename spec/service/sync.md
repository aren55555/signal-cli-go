# Sync messages and linked devices (signal-cli 0.14.7)

Spec role: prose and tables only; no code. This file covers the
`SyncMessage` that an account's devices exchange with each other (Content
field 2), when signal-cli 0.14.7 sends each kind and what it does when it
receives one. It also covers the device-management commands (`listDevices`,
`removeDevice`, `updateDevice`, `sendSyncRequest`, `sendContacts`,
`unregister`), what "multi-device" changes elsewhere, and note-to-self
messages.

Not repeated here (see the referenced files):

- Content/DataMessage, the 80-byte padding, validation: `content.md`
  (§8 there is the short overview this file expands).
- The `PUT /v1/messages/{destination}` request, the device set for a send
  to one's own account, 409/410 handling, sealed sender: `send.md`.
- Linking a device (provisioning, `PUT /v1/devices/link`, device-name
  encryption): `linking.md`. §7 below fills the gaps listed there.
- Uploading and downloading the contacts/groups blob: the attachments
  spec being written in parallel (`spec/service/attachments.md`). This file
  only says what goes *into* the blob and which pointer fields are set.
- Group sends, sender keys and stories: the parallel group/sender-key and
  stories specs. Only their transcript behaviour is summarised here (§4.5).
- Received-message output (plain text and JSON) of sync messages:
  `spec/surface/cli/_received-message-output.md` ("SYNC block" and
  "SyncMessage"). Command surfaces: `spec/surface/cli/{listDevices,
  removeDevice,updateDevice,sendSyncRequest,sendContacts,unregister,
  send,trust,block,unblock,updateConfiguration,sendReceipt,
  sendMessageRequestResponse,addStickerPack,uploadStickerPack}.md`.
- Storage-service sync itself (what the queued "storage sync" job does):
  out of scope; it is only named here as a trigger.

## Sources

| Ref | What | URL @ version |
|-----|------|---------------|
| Y1 | signal-cli | https://github.com/AsamK/signal-cli @ v0.14.7 (`b01b6b370dc063599a1a2b9fde0f5ff4e2d78fe8`): `lib/src/main/java/org/asamk/signal/manager/helper/{SyncHelper,IncomingMessageHandler,SendHelper,AccountHelper,ReceiveHelper,IdentityHelper,StorageHelper,AttachmentHelper}.java`, `.../manager/actions/{SendSync*Action,SyncStorageDataAction,RetrieveDeviceNameAction,SendReceiptAction,HandleAction}.java`, `.../manager/internal/{ManagerImpl,ProvisioningManagerImpl}.java`, `.../manager/storage/{SignalAccount,AvatarStore}.java`, `.../manager/storage/recipients/RecipientStore.java` (contact query), `.../manager/storage/identities/IdentityKeyStore.java`, `.../manager/api/{Contact,TrustLevel}.java`, `.../manager/util/Utils.java`; `src/main/java/org/asamk/signal/commands/UpdateDeviceCommand.java` |
| Y2 | signal-service-java (Turasa fork) | https://github.com/Turasa/libsignal-service-java @ v2.15.3_unofficial_151 (`66956cb4a0394d50cee3590209baf58312084a44`): `lib/libsignal-service/src/main/java/org/whispersystems/signalservice/api/SignalServiceMessageSender.java`, `.../api/messages/{SignalServiceContent,EnvelopeContentValidator}.kt`, `.../api/messages/multidevice/{Chunked,DeviceContacts,DeviceGroups}{Input,Output}Stream.java`, `.../multidevice/{RequestMessage,SentTranscriptMessage,DeviceInfo}.java`, `.../api/account/AccountApi.kt` (`clearFcmToken`), `.../api/crypto/SealedSenderAccess.kt`, `.../internal/util/Util.java` (`getRandomLengthSecretBytes`), `lib/libsignal-service/src/main/protowire/SignalService.proto`, `SignalServiceLegacy.proto` (GroupDetails); `lib/network/src/main/java/org/signal/network/api/LinkDeviceApi.kt`, `.../api/link/SetDeviceNameRequest.kt`; `core/models-jvm/.../ServiceId.kt` (binary-before-string parsing); test `lib/libsignal-service/src/test/java/.../multidevice/DeviceContactsInputStreamTest.java` |
| Y3 | Reference jar | `/work/third_party/signal-cli-0.14.7/lib/signal-service-java-2.15.3_unofficial_151.jar`: bundled Wire-formatted `SignalService.proto` and `SignalServiceLegacy.proto` (field numbers below were read from these and agree with Y2). Field emission order was checked by an offline `jshell` run that encoded hand-built `SyncMessage`, `SyncMessage.Sent`, `SyncMessage.Blocked`, `ContactDetails` and `GroupDetails` values with the jar's Wire classes. |
| Y4 | libsignal | https://github.com/signalapp/libsignal @ v0.99.1 (`97801d22dcf9f5bf714f7b8fa3212cdc973ae1c8`): `java/client/src/main/java/org/signal/libsignal/net/AuthDevicesService.kt`, `rust/bridge/shared/src/net/chat.rs` (`AuthenticatedChatConnection_{get_devices,remove_device,set_device_name}` all use `require_grpc`), `rust/bridge/shared/types/src/net/chat.rs` (`require_grpc`: the chat connection's shared HTTP/2 connection), `rust/net/src/chat.rs` (default per-request headers on that connection), `rust/net/chat/src/grpc/devices.rs`, `rust/net/grpc/proto/org/signal/chat/device.proto`, `rust/net/infra/src/http_client.rs`, `java/shared/java/org/signal/libsignal/protocol/ecc/ECPrivateKey.kt` (`open`), `rust/crypto/src/hpke.rs` |
| Y5 | Signal-Server | https://github.com/signalapp/Signal-Server @ v20261022.0.0 (`8edf3488f7d97398db7ffa5184c6b769a272ff59`), server behaviour only: `service/src/main/java/org/whispersystems/textsecuregcm/grpc/DevicesGrpcService.java`, `.../controllers/{MessageController,AccountController,DeviceController}.java`, `.../entities/DeviceName.java`, `.../storage/Device.java` |
| Y6 | Standards | RFC 9180 (HPKE), gRPC over HTTP/2 (grpc.io "PROTOCOL-HTTP2"), proto2 encoding |

No fixed test vectors exist upstream for sync messages: the only
contact-stream test (Y2 `DeviceContactsInputStreamTest`) uses random
UUIDs and checks a round trip. No `testdata/vectors/sync.json` was
written.

## 1. Terms and gates

- **Primary** = device ID 1. **Linked** = any other device ID.
- **Multi-device flag.** signal-cli keeps a persisted boolean
  "multi-device" per account. It is the master gate for sending sync
  messages (§3). It is set or cleared as follows (Y1):

  | Event | New value |
  |---|---|
  | Account registered (primary) | false |
  | Account created by linking (linked device) | true, from the start |
  | `addDevice` succeeds | true |
  | Any SyncMessage is received (of any kind, before it is handled) | true |
  | `listDevices` | true if the server lists more than one device, else false |
  | `removeDevice` | after removal, the device list is fetched again: true if more than one device remains, else false |
  | A send addressed only to the own account was encrypted for the local device alone (because no other device had a session) and the server answered 409 with no missing devices | false (`send.md` §5, the "special case" row) |

- **needsSync.** The server's 200 response to `PUT /v1/messages` carries
  `needsSync`. Signal-Server sets it to true only for an **authenticated,
  non-story** send to **another** account when the sender's account has
  more than one device; it is always false for sealed-sender sends, story
  sends and sends to one's own account (Y5 `MessageController`).
  signal-service-java treats a send result as "needs sync" when
  `needsSync` is true **or** the local multi-device flag is set (Y2).
  So with sealed sender, only the local flag matters.
- **Sync sends to oneself.** Every sync message (and every sent
  transcript) is sent as an ordinary 1:1 message whose destination is the
  account's own ACI, **unsealed** (authenticated), with content hint
  IMPLICIT, `online = false`. The device set follows `send.md` §3.2/§4.1:
  device 1 plus every device with an existing session, minus the local
  device (if that leaves nothing, the local device is kept, which the
  server then rejects with 409 and triggers session setup for the missing
  devices). Signal-Server expects such a "sync" send to cover all of the
  account's devices except the sender (Y5 `MessageController`, sync path).

## 2. `SyncMessage` wire format (Content field 2)

All proto2. Field numbers and types from Y3.

### 2.1 Top level

| # | Name | Type | signal-cli sends | signal-cli handles on receipt |
|---|---|---|---|---|
| 1 | sent | Sent (§2.2) | yes (§4) | yes (§6.1) |
| 2 | contacts | Contacts (§5.1) | yes | yes |
| 3 | groups | Groups (§5.2) | yes (legacy v1 groups only) | yes (v1 groups) |
| 4 | request | Request (§2.3) | yes (linked only) | yes (primary only) |
| 6 | blocked | Blocked | yes | yes (adds blocks only) |
| 7 | verified | Verified | yes | yes |
| 9 | configuration | Configuration | yes | yes |
| 11 | viewOnceOpen | ViewOnceOpen | no | output only |
| 12 | fetchLatest | FetchLatest | yes | yes |
| 13 | keys | Keys | yes (primary) | yes |
| 14 | messageRequestResponse | MessageRequestResponse | yes | output only |
| 15 | outgoingPayment | OutgoingPayment | no | parsed, ignored |
| 18 | pniChangeNumber | PniChangeNumber | yes, inside change-number (§6.13) | yes (linked only) |
| 19 | callEvent | CallEvent | no | parsed, ignored |
| 20 | callLinkUpdate | CallLinkUpdate | no | not parsed |
| 21 | callLogEvent | CallLogEvent | no | not parsed |
| 22 | deleteForMe | DeleteForMe | no | not parsed |
| 23 | deviceNameChange | DeviceNameChange | yes | yes (§8.4) |
| 24 | attachmentBackfillRequest | | no | not parsed |
| 25 | attachmentBackfillResponse | | no | not parsed |
| 26 | usernameChange | UsernameChange (empty message) | no | not parsed |
| 5 | read | repeated Read | yes | output only |
| 10 | stickerPackOperation | repeated StickerPackOperation | yes | yes |
| 16 | viewed | repeated Viewed | yes | output only |
| 8 | padding | bytes | always | ignored |

Fields 1–4, 6, 7, 9, 11–15, 18–26 form the `content` oneof; 5, 10 and 16
are repeated and therefore outside it, but are meant to be exclusive with
it. Field 17 is reserved.

**Padding.** Every SyncMessage signal-service-java builds carries
`padding` = N random bytes, N uniform in 1..512 (Y2
`getRandomLengthSecretBytes(512)`: random int in [0, 512) plus one). This
is in addition to the Content padding of `content.md` §2.

**Emission order** (Wire, verified with Y3): `read`, `stickerPackOperation`,
`viewed`, `padding`, then the one oneof member. Within nested messages the
order is the declaration order shown in the tables below. As
`content.md` §1 says, byte identity is not needed for interoperability.

**Which member a receiver sees.** signal-service-java's parser looks at
the members in this fixed priority and uses only the first one present:
sent, request, read (non-empty), viewed (non-empty), viewOnceOpen,
verified, stickerPackOperation (non-empty), blocked, configuration,
fetchLatest (with a type), messageRequestResponse (with a type), groups,
outgoingPayment, keys, contacts, pniChangeNumber, deviceNameChange,
callEvent. Anything else (deleteForMe, callLinkUpdate, callLogEvent,
attachment backfill, usernameChange, or a message with only padding)
becomes an **empty** sync message: still output as "Received a sync
message" with no further lines (`"syncMessage":{}` in JSON), and still
sets the multi-device flag.

**Validation** (Y2 `EnvelopeContentValidator`, beyond `content.md` §7).
A Content with a SyncMessage is dropped (logged, no output) if:

- the envelope source is not the account's own ACI;
- `sent` is present and: `timestamp` is missing; or there is no valid
  destination (`destinationServiceId`/`destinationServiceIdBinary`), no
  group context in `message`, `storyMessage` or `editMessage.dataMessage`,
  no story recipients and `message` is not a remote delete; or a group
  context is malformed; or an `unidentifiedStatus` entry has no valid
  service ID; or a story recipient with an ID lacks `isAllowedToReply`; or
  story recipients are present without `storyMessage` and
  `isRecipientUpdate` is not true; or none of `message`, `storyMessage`,
  story recipients, `editMessage` is present. The nested DataMessage is
  validated like a normal one, so **its `timestamp` must equal the
  envelope timestamp** (which is why transcripts are sent with the
  original message timestamp, §4.1);
- a `read`/`viewed` entry lacks a valid ACI (string or binary) or a
  `read` entry lacks `timestamp`; `viewOnceOpen` or `verified` lacks a
  valid ACI; a sticker operation lacks `packId`; `blocked` contains an
  invalid ACI (string or binary); a `messageRequestResponse` has neither
  `groupId` nor a valid thread ACI; an `outgoingPayment` is malformed; a
  1:1 `callEvent` has a non-ACI `conversationId`.

Where a message has both a string and a binary form of a service ID, the
binary form is preferred when it parses, else the string (Y2 `ServiceId`).
signal-service-java only ever **writes** the binary forms.

### 2.2 `SyncMessage.Sent`

| # | Name | Type | Notes |
|---|---|---|---|
| 1 | destinationE164 | string | Recipient phone number, set when known. |
| 7 | destinationServiceId | string | Never set by signal-cli. |
| 2 | timestamp | uint64 | The original message timestamp. Required. |
| 3 | message | DataMessage | The sent DataMessage (absent for edits and stories). |
| 4 | expirationStartTimestamp | uint64 | See §4.1. |
| 5 | unidentifiedStatus | repeated UnidentifiedDeliveryStatus | One per successful recipient. |
| 6 | isRecipientUpdate | bool, default false | Always written, even when false. |
| 8 | storyMessage | StoryMessage | Story transcripts (stories spec). |
| 9 | storyMessageRecipients | repeated StoryMessageRecipient | Story transcripts. |
| 10 | editMessage | EditMessage | Edit transcripts (`targetSentTimestamp` 1, `dataMessage` 2). |
| 12 | destinationServiceIdBinary | bytes | 16 bytes for an ACI; 17 bytes (prefix byte + 16) for a PNI. |

Field 11 is reserved.

`UnidentifiedDeliveryStatus`:

| # | Name | Type | Notes |
|---|---|---|---|
| 3 | destinationServiceId | string | Never set by signal-cli. |
| 2 | unidentified | bool | signal-service-java **always writes false**, even when the message was sent sealed (Y2 builds the status without consulting the result's sealed flag). |
| 5 | destinationPniIdentityKey | bytes | Only for a PNI destination whose identity key is stored: the 33-byte serialized identity public key. |
| 6 | destinationServiceIdBinary | bytes | As in Sent field 12. |

`StoryMessageRecipient`: 1 `destinationServiceId` string (unused by the
writer), 2 repeated `distributionListIds` string, 3 `isAllowedToReply`
bool, 5 `destinationServiceIdBinary` bytes (field 4 reserved).

### 2.3 Other members (field tables)

**Contacts** (field 2): 1 `blob` AttachmentPointer; 2 `complete` bool
(default false).

**Groups** (field 3): 1 `blob` AttachmentPointer.

**Request** (field 4): 1 `type` enum Type.

| Request.Type | Value |
|---|---|
| UNKNOWN | 0 |
| CONTACTS | 1 |
| GROUPS | 2 |
| BLOCKED | 3 |
| CONFIGURATION | 4 |
| KEYS | 5 |
| (reserved, formerly PNI_IDENTITY) | 6 |

**Blocked** (field 6): 1 repeated `numbers` string; 3 repeated `acis`
string; 2 repeated `groupIds` bytes; 4 repeated `acisBinary` bytes
(16-byte UUIDs). Declaration/emission order 1, 3, 2, 4.

**Verified** (field 7; the message type is the top-level `Verified`):
5 `destinationAci` string; 2 `identityKey` bytes; 3 `state` enum
(DEFAULT 0, VERIFIED 1, UNVERIFIED 2); 4 `nullMessage` bytes;
6 `destinationAciBinary` bytes. Field 1 reserved.

**Configuration** (field 9): 1 `readReceipts` bool; 2
`unidentifiedDeliveryIndicators` bool; 3 `typingIndicators` bool;
6 `linkPreviews` bool. Fields 4 and 5 reserved.

**Read** (field 5) and **Viewed** (field 16), and **ViewOnceOpen**
(field 11): 3 `senderAci` string; 2 `timestamp` uint64 (the sent
timestamp of the message read/viewed/opened); 4 `senderAciBinary` bytes.
Field 1 reserved.

**StickerPackOperation** (field 10): 1 `packId` bytes; 2 `packKey`
bytes; 3 `type` enum (INSTALL 0, REMOVE 1).

**FetchLatest** (field 12): 1 `type` enum (UNKNOWN 0, LOCAL_PROFILE 1,
STORAGE_MANIFEST 2, SUBSCRIPTION_STATUS 3).

**Keys** (field 13): 3 `accountEntropyPool` string (the 64-character
account entropy pool); 4 `mediaRootBackupKey` bytes (32 bytes). Fields 1
(formerly the storage-service key) and 2 (formerly the master key) are
reserved: **the storage key and master key are no longer transmitted**.

**MessageRequestResponse** (field 14): 2 `threadAci` string; 3 `groupId`
bytes; 4 `type` enum; 5 `threadAciBinary` bytes. Field 1 reserved.

| MessageRequestResponse.Type | Value |
|---|---|
| UNKNOWN | 0 |
| ACCEPT | 1 |
| DELETE | 2 |
| BLOCK | 3 |
| BLOCK_AND_DELETE | 4 |
| SPAM | 5 |
| BLOCK_AND_SPAM | 6 |

**PniChangeNumber** (field 18): 1 `identityKeyPair` bytes (serialized
libsignal IdentityKeyPair); 2 `signedPreKey` bytes (serialized
SignedPreKeyRecord); 5 `lastResortKyberPreKey` bytes (serialized
KyberPreKeyRecord); 3 `registrationId` uint32; 4 `newE164` string.

**CallEvent** (field 19): 1 `conversationId` bytes; 2 `callId` uint64;
3 `timestamp` uint64; 4 `type` (UNKNOWN_TYPE 0, AUDIO_CALL 1,
VIDEO_CALL 2, GROUP_CALL 3, AD_HOC_CALL 4); 5 `direction`
(UNKNOWN_DIRECTION 0, INCOMING 1, OUTGOING 2); 6 `event`
(UNKNOWN_EVENT 0, ACCEPTED 1, NOT_ACCEPTED 2, DELETE 3, OBSERVED 4).
Listed for completeness; signal-cli neither sends nor acts on it.

**DeleteForMe** (field 22): 1 repeated `messageDeletes`, 2 repeated
`conversationDeletes`, 3 repeated `localOnlyConversationDeletes`,
4 repeated `attachmentDeletes`. signal-cli 0.14.7 neither sends nor parses
it (its nested `ConversationIdentifier`/`AddressableMessage` types are
therefore not specified here).

**DeviceNameChange** (field 23): 2 `deviceId` uint32. Field 1 (the
former name) is reserved.

**OutgoingPayment** (field 15), **CallLinkUpdate** (20), **CallLogEvent**
(21), **AttachmentBackfillRequest/Response** (24/25): not used by
signal-cli; not specified.

## 3. Sending a sync message: common rules

1. If the local multi-device flag is false, nothing is sent: signal-cli
   returns a local "success with no devices" result and no network
   request is made (Y1 `SendHelper.sendSyncMessage`; the same check is
   repeated in Y2). Exception: sent transcripts after a 1:1 send are
   gated by "needs sync" instead (§4.1), and the primary's change-number
   messages are not sent this way at all (§6.13).
2. Content = only field 2 `syncMessage`. No profile key, no PNI signature.
3. Envelope (`PUT /v1/messages/<own ACI>` body) `timestamp`: the sent
   transcript's timestamp for `sent`; otherwise **the current wall-clock
   time in milliseconds** (not the per-process monotonic message clock of
   `content.md` §9).
4. Content hint IMPLICIT, `online = false`, unsealed, `story=false`.
5. `urgent` is **true** only for: `read`; `request` with type CONTACTS
   or KEYS; attachment-backfill requests (unused). Everything else,
   including GROUPS/BLOCKED/CONFIGURATION requests and all transcripts,
   is sent with `urgent = false`.
6. Send failures are logged, never retried beyond the normal send loop
   of `send.md` §5, and never reported to the user (sync results are not
   part of any command's printed results except note-to-self, §9).
7. Sync sends are not written to the resend log.

## 4. Sent transcripts (`sent`)

### 4.1 After a 1:1 send

After every successful single-recipient send of a Content that holds a
DataMessage or an EditMessage (Y2 `sendContent`), signal-service-java
sends a transcript to the own account if the result "needs sync" (§1) and
the recipient is not the account itself. This covers `send`, reactions,
remote deletes, pins, polls, payment notifications, expiration-timer
updates, profile-key updates sent while receiving (`content.md` §4.4),
and edits. Receipts, typing indicators, null messages and retry receipts
produce **no** transcript.

The transcript is built from the Content exactly as it was sent:

| Sent field | Value |
|---|---|
| 1 destinationE164 | The recipient's phone number, if signal-cli knows it. |
| 2 timestamp | The message timestamp `T` (same as the original). |
| 3 message | The original DataMessage, byte-for-byte the same fields (body, profile key, timer, `requiredProtocolVersion`, …). For a **view-once** message, a copy with all `attachments` removed. Absent for an edit. |
| 4 expirationStartTimestamp | Present only if the DataMessage's `expireTimer` > 0: the wall-clock time (ms) when the transcript is built. |
| 5 unidentifiedStatus | Exactly one entry: the recipient's `destinationServiceIdBinary`, `unidentified = false`, plus `destinationPniIdentityKey` for a PNI recipient (§2.2). |
| 6 isRecipientUpdate | false (written). |
| 9 storyMessageRecipients | none. |
| 10 editMessage | For an edit: the original EditMessage (`targetSentTimestamp`, `dataMessage`). |
| 12 destinationServiceIdBinary | The recipient's service ID, binary. |

Envelope timestamp = `T`, `urgent = false`. A failed 1:1 send (any
non-success result) produces no transcript.

### 4.2 Note-to-self (`send --note-to-self`, or the account's own number/ACI as recipient)

Without `--notify-self` (Y1 `ManagerImpl.sendMessage`,
`SendHelper.sendSelfMessage`):

1. The DataMessage gets the own contact entry's disappearing timer (0 if
   there is no contact entry) and timer version (1 if none). **No profile
   key** is added (unlike a send to another recipient).
2. No DataMessage is sent to anyone. Instead a transcript is sent through
   the §3 gate:
   - single-device account: **nothing is sent**; the command reports
     success for `Note-To-Self` (see `_send-results.md`);
   - multi-device account: a transcript to the own account's other
     devices, with `destinationE164` = own number, `timestamp` = `T`,
     `message` = the DataMessage (or `editMessage` for an edit),
     `expirationStartTimestamp` = now if the timer is > 0,
     one `unidentifiedStatus` entry for the own ACI with
     `unidentified = false`, `isRecipientUpdate = false`,
     `destinationServiceIdBinary` = own ACI (16 bytes). The
     `expirationStartTimestamp` argument signal-cli passes is ignored by
     signal-service-java, which recomputes it.

With `--notify-self` (not applied to edits, which always use the
transcript path): the self entry is handled like any other recipient: a
real DataMessage (with profile key) is sent to the own ACI over the
normal 1:1 path of `send.md`; the own devices receive it as a normal
incoming DataMessage from themselves. No transcript is added (the
recipient is the account itself). On a single-device account this goes to
the server with only the local device as target; the 409 "special case" of
`send.md` §5 then reports success with no devices and clears the
multi-device flag.

### 4.3 Received from another device

See §6.1.

### 4.4 `isRecipientUpdate`

Only set true by group sends: when a group message is sent partly with
sender keys and partly by fan-out, the transcript of the second batch is
marked as a recipient update (group spec). 1:1 and note-to-self
transcripts always carry false.

### 4.5 Group and story transcripts (summary, details elsewhere)

- Group data/edit message via sender key: a transcript is sent if the
  multi-device flag is set and the account is not among the recipients;
  no destination fields, one `unidentifiedStatus` entry per successful
  recipient, `message` carries the group context.
- Group fan-out (legacy) send: transcript if any result needs sync or the
  flag is set (and the account is not among the recipients); a group with
  the account as its only member still yields a transcript when the flag
  is set.
- Stories: if the flag is set, a transcript with `storyMessage` and
  `storyMessageRecipients` (each with `distributionListIds`,
  `isAllowedToReply`, `destinationServiceIdBinary`); a non-group story
  with an empty manifest is not synced.

## 5. Attachment-based streams (contacts and groups)

### 5.1 Contact stream (`contacts`)

**Framing.** The blob's plaintext (before attachment encryption) is a
concatenation of records, each:

1. a varint (protobuf base-128, at most 5 bytes for 32 bits) giving the
   length L of the serialized `ContactDetails`;
2. L bytes: the serialized `ContactDetails`;
3. if that ContactDetails has an `avatar` with a `length` field: exactly
   `avatar.length` bytes of raw avatar image data.

There is no header, no record count and no terminator; the stream ends
at end-of-data. A reader treats end-of-data where a length varint would
start as the end of the stream.

**`ContactDetails`** (declaration/emission order as listed):

| # | Name | Type | Written by signal-cli |
|---|---|---|---|
| 1 | number | string | E.164, if known |
| 9 | aci | string | never |
| 13 | aciBinary | bytes | 16-byte ACI, if known |
| 2 | name | string | see below |
| 3 | avatar | Avatar | if a local contact avatar file exists |
| 8 | expireTimer | uint32 | see below |
| 12 | expireTimerVersion | uint32 | **never** (the writer in Y2 drops it although signal-cli supplies it) |
| 10 | inboxPosition | uint32 | never |

Fields 4, 5, 6, 7, 11 are reserved. `ContactDetails.Avatar`:
1 `contentType` string, 2 `length` uint32.

**What a signal-cli primary puts in the stream** (Y1
`SyncHelper.sendContacts`):

1. One record per stored contact. "Contact" means a recipient row that
   has a phone number, PNI or ACI, is not hidden, and has at least one of:
   a given/family name, a nickname (any part), a note, a disappearing
   timer > 0, profile sharing enabled, a colour, blocked, archived. Order:
   database order.
   - `number`, `aciBinary` as known (a contact known only by PNI gets
     neither).
   - `name`: the contact's given and family name joined by one space, or
     whichever one exists; **an empty string when neither exists** (the
     field is then present with length 0).
   - `expireTimer`: the contact's timer in seconds, **written even when
     0**.
   - `avatar`: the locally stored contact avatar (not the profile avatar),
     `contentType` = the MIME type guessed from the file (falling back to
     `application/octet-stream`), `length` = file size; the image bytes
     follow the record.
2. Then, if the account has a profile key (always, in practice), one
   record for the account itself, built the same way from the self
   recipient. If the self recipient has no contact row, its record has
   only `number`, `aciBinary` and possibly `avatar` (no `name`, no
   `expireTimer`). If the self recipient is also a contact, it appears
   twice.
3. The stream is written to a temporary file. If it is non-empty (which
   is always the case because of step 2), signal-cli requests a resumable
   upload slot for it **before** checking the multi-device flag, i.e. an
   upload-form request happens even on a single-device account; the
   actual upload and the sync send happen only when multi-device.
   Upload: content type `application/octet-stream`, size = file length,
   no file name, no flags (attachment details: attachments spec).
4. The sync message is `contacts { blob = <pointer>, complete = true }`.

**Reading** (Y2 `DeviceContactsInputStream`, Y1
`handleSyncDeviceContacts`):

- The ACI is taken from `aciBinary`, else `aci`; the phone number from
  `number`. `expireTimer` and `expireTimerVersion` count as present only
  when > 0. A missing avatar `contentType` defaults to `image/*`.
- **Quirk:** a record whose string `aci` **and** `number` are both absent
  is rejected as "Missing contact address!" (its avatar bytes are skipped)
  **even if `aciBinary` is present**. signal-cli skips such records
  silently. Consequence: a contact known only by ACI, as written by a
  signal-cli primary, is dropped by a signal-cli linked device.
- Any other read error (truncated record, malformed varint, undecodable
  protobuf) aborts the whole import with a warning; records already
  processed stay applied.
- For each accepted record: the ACI and number are linked as one
  recipient (trusted association). If the local contact has neither given
  nor family name (or does not exist), `name` becomes the given name and
  the family name is cleared. The timer is applied only if both
  `expireTimer` and `expireTimerVersion` are present and the version is
  greater than the stored one (a missing local contact counts as
  "apply"); since signal-cli never writes the version, timers from a
  signal-cli primary are never applied. The contact is stored (so the
  recipient becomes a contact). The avatar is saved as the contact avatar
  unless `--ignore-avatars` (receive option) is in effect, in which case
  the bytes are read and discarded.
- `complete` has no effect on import (it only selects the output text
  "complete"/"partial").

### 5.2 Group stream (`groups`)

Same framing as §5.1, with `GroupDetails` records (legacy group v1
data; message type from `SignalServiceLegacy.proto`):

| # | Name | Type | Written by signal-cli |
|---|---|---|---|
| 1 | id | bytes | v1 group ID |
| 2 | name | string | if set |
| 3 | membersE164 | repeated string | members' phone numbers |
| 9 | members | repeated Member | same numbers, as Member{2 `e164`} (Member field 1 reserved) |
| 4 | avatar | Avatar | 1 `contentType`, 2 `length`, bytes follow |
| 5 | active | bool, default true | whether the account is a member (always written) |
| 6 | expireTimer | uint32 | always written (may be 0) |
| 7 | color | string | if set |
| 8 | blocked | bool | always written |
| 10 | inboxPosition | uint32 | never |
| 11 | archived | bool | always written |

signal-cli only writes **group v1** entries, which a current account no
longer has; with no v1 groups the file is empty and **no groups sync
message is sent at all** (no upload either). Group v2 state reaches
linked devices through storage service and group context in messages,
not through this stream.

On receipt, each entry creates or updates a local v1 group: name (if
present), members added (resolved by number; a member without a number
makes the record fail and be skipped), the account added to or removed
from the members according to `active`, `blocked`, `color` (if present),
avatar downloaded unless avatars are ignored, `archived`.

## 6. Each member: when sent, what happens on receipt

### 6.1 `sent`

Sent: §4.

Received (any device): the nested DataMessage (if any) is processed as
an **outgoing** message of the account: "self" is the sending device and
the conversation partner is the transcript's destination (absent for
group messages). Effects:

- group context handled as for an incoming group message (group v2 state
  fetched if the revision is newer; legacy v1 update/quit handling;
  v1 DELIVER and REQUEST_INFO trigger nothing for transcripts);
- if the message has a body or is an expiration-timer update and is not
  a group message: the destination contact's disappearing timer and
  version are set from the message (also for note-to-self, where the
  destination is the account itself);
- attachments, sticker packs, shared-contact avatars, preview images and
  quote thumbnails are downloaded as for incoming messages, subject to the
  same `--ignore-attachments`/`--ignore-stickers` options;
- the DataMessage's `profileKey`, if exactly 32 bytes, is stored for the
  **sender, i.e. the account itself**, which updates the account's own
  profile key. This is how a linked signal-cli learns a rotated profile
  key from transcripts of the primary's sends;
- a `storyMessage` in the transcript is handled like an incoming story
  (group context, attachment download, its profile key stored as the
  account's own);
- an `editMessage`-only transcript is not processed (output only);
- no delivery receipt and no profile-key reply are sent for a transcript.

Output: "Received sync sent message" with `To:` the destination
(`_received-message-output.md`).

### 6.2 `request`

Sent by: a **linked** device only.
- After linking (`linking.md` §7 step 7) and by `sendSyncRequest`: four
  requests in this order: **GROUPS, CONTACTS, BLOCKED, CONFIGURATION**,
  then **KEYS**; then a storage sync job is queued. Each request is its
  own sync message (own envelope, own padding), urgent only for CONTACTS
  and KEYS. Failures only log; after linking a failure logs "Failed to
  request sync messages from linked device, data can be requested again
  with `sendSyncRequest`."
- KEYS alone, from the storage sync job on a linked device, when it has
  no storage key, or when the storage manifest cannot be decrypted with
  the key it has.
- On a primary, signal-service-java refuses to build a request ("Sync
  requests should only be sent from a linked device"); `sendSyncRequest`
  on a primary therefore sends nothing and still exits 0 with no output.

Received: ignored on a linked device. On the **primary**, each request
queues one response action (§6.2.1) and, for every request message
regardless of type, a storage sync job. Actions are de-duplicated within a
batch (five requests of the same type in one batch give one response),
and run after the server's queue has been drained ("queue empty"
marker), or immediately per message once caught up (`pipe.md`). The order
in which different actions of one batch run is unspecified (hash order).
A failing action logs "Message action failed." and the others still run.

#### 6.2.1 How the primary answers

| Request | Response sent by signal-cli |
|---|---|
| CONTACTS | A `contacts` message (§5.1), complete = true. |
| GROUPS | A `groups` message with the v1 group stream (§5.2), or nothing if there are no v1 groups (the normal case). |
| BLOCKED | A `blocked` message (§6.4). |
| CONFIGURATION | A `configuration` message (§6.6). |
| KEYS | A `keys` message (§6.7). |
| UNKNOWN / other | Nothing (only the storage sync job). |

### 6.3 `contacts`, `groups`

Sent: as answers (§6.2.1) and, for contacts, by the `sendContacts`
command (§8.6). Received: §5.1 / §5.2. The blob is downloaded through the
attachment path to a temporary file first; download or parse failures
log "Failed to handle received sync contacts/groups, ignoring".

### 6.4 `blocked`

Sent: as the BLOCKED answer, and after every `block`/`unblock` that
changed anything (contacts or groups), after the message-request-response
syncs of §6.9. Content (Y1 `sendBlockedList`, Y2): for each blocked
contact that has an ACI or a number, the ACI goes into `acisBinary` and
the number into `numbers` (the two lists are filled independently, so
pairing is lost); `groupIds` = serialized IDs of all blocked groups (v1:
16 bytes; v2: 32 bytes). The string `acis` list is never filled.

Received: signal-service-java reads **only `numbers` and the string
`acis`**; `acisBinary` is validated but otherwise **ignored**. So from a
signal-cli primary, only blocked contacts with a known number take
effect on a signal-cli linked device. Each listed recipient is marked
blocked; each listed group is marked blocked (unknown group IDs log a
warning). Recipients and groups **not** in the list are **not**
unblocked: receipt only ever adds blocks.

### 6.5 `verified`

Sent: by `trust` (any of its forms) after the trust level of a stored
identity was changed (Y1 `IdentityHelper`), only on a multi-device
account. signal-service-java first sends a **null message to the
contact** (unsealed, timestamp = the time the verified message was
created, content hint IMPLICIT, `urgent = false`; padding as
`content.md` §6), and only if that send succeeds and needs sync, a
`verified` sync to the own devices with:
`identityKey` = the contact's 33-byte identity key,
`destinationAciBinary` = the contact's service ID (binary),
`state` from the new trust level (TRUSTED_VERIFIED → VERIFIED,
TRUSTED_UNVERIFIED → DEFAULT, UNTRUSTED → UNVERIFIED),
`nullMessage` = the serialized NullMessage that was just sent to the
contact. The sync envelope uses the same timestamp. On a single-device
account neither message is sent.

Received: the contact's stored identity's trust level is set from the
state (VERIFIED → TRUSTED_VERIFIED, DEFAULT → TRUSTED_UNVERIFIED,
UNVERIFIED → UNTRUSTED), **only if** an identity is stored for that
service ID and it equals `identityKey`; otherwise nothing changes. A
missing or invalid key or an unknown state makes the whole message
invalid.

### 6.6 `configuration`

Sent: as the CONFIGURATION answer, and by `updateConfiguration` (after
storing the new values; a storage sync job follows). Each of the four
booleans is written only if the local configuration has a value for it
(an unset setting is omitted, not sent as false); link previews use
field 6.

Received: each present field overwrites the corresponding local setting
(read receipts, unidentified-delivery indicators, typing indicators, link
previews). Absent fields leave settings unchanged. Note that the
read-receipts setting gates explicit read/viewed receipts
(`content.md` §5.1).

### 6.7 `keys`

Sent: as the KEYS answer (by the primary), and by the primary's storage
sync job right after it migrates from a PIN master key or storage key to
an account entropy pool. Content: `accountEntropyPool` = the account's
pool (created and persisted on the spot if the account had none) and
`mediaRootBackupKey` (likewise created if absent). signal-cli also passes
its storage key, but signal-service-java has no field for it, so it is
not sent.

Received (any device): if `accountEntropyPool` is present, it is stored
and the locally stored storage key and PIN master key are discarded (both
are derived from the pool from then on); a storage sync job is queued. If
`mediaRootBackupKey` is present it is stored and a storage sync job is
queued. (signal-cli also has a branch for a received storage key; it can
never trigger, because the parser always reports none.)

### 6.8 `read`, `viewed`, `viewOnceOpen`

Sent: `sendReceipt --type read` / `--type viewed` (Y1
`ManagerImpl.sendReceiptMessage`): after the receipt to the sender (or
without it, when read receipts are disabled in the configuration), a
`read` (or `viewed`) sync with one entry per given timestamp, each with
`timestamp` and `senderAciBinary` = the sender's ACI. Only sent when the
sender has a known ACI. `read` is urgent, `viewed` is not. Delivery
receipts produce no sync. Receipts sent automatically by
`receive --send-read-receipts` produce **no** read sync. signal-cli never
sends `viewOnceOpen`.

Received: no local state is changed (signal-cli keeps no read state); the
entries are only output. Entries without a valid ACI or timestamp make
the message invalid (§2.1).

### 6.9 `messageRequestResponse`

Sent:
- by `sendMessageRequestResponse` (each recipient/group given), and
- by `block`/`unblock` for each contact or group whose state actually
  changed: type BLOCK when blocking, UNBLOCK_AND_ACCEPT when unblocking.

For a person: `threadAciBinary` = the contact's service ID, binary (a
recipient without a service ID is skipped, and nothing is reported for
it); additionally the contact's profile-sharing flag is set to true for
ACCEPT and UNBLOCK_AND_ACCEPT, false otherwise. For a group: `groupId` =
the serialized group ID. For note-to-self: the account's own ACI.

**Type mapping on the wire** (Y2): ACCEPT 1, DELETE 2, BLOCK 3,
BLOCK_AND_DELETE 4, SPAM 5, BLOCK_AND_SPAM 6 are sent as such;
**UNBLOCK_AND_ACCEPT and UNKNOWN are both sent as UNKNOWN (0)** (the
sender's switch has no case for UNBLOCK_AND_ACCEPT and logs "Unknown
type!"). So `unblock` syncs a message request response of type 0.

Received: output only, no local change. The parser maps wire values 1–4
to ACCEPT/DELETE/BLOCK/BLOCK_AND_DELETE and **everything else (0, 5, 6,
unknown) to UNKNOWN**. A message with a type but neither group ID nor
valid thread ACI is invalid; a message without `type` is not recognised
at all (falls through to the next member in the priority list).

### 6.10 `fetchLatest`

Sent: `updateProfile` sends type LOCAL_PROFILE after updating the profile;
the storage sync job sends STORAGE_MANIFEST after it wrote to or force
pushed the storage service.

Received: LOCAL_PROFILE → the account's own profile is re-fetched;
STORAGE_MANIFEST → a storage sync job is queued; SUBSCRIPTION_STATUS →
nothing; UNKNOWN (0) → the message is rejected as malformed (the whole
envelope is reported with an exception); no type → not recognised.

### 6.11 `stickerPackOperation`

Sent: by `addStickerPack` (install) and `uploadStickerPack` (after
upload): one entry with `packId` (16 bytes), `packKey` (32 bytes),
`type` = INSTALL (0, written explicitly). REMOVE is never sent.

Received: entries without `packId` are skipped (and make the message
invalid at validation). For each entry the pack is added or updated
locally as installed (type absent or INSTALL) or not installed (REMOVE),
with the key if given; for an installed pack, unless
`--ignore-stickers`, a job downloads it.

### 6.12 `callEvent`, `deleteForMe`, `outgoingPayment`, others

signal-cli never sends them. `callEvent` and `outgoingPayment` are parsed
but cause no action and no output; `deleteForMe`, `callLinkUpdate`,
`callLogEvent`, attachment backfill and `usernameChange` are not parsed
(§2.1, empty sync message). `viewOnceOpen` is output only (§6.8).

### 6.13 `pniChangeNumber`

Sent by the **primary** only as part of `finishChangeNumber`, not through
the sync path: for each own device (device 1 plus every own device with a
session), a fresh PNI identity key pair (shared), and per device a new
signed prekey and last-resort Kyber prekey (random IDs, signed with the
new PNI identity key) and a new PNI registration ID (unique among the
devices) are generated. For every device other than the primary itself a
SyncMessage with `pniChangeNumber` (fields 1, 2, 5, 3, 4 = new number)
and the usual padding is encrypted (unsealed, content hint IMPLICIT) and
the resulting per-device ciphertexts are included in the change-number
request rather than sent with `PUT /v1/messages` (request format: the
change-number spec). The primary applies its own entry locally when the
server reports a new PNI.

Received: ignored (warning) on a primary, and ignored when it does not
come from device 1. Otherwise it is treated as a replay and ignored
unless the envelope's server-delivered timestamp is greater than the last
applied one. If the envelope carries `updatedPni` and the message has
identity key pair, registration ID and signed prekey, the account's PNI,
PNI identity key pair, number (`newE164`), PNI registration ID, signed
prekey and (if present) last-resort Kyber prekey are replaced, PNI
prekeys are force-refreshed, and the server timestamp is remembered.

### 6.14 `deviceNameChange`

Sent: by `updateDevice` on the primary for another device, after the
name was changed on the server (§8.4): `deviceId` = that device.

Received: if `deviceId` equals the local device ID, signal-cli queues an
action that re-fetches the device list and stores its own encrypted name
from it. **By source reading this action fails at runtime in 0.14.7**
(the device list returned by libsignal is cast to an incompatible type),
so the effect is a logged "Message action failed." warning and the
locally stored name is not updated. Not observed black-box. A Go
implementation may simply update the stored name.

## 7. Linking gaps (fills `linking.md` §6.3, §7, §8)

- The post-link sync requests are exactly those of §6.2: GROUPS,
  CONTACTS, BLOCKED, CONFIGURATION, KEYS, each a separate unsealed sync
  message to the own ACI (wall-clock timestamps, urgent only for CONTACTS
  and KEYS), followed by queuing a storage sync job. They are sent right
  after the prekey refresh, inside the `link`/`finishLink` process. The
  new device has no sessions yet, so the first of them triggers the bundle
  fetch for the own ACI (`send.md` §4.1) and session setup with the
  primary (and any other devices the server returns).
- The **primary does not push anything** to a new device after
  `addDevice`: it only sets the multi-device flag and queues a storage
  sync job. Its first message to its own account afterwards (a transcript,
  a request answer) finds no session with the new device; with the local
  device removed from the target set the set is empty, so the local
  device is used, the server answers 409 listing the new device as
  missing, the primary fetches its bundle, and retries (`send.md` §5).
- The answers to the new device's requests are produced the next time
  the primary runs `receive` (or the daemon) and processes them (§6.2.1).
- signal-cli passes its PIN master key (creating it if needed) to the
  link call, but it is not put into the ProvisionMessage (field 13 is
  reserved); `linking.md` §6.2 is correct.

## 8. Linked-device management

All requests in this section go over the **authenticated chat
connection** (libsignal-net, `transport.md`). Three of them are **gRPC**
calls, which contradicts `transport.md` §3.6 ("A Go client needs no
gRPC"): libsignal 0.99.1 implements `getDevices`, `removeDevice`,
`setDeviceName` and `clearPushToken` only as gRPC, with no WebSocket
fallback and no remote-config switch (Y4 bridge `require_grpc`).
signal-cli uses the first, second and fourth of these; for renaming it
uses a WebSocket request instead (§8.4).

### 8.1 gRPC transport facts (for §8.2, §8.3, §8.7)

- The calls are sent as ordinary gRPC unary requests (HTTP/2 POST,
  path `/org.signal.chat.device.Devices/<Method>`, gRPC 5-byte message
  framing, protobuf bodies) on the **same HTTP/2 connection** that carries
  the authenticated chat WebSocket (the extended-CONNECT connection to
  `grpc.chat.staging.signal.org`, `transport.md` §3.4).
- Every such request carries, as default headers, the **same headers as
  the WebSocket upgrade** (in particular `Authorization: Basic
  base64("<ACI>.<deviceId>:<password>")` and the user agent) **except**
  `x-signal-receive-stories` (Y4 `rust/net/src/chat.rs`). The server
  authenticates the call with them (service option "authenticated only").
- The exact remaining header set (`content-type: application/grpc`,
  `te: trailers`, timeouts) follows the generic gRPC-over-HTTP/2
  convention of the tonic client; not further specified here (gap §11).
- Errors: a gRPC error status is converted into a request failure; the
  calls below define no request-specific error except where noted.

Messages (package `org.signal.chat.device`, **proto3**; field numbers
from Y4 `device.proto`):

| Message | Fields |
|---|---|
| GetDevicesRequest | (empty) |
| GetDevicesResponse | 1 repeated `devices` LinkedDevice |
| GetDevicesResponse.LinkedDevice | 1 `id` uint32; 2 `name` bytes; 3 `last_seen` uint64 (ms since epoch); 4 `registration_id` uint32 (≤ 0x3fff); 5 `created_at_ciphertext` bytes |
| RemoveDeviceRequest | 1 `id` uint32 |
| RemoveDeviceResponse | (empty) |
| SetDeviceNameRequest | 1 `name` bytes (1..225 bytes); 2 `id` uint32 |
| SetDeviceNameResponse | oneof: 1 `success` (google.protobuf.Empty), 2 `target_device_not_found` (empty NotFound) |
| ClearPushTokenRequest / ClearPushTokenResponse | (empty) |

### 8.2 `listDevices`

1. gRPC `Devices/GetDevices` with an empty request.
2. The multi-device flag becomes "more than one device listed".
3. For each device, in server order (Y1 `ManagerImpl.getLinkedDevices`):
   - **Name.** `name` holds the raw serialized `DeviceName` protobuf
     (`linking.md` §4.5) as stored on the server: the bytes are **not**
     base64 here (the server stores the decoded bytes of the base64 string
     given at link time). If `name` is non-empty, decode it as
     `DeviceName` and decrypt it with the account's **ACI identity
     private key** (inverse of `linking.md` §4.5: X25519 with the
     ephemeral public key, derive the cipher key from the synthetic IV,
     AES-256-CTR decrypt, recompute and check the synthetic IV); the
     result, as UTF-8, is the name. If decoding or decryption fails, the
     raw bytes interpreted as UTF-8 are used instead. An empty or absent
     `name` gives no name (printed `null`).
   - **Created.** `created_at_ciphertext` is an HPKE (RFC 9180) ciphertext
     sealed to the account's ACI identity public key, laid out as: one
     type byte **0x01** (= base mode, DHKEM(X25519, HKDF-SHA256),
     HKDF-SHA256, AES-256-GCM), then the 32-byte encapsulated key, then
     the AES-GCM ciphertext with its 16-byte tag. Open it with the ACI
     identity private key, `info` = ASCII `deviceCreatedAt`, and
     associated data = **one byte** holding the device ID followed by the
     **4-byte big-endian** registration ID (from field 4). The plaintext
     is an 8-byte big-endian signed integer: the creation time in ms. On
     any failure signal-cli logs a warning and reports 0.
   - **Last seen** = `last_seen` (ms). **This device** = `id` equals the
     local device ID.
4. Output per `spec/surface/cli/listDevices.md`.

Works on primary and linked devices. The REST endpoint `GET /v1/devices`
(JSON `{"devices":[{"id","name","lastSeen","registrationId",
"createdAtCiphertext"}]}` with base64 byte fields, Y5 `DeviceController`)
exists but is **not** used by signal-cli 0.14.7.

### 8.3 `removeDevice -d N`

Primary only: on a linked device signal-cli fails with "This command
doesn't work on linked devices." before any request (although the server
would let a linked device remove itself). Then:

1. gRPC `Devices/RemoveDevice` with `id = N`. The server rejects removing
   device 1 (invalid argument) and accepts a device ID that does not exist
   (idempotent success) (Y5, Y4 docs).
2. gRPC `Devices/GetDevices`; the multi-device flag becomes "more than one
   device listed".

Sessions with the removed device are not touched here; they are archived
when a later send to the own account gets a 409/410 (`send.md` §5).

### 8.4 `updateDevice -d N -n NAME`

- **N = the local device ID** (allowed on primary *and* linked devices;
  this corrects `spec/surface/cli/updateDevice.md`, which lists the
  linked-device error unconditionally): the name is encrypted
  (`linking.md` §4.5, with the account's ACI identity key), base64
  **without** padding, and stored locally only. No request is made by the
  command. It reaches the server as the `name` member of the account
  attributes on the next `PUT /v1/accounts/attributes`, which signal-cli
  sends at the start of every later command that checks the account
  (`api.md` §4.3, `registration.md`).
- **N = another device**: primary only (else "This command doesn't work
  on linked devices."). The name is encrypted the same way, then
  `PUT /v1/accounts/name?deviceId=N` over the authenticated WebSocket
  (not gRPC), JSON body `{"deviceName":"<base64 without padding>"}`.
  Server: 204 success, 403 not allowed, 404 no such device; the decoded
  name must be 1..225 bytes; the server accepts unpadded base64 (Y5
  `AccountController`, `DeviceName`). Errors map per `api.md`. On success a
  `deviceNameChange` sync with `deviceId = N` is sent (§6.14).

### 8.5 `sendSyncRequest`

The five requests of §6.2 (GROUPS, CONTACTS, BLOCKED, CONFIGURATION,
KEYS), then a storage sync job. Individual results are ignored; exit 0,
no output. On a primary nothing is sent (§6.2). On a linked device whose
multi-device flag were false nothing would be sent either (cannot happen
normally, since linking sets it).

### 8.6 `sendContacts`

Builds and sends the contact stream exactly as in §5.1 (also on a linked
device; the command does not check). On a single-device account the
upload-slot request still happens but nothing is uploaded or sent.
Exit 0, no output.

### 8.7 `unregister` (without `--delete-account`)

1. gRPC `Devices/ClearPushToken` (empty request), on primary and linked
   devices alike.
2. Locally: the account is marked not registered (the account files are
   kept); later commands fail with "User … is not registered."

What this does on the server (Y5 `DevicesGrpcService.clearPushToken`):
it clears the device's APNs/FCM tokens and sets **`fetchesMessages =
true`**. For signal-cli, which never sets push tokens and registers with
`fetchesMessages: true`, this changes nothing server-side: a linked
device stays linked and keeps receiving queued messages, and a primary
stays reachable. (signal-cli's own source comment and man page describe
the older effect of disabling the device; that is no longer what the
server does.) To really remove a linked device, run `removeDevice` on the
primary. `--delete-account` is out of scope here.

## 9. What multi-device mode changes elsewhere

- **Transcripts** after 1:1 sends (§4.1) and group/story sends (§4.5).
  On a multi-device account every successful DataMessage/EditMessage send
  costs a second `PUT /v1/messages/<own ACI>`.
- **Sync side effects of commands** (all no-ops when single-device):
  `block`/`unblock` (message-request responses + blocked list),
  `sendMessageRequestResponse`, `trust` (null message + verified),
  `updateConfiguration`, `updateProfile` (fetch latest), `sendReceipt`
  read/viewed, sticker install/upload, `updateDevice` for another device,
  storage sync (fetch-latest STORAGE_MANIFEST, keys after migration),
  `sendContacts`.
- **Storage key creation** on a primary: a storage key (and hence an
  account entropy pool) is created on demand when the account is
  multi-device; a single-device primary without an existing key does not
  create one for storage sync.
- **Note-to-self** becomes a real network message (§4.2).
- **Receiving**: SyncMessages are accepted only from the own ACI (any of
  its devices) and processed per §6. Sync-request answers are produced
  only on the primary. Messages sent by the account's other devices to
  third parties arrive as transcripts; messages to the account from
  others arrive at every device independently (each device decrypts and
  sends its own delivery receipts per `content.md` §5.1; the profile-key
  reply of `content.md` §4.4 is sent by the primary only).
- **Linked-device restrictions** in signal-cli (fail with "not a primary
  device"/"This command doesn't work on linked devices." before any
  request): `addDevice`, `removeDevice`, `updateDevice` for another
  device, `startChangeNumber`, `finishChangeNumber`, `setPin`/`removePin`.
  A linked device also never answers sync requests, ignores
  `request` messages, and is the only kind of device that applies
  `pniChangeNumber`.

## 10. Self-messages and how they are reported

| Situation | What is on the wire | What the other own device reports |
|---|---|---|
| `send --note-to-self` (or own number) on a single-device account | nothing | — (local success result only) |
| same on a multi-device account | transcript (§4.2) | "Received a sync message" → "Received sync sent message", `To:` the own address, then the DATA block; JSON `syncMessage.sentMessage` with `destinationNumber`/`destinationUuid` = own number/ACI |
| `send --note-to-self --notify-self` | a normal DataMessage to the own ACI | a normal incoming data message whose sender is the account itself |
| `send` to a third party from a multi-device account | the message + a transcript | sync sent message with `To:` the third party |
| group send from a multi-device account | the group message + a transcript without destination | sync sent message; `To:` lists the recipients from `unidentifiedStatus` (each `unidentified` false) |
| edit of a note-to-self / of a 1:1 message | transcript with `editMessage` | JSON `sentMessage.editMessage`; not printed in plain text |

Rendering details: `_received-message-output.md` (SYNC block; JSON
`SyncMessage`, `SyncDataMessage`). The envelope of a transcript shows the
own number/ACI as source and the sending device ID as source device; it
is never sealed.

For the sending command, note-to-self results follow `_send-results.md`
(key `Note-To-Self`): on a single-device account the result is a success
with no devices; on a multi-device account it is the result of the
transcript send to the own account (success, or e.g. a network failure).

## 11. Gaps and uncertainties

- **gRPC header set** for the device calls (§8.1) beyond the default
  chat headers: taken to be tonic's standard gRPC headers; not traced in
  detail. `transport.md` §3.6 should be amended: device listing, removal,
  renaming via libsignal and push-token clearing are gRPC-only in
  libsignal 0.99.1.
- **gRPC error statuses** returned by the server for these calls (e.g.
  for removing device 1, or bad authentication) and how signal-cli prints
  them: only "Failed to get linked devices: …"/"Error while removing
  device: …" with the exception message, whose exact text was not traced.
- **`deviceNameChange` handling** failure (§6.14) is inferred from types
  in the source, not observed.
- **Action order** of a batch of sync-request answers is unspecified
  (hash order of singleton objects); a client may use any order.
- **Change-number request body** carrying the `pniChangeNumber`
  ciphertexts belongs to a change-number spec that does not yet exist.
- **Contact-avatar MIME detection** (which types the file sniffing can
  return) was not traced; `application/octet-stream` is the fallback.
- No upstream fixed vectors for the contact/group streams; none invented.
  Field order in this file was checked by encoding values with the
  reference jar (Y3), not taken from vectors.
