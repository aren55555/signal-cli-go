# Registration and verification as signal-cli 0.14.7 does it (staging)

Spec role: prose only; no code. This file covers `register` and `verify`
for a fresh primary account: the local material created beforehand, the
verification-session API, the `/v1/registration` body, what signal-cli
does right after registration succeeds, and error handling. Registration
lock and PIN behaviour is covered only as far as a fresh account needs it.

Conventions (transport, authentication, JSON, status codes) are in
`api.md`. Prekey formats are in `keys.md`. The profile upload done right
after registration is in `profile.md`.

## Sources

| Ref | What | URL @ version |
|-----|------|---------------|
| R1 | signal-cli | https://github.com/AsamK/signal-cli @ v0.14.7 (b01b6b370dc063599a1a2b9fde0f5ff4e2d78fe8): `lib/src/main/java/org/asamk/signal/manager/internal/{RegistrationManagerImpl,ManagerImpl}.java`, `.../manager/util/{NumberVerificationUtils,KeyUtils,Utils}.java`, `.../manager/storage/SignalAccount.java`, `.../manager/SignalAccountFiles.java`, `.../manager/config/ServiceConfig.java`, `.../manager/helper/{AccountHelper,PreKeyHelper,ProfileHelper,PinHelper}.java`, `src/main/java/org/asamk/signal/commands/{RegisterCommand,VerifyCommand}.java`, `man/signal-cli.1.adoc` |
| R2 | signal-service-java (Turasa fork) | https://github.com/Turasa/libsignal-service-java @ v2.15.3_unofficial_151 (66956cb4a0394d50cee3590209baf58312084a44): `lib/libsignal-service/src/main/java/org/whispersystems/signalservice/api/registration/RegistrationApi.kt`, `.../internal/push/{PushServiceSocket.java,VerificationSessionMetadataRequestBody.kt,UpdateVerificationSessionRequestBody.kt,RegistrationSessionRequestBody.kt,RegistrationSessionMetadataResponse.kt,VerificationCodeFailureResponseBody.kt,VerifyAccountResponse.java,GcmRegistrationId.kt,KyberPreKeyEntity.java,PreKeyEntity.java}`, `.../api/push/SignedPreKeyEntity.java`, `.../api/account/AccountAttributes.kt`, `.../api/crypto/UnidentifiedAccess.java` |
| R3 | libsignal | https://github.com/signalapp/libsignal @ v0.99.1 (97801d22dcf9f5bf714f7b8fa3212cdc973ae1c8): `java/shared/java/org/signal/libsignal/protocol/util/KeyHelper.java` (registration ID range) |
| R4 | Signal-Server | https://github.com/signalapp/Signal-Server @ v20261022.0.0 (8edf3488f7d97398db7ffa5184c6b769a272ff59): `service/src/main/java/org/whispersystems/textsecuregcm/controllers/{VerificationController,RegistrationController}.java`, `.../entities/{RegistrationRequest,AccountAttributes,AccountIdentityResponse,AccountCreationResponse}.java`, `.../storage/DeviceCapability.java` (server behaviour notes only) |
| R5 | Reference distribution, offline | `jshell` over `/work/third_party/signal-cli-0.14.7/lib/*.jar`, no network: the bundled serializer applied to hand-built `AccountAttributes`, `VerificationSessionMetadataRequestBody`, `UpdateVerificationSessionRequestBody` and `RegistrationSessionRequestBody` objects, to confirm the exact JSON shapes in §2 and §4 |

## 1. Local material created before any request

When `register` runs for a number with no local account, signal-cli
creates the account file first (R1 `SignalAccountFiles.initRegistrationManager`,
`SignalAccount.create`):

| Item | How it is made |
|------|----------------|
| ACI identity key pair | fresh Curve25519 pair (`spec/crypto/xeddsa.md` encodings) |
| PNI identity key pair | fresh Curve25519 pair (regenerated at `verify` time if missing) |
| profile key | 32 random bytes |
| password | 18 random bytes, standard base64 with padding (`api.md` §2.1) |
| device ID | 1 |
| ACI registration ID, PNI registration ID | each uniformly random in **1..16380** inclusive (R3 `generateRegistrationId(false)`) |
| prekey ID counters | for each of ACI and PNI: next one-time EC ID, next signed-prekey ID and next Kyber ID are each uniformly random in **0..16777214** (that is, `[0, 0xFFFFFF)`). The "active signed prekey" and "active last-resort Kyber" markers start at −1 (none). |

Nothing about the account is sent to the server until the verification
session starts.

## 2. Verification session

All of these are **REST** requests to `https://chat.staging.signal.org`,
authenticated with Basic `+E164:password` (`api.md` §2.2), with
`User-Agent` and `X-Signal-Agent`.

