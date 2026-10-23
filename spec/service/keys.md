# Prekeys: upload, refill, fetch (signal-cli 0.14.7, staging)

Spec role: prose only; no code. This file covers how signal-cli 0.14.7
publishes its prekeys and fetches other users' bundles. The key formats
themselves are in `spec/crypto/xeddsa.md` (EC keys, XEd25519) and
`spec/crypto/kem.md` (Kyber-1024). How a fetched bundle is used is in
`spec/protocol/pqxdh.md` §6. Transport and JSON conventions are in
`api.md`. The signed and last-resort keys sent at registration are in
`registration.md` §4.

## Sources

| Ref | What | URL @ version |
|-----|------|---------------|
| K1 | signal-cli | https://github.com/AsamK/signal-cli @ v0.14.7 (b01b6b370dc063599a1a2b9fde0f5ff4e2d78fe8): `lib/src/main/java/org/asamk/signal/manager/helper/{PreKeyHelper,AccountHelper,IncomingMessageHandler,ReceiveHelper}.java`, `.../manager/util/KeyUtils.java`, `.../manager/config/ServiceConfig.java`, `.../manager/storage/SignalAccount.java`, `.../manager/storage/prekeys/{PreKeyStore,SignedPreKeyStore,KyberPreKeyStore}.java`, `.../manager/actions/RefreshPreKeysAction.java`, `.../manager/jobs/CleanOldPreKeysJob.java`, `.../manager/internal/ManagerImpl.java` |
| K2 | signal-service-java (Turasa fork) | https://github.com/Turasa/libsignal-service-java @ v2.15.3_unofficial_151 (66956cb4a0394d50cee3590209baf58312084a44): `lib/libsignal-service/src/main/java/org/whispersystems/signalservice/api/keys/{KeysApi.kt,OneTimePreKeyCounts.java}`, `.../internal/push/{PreKeyState,PreKeyEntity,KyberPreKeyEntity,PreKeyResponse,PreKeyResponseItem}.java`, `.../api/push/{SignedPreKeyEntity,ServiceIdType}.java`, `.../api/NetworkResultUtil.kt`, `.../api/SignalServiceMessageSender.java` (bundle use); `core/models-jvm/src/main/java/org/signal/core/models/ServiceId.kt` |
| K3 | libsignal | https://github.com/signalapp/libsignal @ v0.99.1 (97801d22dcf9f5bf714f7b8fa3212cdc973ae1c8): `java/shared/java/org/signal/libsignal/protocol/util/Medium.java` (ID bound), `rust/core/src/address.rs` (service-ID string form) |
| K4 | Reference distribution, offline | `jshell` over `/work/third_party/signal-cli-0.14.7/lib/*.jar`, no network: the bundled serializer applied to a hand-built upload body (key order and null emission in §2.2) |

## 1. Identities and numbering

- Every operation here is done separately for the **ACI** identity and the
  **PNI** identity, each with its own identity key pair, ID counters and
  key stores. The query parameter value is `aci` or `pni`.
- **ID space.** IDs are integers in `[0, 16777215)`, that is
  `0..0xFFFFFE`. Counters advance by 1 modulo 16777215 (K3 `Medium`).
  Initial counters are random in that range (`registration.md` §1).
  **The value 0 is a valid ID.**
- Each identity has three counters: next one-time EC ID, next signed-prekey
  ID and next Kyber ID. One-time Kyber keys and last-resort Kyber keys
  share the Kyber counter.
- Each identity also has two markers: the active signed prekey ID and the
  active last-resort Kyber ID (−1 = none).

## 2. Upload: `PUT /v2/keys?identity={aci|pni}`

Sent over the **authenticated WebSocket** with header
`content-type:application/json` (K2 `KeysApi.setPreKeysSync`).

### 2.1 Entities

