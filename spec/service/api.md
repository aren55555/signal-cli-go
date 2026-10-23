# Chat-service API conventions for signal-cli 0.14.7 (staging)

Spec role: prose only; no code. This file is the common ground for
`registration.md`, `keys.md`, `send.md` and `profile.md`: which transport
each request in the milestone-4 scope uses, how it is authenticated, how
JSON bodies are encoded, how status codes are interpreted, and how rate
limits are handled.

Related specs, not repeated here:

- `spec/service/transport.md`: hosts, TLS, the libsignal-net WebSocket
  (HTTP/2 extended CONNECT), frame layout, the `Authorization` and
  `User-Agent` / `X-Signal-Agent` values, proxies.
- `spec/service/pipe.md`: the authenticated pipe and `receive`.
- `spec/service/content.md`: the Content protobufs and plaintext padding.
- `spec/protocol/sealed-sender.md`: sender certificates and the sealed
  envelope format.

## Sources

| Ref | What | URL @ version |
|-----|------|---------------|
| A1 | signal-cli | https://github.com/AsamK/signal-cli @ v0.14.7 (b01b6b370dc063599a1a2b9fde0f5ff4e2d78fe8): `lib/src/main/java/org/asamk/signal/manager/internal/{SignalDependencies,RegistrationManagerImpl,ManagerImpl}.java`, `.../manager/helper/{AccountHelper,PreKeyHelper,SendHelper,UnidentifiedAccessHelper,ProfileHelper,RecipientHelper}.java`, `.../manager/util/{Utils,KeyUtils,NumberVerificationUtils}.java`, `.../manager/storage/SignalAccount.java`, `.../manager/api/RateLimitException.java`, `.../manager/SignalAccountFiles.java`, `src/main/java/org/asamk/signal/commands/RegisterCommand.java` |
| A2 | signal-service-java (Turasa fork) | https://github.com/Turasa/libsignal-service-java @ v2.15.3_unofficial_151 (66956cb4a0394d50cee3590209baf58312084a44): `lib/libsignal-service/src/main/java/org/whispersystems/signalservice/internal/push/PushServiceSocket.java`, `.../api/NetworkResultUtil.kt`, `.../api/crypto/{SealedSenderAccess.kt,UnidentifiedAccess.java}`, `.../api/websocket/SignalWebSocket.kt`, `.../api/account/{AccountApi.kt,AccountAttributes.kt}`; `core/network/src/main/java/org/signal/network/{NetworkResult.kt,util/JsonUtil.java,websocket/WebSocketRequestExt.kt}`; `core/util-jvm/src/main/java/org/signal/core/util/Base64.kt`; `lib/network/src/main/java/org/signal/network/api/CertificateApi.kt` |
| A3 | Signal-Server | https://github.com/signalapp/Signal-Server @ v20261022.0.0 (8edf3488f7d97398db7ffa5184c6b769a272ff59): `service/src/main/java/org/whispersystems/textsecuregcm/auth/UnidentifiedAccessUtil.java`, `.../entities/AccountAttributes.java` (server-side acceptance rules only; staging may run a different build) |
| A4 | Reference distribution, offline | `/work/third_party/signal-cli-0.14.7/lib/*.jar` loaded in `jshell` with no network: the bundled `JsonUtil` serializer was applied to hand-built request objects to confirm key order, null handling and base64 flavour (section 3), and `UnidentifiedAccess.deriveAccessKeyFrom` was run on an all-zero profile key (section 5.1). No account, no server. |

## 1. Two transports, and which request uses which

signal-cli 0.14.7 talks to the chat service in two ways (details in
`transport.md` §2):

- **Chat WebSocket (libsignal-net).** Two connections: an *authenticated*
  one and an *unauthenticated* ("unidentified") one. Requests are framed as
  WebSocket request messages with a verb, a path, a header list and a body.
- **REST over HTTPS (OkHttp)** to `https://chat.staging.signal.org`.

For the milestone-4 flows:

