# Changing the account's phone number (signal-cli 0.14.7, staging)

Spec role: prose and tables only; no code. This file covers
`startChangeNumber NUMBER [--voice] [--captcha CAPTCHA]` and
`finishChangeNumber NUMBER -v CODE [-p PIN]` on a **primary** device: the
verification session for the new number, the per-device key material and
sync messages, the `PUT /v2/accounts/number` request, how each server
answer turns into a user-visible outcome, and what changes locally.

Not repeated here (see the referenced files):

- verification-session endpoints, session metadata and the session
  algorithm: `registration.md` §2, §3, §6;
- prekey formats, signatures and the refresh pass: `keys.md` §2, §3;
  `spec/crypto/xeddsa.md`, `spec/crypto/kem.md`;
- per-device encryption and the per-device message entry
  (`type`, `destinationDeviceId`, `destinationRegistrationId`, `content`):
  `send.md` §2.1, §4; padding: `content.md` §2;
- `SyncMessage` wire format and how a linked device applies a received
  `pniChangeNumber`: `sync.md` §2.1, §2.3, §6.13;
- transports and credentials: `api.md` §2, `transport.md` §2, §4;
- SVR2 and the registration-lock value: `svr.md` §4.3;
- command surfaces and exit codes: `spec/surface/cli/startChangeNumber.md`,
  `spec/surface/cli/finishChangeNumber.md`,
  `spec/surface/jsonrpc/methods/startChangeNumber.md`,
  `spec/surface/jsonrpc/methods/finishChangeNumber.md`.

## Sources

| Ref | What | URL @ version |
|-----|------|---------------|
| C1 | signal-cli | https://github.com/AsamK/signal-cli @ v0.14.7 (b01b6b370dc063599a1a2b9fde0f5ff4e2d78fe8): `lib/src/main/java/org/asamk/signal/manager/helper/AccountHelper.java` (start/finish change number, applying the PNI change), `.../manager/internal/{ManagerImpl,SignalDependencies}.java`, `.../manager/util/{NumberVerificationUtils,KeyUtils,Utils}.java`, `.../manager/storage/SignalAccount.java` (session slot, `setNewPniIdentity`, `setPni`, prekey counters), `.../manager/config/ServiceConfig.java`, `.../manager/api/{CaptchaRequiredException,RateLimitException,VerificationMethodNotAvailableException}.java`; `src/main/java/org/asamk/signal/commands/{StartChangeNumberCommand,FinishChangeNumberCommand}.java`, `src/main/java/org/asamk/signal/util/CommandUtil.java`; `man/signal-cli.1.adoc` |
| C2 | signal-service-java (Turasa fork) | https://github.com/Turasa/libsignal-service-java @ v2.15.3_unofficial_151 (66956cb4a0394d50cee3590209baf58312084a44): `lib/libsignal-service/src/main/java/org/whispersystems/signalservice/api/account/{AccountApi.kt,ChangePhoneNumberRequest.java}`, `.../api/registration/RegistrationApi.kt`, `.../api/NetworkResultUtil.kt`, `.../api/NetworkResultExtensions.kt`, `.../api/SignalServiceMessageSender.java` (`getEncryptedSyncPniInitializeDeviceMessage`, `handleChangeNumberMismatchDevices`, `getEncryptedMessage`), `.../internal/push/{PushServiceSocket,VerifyAccountResponse,MismatchedDevices,StaleDevices,LockedException}.java`, `.../api/push/exceptions/*.kt|java` (status-code exception classes and their messages), `lib/libsignal-service/src/main/protowire/SignalService.proto`; `core/network/src/main/java/org/signal/network/{NetworkResult.kt,exceptions/NonSuccessfulResponseCodeException.kt,websocket/WebSocketRequestExt.kt,websocket/WebsocketResponse.java}` |
| C3 | libsignal | https://github.com/signalapp/libsignal @ v0.99.1 (97801d22dcf9f5bf714f7b8fa3212cdc973ae1c8): `rust/protocol/src/proto/storage.proto` (record structures), `rust/protocol/src/identity_key.rs` (identity key pair serialization) |
| C4 | Signal-Server | https://github.com/signalapp/Signal-Server @ v20261022.0.0 (8edf3488f7d97398db7ffa5184c6b769a272ff59): `service/src/main/java/org/whispersystems/textsecuregcm/controllers/AccountControllerV2.java`, `.../entities/{ChangeNumberRequest,PhoneVerificationRequest,IncomingMessage,AccountIdentityResponse}.java`, `.../storage/{ChangeNumberManager,AccountsManager}.java`, `.../auth/PhoneVerificationTokenManager.java`, `.../push/MessageSender.java`, `.../controllers/VerificationController.java`, `.../mappers/NonNormalizedPhoneNumberExceptionMapper.java` (server behaviour notes only) |
| C5 | Reference distribution, offline | `jshell` over `/work/third_party/signal-cli-0.14.7/lib/*.jar`, no network: (a) the bundled JSON serializer applied to a hand-built change-number request (key order, null emission, encodings in §3.5); (b) the bundled WebSocket response converter plus the "basic" status mapping applied to synthetic responses with statuses 400, 401, 403, 409, 410, 413, 422, 423, 428, 429, 500 and to a synthetic 200 body (exception classes and messages in §3.8); (c) a byte search of all bundled jars for code that constructs the library's non-normalized-number exception (none found) |

