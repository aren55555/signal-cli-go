# Usernames: service API and signal-cli behaviour (signal-cli 0.14.7)

How a username gets onto the server, how it is looked up, what signal-cli
stores and prints, and how `u:<username>` recipients resolve.

All of the local cryptography — syntax rules and error strings, the hash,
the confirm proof, candidate generation, link encryption and the
`signal.me` URL — is in `spec/crypto/usernames.md` (**CU**). This file
references it rather than repeating it.

Transport background: `spec/service/transport.md` (**T**) for the
authenticated and unauthenticated chat connections and the gRPC override
rules, `spec/service/api.md` for the generic request/response envelope,
`spec/service/sync.md` §8.1 for how libsignal's gRPC calls ride the same
HTTP/2 connection.

Vectors: `testdata/vectors/usernames.json`.

## Sources

| Ref | What | URL @ version |
|-----|------|---------------|
| C1 | signal-cli | https://github.com/AsamK/signal-cli @ v0.14.7 = commit b01b6b370dc063599a1a2b9fde0f5ff4e2d78fe8: `lib/src/main/java/org/asamk/signal/manager/helper/{AccountHelper, RecipientHelper}.java`, `internal/{ManagerImpl, RegistrationManagerImpl}.java`, `api/{UsernameLinkUrl, UsernameStatus, InvalidUsernameException, RecipientIdentifier, RecipientAddress}.java`, `storage/SignalAccount.java`, `storage/recipients/{RecipientStore, RecipientAddress}.java`, `syncStorage/{AccountRecordProcessor, StorageSyncModels, ContactRecordProcessor}.java`, `util/Utils.java`; `src/main/java/org/asamk/signal/commands/{UpdateAccountCommand, GetUserStatusCommand, ListContactsCommand, SendCommand}.java`, `util/CommandUtil.java`, `json/{JsonContact, JsonRecipientAddress}.java`, `man/signal-cli.1.adoc` |
| C2 | libsignal-service-java fork | https://github.com/Turasa/libsignal-service-java @ v2.15.3_unofficial_151 = commit 66956cb4a0394d50cee3590209baf58312084a44: `lib/libsignal-service/src/main/java/org/whispersystems/signalservice/api/account/AccountApi.kt`, `internal/push/{ReserveUsernameRequest, ReserveUsernameResponse, ConfirmUsernameRequest, ConfirmUsernameResponse, SetUsernameLinkRequestBody, SetUsernameLinkResponseBody, GetUsernameFromLinkResponseBody, WhoAmIResponse}`, `api/push/UsernameLinkComponents.kt`, `api/NetworkResultUtil.kt`, `core/network/.../NetworkResult.kt`, `core/util/.../Base64.kt`, `UuidUtil.kt`, `lib/network/src/main/java/org/signal/network/api/UsernameApi.kt`, `lib/libsignal-service/src/main/protowire/StorageService.proto` |
| L1 | libsignal | https://github.com/signalapp/libsignal @ v0.99.1 = commit 97801d22dcf9f5bf714f7b8fa3212cdc973ae1c8: `rust/net/chat/src/{api,ws,grpc}/usernames.rs`, `rust/net/chat/src/grpc/accounts.rs`, `rust/bridge/shared/src/net/chat.rs`, `java/client/src/main/java/org/signal/libsignal/net/{AuthUsernamesService, UnauthUsernamesService}.kt`, `rust/net/grpc/proto/org/signal/chat/account.proto` |
| S1 | Signal-Server (behaviour only) | https://github.com/signalapp/Signal-Server @ v20261022.0.0 = commit 8edf3488f7d97398db7ffa5184c6b769a272ff59: `service/src/main/java/org/whispersystems/textsecuregcm/controllers/AccountController.java`, `entities/{ReserveUsernameHashRequest, ReserveUsernameHashResponse, ConfirmUsernameHashRequest, UsernameHashResponse, EncryptedUsername, UsernameLinkHandle, AccountIdentityResponse}.java`, `util/{UsernameHashZkProofVerifier, ByteArrayBase64UrlAdapter}.java`, `storage/AccountsManager.java` |
| J1 | Reference distribution, offline | `java` over `/work/third_party/signal-cli-0.14.7/lib/*.jar`, no network: URL parsing/serialization cases and the crypto vectors |

## 1. Overview of the model

- An account may have at most one **username hash** (32 bytes) and at most
  one **username link** (a server-side handle plus an opaque ciphertext).
