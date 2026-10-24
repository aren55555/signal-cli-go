# Stories and sender-key / multi-recipient sending (signal-cli 0.14.7)

Spec role: prose only; no code. This file covers:

- the `StoryMessage` plaintext and what `sendStory` puts on the wire;
- the story sync transcript;
- story replies and reactions, as far as they differ from normal data
  messages;
- receive-side story handling;
- the sender-key send path as signal-cli 0.14.7 uses it: SKDM fan-out,
  "shared with" tracking, rotation, and `PUT /v1/messages/multi_recipient`.

Group-send endorsements and normal group messages over sender key are
milestone 6. They are mentioned only where the story code shares them.

Not repeated here:

- Content framing and padding, SyncMessage.Sent fields (`content.md`);
- the 1:1 send loop, sealed/unsealed choice, 409/410 handling for
  `PUT /v1/messages/{dest}` (`send.md`);
- sender-key cryptography and the SKDM wire format
  (`spec/protocol/sender-keys.md`);
- sealed sender v1/v2 byte layouts (`spec/protocol/sealed-sender.md`);
- the WebSocket and its `x-signal-receive-stories` header (`pipe.md`,
  `transport.md`);
- attachment upload and `AttachmentPointer` fields (attachments spec,
  written in parallel);
- DataMessage fields other than `storyContext` (data-message spec,
  written in parallel);
- sync/linked-device behaviour beyond the story transcript (sync spec,
  written in parallel);
- CLI and JSON output shapes (`spec/surface/cli/sendStory.md`,
  `_send-results.md`, `_received-message-output.md`,
  `spec/surface/jsonrpc/notifications.md` §4.12, §6, §7.2).

## Sources

| Ref | What | URL @ version |
|-----|------|---------------|
| Y1 | signal-cli | https://github.com/AsamK/signal-cli @ v0.14.7 (b01b6b370dc063599a1a2b9fde0f5ff4e2d78fe8): `lib/src/main/java/org/asamk/signal/manager/internal/{ManagerImpl,SignalDependencies}.java`, `.../manager/helper/{SendHelper,UnidentifiedAccessHelper,IncomingMessageHandler,ReceiveHelper,RecipientHelper}.java`, `.../manager/storage/senderKeys/{SenderKeyStore,SenderKeySharedStore,SenderKeyRecordStore}.java`, `.../manager/storage/sessions/SessionStore.java`, `.../manager/storage/protocol/SignalProtocolStore.java`, `.../manager/storage/recipients/RecipientStore.java`, `.../manager/storage/SignalAccount.java`, `.../manager/storage/groups/GroupInfoV2.java`, `.../manager/api/{MessageEnvelope,SendMessageResults,AttachmentInvalidException}.java`, `.../manager/util/MimeUtils.java`, `src/main/java/org/asamk/signal/commands/SendStoryCommand.java`, `src/main/java/org/asamk/signal/util/SendMessageResultUtils.java` |
| Y2 | signal-service-java (Turasa fork) | https://github.com/Turasa/libsignal-service-java @ v2.15.3_unofficial_151 (66956cb4a0394d50cee3590209baf58312084a44): `lib/libsignal-service/src/main/java/org/whispersystems/signalservice/api/SignalServiceMessageSender.java`, `.../api/message/MessageApi.kt`, `.../api/crypto/{SealedSenderAccess.kt,UnidentifiedAccess.java,SignalServiceCipher.java,SignalSealedSessionCipher.java}`, `.../api/keys/PreKeyRepository.kt`, `.../api/messages/{SignalServiceStoryMessage,SignalServiceStoryMessageRecipient}.java`, `.../api/messages/SignalServiceDataMessage.kt`, `.../api/messages/EnvelopeContentValidator.kt`, `.../api/push/DistributionId.java`, `.../internal/push/exceptions/InvalidUnidentifiedAccessHeaderException.java`, `lib/libsignal-service/src/main/protowire/SignalService.proto` (field numbers) |
| Y3 | libsignal | https://github.com/signalapp/libsignal @ v0.99.1 (97801d22dcf9f5bf714f7b8fa3212cdc973ae1c8): `rust/net/chat/src/ws/messages.rs` (request and response handling plus tests), `rust/net/chat/src/api/messages.rs`, `java/client/src/main/java/org/signal/libsignal/net/{UnauthMessagesService.kt,RequestResult.kt}`, `java/client/src/main/java/org/signal/libsignal/metadata/SealedSessionCipher.java` |
| Y4 | Signal-Server | https://github.com/signalapp/Signal-Server @ v20261022.0.0 (8edf3488f7d97398db7ffa5184c6b769a272ff59): `service/src/main/java/org/whispersystems/textsecuregcm/controllers/MessageController.java`, `.../entities/SendMultiRecipientMessageResponse.java`, `.../websocket/WebSocketConnection.java`. Used for server behaviour only. |