| Request | Transport | Credential |
|---------|-----------|------------|
| `POST /v1/verification/session` | REST | Basic, E.164 form (§2) |
| `GET /v1/verification/session/{id}` | REST | Basic, E.164 form |
| `PATCH /v1/verification/session/{id}` (captcha) | REST | Basic, E.164 form |
| `POST /v1/verification/session/{id}/code` | REST | Basic, E.164 form |
| `PUT /v1/verification/session/{id}/code` | REST | Basic, E.164 form |
| `POST /v1/registration` | REST | Basic, E.164 form |
| `PUT /v1/accounts/attributes` | authenticated WebSocket | ACI form |
| `GET /v1/accounts/whoami` | authenticated WebSocket | ACI form |
| `GET /v2/keys?identity=…`, `PUT /v2/keys?identity=…` | authenticated WebSocket | ACI form |
| `GET /v2/keys/{serviceId}/{device}` | authenticated WebSocket, or unauthenticated WebSocket with an access header (`send.md` §4) | either |
| `GET /v1/certificate/delivery[?includeE164=false]` | authenticated WebSocket | ACI form |
| `GET /v1/profile/…`, `PUT /v1/profile` | authenticated WebSocket (profile GET may go unauthenticated with an access header) | either |
| `PUT /v1/messages/{destination}?story=false` | unauthenticated WebSocket with an access header (sealed) or authenticated WebSocket (unsealed); **REST fallback** when the socket fails for a technical reason (`send.md` §6) | either |
| CDSI lookup (phone number to ACI/PNI) | libsignal-net CDSI (separate enclave connection, not in this spec) | auth from the chat service |

There is no REST path for prekeys, profiles, certificates or attributes in
signal-cli's configuration: those classes are built only on the WebSocket
(A1 `SignalDependencies`).

## 2. Authentication

### 2.1 Account password

- Generated once when the local account is created (before
  `register`): **18 bytes from a CSPRNG, encoded with standard base64 with
  padding**. Because 18 is a multiple of 3, the result is always 24
  characters with no `=` (A1 `KeyUtils.createPassword`,
  `SignalAccount.create`).
- The same password is sent to the server at registration (as the Basic
  credential, which the server stores) and used for every later request.
- Re-running `register` for the same number reuses the stored password.

### 2.2 Basic credential forms

The full header formats and per-transport differences are in
`transport.md` §4.1. Summary for this scope:

| Phase | Username part | Where |
|-------|---------------|-------|
| Before `/v1/registration` succeeds | the E.164 number, e.g. `+15555550100` (no device suffix, because the device ID is 1) | all REST registration calls |
| After registration, REST | the ACI string; a `.deviceId` suffix only when the device is not 1 | REST fallback sends |
| After registration, WebSocket | `aci.deviceId`, i.e. `….1` for the primary | `transport.md` §4.1 |

The REST `Authorization` value is `Basic ` followed by standard padded
base64 of `username:password` in UTF-8 (A2 `PushServiceSocket`).

On REST, a request carries **either** `Authorization` **or** a sealed
sender access header, never both: when an access header is supplied, the
Basic header is not added (A2 `PushServiceSocket.buildServiceRequest`).

### 2.3 Other headers

- REST: `User-Agent` and `X-Signal-Agent` (values in `transport.md` §4.2),
  plus `Content-Type: application/json; charset=utf-8` on requests with a
  JSON body (OkHttp appends the charset to the `application/json` media
  type that signal-service-java passes; confirmed offline, A4).
- WebSocket requests built by signal-service-java carry
  `content-type:application/json` as their first header whenever they have
  a JSON body (PUT always, POST when a body is present; GET and DELETE
  never). libsignal re-emits headers as `lower-name: value`
  (`transport.md` §3.5).
- `Accept-Language` appears on two requests in scope:
  - `POST …/code`: `<language>-<COUNTRY>` from the JVM default locale, for
    example `en-US`. If the default locale does not form a valid language
    range (for instance an empty country), signal-cli falls back to
    `en-US` (A1 `Utils.getDefaultLocale`).
  - profile GET: a weighted list. For an English locale it is exactly
    `en;q=1`. For another language with a country it is
    `xx-YY;q=1,xx;q=0.75,en;q=0.5`, and with no country `xx;q=1,en;q=0.5`
    (A2 `AcceptLanguagesUtil`).

## 3. JSON encoding conventions

All bodies are produced by one Jackson `ObjectMapper` (A2 `JsonUtil`) with
default settings plus the Kotlin module. Consequences, confirmed offline
(A4):

