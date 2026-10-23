# Sending a 1:1 message (signal-cli 0.14.7, staging)

Spec role: prose only; no code. This file covers what signal-cli 0.14.7
puts on the wire for `send -m TEXT RECIPIENT` to one recipient: the
choices made beforehand (sealed or not, which devices), the
`PUT /v1/messages/{destination}` body, and how each response is handled.

Not repeated here:

- the Content/DataMessage plaintext and its padding (`content.md`);
- encryption (`spec/protocol/pqxdh.md`, `double-ratchet.md`,
  `sealed-sender.md`);
- transport details (`transport.md`);
- general conventions (`api.md`);
- bundle fetching (`keys.md` §4);
- profile fetching (`profile.md`).

## Sources

| Ref | What | URL @ version |
|-----|------|---------------|
| S1 | signal-cli | https://github.com/AsamK/signal-cli @ v0.14.7 (b01b6b370dc063599a1a2b9fde0f5ff4e2d78fe8): `lib/src/main/java/org/asamk/signal/manager/helper/{SendHelper,UnidentifiedAccessHelper,ProfileHelper,RecipientHelper}.java`, `.../manager/internal/{ManagerImpl,SignalDependencies}.java`, `.../manager/util/ProfileUtils.java`, `.../manager/storage/configuration/ConfigurationStore.java`, `src/main/java/org/asamk/signal/commands/SendCommand.java` |
| S2 | signal-service-java (Turasa fork) | https://github.com/Turasa/libsignal-service-java @ v2.15.3_unofficial_151 (66956cb4a0394d50cee3590209baf58312084a44): `lib/libsignal-service/src/main/java/org/whispersystems/signalservice/api/SignalServiceMessageSender.java`, `.../api/message/MessageApi.kt`, `.../api/NetworkResultUtil.kt`, `.../api/crypto/{SignalServiceCipher,EnvelopeContent,SealedSenderAccess.kt,UnidentifiedAccess}.java`, `.../api/websocket/SignalWebSocket.kt`, `.../internal/websocket/{LibSignalChatConnection,LibSignalResponseExtension}.kt`, `.../internal/push/{OutgoingPushMessageList,OutgoingPushMessage,SendMessageResponse,MismatchedDevices,StaleDevices,ProofRequiredResponse,PushServiceSocket}.java`, `lib/libsignal-service/src/main/protowire/SignalService.proto` (Envelope.Type numbers) |
| S3 | Signal-Server | https://github.com/signalapp/Signal-Server @ v20261022.0.0 (8edf3488f7d97398db7ffa5184c6b769a272ff59): `service/src/main/java/org/whispersystems/textsecuregcm/auth/UnidentifiedAccessUtil.java` (access-key check), used only to reason about §7 |
| S4 | libsignal | https://github.com/signalapp/libsignal @ v0.99.1 (97801d22dcf9f5bf714f7b8fa3212cdc973ae1c8): `rust/net/src/cdsi.rs` (a CDSI result may lack the ACI), used only in §7 |
| S5 | Reference distribution, offline | `jshell` over `/work/third_party/signal-cli-0.14.7/lib/*.jar`, no network: the bundled serializer applied to a hand-built message list (§2 JSON shape) |

## 1. Sequence for one `send`

These are the network requests of a fresh process (each CLI invocation is
a new process), in order. Steps marked *cached* are skipped when the cache
is warm.

1. **Account check** (`api.md` §4.3): `PUT /v1/accounts/attributes`, then
   the prekey count and refill requests for ACI and PNI (`keys.md` §3).
2. **Recipient resolution.**
   - A UUID recipient is used directly as an ACI.
   - A phone number is looked up in the local recipient store. If unknown,
     signal-cli does a CDSI lookup through libsignal-net (not specified
     here). An unknown number with no registered user gives an
     "unregistered" result without any message request.
   - A Go client can avoid CDSI in milestone 4 by sending to an ACI.
3. **Local contact bookkeeping.** The recipient is stored as a contact
   with profile sharing enabled. The DataMessage gets the account's
   profile key unless the contact is blocked, plus the contact's
   disappearing-message timer and timer version (`content.md` §4.4).