## 1. Wire messages

All proto2. Field numbers come from Y2 `SignalService.proto`. Wire emits
set fields in ascending field-number order for every message below.

### 1.1 `StoryMessage` (Content field 9)

`storyMessage` is a member of Content's `content` oneof (`content.md` §3).

| # | Name | Type | Notes |
|---|------|------|-------|
| 1 | profileKey | bytes | Sender's 32-byte profile key. |
| 2 | group | GroupContextV2 | Present only for a group story. |
| 3 | fileAttachment | AttachmentPointer | oneof `attachment` with 4 |
| 4 | textAttachment | TextAttachment | oneof `attachment` with 3 |
| 5 | allowsReplies | bool | |
| 6 | bodyRanges | repeated BodyRange | Style ranges over the text attachment's text. Same message as DataMessage body ranges (data-message spec). |

`GroupContextV2`: 1 `masterKey` bytes (32), 2 `revision` uint32,
3 `groupChange` bytes. The sender sets field 3 only for a signed group
change of at most 2048 bytes; story sends never set it.

### 1.2 `TextAttachment`

signal-cli never sends one. It only parses one on receive and passes it
to the output layer.

| # | Name | Type |
|---|------|------|
| 1 | text | string |
| 2 | textStyle | enum Style: DEFAULT 0, REGULAR 1, BOLD 2, SERIF 3, SCRIPT 4, CONDENSED 5 |
| 3 | textForegroundColor | uint32 (ARGB integer) |
| 4 | textBackgroundColor | uint32 |
| 5 | preview | Preview (same message as DataMessage previews) |
| 6 | gradient | Gradient, oneof `background` with 7 |
| 7 | color | uint32, oneof `background` with 6 |

`Gradient`:

| # | Name | Type | Notes |
|---|------|------|-------|
| 1 | startColor | uint32 | deprecated |
| 2 | endColor | uint32 | deprecated |
| 3 | angle | uint32 | degrees |
| 4 | colors | repeated uint32 | |
| 5 | positions | repeated float | 0 to 1 |

When signal-service-java builds a gradient, it also fills 1 and 2 with
the first and last of `colors`.

### 1.3 `DataMessage.StoryContext` (DataMessage field 21)

| # | Name | Type | signal-cli sends |
|---|------|------|------------------|
| 1 | authorAci | string | never |
| 2 | sentTimestamp | uint64 | the story's sent timestamp |
| 3 | authorAciBinary | bytes | 16-byte raw ACI of the story author |

Receivers reject a DataMessage whose storyContext has no parseable ACI in
either field 1 or field 3, or no `sentTimestamp` (Y2
`EnvelopeContentValidator`).

### 1.4 `SyncMessage.Sent` story fields

These are fields 8 `storyMessage` and 9 `storyMessageRecipients` of the
`SyncMessage.Sent` table in `content.md` §8.

`StoryMessageRecipient`:

| # | Name | Type | signal-cli sends |
|---|------|------|------------------|
| 1 | destinationServiceId | string | never |
| 2 | distributionListIds | repeated string | exactly one: a distribution ID in lowercase canonical UUID text form (§2.3) |
| 3 | isAllowedToReply | bool | always set |
| 4 | (reserved) | | |
| 5 | destinationServiceIdBinary | bytes | 16 bytes for an ACI; 17 bytes (kind prefix plus UUID) for a PNI |

### 1.5 Envelope `story` flag

Envelope field 16 `story` (bool) is set by the **server** on envelopes it
creates from a story send (`?story=true` on either endpoint). Receivers
reject an envelope flagged as a story unless its Content has one of:

