# Transport: how signal-cli 0.14.7 reaches the chat service (staging)

Spec role: prose only; no code. This file says which network transport
carries which request, which hosts, ports, TLS roots and HTTP versions
are used for Signal **staging**, how credentials and agent headers ride
on each transport, and how proxies are honoured. It covers what signal-cli
0.14.7 actually does with its built-in staging configuration; unused code
paths are named only so you know to skip them.

Related specs, not repeated here:

- `spec/service/pipe.md`: the `WebSocketMessage` framing (field numbers),
  server-pushed requests, keep-alive cadence, the receive loop and its
  reconnect back-off. This file only adds the layers underneath and a few
  framing details pipe.md leaves open.
- `spec/service/api.md`, `registration.md`, `keys.md`, `send.md`: request
  bodies, status codes and per-endpoint behaviour.

## Sources

| Ref | What | URL @ version |
|-----|------|---------------|
| T1 | signal-cli | https://github.com/AsamK/signal-cli @ v0.14.7 (b01b6b370dc063599a1a2b9fde0f5ff4e2d78fe8): `lib/src/main/java/org/asamk/signal/manager/config/{StagingConfig,LiveConfig,ServiceConfig,ServiceEnvironmentConfig,WhisperTrustStore}.java`, `lib/src/main/resources/org/asamk/signal/manager/config/whisper.store`, `.../manager/internal/{SignalDependencies,SignalWebSocketHealthMonitor,RegistrationManagerImpl}.java`, `.../manager/helper/ReceiveHelper.java`, `.../manager/util/Utils.java` (`getHttpsProxy`), `src/main/java/org/asamk/signal/{BaseConfig,App}.java`, `build.gradle.kts` (manifest attributes) |
| T2 | signal-service-java (Turasa fork) | https://github.com/Turasa/libsignal-service-java @ v2.15.3_unofficial_151 (66956cb4a0394d50cee3590209baf58312084a44): `lib/libsignal-service/src/main/java/org/whispersystems/signalservice/internal/websocket/{LibSignalChatConnection,LibSignalNetworkExtensions,WebSocketConnection}.kt`, `.../api/websocket/SignalWebSocket.kt`, `.../internal/push/PushServiceSocket.java`, `.../internal/util/BlacklistingTrustManager.java`, `.../api/SignalServiceAccountManager.java`, `.../api/SignalServiceMessageSender.java`, `.../api/registration/RegistrationApi.kt`, `.../api/message/MessageApi.kt`, `.../api/keys/KeysApi.kt`, `.../api/util/CredentialsProvider.java`, `.../api/crypto/SealedSenderAccess.kt`, `.../api/svr/{SecureValueRecoveryV2,Svr2Socket}.kt`; `core/network/src/main/java/org/signal/network/websocket/WebSocketRequestExt.kt`, `core/network/src/main/java/org/signal/network/config/{SignalServiceConfiguration.kt,SignalUrl.java,SignalServiceUrl.java}`, `core/network/src/main/java/org/signal/network/util/Tls12SocketFactory.java` |
| T3 | libsignal | https://github.com/signalapp/libsignal @ v0.99.1 (97801d22dcf9f5bf714f7b8fa3212cdc973ae1c8): `rust/net/src/{env.rs,chat.rs,auth.rs,certs.rs,connect_state.rs}`, `rust/net/src/chat/{ws.rs,error.rs}`, `rust/net/src/proto/chat_websocket.proto`, `rust/net/res/{signal.cer,signal-ed25519.cer}`, `rust/net/infra/src/{lib.rs,ws.rs,tcp_ssl.rs,certs.rs,dns.rs,timeouts.rs,utils.rs}`, `rust/net/infra/src/route/{http.rs,proxy.rs}`, `rust/net/infra/src/tcp_ssl/proxy/https.rs`, `rust/bridge/shared/src/net.rs`, `rust/bridge/shared/src/net/chat.rs`, `rust/bridge/shared/types/src/net.rs`, `rust/bridge/shared/types/src/net/{chat.rs,remote_config.rs}`, `rust/core/src/{version.rs,address.rs}`, `java/client/src/main/java/org/signal/libsignal/net/{Network,ChatConnection}.java` |
| T4 | Reference distribution | `/work/third_party/signal-cli-0.14.7/lib/`: `libsignal-cli-0.14.7.jar` (`javap -c` of `SignalDependencies` and `StagingConfig`: only `LibSignalChatConnection` is referenced; the staging URLs match T1), `signal-cli-0.14.7.jar` (manifest `Implementation-Title: signal-cli`, `Implementation-Version: 0.14.7`; `BaseConfig` string `Signal-Android/8.21.1`), `libsignal-client-0.99.1.jar` (string search of `libsignal_jni_amd64.so`: contains `grpc.chat.staging.signal.org`, `/v1/websocket/`, `x-signal-receive-stories`), `whisper.store` inside `libsignal-cli-0.14.7.jar` (sha256 388a24e1…6bdf, byte-identical to T1's resource), listed with `keytool -storetype BKS` using the bundled `bcprov-jdk18on-1.85.jar`; certificate facts for T3's `.cer` files from `openssl x509` |

## 1. Answer in brief

signal-cli 0.14.7 uses **two transports to the chat server**:

1. **libsignal-net chat connections** (Rust, inside `libsignal_jni`) for
   almost everything after registration: the authenticated message pipe,
   the unauthenticated socket for sealed-sender sends and access-key
   fetches, prekeys, profiles, certificates, account calls. This is a
   **WebSocket carried over HTTP/2 (RFC 8441 extended CONNECT)** to
   `grpc.chat.staging.signal.org:443`, TLS 1.3 only, trusting only the
   two Signal roots compiled into libsignal. The WebSocket payload is the
   long-standing `WebSocketMessage` protobuf framing (pipe.md §3). There
   is **no Noise layer and no gRPC** on the path signal-cli takes (§3.6).
2. **OkHttp REST** (Java, `PushServiceSocket`) to
   `https://chat.staging.signal.org` for **registration and
   verification-session calls**, as a **fallback for 1:1 sends** when the
   WebSocket is unavailable, and for CDN, storage and SVR2 traffic. TLS 1.2
   or 1.3, trusting only the Signal RSA root in signal-cli's BKS
   truststore.

The old OkHttp WebSocket (`OkHttpWebSocketConnection`) exists in
signal-service-java but is **not used** by signal-cli 0.14.7 (T1
`SignalDependencies`, T4 javap).

## 2. Which transport carries what

| Traffic | Transport | Host (staging) | Evidence |
|---------|-----------|----------------|----------|
| Verification session create, read, patch (captcha, push challenge); request code; submit code; `POST /v1/registration`; SVR2 credential check | OkHttp REST, `PushServiceSocket` built with **static credentials** (no ACI, e164 = the number, device 1, the locally generated password) | `chat.staging.signal.org:443` | T1 `RegistrationManagerImpl` creates its account manager with `createWithStaticCredentials`; T2 `SignalServiceAccountManager.getRegistrationApi` wraps that socket; every `RegistrationApi` method calls a `PushServiceSocket` method |
| First authenticated calls after `/v1/registration` succeeds (account attributes, prekey upload, etc.) | libsignal-net authenticated chat | `grpc.chat.staging.signal.org:443` | T1 `RegistrationManagerImpl` uses `dependencies.getAccountApi()` (authenticated socket) right after registering; T2 `KeysApi`, `AccountApi` |
| Message pipe (receive, acks, `GET /v1/keepalive`) | libsignal-net authenticated chat | same | pipe.md |
| `PUT /v1/messages/{dest}` unsealed | libsignal-net authenticated chat; **REST fallback** | same; fallback `chat.staging.signal.org` | T2 `MessageApi.sendMessage`, `SignalServiceMessageSender` (see §5) |
| `PUT /v1/messages/{dest}` sealed (`Unidentified-Access-Key`) | libsignal-net unauthenticated chat; on 401 retried on authenticated chat; REST fallback | same | T2 `MessageApi`, `SignalWebSocket.UnauthenticatedWebSocket.request` |
| Multi-recipient (group) send | libsignal typed API `UnauthMessagesService` over the unauthenticated chat WebSocket | same | T2 `MessageApi.sendGroupMessage`; §3.6 explains why it stays on the WebSocket |
| Prekey count, upload, check, bundle fetch | libsignal-net chat (auth; bundle fetch may use unauth with access key) | same | T2 `KeysApi` |
| Sender certificate, profiles | libsignal-net chat | same | T1 `SignalDependencies` (`CertificateApi`, `ProfileApi` get the sockets) |
| Attachments, avatars, stickers | OkHttp to CDNs | `cdn-staging.signal.org`, `cdn2-staging.signal.org`, `cdn3-staging.signal.org` | T1 `StagingConfig`; outside M4 |
| Storage service | OkHttp | `storage-staging.signal.org` | outside M4 |
| CDSI (contact discovery) | libsignal-net enclave connection | `cdsi.staging.signal.org` | T1 `RecipientHelper` passes the `Network`; only needed to resolve a phone number to an ACI, not for a send to a known ACI |
| SVR2 (PIN / registration lock) | OkHttp WebSocket in `Svr2Socket` | `svr2.staging.signal.org` | T2 `Svr2Socket`; not touched by a fresh registration without a PIN |
| SVR-B (`svrb.staging.signal.org`) | libsignal-net | — | Present in libsignal's staging environment (T3 `env.rs`) but **not reached** by any M4 flow: signal-cli never creates backups during registration or messaging |

The trailing `false` argument of `SignalServiceConfiguration` in
`StagingConfig` is the field **`censored`**. It would enable libsignal's
censorship-circumvention routes (domain fronting through reflectors) if
signal-service-java's configuration-applying helper were called on the
`Network`. signal-cli **never calls that helper**; it applies only its own
proxy setting (§6). Either way the circumvention routes stay off, which
is also libsignal's default (§3.2).

## 3. libsignal-net chat connection

### 3.1 Endpoint

| Item | Value (staging) |
|------|-----------------|
| Hostname (DNS, SNI, `:authority`) | `grpc.chat.staging.signal.org`. Note: **not** `chat.staging.signal.org`. |
| Port | 443 |
| Path | `/v1/websocket/` (trailing slash, no query) for **both** the authenticated and the unauthenticated socket |
| TLS | min and in practice only **TLS 1.3**; ALPN offers exactly `h2` |
| HTTP | HTTP/2; the WebSocket is opened with an extended CONNECT |
| Roots | Signal's two private roots only (§3.3); no system roots, no pinning beyond that |

Production uses `grpc.chat.signal.org` the same way (T3 `env.rs`).

**DNS.** libsignal resolves the host itself: system resolver first
(5 s budget), then DNS-over-HTTPS to Cloudflare by IP (10 s), then a
built-in static table. For staging the static table is IPv4
`99.83.186.178`, `166.117.39.117` and IPv6
`2600:9000:a61f:527c:a456:24e3:e151:80d`,
`2600:9000:a507:ab6d:8674:deea:e137:6b31` (T3 `env.rs`, `infra/dns.rs`).
A Go client may use the system resolver only. The table is a resilience
measure, not a wire requirement.

### 3.2 Routes

signal-cli creates one libsignal `Network` per account with environment
`STAGING`, its user-agent string (§4), an **empty remote-config map**, and
build variant `PRODUCTION` (T1 `SignalDependencies.getLibSignalNetwork`).
libsignal's connection manager starts with domain fronting **disabled**
and minimum-TLS enforcement **on** (T3 bridge `net.rs`). The only route
is therefore the direct one. With an HTTP or SOCKS proxy configured (§6),
the same TLS/H2 connection is tunnelled through the proxy.

The fronting/reflector material in `env.rs` (Fastly
`reflector-staging-signal.global.ssl.fastly.net` with path prefix
`/service-staging`, Google `reflector-nrgwuv7kwq-uc.a.run.app`, SNI lists
such as `github.githubassets.com`) is **unused** by signal-cli and need
not be implemented.

### 3.3 TLS

- Library: BoringSSL. Minimum version TLS 1.3 for the chat host.
- ALPN: the single protocol `h2`.
- Key-exchange groups offered: X25519, P-256, P-384 (no hybrid ML-KEM).
- Signature algorithms accepted, in preference order: Ed25519,
  ECDSA-P256-SHA256, RSA-PSS-SHA256, RSA-PKCS1-SHA256, ECDSA-P384-SHA384,
  RSA-PSS-SHA384, RSA-PKCS1-SHA384, RSA-PSS-SHA512, RSA-PKCS1-SHA512
  (SHA-1 excluded) (T3 `tcp_ssl.rs`).
- Certificate validation: chain to one of the two roots below, with
  standard hostname verification against `grpc.chat.staging.signal.org`.
  The platform trust store is **not** consulted for this host.

Trusted roots for the chat host (T3 `certs.rs`, `res/*.cer`; facts from
`openssl x509`):

| | Root 1 | Root 2 |
|-|--------|--------|
| Subject = Issuer | `C=US, ST=California, L=Mountain View, O=Signal Messenger, LLC, CN=Signal Messenger` | same DN |
| Key | RSA 4096 | Ed25519 |
| Signature | sha256WithRSAEncryption (self-signed) | Ed25519 (self-signed) |
| Serial | `c1f3e20eb47080c1a4faf583267f24369a3e97` | `67adec3009f0933b73c7985bc835cc79` |
| Validity | 2022-01-26 00:45:51 UTC to 2032-01-24 00:45:50 UTC | 2026-06-16 21:29:14 UTC to 2036-06-16 21:34:14 UTC |
| SHA-256 of DER certificate | `DD:B0:F9:2B:B9:5C:8D:6F:D2:02:EA:6E:8C:C5:CC:D1:82:B5:44:F8:CD:69:6F:47:D5:80:65:9D:DC:9D:F6:5A` | `26:42:F1:F9:0B:38:9F:ED:25:58:D7:32:4C:A2:6D:70:E7:50:B0:6F:87:C5:95:C5:DE:95:5E:25:60:82:16:57` |
| SHA-256 of SubjectPublicKeyInfo | `c7ff606d9e4cdeb5eb5ad8e49ce4b499299b3dff2fe6c3e7dbd36a6f6a0aefb9` | `e5851ab98ad5a2fd564c38eeff3419cc682a3d86ff7dae092e08976a138dd508` |

Both are CA:TRUE. These are public certificates. The Go implementation
should embed its own copies, obtained from a published source, and check
them against the fingerprints above. Which root the staging server's
chain currently uses was **not observed** (no server contact). A Go
client should trust both.

Implementation note for Go: `crypto/tls` verifies Ed25519 certificate
signatures and supports TLS 1.3 and ALPN, so a `tls.Config` with a custom
`RootCAs` pool holding these two roots, `MinVersion` TLS 1.3,
`NextProtos` [`h2`] and `ServerName` set to the host reproduces this
validation.

### 3.4 WebSocket over HTTP/2

The connection carries an HTTP/2 extended CONNECT (RFC 8441). The server
must advertise `SETTINGS_ENABLE_CONNECT_PROTOCOL`; libsignal's H2 client
relies on it (T3 `infra/ws.rs`).

Request pseudo-headers and headers, in the order libsignal adds them:

| Header | Authenticated socket | Unauthenticated socket |
|--------|----------------------|------------------------|
| `:method` | `CONNECT` | same |
| `:protocol` | `websocket` | same |
| `:scheme` | `https` | same |
| `:authority` | `grpc.chat.staging.signal.org` | same |
| `:path` | `/v1/websocket/` | same |
| `authorization` | `Basic ` + standard base64 (with padding) of `<aci>.<deviceId>:<password>` (§4.1) | absent |
| `x-signal-receive-stories` | `true`, or `false` when the receive config ignores stories | absent |
| `accept-language` | absent (signal-cli passes no locale) | absent |
| `user-agent` | §4.2 | same |
| `sec-websocket-version` | `13` | same |

No `sec-websocket-key` and no `sec-websocket-extensions` header is sent
(RFC 8441 drops the key; libsignal refuses extensions and treats a
server-offered `sec-websocket-extensions` as a protocol error, so no
permessage-deflate). Any 2xx status completes the upgrade.

Upgrade-response handling:

| Response | Meaning to libsignal / signal-cli |
|----------|-----------------------------------|
| any 2xx | connected. `x-signal-timestamp` (ms) is reported for clock-skew logging; `x-signal-alert` values are logged (pipe.md §2) |
| has `Retry-After` (integer seconds; HTTP-date form not supported) | "retry later". This takes precedence over the status. All routes back off short-term |
| 403 | device deregistered (authenticated socket): signal-cli enters AUTHENTICATION_FAILED and marks the account unregistered |
| 499 | app expired (REMOTE_DEPRECATED) |
| 401 | generic failure; no back-off recorded (headers might change) |
| 429 | generic failure plus short-term back-off |
| other | generic connect failure |

(T3 `chat/error.rs`, `connect_state.rs`; T2 `LibSignalChatConnection`.)

libsignal lets only one authenticated connection attempt complete at a
time (pipe.md §2).

**Connect timing and back-off inside libsignal** (T3 `connect_state.rs`,
`timeouts.rs`): one route attempt may take at most 60 s (TCP 15 s, TLS
handshake at least 3 s). Failed attempts are remembered per route and
delay later attempts on that route. Delays grow by a factor of 10 per
recent failure, are capped at 30 s, and "recent" means the last 5 minutes
(short-term) or 6 hours (long-term). The Java receive loop adds its own
exponential back-off on top (pipe.md §6).

### 3.5 Frames, requests and the Java-to-wire mapping

The WebSocket carries only **binary** frames, each holding one
`WebSocketMessage` (field table in pipe.md §3; libsignal's copy of the
schema is T3 `chat_websocket.proto`). A text frame from the server is a
protocol error.

How signal-service-java's request objects become frames. These details
are byte-visible:

- signal-service-java builds request objects with a **random 64-bit id**
  and headers written as `name:value` with no space (for example
  `content-type:application/json` on every PUT, and on POSTs with a body)
  (T2 `WebSocketRequestExt.kt`). The adapter
  (T2 `LibSignalChatConnection`) **discards the id**, splits each header
  at the first colon, upper-cases the verb, and hands method, path, header
  map, body and a timeout to libsignal.
- libsignal assigns its **own** request ids: 0 for the first request on
  each connection, then +1 per request, wrapping at 2^64 (T3
  `RECOMMENDED_CHAT_WS_CONFIG.initial_request_id` = 0).
- libsignal re-serialises each header as `<lower-case name>: <value>`,
  with a colon and one space. The value is copied byte for byte from the
  Java side, so the wire form is, e.g., `content-type: application/json`
  and `unidentified-access-key: <base64>`.
- The body field is always present for requests that come from
  signal-service-java. GET requests (for example `GET /v1/keepalive`, `GET
  /v2/keys?identity=aci`) carry field 3 with **zero length**, because the
  adapter passes an empty byte array rather than no body. libsignal's own
  internal keep-alive (after a network change, or after a request has
  been outstanding for 5 s) omits the body field. The server accepts both.
- Default per-request timeout on the Java side is 10 s
  (T2 `WebSocketConnection.DEFAULT_SEND_TIMEOUT`).
- Responses: libsignal parses `status` (must be a valid HTTP code),
  `message`, `headers` (each split at the first `:` and trimmed) and
  `body`, and gives them to Java as an HTTP-like response. A 4xx/5xx is a
  completed round trip (pipe.md §5.2).

**Close codes** from the server (T3 `env.rs`, `chat/ws.rs`; T2):

| Code | Meaning | signal-cli reaction |
|------|---------|---------------------|
| 4401 | connection invalidated (credentials no longer valid, e.g. device unlinked or password changed) | in-flight requests fail as status 4401; the socket is dead |
| 4409 | connected elsewhere (a newer authenticated socket for the same device replaced this one) | socket dead; reconnect per pipe.md §6 |
| 1000 or any other | socket closed | socket dead; reconnect |

**Idle handling.** libsignal sends a WebSocket PING after 31 s of local
silence and drops the connection after 45 s without any frame from the
server. signal-cli's application keep-alive is `GET /v1/keepalive` every
30 s, with a 20 s deadline, while a keep-alive token is held. A socket
with no token is closed 30 s after its last request (pipe.md §5.1, §5.2;
T1 `SignalWebSocketHealthMonitor`).

### 3.6 Not used: gRPC and Noise

- **gRPC.** libsignal 0.99.1 can send some typed requests as gRPC over
  the same HTTP/2 connection. The hostname `grpc.chat…` reflects that.
  Each typed request goes over gRPC only if a remote-config key enables
  it: `grpc.MessagesAnonymousSendMultiRecipientMessage.2`,
  `grpc.MessagesAnonymousSendSingleRecipientMessage`,
  `grpc.MessagesSendMessage`, `grpc.AccountsAnonymousLookupUsernameHash`,
  `grpc.AccountsAnonymousLookupUsernameLink.2`,
  `grpc.AccountsAnonymousCheckAccountExistence.2`,
  `grpc.AttachmentsGetUploadForm`, `grpc.BackupsAnonymousGetUploadForm`.
  Each key must be present with a value other than `ws` (T3
  `remote_config.rs`, bridge `net.rs` `chat_grpc_overrides`). signal-cli
  passes an **empty** map and never calls `setRemoteConfig`, so every
  typed API, `UnauthMessagesService.sendMultiRecipientMessage` included,
  uses its WebSocket form. A Go client needs no gRPC. This resolves the
  question pipe.md §1 left open.
- **Noise.** No Noise handshake is involved on the chat path in 0.14.7.
  The only encrypted-tunnel protocols in libsignal-net are the enclave
  attestation channels (CDSI, SVR), which are outside this spec.

## 4. Credentials and agent headers

### 4.1 Authorization

| Transport | Identity part | Example username (before base64) |
|-----------|---------------|----------------------------------|
| libsignal-net authenticated chat | always `<ACI>.<deviceId>`. If the Java side omits the device, libsignal parses it and re-adds `.1` (T3 bridge `parse_username`, `chat.rs`). ACI is the lower-case hyphenated UUID with no `ACI:` prefix | `0b6ab9b6-…-c5d3.1` |
| OkHttp REST, after registration | `<ACI>` for the primary device (device 1, **no** `.1` suffix); `<ACI>.<deviceId>` for linked devices (T2 `PushServiceSocket.getAuthorizationHeader`) | `0b6ab9b6-…-c5d3` |
| OkHttp REST during registration | no ACI yet, so the **E.164 number** (`+…`), device 1, no suffix | `+15555550123` |

All three use `Authorization: Basic base64(<username>:<password>)`,
standard base64 alphabet with `=` padding. REST adds the header unless
the request already supplies its own `Authorization` header, is marked
unauthenticated, or carries a sealed-sender header instead (T2
`buildServiceRequest`). Which registration endpoints are sent without
credentials is in `registration.md`. How the password is generated is in
`api.md` / `registration.md`.

On the unauthenticated chat socket there is no `Authorization`. Sealed
sender requests add one header line to the individual request:
`Unidentified-Access-Key:<base64>` or `Group-Send-Token:<base64>`, which
goes on the wire as the lower-case `name: value` form (§3.5). If the
response is 401 and a fallback access exists, signal-service-java repeats
the request with the fallback (T2 `SignalWebSocket`,
`SealedSenderAccess`). The derivation of these values is in `send.md`.

### 4.2 User-Agent and X-Signal-Agent

signal-cli's agent string is `Signal-Android/8.21.1 signal-cli/0.14.7`.
The first token is `Signal-Android/8.21.1` unless the environment
variable `SIGNAL_CLI_USER_AGENT` is set, in which case its value replaces
that token. The second token is `<Implementation-Title>/<Implementation-Version>`
from the jar manifest (`signal-cli/0.14.7`), or bare `signal-cli` if the
manifest is missing (T1 `BaseConfig`; T4 manifest).

| Transport | `User-Agent` | `X-Signal-Agent` |
|-----------|--------------|------------------|
| libsignal-net chat (both sockets) | agent string + ` libsignal/0.99.1`, i.e. `Signal-Android/8.21.1 signal-cli/0.14.7 libsignal/0.99.1` (T3 `env.rs` `UserAgent::with_libsignal_version`, `core/version.rs`) | **not sent** |
| OkHttp REST to chat | agent string, set by an OkHttp interceptor that **replaces** OkHttp's default (T1 `ServiceConfig`) | agent string, same value (T2 `PushServiceSocket.buildServiceRequest`) |

No other custom headers ride on every request. REST adds `Host` only
when a URL has a host-header override, and staging URLs have none.

## 5. OkHttp REST side

| Item | Value |
|------|-------|
| Base URL | `https://chat.staging.signal.org` (path appended directly, e.g. `https://chat.staging.signal.org/v1/verification/session`) |
| TLS versions | TLS 1.3 and 1.2 only (T2 `Tls12SocketFactory`); OkHttp `ConnectionSpec.RESTRICTED_TLS` (no URL overrides it) |
| ALPN | OkHttp default: offers `h2` and `http/1.1` |
| Trust | only the certificates in signal-cli's BKS truststore `whisper.store` (password `whisper`), via a standard X.509 trust manager. Hostname verification is OkHttp's default verifier against `chat.staging.signal.org`. There is an extra rejection rule for certificates whose issuer DN is exactly `Open Whisper Systems` with serial 4098, which is irrelevant for current chains |
| Timeouts | connect and read 30 s |
| Connection pool | 5 idle connections, 45 s keep-alive |
| Retry | `retryOnConnectionFailure` = true (`AUTOMATIC_NETWORK_RETRY`). That is OkHttp's transport-level retry (stale pooled connection, failed route); there is **no** automatic retry on HTTP status codes. `Retry-After` handling is per endpoint, see `api.md` |
| DNS | system resolver |

`whisper.store` entries (T1 resource = T4 jar copy; `keytool` listing):

| Alias | Subject | Validity | Key / sig | SHA-256 (DER) |
|-------|---------|----------|-----------|---------------|
| `signal-messenger-ca` | `CN=Signal Messenger, O=Signal Messenger, LLC, L=Mountain View, ST=California, C=US` (self-signed) | 2022-01-26 to 2032-01-24 | RSA 4096 / SHA256withRSA | `DD:B0:F9:2B:…:DC:9D:F6:5A`, **the same certificate as libsignal's Root 1** |
| `textsecure-gcm-ca` | `CN=TextSecure, OU=Open Whisper Systems, O=Open Whisper Systems, L=San Francisco, ST=California, C=US` (self-signed) | 2013-03-25 to **2023-03-23 (expired)** | RSA 2048 / SHA1withRSA | `09:8F:D5:40:3F:63:FE:F8:9F:BA:4C:B4:B9:81:35:CA:2E:73:DC:42:C0:ED:03:A3:77:E8:38:3F:2D:1F:E9:EA` |

So the REST side effectively trusts only the Signal RSA root. The same
truststore applies to the CDN, storage and SVR2 URLs. That
`chat.staging.signal.org` presents a chain to that RSA root is inferred
(registration over REST works with it), **not observed**.

**1:1 send fallback.** signal-cli constructs its message sender with
"use REST fallback" = always true (T1 `SignalDependencies`). If a
WebSocket send fails for a **technical** reason (socket unavailable or an
I/O error), signal-service-java resends the same JSON with `PUT
/v1/messages/{dest}?story=…` over OkHttp: sealed sends with the
`Unidentified-Access-Key` header on the unidentified OkHttp client,
others with Basic auth. Server answers (401/403, 404, 409, 410, 428, 429,
5xx rejection) are **not** retried over REST; they propagate (T2
`SignalServiceMessageSender`). A Go client may implement this fallback or
simply retry on the WebSocket. The server sees the same request either
way.

## 6. Proxies

signal-cli decides on a proxy once, through the JVM's default proxy
selector, asking it about the URI `https://example` (T1
`Utils.getHttpsProxy`). It never sets `java.net.useSystemProxies`, and
the launcher script only passes `--enable-native-access=ALL-UNNAMED`.
With a stock JDK that means:

- **Honoured:** the Java system properties `https.proxyHost` /
  `https.proxyPort` (HTTP CONNECT proxy), or, if those are absent,
  `socksProxyHost` / `socksProxyPort`. They are supplied through
  `JAVA_OPTS` or `SIGNAL_CLI_OPTS`. `http.nonProxyHosts` applies as usual.
- **Not honoured:** the `HTTPS_PROXY` / `https_proxy` / `ALL_PROXY`
  environment variables. The JDK selector reads them only when
  `useSystemProxies` is on, and even then not on Linux, which uses desktop
  proxy settings instead. This is from JDK behaviour, not signal-cli code.

How the chosen proxy is applied:

| Transport | HTTP proxy | SOCKS proxy |
|-----------|-----------|-------------|
| libsignal-net | `Network.setProxy("http", host, port)` with no credentials. libsignal sends `CONNECT grpc.chat.staging.signal.org:443` with a `Host` header (hostname resolved by the proxy), then runs TLS 1.3 and h2 inside the tunnel. Because the proxy came from the system rather than from the app, libsignal uses it as **proxy first, then direct** if the proxy route fails (T3 `infer_proxy_mode_for_config`) | `setProxy("socks", …)` = SOCKS5 with no credentials, **hostname resolved locally** (only `socks5h` resolves remotely). Also proxy-then-direct |
| OkHttp | OkHttp consults the same JVM default selector per request, so the same proxy is used, through standard HTTP CONNECT or SOCKS. No direct fallback unless the selector offers one | same |

signal-cli's own `SignalProxy` (Signal TLS proxy) and `systemHttpProxy`
config fields are empty for staging (T1 `StagingConfig`).

For our egress-proxied test environment: a Go client that honours
`HTTPS_PROXY` with HTTP CONNECT behaves more conveniently than signal-cli,
without changing anything on the wire to Signal.

## 7. What a byte-compatible Go transport must do

1. For everything except registration: dial `grpc.chat.staging.signal.org:443`,
   TLS 1.3, ALPN `h2`, trusting exactly the two Signal roots; open an
   RFC 8441 WebSocket at `/v1/websocket/` with the headers of §3.4; speak
   binary `WebSocketMessage` frames with request ids counting up from 0
   per connection, headers formatted `lower-name: value`.
2. Keep one authenticated and, lazily, one unauthenticated socket per
   device. Follow pipe.md for keep-alive and reconnect.
3. For registration: plain HTTPS to `https://chat.staging.signal.org`
   (TLS 1.2+, Signal RSA root), Basic auth with the E.164 number as
   username, plus `User-Agent` and `X-Signal-Agent` both set to the agent
   string.
4. Send `User-Agent: Signal-Android/8.21.1 signal-cli/0.14.7 libsignal/0.99.1`
   on the chat WebSocket. Mimicking that is a choice. The server uses it
   only for client-version gating (499), so a Go client may substitute its
   own agent if the owner prefers. **Whether staging rejects unknown
   agents was not determined.**

## Gaps

- No server was contacted, so the staging servers' actual certificate
  chains (which root `grpc.chat.staging.signal.org` and
  `chat.staging.signal.org` chain to), their H2 SETTINGS, and their exact
  upgrade-response headers were not observed. A black-box capture of
  signal-cli is impossible without TLS interception, which is out of
  bounds.
- OkHttp 5.3.2 defaults (ALPN list, TLS cipher order) are stated from
  general OkHttp behaviour, not re-read from its source.
- JDK proxy-selector behaviour (§6) is general JDK knowledge, not read
  from JDK sources in this session.