- The server never learns the username text. It stores the hash, an
  encrypted blob, and the link handle.
- Claiming a username is two steps: **reserve** one of up to 20 candidate
  hashes (held for 5 minutes), then **confirm** it with a zero-knowledge
  proof plus the encrypted link blob.
- Other clients resolve a username by computing its hash locally and
  asking the (unauthenticated) server which ACI owns that hash; or they
  resolve a link URL by fetching the ciphertext for the handle and
  decrypting it with the entropy from the URL.

## 2. Endpoints

Transport column: **auth WS** = request over the authenticated chat
WebSocket (T §3); **unauth WS** = over the unauthenticated chat
connection; **gRPC** = a libsignal typed call on the authenticated HTTP/2
connection (sync.md §8.1), with no WebSocket fallback.

| # | Operation | Method, path | Transport in signal-cli 0.14.7 |
|---|---|---|---|
| 1 | Reserve | `PUT /v1/accounts/username_hash/reserve` | auth WS |
| 2 | Confirm | `PUT /v1/accounts/username_hash/confirm` | auth WS |
| 3 | Delete username hash | `DELETE /v1/accounts/username_hash` (REST) | **gRPC** `Accounts/DeleteUsernameHash` |
| 4 | Set/replace link | `PUT /v1/accounts/username_link` | auth WS |
| 5 | Delete link | `DELETE /v1/accounts/username_link` (REST) | gRPC `Accounts/DeleteUsernameLink` — **not used** by signal-cli |
| 6 | Look up by hash | `GET /v1/accounts/username_hash/{hash}` | unauth WS |
| 7 | Look up a link | `GET /v1/accounts/username_link/{uuid}` | unauth WS |
| 8 | Own state | `GET /v1/accounts/whoami` | auth WS |

Rows 6 and 7 have a gRPC form in libsignal
(`AccountsAnonymous/LookupUsernameHash`, `LookupUsernameLink`), used only
when the remote-config keys `grpc.AccountsAnonymousLookupUsernameHash` /
`grpc.AccountsAnonymousLookupUsernameLink.2` are set to something other
than `ws`. signal-cli supplies an empty remote-config map, so both go over
the WebSocket (T §3.6).

Row 3 is the exception: signal-cli's delete path calls libsignal's typed
`AuthUsernamesService.deleteUsernameHash`, which in 0.99.1 *requires*
gRPC. See §4.3.

All auth rows use the standard authenticated-connection credentials
(`Authorization: Basic base64("<ACI>.<deviceId>:<password>")`, T §4);
rows 6 and 7 must be sent **unauthenticated** — the server rejects them
with 400 if credentials are present.

### 2.1 Encoding conventions

- Every byte string in these JSON bodies is **base64url without padding**
  except `usernameLinkEncryptedValue` in the *request* of row 4, which the
  Java client sends base64url **with** padding (the server's decoder
  accepts both; its own serializer emits unpadded).
- UUIDs are the usual lowercase hyphenated strings.
- The path parameter of row 6 is the base64url-no-padding hash (43
  characters); of row 7 the handle UUID.

### 2.2 Request and response bodies

**1. Reserve** — `PUT /v1/accounts/username_hash/reserve`

```json
{"usernameHashes": ["<b64url hash>", "..."]}
```

1 to 20 entries, each decoding to exactly 32 bytes; the list is in
priority order. Response 200:

```json
{"usernameHash": "<b64url hash>"}
```

The returned hash is one of the submitted ones — the first that was free.
It is reserved for the account for **5 minutes**. Calling reserve again
replaces any previous reservation. If the account's already-confirmed
hash is in the list, the server returns it without changing anything.

Status codes: 200 success; 401 auth failed; 409 all hashes taken; 422
malformed body (including any hash that is not 32 bytes); 429 rate
limited.

**2. Confirm** — `PUT /v1/accounts/username_hash/confirm`

```json
{"usernameHash": "<b64url 32 bytes>",
 "zkProof": "<b64url 128 bytes>",
 "encryptedUsername": "<b64url 1..128 bytes>"}
```

`encryptedUsername` is the link blob (CU §6.4); it is nullable on the
server but signal-cli always sends one. The server verifies the proof
against `usernameHash` *before* the rate limiter, using exactly the
verification of CU §5.4. Response 200:

