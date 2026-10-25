# Contact discovery (CDSI) as signal-cli 0.14.7 uses it

Spec role: prose only; no code. This file covers phone-number-to-account
lookup through the contact discovery service (CDSI): how signal-cli gets
credentials, what it sends inside the attested enclave channel, how it
reads the answer, and what it does with the result locally (token
persistence, recipient merging, "unregistered" marking, command output).

Not repeated here:

- the WebSocket transport to the enclave, SGX attestation and the Noise
  channel: `spec/enclave/attestation.md` and `spec/enclave/noise.md`
  (written in parallel). This file only gives the CDSI-specific
  parameters (§2) and what flows inside the established channel (§4).
- the chat WebSocket that carries the credential request: `transport.md`,
  `api.md`;
- the access-key derivation: `api.md` §5.1;
- CLI surfaces in full: `spec/surface/cli/getUserStatus.md`, `send.md`,
  `_send-results.md`, `listContacts.md`;
- contacts commands: `contacts.md` (written in parallel).

## Sources

| Ref | What | URL @ version |
|-----|------|---------------|
| C1 | signal-cli | https://github.com/AsamK/signal-cli @ v0.14.7 (b01b6b370dc063599a1a2b9fde0f5ff4e2d78fe8): `lib/src/main/java/org/asamk/signal/manager/helper/{RecipientHelper,SendHelper,AccountHelper,ContactHelper}.java`, `.../manager/internal/{ManagerImpl,SignalDependencies,JobExecutor}.java`, `.../manager/jobs/RefreshRecipientsJob.java`, `.../manager/storage/recipients/{CdsiStore,RecipientStore,MergeRecipientHelper,RecipientAddress}.java`, `.../manager/storage/{SignalAccount,AccountDatabase}.java`, `.../manager/storage/keyValue/KeyValueStore.java`, `.../manager/config/ServiceConfig.java`, `.../manager/SignalAccountFiles.java`, `.../manager/util/Utils.java`, `.../manager/api/{UnregisteredRecipientException,RecipientAddress,UserStatus}.java`, `src/main/java/org/asamk/signal/commands/GetUserStatusCommand.java`, `src/main/java/org/asamk/signal/util/CommandUtil.java` |
| C2 | signal-service-java (Turasa fork) | https://github.com/Turasa/libsignal-service-java @ v2.15.3_unofficial_151 (66956cb4a0394d50cee3590209baf58312084a44): `lib/network/src/main/java/org/signal/network/api/CdsApi.kt`, `lib/libsignal-service/src/main/java/org/whispersystems/signalservice/api/cds/CdsiV2Service.java`, `.../api/push/exceptions/{CdsiResourceExhaustedException,CdsiInvalidTokenException,CdsiInvalidArgumentException}.java`, `.../internal/push/CdsiAuthResponse.java`, `.../api/NetworkResultUtil.kt` |
| C3 | libsignal | https://github.com/signalapp/libsignal @ v0.99.1 (97801d22dcf9f5bf714f7b8fa3212cdc973ae1c8): `rust/net/src/cdsi.rs` (incl. its unit tests), `rust/net/src/proto/cds2.proto`, `rust/net/src/enclave.rs`, `rust/net/src/env.rs`, `rust/net/src/auth.rs`, `rust/net/src/connect_state.rs`, `rust/attest/src/constants.rs`, `rust/core/src/e164.rs`, `rust/bridge/shared/src/net/cdsi.rs`, `rust/bridge/shared/types/src/net/cdsi.rs`, `rust/bridge/shared/types/src/jni/{convert,error,mod}.rs`, `java/client/src/main/java/org/signal/libsignal/net/{CdsiLookup,CdsiLookupRequest,CdsiLookupResponse,Network}.java` |
| C4 | Signal-Server (server behaviour only) | https://github.com/signalapp/Signal-Server @ v20261022.0.0 (8edf3488f7d97398db7ffa5184c6b769a272ff59): `service/src/main/java/org/whispersystems/textsecuregcm/controllers/DirectoryV2Controller.java`, `.../auth/{ExternalServiceCredentialsGenerator,ExternalServiceCredentials}.java` |
| C5 | ContactDiscoveryService-Icelake (server behaviour only) | https://github.com/signalapp/ContactDiscoveryService-Icelake @ tag 1.25.2 (46540d24ca3437f0100fd53ba2024192d69115f5): `src/main/proto/cdsi.proto`, `src/main/java/org/signal/cdsi/WebSocketHandler.java`, `src/main/java/org/signal/cdsi/limits/RetryAfterMessage.java`, `c/aci_pni_uak_helpers/aci_pni_uak_helpers.{c,h}`, `c/ratelimit/ratelimit.{c,h}` (the tag commit was resolved with `git ls-remote`; the files were first read on `main` and confirmed byte-identical at the tag) |
| C6 | Reference distribution, offline | `jshell` over `/work/third_party/signal-cli-0.14.7/lib/*.jar`, no network, no account: access keys from `ProfileKey.deriveAccessKey`, fixed-width service-ID binaries, exception messages of the CDSI exception classes, behaviour of the models `PNI` constructor on a null argument. `protoc --decode_raw` (third_party protoc 36.2) to cross-check the constructed protobuf vectors. |

