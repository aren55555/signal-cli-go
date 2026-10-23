# The authenticated message pipe and the `receive` command

Spec role: prose only; no code. This file says what travels over the chat
WebSocket while signal-cli 0.14.7 receives messages, how envelopes are
acknowledged, how the connection is kept alive and re-established, and
what `receive` does end to end, including how it answers decryption
failures.

Related specs, not repeated here:

- `spec/service/transport.md`: TLS, hosts, and how libsignal-net opens the
  connection (parallel agent).
- `spec/service/api.md`: authentication and the general request catalogue.
- `spec/service/send.md`: `PUT /v1/messages/{destination}` and its errors.
- `spec/service/keys.md`: prekey refresh.
- `spec/service/content.md`: Content, DataMessage, ReceiptMessage,
  NullMessage, the padding scheme.
- `spec/protocol/capture.md` §3: the Envelope field table and the
  on-disk message cache.
- `spec/protocol/sealed-sender.md`, `double-ratchet.md`, `pqxdh.md`:
  decryption itself.

## Sources

| Ref | What | URL @ version |
|-----|------|---------------|
| P1 | signal-cli | https://github.com/AsamK/signal-cli @ v0.14.7 (b01b6b370dc063599a1a2b9fde0f5ff4e2d78fe8): `lib/src/main/java/org/asamk/signal/manager/helper/{ReceiveHelper,IncomingMessageHandler,SendHelper}.java`, `.../manager/internal/{SignalDependencies,SignalWebSocketHealthMonitor,ManagerImpl}.java`, `.../manager/actions/{SendReceiptAction,SendRetryMessageRequestAction,RenewSessionAction,SendProfileKeyAction,ResendMessageAction,RefreshPreKeysAction}.java`, `.../manager/config/ServiceConfig.java`, `src/main/java/org/asamk/signal/{BaseConfig,commands/ReceiveCommand}.java`; jar manifest of `lib/signal-cli-0.14.7.jar` in the reference distribution |
| P2 | signal-service-java (Turasa fork) | https://github.com/Turasa/libsignal-service-java @ v2.15.3_unofficial_151 (66956cb4a0394d50cee3590209baf58312084a44): `lib/libsignal-service/src/main/java/org/whispersystems/signalservice/api/websocket/SignalWebSocket.kt`, `.../internal/websocket/{LibSignalChatConnection,WebSocketConnection}.kt`, `.../api/crypto/{SignalServiceCipher,EnvelopeContent}.java`, `.../api/SignalServiceMessageSender.java`, `.../api/messages/EnvelopeContentValidator.kt`, `.../api/message/MessageApi.kt`, `core/network/src/main/protowire/WebSocketResources.proto` |
| P3 | Reference jar | `/work/third_party/signal-cli-0.14.7/lib/core-network-2.15.3_unofficial_151.jar`: Wire `@WireField` annotations of `org.signal.network.websocket.{WebSocketMessage,WebSocketRequestMessage,WebSocketResponseMessage}` read with `javap -v` (field tags and types below come from here) |
| P4 | libsignal | https://github.com/signalapp/libsignal @ v0.99.1 (97801d22dcf9f5bf714f7b8fa3212cdc973ae1c8): `rust/net/src/{chat.rs,env.rs,auth.rs}`, `rust/net/src/chat/{ws.rs,server_requests.rs,error.rs}`, `rust/net/infra/src/{lib.rs,timeouts.rs}`, `rust/bridge/shared/src/net/chat.rs`, `rust/bridge/shared/types/src/net/chat.rs`, `rust/core/src/version.rs`, `rust/protocol/src/protocol.rs` (PlaintextContent, DecryptionErrorMessage), `rust/protocol/proto/service.proto`, `java/client/src/main/java/org/signal/libsignal/net/Network.java` |

## 1. Who speaks the WebSocket protocol

In 0.14.7 both chat sockets are **libsignal-net connections**, not
OkHttp WebSockets (P1 `SignalDependencies`: both the authenticated and
the unauthenticated socket are built from libsignal's chat connection over
one shared libsignal `Network` object). The Java layer never sees raw
frames. libsignal-net owns the framing, request IDs, server-request
parsing and WebSocket ping/pong; signal-service-java wraps it in an
adapter that re-creates the old "request/response message" view (P2
`LibSignalChatConnection`). Everything below is what goes over the wire,
whichever layer produces it.