### 2.1 Create: `POST /v1/verification/session`

Body (null members omitted; confirmed by R5):

    {"number":"+15555550100","mcc":"","mnc":""}

- `number` is the account's E.164 number.
- signal-cli passes **empty strings** for `mcc` and `mnc`, so they are
  present and empty.
- `pushToken` and `pushTokenType` are omitted, since signal-cli has no FCM
  token. (`pushTokenType` would be `"fcm"` if a token were given.)

### 2.2 Status: `GET /v1/verification/session/{sessionId}`

No body. `sessionId` is the opaque string from the session metadata,
inserted into the path as is.

### 2.3 Captcha: `PATCH /v1/verification/session/{sessionId}`

Body (null members omitted):

    {"captcha":"<token>"}

The token is whatever the user passed with `--captcha`, after removing
every occurrence of the literal `signalcaptcha://`. Push challenges are
never answered: signal-cli has no FCM, and never sends `pushChallenge`,
`pushToken`, `mcc` or `mnc` in a PATCH.

### 2.4 Session metadata (response of all four session calls)

The response JSON members signal-cli reads:

| Member | Type | Use |
|--------|------|-----|
| `id` | string | session ID; stored with the account, keyed by number |
| `nextSms` | integer seconds or null | delay before an SMS may be requested; null means not available |
| `nextCall` | integer seconds or null | the same for voice |
| `nextVerificationAttempt` | integer seconds or null | delay before a code may be submitted |
| `allowedToRequestCode` | boolean | |
| `requestedInformation` | array of strings | may contain `"captcha"` and/or `"pushChallenge"` |
| `verified` | boolean | |

The `Retry-After` header, if present and numeric, is recorded alongside
the metadata, but nothing in signal-cli's registration flow acts on it.

### 2.5 The session algorithm (`register`)

Before touching the session, `register` does the following:

- It refuses if the local account is already registered. `--reregister`
  skips the refusal and the reactivation step below.
- If the account already has an ACI, it first tries to *reactivate*: a
  `PUT /v1/accounts/attributes` over the authenticated WebSocket with the
  stored credentials. If that succeeds, no verification is needed. A fresh
  account never takes this path.
- If a PIN-derived registration-recovery password exists, it tries
  `/v1/registration` with `recoveryPassword` instead of a session. A fresh
  account has none.

Then the session steps:

1. **Get a session.** If a session ID is stored for this number, it calls
   `GET` status. If no ID is stored, or the server answers 404, it creates
   one with `POST`. If the status or create call answers 409 with
   `requestedInformation` containing `captcha` or `pushChallenge`, it
   either submits the user's captcha (PATCH) or fails with "captcha
   required".
2. Store the session ID.
3. If `verified` is true, or `allowedToRequestCode` is true, go to the
   code request (§3).
4. Otherwise take `nextSms` (or `nextCall` for `--voice`):
   - If it is null, fail with "verification method not available". For
     voice, the message adds that an SMS must be requested first and a
     minute waited.
   - If it is greater than 0, fail with a rate-limit error of that many
     seconds.
5. If `nextVerificationAttempt` is greater than 0, fail with "captcha
   required" carrying that delay.
6. If `requestedInformation` contains `captcha`, submit the captcha if one
   was given. If `allowedToRequestCode` is still false afterwards, fail
   with "captcha required".
7. Go to §3.

Captcha submission errors:

- 403, or a 409 that still asks for a challenge, gives "Captcha not
  accepted".
- 400 gives "Captcha has invalid format".

In practice a fresh staging number needs a captcha (from
`https://signalcaptchas.org/staging/registration/generate.html`; the URL
is quoted from signal-cli's man page `man/signal-cli.1.adoc` at v0.14.7 and was not
fetched here).

## 3. Requesting the code: `POST /v1/verification/session/{sessionId}/code`

Headers: as §2, plus `Accept-Language: <lang>-<COUNTRY>` (`api.md` §2.3).

Body (a plain map, so key order is not fixed):

    {"transport":"sms","client":"android"}

- `transport` is `"sms"`, or `"voice"` with `--voice`.
- `client` is always `"android"`. signal-cli passes "no SMS retriever",
  which selects `"android"` rather than `"android-2021-03"`. The server
  classifies `"android"` as an Android client without FCM (R4).

The response is session metadata. On success `register` stores the
session and marks the account not registered. No code is submitted yet.

## 4. Verifying: `PUT …/code`, then `POST /v1/registration`

`verify CODE [--pin PIN]` does the following:

- It requires a stored session ID, failing with "No registration
  verification session active" otherwise.