```json
{"usernameHash": "<b64url hash>", "usernameLinkHandle": "<uuid or null>"}
```

Status codes: 200; 401; 409 the hash does not match the account's
reservation (or there is none); 410 the hash is no longer available (the
reservation expired and someone else took it); 422 proof invalid or body
malformed; 429 rate limited. Confirming the hash the account already has
is a no-op success (retry-safe).

**3. Delete username hash.** REST form: `DELETE /v1/accounts/username_hash`,
204 with no body, 401 if unauthenticated. Succeeds even if no username is
set. Clears hash, ciphertext and link together. gRPC form: empty
`DeleteUsernameHashRequest`, empty response.

**4. Set/replace link** — `PUT /v1/accounts/username_link`

```json
{"usernameLinkEncryptedValue": "<base64url blob, 1..128 bytes>",
 "keepLinkHandle": false}
```

With `keepLinkHandle` true *and* an existing handle, the server reuses the
handle and only replaces the ciphertext; otherwise it generates a fresh
random UUID. Response 200:

```json
{"usernameLinkHandle": "<uuid>"}
```

Status codes: 200; 401; 409 the account has no username hash set; 422
malformed (e.g. blob longer than 128 bytes); 429 rate limited.

**5. Delete link** — `DELETE /v1/accounts/username_link`, 204, leaves the
username hash in place, succeeds when there is no link.

**6. Look up by hash** — `GET /v1/accounts/username_hash/{b64url-hash}`,
unauthenticated. 200:

```json
{"uuid": "<aci>"}
```

404 when no account has that hash (empty body); 400 if the request was
authenticated; 422 if the path segment is not valid base64url or does not
decode to 32 bytes; 429 rate limited (per IP). The client treats 404 as
"no such username", and parses `uuid` as an ACI service-id string.

**7. Look up a link** — `GET /v1/accounts/username_link/{uuid}`,
unauthenticated. 200:

```json
{"usernameLinkEncryptedValue": "<base64url blob>"}
```

404 when the handle is unknown or the link was deleted (empty body); 400
authenticated; 422 malformed UUID; 429 rate limited. The client then
decrypts with the entropy from the URL (CU §6.5) and validates the
resulting username; a decryption or validation failure is a client-side
error, not a server one.

**8. whoami** — `GET /v1/accounts/whoami` returns, among other fields,
`usernameHash` (base64url, null/absent if unset) and — in the server's own
response record — `usernameLinkHandle`. signal-cli's `WhoAmIResponse` only
deserializes `usernameHash` (plus `uuid`, `pni`, `number`, `entitlements`),
so the handle is dropped client-side.

### 2.3 gRPC forms (for reference)

Package `org.signal.chat.account`, proto3; service `Accounts`
(authenticated) and `AccountsAnonymous` (unauthenticated), paths
`/org.signal.chat.account.<Service>/<Method>`:

| Method | Request | Response |
|---|---|---|
| `Accounts/ReserveUsernameHash` | repeated bytes `username_hashes` | oneof: bytes `username_hash` \| `UsernameNotAvailable` |
| `Accounts/SetUsernameLink` | bytes `username_ciphertext`, bool `keep_link_handle` | oneof: bytes `username_link_handle` (16 bytes) \| `NoUsernameSet` |
| `Accounts/DeleteUsernameHash` | empty | empty |
| `Accounts/DeleteUsernameLink` | empty | empty |
| `AccountsAnonymous/LookupUsernameHash` | bytes `username_hash` | oneof: `ServiceIdentifier` \| `NotFound` |
| `AccountsAnonymous/LookupUsernameLink` | bytes `username_link_handle` | oneof: bytes `username_ciphertext` \| `NotFound` |

Field numbers inside these messages were not transcribed; a Go client that
follows §2's transport column only needs `DeleteUsernameHash`, whose
request and response are both empty messages.

## 3. Claiming a username (client flow)

signal-cli's "set username" takes one string. If it **contains a `.`** it
is treated as an exact username; otherwise as a bare nickname.

### 3.1 Exact username

1. If the account already has exactly this username, try the refresh flow
   (§5) and stop on success. If the refresh throws, fall through and claim
   it again.
2. Build the single-element candidate list `[username]`, validating it
   (CU §2.2/§2.3, no soft length limits — a 2-character nickname is
   accepted here).
3. Continue at §3.3.

### 3.2 Bare nickname