How the TLS connection and route are chosen belongs to `transport.md`.
Whether some typed libsignal requests travel over gRPC instead of the
WebSocket is also a transport question (P4 has a gRPC override table
driven by remote configuration; signal-cli passes an empty remote-config
map when it creates the `Network`). **Not determined here.**

## 2. Connecting

| Item | Value |
|------|-------|
| Path | `/v1/websocket/` (with trailing slash), no query string (P4 `env.rs`). |
| Frames | Every message is one **binary** WebSocket frame holding one encoded `WebSocketMessage` (§3). |
| Authenticated socket | Upgrade request carries `Authorization: Basic base64("<ACI>.<deviceId>:<password>")`. The username is always `ACI.deviceId`, device ID included, even for the primary device (`.1`) (P4 `chat.rs`, bridge `parse_username`). Also `x-signal-receive-stories: true` or `false`; signal-cli sends `true` unless stories are disabled (P1 `SignalDependencies.allowStories`, default true; `receive --ignore-stories` turns it off). `Accept-Language` only if languages are configured; signal-cli configures none. |
| Unauthenticated socket | Same path, no `Authorization` and no receive-stories header. |
| User-Agent | `Signal-Android/8.21.1 signal-cli/0.14.7 libsignal/0.99.1`. The first token can be overridden with the `SIGNAL_CLI_USER_AGENT` environment variable; libsignal appends ` libsignal/<version>` (P1 `BaseConfig` and jar manifest; P4 `env.rs` `UserAgent`). Details in `transport.md`. |
| Upgrade response headers | `x-signal-timestamp` (server time in ms): signal-cli only logs a warning if the clock is off by more than one day. `x-signal-alert` (comma-separated values, may repeat): logged at INFO, nothing else. |
| Upgrade failure | HTTP 403 means the device is deregistered: signal-cli marks the account as not registered and stops. HTTP 499 means the client version is expired. A `Retry-After` header takes precedence over both. Other statuses are generic connect failures (P4 `chat/error.rs`; P2 `LibSignalChatConnection`; P1 `ReceiveHelper.onWebSocketStateChange`). |

libsignal allows only one authenticated connection attempt to complete at
a time, because the server closes earlier authenticated sockets for the
same device when a new one comes up (P4 `chat.rs`). A Go client should
likewise keep a single authenticated socket per device.

## 3. Frame messages

Field numbers and types come from the Wire annotations (P3) and agree
with P2 `WebSocketResources.proto` and P4. All fields are proto2
`optional` unless marked repeated.

**WebSocketMessage**

| # | Name | Type | Meaning |
|---|------|------|---------|
| 1 | type | enum Type | UNKNOWN = 0, REQUEST = 1, RESPONSE = 2 |
| 2 | request | WebSocketRequestMessage | set when type is REQUEST |
| 3 | response | WebSocketResponseMessage | set when type is RESPONSE |

**WebSocketRequestMessage**

| # | Name | Type | Meaning |
|---|------|------|---------|
| 1 | verb | string | HTTP method, upper case (`GET`, `PUT`, …) |
| 2 | path | string | path plus query, e.g. `/api/v1/message` |
| 3 | body | bytes | request body (JSON for client requests; binary Envelope for server deliveries) |
| 4 | id | uint64 | request ID, echoed in the response |
| 5 | headers | repeated string | each entry is `name: value`; libsignal writes lower-case names with `": "` between name and value; parsing splits at the first colon and trims |

**WebSocketResponseMessage**

| # | Name | Type | Meaning |
|---|------|------|---------|
| 1 | id | uint64 | ID of the request being answered |
| 2 | status | uint32 | HTTP status code |
| 3 | message | string | reason phrase, e.g. `OK` |
| 4 | body | bytes | response body (JSON for chat-server replies) |
| 5 | headers | repeated string | `name: value` strings, as for requests |

Requests flow both ways. Each side numbers its own requests; the IDs of
client requests and of server requests are independent. libsignal starts
its client request counter at 0 for each connection and adds 1 per
request, wrapping at 2^64 (P4 `ws.rs`, `RECOMMENDED_CHAT_WS_CONFIG`). The
server does not care about the starting value, only that responses echo
it.