4. **Timestamp.** The current time in milliseconds. If it equals the
   previous message timestamp of this process, signal-cli sleeps 1 ms and
   reads the clock again, so timestamps are unique per process. The same
   value is used for the DataMessage `timestamp` and the request body's
   `timestamp`.
5. **Sealed-sender decision** (§3). This may trigger, in order:
   - *cached* `GET /v1/profile/{recipientAci}` (recipient profile, 6-hour
     cache);
   - *cached* `GET /v1/profile/{selfAci}/{version}` (own profile, 6-hour
     cache);
   - *cached* `GET /v1/certificate/delivery?includeE164=false` (in-memory
     cache, so in practice once per process).
6. For each target device without a session: **bundle fetch**
   `GET /v2/keys/{aci}/*` (`keys.md` §4), then session setup.
7. **Encrypt** for each target device (§4) and
   **`PUT /v1/messages/{aci}?story=false`** (§2).
8. Handle the response (§5), possibly looping back to step 6 or 7.
9. Sync transcript to own linked devices: only if the response's
   `needsSync` is true or the account is multi-device. A fresh
   single-device account sends none.

## 2. `PUT /v1/messages/{destination}?story=false`

- **Path.** `destination` is the recipient's service-ID string: a bare
  UUID for an ACI, `PNI:`+UUID for a PNI. The query is always
  `story=false` for a 1:1 message.
- **Transport.**
  - Sealed: the unauthenticated WebSocket with the
    `Unidentified-Access-Key` header (`api.md` §5.2).
  - Unsealed: the authenticated WebSocket.
  - Either way the request also carries `content-type:application/json`.

### 2.1 Body

Key order exactly as below; no member is ever null (S5):

    {"destination":"<serviceId>","timestamp":1700000000000,"messages":[{"type":3,"destinationDeviceId":1,"destinationRegistrationId":4321,"content":"<base64>"}],"online":false,"urgent":true}

| Member | Value |
|--------|-------|
| `destination` | same string as in the path |
| `timestamp` | the message timestamp (ms), as a JSON number |
| `messages` | one entry per target device (§4.1), in the iteration order of a hash set of device IDs. For small integers this is ascending order in practice. |
| `online` | `false` for data messages. (Typing indicators would use true; out of scope.) |
| `urgent` | `true` for a normal `send`; `false` with `--no-urgent`. Receipts and profile-key updates send `false`. |

Per-device entry:

| Member | Value |
|--------|-------|
| `type` | Envelope type number (§4.2) |
| `destinationDeviceId` | device ID |
| `destinationRegistrationId` | the remote registration ID stored in our session with that device |
| `content` | the ciphertext, standard base64 **with** padding |

### 2.2 Success response

200 with a JSON body. The only member read is `needsSync` (boolean).
signal-cli records the result as "sent unidentified" when the response
arrived on the unauthenticated connection (S2 `LibSignalChatConnection`),
or, on the REST fallback, when an access header was used.

## 3. Choosing sealed or unsealed

For a 1:1 send, signal-cli computes an "access" for the recipient once
per send attempt (S1 `UnidentifiedAccessHelper.getAccessFor`). The send is
**sealed** only if all three of the following produce a value; otherwise
it is **unsealed**:

1. **Recipient key.** This uses the recipient's stored profile, which is
   fetched first if it is missing or more than 6 hours old (`profile.md`
   §3). The key depends on the profile's access mode:
   - *unrestricted*: the profile said `unrestrictedUnidentifiedAccess:
     true`. The key is 16 zero bytes.
   - *enabled*: a profile key is known for the recipient, and the
     profile's `unidentifiedAccess` verifier matches the UAK derived from
     it. The key is that UAK.
   - *disabled*, *unknown*, or no profile: no key, so the send is
     unsealed.
2. **Own key.** This is never absent:
   - if the account's own stored profile is in unrestricted mode, 16 zero
     bytes;
   - otherwise the UAK of the account's own profile key.

   It is only a gate in signal-cli. The value is not sent on a 1:1 send.
3. **Sender certificate.** This is the E.164-less certificate on a fresh
   account (`api.md` §5.3). If the fetch fails, the send is unsealed.

