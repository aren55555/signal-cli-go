# Device linking (provisioning)

Spec role: prose only; no code. This file says how signal-cli 0.14.7 links
itself as a **secondary** device (`link`, `startLink` / `finishLink`), and
what it does as a **primary** when it links another device (`addDevice`).
It covers the provisioning WebSocket, the messages carried on it, the
provisioning cipher, the `sgnl://linkdevice` URL and the REST calls made
before and after.

Related specs (not repeated here):
- `spec/service/transport.md`: TLS, hosts, the authenticated chat
  connection (libsignal-net) and the WebSocket request/response framing
  (`WebSocketMessage` and friends; field tables are also in
  `spec/service/pipe.md`).
- `spec/service/api.md`: HTTP Basic auth, REST error mapping.
- `spec/service/registration.md`: account attributes and capabilities.
- `spec/service/keys.md`: signed-prekey and Kyber-prekey JSON entities and
  how the key material is generated.
- `spec/crypto/xeddsa.md`: Curve25519 key encodings (33-byte serialized
  public key with the 0x05 type byte, 32-byte private key).

## Sources

| Ref | What | URL @ version |
|-----|------|---------------|
| L1 | signal-cli | https://github.com/AsamK/signal-cli @ v0.14.7 (`b01b6b370dc063599a1a2b9fde0f5ff4e2d78fe8`): `lib/src/main/java/org/asamk/signal/manager/internal/ProvisioningManagerImpl.java`, `.../internal/ManagerImpl.java` (`addDeviceLink`), `.../helper/AccountHelper.java` (`addDevice`), `.../helper/SyncHelper.java` (`requestAllSyncData`), `.../api/DeviceLinkUrl.java`, `.../util/Utils.java` (`getQueryMap`), `.../util/KeyUtils.java` (`generatePreKeysForType`), `.../storage/SignalAccount.java` (`setProvisioningData`, `finishLinking`, `getAccountAttributes`, `isPrimaryDevice`), `.../config/ServiceConfig.java` (`getCapabilities`); `src/main/java/org/asamk/signal/commands/{LinkCommand,AddDeviceCommand}.java` |
| L2 | signal-service-java fork | https://github.com/Turasa/libsignal-service-java @ v2.15.3_unofficial_151 (`66956cb4a0394d50cee3590209baf58312084a44`): `lib/libsignal-service/src/main/java/org/whispersystems/signalservice/api/provisioning/{ProvisioningSocket.kt,ProvisioningMessage.java}`, `.../internal/crypto/{PrimaryProvisioningCipher.java,SecondaryProvisioningCipher.kt}`, `.../api/registration/RegistrationApi.kt` (`registerAsSecondaryDevice`), `.../internal/push/{PushServiceSocket.java,RegisterAsSecondaryDeviceRequest.kt,GcmRegistrationId.kt,PreKeyEntity.java,KyberPreKeyEntity.java}`, `.../api/push/SignedPreKeyEntity.java`, `.../api/account/{DeviceAttributes.kt,AccountAttributes.kt}`, `.../api/link/LinkedDeviceVerificationCodeResponse.kt`, `.../api/messages/multidevice/RegisterAsSecondaryDeviceResponse.java`, `.../api/SignalUrlExtensions.kt`; `lib/network/src/main/java/org/signal/network/api/LinkDeviceApi.kt`; `core/util-jvm/src/main/java/org/signal/core/util/{crypto/DeviceNameCipher.kt,Base64.kt,StringExtensions.kt}`, `core/util-jvm/src/main/protowire/DeviceName.proto`; `core/network/src/main/java/org/signal/network/util/JsonUtil.java`; test `lib/libsignal-service/src/test/.../internal/crypto/SecondaryProvisioningCipherTest.kt` |
| L3 | Reference jar | `/work/third_party/signal-cli-0.14.7/lib/signal-service-java-2.15.3_unofficial_151.jar`: bundled `Provisioning.proto` and `RegistrationProvisioning.proto` (field numbers below were read from these), cross-checked with L2 |
| L4 | libsignal | https://github.com/signalapp/libsignal @ v0.99.1 (`97801d22dcf9f5bf714f7b8fa3212cdc973ae1c8`): `java/shared/java/org/signal/libsignal/protocol/kdf/HKDF.kt`, `rust/bridge/shared/src/protocol.rs` (`HKDF_DeriveSecrets`: salt absent when the 3-argument form is used) |
| L5 | Signal-Server | https://github.com/signalapp/Signal-Server @ main `8edf3488f7d97398db7ffa5184c6b769a272ff59` (no tag matches the staging deployment; read for server-side behavior only): `service/src/main/java/org/whispersystems/textsecuregcm/websocket/ProvisioningConnectListener.java`, `.../controllers/ProvisioningController.java` |
| L6 | Standards | RFC 5869 (HKDF), RFC 2104 (HMAC), NIST SP 800-38A (CBC), RFC 5652 §6.3 (PKCS#7 padding), RFC 4648 (base64) |

No fixed test vectors exist upstream: the only provisioning-cipher test
(L2 `SecondaryProvisioningCipherTest.kt`) uses freshly generated random
keys. No `testdata/vectors/provisioning_*.json` was written.

## 1. Overview of a link

1. The new (secondary) device generates a temporary Curve25519 key pair
   and opens an **unauthenticated** provisioning WebSocket.
2. The server sends it a random **provisioning address**.
3. The secondary shows the URL
   `sgnl://linkdevice?uuid=<address>&pub_key=<temporary public key>`
   (usually as a QR code).
4. The primary scans it, fetches a one-time **verification code** from
   the server, encrypts a `ProvisionMessage` (identity keys, service IDs,
   number, profile key, verification code, …) to the secondary's public
   key and uploads it to `PUT /v1/provisioning/<address>`.
5. The server forwards it on the secondary's provisioning socket, then
   closes that socket.
6. The secondary decrypts it, and registers itself with
   `PUT /v1/devices/link`, using the verification code. The server
   assigns a device ID.

## 2. The provisioning WebSocket (secondary side)

### 2.1 Connection

- Transport: **plain OkHttp WebSocket**, *not* libsignal-net. The fork's
  provisioning socket builds its own OkHttp client from the service URL
  configuration (same trust store and TLS connection specs as REST; see
  `transport.md`), with 30 s connect and read timeouts and no retry on
  connection failure. The configured network interceptors apply, so the
  upgrade request carries signal-cli's `User-Agent` header.
- URL: the chat service base URL with `https://` replaced by `wss://`,
  plus the path **`/v1/websocket/provisioning/`** (trailing slash
  included). No query string.
- No authentication: no `Authorization` header, no credentials.
- A `Host` header is added only when the URL configuration carries an
  alternate host header (censorship-circumvention fronting; not used for
  staging or production defaults).

Framing on the socket is the same `WebSocketMessage` envelope as on the
chat socket (type 1 = REQUEST with field 2 a `WebSocketRequestMessage`,
type 2 = RESPONSE with field 3 a `WebSocketResponseMessage`); field
tables are in `pipe.md`. All frames are binary.

### 2.2 Messages from the server

The server sends exactly two kinds of requests (L5):

| Verb | Path | Body | When |
|------|------|------|------|
| `PUT` | `/v1/address` | `ProvisioningAddress` protobuf (§4.1) | Immediately after the socket opens. |
| `PUT` | `/v1/message` | `ProvisionEnvelope` protobuf (§4.2), raw bytes (not base64) | When a primary has uploaded a provisioning message for this address. The server closes the socket (code 1000) right after the client's response to this request. |

Both requests carry one header, a server timestamp (Signal-Server's
standard timestamp header, `X-Signal-Timestamp: <ms>`); the client ignores
it.

The server's provisioning address is 16 random bytes encoded as
**URL-safe base64 with padding** (so 24 characters ending in `==`). The
client must treat it as an opaque string.

The server closes an idle provisioning socket after a server-configured
timeout with code 1000 reason "Timeout" (value not visible in the client;
**unknown**).

### 2.3 Client responses (acknowledgement)

For **every** incoming request frame, the client immediately sends a
RESPONSE frame whose `WebSocketResponseMessage` has:
- `id` = the request's `id`,
- `status` = 200,
- `message` = `"OK"`,
- no body, no headers.

The response is sent *before* the body is interpreted, so it is sent even
for an unknown path or a body that fails to decrypt. If sending fails,
the client closes the socket (1000, "OK").

Processing after acknowledging:
- `PUT /v1/address` with a body: decode `ProvisioningAddress`; if the
  `address` field is missing, the link fails. Otherwise build the link URL
  (§5).
- `PUT /v1/message` with a body: decode `ProvisionEnvelope` and decrypt
  (§3). (In the fork's re-registration mode, not used by signal-cli, the
  body would be a `RegistrationProvisionEnvelope` instead, §4.4.)
- Any other path: ignored (logged). Any non-PUT verb or missing body:
  ignored.

### 2.4 Keepalive and timers

- Every **30 s** after open, the client sends a REQUEST frame: verb `GET`,
  path `/v1/keepalive`, `id` = current wall-clock time in milliseconds, no
  body, no headers. A RESPONSE frame whose `id` equals the last keepalive
  id is swallowed.
- If no `/v1/address` arrives within **10 s** of open, the link fails
  ("No device id received").
- If no `/v1/message` arrives within **90 s** of open (the socket's
  lifespan), the link fails ("No provisioning message received").
- If the server closes with a code other than 1000, the client closes with
  1000 and the attempt ends. On a transport failure the client closes with
  1000 and the attempt ends. No reconnect is attempted; the user must start
  a new link.
- Client-initiated close uses code 1000, reason "Manual shutdown".

signal-cli adds its own waits on top (L1): it waits up to **30 s** for the
link URL and, in `finishLink` / after printing the URL in `link`, up to
**120 s** for the decrypted message. In practice the 90 s socket lifespan
fires first.

## 3. The provisioning cipher

Both directions use the same construction ("version 1"). Notation: `||`
is concatenation.

### 3.1 Keys

- **Secondary**: a fresh Curve25519 identity-style key pair generated for
  this link only (signal-cli: a new identity key pair per
  `ProvisioningManager` instance). Its 33-byte serialized public key
  (0x05 type byte + 32 bytes) goes into the URL.
- **Primary**: a fresh ephemeral Curve25519 key pair per encryption. Its
  33-byte serialized public key goes into `ProvisionEnvelope.publicKey`.

### 3.2 Key derivation

1. `shared` = X25519(own private key, other side's public key), 32 bytes.
   (The type byte is stripped from the 33-byte public key before the
   X25519 operation, as everywhere in libsignal.)
2. `okm` = HKDF-SHA256 (RFC 5869) with
   - IKM = `shared`,
   - **salt absent** (RFC 5869 default: 32 zero bytes),
   - info = the ASCII string **`TextSecure Provisioning Message`**
     (31 bytes, no terminator),
   - L = 64.
3. `cipherKey` = `okm[0..32)`, `macKey` = `okm[32..64)`.

### 3.3 Body layout (`ProvisionEnvelope.body`)

| Offset | Length | Content |
|--------|--------|---------|
| 0 | 1 | Version byte, **0x01** |
| 1 | 16 | AES-CBC IV, random |
| 17 | n | AES-256-CBC ciphertext of the serialized `ProvisionMessage`, PKCS#7 padded (n is a positive multiple of 16) |
| 17+n | 32 | HMAC-SHA256 under `macKey` over bytes `[0, 17+n)`, i.e. version byte, IV and ciphertext |

### 3.4 Encrypt (primary)

Generate the ephemeral key pair; derive keys; generate a random IV; encrypt
the serialized `ProvisionMessage` with AES-256-CBC + PKCS#7 under
`cipherKey`; compute the MAC over `0x01 || IV || ciphertext`; body =
`0x01 || IV || ciphertext || MAC`; wrap in a `ProvisionEnvelope` with
`publicKey` = the ephemeral public key (33 bytes) and `body`.

### 3.5 Decrypt (secondary)

Reject (link fails with "Failed to decrypt provisioning message") if any
of these hold:
- `publicKey` or `body` missing or empty;
- `body` length minus 1 − 16 − 32 is ≤ 0;
- first byte ≠ 0x01;
- the MAC over everything except the last 32 bytes, computed with
  `macKey`, does not equal the last 32 bytes (constant-time compare);
- AES-CBC decryption or PKCS#7 unpadding fails.

Otherwise parse the plaintext as `ProvisionMessage`.

## 4. Protobuf messages

Package `signalservice`, proto2 unless noted. Field numbers from L3.

### 4.1 ProvisioningAddress

| # | Name | Type | Meaning |
|---|------|------|---------|
| 1 | `address` | string | Opaque provisioning address for this socket (§2.2). |

**ProvisioningUuid vs ProvisioningAddress.** Earlier versions of the
upstream protos called this message `ProvisioningUuid` with field 1 named
`uuid` (also a string), and the server then sent a UUID string. The wire
format is identical (field 1, string); only the name and the content
changed: the current server sends 16 random bytes in URL-safe base64 with
`==` padding, not a UUID. The URL query key is still `uuid` (§5). A client
must not validate it as a UUID. (The historical name is from memory of
older upstream releases, not from a file read for this spec.)

### 4.2 ProvisionEnvelope

| # | Name | Type | Meaning |
|---|------|------|---------|
| 1 | `publicKey` | bytes | Primary's ephemeral public key, 33 bytes serialized. |
| 2 | `body` | bytes | Encrypted `ProvisionMessage`, layout §3.3. |

### 4.3 ProvisionMessage

| # | Name | Type | Meaning |
|---|------|------|---------|
| 1 | `aciIdentityKeyPublic` | bytes | ACI identity public key, 33 bytes. |
| 2 | `aciIdentityKeyPrivate` | bytes | ACI identity private key, 32 bytes. |
| 3 | `number` | string | Account E.164 phone number. |
| 4 | `provisioningCode` | string | Verification code from `GET /v1/devices/provisioning/code` (§6.1). |
| 5 | `userAgent` | string | Free-form; unset by signal-cli. |
| 6 | `profileKey` | bytes | 32-byte profile key. |
| 7 | `readReceipts` | bool | Primary's read-receipt setting; unset by signal-cli's primary. |
| 8 | `aci` | string | ACI as a plain lowercase UUID string. |
| 9 | `provisioningVersion` | uint32 | Enum `ProvisioningVersion`: INITIAL = 0, TABLET_SUPPORT = 1, CURRENT = 1 (alias). Senders set 1. |
| 10 | `pni` | string | PNI as a plain UUID string **without** the `PNI:` prefix. |
| 11 | `pniIdentityKeyPublic` | bytes | PNI identity public key, 33 bytes. |
| 12 | `pniIdentityKeyPrivate` | bytes | PNI identity private key, 32 bytes. |
| 13 | — | — | Reserved (formerly the master key; superseded by field 15). |
| 14 | `ephemeralBackupKey` | bytes | 32 bytes, for link-and-sync; unset by signal-cli. |
| 15 | `accountEntropyPool` | string | Account entropy pool (64-character string). |
| 16 | `mediaRootBackupKey` | bytes | 32 bytes. |
| 17 | `aciBinary` | bytes | ACI as 16 raw UUID bytes. |
| 18 | `pniBinary` | bytes | PNI as 16 raw UUID bytes (no prefix byte). |

### 4.4 RegistrationProvisionEnvelope (not used by signal-cli)

The fork also defines (proto3, package of `RegistrationProvisioning.proto`)
`RegistrationProvisionEnvelope` with field 1 `publicKey` bytes and field 2
`body` bytes, carrying an encrypted `RegistrationProvisionMessage`, for
the `sgnl://rereg` re-registration flow. It uses the same cipher. signal-cli
0.14.7 never opens that mode; it is listed only so an implementation knows
to ignore it.

### 4.5 DeviceName (for the link request's `name`)

| # | Name | Type | Meaning |
|---|------|------|---------|
| 1 | `ephemeralPublic` | bytes | 33-byte ephemeral Curve25519 public key. |
| 2 | `syntheticIv` | bytes | 16 bytes. |
| 3 | `ciphertext` | bytes | AES-CTR ciphertext of the UTF-8 name. |

Device-name encryption, keyed to the account's **ACI identity key**:
1. Generate an ephemeral Curve25519 key pair; `master` = X25519(ephemeral
   private, ACI identity public).
2. `ivKey` = HMAC-SHA256(key = `master`, data = ASCII `auth`);
   `syntheticIv` = first 16 bytes of HMAC-SHA256(key = `ivKey`, data =
   plaintext name).
3. `cipherKeyKey` = HMAC-SHA256(key = `master`, data = ASCII `cipher`);
   `cipherKey` = HMAC-SHA256(key = `cipherKeyKey`, data = `syntheticIv`)
   (32 bytes).
4. `ciphertext` = AES-256-CTR with `cipherKey`, initial counter block all
   zeros (16 zero bytes), no padding.
5. Serialize the `DeviceName` message. Decryption recomputes the IV from
   the decrypted plaintext and rejects on mismatch.

signal-cli stores this serialized `DeviceName` as **base64 without
padding** (standard alphabet), and that string is the `name` attribute
sent at link time.

## 5. The `sgnl://linkdevice` URL

Format produced by the secondary (L2 `ProvisioningSocket`):

    sgnl://linkdevice?uuid=<A>&pub_key=<K>[&capabilities=backup5]

- `<A>` = the provisioning address, form-URL-encoded (Java
  `URLEncoder`, UTF-8). Since the address ends in `==` and may contain
  `-`/`_`, the `=` characters become `%3D`.
- `<K>` = the secondary's 33-byte serialized public key in **standard
  base64 (RFC 4648 §4, `+` and `/`) with the `=` padding removed** (44
  characters), then form-URL-encoded (`+` → `%2B`, `/` → `%2F`).
- `&capabilities=backup5` is appended only when the secondary supports
  link-and-sync. **signal-cli 0.14.7 never appends it** (it opens the
  socket in link mode with that capability off).
- Parameter order is `uuid`, `pub_key`, then the optional capability.

Parsing on the primary (signal-cli `addDevice`, L1 `DeviceLinkUrl`):
take the raw query string; split on `&`, each part on the first `=`;
URL-decode names and values (UTF-8); require non-empty `uuid` and
`pub_key`; decode `pub_key` with a standard base64 decoder that accepts
missing padding; it must parse as a Curve25519 public key (33 bytes,
type byte 0x05), else "Invalid device link". The scheme and host are not
checked; unknown parameters (such as `capabilities`) are ignored. A query
string that is entirely missing raises an unchecked error.

signal-cli also has a URL builder for the same format (used for
output/JSON), which produces `uuid` and `pub_key` exactly as above and
never adds `capabilities`.

## 6. Primary side: `addDevice`

Precondition: the local account is the primary (device ID 1); otherwise
signal-cli fails with "not a primary device" without contacting the
server.

Both requests go over the **authenticated chat connection** (libsignal-net,
`transport.md`), as WebSocket requests, not as separate HTTPS requests.

### 6.1 Get a verification code

`GET /v1/devices/provisioning/code`, no body. Response 200 JSON:

| Key | Type | Meaning |
|-----|------|---------|
| `verificationCode` | string | Goes into `ProvisionMessage.provisioningCode`. |
| `tokenIdentifier` | string | For `wait_for_linked_device`; unused by signal-cli. |

Errors: 411 = account at the device limit (signal-cli: "Too many linked
devices"), 429 = rate limited. Other errors per `api.md`.

### 6.2 Send the provisioning message

Build a `ProvisionMessage` with:
`aciIdentityKeyPublic`/`Private`, `pniIdentityKeyPublic`/`Private` (the
account's two identity key pairs), `aci` (plain UUID string), `pni` (UUID
string without prefix), `number` (account E.164), `provisioningCode` (§6.1),
`profileKey` (the account's), `provisioningVersion` = 1,
`accountEntropyPool` (created on the spot if the account has none),
`mediaRootBackupKey` (created if absent), `aciBinary` and `pniBinary`
(16 raw bytes each). Left unset: `userAgent`, `readReceipts`,
`ephemeralBackupKey`. (signal-cli also creates a PIN master key if none
exists, but it is not put in the message.)

Encrypt it to the URL's public key (§3.4), serialize the
`ProvisionEnvelope`, base64-encode it **with padding** (standard
alphabet), and send:

`PUT /v1/provisioning/<address>` where `<address>` is the decoded `uuid`
value re-encoded with Java form-URL encoding (so `=` → `%3D`).
Body: JSON `{"body": "<base64 ProvisionEnvelope>"}`, content type JSON.

Server responses (L2/L5): 204 (or 200) success; 400 body larger than
256 KiB; 403 bad account or code; 404 no listening socket at that address
(the secondary's socket is gone); 409 new device lacks a required
capability; 411 device limit; 422 malformed; 429 rate limited.

### 6.3 After success

signal-cli marks the account as multi-device and queues a storage-service
sync job. It does **not** send contact/group/configuration sync messages
here; the new device asks for them (§7 step 7) and signal-cli answers such
requests through its normal receive path (out of scope for this file).

## 7. Secondary side: signal-cli `link` / `startLink` + `finishLink`

1. Generate the temporary key pair and a random account **password**
   (used later for Basic auth). Open the provisioning socket (§2).
2. Wait (≤ 30 s) for the URL; `link` prints it, `startLink` returns it.
3. Wait (≤ 120 s, bounded in practice by the 90 s socket lifespan) for
   the decrypted `ProvisionMessage`. Decryption failure → error.
4. Extract `number`; ACI from `aci` / `aciBinary` and PNI from `pni` /
   `pniBinary` (either form is accepted; a value that cannot be parsed
   fails the link). If a local account with this ACI (else this number)
   already exists, it may be re-linked only when it is not a primary, not
   registered in the other environment, and either has no valid device ID
   or the server rejects its current credentials; otherwise "user already
   exists".
5. Load the ACI and PNI identity key pairs from fields 1/2 and 11/12
   (invalid → "Invalid key material"); the profile key from field 6, or
   generate a fresh one if absent; the account entropy pool (15) and media
   root backup key (16) if present. Encrypt the device name (default
   `"cli"`, overridable with `--name`) per §4.5. Apply `readReceipts` if
   present. Generate, for each of ACI and PNI, one signed EC prekey and one
   last-resort Kyber prekey (both signed with that identity's key; ID
   allocation per `keys.md`). One-time prekeys are not uploaded here.
6. **Register the device**: `PUT /v1/devices/link`, sent as a plain
   **HTTPS REST** request (the fork's push-service socket, not the chat
   WebSocket), JSON body, with header
   `Authorization: Basic base64("<number>:<password>")`, where `<number>`
   is the E.164 from the message (no device suffix, since the device ID is
   not yet known) and `<password>` the one from step 1.

   Request JSON:

   | Key | Type | Value |
   |-----|------|-------|
   | `verificationCode` | string | `ProvisionMessage.provisioningCode` |
   | `accountAttributes` | object | see below |
   | `aciSignedPreKey` | object | `{keyId, publicKey, signature}` (`keys.md`) |
   | `pniSignedPreKey` | object | same, PNI |
   | `aciPqLastResortPreKey` | object | `{keyId, publicKey, signature}` Kyber (`keys.md`) |
   | `pniPqLastResortPreKey` | object | same, PNI |
   | `gcmToken` | null | always JSON `null` (the serializer writes nulls) |

   `accountAttributes` (exactly these five keys):

   | Key | Type | Value from signal-cli |
   |-----|------|------|
   | `fetchesMessages` | bool | `true` |
   | `registrationId` | int | ACI registration ID |
   | `pniRegistrationId` | int | PNI registration ID |
   | `name` | string | encrypted device name, base64 without padding (§4.5) |
   | `capabilities` | object | linked-device set: `storage` true, `versionedExpirationTimer` true, `attachmentBackfill` true, `spqr` true, `usernameChangeSyncMessage` true |

   (Capabilities depend on "is primary"; the device ID is still 0 at this
   point, so the linked-device values apply. See `registration.md` for the
   primary values.)

   Response 200 JSON: `uuid` (ACI), `pni`, `deviceId`. signal-cli reads
   only `deviceId` and parses it as an integer (the server sends a number;
   the fork's model declares it as a string, which Jackson coerces).
   Errors per `api.md` (403 bad code, 409 missing capability, 411 device
   limit, 422, 429).
7. Store device ID, mark registered, store the prekeys as uploaded. Then,
   now authenticated as `<aci>.<deviceId>` (see `api.md`): refresh
   prekeys (upload one-time EC and Kyber prekeys as needed, `keys.md`),
   and send sync-request messages to the own account (groups, contacts,
   blocked, configuration, then keys, in that order (L1 `SyncHelper.requestAllSyncData`); failure only logs a warning
   telling the user to run `sendSyncRequest`). The account is
   multi-device from the start.
8. If anything before step 6's success fails and the account file was
   newly created, the partial account is deleted.

## 8. Gaps and unknowns

- Server-side provisioning-socket idle timeout: not visible in client or
  in the server class read (it is configuration); unknown.
- Whether `PUT /v1/provisioning/<address>` returns 200 or 204 on staging:
  the server code declares 204; the client accepts any 2xx.
- The historical `ProvisioningUuid` name (§4.1) is recalled, not cited.
- The exact sync-request set sent after linking belongs to a sync spec.
- No fixed provisioning vectors exist upstream; none were invented.