- It removes every `-` from the code.
- It generates a PNI identity key pair if missing.
- It generates, for **each** of ACI and PNI, using that identity's
  private key:
  - one **signed EC prekey** with ID = the identity's "next signed prekey
    ID" counter, timestamp = now, and an XEd25519 signature over the
    33-byte serialized public key;
  - one **last-resort Kyber-1024 prekey** with ID = the identity's "next
    Kyber ID" counter, timestamp = now, and an XEd25519 signature over the
    1569-byte serialized KEM public key (`spec/crypto/kem.md`).

  No one-time prekeys are generated at this stage (`keys.md` §3 uploads
  them right after).

### 4.1 Submit the code: `PUT /v1/verification/session/{sessionId}/code`

Body:

    {"code":"123456"}

Response handling:

- 200: success. Note that a **wrong code also returns 200** with
  `verified: false` (R4). signal-cli does not inspect the metadata here and
  goes on to `/v1/registration`, which then fails (see §6).
- 409 with `verified: true`: treated as already verified, so signal-cli
  continues to registration.
- 409 with `verified: false` and `nextVerificationAttempt` null: fails with
  "Verification code expired, please request a new one by registering
  again."
- Other codes: §6.

### 4.2 Register: `POST /v1/registration`

REST, Basic `+E164:password`. The body is the registration envelope (null
members **omitted**) wrapping `accountAttributes` (null members
**emitted**). Shape confirmed with the reference serializer (R5); values
illustrative:

    {"sessionId":"<id>",
     "accountAttributes":{
        "signalingKey":null,
        "registrationId":1234,
        "voice":false,
        "video":false,
        "fetchesMessages":true,
        "registrationLock":null,
        "unidentifiedAccessKey":"<24 chars>",
        "unrestrictedUnidentifiedAccess":false,
        "discoverableByPhoneNumber":true,
        "capabilities":{"storage":true,"versionedExpirationTimer":true,
                        "attachmentBackfill":false,"spqr":true,
                        "usernameChangeSyncMessage":false},
        "name":null,
        "pniRegistrationId":5678,
        "recoveryPassword":null},
     "aciIdentityKey":"<b64>",
     "pniIdentityKey":"<b64>",
     "aciSignedPreKey":{"keyId":N,"publicKey":"<b64>","signature":"<b64>"},
     "pniSignedPreKey":{"keyId":N,"publicKey":"<b64>","signature":"<b64>"},
     "aciPqLastResortPreKey":{"keyId":N,"publicKey":"<b64>","signature":"<b64>"},
     "pniPqLastResortPreKey":{"keyId":N,"publicKey":"<b64>","signature":"<b64>"},
     "skipDeviceTransfer":true,
     "requireAtomic":true}

(Line breaks are for reading only. The real body is compact JSON with no
whitespace, in exactly this key order.)

Field details:

| Field | Value in signal-cli 0.14.7 |
|-------|---------------------------|
| `sessionId` | the verified session ID. `recoveryPassword` (top level) is omitted, being mutually exclusive with `sessionId`. |
| `gcmToken` | omitted, because `fetchesMessages` is true |
| `aciIdentityKey`, `pniIdentityKey` | 33-byte serialized identity public keys (`0x05` prefix), standard base64 **without** padding (44 characters) |
| `*SignedPreKey.keyId` | the signed prekey ID (JSON number) |
| `*SignedPreKey.publicKey` | 33-byte serialized EC public key, unpadded standard base64 |
| `*SignedPreKey.signature` | 64-byte XEd25519 signature, unpadded standard base64 |
| `*PqLastResortPreKey.publicKey` | 1569-byte serialized Kyber-1024 public key (`0x08` prefix), unpadded standard base64 |
| `*PqLastResortPreKey.signature` | 64 bytes, unpadded standard base64 |
| `skipDeviceTransfer`, `requireAtomic` | always true |

Account-attribute details (R1 `SignalAccount.getAccountAttributes`,
`ServiceConfig.getCapabilities`):

| Member | Value for a fresh primary |
|--------|---------------------------|
| `signalingKey` | null |
| `registrationId` | ACI registration ID |
| `voice`, `video` | **false**. signal-cli uses the full constructor and passes false; the shorter constructor that defaults them to true is not used. |
| `fetchesMessages` | true |
| `registrationLock` | null. It is only set when a PIN-derived master key exists, or a lock value is being supplied after a 423 (§5); it is then 64 hex characters. |
| `unidentifiedAccessKey` | UAK of the account's own profile key (`api.md` §5.1), standard base64 **with** padding |
| `unrestrictedUnidentifiedAccess` | false unless the user enabled it locally |
| `discoverableByPhoneNumber` | true unless the user made the number unlisted |
| `capabilities` | primary: `storage` true, `versionedExpirationTimer` true, `attachmentBackfill` false, `spqr` true, `usernameChangeSyncMessage` false. A linked device would send `attachmentBackfill` and `usernameChangeSyncMessage` true. |
| `name` | null for a primary (the encrypted device name is cleared at registration) |
| `pniRegistrationId` | PNI registration ID |
| `recoveryPassword` | null without a PIN |

