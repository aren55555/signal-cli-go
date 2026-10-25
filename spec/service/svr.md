# SVR2, registration lock and re-registration (signal-cli 0.14.7, staging)

Spec role: prose only; no code. This file covers everything signal-cli
0.14.7 does with Secure Value Recovery 2 (SVR2) and the registration lock:

- the chat-server endpoints involved;
- the messages exchanged with the SVR2 enclave inside the attested
  channel;
- the `setPin` and `removePin` flows;
- `verify` against a registration-locked number;
- reactivation and recovery-password re-registration in `register`
  (deferred from `registration.md` §2.5 and §5);
- outputs, errors and exit codes.

The key material (account entropy pool, master key, PIN normalization,
Argon2id, master-key encryption) is in `spec/crypto/pin.md`. The
WebSocket transport, SGX attestation and Noise channel are in
`spec/enclave/attestation.md` and `spec/enclave/noise.md`; this file
gives only the SVR2-specific parameters for them. Conventions for chat
requests (WebSocket vs REST, authentication, JSON, status-code mapping)
are in `api.md`; the registration request body is in `registration.md`.

## Sources

| Ref | What | URL @ version |
|-----|------|---------------|
| S1 | signal-cli | https://github.com/AsamK/signal-cli @ v0.14.7 (b01b6b370dc063599a1a2b9fde0f5ff4e2d78fe8): `lib/src/main/java/org/asamk/signal/manager/helper/{PinHelper,AccountHelper,StorageHelper}.java`, `.../manager/internal/{RegistrationManagerImpl,ManagerImpl,SignalDependencies}.java`, `.../manager/util/{NumberVerificationUtils,Utils}.java`, `.../manager/storage/SignalAccount.java`, `.../manager/SignalAccountFiles.java`, `.../manager/config/{StagingConfig,LiveConfig,ServiceConfig}.java`, `.../manager/api/{IncorrectPinException,PinLockedException,PinLockMissingException}.java`, `src/main/java/org/asamk/signal/{Main.java,commands/SetPinCommand.java,commands/RemovePinCommand.java,commands/VerifyCommand.java,commands/RegisterCommand.java,commands/FinishChangeNumberCommand.java}` |
| S2 | signal-service-java (Turasa fork) | https://github.com/Turasa/libsignal-service-java @ v2.15.3_unofficial_151 (66956cb4a0394d50cee3590209baf58312084a44): `lib/libsignal-service/src/main/java/org/whispersystems/signalservice/api/svr/{SecureValueRecovery,SecureValueRecoveryV2,Svr2Socket,SecureValueRecoveryV3}.kt`, `lib/libsignal-service/src/main/protowire/SVR2.proto` (field numbers only), `.../api/kbs/PinHashUtil.kt`, `.../api/account/AccountApi.kt`, `.../api/registration/RegistrationApi.kt`, `.../api/SignalUrlExtensions.kt`, `.../internal/push/{PushServiceSocket.java,LockedException.java,AuthCredentials.java}`, `core/network/src/main/java/org/signal/network/exceptions/NonSuccessfulResponseCodeException.kt` |
| S3 | libsignal | https://github.com/signalapp/libsignal @ v0.99.1 (97801d22dcf9f5bf714f7b8fa3212cdc973ae1c8): `rust/attest/src/{svr2.rs,constants.rs}`, `rust/net/src/proto/svr2.proto` (field numbers only), `rust/bridge/shared/src/{svr2.rs,account_keys.rs}`, `java/client/src/main/java/org/signal/libsignal/svr2/{PinHash,Svr2Client}.java` |
| S4 | Signal-Server (server behaviour only) | https://github.com/signalapp/Signal-Server @ v20261022.0.0 (8edf3488f7d97398db7ffa5184c6b769a272ff59): `service/src/main/java/org/whispersystems/textsecuregcm/controllers/{SecureValueRecovery2Controller,AccountController,RegistrationController}.java`, `.../auth/{RegistrationLockVerificationManager,StoredRegistrationLock,ExternalServiceCredentialsGenerator,PhoneVerificationTokenManager}.java`, `.../entities/{RegistrationLock,RegistrationLockFailure,AccountAttributes}.java`, `.../storage/{Account,AccountsManager}.java` |
| S5 | SecureValueRecovery2 (server behaviour only) | https://github.com/signalapp/SecureValueRecovery2 @ v1.26.0 (42a3069a413ea9ca8340744fed7f3fb32337ee35): `shared/proto/client.proto` (field numbers and status values only), `enclave/db/db2.cc`, `host/web/handlers/websocket.go`, `host/auth/auth.go`. The staging enclave builds signal-cli uses are newer than this tag (Gap 1). |
| S6 | Reference distribution, offline | `jshell` over `/work/third_party/signal-cli-0.14.7/lib/*.jar`, no network: SVR2 message encodings with the bundled Wire classes; JSON shapes of `RegistrationLockV2`, `RegistrationLockFailure` parsing and `RegistrationSessionRequestBody` with a recovery password; `AuthCredentials` Basic header |