1. If the account already has a username whose nickname part (everything
   before the **first** `.`) equals this nickname, try the refresh flow
   (§5) and stop on success; on failure fall through.
2. Generate 20 candidates (CU §4) with limits `[3, 32]`.
3. Continue at §3.3.

### 3.3 Reserve, then confirm

1. Hash every candidate (CU §3) and base64url-encode without padding,
   keeping the order.
2. `PUT …/username_hash/reserve` with the list.
3. Find the returned hash in the submitted list. If it is not there, abort
   with the I/O error `Unexpected username response` — this is a
   client-side consistency check, not a server error.
4. Take the candidate at that index as "the" username.
5. Generate a link for it with **fresh** entropy (CU §6.1–§6.4).
6. `PUT …/username_hash/confirm` with the hash, a fresh proof (CU §5) and
   the link blob. The response's `usernameLinkHandle` becomes the link's
   server ID.
7. Store locally: username string, link components (entropy + server ID).
8. Re-resolve the self recipient with the new username and **rotate the
   self storage-service ID** so the change is uploaded (§6).

The link entropy is generated once per confirm and kept; the server ID
comes back in the confirm response, so no separate "create link" call is
needed on this path.

## 4. The other operations

### 4.1 Creating or replacing a link on its own

`PUT /v1/accounts/username_link` with a freshly generated blob and
`keepLinkHandle: false`; the response handle plus the entropy used form
the new link components. signal-cli does this only as a repair step: when
loading an account that has a username but no stored link, it tries up to
**3** times (logging each `IOException`) and silently gives up after that.
The attempt happens during the routine account-state check, i.e. before
most commands that touch the network.

### 4.2 Reclaiming with the old entropy

The refresh flow (§5) re-encrypts the *same* username with the *existing*
entropy (a new IV, so a different blob) and sends it through **confirm**,
not through the link endpoint. The handle in the confirm response replaces
the stored one; the entropy — and therefore the URL's first 32 bytes — is
unchanged, but the URL as a whole changes if the handle changed.

### 4.3 Deleting

`deleteUsername` calls the typed `deleteUsernameHash` (gRPC, §2 row 3),
then clears the stored username and link components. It does not touch the
recipient row for self and does not rotate the storage ID, so the deletion
reaches storage service only on the next unrelated self-record upload
(quirk; see Gap 3).

A Go implementation that does not want to speak gRPC can send the REST
`DELETE /v1/accounts/username_hash` over the authenticated WebSocket
instead; the server exposes both and they have the same effect (Gap 2).

## 5. Refreshing / resynchronising the username

Run on three occasions: right after registering an account; when setting a
username that is already the local one; and never otherwise automatically.

1. If there is no local username, do nothing.
2. `GET /v1/accounts/whoami`; take `usernameHash`.
3. Compute the local username's hash and base64url-encode it.
4. If the server has a hash and it equals the local one — nothing to do.
5. Otherwise resynchronise:
   - if there is **no stored link**: reserve the single local hash, then
     confirm it with a brand-new link (fresh entropy), and store the link;
   - if there **is** a stored link: skip reserve and go straight to
     confirm, re-encrypting the username with the stored entropy (§4.2);
     store the new handle.
   - rotate the self storage-service ID.
6. If any of that fails with a status-code error (any non-2xx), **clear**
   the local username and link, rotate the self storage ID, and propagate
   the error. A pure network error leaves the local state alone.

Skipping the reserve in the second branch is deliberate: confirming the
hash an account already holds is a no-op success on the server, so this
doubles as "rotate my link ciphertext".

## 6. Local storage and sync

### 6.1 Account file

The account JSON holds three username-related fields:

| Key | Type | Content |
|---|---|---|
| `username` | string or null | the full username with discriminator, case as typed. An empty string read from the file is normalised to null |
| `usernameLinkEntropy` | string or null | 32 entropy bytes, **standard** base64 with padding |
| `usernameLinkServerId` | string or null | the link handle as a hyphenated UUID string |

The link is considered present only when both link fields are non-null.
(Legacy pre-migration account files keep the account's phone number under
a key literally named `username`, and the actual Signal username under
`usernameIdentifier`; the current format uses `username` for the Signal
username.)

### 6.2 Recipient database

The recipient table has a `username TEXT UNIQUE` column alongside
`number`, `aci`, `pni`. A recipient address is the 4-tuple
(ACI, PNI, number, username) and must have at least one of them. Lookups
by username are exact string matches on that column, so a differently
cased spelling of the same username is a different row.

