# signal-cli 0.14.7 JSON-RPC notifications: `receive` and `callEvent`

## Sources

All upstream URLs are at tag **v0.14.7** (https://github.com/AsamK/signal-cli/tree/v0.14.7):

- `src/main/java/org/asamk/signal/json/` (every file: envelope, data, sync, edit, story, receipt, typing, call, attachment, quote, mention, reaction, preview, sticker, shared contact and its parts, group info, remote delete, admin delete, pin, unpin, polls, payment, text style, story context, error, call event)
  https://github.com/AsamK/signal-cli/tree/v0.14.7/src/main/java/org/asamk/signal/json
- `src/main/java/org/asamk/signal/jsonrpc/SignalJsonRpcDispatcherHandler.java` (subscription ids, notification framing)
- `src/main/java/org/asamk/signal/http/HttpServerHandler.java`, `http/ServerSentEventSender.java` (SSE delivery)
- `src/main/java/org/asamk/signal/util/Hex.java` (sticker pack id formatting)
- `lib/src/main/java/org/asamk/signal/manager/api/MessageEnvelope.java`, `RecipientAddress.java`, `CallInfo.java`, `Color.java`, `TextStyle.java` (enum value sets, which source fields are optional)
- `lib/src/main/java/org/asamk/signal/manager/helper/CallManager.java` (when call events fire, `reason` strings)
- `lib/src/main/java/org/asamk/signal/manager/util/Utils.java` (unsigned call-id rendering)
- `lib/src/main/java/org/asamk/signal/manager/internal/ManagerImpl.java` (receive-handler semantics)
- `docs/CALL_TUNNEL.md`
- Tests read for behavior confirmation: `src/test/java/org/asamk/signal/json/JsonCallEventTest.java`, `src/test/java/org/asamk/signal/jsonrpc/SubscribeCallEventsTest.java`
- Upstream release asset (data only): https://github.com/AsamK/signal-cli/releases/download/v0.14.7/signal-cli-0.14.7-json-schemas.tar.gz,
  sha256 `5ca03152ce113bdf6aaaa2176b82858cad399ebdf0862ff11cde51863674fc1c` (verified before extraction). Used to
  cross-check key names, types and enum values. The schemas do **not** encode nullability; nullability below comes from
  the source.
- Binary observation: none for this file (a zero-account daemon produces no notifications). Key order of the bare
  notification object was determined by evaluating Java HashMap iteration order for the three key strings with a
  throw-away local Java program (not upstream code).

## Serialization ground rules

- Objects are emitted with keys in the order listed in this document (declaration order). The one exception is the
  top-level "message wrapper" object (see below), whose order is `exception`, `envelope`, `account`.
- "always present" = the key is always emitted; if it is also "nullable" it may carry JSON `null`.
- "omitted when absent/empty" = the key is left out entirely instead of being `null` (or an empty list).
- Integers marked int64 are Java `long` (millisecond timestamps etc.). Call-message ids are **unsigned** 64-bit
  integers rendered as plain JSON numbers (may exceed 2^63-1). Call-event ids are **signed** 64-bit (may be negative).
- Byte arrays (payment receipt) are rendered as standard base64 strings (with padding).
- Each JSON value is written as one line terminated by the platform line separator (`\n` on Linux) on socket/TCP/stdio.

## 1. The message wrapper object

Every received item (message or decryption failure) is turned into one wrapper object:

| key | type | presence |
|---|---|---|
| `exception` | Error object (section 9) | omitted when processing succeeded |
| `envelope` | Envelope object (section 2) | omitted only if there is no envelope at all (rare; failure before an envelope exists) |
| `account` | string, the account's own phone number (E.164) | always present (may be `null` if the account has no number, not expected in practice) |

Key order as emitted: `exception` (if present), `envelope`, `account`.
The same wrapper object is used for: the `receive` notification params (internal subscription), the `result` inside an
explicit-subscription notification, each SSE `data:` line, and each element of the `receive` method's result array.

## 2. Delivery per transport

### Socket (UNIX), TCP, and the `jsonRpc` stdio command

Notifications are JSON-RPC 2.0 requests **without** an `id` key, method `"receive"` (or `"callEvent"`), written on the
same connection as responses, one per line. Keys are emitted in order `jsonrpc`, `method`, `params` (no `id`).

Two shapes, depending on how the subscription was created:

1. **Implicit (automatic) subscription** — created when a connection opens, unless the daemon/`jsonRpc` runs with
   `--receive-mode manual`. `params` is the bare wrapper object:
   `{"jsonrpc":"2.0","method":"receive","params":{"envelope":{...},"account":"+15550000000"}}`
2. **Explicit subscription** (created by the `subscribeReceive` method): `params` is
   `{"subscription": <int>, "result": <wrapper object>}` (key order `subscription`, `result`):
   `{"jsonrpc":"2.0","method":"receive","params":{"subscription":1,"result":{"envelope":{...},"account":"+15550000000"}}}`

A connection may hold the implicit subscription and any number of explicit ones simultaneously; each one delivers its
own copy of every message. Subscription ids come from one process-wide counter starting at 0, shared by receive and
call-event subscriptions and by all connections; the implicit subscription also consumes an id (it is never reported to
the client). See `methods/subscribeReceive.md`.

Multi-account mode: the implicit subscription covers all loaded accounts; the `account` key in the wrapper tells them
apart. Quirk (multi-account): when an account is added at runtime (e.g. after `register`/`verify` or `finishLink` in the
same daemon), the new account gets its own new implicit subscription **and** is additionally attached, in wrapped
`{subscription, result}` form, to every subscription that existed on the connection at that moment, including the
implicit ones. A client of such a connection can therefore receive the same message several times (once bare, once or
more wrapped with ids it never asked for). Reproducing this is only needed for exact parity.

If writing a notification fails because the connection is closed, that subscription is dropped silently.

### HTTP

`POST /api/v1/rpc` never carries notifications (`subscribeReceive` etc. are not available over HTTP; they return
-32601 `Method not implemented`). Messages are delivered over Server-Sent Events at `GET /api/v1/events`:

- Response header `Content-Type: text/event-stream`, status 200, chunked; headers are flushed immediately.
- Each message: the lines `event:receive`, `data:<wrapper object as single-line JSON>`, then an empty line. No `id:`
  line. No space after the colons. Lines end with `\n`.
  ```
  event:receive
  data:{"envelope":{...},"account":"+15550000000"}

  ```
- Keep-alive: every 15 seconds without other activity a comment line `:` followed by `\n` is written (a single line,
  not followed by a blank line).
- The SSE payload is the bare wrapper (never the `{subscription,result}` form).
- Multi-account mode: optional query parameter `account=<number>` restricts the stream to that account; missing/empty
  means all accounts loaded at connection time. Unknown account -> HTTP 400 with empty body. Single-account mode
  ignores the query.
- Opening an SSE stream counts as an active (strong) receiver for the account(s), so it starts message reception and
  makes the `receive` method fail with "already being received" while it is open.

`callEvent` notifications are never sent over HTTP/SSE.

## 3. Envelope object

| key | type | presence / notes |
|---|---|---|
| `source` | string | always present, nullable. Deprecated legacy identifier: sender's phone number if known, else ACI UUID string (else PNI, else username) |
| `sourceNumber` | string | always present, nullable. Sender E.164 |
| `sourceUuid` | string | always present, nullable. Sender ACI UUID (lower-case canonical form) |
| `sourceName` | string | always present, nullable. Contact name, else profile name of sender, as known locally |
| `sourceDevice` | integer | always present, nullable. Sender device id |
| `timestamp` | int64 | always present. Envelope/client timestamp (ms) |
| `serverReceivedTimestamp` | int64 | always present |
| `serverDeliveredTimestamp` | int64 | always present |
| `dataMessage` | DataMessage | omitted when absent |
| `editMessage` | EditMessage | omitted when absent |
| `storyMessage` | StoryMessage | omitted when absent |
| `syncMessage` | SyncMessage | omitted when absent |
| `callMessage` | CallMessage | omitted when absent |
| `receiptMessage` | ReceiptMessage | omitted when absent |
| `typingMessage` | TypingMessage | omitted when absent |

Sender resolution: if the envelope has a sender (non-sealed or successfully unsealed), its address and device are used.
If not, but the accompanying exception is an untrusted-identity failure, the sender and device from that failure are
used. Otherwise all five `source*` keys are `null`. There is no unidentified-sender flag in the JSON.

## 4. DataMessage object

| key | type | presence / notes |
|---|---|---|
| `timestamp` | int64 | always |
| `message` | string | always, nullable. Body text; if a long-text attachment was received, its full text replaces the body |
| `expiresInSeconds` | integer | always (0 = no disappearing timer) |
| `isExpirationUpdate` | boolean | always |
| `viewOnce` | boolean | always |
| `reaction` | Reaction | omitted when absent |
| `quote` | Quote | omitted when absent |
| `payment` | Payment | omitted when absent |
| `mentions` | array of Mention | omitted when empty |
| `previews` | array of Preview | omitted when empty |
| `attachments` | array of Attachment | omitted when empty |
| `sticker` | Sticker | omitted when absent |
| `remoteDelete` | RemoteDelete | omitted when absent |
| `contacts` | array of SharedContact | omitted when empty |
| `pollCreate` | PollCreate | omitted when absent |
| `pollVote` | PollVote | omitted when absent |
| `pollTerminate` | PollTerminate | omitted when absent |
| `textStyles` | array of TextStyle | omitted when empty |
| `groupInfo` | GroupInfo | omitted when not a group message |
| `storyContext` | StoryContext | omitted when not a story reply |
| `pinMessage` | PinMessage | omitted when absent |
| `unpinMessage` | UnpinMessage | omitted when absent |
| `adminDelete` | AdminDelete | omitted when absent |

Not represented in JSON at all: end-session flag, profile-key-update flag, has-profile-key, group-call update, body
ranges other than styles/mentions, voice-note/gif/borderless attachment flags, preview date.

### 4.1 Reaction
`emoji` string; `targetAuthor` string (deprecated legacy identifier: number else uuid); `targetAuthorNumber` string|null;
`targetAuthorUuid` string|null; `targetSentTimestamp` int64; `isRemove` boolean. All always present.

### 4.2 Quote
| key | type | presence |
|---|---|---|
| `id` | int64 | always (timestamp of quoted message) |
| `author` | string | always (deprecated legacy identifier) |
| `authorNumber` | string | always, nullable |
| `authorUuid` | string | always, nullable |
| `text` | string | always, nullable |
| `mentions` | array of Mention | omitted when empty |
| `attachments` | array of QuotedAttachment | **always present**, `[]` when none |
| `textStyles` | array of TextStyle | omitted when empty |

QuotedAttachment: `contentType` string (always); `filename` string|null (always); `thumbnail` Attachment (omitted when
absent).

### 4.3 Mention
`name` string (deprecated legacy identifier), `number` string|null, `uuid` string|null, `start` integer, `length` integer
(UTF-16 code units). All always present.

### 4.4 Attachment
All keys always present, each nullable except `contentType`:
`contentType` string; `filename` string|null; `id` string|null; `size` integer|null (bytes); `width` integer|null;
`height` integer|null; `caption` string|null; `uploadTimestamp` int64|null.

- `id` is the local stored-file name of the downloaded attachment (usable with `getAttachment`); `null` for
  attachments that are not remote pointers (and for quoted-attachment metadata).
- `width`/`height`/`uploadTimestamp` are `null` when the sender sent 0. An empty caption becomes `null`.

### 4.5 Preview
`url` string, `title` string|null, `description` string|null, `image` Attachment|null — all always present.

### 4.6 Sticker
`packId` string: lower-case hex of the pack id bytes, no separators; `stickerId` integer. Pack key is not included.

### 4.7 RemoteDelete
`timestamp` int64 (target sent timestamp).

### 4.8 SharedContact
| key | type | presence |
|---|---|---|
| `name` | ContactName | always |
| `avatar` | ContactAvatar | omitted when absent |
| `phone` | array of ContactPhone | omitted when empty |
| `email` | array of ContactEmail | omitted when empty |
| `address` | array of ContactAddress | omitted when empty |
| `organization` | string | always, nullable |

- ContactName: `nickname`, `given`, `family`, `prefix`, `suffix`, `middle` — all strings, always present, `null` when
  missing or blank.
- ContactAvatar: `attachment` Attachment, `isProfile` boolean.
- ContactPhone / ContactEmail: `value` string, `type` one of `HOME`, `WORK`, `MOBILE`, `CUSTOM`, `label` string|null
  (null when blank).
- ContactAddress: `type` one of `HOME`, `WORK`, `CUSTOM`; `label`, `street`, `pobox`, `neighborhood`, `city`, `region`,
  `postcode`, `country` — strings, always present, null when blank.

### 4.9 Polls
- PollCreate: `question` string, `allowMultiple` boolean, `options` array of string.
- PollVote: `author` string (deprecated legacy identifier of the poll's author), `authorNumber` string|null,
  `authorUuid` string|null, `targetSentTimestamp` int64, `optionIndexes` array of integer, `voteCount` integer.
- PollTerminate: `targetSentTimestamp` int64.

### 4.10 TextStyle
`style` one of `NONE`, `BOLD`, `ITALIC`, `SPOILER`, `STRIKETHROUGH`, `MONOSPACE`; `start` integer; `length` integer.

### 4.11 GroupInfo
`groupId` string (base64 of group id); `groupName` string|null (locally known title); `revision` integer (0 for
groups without revision); `type` `"UPDATE"` if the message is a group update, else `"DELIVER"`. All always present.

### 4.12 StoryContext
`authorNumber` string|null, `authorUuid` string|null, `sentTimestamp` int64.

### 4.13 Payment
`note` string|null, `receipt` base64 string (of the receipt bytes; `null` if absent).

### 4.14 PinMessage / UnpinMessage / AdminDelete
- PinMessage: `targetAuthor` (deprecated legacy identifier), `targetAuthorNumber` string|null, `targetAuthorUuid`
  string|null, `targetSentTimestamp` int64, `pinDurationSeconds` int64.
- UnpinMessage: `targetAuthor`, `targetAuthorNumber`, `targetAuthorUuid`, `targetSentTimestamp`.
- AdminDelete: `targetAuthor`, `targetAuthorNumber`, `targetAuthorUuid`, `targetSentTimestamp`.

## 5. EditMessage
`targetSentTimestamp` int64; `dataMessage` DataMessage (the replacement content; full DataMessage rules apply).

## 6. StoryMessage
| key | type | presence |
|---|---|---|
| `allowsReplies` | boolean | always |
| `groupId` | string (base64) | omitted when not a group story |
| `fileAttachment` | Attachment | omitted when absent |
| `textAttachment` | TextAttachment | omitted when absent |

TextAttachment: `text` string|null (always present); `style` (omitted when absent) one of `DEFAULT`, `REGULAR`,
`BOLD`, `SERIF`, `SCRIPT`, `CONDENSED`; `textForegroundColor`, `textBackgroundColor` (omitted when absent); `preview`
Preview (omitted when absent); `backgroundGradient` Gradient (omitted when absent); `backgroundColor` (omitted when
absent). Colors are `#` followed by 8 lower-case hex digits of the 32-bit ARGB value, e.g. `#ff112233`.

Gradient (all keys always present): `startColor` string|null and `endColor` string|null — filled only for "legacy"
two-stop gradients (exactly 2 colors at positions 0 and 1), otherwise null; `colors` array of color strings;
`positions` array of numbers (floats); `angle` integer|null.

## 7. SyncMessage

| key | type | presence |
|---|---|---|
| `sentMessage` | SyncDataMessage | omitted unless a sent transcript **without** a story |
| `sentStoryMessage` | SyncStoryMessage | omitted unless a sent transcript **with** a story |
| `blockedNumbers` | array of string | omitted unless a blocked-list sync; legacy identifiers (number else uuid) |
| `blockedGroupIds` | array of string (base64) | present iff `blockedNumbers` present |
| `readMessages` | array of SyncReadMessage | omitted when empty |
| `type` | string `CONTACTS_SYNC` or `GROUPS_SYNC` | omitted otherwise. `CONTACTS_SYNC` if a contacts sync was received, else `GROUPS_SYNC` if a groups sync; the schema also lists `REQUEST_SYNC` but it is never emitted |

Viewed sync, view-once-open sync, message-request-response sync and other sync kinds are **not** rendered; such a sync
message appears as `"syncMessage":{}`.

### 7.1 SyncDataMessage (`sentMessage`)
Keys in order: `destination` (deprecated legacy identifier of 1:1 destination, string|null, always present),
`destinationNumber` string|null (always), `destinationUuid` string|null (always), `editMessage` EditMessage (omitted when
absent), followed by **all DataMessage keys flattened into the same object** (not nested), following the DataMessage
presence rules. When the transcript has no data message (e.g. edit-only), no DataMessage keys appear. Group sends have
null destinations and carry `groupInfo`.

### 7.2 SyncStoryMessage (`sentStoryMessage`)
`destinationNumber` string|null, `destinationUuid` string|null (always present), followed by all StoryMessage keys
flattened into the same object.

### 7.3 SyncReadMessage
`sender` (deprecated legacy identifier), `senderNumber` string|null, `senderUuid` string|null, `timestamp` int64.

## 8. ReceiptMessage, TypingMessage, CallMessage

ReceiptMessage (all always present): `when` int64; `isDelivery` boolean; `isRead` boolean; `isViewed` boolean;
`timestamps` array of int64. For an unknown receipt type all three booleans are false.

TypingMessage: `action` `"STARTED"` or `"STOPPED"`; `timestamp` int64; `groupId` base64 string (omitted when not a group).

CallMessage (1:1 call signalling as received from the peer):
| key | type | presence |
|---|---|---|
| `offerMessage` | `{id, type, opaque}` | omitted when absent |
| `answerMessage` | `{id, opaque}` | omitted when absent |
| `busyMessage` | `{id}` | omitted when absent |
| `hangupMessage` | `{id, type, deviceId}` | omitted when absent |
| `iceUpdateMessages` | array of `{id, opaque}` | omitted when empty |

`id` is the call id as an **unsigned** 64-bit JSON number; `opaque` is base64; offer `type` is `AUDIO_CALL` or
`VIDEO_CALL`; hangup `type` is one of `NORMAL`, `ACCEPTED`, `DECLINED`, `BUSY`, `NEED_PERMISSION`; `deviceId` integer.
Opaque (group-call) messages and destination device/timestamp/urgency are not rendered (may yield `"callMessage":{}`).

## 9. Error object (`exception`)
`message` string|null (the failure's message text); `type` string (the short name of the failure kind, e.g.
`UntrustedIdentityException`, `ProtocolInvalidMessageException`, `ProtocolNoSessionException`,
`ProtocolDuplicateMessageException`, `ProtocolInvalidKeyIdException`, `SelfSendException`; the set is open-ended — it is
whatever kind of error reception produced). Both always present.

## 10. `callEvent` notification

Only on socket/TCP/stdio connections that called `subscribeCallEvents`. Never automatic, never over HTTP.

`{"jsonrpc":"2.0","method":"callEvent","params":{"subscription":<int>,"result":<CallEvent>}}`

CallEvent:
| key | type | presence |
|---|---|---|
| `callId` | int64 (signed) | always |
| `state` | string | always; one of `IDLE`, `RINGING_INCOMING`, `RINGING_OUTGOING`, `CONNECTING`, `CONNECTED`, `RECONNECTING`, `ENDED` |
| `number` | string | omitted when peer number unknown |
| `uuid` | string | omitted when peer ACI unknown |
| `isOutgoing` | boolean | always |
| `inputDeviceName` | string | omitted until the media tunnel reported it |
| `outputDeviceName` | string | omitted until the media tunnel reported it |
| `reason` | string | omitted when none |

When events fire: outgoing call started (`RINGING_OUTGOING`); incoming offer (`RINGING_INCOMING`); local accept
(`CONNECTING`); peer answer received for an outgoing call (`CONNECTING`); media state changes (`CONNECTED`,
`RECONNECTING`, or ringing states, with an optional `reason` passed through from the media layer); and call end
(`ENDED`). End `reason` values emitted by signal-cli itself: `local_hangup` (hangupCall), `rejected` (rejectCall),
`remote_hangup`, `remote_busy`, `ring_timeout` (still ringing after the ring timeout), `shutdown` (daemon/account
closing), `tunnel_exit`, `tunnel_spawn_error`, `tunnel_error`; when the media layer reports the call ended/rejected the
reason is the media layer's reason string if it gave one, else `ended` or `rejected`. Incoming call offers are ignored entirely (no event, no state) when no call-event subscription exists for the
account.