The resulting access uses header `Unidentified-Access-Key` with the
recipient key. It has **no fallback** (signal-cli supplies no group send
token for 1:1).

### 3.1 What a sealed send changes

- The bundle fetch for a device without a session goes over the
  unauthenticated WebSocket with the same header.
- Each device's ciphertext is wrapped in sealed sender v1 with content
  hint RESENDABLE (1) and no group ID, using the sender certificate
  (`spec/protocol/sealed-sender.md`). The entry `type` becomes 6.
- `PUT /v1/messages` goes over the unauthenticated WebSocket.

### 3.2 Self-sends

When the recipient is the account itself and the send is unsealed, the
local device is removed from the target devices. If that leaves none, the
local device is kept. Out of scope otherwise.

### 3.3 Falling back to unsealed

There are three distinct fallbacks (S2):

- **401 on the sealed `PUT /v1/messages`.** The request is immediately
  repeated **unchanged** (same body, same type-6 ciphertexts) on the
  authenticated WebSocket. Success is then recorded as *not*
  unidentified.
- **401 on the sealed bundle fetch.** This surfaces as authorization
  failed. The send loop replaces the access with its fallback, which is
  none, and the next attempt is fully unsealed: authenticated fetch,
  type 1 or 3 ciphertexts.
- **401 on a sealed send that reaches the send loop** (the REST fallback
  path), or an invalid-key error while encrypting: the same switch to
  unsealed for the next attempt.

### 3.4 Sticky downgrade after an unsealed success

After any 1:1 send result (data message, receipt, profile-key update) that
succeeded **without** being unidentified, signal-cli checks the
recipient's stored profile. If its access mode is enabled or unrestricted,
it rewrites the mode to *unknown*. The same happens after an
"unregistered" result. Until the profile is refetched (6-hour cache),
every later send to that recipient is unsealed (S1
`SendHelper.handleSendMessageResult`).

## 4. Per-device encryption

### 4.1 Target devices

The set is device 1 plus every other device ID for which a session with
the recipient exists. If there is no session with device 1, the bundle
fetch with `*` creates sessions for all devices the server returns. Those
devices only enter the target set on the next attempt, after a 409 or on
the next send. The set is built per attempt.

### 4.2 Entry type numbers

| Case | `type` | `content` |
|------|--------|-----------|
| unsealed, session has an unacknowledged PreKey state | 3 (`PREKEY_BUNDLE`) | serialized PreKeySignalMessage |
| unsealed, established session | 1 (`CIPHERTEXT`) | serialized SignalMessage |
| sealed (either of the above inside) | 6 (`UNIDENTIFIED_SENDER`) | sealed-sender v1 message |
| plaintext content (retry receipts only, out of scope) | 8 (`PLAINTEXT_CONTENT`) | |

The plaintext is the padded Content (`content.md` §2). The registration
ID is read from the session after encryption.

## 5. Responses and reactions

The send loop makes at most **4 attempts**. Each attempt re-reads
sessions, rebuilds the device set and re-encrypts. Status mapping for the
WebSocket send (S2 `NetworkResultUtil.toMessageSendLegacy`):

| Status | Body | Reaction |
|--------|------|----------|
| 200 | `{"needsSync":bool}` | success |
| 401 | — | sealed: §3.3. Unsealed: failure "authorization failed", no retry. |
| 404 | — | "unregistered user". signal-cli re-resolves the recipient through CDSI. If CDSI still finds an account, the whole send is retried **once** (with a freshly computed access). Otherwise the result is "unregistered". |
| 409 | `{"missingDevices":[int…],"extraDevices":[int…]}` | (a) archive our sessions with every extra device; (b) clear sender-key sharing state for extra and missing devices; (c) for each missing device, fetch its bundle **by number** on the authenticated WebSocket and build a session (an untrusted identity aborts with an identity failure; no valid bundle gives an "invalid prekey" failure); (d) next attempt. Special case: sending only to our own device and the server reports no missing devices: archive the extras, mark the account single-device and report success with no devices. |
| 410 | `{"staleDevices":[int…]}` | archive sessions with those devices, clear sender-key sharing state, next attempt. The next attempt finds no session for them, so for device 1 it refetches with `*`. A stale non-primary device simply drops out until a 409 lists it as missing. |
| 413, 429 | — | rate-limit failure with `retry-after` seconds; no retry |
| 428 | `{"token":…, "options":[…]}` | "proof required" failure, with `retry-after` seconds if any; no retry |
| 508 | — | "server rejected"; aborts the send with an error |
| other | — | generic failure |