### 6.3 Storage service

AccountRecord (proto field numbers, C2 `StorageService.proto`):

| Field | # | Content |
|---|---|---|
| `username` | 33 | the full username, or `""` when unset |
| `hasCompletedUsernameOnboarding` | 34 | bool, merged with logical OR; signal-cli never sets it itself |
| `usernameLink` | 35 | message: `entropy` = 1 (bytes, 32), `serverId` = 2 (bytes, 16 UUID), `color` = 3 (enum) |

`UsernameLink.Color`: `UNKNOWN` 0, `BLUE` 1, `WHITE` 2, `GREY` 3, `OLIVE`
4, `GREEN` 5, `ORANGE` 6, `PINK` 7, `PURPLE` 8. signal-cli keeps the
colour as a string in its configuration store and round-trips it; an
unknown name would throw, and a missing one is sent as `UNKNOWN`.

ContactRecord carries a contact's username in field **8** (`string
username`, `""` when unknown).

Merge rules:

- AccountRecord: the remote `username` and `usernameLink` win outright;
  `hasCompletedUsernameOnboarding` is OR-ed.
- Applying a remote AccountRecord locally sets the account username (empty
  → null) and, if `usernameLink` is present, the link components and the
  colour.
- ContactRecord: `username` takes the remote value if non-empty, else the
  local one.
- Uploading the local AccountRecord writes the self recipient's username
  (or `""`) and, when link components exist, the entropy/serverId/colour.

There is an incoming **sync message type 26 `usernameChange`** (an empty
message); signal-cli does not parse it (sync.md §2).

## 7. signal-cli surface

### 7.1 `updateAccount -u/--username` and `--delete-username`

Full argument table, help text, exit codes and the `--help` transcript are
in `spec/surface/cli/updateAccount.md`. Username-specific behaviour:

- The two options are mutually exclusive.
- `-u VALUE`: VALUE is either a bare nickname (`test`) or a full username
  (`test.000`); the dot decides which path of §3 runs.
- After success the command reads back the stored username and link and
  prints them:
  - plain text: `Your new username: <username> (<url>)`, where `<url>` is
    the `signal.me` URL (CU §6.6) or `-` when there is no link;
  - JSON: one object, keys in order, each omitted when null:
    `{"username":"…","usernameLink":"…"}`.
- `--delete-username` prints nothing on success.

Error mapping (the command's own wrapping; exits per
`spec/surface/cli/index.md`):

| Cause | Message | Exit |
|---|---|---|
| any status-code failure of reserve/confirm (409, 410, 422, 429, 401/403 included) | `Invalid username: Username is malformed or already taken` | 1 |
| local validation failure (CU §2.5) | `Invalid username: <message> (<ExceptionSimpleName>)`, e.g. `Invalid username: Discriminator cannot be zero (DiscriminatorCannotBeZeroException)` | 1 |
| other I/O failure while setting | `Failed to set username: <detail>` | 3 |
| I/O failure while deleting | `Failed to delete username: <detail>` | 3 |

The first row is a lossy mapping worth reproducing exactly: a 429 rate
limit and a 409 "taken" both surface as the same sentence, because the
status-code exceptions are all subclasses of the one type the manager
catches. The `<detail>` of the generic rows is the underlying exception
message, which for a status-code error would be `[<code>] <reason>`.

Quirk: reading back the link uses a constructor that dereferences the
stored components, so if an account somehow has a username but no link
(possible when the refresh of §5 decides "already set, not refreshing"
while the local link is missing) the command fails with a
`NullPointerException` instead of printing `-`. Observed only by reading
the code path; see Gap 4.

### 7.2 Recipients given as usernames

`u:<username>` is accepted wherever a RECIPIENT is (`send`, `block`,
`sendReaction`, …), and the `-u/--username` option of the send-family
commands takes bare username strings (no `u:` prefix) or full link URLs.
Note that at the top level `-u` is also the short form of
`--account`/`--username`; the collision is harmless because the top-level
option is parsed before the subcommand.

Resolution of one username or link:

1. If the string parses as a `signal.me` link URL (CU §6.6): look up the
   handle (§2 row 7), decrypt with the entropy, validate — the result is a
   username with its hash already computed.
   Otherwise: validate the string as a username and compute its hash.
2. Look for a recipient row whose `username` column matches exactly and
   which already has a service ID; if found, use it.
3. Otherwise look the hash up on the server (§2 row 6). A returned ACI is
   stored as a trusted (ACI, username) pair; 404 or any other failure
   means unresolved.
4. Unresolved recipients become an "unregistered user" send result with
   the address `{uuid: null, number: null, username: "<input>"}`
   (`spec/surface/cli/_send-results.md`). In the identifier string form a
   username recipient is `u:<name>`.

### 7.3 `getUserStatus --username`

Documented in `spec/surface/cli/getUserStatus.md`. Username entries take
the same resolution as §7.2 but with **forced refresh**: the local
recipient table is bypassed and the hash lookup always goes to the server.
A 404 means "not registered"; any other status-code failure becomes
`Unable to check if users are registered: <detail> (<ExceptionName>)`,
exit 3. If the lookup succeeds, the profile of the resulting ACI is
fetched to fill in the "unrestricted sealed sender" flag.

Output keys: `recipient` (the string as given), `username` (the same
string), `uuid`, `isRegistered`; `number` is omitted for username entries.
Phone-number results are printed first, then username results.

Quirk: a **malformed** username (or a link URL that fails to decrypt) does
not produce the friendly "not registered" result — the validation error is
re-thrown as an unchecked exception from inside the resolution helper and
is not caught by the command. See Gap 5.

### 7.4 Where usernames appear in output

- `listContacts` plain text includes `Username: <username or "">`; its
  JSON has a `username` key (string or null)
  (`spec/surface/cli/listContacts.md`).
- Any `recipientAddress` object is `{"uuid", "number", "username"}`, each
  string or null (`spec/surface/cli/_send-results.md`).
- In received-message output the sender is identified by number, else
  ACI, else PNI, else username
  (`spec/surface/cli/_received-message-output.md`); incoming envelopes
  never carry a username, so it only appears when the recipient row
  already has one (from a previous lookup or from storage sync).

## 8. Status-code to exception mapping (shared)

signal-cli's generic conversion, applied to all of the WebSocket calls in
§2:

| Status | Becomes |
|---|---|
| 2xx | success (body parsed as the documented JSON) |
| 401, 403 | `AuthorizationFailedException`, message `Authorization failed!` |
| 413, 429 | `RateLimitException`, retry-after taken from the `retry-after` header (seconds → milliseconds) |
| any other non-2xx | `NonSuccessfulResponseCodeException`, message `[<code>] <reason>` |

All three are `IOException`s, and all three are also
`NonSuccessfulResponseCodeException`s except the generic I/O ones — which
is why §7.1's first row swallows them all into one message.

## Gaps

1. **gRPC field numbers** for the `org.signal.chat.account` messages in
   §2.3 were not transcribed; only the empty `DeleteUsernameHash` pair is
   needed for the flows signal-cli uses, and that one has no fields.
2. **Delete over REST vs gRPC.** §4.3 asserts the REST
   `DELETE /v1/accounts/username_hash` is equivalent to the gRPC call.
   That is from reading the server controller, not from observing
   signal-cli, which only ever sends the gRPC form. If byte-level parity
   with signal-cli's traffic matters, the gRPC form is the faithful one.
3. **No storage-ID rotation on delete.** Deleting the username clears the
   local state without rotating the self storage ID, so other devices
   learn about it only on the next self-record upload. Stated from the
   code path; not observed end-to-end.
4. **The link-readback NPE** in §7.1 is inferred from the code path, not
   reproduced (it needs a registered account in a specific state).
5. **Malformed-username crash** in §7.3 is likewise inferred: the helper
   wraps the validation error in an unchecked exception and no caller
   catches it. The resulting stderr text and exit code are therefore
   unknown — probably a stack trace and the JVM's generic failure exit.
6. **Rate limits.** The server applies separate limiters to reserve,
   confirm, link operations, hash lookup (per IP) and link lookup (per
   IP); their configured values are deployment data and are not specified
   here.
7. **`keepLinkHandle: true`** is exposed by libsignal and the server but
   is never sent by signal-cli 0.14.7 (its "update link" wrapper exists
   but is unused), so its on-the-wire behaviour is documented from the
   server only.
8. **Username holds.** The server has machinery for expiring "username
   holds" (a reclaim window after a username is released); whether and how
   that changes the 409/410 answers on reserve/confirm was not traced.