## 1. Overview

A number change is two CLI invocations, each a fresh process that first
runs the normal account check (`api.md` §4.3):

1. `startChangeNumber NEW` opens or reuses a verification session **for
   the new number** and asks the server to send a code to it (§2).
2. `finishChangeNumber NEW -v CODE` generates a fresh PNI identity and
   per-device PNI prekeys, encrypts one `pniChangeNumber` sync message
   per linked device, submits the code to the session, then sends
   `PUT /v2/accounts/number` over the authenticated WebSocket (§3). On
   success the account's number, PNI and PNI key material are replaced
   locally (§4).

Both commands refuse on a linked device with
`This command doesn't work on linked devices.` (exit 1, JSON-RPC −1)
before any request of their own; the check is local (the device ID is
not 1).

## 2. `startChangeNumber NEW [--voice] [--captcha CAPTCHA]`

### 2.1 Transport and credentials

All requests are **REST** to `https://chat.staging.signal.org`, made
through a separate socket built with *static credentials*: no ACI,
E.164 = **the new number exactly as typed**, device 1, password = the
account's existing password (`api.md` §2.1). So the `Authorization`
header is Basic over `<NEW>:<password>` (for example
`+15555550101:<password>`). The verification-session endpoints do not
authenticate the caller (C4 `VerificationController` takes no account
principal), so the fact that this credential does not belong to any
account does not matter. Headers otherwise as `registration.md` §2.

signal-cli does not normalize or validate `NEW`. It is sent as given.

### 2.2 Session flow

Identical to `register` (`registration.md` §2.5, steps 1–7, and the
captcha handling there), with these differences:

| Aspect | `register` | `startChangeNumber` |
|---|---|---|
| Number in `POST /v1/verification/session` body (`number`) | account number | `NEW` |
| Credential | `+E164:password` of the account | `NEW:password` (§2.1) |
| Pre-steps (refuse if registered, reactivation, recovery-password path) | yes | **none**; the session algorithm starts immediately |
| Session ID storage | the account's single verification-session slot, tagged with the number | the **same** slot, tagged with `NEW`. Starting a change number overwrites any pending registration session and vice versa. A stored ID is only reused when the tag equals `NEW`; otherwise a new session is created. |
| PNI key generation | at `verify` | none at this stage |

The code request is exactly `registration.md` §3:
`POST /v1/verification/session/{id}/code` with
`{"transport":"sms"|"voice","client":"android"}` and `Accept-Language`.
signal-cli prints nothing on success; JSON-RPC result `{}`.