- **Key order** is the declaration order of the body's fields. The server
  does not care, but matching it costs nothing.
- **Null handling varies by body.** Some bodies omit null members, others
  emit `null` explicitly. Each body's rule is given where it is specified.
  In short: the verification-session bodies and the registration envelope
  omit nulls; `accountAttributes`, the prekey upload body and the profile
  write body emit them.
- **Base64 flavours.** These differ per field, so each field's flavour is
  stated where it is defined. The general rule:
  - Key material (identity keys, prekey public keys, signatures, Kyber
    keys) uses **standard base64 without padding**.
  - Raw byte-array members handled by Jackson's default (for example
    `unidentifiedAccessKey`, the profile write fields) use **standard
    base64 with padding**.
  - Message `content` uses **standard base64 with padding**.
- Integers are bare JSON numbers. A message timestamp is a 64-bit
  millisecond count.
- **Decoding** is lenient on the client: signal-service-java's decoder
  accepts padded or unpadded input, and switches to the URL-safe alphabet
  if the input contains `-` or `_`. Unknown response members are ignored.

## 4. Status codes and how signal-cli reacts

### 4.1 REST (registration calls, REST send fallback)

Each REST call first runs a call-specific handler (registration handlers
in `registration.md` §6). The generic mapping then applies (A2
`PushServiceSocket.validateServiceResponse`):

| Status | Meaning to signal-cli |
|--------|----------------------|
| 200, 202, 204, 207 | success |
| 413, 429 | rate limited. `Retry-After` is parsed as **integer seconds**, if present and numeric, and carried as a delay. No automatic retry. |
| 401, 403 | authorization failed |
| 404 | not found (on a send: unregistered recipient) |
| 409 | mismatched devices; body parsed as `{missingDevices, extraDevices}` |
| 410 | stale devices; body `{staleDevices}` |
| 411 | device limit; body parsed |
| 417 | expectation failed |
| 423 | registration lock; body `{length, timeRemaining, backupCredentials, svr2Credentials, svr3Credentials}` |
| 428 | proof required; body `{token, options}`, `Retry-After` integer seconds |
| 499 | deprecated client version |
| 508 | server rejected |
| anything else | generic failure with the status code |

### 4.2 WebSocket responses

A WebSocket response with status outside 200..299 becomes a status-code
error that carries the status, body and headers. How it is then
interpreted depends on the call:

- **Basic calls** (account attributes, whoami, sender certificate, prekey
  counts): 401 and 403 mean authorization failed; 413 and 429 mean rate
  limited, with `retry-after` header seconds converted to a delay; any
  other status is a generic failure (A2 `NetworkResultUtil.toBasicLegacy`).
- **Prekey upload and fetch** (A2 `toPreKeysLegacy`):
  - 400 and 401 mean authorization failed.
  - 404 means not found. On a bundle fetch it is first turned into
    "unregistered user".
  - 429 means rate limited, **without** reading `Retry-After`.
  - 508 means server rejected.
- **Message send**: see `send.md` §5.

### 4.3 What happens on authorization failure at start-up

Every signal-cli command that loads a registered account first runs an
account check. The check does `PUT /v1/accounts/attributes`, then the
prekey refresh (`keys.md` §3), then `whoami` if the PNI is unknown. If any
of these fails with 401 or 403, signal-cli marks the account
**unregistered** locally and aborts. A 499 aborts with "signal-cli version
is too old" (A1 `AccountHelper.checkAccountState`, `SignalAccountFiles`).

## 5. Unidentified access (sealed sender) headers

### 5.1 Access key derivation

The 16-byte unidentified access key (UAK) for a profile key is computed
as follows (A2 `UnidentifiedAccess.deriveAccessKeyFrom`):

1. Encrypt 16 zero bytes with AES-256-GCM, using the 32-byte profile key
   as the key, a 12-byte all-zero nonce, a 128-bit tag and no associated
   data.
2. Take the first 16 bytes of the ciphertext and drop the tag. This is
   equivalent to AES-256 encryption of the GCM counter block
   `00…00 00000002` under the profile key.