- a senderKeyDistributionMessage;
- a storyMessage;
- a dataMessage with both storyContext and groupV2;
- a dataMessage with a delete.

## 2. `sendStory`: what is sent

### 2.1 Order of operations (Y1 `ManagerImpl.sendStory`)

1. **MIME check.** Probe the file's MIME type: OS/extension-based first,
   then content sniffing (attachments spec). If there is no type, or it
   starts with neither `image/` nor `video/`, fail with an
   attachment-invalid error whose message is
   `<path>: Stories only support image and video attachments`. The CLI
   prints `Failed to send story: <message> (AttachmentInvalidException)`
   and exits 2 (`sendStory.md`).
2. **Group path** if `-g` was given (§2.4). Otherwise continue with My
   Story.
3. **Recipient selection** (§2.2). An empty set fails with the I/O error
   `No eligible contacts found for story delivery`. This happens before
   any upload.
4. **Upload** the attachment (attachments spec). The result is an
   `AttachmentPointer`.
5. **Timestamp:** the next unique message timestamp (`send.md` §1 step 4).
6. **Build the StoryMessage:**
   - 1 = own profile key;
   - 3 = the pointer;
   - 5 = `allowsReplies`, true unless `--no-replies`. It is always
     emitted, including as `false`;
   - no group;
   - no body ranges.
7. **Sender-key send** with distribution ID **My Story** (§3), followed by
   the sync transcript (§2.5).
8. **Results:** one result per recipient, keyed by that recipient's
   address. Output is per `_send-results.md`.

### 2.2 My Story recipients

A recipient goes into the set if all of these hold in the local
recipient store (Y1 `RecipientStore.getRecipients` with "only contacts",
"not blocked", plus the filter in `ManagerImpl`):

- it has at least one of number, ACI, PNI;
- it is **not** marked unregistered;
- it is **not** hidden;
- it is **not** blocked;
- it is a "contact", meaning any of these is set: given/family name,
  nickname fields, note, expiration timer > 0, profile sharing, color,
  blocked, archived;
- its contact record does **not** have `hide_story`;
- it is not the account itself.

There are no distribution lists. signal-cli has no list management, and
all eligible contacts receive every My Story post.

Each recipient's address is resolved as for any send. A recipient stored
with only a phone number triggers a CDSI lookup. If that lookup fails,
the address keeps an unknown service ID (gap G3).

### 2.3 Distribution IDs

- **My Story:** the all-zero UUID `00000000-0000-0000-0000-000000000000`
  (Y2 `DistributionId.MY_STORY`).
- **Group:** each V2 group stored by signal-cli has its own random v4
  UUID, created when the group is first stored and persisted with the
  group. The same ID is used for group stories and (milestone 6) normal
  group messages.

### 2.4 Group story (`-g GROUP_ID`)

1. The group must exist (`Group not found: …`, exit 1). The account must
   be a member (`User is not a member in group: …`, exit 1). It must be a
   V2 group; otherwise the I/O error `Stories are only supported for V2
   groups` (exit 2).
2. Upload, then the StoryMessage as in §2.1, plus field 2 `group` =
   {masterKey, revision}. The revision is the stored group revision, or 0
   if the group state is not loaded.
3. Recipients: all members except self. Known-unregistered members are
   skipped and get an "unregistered" result.
4. The send goes through the shared group routine (milestone 6), with
   these story-specific facts:
   - It needs group-send endorsements for the group. If they are missing
     or expire within 2 hours, signal-cli first tries to refresh them from
     the groups service. If that fails, **no sender-key targets** are
     chosen.
   - A member is a sender-key target only if all of these hold:
     - it has an unidentified access (`send.md` §3);
     - it has a valid ACI;
     - an endorsement exists for it;
     - its stored identity is trusted.
   - If fewer than **2** members are targets, the target set is emptied.
   - **There is no fallback for group stories.** The 1:1 fallback that
     group data messages use is absent. Members that are not targets, and
     targets dropped after a fallback-worthy error (§3.6), receive
     **nothing** and get **no result entry**.
   - If nothing is sent, the result list contains only the
     "unregistered" entries, possibly none. An empty list counts as
     success: the CLI prints the timestamp and exits 0.