## 4. Server-to-client requests

The server pushes two kinds of request on the **authenticated** socket.
Anything else (other verb, other path, or a missing path) is logged by
libsignal and dropped **without a response** (P4 `server_requests.rs`,
bridge `into_event_listener`).

### 4.1 Envelope delivery: `PUT /api/v1/message`

- `body` is the **binary protobuf `Envelope`** (field table in
  `spec/protocol/capture.md` §3), not JSON and not base64.
- One header `X-Signal-Timestamp: <ms>` gives the server delivery time.
  The name is matched case-insensitively and the value is trimmed and
  parsed as an unsigned decimal; if several are present the last valid
  one wins; if none parses, 0 is used. signal-cli keeps this value as the
  envelope's "server delivered timestamp" (stored in the message cache
  and used, among other things, to expire untrusted-identity retries after
  14 days).
- A missing or empty body is still delivered upward as an empty envelope
  so that it can be acknowledged; otherwise the server would redeliver it
  forever (P4).

**Acknowledgement.** The client answers with a `WebSocketMessage` of type
RESPONSE whose response has `id` = the request's `id`, `status` = 200,
`message` = `OK`, and no body or headers (P4 `response_for_status`,
`ServerMessageAck_Send`). This response deletes the envelope from the
server queue. No other status is ever sent for envelopes by signal-cli.
The signal-service adapter forwards only a 200/`OK` acknowledgement to
libsignal; any other response it is asked to send is silently dropped
(P2 `LibSignalChatConnection.sendResponse`).

**When signal-cli acks:** immediately after it has written the envelope to
its on-disk message cache, and **before** decrypting it (P1
`ReceiveHelper`; cache format in `capture.md` §3). Decryption failures
therefore never cause a redelivery; recovery happens through the cache
(untrusted identity) or through retry receipts (§7). If sending the ack
fails, signal-cli logs a warning and carries on; the server will deliver
the envelope again on a later connection and it is processed again.

### 4.2 Queue drained: `PUT /api/v1/queue/empty`

Sent once per connection, after the server has delivered everything that
was queued when the socket opened. It carries no meaningful body or
headers. **The client does not answer it**: libsignal emits a "queue
empty" event and drops the responder (P4). Envelopes that arrive later on
the same connection are live deliveries; no second queue-empty follows.

## 5. Client-to-server requests on the socket

### 5.1 Keep-alive: `GET /v1/keepalive`

Two independent mechanisms keep the socket alive.

1. **Application keep-alive** (signal-cli, P1 `SignalWebSocketHealthMonitor`).
   While at least one "keep-alive token" is registered on a socket
   (`receive` registers one for its whole run), signal-cli sends a request
   with verb `GET`, path `/v1/keepalive`, no headers, empty body, every
   **30 s**. The first goes out 30 s after the socket reaches the
   connected state. If no 2xx response has arrived **20 s** after a
   keep-alive was sent, the socket is torn down and treated as closed,
   which triggers the reconnect path of §6. A 4xx/5xx response counts as
   "no keep-alive received". Each keep-alive request has a 10 s request
   timeout inside libsignal (P2 `WebSocketConnection.DEFAULT_SEND_TIMEOUT`);
   a transport failure while sending one disconnects immediately. The
   server answers with status 200 and an empty body.
2. **WebSocket-level ping** (libsignal-net, P4 `timeouts.rs`,
   `infra/lib.rs`). If nothing has been sent or received for **31 s**,
   libsignal sends a WebSocket PING control frame. If nothing at all has
   arrived from the server for **45 s**, it closes the connection. libsignal
   also sends one `GET /v1/keepalive` of its own after a local network
   change, to check whether the connection still works. It does the same
   when a request has been outstanding for 5 s.

A Go client that sends the 30 s keep-alive requests and answers server
PINGs with PONGs will look the same on the wire. The PINGs are an
optimisation.

### 5.2 Other requests