## 1. Overview

A lookup has two network phases:

1. **Credentials** from the chat service: `GET /v2/directory/auth` on the
   authenticated chat WebSocket (§3).
2. **Enclave session**: a new WebSocket to the CDSI host, attestation and
   a Noise handshake (enclave specs), then exactly two client messages and
   two or more server messages inside the encrypted channel (§4). The
   server closes the WebSocket normally when it is done.

signal-cli batches nothing. Each lookup is one credential request plus one
enclave session carrying every number in a single request message.

## 2. CDSI-specific connection parameters (staging)

| Item | Value | Source |
|------|-------|--------|
| Host | `cdsi.staging.signal.org`, port 443, TLS 1.3 minimum, HTTP/1.1 for the upgrade | C3 `env.rs` |
| Static fallback IPs (used only when DNS fails) | `104.43.162.137`, `2603:1030:7::732` | C3 `env.rs` |
| Enclave ID (MRENCLAVE), staging | `6d9b9649fa3a337754a98059c66d48ac77aaca5299d3b27d6ed1e646c7c81c0a` | C3 `attest/src/constants.rs` |
| Enclave ID, production (reference only) | `15637fa1e54fe655176d3df1a9f94b87c01ed377acaa570682dc5d72c95ef07b` | same |
| WebSocket path | `/v1/<enclave ID as lowercase hex>/discovery`, so staging is `wss://cdsi.staging.signal.org/v1/6d9b9649fa3a337754a98059c66d48ac77aaca5299d3b27d6ed1e646c7c81c0a/discovery` | C3 `enclave.rs` |
| Authentication on the upgrade request | `Authorization: Basic base64(username ":" password)`, using the pair from §3 | C3 `auth.rs`, `connect_state.rs` |
| Attestation flavour | the "CDS2" SGX handshake: MRENCLAVE check only, no Raft membership configuration (unlike SVR2) | C3 `enclave.rs` (see `attestation.md`) |
| Proxy path prefix | `/cdsi-staging`, only when a censorship-circumvention proxy is used. signal-cli's staging configuration has `censored = false` (`transport.md`), so it is not used | C3 `env.rs` |

The first server WebSocket message after the upgrade is the attestation
message, and the first client message is the Noise handshake. Both are
covered by the enclave specs. "Message 1" in §4 is the first application
message after the handshake has completed.

An HTTP 429 answer to the upgrade request, with a `Retry-After` header in
seconds, is reported as the same rate-limit error as close code 4008
(§4.5). libsignal has a unit test for this (C3).

## 3. Credentials: `GET /v2/directory/auth`

- Sent on the **authenticated** chat WebSocket (C2 `CdsApi`: a plain GET
  with no body and no extra headers).
- 200 response body: JSON object with two strings:

  | Field | Meaning |
  |-------|---------|
  | `username` | opaque; the server derives it from the account's ACI with a keyed HMAC (hex, truncated) |
  | `password` | opaque; the server's form is `<unix seconds>:<hex HMAC signature>` |

  The client must treat both as opaque strings and pass them through to
  the Basic authorization of §2 unchanged (C4). The server says the
  credentials are valid for 24 hours, but it controls the TTL.
- Unknown JSON fields are ignored.
- **No caching**: signal-cli requests fresh credentials before every
  enclave session and never stores them (C2 `CdsApi`).