5. The multi-recipient request still uses **story authorization** (no
   token, §3.4). Endorsements are used only for target selection and for
   the SKDM fan-out (§3.2).
6. In the USMC, and on the 1:1 SKDM sends, the group ID is the group's
   **32-byte master key**. It is **not** the 32-byte group identifier
   that group data messages use (`sealed-sender.md` §8). The story code
   passes the master key there directly. This is a signal-cli quirk; a
   compatible client reproduces it.

### 2.5 Sync transcript

signal-cli sends it only when the account is multi-device, and only after
the multi-recipient call returned. That includes the case where it
returned per-recipient failures (§3.2). It is not sent when that call
threw. (Y2 `sendGroupStory`, `createSelfSendSyncMessageForStory`.)

Content = SyncMessage with field 1 `sent`:

| Sent field | Value |
|------------|-------|
| 2 timestamp | the story timestamp |
| 1 destinationE164 | own number, if known |
| 12 destinationServiceIdBinary | own ACI, 16 bytes |
| 5 unidentifiedStatus | exactly one entry: 6 `destinationServiceIdBinary` = own ACI, 2 `unidentified` = false |
| 8 storyMessage | identical to the StoryMessage that was sent |
| 9 storyMessageRecipients | one entry per recipient in the manifest: 5 = recipient service ID binary, 2 = [distribution ID text], 3 = allowsReplies |
| 6 isRecipientUpdate | false (always emitted) |

There is no `message`, no `expirationStartTimestamp`, and `padding` as in
`content.md` §8.

- **Manifest for My Story:** every selected recipient.
- **Manifest for a group story:** only the sender-key targets of that
  send. With fewer than 2 targets nothing is sent, so there is no sync
  either.

Transport: an unsealed 1:1 send to the own ACI (local device excluded),
with `?story=false`, `urgent=false`, content hint IMPLICIT, and no group
ID (`send.md` §3.2). It uses the story timestamp as its envelope
timestamp.

## 3. The sender-key send path

This path is used for every story. Normal group messages will use it in
milestone 6. Inputs:

- a distribution ID;
- the recipient list (service IDs);
- each recipient's unidentified access, which may be absent;
- optional group-send endorsements;
- the story timestamp;
- the Content;
- content hint IMPLICIT;
- the optional group ID (§2.4.6);
- `online=false`, `urgent=false`, `story=true`.

### 3.1 Target device set

The set is rebuilt at the start of every attempt (up to **4** attempts).

- **Destinations:** every (service ID, device) for which an *active*
  session exists, plus (service ID, device 1) for every recipient whether
  or not a session exists. "Active" means the stored session record has a
  current state with a sender chain (Y1 `SessionStore.isActive`).
- **Session map:** the subset of destinations that has an active
  session.

### 3.2 SKDM fan-out ("does this device have our key?")

**Shared-with table** (Y1 `SenderKeySharedStore`, SQLite table
`sender_key_shared`):

- Columns: `address` (service-ID string), `device_id`, `distribution_id`
  (16-byte UUID), `timestamp` (ms of last marking).
- Unique on (address, device_id, distribution_id).
- A row means "this device has been sent our current sender key for
  this distribution".

**Which recipients need the SKDM.** A destination needs it if it has no
row for this distribution, or it is not in the session map. Every
recipient with at least one such device needs the SKDM. It is sent per
recipient, not per device, so all of that recipient's devices get it
again.

**Building the SKDM.** If any recipient needs it, signal-cli gets or
creates our sender-key session for (own ACI, own device ID, distribution
ID). If no state exists, a new one is created: random chain ID, chain key
and signing key, iteration 0 (`sender-keys.md`). The SKDM reflects the
current chain state.

**Sending it.** A separate 1:1 send to each recipient that needs it,
through the multi-recipient 1:1 routine (`send.md` loop: eager prekey
fetch, 409/410 handling, REST fallback):

- Content has **only** field 7 `senderKeyDistributionMessage`.
- Content hint IMPLICIT; the group ID as in §2.4.6 (none for My Story).
- Envelope timestamp = the current clock at that moment. It is **not**
  the story timestamp.
- `online=false`, `urgent=false`.
- Path `PUT /v1/messages/{serviceId}?story=…`:
  - `story=true` for My Story;
  - `story=false` for a group story, because group distribution IDs are
    reused for normal group messages.