## 1. Which SVR versions signal-cli 0.14.7 uses

- **SVR2 only.** The service configuration lists two SVR2 enclaves per
  environment, current and legacy (§3.1). Every SVR operation goes to
  both, in that order.
- **SVR3 and SVR-B are not used.** The SVR3 client class exists in
  signal-service-java but signal-cli never builds one. A 423 body's
  `svr3Credentials` member is read into memory and ignored, as is the
  old `backupCredentials` (SVR1) member. SVR-B (backups) is never
  contacted; signal-cli makes no message backups.
- Not used either: the legacy `PUT /v1/accounts/pin` and the
  `POST /v2/svr/auth/check` / `POST /v3/backup/auth/check` credential
  checks.

## 2. Chat-server endpoints

All requests in §2.1 and §2.2 go over the **authenticated chat WebSocket**
(`api.md`), with the account's ACI credentials.

### 2.1 `GET /v2/svr/auth`

No body. Success is 200 with JSON:

| Member | Type | Content |
|--------|------|---------|
| `username` | string | 32 lowercase hex characters (16 bytes). The server derives it from the ACI with a keyed HMAC truncated to 16 bytes, so it is stable per account. |
| `password` | string | `<unix seconds>:<20 lowercase hex characters>` (a truncated HMAC over `username:seconds`). |

The SVR2 host accepts these credentials for 30 days (S5 `auth.go`).
signal-cli fetches a new pair for every enclave operation and never caches
them. (The server also serves the same endpoint as `/v2/backup/auth`;
signal-cli uses `/v2/svr/auth`.)

### 2.2 Registration-lock endpoints

| Request | Body | Success |
|---------|------|---------|
| `PUT /v1/accounts/registration_lock` | `{"registrationLock":"<64 lowercase hex>"}`, the registration lock token of the master key (`pin.md` §3) | 204 |
| `DELETE /v1/accounts/registration_lock` | none | 204 |

The server stores a salted hash of the token. It rejects a token that is
not exactly 64 characters as a validation error (S4 `RegistrationLock`).

### 2.3 Account attributes and the lock (server behaviour)

On every `PUT /v1/accounts/attributes` (and at account creation), the
server applies the attributes' `registrationLock`:

- non-empty: the lock is set to that token, replacing any previous one;
- null or empty: the lock is **removed**.

If `recoveryPassword` (32 bytes, base64) is present, it is stored as the
registration recovery password for the account's phone-number identity.

signal-cli always sends these members from the PIN-backed master key
(`pin.md` §4.2). So the server-side lock follows the local
`registrationLockPin` on the next account check, whatever
`PUT`/`DELETE /v1/accounts/registration_lock` did before.

### 2.4 The 423 response of `POST /v1/registration`

If the number's existing account has a registration lock and the lock is
in force, `POST /v1/registration` with a wrong or missing
`accountAttributes.registrationLock` answers **423** with JSON:

| Member | Type | Content |
|--------|------|---------|
| `timeRemaining` | integer (ms) | 7 days minus the time since the existing account was last seen. |
| `svr2Credentials` | object or null | `{"username": …, "password": …}`, the same form as §2.1, for the **existing** account. Null only if the account has no lock. |

signal-cli reads `timeRemaining` and `svr2Credentials`, plus `length`,
`backupCredentials` and `svr3Credentials`, which it ignores. Unknown
members are ignored.

When a registration lock is in force, the server does this (S4
`RegistrationLockVerificationManager`):

- The lock is in force only if it is set **and** the account was seen less
  than 7 days ago. If not, registration simply proceeds.
- A request carrying a non-empty lock token counts against a per-number
  PIN rate limiter (413/429 when exceeded).
- On a mismatch, the server first locks the existing account's
  credentials, so its devices can no longer authenticate, and disconnects
  them. It also deletes the stored recovery password (unless the request
  was verified by recovery password and carried no token at all). Only
  then does it answer 423.
- A correct token clears the PIN rate limiter, and registration proceeds.

## 3. The SVR2 enclave channel

### 3.1 Endpoints and credentials