Nearly every chat-server REST call signal-cli makes in 0.14.7 travels as a
`WebSocketRequestMessage` on one of the two sockets rather than as HTTPS.
The verb, path, JSON body and headers are the same as the REST form
(P1 `SignalDependencies` hands the sockets to the message, keys, profile,
account, certificate, link-device and other API objects). A few
operations use typed libsignal-net calls whose wire form is defined by
libsignal (P2 `runCatchingWithChatConnection` users), and attachment
upload/download still goes over plain HTTPS to the CDNs. The relevant
ones for receiving:

- **1:1 send** (receipts, retry receipts, null messages, profile-key
  updates): `PUT /v1/messages/{serviceId}?story=false` with a JSON body.
  It goes on the unauthenticated socket with the
  `Unidentified-Access-Key` header when sealed sender is possible;
  otherwise, or if that returns 401, it goes on the authenticated socket.
  See `send.md`.
- **Prekey count and refill**: see `keys.md`.

The response's `status`, `body` and `headers` are read the same way as
an HTTPS response. A response with status 4xx/5xx still counts as a
successful round trip (P2). If the connection dies before the response
arrives, the request fails with a socket error and is not retried
automatically by the socket layer.

A socket with no keep-alive token (for instance the unauthenticated
socket used only for sends) is closed 30 s after its last request (P1
`SignalDependencies`, disconnect timeout 30000 ms; P2 `SignalWebSocket`).

### 5.3 The unauthenticated socket

Yes, signal-cli uses it, but only for requests: sealed-sender sends,
access-key profile and prekey fetches, username lookups and similar. The
server never pushes envelopes on it. It is opened lazily when first
needed. It gets the same keep-alive machinery, but nothing registers a
token on it during `receive`, so no application keep-alives go out on it.
It closes 30 s after it was last used.

## 6. Reconnect and back-off

Behaviour of the receive loop (P1 `ReceiveHelper`):

- The loop waits for the next server request with a per-wait timeout (§7).
  If the wait fails because the socket is closed or unavailable, it
  sleeps **100 ms × 2^n** and reconnects. n starts at 0 and goes up by
  one after each consecutive failure, up to 9 at most, so the delays are
  0.1, 0.2, 0.4, … 25.6, 51.2, 51.2 s …. n resets to 0 after any
  successful read or any plain timeout.
- After a reconnect, the "caught up" flag is cleared, so receipts and other
  follow-up actions are queued again until the next queue-empty (§7).
- Other I/O errors end the receive call with an error. `receive` in
  daemon/continuous mode wraps that in an outer loop that logs "Receiving
  messages failed, retrying" and starts over immediately.
- A 403 on the upgrade request marks the account unregistered (§2). That
  is not retried.

## 7. `receive` end to end

Defaults: `receive` waits **3 s** (`-t/--timeout`, seconds, fractional
allowed, negative = no timeout). `--max-messages` defaults to unlimited.
The JSON-RPC `receive` method also defaults to 3 s. In continuous modes
(daemon, `jsonRpc`), each wait is 1 minute and the loop never ends on a
timeout (P1 `ReceiveCommand`, `ReceiveHelper`).

1. **Retry the cache.** signal-cli first re-processes every envelope
   still in its message cache from earlier runs. A retry that still fails
   with untrusted identity keeps the file until it is 14 days older than
   its server-delivered timestamp. Every other outcome deletes it
   (`capture.md` §3).
2. **Connect** the authenticated socket and register the keep-alive
   token. Keep-alives start (§5.1).
3. **Wait** for one server request with the timeout above. Envelopes are
   taken **one at a time** (batch size 1).
   - On `PUT /api/v1/message`: cache the envelope, ack it (§4.1), then
     handle it (step 4). If `--max-messages` was given, count one down and
     stop at 0.
   - On `PUT /api/v1/queue/empty`: run all queued actions (step 5), start
     a background cleanup of old prekeys (local only), mark the run as
     "caught up", then **keep waiting** for another full timeout. The
     queue-empty signal does **not** end `receive` by itself.
   - If the wait times out: with a timeout configured, `receive` ends.
     In continuous mode it just waits again.
   So a one-shot `receive` returns after a period equal to the timeout
   with nothing new arriving, counted after the last envelope or the
   queue-empty.