| Entity | JSON members (in this order) | Encodings |
|--------|------------------------------|-----------|
| one-time EC prekey | `keyId`, `publicKey` | `keyId` number. `publicKey` is the 33-byte serialized key (`0x05` + 32), standard base64 **without** padding (44 chars). |
| signed EC prekey | `keyId`, `publicKey`, `signature` | as above, plus `signature`: 64-byte XEd25519 signature by the identity key over the 33-byte serialized public key, unpadded standard base64 (86 chars) |
| Kyber prekey (one-time or last-resort) | `keyId`, `publicKey`, `signature` | `publicKey` is the 1569-byte serialized Kyber-1024 key (`0x08` + 1568), unpadded standard base64 (2092 chars). `signature`: XEd25519 by the identity key over those 1569 bytes, unpadded. |

### 2.2 Body

Members, in this order, with **null members emitted as `null`** (K4):

| Member | Content |
|--------|---------|
| `preKeys` | array of one-time EC prekeys, or null |
| `signedPreKey` | signed EC prekey, or null |
| `pqLastResortPreKey` | last-resort Kyber prekey, or null |
| `pqPreKeys` | array of one-time Kyber prekeys, or null |

The identity key is **not** part of this body, since the server already
has it from registration.

Example with one one-time EC key and a signed key (keys elided):

    {"preKeys":[{"keyId":3,"publicKey":"…"}],"signedPreKey":{"keyId":7,"publicKey":"…","signature":"…"},"pqLastResortPreKey":null,"pqPreKeys":null}

### 2.3 Response handling

- 2xx: success. The new keys are stored locally only **after** the server
  accepts them.
- 401 (also 400, which the prekey mapping treats as authorization failed):
  logged ("Failed to updated pre keys") and **ignored**.