- **Access per recipient:**
  - *My Story* (no endorsements): sealed v1 with
    `Unidentified-Access-Key` if the recipient has an access, otherwise
    unsealed on the authenticated socket. The server does not check
    access keys for `story=true` (Y4), so the key is effectively unused,
    but signal-cli sends it anyway.
  - *Group story:* the access key first, with a group-send token
    fallback. With no access key, a `Group-Send-Token` (full token from
    the member's endorsement). Milestone 6.
  - signal-cli always marks its accesses as "not unrestricted-for-story".
    So the special "drop the access key for story prekey fetches" branch
    never fires. A 401 on a `story=true` prekey fetch is still retried
    once without the access header (Y2 `PreKeyRepository`).

**After the fan-out:**

- **Marking:** for each recipient whose SKDM send succeeded, every
  destination of that recipient from the pre-send device set (§3.1) gets
  a row. The timestamp is the current time, and an existing row's
  timestamp is overwritten. Devices that the SKDM send itself discovered
  (through a 409) receive the SKDM but are not marked, so the next send
  sends them an SKDM again.
- **Any SKDM failure aborts the story.** If any recipient's SKDM send
  failed (unregistered, identity, network, rate limit, proof required,
  invalid prekey), the multi-recipient request is **not** made. The
  routine returns the real failures plus a *network failure* result for
  every other recipient, including those whose SKDM succeeded. My Story
  then reports those results. A sync transcript is still sent (§2.5).
- **All succeeded:** the device set is rebuilt (sessions created during
  the fan-out now count) and encryption proceeds.

### 3.3 Encryption

The rest of the path, in order:

1. Pad the Content (`content.md` §2).
2. Encrypt it with our sender key for the distribution ID. This gives a
   SenderKeyMessage and advances the chain by one (`sender-keys.md` §5).
3. Wrap it in a USMC: message type 7 (SenderKey), the sender certificate,
   content hint IMPLICIT (2), and the group ID if any.
4. Seal it as sealed sender v2 for every destination in the device set,
   using each session's stored remote registration ID
   (`sealed-sender.md` §6).

**Sender certificate:** the certificate of the **first** recipient in
input order that has an unidentified access. Each access carries the
certificate signal-cli chose for that recipient, with or without E.164
depending on the phone-number-sharing mode (Y1
`UnidentifiedAccessHelper.getSenderCertificateFor`).

**No recipient has an access** (My Story only; group targets always have
one): there is no certificate, and signal-service-java fails with an
unchecked null-pointer error before anything is sent. The CLI reports
this as an uncaught exception (`spec/surface/cli/index.md`, exit 2).
This is gap G1: such a story can never be sent.

**Destination order:** the iteration order of a hash set, so one
recipient's devices may be non-adjacent. libsignal then emits repeated
entries for that service ID, which the server merges
(`sealed-sender.md` §6.3, §6.4). A compatible client may group devices
per recipient; the bytes are randomized anyway.

**Encoder errors:**

- A destination missing from the session map raises "no session".
- A registration ID above `0x3FFF` raises "invalid registration ID".
- An untrusted identity raises an identity error.

### 3.4 `PUT /v1/messages/multi_recipient`

**Transport.** A typed libsignal-net request on the **unauthenticated**
chat connection: a WebSocket request on the unauth socket, per
`transport.md`. That section notes the gRPC form is only used when
remote config key `grpc.MessagesAnonymousSendMultiRecipientMessage.2`
enables it, and signal-cli never does. There is **no REST fallback** for
this request.

**Request:**

| Item | Value |
|------|-------|
| Method | `PUT` |
| Path | `/v1/messages/multi_recipient?ts=<timestamp>&online=<bool>&urgent=<bool>` plus `&story=true` for a story send. Parameters appear in exactly this order; booleans are `true`/`false`; `ts` is decimal milliseconds. |
| Story send | `?ts=<story ts>&online=false&urgent=false&story=true` |
| `content-type` | `application/vnd.signal-messenger.mrm` |
| `group-send-token` | **only for non-story group sends**: standard base64 with padding of the serialized full token covering all recipients (milestone 6). Absent for stories. |
| `Unidentified-Access-Key` | never sent by libsignal 0.99.1. The server still accepts a combined (XOR) access key for non-story sends and ignores one on story sends. |
| Body | the raw sealed-sender v2 SentMessage (`sealed-sender.md` §6.3), no base64 |

**Server rules** (Y4 `MessageController.sendMultiRecipientMessage`):

- 400 if `ts` < 0 or `ts` > 8.64e15, if the recipient list is empty, or
  if a device appears twice for a recipient.
- 400 if a story send carries a group-send token.
- **Story sends:** no authorization check. Unknown service IDs are
  silently dropped. If none resolves, the response is 200 with nothing
  delivered. Each resolved recipient is checked against the per-account
  stories rate limiter (429).
- **Non-story sends** (milestone 6):
  - exactly one of the token and the combined key is required; none
    gives 401, both give 400;
  - an invalid token gives 401;
  - with the combined key, any unknown recipient gives 404, and a key
    mismatch gives 401.
- Device and registration-ID mismatches give 409, or 410 when there are
  only stale devices (shapes below).

**Responses** (Y3 `ws/messages.rs` parsing):

| Status | Body | Meaning |
|--------|------|---------|
| 200 | `{"uuids404":[…]}`; a missing member counts as empty | Success. `uuids404` lists service-ID strings (bare UUID for an ACI, `PNI:`+UUID for a PNI) of unregistered recipients. Only token sends fill it; **story sends always get `[]`**. An unparseable entry is an unexpected-response error. |
| 401 | empty | Unauthorized. It cannot occur for stories. |
| 409 | JSON array of `{"uuid":"<serviceId>","devices":{"missingDevices":[…],"extraDevices":[…]}}` | Device-list mismatch for each listed recipient. Nothing was delivered to anyone. |
| 410 | JSON array of `{"uuid":"<serviceId>","devices":{"staleDevices":[…]}}` | Registration-ID mismatch. Nothing was delivered. |
| 413 | | Message too large |
| 429 | `Retry-After` | Rate limited |
| 5xx | | Server-side error |

Parsing details for 409 and 410:

- Each `devices` object may omit any of its three arrays.
- An entry whose arrays are all empty or missing is an error.
- A non-array body is an error.
- A device ID outside 1..127 or a non-integer is an error.
- Any JSON body (200, 409, 410) is parsed only if the response
  content type is `application/json`; anything else is an
  unexpected-response error (Y3 `ws.rs`).

The server sends 409 if any recipient has missing or extra devices
(listing only those recipients). Otherwise it sends 410, listing the
recipients with stale devices.

### 3.5 Reaction to each outcome (Y2 `sendGroupMessage`)

- **200:** every recipient that appears in the device set gets a
  *success* result. That is every recipient, since device 1 is always
  included. The result has `unidentified=true`, its device list, and
  "needs sync" set to the multi-device flag. Recipients in `uuids404` get
  *unregistered* results instead. Because every success is unidentified,
  the sticky downgrade of `send.md` §3.4 never triggers on this path.
- **409/410:** for each entry:
  - archive our sessions with its extra devices;
  - delete shared-with rows for its extra and missing devices, across
    **all** distribution IDs;
  - for each missing device, fetch its bundle by device number on the
    authenticated socket and build a session;
  - if the entry has stale devices, archive those sessions and delete
    their shared-with rows (all distributions).

  Then start the next attempt (back to §3.1). Archived devices drop out
  of the device set; a fresh session or SKDM follows on that attempt
  through §3.2. If a missing device's bundle is invalid, the routine
  returns *invalid prekey* for that recipient and *network failure* for
  all others, without retrying.
- **401:** raises "invalid unidentified access header". It cannot occur
  for stories.
- **Retryable network error** (timeout, transport failure, connection
  invalidated, 5xx, 429 with a retry time): raised as an I/O error,
  carrying the retry-after time when there is one.
- **Anything else:** raised as an I/O error.
- **After 4 attempts:** the I/O error `Failed to resolve conflicts after
  4 attempts!`.

### 3.6 Errors seen by the caller

**My Story** (Y1 `SendHelper.sendStoryMessage`): untrusted identity, "no
session", invalid key, and invalid registration ID are wrapped as I/O
errors. Every I/O error reaches the CLI as `Failed to send story: <msg>
(IOException)` or a subclass name, exit 2. No result list is printed.

**Group story** (Y1 `sendGroupMessageInternalWithSenderKey`): these errors
mean "fall back", which for stories means **send nothing more**:

| Error | Extra action |
|-------|--------------|
| untrusted identity | none |
| invalid access header | none |
| invalid registration ID | none |
| not found | none |
| "no session" | delete our sender key for the distribution (§3.7) |
| invalid key | delete our sender key for the distribution (§3.7) |
| I/O error caused by an invalid key | none |

Other I/O errors propagate to the CLI, exit 2.

### 3.7 Rotation and reset

**Age rotation** (group path only, Y1
`sendGroupMessageInternalWithSenderKey`). Before each send, signal-cli
reads the `created_timestamp` of our own sender-key record for (own ACI,
own device, distribution). If a record exists and is older than **14
days**, it deletes our key:

- the record row is removed;
- **all** shared-with rows for that distribution are removed.

The next send then creates a new key and sends SKDMs to everyone.
`created_timestamp` is set when the row is first inserted. Later updates
of the record (each encryption advances the chain) keep it. **The My
Story path has no age check**, so the My Story key is never rotated by
age.

**Other resets of the shared-with state:**

| Trigger | Effect |
|---------|--------|
| Archiving a session through the protocol store's per-device archive, as the send loop's 409/410 handling does (Y1 `SignalProtocolStore.archiveSession`) | delete rows for that (address, device), all distributions |
| An identity key changes for a service ID | archive all its sessions; delete all rows for that service ID |
| A DecryptionErrorMessage (retry request) for one of our messages | for each matching send-log entry that has a group: delete the row for (sender, device, group distribution); if no matching entry has a group: delete all rows for the sender. Story sends are not written to the send log, so a retry request for a story takes the second branch. |
| `deleteOurKey` (age rotation, "no session", invalid key on the group path) | delete our record and all rows of that distribution |
| Recipient removed from the store | delete its rows and all sender-key records **received** from it |
| Registration finished, or linking finished | delete the whole sender-key store (records and rows) |

A resend of a group message (`send.md` retry handling) attaches a fresh
SKDM for the group's distribution to the re-sent Content (field 7
alongside the original content). On success it marks the recipient's
devices as shared. This is the only case where signal-cli sends field 7
together with other content. It is milestone 6, never used for stories.