- Errors on this request follow the generic chat mapping (C2
  `NetworkResultUtil.toBasicLegacy`): 401 or 403 gives
  `AuthorizationFailedException` with message `[401] Authorization failed!`
  (or `[403] …`), 413 or 429 gives the library's own
  `RateLimitException` (`[429] Rate Limited`, an I/O exception, **not** the
  manager-level rate-limit error of §7.4), other codes give a generic
  status-code exception. In each case no enclave session is opened.

## 4. Messages inside the enclave channel

Every message in this section is one protobuf, carried as one Noise
transport payload sequence in **one** WebSocket binary message. A large
request is split into several Noise packets, which are concatenated into
the same WebSocket message (C3 test `large_request_split`; the framing is
in `noise.md`). The host accepts WebSocket messages up to 10 MiB, i.e.
10 × 2^20 bytes (C5 `WebSocketHandler`).

### 4.1 `ClientRequest` (package `org.signal.cdsi`)

| # | Field | Type | Content |
|---|-------|------|---------|
| 1 | `aci_uak_pairs` | bytes | concatenation of 32-byte entries: 16-byte ACI (UUID bytes in network order, no type prefix) followed by the 16-byte unidentified access key derived from that ACI's profile key (`api.md` §5.1) |
| 2 | `prev_e164s` | bytes | concatenation of 8-byte entries, each the number as an unsigned 64-bit **big-endian** integer (the digits after `+`) |
| 3 | `new_e164s` | bytes | same encoding |
| 4 | `discard_e164s` | bytes | same encoding. libsignal 0.99.1 always leaves it empty |
| 5 | — | reserved | formerly `has_more` (bool). Never sent |
| 6 | `token` | bytes | rate-limit token from an earlier lookup; empty if none |
| 7 | `token_ack` | bool | true only in the acknowledgement message (§4.3) |
| 8 | — | reserved | formerly `return_acis_without_uaks` (bool). Never sent |

Proto3 rules apply: an empty bytes field and a false bool are omitted, and
libsignal writes the present fields in field-number order. The order of
entries inside `prev_e164s`, `new_e164s` and `aci_uak_pairs` carries no
meaning. signal-cli builds them from hash sets, so the order is
arbitrary, and the server sorts the numbers before it checks the token
(C5 `ratelimit.c`). No compression of any kind is applied. Entries are
not deduplicated beyond what the caller's sets guarantee.

### 4.2 `ClientResponse`

| # | Field | Type | Content |
|---|-------|------|---------|
| 1 | `e164_pni_aci_triples` | bytes | concatenation of 40-byte records: 8-byte big-endian number, 16-byte PNI, 16-byte ACI (UUID bytes, no type prefix) |
| 2 | — | reserved | formerly `retry_after_secs` (int32) |
| 3 | `token` | bytes | new rate-limit token |
| 4 | `debug_permits_used` | int32 | rate-limit permits charged for this lookup; for logging only |

### 4.3 Exchange

1. **Client → server, message 1**: a `ClientRequest` with fields 1, 2, 3
   and 6 as applicable, `token_ack` false.
2. **Server → client**: a `ClientResponse` whose `token` is non-empty. An
   empty token here is a protocol error ("no token found in response").
   The other fields of this message are ignored.
3. At this point the client hands the new token to its caller **before**
   continuing (C3 `Network.cdsiLookup`; signal-cli persists it here, see
   §6.3).
4. **Client → server, message 2**: a `ClientRequest` with only
   `token_ack = true`. Encoded, that is the two bytes `38 01`.
5. **Server → client**: a `ClientResponse` with the triples and
   `debug_permits_used`. The server sends one message and then closes the
   WebSocket with code 1000 (C5). The client must accept any number of
   further messages before the close and merge each one into the first
   with protobuf merge semantics. For bytes fields that means the last
   non-empty value wins, not concatenation.
6. **Close**: a close frame with code 1000 (any reason), or a close
   without a frame, ends the lookup successfully. Any other close code is
   an error (§4.5).

The server checks the token in message 1 as follows (C5 `ratelimit.c`):