4. **Handle the envelope** (P1 `IncomingMessageHandler`):
   - SERVER_DELIVERY_RECEIPT envelopes are not decrypted. They only
     delete the matching send-log entry.
   - For everything else the destination service ID must be this
     account's ACI or PNI, or the envelope is rejected as invalid. A
     sealed-sender envelope addressed to the PNI is rejected. An envelope
     whose source is a PNI is rejected unless it is a delivery receipt.
     PNI-addressed envelopes are decrypted with the PNI identity, all
     others with the ACI identity.
   - Decryption by type: PREKEY_MESSAGE (3) and DOUBLE_RATCHET (1) use
     the 1:1 session, UNIDENTIFIED_SENDER (6) uses sealed sender, and
     PLAINTEXT_CONTENT (8) is not encrypted (its body is 0xC0 followed by
     the content and a 0x80 end marker, §7.2). Any other type is an
     error. After decryption the padding is stripped (`content.md`).
   - After a successful decryption the Content is checked. It is dropped
     silently, with no output, when: it is empty (no recognised
     sub-message); a DataMessage has no `timestamp` or one different from
     the envelope's client timestamp (field 5); its
     `requiredProtocolVersion` is above our current version; or one of
     the structural checks on quotes, mentions, body ranges or attachment
     pointers fails (P2 `EnvelopeContentValidator`). The envelope has
     already been acked, and its cache file is deleted.
   - Any PREKEY_MESSAGE envelope queues a "refresh prekeys if needed"
     action (`keys.md`). An invalid-key-ID failure queues one too.
5. **Queued actions.** Follow-up sends are collected as actions and
   deduplicated. Before the run is caught up, they run in one go when
   queue-empty arrives. After that, they run straight after each
   envelope. Whatever is left runs when `receive` ends. Delivery receipts
   for the same sender are **merged**: one ReceiptMessage carries all the
   timestamps collected for that sender and receipt type.

### 7.1 Receipts and profile keys

For each DataMessage (or EditMessage) received:

- **Delivery receipt**: sent only if the message came as sealed sender
  (UNIDENTIFIED_SENDER) over the normal pipe. libsignal-service calls
  this "needs receipt". Envelopes of type 1 or 3, and sealed-sender
  envelopes that nevertheless carry a source service ID, get no delivery
  receipt (P2 `SignalServiceCipher`). The receipt is a Content with a
  ReceiptMessage of type DELIVERY that lists the DataMessage timestamps.
  It is sent as a normal 1:1 message to every device of the sender, with
  content hint IMPLICIT, `online` false and `urgent` **false**. Its
  timestamp is the current time (P1 `SendReceiptAction`, P2
  `SignalServiceMessageSender.sendReceipt`). A PniSignatureMessage is
  attached when the recipient has been marked as needing one.
- **Instead, for non-sealed messages**: if this is the primary device and
  the sender is a known, unblocked contact with profile sharing enabled,
  signal-cli queues an account-attributes update and a **profile-key
  update** to the sender. That is a DataMessage with the
  PROFILE_KEY_UPDATE flag and our profile key (`content.md`), urgent false.
- **Read receipt**: only with `--send-read-receipts`. It is a
  ReceiptMessage of type READ, sent the same way as a delivery receipt.
- Typing, receipt, sync and null messages never trigger receipts.

### 7.2 Decryption failures: retry receipts and session renewal

**Untrusted identity**: no message goes out. A profile refresh for the
sender is queued and the envelope stays in the cache (`capture.md` §3).
The user sees the envelope with an error.

**Invalid message, no session, invalid key, invalid key ID** (libsignal's
"protocol" failures, which name the sender and device; for sealed sender
they also carry the inner unsealed content). If the sender is blocked,
nothing is sent. Otherwise, provided the sender and our destination are
known, signal-cli queues:

1. **Session renewal**: archive every session with that sender, under the
   identity (ACI or PNI) the envelope was addressed to, and then send the
   sender a **NullMessage** (`content.md`). The NullMessage goes as an
   ordinary encrypted 1:1 message: urgent false, current time as
   timestamp, content hint IMPLICIT. Because the sessions were archived,
   it opens a fresh session, so it is a PreKey message. Renewals are
   deduplicated per sender.