### 3.8 Receiving SKDMs and sender-key messages

When a decrypted Content has field 7, signal-cli processes it into its
sender-key store under (sender service ID, sender device, distribution
ID taken from the SKDM). This happens before the other content is
handled and whatever else the Content holds. Decrypting a USMC of type 7
(SenderKey) uses that store (`sender-keys.md` §6). If no state is found,
signal-cli's normal decryption-error/retry path runs (`pipe.md`).

## 4. Story replies and reactions

Replies and reactions are **DataMessages** sent the normal way: 1:1 or
group. They are never sent through §3.

- **Reply** (`send --story-timestamp T --story-author A …`): the
  DataMessage gets field 21 storyContext with `authorAciBinary` =
  resolved ACI of A and `sentTimestamp` = T. Everything else follows the
  data-message spec. The recipients are whatever the command names; for
  a story by A, that is normally A, or the group for a group story.
- **Reaction / admin-delete / pin / unpin with `--story`:** the same
  DataMessage the command always sends (data-message spec), plus
  storyContext with author = the command's target author and
  `sentTimestamp` = the target timestamp.
- On receive, a DataMessage's storyContext is reported as
  `storyContext {authorNumber, authorUuid, sentTimestamp}`
  (`notifications.md` §4.12) and as `Story reply:` in plain text
  (`_received-message-output.md`). signal-cli stores no story state;
  replies are not checked against known stories or against
  `allowsReplies`.