### 4.3 Response

The response JSON members signal-cli reads:

- `uuid` (the ACI), required;
- `pni`, required;
- `storageCapable` (boolean);
- `number`;
- `reregistration` (boolean).

Unknown members (`usernameHash`, `usernameLinkHandle`, `entitlements`,
`authCredentialSalt` in R4's server) are ignored. A missing or unparsable
`uuid` or `pni` aborts.

## 5. Registration lock (only what a fresh account needs)

A brand-new number has no lock, so 423 does not occur. If it does (the
number was registered before with a PIN):

- Without `--pin`, `verify` fails with "PIN locked", carrying
  `timeRemaining`.
- With `--pin`, signal-cli recovers the master key through SVR2, using the
  credentials in the 423 body. It derives the 64-hex-character
  registration-lock value and repeats `/v1/registration` with it in
  `accountAttributes.registrationLock`. The SVR2 part is out of scope for
  milestone 4.

## 6. Error handling

These are call-specific handlers that run before the generic REST mapping
in `api.md` §4.1. "Challenge" means `requestedInformation` contains
`captcha` or `pushChallenge`.

| Call | Status | Outcome in signal-cli |
|------|--------|-----------------------|
| create, status, `/v1/registration` | 400 | invalid session ID |
| | 403 | incorrect recovery password |
| | 404 | no such session. For status, signal-cli then creates a new session. |
| | 409 | verified → already verified. Challenge → challenge required (captcha). Otherwise a conflict error. |
| | 502 | external service failure; body `{permanentFailure, reason}` |
| code request | 400 | malformed request |
| | 403 | incorrect recovery password |
| | 404 | no such session |
| | 409 | as above; a challenge becomes "captcha required" |
| | 418 | transport not allowed (for example voice before SMS) |
| | 429 | rate limited; body is session metadata. Surfaced as a failure, no retry. |
| | 440 | external service (SMS provider) failure; body `{permanentFailure, reason}` |
| captcha PATCH | 403 | token not accepted, shown as "Captcha not accepted" |
| | 404 | no such session |
| | 409 | as above |
| code submit | 400 | invalid transport mode |
| | 404 | no such session |
| | 409 | see §4.1 |
| | 429 | rate limited; body is session metadata |
| | 440 | external service failure |
| any | 401 | authorization failed. This is how `/v1/registration` reports an unverified session (for example after a wrong code, R4). |
| any | 413, 429 (if not handled above) | rate limit, with `Retry-After` seconds |
| any | 423 | registration lock (§5) |
| any | 499 | deprecated version |

Server-side codes in R4 that signal-cli 0.14.7 does not special-case on
`/v1/registration`:

- 422: validation failed;
- 441: second-factor failure;
- 499, which R4 documents as "client must support post-quantum ratchet"
  and signal-cli maps to "deprecated version".

All of these surface as failures. signal-cli always declares `spqr: true`.

No registration step is retried automatically. The CLI exits with the
error, and the user re-runs `register` or `verify`.

## 7. After `/v1/registration` succeeds

In order (R1 `RegistrationManagerImpl.finishAccountRegistration`,
`SignalAccount.finishRegistration`):

1. **Local state.** Store the ACI and PNI, set `registered` = true and
   device ID 1, and clear any storage key, manifest and device name. Store
   the signed prekey and last-resort Kyber prekey for each identity as
   active. Each counter becomes the used ID + 1, modulo 16777215. Archive
   all sessions, delete sender keys, trust the account's own identity
   keys, and clear the stored session ID.
2. **Prekey refresh** for ACI then PNI (`keys.md` §3). On a fresh account
   this uploads 100 one-time EC and 100 one-time Kyber prekeys per
   identity. The signed and last-resort keys are not re-uploaded, since
   they are fresh.
3. If the response had `storageCapable: true`, a storage-service sync job
   is queued (storage service, out of scope).
4. **Initial profile upload**: an empty versioned profile (`profile.md`
   §2).
5. **Username refresh**: does nothing, because a fresh account has no
   local username.

The account-check sequence (`api.md` §4.3) then runs on every later
command.

## Gaps

- The server's exact session metadata for a fresh staging number (whether
  `nextSms` is 0 or null while a captcha is outstanding) was not observed.
  The algorithm in §2.5 is what signal-cli does with whatever it receives.
- Staging's server build may differ from R4. Status codes quoted from R4
  that signal-cli does not handle are informational only.