- 422 for the **PNI** identity: logged and ignored ("Account needs to be
  reregistered to fix this"). 422 for ACI is an error.
- 429: rate-limit error with no delay parsed.
- 508: server rejected.
- Anything else: error.

## 3. Refill: when and how much

### 3.1 Counts: `GET /v2/keys?identity={aci|pni}`

Authenticated WebSocket, no body. The response is
`{"count": <int>, "pqCount": <int>}`: the server's number of unused
one-time EC and one-time Kyber prekeys for this device and identity. If
this call fails with authorization failed (401 or 403), the failure is
ignored and both counts are taken as 0.

### 3.2 Decision rules

Constants (K1 `ServiceConfig`):

| Name | Value |
|------|-------|
| minimum one-time count | **10** |
| batch size | **100** |
| signed-prekey and last-resort rotation age | **2 days** |
| archive age for old signed and last-resort keys | 30 days |
| stale one-time key deletion age | 90 days, keeping at least 200 one-time keys |

For one identity, one refresh pass decides four things independently:

1. **One-time EC keys:** if `count` < 10, generate 100 new ones with IDs
   `next, next+1, …` (mod 16777215).
2. **Signed EC prekey:** rotate if there is no active signed prekey, it
   cannot be loaded, or its timestamp is more than 2 days old. The new ID
   is the next signed-prekey ID.
3. **One-time Kyber keys:** if `pqCount` < 10, generate 100 new ones with
   IDs from the Kyber counter. Each is signed by the identity key.
4. **Last-resort Kyber key:** rotate under the same rule as (2). Its ID is
   the Kyber counter plus the number of one-time Kyber keys generated in
   this same pass. In other words it comes right after them. (At the
   counter wrap this addition is not reduced modulo 16777215; a wrap is
   astronomically rare.)

If nothing was generated, no request is sent. Otherwise **one**
`PUT /v2/keys` carries everything generated in this pass, with null for
the parts not regenerated.

After a successful upload, the local store is updated:

- New one-time keys are added. **Before** adding them, every existing
  one-time key of that kind that has no stale mark gets marked stale with
  the current time.
- The new signed and last-resort keys become active.
- Counters advance.

If storing fails locally, the counters are reset to new random values and
the pass is repeated once.

### 3.3 Triggers

A refresh pass (ACI first, then PNI) runs in these cases (K1):

- right after registration (`registration.md` §7). On a fresh account
  this uploads 100 EC and 100 Kyber one-time keys per identity. The signed
  and last-resort keys from registration are less than 2 days old and are
  not resent.
- at the start of every command that loads the account (the account check,
  `api.md` §4.3). Accounts with storage version below 9 get a *forced*
  refresh instead, which regenerates all four kinds.
- after processing a received envelope whose type is `PREKEY_BUNDLE` (3),
  queued as an action, so one of our one-time keys may have been used.
- after a PNI change (out of scope).

So signal-cli has no timer: refills happen when the process starts or when
a prekey message arrives.

### 3.4 Local cleanup (no network)

When the server reports the queue empty during `receive`, a cleanup job
runs:

- It deletes signed prekeys and last-resort Kyber keys other than the
  active one that are older than 30 days. The newest such old key is kept.
- It deletes one-time keys (EC and Kyber separately) whose stale mark is
  older than 90 days. The 200 most recent keys are never deleted; the
  never-stale ones count as most recent.

A Go implementation only needs this behaviour to keep accepting prekey
messages that use recently replaced keys.

### 3.5 Not used: `POST /v2/keys/check`

signal-service-java has a repeated-use-key consistency check
(`POST /v2/keys/check` with a SHA-256 digest). signal-cli 0.14.7 never
calls it.

## 4. Fetching a bundle: `GET /v2/keys/{serviceId}/{device}`

### 4.1 Request

- `serviceId` is the service-ID string: a bare lowercase hyphenated UUID
  for an ACI, `PNI:` + UUID for a PNI (K2 `ServiceId.toString`, K3).
- `device` is `*` when the requested device ID is 1, which asks for **all**
  devices. Otherwise it is the decimal device ID. signal-cli asks for `*`
  when it has no session with device 1. When handling a 409 it asks for
  each missing device by number (`send.md` §5).
- **Transport:**
  - With sealed-sender access for the recipient, the request goes over
    the **unauthenticated** WebSocket with the
    `Unidentified-Access-Key:<b64>` header (`api.md` §5.2).
  - Otherwise it goes over the authenticated WebSocket.
  - Handling a 409 always fetches on the authenticated WebSocket.

### 4.2 Response JSON

| Member | Type | Notes |
|--------|------|-------|
| `identityKey` | string | 33-byte identity public key, base64 (decoded leniently) |
| `devices` | array | one entry per device |
| `devices[].deviceId` | int | |
| `devices[].registrationId` | int | |
| `devices[].signedPreKey` | object `{keyId, publicKey, signature}` | |
| `devices[].preKey` | object `{keyId, publicKey}` or absent | one-time EC key |
| `devices[].pqPreKey` | object `{keyId, publicKey, signature}` | a one-time Kyber key or the last-resort key; the client cannot tell which |

### 4.3 Mapping to the bundle of `spec/protocol/pqxdh.md` §6

For each device entry:

| Bundle item | From |
|-------------|------|
| registration ID | `registrationId` |
| device ID | `deviceId` |
| identity key | top-level `identityKey` (shared by all devices) |
| signed prekey ID, key, signature | `signedPreKey.*` |
| one-time EC prekey ID, key | `preKey.*` if present; otherwise "no one-time prekey" |
| Kyber prekey ID, key, signature | `pqPreKey.*` |

Devices are **skipped**, with a warning, when:

- `signedPreKey` is missing;
- `pqPreKey` is missing;
- any present key ID is outside `0..2147483647`.

If no device survives, the fetch counts as "no valid prekey bundles" and
the send to that recipient fails (`send.md` §5).

A session is then built for every returned device, not only device 1
(`pqxdh.md` §6 processing order, including the identity trust check).

### 4.4 Errors

These follow the prekey mapping in `api.md` §4.2:

- **404** means the recipient is not registered. signal-cli then re-checks
  the number through CDSI and retries once (`send.md` §5).
- **401** on an unauthenticated fetch means the access key was rejected.
  It surfaces as authorization failed, which makes the send loop drop
  sealed sender for the next attempt (`send.md` §3.3).
- **429**: rate limited, no delay parsed.

## Gaps

- Server limits on upload size or count per request were not checked;
  signal-cli never sends more than 100 + 100 one-time keys at once.