## 5. Receiving stories

### 5.1 Opting in or out

The authenticated WebSocket upgrade carries `x-signal-receive-stories:
true`, or `false` with `--ignore-stories` on `receive`, `daemon` or
`jsonRpc` (`pipe.md` §2; Y1 `ReceiveHelper`, `SignalDependencies`). The
value is fixed when the socket is created.

With `false`, the server **acknowledges and deletes** every queued
envelope flagged `story` without delivering it to this device (Y4
`WebSocketConnection.sendMessage`). Those stories are lost for this
device, including My Story SKDMs, since those are also sent with
`story=true`. signal-cli does no client-side story filtering. A story
that arrives anyway is handled normally.

### 5.2 Handling an incoming StoryMessage (Y1 `IncomingMessageHandler`)

1. **Blocking.** The envelope is dropped as blocked if the sender is a
   blocked contact. It is also dropped if the story has a group context
   and that group is blocked.
2. **Group context present:** signal-cli looks up the group by its master
   key, fetching or updating it from the groups service if it is unknown
   or the revision is newer. That is the same handling as a group
   DataMessage (milestone 6).
3. **Attachments:** unless `--ignore-attachments`, the file attachment is
   downloaded, as is a text attachment's preview image if any
   (attachments spec).
4. **Profile key:** if field 1 is exactly 32 bytes, it is stored as the
   sender's profile key, as for a DataMessage profile key. Other lengths
   are ignored.