- no token and a non-empty `prev_e164s`: invalid argument (4003);
- a token that is not exactly 65 bytes: invalid argument;
- first token byte not equal to 1 (the token version): invalid argument;
- otherwise the token must equal its own recomputation over the token's
  version and entropy bytes plus the **sorted union of `prev_e164s` and
  `discard_e164s`**. If it does not match, the close code is 4101
  (invalid token).

So a stored token is only valid together with exactly the set of numbers
it was issued for. Clients learn the token layout (1 version byte, 32
entropy bytes, 32 MAC bytes) only for diagnostics and must otherwise
treat it as opaque. The server charges rate-limit permits for
`new_e164s`, and for `prev_e164s` as well when no token is given (C5 proto
comments).

### 4.4 Interpreting the triples

For each 40-byte record:

- **Number field zero**: skip the record. The server writes an **all-zero
  record** (number included) for every requested number that has no
  account (C5 `aci_pni_uak_helpers.c`), so absent numbers simply do not
  appear in the result.
- **PNI**: present if non-zero. In a non-skipped record it is always
  present in practice.
- **ACI**: present if non-zero. The server returns the ACI **only if** the
  request contained an `aci_uak_pairs` entry whose ACI and access key both
  match the account's stored values, and the account has a non-zero
  stored access key. Otherwise the ACI is all zeros and only the PNI is
  learned (C5).
- The result is a map from number string to (ACI optional, PNI optional).
  The key is `+` followed by the decimal value. A later record with the
  same number overwrites an earlier one.
- A triples field whose length is not a multiple of 40 is a protocol error
  ("could not parse response triples (N bytes)").

On a successful lookup the server returns exactly one record per
requested number (prev and new together), so the plaintext size does not
depend on the results (C5 proto comment).

### 4.5 Close codes and errors

| Close code | Meaning (server) | libsignal error | Java exception signal-cli sees (C2 mapping) |
|-----------:|------------------|-----------------|---------------------------------------------|
| 1000 | done | success (only after the token ack) | — |
| 4003 | invalid argument: malformed request, numbers not a multiple of 8 bytes, `prev_e164s` without a token, bad token length or version, missing token ack, and similar | invalid argument (server reason kept) | `IllegalArgumentException` → `CdsiInvalidArgumentException`, message `StatusCode: 4003` |
| 4008 | rate limit exceeded. The close **reason** is JSON `{"retry_after": <seconds>}` | rate limited, with the seconds | `RetryLaterException` → `CdsiResourceExhaustedException`, message `StatusCode: 4008`, carrying the retry-after seconds |
| 4013 | server internal error, or the enclave hit an unexpected error | server error, reason `ServerInternalError` | `CdsiProtocolException` → wrapped in a plain `IOException` |
| 4014 | server unavailable (host I/O failure) | server error, reason `ServerUnavailable` | same as 4013 |
| 4101 | invalid token | invalid token | `CdsiInvalidTokenException`, message `StatusCode: 4101` |
| anything else, or 4008 whose reason is not JSON with a `retry_after` field | — | unexpected close (enclave protocol error) | `CdsiProtocolException` → `IOException` |

Notes:

- The 4008 reason is parsed as JSON with an unsigned 32-bit `retry_after`
  field. Unknown extra fields are ignored. libsignal's tests use
  `{"retry_after": 12345}` and `{"retry_after": 513}`.
- 4008 and 4101 may arrive in answer to message 1 or to message 2 (the
  token ack). libsignal tests cover 4008 in both places.
- A request containing a number string that does not parse (§5.2) fails
  on the client **before any connection**, with `IllegalArgumentException`.
  The service layer maps that to `CdsiInvalidArgumentException` too, so
  it has the same local consequences as a server 4003 (§6.3).
- Message texts of the wrapped protocol errors (Rust display strings,
  surfaced through `IOException(cause)` as
  `org.signal.libsignal.net.CdsiProtocolException: <text>`):
  `Protocol error after establishing a connection` (enclave protocol or
  unexpected close), `CDS protocol: no token found in response`,
  `CDS protocol: could not parse response triples (<n> bytes)`,
  `Server error: ServerInternalError`, `Server error: ServerUnavailable`.
- Attestation failures, transport failures and "all connection attempts
  failed" use libsignal's general network exception classes (enclave
  specs). See Gap 4 for their exact surfaced text.
- There is no per-lookup timeout at the service layer (signal-cli passes
  none). Only libsignal's connection timeouts apply.