| Item | Value |
|------|-------|
| Base URL (staging) | `https://svr2.staging.signal.org` (live: `https://svr2.signal.org`) |
| Connection URL | `wss://svr2.staging.signal.org/v1/<MRENCLAVE as 64 lowercase hex>` |
| Enclaves, in the order signal-cli uses them (staging) | 1. current `3c699f4975aaa3d172c0aad042f94f031b2b03e10b9c19a45116a01693d83302`; 2. legacy `97f151f6ed078edbbfd72fa9cae694dcc08353f1f5e8d9ccd79a971b10ffc535` |
| Enclaves (live, for reference) | 1. `ced8217b26228e4b210c985786999d095c4958a94faf37b14acaf25c4cbb02a4`; 2. `1240acbd4aa26974184844c8a46b1022d3957ac8a76c1fd8f5b1a15141ee0708` |
| HTTP headers on the upgrade | `Authorization: Basic base64(username:password)` with the §2.1 credentials (or the 423 body's `svr2Credentials`); `User-Agent` = signal-cli's agent string (`transport.md` §4.2). No `X-Signal-Agent`, no `Host` override. |
| TLS | OkHttp with signal-cli's truststore, TLS 1.3 and 1.2 only (`RESTRICTED_TLS`), as in `transport.md` §5 |
| Timeouts | connect 30 s, read 30 s. `retryOnConnectionFailure` is **false** (unlike chat REST). |
| Frames | binary WebSocket frames only, no compression |
| Connections | **one fresh WebSocket per request**: backup, expose, restore and delete each open their own connection, carry one request, and close it |

SVR2-specific attestation parameters for `attestation.md`. libsignal
validates the attested Raft group configuration against a built-in table
keyed by MRENCLAVE (S3 `constants.rs`). All fields must match exactly:

| Enclave | min_voting_replicas | max_voting_replicas | super_majority | group_id | db_version | attestation_timeout | simulated |
|---------|---|---|---|---|---|---|---|
| staging current `3c699f49…` | 3 | 9 | 0 | 5138641357881452604 | 2 | 604800 | false |
| staging legacy `97f151f6…` | 3 | 9 | 0 | 2330628069874851020 | 2 | 604800 | false |
| live current `ced8217b…` | 4 | 13 | 2 | 11311619198250676136 | 2 | 604800 | false |
| live legacy `1240acbd…` | 4 | 13 | 2 | 2076725645304009823 | 2 | 604800 | false |

- Acceptable Intel SW advisories for all four: `INTEL-SA-00615`,
  `INTEL-SA-00657`.
- libsignal uses its **post-quantum** handshake variant for SVR2.
- The attestation is evaluated at the current wall-clock time.
- An MRENCLAVE outside the table fails with an attestation-data error.

The `group_id` column is also the PIN salt input (`pin.md` §6.2). The
same constants are in `testdata/vectors/svr2.json` → `enclaves`.

### 3.2 Connection lifecycle

1. Open the WebSocket (§3.1).
2. The server's first binary frame is the attestation handshake start
   (`attestation.md`). The client validates it and sends its Noise
   handshake message (`noise.md`).
3. The server's second frame completes the handshake. The client then
   sends **one** Noise-encrypted frame carrying a serialized `Request`
   (§3.3).
4. The server's third frame is one Noise-encrypted `Response`. On receipt
   the client closes with code 1000, reason `OK`.

Error handling in signal-service-java (S2 `Svr2Socket`):

| Event | Result for the operation |
|-------|--------------------------|
| WebSocket upgrade fails and the failure text contains `404` | status 404: "enclave not found" |
| upgrade fails and the text contains `429` | status 429: "rate limited" |
| any other connect or upgrade failure (DNS, TLS, 401, 400, …) | network error wrapping the cause |
| a message arrives before the socket opened | network error `Received a message before we were open!` |
| attestation data invalid | client closes with 1007 `OK`; network error wrapping it |
| SGX or Noise communication failure | client closes with 1000 `OK`; network error wrapping it |
| server closes with 1000 before the response arrived | network error `Websocket was closed with code 1000` |
| server closes with any other code N | client answers with close 1000 `Remote closed with code N`; status error N. The text is `StatusCode: N`. |

SVR2 host close codes (S5 `websocket.go`):

| Code | Meaning |
|------|---------|
| 1000 | normal, after the response |
| 4003 | bad arguments, e.g. data size, PIN size, `max_tries` 0 or above 255 |
| 4013 | internal error |
| 4014 | unavailable, transient (leader unknown, transaction cancelled) |

The host rejects an `Authorization` username that is not 32 hex
characters (16 bytes) with HTTP 400 before upgrading.

### 3.3 Messages (inside the channel)

Protobuf 3 encodings; field numbers are wire facts. Names are descriptive.

**Request** (exactly one of the inner fields is set):

| # | Name | Type | Used by signal-cli |
|---|------|------|--------------------|
| 1 | (reserved; backup ID, server-internal) | — | never sent |
| 2 | backup | BackupRequest | yes |
| 3 | restore | RestoreRequest | yes |
| 4 | delete | DeleteRequest | yes |
| 5 | expose | ExposeRequest | yes |
| 6 | tries | TriesRequest (empty) | no; it is not in signal-cli's schema at all |

**Response** (one inner field):

| # | Name | Type |
|---|------|------|
| 1 | backup | BackupResponse |
| 2 | restore | RestoreResponse |
| 3 | delete | DeleteResponse (empty) |
| 4 | expose | ExposeResponse |
| 5 | tries | TriesResponse: status 1 (enum UNSET 0, OK 1, MISSING 2), tries 2 (uint32), exposed 3 (bool). Unknown to signal-cli; skipped. |

**BackupRequest**

| # | Name | Type | Value from signal-cli |
|---|------|------|-----------------------|
| 1 | data | bytes | the 48-byte encrypted master key (`pin.md` §7). The server accepts 16..48 bytes. |
| 2 | pin | bytes | the 32-byte access key (`pin.md` §6) |
| 3 | max_tries | uint32 | **10**. The server accepts 1..255. |

**BackupResponse**: field 1 `status`, enum: 0 UNSET (never sent), 1 OK.
signal-cli's schema also names value 2 REQUEST_INVALID, which the S5
server never sends.

**ExposeRequest**: field 1 `data` (bytes), the same 48-byte blob as in
the backup.

**ExposeResponse**: field 1 `status`, enum: 0 UNSET, 1 OK, 2 ERROR.

**RestoreRequest**: field 1 `pin` (bytes), the 32-byte access key.

**RestoreResponse**

| # | Name | Type | Notes |
|---|------|------|-------|
| 1 | status | enum | 0 UNSET, 1 OK, 2 MISSING, 3 PIN_MISMATCH, 4 ERROR (signal-cli's schema calls value 4 REQUEST_INVALID) |
| 2 | data | bytes | the stored blob when status is OK |
| 3 | tries | uint32 | remaining tries (0..255) for OK and PIN_MISMATCH |

**DeleteRequest** and **DeleteResponse** are empty messages.

Encoded examples (`svr2.json` → `requests`, `responses`): a delete request
is `22 00`; a restore request is `1a 22 0a 20 <32-byte pin>`; a
PIN_MISMATCH response with 9 tries is `12 04 08 03 18 09`.

Enclave database semantics (S5 `db2.cc`, server behaviour). The row key
is the authenticated username, so it is per account and per enclave.

- **Backup** creates or overwrites the row. It sets data, PIN and
  `tries = max_tries`, and the state "populated, not restorable". It
  always answers OK.
- **Expose** answers ERROR if the row is missing or its data differs from
  the request's `data` (compared over the stored length). Otherwise it
  makes the row restorable and answers OK. Repeating it is harmless.
- **Restore**:
  - missing row, or a row not yet exposed: MISSING;
  - integrity failure inside the enclave: ERROR;
  - PIN equal (constant time): OK with data and the current tries. A
    successful restore does **not** reset the counter.
  - PIN different: tries goes down by 1. If it reaches 0, the row is
    deleted and the answer is MISSING; otherwise PIN_MISMATCH with the
    new tries.
- **Delete** removes the row if present and always answers with an empty
  delete response.

Backup followed by expose is a two-phase commit. Exposing needs the data
itself, and the backup is not restorable until exposed. This stops an
operator who drops the backup reply from guessing PINs and then letting
the client's retry reset the counter. signal-service-java never repeats
the backup once it has succeeded within one operation.

## 4. signal-cli flows

"For each enclave" means the two enclaves of §3.1 in order, current then
legacy, one after the other (not in parallel).

### 4.1 `setPin PIN`

Preconditions: a registered account; the command is refused on a linked
device (`This command doesn't work on linked devices.`, exit 1) before
anything is sent. The normal account check (`api.md` §4.3) runs first, as
for every local command.

1. Master key M := the get-or-create master key (`pin.md` §4.2). This may
   generate and store an AEP.
2. For each enclave:
   1. `GET /v2/svr/auth` → credentials (U, P).
   2. Hash the normalized PIN with salt(U, enclave) (`pin.md` §6). Encrypt
      M into blob B (`pin.md` §7).
   3. Connection 1: send `Request{backup{data=B, pin=access key,
      max_tries=10}}`. The response must carry a backup response.
   4. If its status is OK, connection 2: send `Request{expose{data=B}}`.
   5. The enclave's outcome:

      | Outcome | Condition |
      |---------|-----------|
      | **success** | expose OK |
      | expose failure | expose ERROR. Log `WARN  PinHelper - Backup svr failed: ExposeFailure`. |
      | enclave not found | status 404 on auth or upgrade. Log `WARN  PinHelper - Backup svr failed: EnclaveNotFound`. |
      | rate limited | status 429 on auth or upgrade. signal-cli does not handle this case: it raises an internal assertion error, so the command ends with a stack trace on stderr and **exit 2** (`Main`). |
      | application error | backup status not OK (UNSET, or value 2), a response without a backup or expose part, a status error other than 404/429 (including enclave close codes), or any other exception. Recorded as an I/O error whose message is the cause's Java `toString()` (see §5.1). |
      | network error | an I/O failure. Recorded as an I/O error. |

      An I/O error from one enclave does not stop the loop.
3. If no enclave succeeded **and** an I/O error was recorded, the command
   fails with the **last** recorded error (§5). Nothing below happens.
   If no enclave succeeded but none recorded an I/O error (both "expose
   failure" or "enclave not found"), signal-cli **continues as if it had
   succeeded**.
4. `PUT /v1/accounts/registration_lock` with M's registration lock token.
   A failure here fails the command (§5) after the SVR2 data has been
   written.
5. Store `registrationLockPin` := PIN (raw string) and save the account
   file.
6. `PUT /v1/accounts/attributes` with the now non-null `registrationLock`
   and `recoveryPassword` (`registration.md` §4.2 body).
7. Exit 0, no output.

Traffic for one successful run on staging: 2 auth GETs, 4 enclave
WebSockets, 1 lock PUT, 1 attributes PUT (plus the start-up account
check).

### 4.2 `removePin`

Same preconditions (linked device refused, exit 1).

1. For each enclave: `GET /v2/svr/auth`, then one connection sending
   `Request{delete{}}`.
   - Any decoded response counts as success, whatever its content.
   - 404 on auth or upgrade: log
     `WARN  PinHelper - Delete svr2 failed: EnclaveNotFound`.
   - Another status error or exception: an application error, recorded as
     an I/O error with the cause's `toString()` as message.
   - An I/O failure: recorded as an I/O error.
2. If no enclave succeeded and an I/O error was recorded, fail with the
   last one (§5).
3. `DELETE /v1/accounts/registration_lock`. A failure fails the command.
4. `registrationLockPin` := null, save. The AEP and `pinMasterKey` are
   kept. Account attributes are **not** uploaded here; the next account
   check uploads them with `registrationLock` and `recoveryPassword` null
   (§2.3).
5. Exit 0, no output.

### 4.3 `verify CODE [--pin PIN]` against a locked number

The normal path is `registration.md` §4. Lock-specific behaviour (S1
`NumberVerificationUtils.verifyNumber`, `RegistrationManagerImpl`):

1. First attempt, exactly as `registration.md` §4.1–4.2. Its
   `accountAttributes.registrationLock` and `recoveryPassword` come from
   the local PIN-backed master key. A brand-new account file has none, so
   both are null. An old account file that still holds a PIN sends that
   PIN's token and recovery password, which may satisfy the lock straight
   away.
2. If `POST /v1/registration` answers anything but 423, continue as in
   `registration.md`.
3. On **423** without `--pin`: fail with the "locked" message carrying
   `timeRemaining` (§5.2). Nothing else is sent.
4. On **423** with `--pin`:
   1. If `svr2Credentials` is null: fail with "pin data deleted" (§5.2).
   2. For each enclave, using the 423 body's credentials (U, P) for the
      upgrade's Basic header. No `/v2/svr/auth` call is made; the account
      is not authenticated yet.
      1. Hash the normalized PIN with salt(U, enclave).
      2. One connection sending `Request{restore{pin=access key}}`.
      3. The outcome:

         | Outcome | Condition | Next step |
         |---------|-----------|-----------|
         | **OK** | — | decrypt `data` with the encryption key (`pin.md` §7). If that succeeds, M is restored: stop the loop. A decryption failure is an application error. |
         | MISSING | — | log at debug, try the next enclave |
         | PIN_MISMATCH with tries N | — | fail **at once** with "invalid pin, tries remaining: N" (§5.2); the other enclave is not tried |
         | application error | status 4, UNSET, no restore part, a status error other than 404, other exceptions | recorded as an I/O error; next enclave |
         | network error | — | recorded; next enclave |
         | enclave not found (404) | — | not handled by signal-cli: internal assertion error, stack trace, **exit 2** |
   3. After the loop without a restored key: if an I/O error was recorded,
      fail with the last one (`Verify error: …`, exit 3). Otherwise fail
      with "pin data deleted" (§5.2).
   4. Second attempt with the restored M:
      - `PUT /v1/verification/session/{id}/code` again. "Already verified"
        (409 with `verified` true) is tolerated.
      - Then `POST /v1/registration` with the **same** prekeys as the
        first attempt and `accountAttributes.registrationLock` = M's
        token (lowercase hex). `recoveryPassword` still comes from the
        local PIN-backed key, normally null.
   5. If this second attempt answers 423 again, signal-cli raises an
      internal assertion (`KBS Pin appeared to matched but reg lock still
      failed!`): stack trace, exit 2. Other errors are handled as in
      `registration.md` §6.
5. On success, local state as in `registration.md` §7, with
   `pinMasterKey` := M (or none) and `registrationLockPin` := PIN. The
   PIN is stored only if a master key was restored. A `--pin` given when
   no 423 occurred is ignored and not stored, and any older local PIN is
   dropped (`pin.md` §4.3). Note that `pin.md` §4.3 also describes the
   later storage-key migration that replaces M.

Each wrong PIN costs one enclave try. After 10 wrong PINs the enclave
deletes the backup, and from then on the answer is "pin data deleted".
The server-side lock still applies until `timeRemaining` runs out.

### 4.4 `register`: reactivation and recovery-password re-registration

Full order of `register [--voice] [--captcha C] [--reregister]` (S1
`RegistrationManagerImpl.register`). It extends `registration.md` §2.5.

1. If the local account is marked registered and its stored environment
   differs from the selected one: fail
   `Account is registered in another environment: <ENV>`, where ENV is
   `LIVE` or `STAGING` (§5.3).
2. Unless `--reregister`:
   1. If the local account is registered: fail
      `Account is already registered` (§5.3).
   2. If the account file has an ACI, try **reactivation** (§4.4.1). On
      success, stop.
3. If the PIN-backed master key gives a recovery password (that is,
   `registrationLockPin` is set) **and** this is a primary device, try
   **recovery-password re-registration** (§4.4.2). On success, stop. Note
   that computing the PIN-backed key on a primary may generate and store
   an AEP as a side effect (`pin.md` §4.2).
4. Otherwise run the verification-session flow of `registration.md` §2.5
   steps 1–7 and §3, then set `registered` := false locally.

Both attempts reuse the existing account file, including its password
(`registration.md` §1 applies only to a new file).

#### 4.4.1 Reactivation

When it applies: a local account that has an ACI but is marked not
registered. That happens after `unregister`, after an account check got
401/403 (`api.md` §4.3), or after an earlier failed registration attempt
on an old file.

1. Connect the **authenticated** chat WebSocket with the stored
   credentials (ACI, device ID, password; `api.md`). Send
   `PUT /v1/accounts/attributes` with the account's current attributes
   (`registration.md` §4.2 body, including `registrationLock` and
   `recoveryPassword` from the PIN-backed master key).
2. On success (2xx):
   - set `registered` := true and save;
   - log `INFO  RegistrationManagerImpl - Reactivated existing account,
     verify is not necessary.` (stderr; signal-cli's own loggers log INFO
     by default, `surface/cli/index.md`);
   - in daemon/JSON-RPC multi-account mode, the reactivated account is
     handed to the running process, which starts using it;
   - `register` ends with exit 0 and no stdout.

   Nothing else is sent at this point: no prekeys, no profile. The next
   command's account check does the usual attribute upload and prekey
   refresh (`api.md` §4.3, `keys.md`).
3. On any I/O failure (including 401/403 when the credentials are no
   longer valid, for example after re-registration elsewhere or a
   registration-lock credential lock): log at debug
   `Failed to reactivate account` and fall through to step 3 of §4.4.

Typical use: `unregister` clears the push token, so the server stops
delivering to the primary (`fetchesMessages` false). A later `register`
reactivates the account by sending `fetchesMessages: true` again.

#### 4.4.2 Recovery-password re-registration

1. If the account file has no PNI identity key pair, generate one.
2. Generate fresh prekey material for ACI and PNI: a signed prekey and a
   last-resort Kyber prekey each (`keys.md`; as in `registration.md` §4).
3. `POST /v1/registration` over REST with Basic `+E164:password` (the
   stored password). The body is the `registration.md` §4.2 envelope with
   these differences:
   - the top-level `recoveryPassword` (the PIN-backed master key's
     recovery password, base64 with padding) replaces `sessionId`. It is
     the **first** member, and `sessionId` is omitted;
   - `accountAttributes.registrationLock` = the PIN-backed key's token;
   - `accountAttributes.recoveryPassword` = the same recovery password.

   Shape (confirmed with the reference serializer, S6; values
   illustrative):

        {"recoveryPassword":"Ve3mYvM90mir28jVHNx80Bgl4c8wLbv9JPAUU5tfe0M=",
         "accountAttributes":{…,"registrationLock":"2bf79882…e3c6",…,
                              "recoveryPassword":"Ve3mYvM90mir28jVHNx80Bgl4c8wLbv9JPAUU5tfe0M="},
         "aciIdentityKey":…, "pniIdentityKey":…, "aciSignedPreKey":…,
         "pniSignedPreKey":…, "aciPqLastResortPreKey":…,
         "pniPqLastResortPreKey":…, "skipDeviceTransfer":true,
         "requireAtomic":true}

4. On success, do everything in `registration.md` §7 (local state with
   `pinMasterKey` := the PIN-backed key and `registrationLockPin` kept,
   prekey refresh, storage sync if `storageCapable`, empty profile
   upload, username refresh). Log `INFO  RegistrationManagerImpl -
   Reregistered existing account, verify is not necessary.` and end with
   exit 0, no stdout.
5. On any I/O failure, log at debug and fall back to the session flow.
   This covers 403 "incorrect recovery password", 423, 401, 429 and
   network errors.

Server side (S4): the recovery password is checked against the one
stored for the number's phone-number identity (§2.3). The registration
lock is checked as in §2.4.

### 4.5 Other users of the same machinery

- **`finishChangeNumber --pin`** uses the same lock handling as §4.3, via
  the change-number request (`PUT /v2/accounts/number` with a
  `registrationLock` member). Its messages are the same as `verify`'s,
  except that the generic I/O failure is
  `Failed to change number: <msg> (<ExceptionSimpleName>)`
  (`surface/cli/finishChangeNumber.md`). See Gap 4.
- **Account deletion** (`unregister --delete-account`): first a §4.2
  SVR2 delete on both enclaves. A failure only logs
  `WARN  AccountHelper - Failed to remove registration lock pin`. Then
  the local PIN is cleared and `DELETE /v1/accounts/me` follows.
- **Legacy migration**: during the account check, if the account file's
  previous storage version is below 10, the device is the primary, and a
  PIN is set, signal-cli re-runs §4.1 steps 1–4 with the stored PIN.
  Files written by 0.14.7 never trigger this.

## 5. Outputs, errors and exit codes

Exit codes (S1 `Main`): user error 1, unexpected error 2, I/O error 3;
an uncaught Java error or exception prints a stack trace to stderr and
exits 2. Messages go to stderr; nothing goes to stdout in any of these
flows.

### 5.1 How I/O error messages are built

- An application error from SVR2 is wrapped so that its message is the
  cause's Java `toString()`, i.e. `<fully.qualified.ClassName>: <message>`.
  Common values:

  | Cause | Message text |
  |-------|--------------|
  | enclave closed with code N | `org.signal.network.exceptions.NonSuccessfulResponseCodeException: StatusCode: N` |
  | backup status 2 | `org.whispersystems.signalservice.api.svr.SecureValueRecovery$InvalidRequestException: BackupResponse returned status code for REQUEST_INVALID` |
  | backup status UNSET | `java.lang.IllegalStateException: Unknown status: UNSET` |
  | response lacks the backup part (backup phase), or lacks the expose part or has expose status UNSET (expose phase) | `java.lang.IllegalStateException: Backup response not set!` |
  | restore status 4 | `org.whispersystems.signalservice.api.svr.SecureValueRecovery$InvalidRequestException: RestoreResponse returned status code for REQUEST_INVALID` |
  | restore status UNSET or no restore part | `java.lang.IllegalStateException: Unknown status: null` (signal-service-java prints the backup field here, which is absent) |
  | restore blob fails to decrypt | `org.whispersystems.signalservice.api.crypto.InvalidCiphertextException` followed by `: <detail>` if the exception has a message (Gap 3) |

- A network error is rethrown as is. Its message is the I/O exception's
  own message, e.g. `Websocket was closed with code 1000`, or for a
  wrapped connect failure the wrapped exception's `toString()` (for
  example `java.net.UnknownHostException: svr2.staging.signal.org`).
- Chat-request failures (`/v2/svr/auth`, the lock PUT/DELETE) carry the
  messages of `api.md` §4.

### 5.2 Messages

| Command | Condition | stderr (exact) | Exit |
|---------|-----------|----------------|------|
| `setPin` | linked device | `This command doesn't work on linked devices.` | 1 |
| `setPin` | §4.1 step 3 failure, or lock PUT / attributes PUT failure | `Set pin error: <message>` | 3 |
| `removePin` | linked device | `This command doesn't work on linked devices.` | 1 |
| `removePin` | §4.2 step 2 failure, or lock DELETE failure | `Remove pin error: <message>` | 3 |
| `verify` | 423 without `--pin` | two lines: `Verification failed! This number is locked with a pin. Hours remaining until reset: <H>` and `Use '--pin PIN_CODE' to specify the registration lock PIN`, where H = `timeRemaining` / 3,600,000 by integer division (604799000 ms → 167) | 1 |
| `verify` | PIN_MISMATCH | `Verification failed! Invalid pin, tries remaining: <N>` | 1 |
| `verify` | `svr2Credentials` null, or every enclave MISSING with no I/O error | `Account is pin locked, but pin data has been deleted on the server.` | 1 |
| `verify` | restore I/O error (last one), or any other I/O failure | `Verify error: <message>` | 3 |
| `verify` | enclave 404 during restore; second 423 after a successful restore | Java stack trace (`java.lang.AssertionError: …`) | 2 |
| `setPin` | enclave or auth 429 during backup | Java stack trace (`java.lang.AssertionError: Unexpected response`) | 2 |

Log lines (stderr, default verbosity, format of `surface/cli/index.md`):
`WARN  PinHelper - Backup svr failed: ExposeFailure`,
`WARN  PinHelper - Backup svr failed: EnclaveNotFound`,
`WARN  PinHelper - Delete svr2 failed: EnclaveNotFound`,
`INFO  RegistrationManagerImpl - Reactivated existing account, verify is not necessary.`,
`INFO  RegistrationManagerImpl - Reregistered existing account, verify is not necessary.`.

### 5.3 `register` failures added by this file

| Condition | stderr | Exit |
|-----------|--------|------|
| registered in another environment | `Failed to register: Account is registered in another environment: <ENV> (IOException)` | 3 |
| already registered, no `--reregister` | `Failed to register: Account is already registered (IOException)` | 3 |

Reactivation and recovery-password failures are silent (debug log) and
fall through to the session flow. Its errors are in `registration.md`
and `surface/cli/register.md`.

JSON-RPC: `setPin`, `removePin`, `register` and `verify` raise the same
error classes. Their mapping to JSON-RPC error codes is in
`surface/jsonrpc/`.

## 6. Test vectors

`testdata/vectors/svr2.json`:

- the enclave constants (MRENCLAVE, Raft configuration, advisories, URL);
- plaintext encodings of the backup, expose, restore and delete requests
  built from libsignal's published PIN-hash inputs;
- decodings of response messages, including signal-cli's reading of
  status 4 and of an unknown field 5;
- example `/v2/svr/auth` credentials with their Basic header;
- the registration-lock PUT body;
- 423 bodies with the parsed values and the hours shown.

PIN hashes, salts and encrypted master keys per enclave are in
`testdata/vectors/pin.json`.

## Gaps

1. **Enclave server version.** S5 is tag v1.26.0, the newest tag. The
   staging enclaves signal-cli uses (`3c699f49…`, `97f151f6…`) are newer
   builds with no matching public tag, so the database semantics and close
   codes in §3.2 and §3.3 are inferred from v1.26.0. Not observed on
   staging (no live SVR2 traffic in this spec task).
2. **Exact failure texts** of the HTTP upgrade (OkHttp's messages such as
   `Expected HTTP 101 response but was '401 Unauthorized'`) and of TLS
   or DNS errors come from OkHttp and the JVM. They were not observed,
   and only matter for the `<message>` part of exit-3 errors.
3. Whether `InvalidCiphertextException` (restore blob decryption failure)
   carries a message, and so the exact text in §5.1, was not checked.
4. **Change number with a lock.** The change-number request goes over the
   authenticated WebSocket. Whether a 423 there is turned into the
   "locked" outcome, as on the REST registration path, or surfaces as a
   generic status error was not confirmed. The `finishChangeNumber` lock
   messages exist in the CLI, but the WebSocket error mapping in S2 shows
   no special case for 423. Defer to the change-number spec.
5. **Server-side lock after `removePin`.** If the attributes upload of a
   later account check fails, the server's lock state depends only on the
   `DELETE`. This was not observed on staging.
6. The `timeRemaining` value and the credential lifetime on staging may
   differ from S4/S5 defaults. Not observed.