5. **No receipts.** No delivery receipt and no read receipt is sent for a
   story. Delivery receipts are only sent for data and edit messages.
6. **Nothing is persisted** beyond the effects above. The story is
   reported once through the output layer:
   - JSON `storyMessage`: `allowsReplies`, false when the field is absent;
     `groupId`; `fileAttachment` or `textAttachment`.
   - Plain-text `Story:` block.

### 5.3 Sync transcripts of stories from own devices

A `SyncMessage.Sent` with a storyMessage (from another device of the
same account) is processed through §5.2 steps 2 to 4, with the sender =
self. It is reported as `sentStoryMessage` (`notifications.md` §7.2) with
the transcript's destination. `storyMessageRecipients` is validated but
not reported or stored.

Validation (Y2 `EnvelopeContentValidator`) rejects a story transcript in
any of these cases:

- a manifest entry has a service ID that does not parse;
- a manifest entry that has a service ID lacks `isAllowedToReply`;
- there is a manifest, no storyMessage, and `isRecipientUpdate` is not
  true;
- the story group context is invalid.

A Sent with a manifest and no valid destination is accepted. The
transcript in §2.5 always carries a valid destination, the own ACI.

## 6. Checklist for a compatible client (stories, milestone 5)

1. Select recipients by the §2.2 rules; distribution ID all-zero.
2. Upload; build StoryMessage {1 profile key, 3 pointer, 5
   allowsReplies}.
3. Build the device set (§3.1). For recipients needing it, send an
   SKDM-only Content 1:1 with `?story=true`, sealed if an access exists.
   Mark devices as shared on success. Abort on any SKDM failure, as in
   §3.2.
4. Sender-key encrypt the padded Content. USMC: type 7, hint IMPLICIT,
   no group ID. Then seal with v2.
5. `PUT /v1/messages/multi_recipient?ts=T&online=false&urgent=false&story=true`
   on the unauthenticated socket, with `content-type:
   application/vnd.signal-messenger.mrm` and no auth header.
6. Handle 409/410 by the §3.5 rules, up to 4 attempts.
7. If multi-device, send the §2.5 sync transcript.

## Gaps

- **G1.** A My Story send where no selected recipient has an
  unidentified access fails with an unchecked null-pointer error inside
  signal-service-java (§3.3). The exact CLI stderr text was not observed
  black-box.
- **G2.** No live capture of a story send. Everything here comes from
  source; the server rules are from Signal-Server main at the cited
  commit, and no tag matches the deployed server.
- **G3.** A contact with only a phone number, whose CDSI lookup fails,
  enters the recipient list with an unknown service ID. The resulting
  SKDM and v2 behaviour was not traced. PNI-only contacts would be
  addressed by PNI in the v2 entries and SKDM; untested.
- **G4.** Group-story target selection depends on group-send endorsements
  and the groups service (milestone 6). Without endorsements, a group
  story silently sends nothing and reports success (§2.4).
- **G5.** Fixed upstream vectors exist only for the request line, headers
  and response parsing of `multi_recipient`
  (`testdata/vectors/stories.json`). There are none for StoryMessage
  bytes, SKDM fan-out, or end-to-end sender-key ciphertexts (upstream
  randomizes them).