Test value (A4, computed by running the reference jar offline): with an
all-zero 32-byte profile key, the UAK is
`cea7403d4d606b6e074ec5d3baf39d18` (hex).

Other values:

- The **unrestricted** key is 16 zero bytes. signal-cli uses it for a
  recipient whose profile says `unrestrictedUnidentifiedAccess: true`
  (`send.md` §3).
- The **profile verifier** (`unidentifiedAccess` in a profile response) is
  HMAC-SHA256, keyed with the UAK, over 32 zero bytes. A recipient's mode
  is "enabled" only if this verifier matches the UAK computed from the
  profile key we hold for them (A1 `ProfileUtils`, A2 `ProfileCipher`).
- The account's **own** UAK is sent as `unidentifiedAccessKey` in account
  attributes (`registration.md` §4.2), standard base64 with padding
  (24 characters).

### 5.2 Header forms

On the unauthenticated WebSocket, signal-service-java appends exactly one
header to the request's header list (A2 `SealedSenderAccess`,
`SignalWebSocket.UnauthenticatedWebSocket`):

| Access kind | Header as signal-service-java writes it | Value |
|-------------|-----------------------------------------|-------|
| individual access key | `Unidentified-Access-Key:<value>` | standard base64 **with** padding of the 16-byte UAK |
| group send token (not used for 1:1 in signal-cli) | `Group-Send-Token:<value>` | standard base64 with padding of the serialized token |

libsignal lowercases the name on the wire (`transport.md` §3.5). On REST
(send fallback only) the same name and value are sent as an ordinary HTTP
header, in place of `Authorization`.

If an unauthenticated request returns **401**, the socket layer retries
with the access's fallback. For a plain access-key access created by
signal-cli for a 1:1 send there is no fallback (no group token supplier),
so the 401 propagates to the caller (`send.md` §5).

Group send endorsements and tokens are out of scope: signal-cli uses them
only for groups.

### 5.3 Sender certificate

- `GET /v1/certificate/delivery` returns a certificate that includes the
  sender's E.164. `GET /v1/certificate/delivery?includeE164=false` returns
  one without it. Both go over the authenticated WebSocket. The response
  is JSON `{"certificate": "<base64>"}` (decoded leniently), carrying the
  serialized `SenderCertificate` from `spec/protocol/sealed-sender.md`
  (A2 `CertificateApi`).
- signal-cli uses the **E.164-less** certificate unless the account's
  phone-number sharing mode is explicitly "everybody", or is "contacts" and
  the recipient is a contact. On a fresh account the mode is unset, so the
  `includeE164=false` certificate is used (A1
  `UnidentifiedAccessHelper.getSenderCertificateFor`, `ConfigurationStore`).
- Certificates are cached in memory per process. A certificate is
  refetched when its expiration is less than one day away. A fetch failure
  is logged ("Failed to get sender certificate …, ignoring") and the send
  silently becomes unsealed.

## 6. Rate limits and back-off

signal-cli never retries a rate-limited request automatically. It surfaces
the error, with the delay when known, and leaves retrying to the user or
caller:

- **REST 413 or 429**: delay = `Retry-After` integer seconds × 1000 ms,
  if present.
- **WebSocket basic calls and sends, 413 or 429**: same, from the
  `retry-after` response header.
- **Prekey calls, 429**: no delay is parsed.
- **Registration**:
  - When the session metadata says the next SMS or call is `n > 0`
    seconds away, `register` fails locally with a rate-limit error of
    `n × 1000` ms **without** sending the code request.
  - If `nextVerificationAttempt > 0`, it fails with "captcha required"
    carrying that delay.
  - 429 on the code request or the code submission is surfaced as a
    failure. The response carries session metadata, and `Retry-After` is
    recorded, but nothing is retried (`registration.md` §6).
- **428 proof required** on a send: reported as a per-recipient failure
  with the token and options. The user must solve a challenge (out of
  scope) before retrying.
- **Send conflicts**: 409 and 410 are not rate limits. They are retried
  immediately, up to 4 attempts in total (`send.md` §5).

A polite Go client should do at least the same: never loop on 413, 428 or
429, and honour `Retry-After` when present.

## Gaps

- Staging may run a different Signal-Server build from A3. Server-side
  facts quoted from A3 are indicative only.