2. **Retry receipt**, unless the failing envelope came from our own
   device: a DecryptionErrorMessage, built as follows (P4 `protocol.rs`):
   - Protobuf fields: 1 `ratchet_key` bytes, 2 `timestamp` uint64,
     3 `device_id` uint32.
   - `ratchet_key` is the 33-byte serialized sender ratchet public key
     taken from the undecryptable SignalMessage, or from the SignalMessage
     inside the PreKeySignalMessage. It is absent for sender-key
     messages. A DecryptionErrorMessage cannot be made for plaintext
     content.
   - `timestamp` is the failing envelope's client timestamp (Envelope
     field 5).
   - `device_id` is the sender's device ID.
   - The original type comes from the unsealed content's message type
     when sealed sender was unwrapped. Otherwise it is mapped from the
     envelope type: 3 maps to PreKey, 6 to sender-key, 8 to plaintext,
     and anything else to Whisper.

   The serialized DecryptionErrorMessage goes into field 8
   (`decryptionErrorMessage`) of an otherwise empty Content. That Content
   is wrapped as **PlaintextContent**: one byte 0xC0, then the encoded
   Content, then a single 0x80 byte. There is no further padding. The
   PlaintextContent is sent unencrypted:
   - with sealed sender, inside an UnidentifiedSenderMessageContent
     with message type "plaintext", content hint IMPLICIT, and the group
     ID if the failure carried one; envelope type 6;
   - without sealed sender, as an envelope of type **PLAINTEXT_CONTENT
     (8)** whose `content` is the PlaintextContent bytes.

   It is a normal 1:1 send (`send.md`) with current time as timestamp,
   urgent false and online false (P1 `SendRetryMessageRequestAction`,
   `SendHelper.sendRetryReceipt`; P2 `sendRetryReceipt`,
   `EnvelopeContent.Plaintext`).

The failing envelope is shown to the user with the error, and its cache
file is deleted.

**Duplicate message** (already decrypted), **self-send** (sealed sender
from our own ACI and device) and other errors: no reply. Self-sends are
dropped silently. The others are shown with the error.

### 7.3 Receiving a DecryptionErrorMessage

When a peer sends us a retry receipt whose `device_id` is our device ID:

- signal-cli looks in its send log (unless `--disable-send-log` is set)
  for what it sent to that peer device at `timestamp`, and **resends**
  every matching entry.
- If the DecryptionErrorMessage has a ratchet key equal to our current
  session's ratchet key with that peer device, the session is renewed.
  With nothing to resend, renewal is the procedure of §7.2 item 1
  (archive, then a NullMessage). Otherwise only the archiving is done,
  and the resend itself starts the new session.
- Without a ratchet key (a sender-key failure), the matching group entries
  trigger a sender-key redistribution. That is out of scope for 1:1.

A Go implementation that sends only to single-device peers and keeps no
send log may skip resending. It should still answer with session renewal.
That last point is a recommendation, not observed behaviour.

## 8. Summary of what a compatible Go receiver must do on the wire

1. Open `wss://<chat host>/v1/websocket/` with Basic auth `ACI.deviceId`
   and `x-signal-receive-stories`.
2. For each server REQUEST `PUT /api/v1/message`: decode the body as an
   Envelope, persist it, then reply RESPONSE {id echoed, 200, "OK"}.
3. Treat `PUT /api/v1/queue/empty` as "drained". Do not reply.
4. Send `GET /v1/keepalive` every 30 s. Reconnect if no 2xx arrives within
   20 s. Answer WebSocket PINGs.
5. On disconnect, back off 100 ms × 2^n (n up to 9) and reconnect.
6. After decryption, send merged delivery receipts for sealed-sender data
   messages. On protocol failures, send a NullMessage after archiving the
   sessions, then a retry receipt (PlaintextContent with
   DecryptionErrorMessage).

## Gaps

- Exactly when libsignal-net routes a typed request over gRPC instead of
  the WebSocket is not determined; it is left to `transport.md`.
- Whether the libsignal JNI bridge overrides the client request-ID start
  value of 0 was not checked. It makes no difference to interoperability.
- The server's own behaviour (its keep-alive response body, and how long
  it waits for an ack before redelivering) comes from client sources
  only; Signal-Server was not read.