When all 4 attempts end in conflicts, the send fails with "Failed to
resolve conflicts after 4 attempts!".

Session-level retries inside the loop:

- If a session becomes invalid mid-send, the first time it simply
  retries. The second time it archives the sessions for device 1 and all
  sub-devices, then retries.
- "No session" mid-send is retried once.

Per-recipient result kinds reported to the user: success (with
"unidentified" true or false), unregistered, identity failure (untrusted
identity key), network failure, rate-limit failure, proof-required
failure, invalid-prekey failure.

## 6. REST fallback

If the WebSocket send fails for a **technical** reason (socket
unavailable, or an I/O error other than the HTTP-level outcomes above),
signal-cli retries the same attempt over REST, because its "use REST
fallback" switch is always on (S1 `SignalDependencies`):

- `PUT https://chat.staging.signal.org/v1/messages/{destination}?story=false`
- the same JSON body;
- with `Unidentified-Access-Key: <b64>` if sealed, else
  `Authorization: Basic …` (`api.md` §2.2);
- status mapping as in `api.md` §4.1, with 404 turned into "unregistered
  user".

Non-technical outcomes (401, 404, 409, 410, 413, 428, 429, 508) are
**not** retried over REST.

## 7. Why a capture showed an unsealed first send

The observation: signal-cli 0.14.7 sent **unsealed** to a fresh recipient
whose account had unrestricted unidentified access enabled.

Reading the source, the straight path in §3 *would* produce a sealed send
in that situation. The recipient profile would say unrestricted, the key
would be 16 zero bytes, the own key is always present, and the server
accepts any key for an unrestricted account (S3). So one of the
conditions or fallbacks above must have fired. These are the candidates
the source allows, roughly from most to least likely:

1. **Stale cached profile.** signal-cli registers new accounts with
   `unrestrictedUnidentifiedAccess: false`. If the sender fetched the
   recipient's profile before the flag was turned on, the cached mode is
   *disabled* for up to 6 hours. The sender has no profile key for a
   fresh recipient, so *enabled* is impossible. The result is unsealed.
2. **Sticky downgrade** (§3.4). Any earlier unsealed success to that
   recipient (for example a delivery receipt sent while receiving, or an
   earlier message) rewrites the mode to *unknown* for up to 6 hours.
3. **Profile fetch failure.** On any error the stored mode becomes
   *unknown* (`profile.md` §3), and the send is unsealed.
4. **Sender-certificate fetch failure.** This is logged as "Failed to get
   sender certificate …, ignoring".
5. **401 on the sealed bundle fetch or the sealed PUT** (§3.3). For the
   PUT the ciphertext stays type 6 but travels on the authenticated
   socket, which a capture of the connection would show as an unsealed
   send.
6. **Recipient known only by PNI.** A CDSI result may omit the ACI (S4).
   The send then targets `PNI:…`, and a PNI profile does not yield an
   enabled or unrestricted mode in practice. Not verified against the
   server.

How to tell which one fired, from black-box observation:

- Run with `-vv` (trace logging) and look for:
  - "Unidentified access not available for …";
  - "Unidentified access not available due to missing sender
    certificate";
  - "Failed to retrieve profile for …";
  - "Got an AuthorizationFailedException when trying to send using sealed
    sender".
- Check whether a `GET /v1/profile/{aci}` preceded the send.
- Check which connection carried the `PUT`.

For byte compatibility, a Go client should reproduce the decision rules
in §3 and §3.4, including the 6-hour cache and the downgrade, rather than
always sealing.

## Gaps

- The exact cause in the specific capture cannot be settled from source
  alone (§7).
- CDSI (number to ACI/PNI) is out of scope here. It runs over
  libsignal-net's enclave connection to `cdsi.staging.signal.org`, using
  credentials from the chat service.