Quirk (same code path as `register`): if the very first status/create
call answers 409 with a challenge and `--captcha` was given, the captcha
PATCH is addressed to the *previously stored* session ID for `NEW`. When
none was stored, the path segment is the literal text `null`
(`PATCH /v1/verification/session/null`), which the server rejects; the
user simply re-runs the command.

### 2.3 Outcomes

"Local" means decided from session metadata without a further request.
Exit codes are for the CLI; JSON-RPC codes are −1 (user error), −3 (I/O
error), −5 (rate limit), per `spec/surface/jsonrpc/methods/startChangeNumber.md`.

| Situation | Message (stderr) | Exit |
|---|---|---|
| Success (code requested) | none | 0 |
| Linked device | `This command doesn't work on linked devices.` | 1 |
| Status/create answered 409 with `captcha`/`pushChallenge` and no `--captcha`; or code request answered 409 with a challenge; or `requestedInformation` contains `captcha` and the session is still not allowed to request a code | the three-line captcha text of `CommandUtil` (`Captcha required for verification, use --captcha CAPTCHA` / `To get the token, go to https://signalcaptchas.org/registration/generate.html` / `After solving the captcha, right-click on the "Open Signal" link and copy the link.`); with `--captcha` given instead: `Invalid captcha given.` | 1 |
| Captcha PATCH answered 403, 409-with-challenge, or 400 | `Invalid captcha given.` (the library's distinguishing text is not shown) | 1 |
| Local: `nextVerificationAttempt` > 0 (and no earlier rule fired) | captcha text as above, plus a line `Next Captcha may be provided at <ms> (<ISO-8601 UTC>)` | 1 |
| Local: `nextSms` (or `nextCall` with `--voice`) is null | `Failed to register: Invalid verification method`, with `--voice` followed by `: Before requesting voice verification you need to request SMS verification and wait a minute.` | 1 |
| Local: `nextSms`/`nextCall` > 0 | `Rate limit reached` + newline + `Next attempt may be tried at <ms> (<ISO-8601 UTC>)`, where the time is now + that many seconds | 5 |
| **Server** 413/429 on any of the session calls or the code request | `Failed to change number: [429] Rate Limited (RateLimitException)` (or `[413]`). The library's rate-limit exception is an I/O error, not signal-cli's rate-limit type, so this is exit 3 and no retry time is shown. | 3 |
| Create/status answered 400 (this includes a non-normalized number: the server answers 400 with `{originalNumber, normalizedNumber}`, C4 mapper) | `Failed to change number: StatusCode: 400 (InvalidRegistrationSessionIdException)` | 3 |
| Code request 418 (transport not allowed) | `Failed to change number: StatusCode: 400 (InvalidTransportModeException)` | 3 |
| Code request 440 / create 502 (SMS provider failure) | `Failed to change number: StatusCode: 502 (ExternalServiceFailureException)` | 3 |
| Other status with a call-specific class (`registration.md` §6) | `Failed to change number: StatusCode: <code> (<ClassSimpleName>)` | 3 |
| Other status without one (e.g. 422 from server validation of a malformed number) | `Failed to change number: [<code>] Bad response: <code> <reason>: <body> (NonSuccessfulResponseCodeException)` | 3 |
| Network/TLS failure | `Failed to change number: <message> (<ExceptionSimpleName>)` | 3 |

The surface files list `Failed to change number: <detail>` (exit 1) for
a "non-normalized number". In 0.14.7 that branch is **unreachable**: no
class in the bundled libraries constructs the library exception it maps
from (C5 c). A non-normalized number produces the 400 row above.

## 3. `finishChangeNumber NEW -v CODE [-p PIN]`

### 3.1 Order of operations

1. Local primary check (§1).
2. Generate the new PNI identity and per-device material (§3.2) and the
   encrypted per-device messages (§3.3). This happens **before** the
   session lookup, so it also happens when the command later fails; the
   ratchet state with each linked device advances by one message.
3. Look up the session ID stored for `NEW` (§2.2). If none:
   `Failed to change number: No change number verification session active (IOException)`, exit 3.
4. Remove every `-` from `CODE`. Submit it (§3.4).
5. Send the change-number request (§3.5) with `reglock` null.
6. On success: clear the session slot, then apply the result (§4).

### 3.2 Key material

| Item | How it is made |
|---|---|
| New PNI identity key pair | one fresh Curve25519 pair, shared by all devices |
| Device list | device **1**, followed by every other device ID of the account's **own ACI** for which an (unarchived) session exists in the ACI session store. Devices the primary has never exchanged messages with, or whose sessions were archived, are not included. |
| Per device: signed EC prekey | fresh key pair; ID uniformly random in **0..16777214**; timestamp = now (ms); XEd25519 signature by the **new PNI identity private key** over the 33-byte serialized public key |
| Per device: last-resort Kyber-1024 prekey | fresh KEM key pair; ID uniformly random in 0..16777214 (drawn independently); timestamp = now; signature by the new PNI identity private key over the 1569-byte serialized public key |
| Per device: PNI registration ID | uniformly random in **1..16380**, redrawn until it differs from the IDs already chosen for earlier devices in this list |

### 3.3 Per-device sync messages

For each device in the list:

- A `SyncMessage.PniChangeNumber` (Content → SyncMessage field 18) is
  built with all five fields (table in §6.3): the serialized new identity
  key pair, that device's serialized signed-prekey record, that device's
  serialized last-resort Kyber record, that device's PNI registration ID,
  and `newE164` = `NEW` as typed.
- For device 1 (the primary itself) it is **kept locally** and applied in
  §4. It is never sent.
- For every other device it is wrapped in a `SyncMessage` carrying only
  `pniChangeNumber` plus the usual random `padding` (1..512 bytes,
  `sync.md` §2.1), put into a `Content` with only field 2
  (`syncMessage`), padded (`content.md` §2) and encrypted **unsealed** to
  the protocol address (own ACI, device ID) with the existing session
  (content hint IMPLICIT, irrelevant when unsealed). Because only devices
  with a session are in the list, no bundle is fetched.
- The result is one per-device entry exactly as in `send.md` §2.1:
  `type` 1 (Signal message) or 3 (prekey message, while the session is
  still unacknowledged), `destinationDeviceId`, `destinationRegistrationId`
  = the remote registration ID stored in that session, and `content` =
  ciphertext in standard base64 **with** padding.

An encryption failure here (untrusted identity, invalid key, missing
session) is not caught by the command and surfaces as an unexpected
error (see `spec/surface/cli/index.md` for unexpected exceptions).

A single-device account produces an empty `deviceMessages` list.

### 3.4 Submitting the code: `PUT /v1/verification/session/{id}/code`

Body `{"code":"<CODE without dashes>"}`, sent over **REST** with the
account's own credentials (Basic `<ACI>:<password>`, `api.md` §2.2), not
the static ones of §2.1. Response handling:

| Status | Effect |
|---|---|
| 200 | continue. A wrong code also yields 200 with `verified:false` (`registration.md` §4.1); signal-cli does not look, and the change-number request then fails with 401 (§3.8). |
| 409 with `verified:true` | treated as already verified; continue |
| 409, `verified:false`, `nextVerificationAttempt` null | `Failed to change number: StatusCode: 409 (MustRequestNewCodeException)`, exit 3 |
| 409 otherwise | `Failed to change number: StatusCode: 409 (HttpConflictException)`, exit 3 |
| 400 | `Failed to change number: StatusCode: 400 (InvalidTransportModeException)`, exit 3 |
| 404 | `Failed to change number: StatusCode: 404 (NoSuchSessionException)`, exit 3 |
| 413, 429 | `Failed to change number: [429] Rate Limited (RateLimitException)`, exit 3 |
| 440 | `Failed to change number: StatusCode: 502 (ExternalServiceFailureException)`, exit 3 |

### 3.5 The request: `PUT /v2/accounts/number`

- **Transport:** the **authenticated WebSocket** (libsignal chat
  connection, `transport.md` §3), credentials `<ACI>.1:<password>`.
  Request headers: `content-type:application/json` only (plus whatever
  the connection adds, `transport.md` §3.5). There is no REST fallback.
- **Body:** compact JSON, members in exactly this order, null members
  **emitted** (confirmed with C5 a):

| # | Member | JSON type | Value in signal-cli 0.14.7 |
|---|---|---|---|
| 1 | `sessionId` | string | the stored session ID, verbatim (the server base64url-decodes it; C4) |
| 2 | `recoveryPassword` | null | always null (mutually exclusive with `sessionId`) |
| 3 | `number` | string | `NEW` as typed. The server requires E.164 (`@E164`, else 422). |
| 4 | `pniIdentityKey` | string | 33-byte serialized new PNI identity public key (`0x05` prefix), standard base64 **without** padding (44 characters) |
| 5 | `deviceMessages` | array | the per-device entries of §3.3, linked devices only, in device-list order; each `{"type":int,"destinationDeviceId":int,"destinationRegistrationId":int,"content":"<padded b64>"}` |
| 6 | `devicePniSignedPrekeys` | object | key = device ID as a decimal string, for **every** device in the list including `"1"`; value `{"keyId":int,"publicKey":"<unpadded b64, 33 bytes>","signature":"<unpadded b64, 64 bytes>"}` |
| 7 | `pniRegistrationIds` | object | key = device ID string (all devices incl. `"1"`); value = that device's new PNI registration ID (number) |
| 8 | `reglock` | string or null | null on the first (and in practice only) attempt; 64 lowercase hex characters if a registration-lock value is supplied (§3.9) |
| 9 | `devicePniPqLastResortPrekeys` | object | key = device ID string (all devices incl. `"1"`); value `{"keyId":int,"publicKey":"<unpadded b64, 1569 bytes>","signature":"<unpadded b64, 64 bytes>"}` |

Members 8 and 9 come last because they carry renamed JSON names; the
order is a serializer artifact, not a server requirement. Key order
inside the three maps is hash-map order of the decimal-string keys
(ascending for single-digit IDs in practice); the server does not care.
Entity encodings are the same as `keys.md` §2.1 and `registration.md`
§4.2.

Example (values illustrative, long strings shortened, line breaks for
reading only), primary plus linked device 2:

    {"sessionId":"<opaque id>","recoveryPassword":null,
     "number":"+15555550101",
     "pniIdentityKey":"Bc7K…xnc1BK",
     "deviceMessages":[{"type":1,"destinationDeviceId":2,
                        "destinationRegistrationId":4321,"content":"Mwo…=="}],
     "devicePniSignedPrekeys":{"1":{"keyId":8812001,"publicKey":"BeeY…ARBj","signature":"…"},
                               "2":{"keyId":120443,"publicKey":"BQ3x…","signature":"…"}},
     "pniRegistrationIds":{"1":5310,"2":977},
     "reglock":null,
     "devicePniPqLastResortPrekeys":{"1":{"keyId":40211,"publicKey":"CK…","signature":"…"},
                                     "2":{"keyId":9910233,"publicKey":"CN…","signature":"…"}}}

### 3.6 Response

200 with the account-identity JSON (C4 `AccountIdentityResponse`):
`uuid`, `number`, `pni`, `usernameHash`, `usernameLinkHandle`,
`storageCapable`, `entitlements`, `authCredentialSalt`. signal-cli reads
`pni` (bare UUID string; required, a missing or unparsable value aborts
with an unexpected error) and ignores the rest, including `number`.

### 3.7 Server-side checks (C4, informative)

In this order, so the status tells how far the request got:

1. Caller is not device 1 → 403.
2. Any signature in the two prekey maps fails against `pniIdentityKey`
   → 422 `Invalid signature`. Body validation failures (bad E.164,
   empty maps, registration ID out of range) → 422.
3. Only if `number` differs from the account's current number:
   a post-registration waiting period not yet over → 429 with
   `Retry-After`; per-number registration rate limit → 429; session
   lookup: unknown or unverified → 401, number mismatch or malformed →
   400 (malformed base64url → 422); if another account holds `number`,
   its registration lock is checked → 423 with the registration-lock
   failure body (`svr.md` §2.4).
4. `deviceMessages` are checked against the account's devices excluding
   the primary: a missing or extra device → 409
   `{"missingDevices":[…],"extraDevices":[…]}`; a registration-ID
   mismatch → 410 `{"staleDevices":[…]}`; an oversize message → 413.
5. All three maps must contain exactly the account's device IDs
   (including 1) → otherwise 409 as above.
6. The change is applied: new PNI (the PNI assigned to `number`), new
   PNI identity key, the given signed and last-resort PNI prekeys and
   PNI registration IDs; one-time prekeys of both the old and the new PNI
   are deleted. Then each device message is delivered (§5).

### 3.8 Change-number status → outcome (what 0.14.7 actually does)

The WebSocket response goes through the library's generic converter and
the "basic" status mapping (`api.md` §4.2), **not** the message-send
mapping. Verified offline (C5 b):

| Status | Exception (simple name) and message | Printed (stderr) | Exit / JSON-RPC |
|---|---|---|---|
| 200 | — | nothing | 0 / `{}` |
| 401, 403 | `AuthorizationFailedException`, `[401] Authorization failed!` | `Failed to change number: [401] Authorization failed! (AuthorizationFailedException)` | 3 / −3 |
| 413, 429 | `RateLimitException`, `[429] Rate Limited` | `Failed to change number: [429] Rate Limited (RateLimitException)` | 3 / −3 |
| any other non-2xx: 400, 409, 410, 422, 423, 5xx … | `NonSuccessfulResponseCodeException`, `[<code>] ` (with a trailing space) | `Failed to change number: [409]  (NonSuccessfulResponseCodeException)` (note the **two** spaces) | 3 / −3 |
| no response / socket failure | the I/O exception | `Failed to change number: <message> (<ExceptionSimpleName>)` | 3 / −3 |

The common failure cases therefore look like this:

- wrong verification code → 401 row;
- a linked device with no session on the primary, or a device unknown
  to the server, → 409 row;
- a stale registration ID → 410 row;
- new number registration-locked → 423 row.

Nothing is changed locally on failure except the advanced ratchet state
(§3.1 step 2) and, for a session that does not exist, nothing at all. The
session slot is kept, so the user can retry `finishChangeNumber`.

### 3.9 Intended handling that is dead code in 0.14.7

The source contains two recovery paths that the status mapping of §3.8
never triggers, because they wait for typed exceptions that only the REST
mapping produces:

- **Mismatched devices, up to 5 attempts.** The whole of §3.1 steps 2–6
  sits in a loop of at most 5 attempts that is meant to catch a 409.
  The handler archives the sessions of `extraDevices`, clears sender-key
  "shared with" state for extra and missing devices, fetches a bundle
  `GET /v2/keys/{ownAci}/{deviceId}` (`keys.md` §4) for each
  `missingDevices` entry and builds a session, then starts over with
  **fresh** key material. The code is resubmitted, and the session's
  409 `verified:true` is accepted (§3.4). If all 5 attempts saw a 409,
  the loop would end silently with no error and no local change. 410 has
  no handler at all.
- **Registration lock.** It is meant to catch a 423. Without `--pin` it
  would print `Verification failed! This number is locked with a pin.
  Hours remaining until reset: <H>` + newline +
  `Use '--pin PIN_CODE' to specify the registration lock PIN`. With
  `--pin` it would recover the master key over SVR2 using the 423 body
  (`svr.md` §4.3) and repeat the request with `reglock` = the 64-hex
  registration-lock value. The other messages it could print are
  `Verification failed! Invalid pin, tries remaining: <N>` and
  `Account is pin locked, but pin data has been deleted on the server.`.

In 0.14.7 neither path runs. A 409, 410 or 423 ends the command with the
generic row of §3.8, and `--pin` has no effect. This settles `svr.md`
Gap 4. The PIN messages in `spec/surface/cli/finishChangeNumber.md` and
the "retried up to 5 times" note in
`spec/surface/jsonrpc/methods/finishChangeNumber.md` are not reachable
against a real server.

A Go implementation that wants **byte-identical** behaviour reproduces
§3.8. One that implements the intended recovery instead (it interoperates
fine with the server) should keep the 5-attempt cap, regenerate all key
material on every attempt, and report a final 409 as an error rather than
ending silently. The choice should be recorded as a deliberate
divergence.

## 4. Local state after a successful change

1. The verification-session slot (ID and number tag) is cleared.
2. If the response `pni` equals the current local PNI, signal-cli logs
   "PNI is unchanged after change number" and **stops**. Neither the
   number nor any key is updated locally, although the server has
   replaced the PNI identity key, signed prekeys and registration IDs
   (§3.7 step 6). This can happen when `NEW` is already the account's
   number. It is a latent inconsistency in 0.14.7.
3. Otherwise the primary applies its own `pniChangeNumber` (§3.3), the
   same routine a linked device uses (`sync.md` §6.13), in this order:
   - **Identifiers:** the number becomes `newE164` (`NEW` as typed) and
     the PNI becomes the response `pni`. The identity record stored for
     the **old** PNI is deleted. The self recipient is re-resolved with
     (ACI, new PNI, new number), merging into it any recipient already
     known under the new number or PNI. Cached sender certificates are
     dropped, so the next sealed send fetches a fresh one with the new
     number. The service connections are reset for the new address. The
     group auth-credential cache is cleared. The accounts index file
     maps the account to the new number. A storage-service sync job is
     queued (out of scope).
   - **PNI identity:** the PNI identity key pair is replaced by the new
     one and trusted as the account's own PNI identity.
   - **PNI registration ID:** the primary's entry (device 1) from §3.2.
   - **PNI signed prekey and last-resort Kyber prekey:** device 1's
     records are stored and made **active**. Each counter is first set to
     the record's ID and then advanced to ID + 1 (mod 16777215), so later
     rotations continue from there. Older PNI signed and Kyber records
     are not deleted here; the normal clean-up applies (`keys.md` §3.4).
   - **PNI prekey refresh** (`keys.md` §3, PNI only). The server deleted
     all one-time PNI prekeys, so the counts are 0 and 100 one-time EC
     plus 100 one-time Kyber prekeys are uploaded with
     `PUT /v2/keys?identity=pni`. The fresh signed and last-resort keys
     are not rotated.
4. Sessions are untouched: no ACI or PNI session is archived, and no
   sender key is reset.
5. Any exception inside step 3 is caught and only logged as a warning
   ("Failed to handle change number message"). The command still exits
   0, possibly with only part of step 3 applied (for example, number and
   PNI updated but the prekey upload failed).

Output: nothing on stdout or stderr; exit 0; JSON-RPC result `{}`.
Afterwards the account must be addressed by the new number (or the
ACI) in `-a` and in the JSON-RPC `account` parameter.

## 5. What the linked devices receive

The server turns each `deviceMessages` entry into an envelope for that
device (C4 `ChangeNumberManager`, `IncomingMessage`). The envelope has:

- `type` = the entry's type;
- `sourceServiceId` = own ACI, `sourceDevice` 1;
- `destinationServiceId` = own ACI;
- `clientTimestamp` and `serverTimestamp` = the **server's** clock at
  processing time (the request carries no client timestamp);
- `urgent` true, not ephemeral, no story flag;
- `updatedPni` = the new PNI as 16 raw bytes in `updatedPniBinary`
  (field 22).

The envelopes are stored only after the number change has been committed.
Envelope field numbers: `spec/protocol/capture.md` §3. How the linked
device validates and applies the message: `sync.md` §6.13.

## 6. Protobuf and record formats used

All proto2/proto3 wire-compatible; names here are descriptive, only
numbers and types matter.

### 6.1 Identity key pair (serialized, C3 `storage.proto`, `identity_key.rs`)

| # | Field | Type | Content |
|---|---|---|---|
| 1 | public key | bytes | 33-byte serialized identity public key (`0x05` ‖ 32 bytes) |
| 2 | private key | bytes | 32-byte Curve25519 private key |

### 6.2 Signed prekey record and Kyber prekey record

Layout as in `spec/protocol/capture.md` §2 (record structure: 1 `id`
uint32, 2 public key bytes, 3 private key bytes, 4 signature bytes,
5 timestamp fixed64 ms). For the Kyber record, the public key is the
1569-byte serialized Kyber-1024 public key and the private key is the
3169-byte serialized secret key (`spec/crypto/kem.md`). The signature is
the 64-byte XEd25519 signature by the new PNI identity key (§3.2).

### 6.3 `SyncMessage.PniChangeNumber` (SyncMessage field 18)

C2 `SignalService.proto`. This agrees with `sync.md` §2.3.

| # | Field | Type | Value sent |
|---|---|---|---|
| 1 | identityKeyPair | bytes | §6.1 serialization of the new PNI identity key pair (same for all devices) |
| 2 | signedPreKey | bytes | §6.2 record of that device's new signed EC prekey (includes the private key) |
| 5 | lastResortKyberPreKey | bytes | §6.2 record of that device's new last-resort Kyber prekey (includes the secret key) |
| 3 | registrationId | uint32 | that device's new PNI registration ID |
| 4 | newE164 | string | `NEW` |

The reference encoder emits fields in the order 1, 2, 5, 3, 4. Byte order
is not needed for interoperability.

### 6.4 Containers

- `SyncMessage`: field 18 `pniChangeNumber` (message), field 8 `padding`
  (bytes, 1..512 random); nothing else.
- `Content`: field 2 `syncMessage`; nothing else (no profile key, no PNI
  signature).
- Envelope fields relevant to the receiver: 15 `updatedPni` string and
  22 `updatedPniBinary` bytes (`spec/protocol/capture.md` §3).

## 7. Interoperability checklist for a Go client

- `startChangeNumber`: reuse the registration session code with the new
  number in the body and Basic `NEW:password`. Store the session ID in
  the shared slot, tagged with `NEW`.
- `finishChangeNumber`:
  - Generate everything before the session check.
  - Include device 1 in all three maps but not in `deviceMessages`.
  - Use only own devices that have a session.
  - Encode identity and prekey keys as unpadded base64, and message
    content as padded base64.
  - Send `PUT /v2/accounts/number` over the authenticated WebSocket.
  - Map the statuses as in §3.8.
- On success: apply §4, including the PNI-only prekey refresh. Print
  nothing.

## Gaps

1. **Staging server build.** C4 is the newest public tag. Status order
   and the waiting-period rule on staging were not observed (no live
   traffic in this task).
2. **`deviceMessages` order.** The order of non-primary device IDs comes
   from the session-store query (C1). It was not checked whether that is
   ascending. The server does not depend on it.
3. **Partial failure in §4 step 3** (for example, the prekey upload
   fails after the identifiers changed) leaves state that 0.14.7 does
   not repair automatically. The next account check's refresh pass
   (`keys.md` §3.3) would upload PNI one-time keys. Not exercised.
4. **Captcha action for change-number sessions.** The server checks the
   captcha action per endpoint. signal-cli points users at the
   *registration* captcha page for `startChangeNumber`. This matches the
   verification-session flow, but was not observed on staging.