## 5. Building the request (libsignal side)

### 5.1 Inputs

The service layer passes (C2 `CdsiV2Service`, C3 `CdsiLookupRequest`):

- `previousE164s`: a set of number strings;
- `newE164s`: a set of number strings;
- `serviceIds`: a map from service ID to profile key. Each entry becomes
  one `aci_uak_pairs` entry, with the access key derived from the profile
  key (`api.md` §5.1);
- `token`: optional bytes.

A non-empty previous set without a token is rejected locally with
`IllegalArgumentException` ("You must have a token if you have
previousE164s!"). signal-cli never triggers this (§6.2). The removed or
discard set is always empty.

### 5.2 Number strings

Each number string is converted to a 64-bit value as follows (C3
`e164.rs`):

1. If the string starts with `+`, drop that one character.
2. Parse the rest as an unsigned decimal 64-bit integer, using the Rust
   standard parser: one optional leading `+` is allowed again, digits
   only, no spaces, no `-`, no overflow past 2^64−1.
3. Zero is rejected.

Examples: `+18005551001` and `18005551001` both become 18005551001
(`0x00000004 3136e799`). `+0049123` becomes 49123 (leading zeros are
accepted). `++1` becomes 1. `+0`, an empty string, `+1 555`, `+1-555` and
`+abc` are rejected. A rejection makes the whole request fail (§4.5). No
connection is made.

In practice signal-cli only passes numbers it has already canonicalized to
E.164 (`+` and digits) or numbers read back from its own store, so
rejection is rare.

### 5.3 Service-ID entries

The Java bridge passes each service ID in its 17-byte fixed-width form
(type byte `00` for ACI, followed by the UUID). The bridge only accepts
ACIs. A PNI key is rejected. The type byte is stripped, and the 32-byte
pair is the 16 UUID bytes followed by the 16-byte access key. An access
key that is not 16 bytes is rejected ("access_key has wrong number of
bytes").

## 6. signal-cli's use

### 6.1 When a lookup runs

| Trigger | Kind | Numbers |
|---------|------|---------|
| Resolving a phone-number recipient (`send`, `sendReaction`, `updateGroup` members, `block`, `trust`, `updateContact`, mentions, quotes, etc.) when the recipient store has **no entry for that number, or an entry without any service ID** | partial | that one number |
| Resolving the address of a stored recipient that has a number but no ACI/PNI (e.g. group members, sync targets, profile fetch). This lookup runs **again every time** such a recipient is resolved | partial | that one number |
| A send that gets 404 "unregistered" from the chat server (`send.md` §5) | partial | the recipient's stored number, if it has one |
| `getUserStatus` with **at most 3** distinct valid numbers | partial | those numbers |
| `getUserStatus` with **more than 3** distinct valid numbers | full | all stored recipient numbers plus the requested numbers |
| Opening the account (every CLI command or daemon start that loads a registered account), if `last-recipients-refresh` is unset or older than 24 hours | full, as a background job | all stored recipient numbers |

The 3 is `MAXIMUM_ONE_OFF_REQUEST_SIZE` (C1 `ServiceConfig`).

Details of the scheduled refresh:

- It is queued after the account checks at startup (C1
  `ManagerImpl.checkAccountState`), together with a whoami check. It runs
  on a background thread while the command proceeds. Closing the account
  waits for queued jobs, so a short CLI command can take as long as the
  refresh.
- There is no periodic timer. A long-running daemon refreshes only at
  start (or when an account is added or loaded).
- Any failure of the job, including a rate limit, is logged at WARN
  (`Full CDSI recipients refresh failed, ignoring: <message>
  (<ExceptionSimpleName>)`) and otherwise ignored. There is no retry or
  backoff state.
- `last-recipients-refresh` is cleared when registration finishes and when
  provisioning (linking) data is set, so a fresh account does a full
  refresh on its next start.

"Stored recipient numbers" (C1 `RecipientStore.getAllNumbers`) means
every non-null `number` in the recipient table, except the account's own
number and any value that Java `Long.parseLong` rejects. That check
accepts a single leading `+` or `-` sign.

### 6.2 What goes into the request

Partial lookup (requested set N):

- `new_e164s` = N, `prev_e164s` empty, no token;
- the CDSI number table (§6.3) is not consulted for the request.

Full lookup (requested set N, stored table P, stored token T):

- `new_e164s` = N minus P.
- If T exists and P is non-empty: `prev_e164s` = P and `token` = T.
- If P is non-empty but no token is stored: `prev_e164s` is sent
  **empty** and no token is sent, **while `new_e164s` still excludes P**.
  Numbers in P are then not looked up at all. Normally P and T are
  written and cleared together, so this case should not arise. See
  Gap 7.
- If P is empty: no prev and no token.
- If both `new_e164s` and P are empty, no lookup is made and the result
  is empty.

Both kinds:

- `aci_uak_pairs`: one entry for **every** recipient that has both an ACI
  and a profile key, including the account itself with its own profile
  key. This is how the server is allowed to return ACIs (§4.4). A
  recipient whose ACI is known but profile key is not gets back only its
  PNI.
- No `discard_e164s`. The ack is sent by libsignal automatically.

### 6.3 Token and number-table persistence

Both live in the account database (the SQLite file of the account):

- Key-value table `key_value(key TEXT UNIQUE NOT NULL, value ANY)`:
  - `cdsi-token`: the token as a BLOB, or NULL when cleared;
  - `last-recipients-refresh`: INTEGER, Unix milliseconds of the last full
    lookup that received a token.
- Table `cdsi`, created as
  `cdsi(_id INTEGER PRIMARY KEY, number TEXT NOT NULL UNIQUE,
  last_seen_at INTEGER NOT NULL) STRICT`. `number` is the `+`-prefixed
  E.164 string and `last_seen_at` is in Unix milliseconds.

All updates happen **when the token arrives** (§4.3 step 3), i.e. before
the results do, and whether or not the lookup then succeeds:

- Partial lookup: the new token is **discarded**. The `last_seen_at` of
  any requested number that is already in `cdsi` is set to now. No rows
  are added.
- Full lookup:
  - rows whose number is not in P ∪ new are deleted;
  - numbers in P ∪ new that have no row are inserted, with `last_seen_at`
    = now (an existing row is updated on conflict);
  - `last_seen_at` = now for every number in N ∪ new;
  - `cdsi-token` = the new token;
  - `last-recipients-refresh` = now.

  The table then holds exactly P ∪ new, which is the set the new token
  covers (Gap 2). Note: P normally includes numbers no longer in the
  recipient store, and they are kept and re-sent as `prev_e164s` for good.
- If the lookup fails with **invalid token (4101)** or **invalid argument
  (4003, or a client-side rejected number)**: `cdsi-token` is set to NULL
  and every row of `cdsi` is deleted, and the error is then passed on.
  The next full lookup starts from scratch, with all numbers as new.
- Other failures (rate limit, network, protocol) change nothing beyond
  what was already written at token time. A 4008 that arrives **after**
  the token ack leaves the new token, the table and
  `last-recipients-refresh` updated even though no results arrived. The
  next scheduled refresh is then up to 24 h away.

### 6.4 Applying results to the recipient store

Let R be the result map (number → ACI optional, PNI optional), dropping
any all-unknown entries. For a full lookup R covers P ∪ new. For a partial
one it covers N. Let N be the requested set.

1. **Trusted resolution**: for every entry in R, signal-cli records the
   association {ACI if present, PNI if present, number} as a high-trust
   address (rules below). This also creates recipients for numbers that
   were only in P.
2. **Not found**: for each number in N that is not a key of R and has a
   local recipient:
   - set `discoverable` = false;
   - if that recipient has **no ACI**, or its contact data already has an
     unregistered timestamp, also mark it unregistered. That sets
     `unregistered_timestamp` = now (ms) and `discoverable` = false. If the
     recipient then has both an ACI and a PNI, it is **split**: the PNI and
     number move to a new recipient, and the old one keeps the ACI (and
     username).

   A recipient with a known ACI that is merely not found is therefore only
   marked undiscoverable (for example a user who hides their number), not
   unregistered.
3. **Found**: each number in R that has a local recipient gets
   `discoverable` = true.

Trusted resolution of an address A that has a number and at least one
service ID (C1 `MergeRecipientHelper`). If A matches the account's own
address, it is resolved normally without merging. Otherwise:

- Find every recipient that shares any identifier with A (ACI, PNI,
  number).
- **None**: create one with A.
- **Exactly one, call it X**:
  - if X already has all of A's identifiers, keep X;
  - else if X has no ACI, or A has no ACI and (A has no PNI or X's PNI
    equals A's), or X's ACI equals A's: add A's identifiers to X. A's
    values win, and X keeps any identifier that A lacks;
  - else (X has a different ACI): remove from X every identifier it
    shares with A, and create a new recipient with A.
- **Several**: the target is the one whose ACI equals A's. Failing that,
  the one whose PNI equals A's. Every other match is either **merged**
  into the target, if it has no identifier beyond those in A or it holds
  only a PNI and a number, or **stripped** of the identifiers it shares
  with the final address. The final address is A plus any missing
  identifiers taken from the target and the merged ones. If there is no
  target, a new recipient is created with the final address. Merged
  recipients have their data (sessions, identities, group membership,
  etc.) moved to the target and are deleted. A merged recipient's "PNI
  signature verified" flag carries over.
- The resulting recipient is marked registered
  (`unregistered_timestamp` = NULL).

### 6.5 Choosing the service ID

From a result entry signal-cli uses the ACI if present, else the PNI.
Consequences:

- A number whose ACI the server withholds (no matching profile key) is
  addressed by its **PNI**: sends target `PNI:<uuid>` (`send.md` §7), and
  `getUserStatus` reports the PNI's UUID in `uuid` (§7.1).
- An entry with neither is impossible after §4.4 (such records are
  skipped). See Gap 3 for the theoretical case "number present, PNI zero,
  ACI non-zero".

### 6.6 Single-number resolution flow

To resolve a phone-number recipient (C1 `RecipientHelper` and
`RecipientStore.resolveRecipientByNumber`):

1. If the store has a recipient with that number **and** a service ID,
   use it. No network.
2. Otherwise do a partial lookup for that number (§6.2 to §6.4). **Any**
   failure of the lookup (network, rate limit, invalid argument, protocol)
   counts as "not found".
3. Not found: the command fails with "unregistered" for that number
   (§7.2). Found: the trusted recipient of §6.4 is used.

`removeContact` (hide, forget, or delete contact) resolves the same way.
When the lookup says not found, it falls back to an existing number-only
recipient, and when there is none it does nothing. It does not fail.

When resolving a stored recipient's address for sending (number but no
service ID), a failed lookup is logged at WARN (`Failed to get uuid for
e164 number: <number>`) and the address is used without a service ID.

## 7. Command-level behaviour

### 7.1 `getUserStatus`

Full surface: `spec/surface/cli/getUserStatus.md`. CDSI-specific points:

- Inputs are canonicalized against the account's own number. Unparsable
  inputs are not sent. The valid ones go into one lookup, partial if there
  are 3 or fewer and full otherwise (§6.1).
- Found: `uuid` is the ACI if returned, else the **PNI** UUID (§6.5).
  `isRegistered` is true. The plain-text suffix
  ` (unrestricted sealed sender)` comes from a profile fetch for that
  service ID (`profile.md`).
- Not found (absent from the result): `uuid` null, `isRegistered` false,
  plain text `<input>: false`.
- Rate limit (4008, or HTTP 429 at the enclave upgrade): stderr
  `Rate limit reached` + newline + `Next attempt may be tried at <ms>
  (<ISO time>)`, where the time is now + retry-after seconds. Exit 5.
- Any other failure: stderr `Unable to check if users are registered:
  <message> (<ExceptionSimpleName>)`, exit 3. Examples derived from C6:
  - invalid token: `Unable to check if users are registered: StatusCode: 4101 (CdsiInvalidTokenException)`;
  - invalid argument: `Unable to check if users are registered: StatusCode: 4003 (CdsiInvalidArgumentException)`;
  - server error: `Unable to check if users are registered: org.signal.libsignal.net.CdsiProtocolException: Server error: ServerInternalError (IOException)`;
  - credential request rate-limited by the chat server: `Unable to check if users are registered: [429] Rate Limited (RateLimitException)`;
  - credential request unauthorized: `Unable to check if users are registered: [401] Authorization failed! (AuthorizationFailedException)`.

  These texts are derived from source and the exception classes, not
  observed live (Gap 5).

### 7.2 Commands that resolve a number recipient

When a number cannot be resolved (§6.6 step 3), the manager raises an
"unregistered recipient" error whose identifier is the canonical number
(exception message `Unregistered user: <number>`). The CLI texts are
already specified:

- CLI commands with recipient arguments (`send`, `sendReaction`, `block`,
  `trust`, `updateContact`, `updateGroup`, `quitGroup --admin`,
  `getAvatar`, poll and pin commands, `sendAdminDelete`, …): stderr
  `The user <number> is not registered.`, exit 1 (see the per-command
  surface files).
- In send results, a recipient that becomes unregistered **during** the
  send (404 then a failed re-lookup): `Unregistered user "<id>"`, type
  `UNREGISTERED_FAILURE` (`_send-results.md`).

Because §6.6 treats every lookup failure as "not found", a CDSI rate limit
or outage while sending to an unknown number also shows up as
`The user <number> is not registered.` There is no rate-limit message on
this path.

### 7.3 `listContacts` and others

`listContacts` shows the `Unregistered` and discoverability state written
by §6.4. It never triggers a lookup itself, apart from the startup
refresh (§6.1).

### 7.4 Rate-limit object

signal-cli turns a CDSI 4008 into its manager-level rate-limit error,
with retry-after = seconds × 1000 ms, **only** in `getUserStatus`.
Everywhere else the exception is swallowed (§6.6) or logged (§6.1).

## 8. Implementation checklist (Go)

- Credentials: one authenticated GET per lookup; no cache.
- Request: 8-byte big-endian numbers, 32-byte ACI‖UAK pairs, proto3 field
  order, empty fields omitted; token only together with the stored prev
  set.
- Persist the token and the table at token time, and only for full
  lookups. Clear both on 4101 or 4003.
- Skip records whose number is zero. ACI zero means unknown. Prefer the
  ACI over the PNI.
- Map close codes per §4.5. The 4008 reason is JSON `retry_after`.
- Scheduled full refresh at account open if more than 24 h have passed;
  the 3-number threshold for `getUserStatus`.
- Vectors: `testdata/vectors/cdsi.json`.

## Gaps

1. **Live confirmation.** Nothing here was observed against the staging
   CDSI. The sequence, the server's single final message followed by a
   1000 close, and the exact close reasons come from source (C3, C5).
2. **Which numbers the new token covers.** The enclave code that issues
   the token was not located. From the validation rule (§4.3) and the
   client's bookkeeping, it must cover prev ∪ new of the issuing request,
   or that minus `discard_e164s`. signal-cli's table update assumes
   prev ∪ new.
3. **Record with zero PNI but non-zero ACI.** The server cannot produce
   it (the ACI is masked whenever the record is empty). If it ever
   arrived, signal-cli 0.14.7 would fail with a `NullPointerException`
   while converting the result: C6 shows the models `PNI` constructor
   rejects null. That would surface as a runtime error of the whole
   lookup. A Go client may treat such a record as PNI-unknown instead.
4. **Text of transport and attestation failures** surfaced to
   `getUserStatus` (`Unable to check …: <message> (<class>)`) depends on
   libsignal's network exception classes (enclave specs) and was not
   derived.
5. **CLI error texts in §7.1** were assembled from the exception classes
   (C6) and the command's formatting (C1), not observed.
6. **Server request limits.** Beyond the 10 MiB WebSocket message cap, the
   per-request limits on the number of new numbers or pairs, and the
   rate-limit permit accounting, were not traced in C5. A comment in C2
   mentions "over 50k new contacts" as one cause of 4003 (unverified).
7. **Prev set without a token** (§6.2). The case where the `cdsi` table is
   non-empty but `cdsi-token` is NULL is reachable only if one is changed
   without the other (e.g. by an external edit, or a crash between the two
   writes). In that case signal-cli silently skips the numbers already in
   the table, and those that are also in the recipient store are then
   marked undiscoverable. Matching this exactly is optional.
8. **Recipient-merge side effects** (§6.4: which tables move from a merged
   recipient to the target) belong to a recipient-store spec, which does
   not exist yet. This file only fixes the identifier outcome.
9. **Profile key that fails to load.** If a stored profile key cannot be
   parsed, the ACI→profile-key map building fails with a runtime error
   before any network activity (C1 `getServiceIdToProfileKeyMap` collects
   null values). Not tested.
