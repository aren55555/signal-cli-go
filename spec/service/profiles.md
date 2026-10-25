# Profiles: own profile, fetching others, avatars, profile keys (signal-cli 0.14.7)

Spec role: prose only; no code. This is the full profile behaviour of
signal-cli 0.14.7. It **supersedes `spec/service/profile.md` §2 and §3**
where they overlap; profile.md stays valid as the registration-time summary.
Field encryption is in `spec/crypto/profile-cipher.md`; the zkgroup values
(version, commitment, expiring profile-key credential) are in
`spec/crypto/zkgroup-profile.md` and `spec/crypto/zkgroup-credentials.md`
§8. The recipient and contact store, and the commands that list or edit
contacts, are in `spec/service/contacts.md`.

## Sources

| Ref | What | URL @ version |
|-----|------|---------------|
| P1 | signal-cli | https://raw.githubusercontent.com/AsamK/signal-cli/b01b6b370dc063599a1a2b9fde0f5ff4e2d78fe8/ (tag v0.14.7): `lib/src/main/java/org/asamk/signal/manager/helper/{ProfileHelper,UnidentifiedAccessHelper,IncomingMessageHandler,SyncHelper,SendHelper,IdentityHelper,RecipientHelper,ContactHelper,AccountHelper}.java`, `.../manager/internal/ManagerImpl.java`, `.../manager/util/{ProfileUtils,PaymentUtils,Utils,KeyUtils}.java`, `.../manager/api/{Profile,UpdateProfile,PhoneNumberSharingMode}.java`, `.../manager/actions/RetrieveProfileAction.java`, `.../manager/jobs/RefreshRecipientsJob.java`, `.../manager/storage/{AvatarStore,SignalAccount}.java`, `.../manager/storage/recipients/RecipientStore.java`, `.../manager/storage/identities/IdentityKeyStore.java`, `.../manager/config/ServiceConfig.java`; `src/main/java/org/asamk/signal/commands/UpdateProfileCommand.java` |
| P2 | signal-service-java (Turasa fork) | https://raw.githubusercontent.com/Turasa/libsignal-service-java/66956cb4a0394d50cee3590209baf58312084a44/ (tag v2.15.3_unofficial_151): `lib/libsignal-service/src/main/java/org/whispersystems/signalservice/api/profiles/{ProfileApi.kt,SignalServiceProfileWrite.kt,SignalServiceProfile.java,AvatarUploadParams.java,ProfileAndCredential.java}`, `.../api/services/ProfileService.java`, `.../api/NetworkResultUtil.kt`, `.../api/SignalServiceMessageReceiver.java`, `.../internal/push/{PushServiceSocket,ProfileAvatarData,ProfileAvatarUploadAttributes}.java` |
| P3 | Signal-Server (server behaviour only) | https://raw.githubusercontent.com/signalapp/Signal-Server/8edf3488f7d97398db7ffa5184c6b769a272ff59/ (tag v20261022.0.0): `service/src/main/java/org/whispersystems/textsecuregcm/controllers/ProfileController.java`, `.../entities/{CreateProfileRequest,BaseProfileResponse,VersionedProfileResponse,ExpiringProfileKeyCredentialProfileResponse,ProfileAvatarUploadAttributes}.java`, `.../util/ProfileHelper.java`, `.../s3/PostPolicyGenerator.java` |
| P4 | Reference distribution, offline | jshell over `/work/third_party/signal-cli-0.14.7/lib/*`: the serialized write body (`testdata/vectors/profile_cipher.json` `put_profile_body`), the decode results (`decode_profile`), `ExpiringProfileCredentialUtil.isValid` (via `javap -c` of `util-jvm-2.15.3_unofficial_151.jar`), and the string form of service IDs (`org.signal.core.models.ServiceId`: ACI → bare UUID, PNI → `PNI:<uuid>`) |

## 1. Stored state

### 1.1 Per recipient

signal-cli keeps these profile-related values on every recipient row
(column names in `spec/service/contacts.md` §2):

| Value | Meaning |
|-------|---------|
| profile key | 32 bytes, or none. For the account itself, the account's own key is always returned instead of the row value. |
| expiring profile-key credential | serialized zkgroup credential, or none (§5.6) |
| profile | the decoded profile record below, or "no profile" |

The **profile record** has these fields:

| Field | Type |
|-------|------|
| lastUpdateTimestamp | epoch ms. 0 means "never or invalidated". |
| givenName, familyName, about, aboutEmoji | string or null |
| avatarUrlPath | CDN path string (e.g. `profiles/…`) or null |
| mobileCoinAddress | bytes or null (the verified public address) |
| unidentifiedAccessMode | `UNKNOWN`, `DISABLED`, `ENABLED` or `UNRESTRICTED` |
| capabilities | set of `storage`, `storageServiceEncryptionV2Capability` |
| phoneNumberSharingMode | `EVERYBODY`, `NOBODY` or null |

A recipient "has a profile" once a profile record has been stored for it
(technically: the stored capabilities value is not null). Badges are
**never stored** and never shown anywhere.

### 1.2 Account-level

- The account's own **profile key** (§2).
- The local **phone-number sharing mode** (`EVERYBODY`, `CONTACTS` or
  `NOBODY`). It is set by `updateAccount --number-sharing` and is what the
  encrypted `phoneNumberSharing` field publishes: true only for `EVERYBODY`.
- The local **unrestricted unidentified access** flag (account attributes).
- The own profile record is stored on the self recipient like any other
  profile.

### 1.3 Avatar files

These are under the config directory's shared `avatars/` directory
(`spec/service/attachments.md` §6.1):

- `avatars/profile-<id>`: decrypted profile avatar.
- `avatars/contact-<id>`: contact-sync avatar.

`<id>` is the recipient's phone number if known, else its ACI (bare UUID),
else its PNI in the form `PNI:<uuid>`. The directory is created with mode
0700 on first write. The file is written directly (no temporary file and
rename).

## 2. The own profile key

- 32 random bytes, created with the account (`spec/service/profile.md` §1).
- It is sent in every 1:1 DataMessage to a non-blocked recipient, and in
  dedicated profile-key updates (`spec/service/content.md` §4.4).
- **Receiving the own key:** a DataMessage or story whose sender is the
  account itself (for example a sent transcript from a linked device), and
  that carries a 32-byte `profileKey`, **replaces the account's own profile
  key** with that value.
- It changes otherwise only by rotation (§7).

### 2.1 Learning other recipients' keys

A peer's key is stored when it arrives in any of these:

- the `profileKey` of a DataMessage or story message from that peer (a
  length other than 32 is ignored);
- group state and changes (`spec/service/groups.md` §4.4);
- storage-service records (`spec/service/storage.md`, written in parallel).

Storing a key for a recipient works as follows:

- If the key equals the stored one **and** the stored profile's access mode
  is `ENABLED` or `UNRESTRICTED` (or there is no stored profile), nothing
  happens.
- Otherwise the key is written, the stored credential is **cleared**, and
  the profile's **lastUpdateTimestamp is reset to 0**. The next time the
  profile is needed it is fetched again (§5.1), now versioned and decrypted.
  This also happens when the same key arrives again while the mode is
  `UNKNOWN` or `DISABLED`.

(Keys copied during a recipient merge do not reset the timestamp;
`spec/service/contacts.md` §4.3.)

## 3. Writing the own profile

The own profile is written by:

- registration (`spec/service/profile.md` §2);
- `updateProfile` (§4);
- profile-key rotation (§7).

All three use the same procedure.

### 3.1 New values

1. **Read the current own profile.** This fetches it first if the stored
   one is missing or 6 hours old (§5.1). A failed fetch is logged and
   ignored (§5.5).
2. Start from the stored own profile, or from an empty record if there is
   none. Copying a record keeps every field **except
   `phoneNumberSharingMode`, which is lost** (it becomes null).
3. Each of given name, family name, about, about emoji and MobileCoin
   address that the caller supplied **replaces** the stored value. Values
   not supplied stay as they are. An empty string is a supplied value (it
   clears the field).
4. Derive the wire values:
   - name: the given and family name joined as in
     `spec/crypto/profile-cipher.md` §2.2. A null given name counts as
     `""`. With a null family name, only the given name is used.
   - about, about emoji: the value, or `""` if null.
   - payment address: none if the MobileCoin address is null. Otherwise a
     `PaymentAddress` signed with the ACI identity key
     (profile-cipher.md §4.1).
   - phone-number sharing: true if and only if the local sharing mode is
     `EVERYBODY`.

**Quirk:** if the own profile has never been fetched successfully, and no
own profile was stored by an earlier write, step 2 starts empty. Then every
field the caller did not supply is sent as empty. For example,
`updateProfile --about x` in that state also clears the name on the server.

### 3.2 Avatar modes

| Caller asks for | `avatar` | `sameAvatar` | Upload | Server effect (§3.4) |
|-----------------|----------|--------------|--------|----------------------|
| nothing (`updateProfile` without avatar options, registration) | true | true | no | keep the avatar of **the same profile version** |
| a new image (`--avatar PATH`) | true | false | yes (§3.5) | new object name, old object deleted |
| removal (`--remove-avatar`) | false | true | no | avatar cleared, old object deleted |
| rotation (§7) with a local own avatar file | true | false | yes, from `avatars/profile-<self id>` | as "new image" |
| rotation without a local own avatar file | true | true | no | kept only if the same version exists, which it does not after rotation, so effectively **no avatar** |

`--avatar` accepts a file path or a data URI, with the same parsing as
`send -a` (`spec/service/attachments.md` §7.1 and §7.2, including the
content-type detection). The "files from the data directory are refused"
rule of attachments does **not** apply to avatars. The image is used as is:
no resizing, re-encoding or type check.

### 3.3 `PUT /v1/profile`

Sent over the **authenticated WebSocket**, with header
`content-type:application/json`. The body is a JSON object with exactly these
members, in this order. Byte arrays are standard base64 **with padding**.

| # | Member | Value |
|---|--------|-------|
| 1 | `version` | profile-key version for (own key, own ACI): 64 lowercase hex characters (zkgroup-profile.md §5) |
| 2 | `name` | encrypted name: 81 or 285 bytes |
| 3 | `about` | encrypted about: 156, 282 or 540 bytes |
| 4 | `aboutEmoji` | encrypted emoji: 60 bytes |
| 5 | `paymentAddress` | encrypted payment address (582 bytes), or JSON `null` |
| 6 | `phoneNumberSharing` | encrypted boolean: 29 bytes |
| 7 | `avatar` | boolean (§3.2) |
| 8 | `sameAvatar` | boolean (§3.2) |
| 9 | `commitment` | profile-key commitment: 97 bytes (zkgroup-profile.md §4) |
| 10 | `badgeIds` | always `[]` (signal-cli has no badge support) |

- Every encrypted field uses a fresh random nonce, so the body differs on
  every write even when nothing changed.
- There are no other members and no whitespace (compact JSON). See
  `put_profile_body` in `testdata/vectors/profile_cipher.json` for an exact
  serialization.

### 3.4 Server behaviour (P3)

- Validation: `version` must be 64 hex characters and `commitment` must be
  present and valid. Ciphertext sizes must be exactly as listed in
  profile-cipher.md §2.1, §3 and §4.2 (an empty array is also accepted for
  all but `name`). A malformed body gets 400 or 422.
- 401: authentication failed.
- 403: a non-empty `paymentAddress` was sent from a region where payments
  are disallowed (by phone-number prefix, or by the caller's ASN region for
  accounts without a number), and the stored profile of this version has
  no payment address yet.
- 412: the account has the `profiles_v2` capability (signal-cli never sets
  it).
- Avatar, decided per request:
  - `avatar=false`: **clear**. The stored avatar is null.
  - `avatar=true, sameAvatar=false`: **update**. A new object name
    `profiles/<24 characters>` is generated (16 random bytes, URL-safe
    base64 with `=` padding; for example
    `profiles/AAECAwQFBgcICQoLDA0ODw==`).
  - `avatar=true, sameAvatar=true`: **unchanged**. The avatar of the
    profile stored **under the same `version`** is kept, but only if it
    starts with `profiles/`. Otherwise there is none.

  For clear and update, the previous object of that version is deleted
  from the CDN bucket.
- `badgeIds`: badges listed are made visible in the listed order, and
  **badges not listed become invisible**. So every signal-cli profile write
  hides all of the account's badges.
- The account's "current profile version" is set to `version`. This affects
  credential issuance and payment-address visibility (§5.3).
- Response 200:
  - for an avatar **update**: the upload form (§3.5) as a JSON body;
  - otherwise: an **empty body**.

### 3.5 Avatar upload

The 200 body for an avatar update is:

```
{"key":"profiles/…","credential":"<accessId>/<yyyyMMdd>/<region>/s3/aws4_request",
 "acl":"private","algorithm":"AWS4-HMAC-SHA256","date":"<yyyyMMdd'T'HHmmss'Z'>",
 "policy":"<base64 policy>","signature":"<lowercase hex>"}
```

(Member order as the server emits it. The client must not depend on order.
Line breaks above are for readability only.) The policy allows 1 to
10 485 760 bytes and any `Content-Type`, and expires 30 minutes after
`date`.

signal-cli then uploads at once:

- `POST <CDN 0 base URL>/` (empty path, so the URL is the CDN 0 base URL plus
  one `/`, for example `https://cdn-staging.signal.org/`). This uses the
  plain HTTPS client of `spec/service/attachments.md` §4.7: no chat
  authentication, no WebSocket. The host is the configured CDN 0 host
  (attachments.md §4.3).
- Body: `multipart/form-data` with these parts **in this order**:

  | # | Part name | Value |
  |---|-----------|-------|
  | 1 | `acl` | form `acl` |
  | 2 | `key` | form `key` |
  | 3 | `policy` | form `policy` |
  | 4 | `Content-Type` | the avatar's content type (§3.2) |
  | 5 | `x-amz-algorithm` | form `algorithm` |
  | 6 | `x-amz-credential` | form `credential` |
  | 7 | `x-amz-date` | form `date` |
  | 8 | `x-amz-signature` | form `signature` |
  | 9 | `file` | filename `file`, part content type = the avatar's content type, content = the encrypted avatar (profile-cipher.md §5.1), length = image length + 28 |

- Any 2xx is success; anything else is an error. The CDN response body is
  ignored.
- The resulting avatar path is the form's `key`.

This is the same form-post mechanism as sticker uploads (attachments.md
§4.9), except for the content type of part 4 and the file part.

**Failure ordering:** the `PUT` has already succeeded when the upload runs.
If the upload fails, the server profile points at an object that was never
written. signal-cli reports the error (§4.4) and stores **nothing** locally
for this write (§3.6 does not happen). There is no retry.

### 3.6 After a successful write

In this order:

1. **Local avatar file** (self id = own phone number):
   - new image: the **plaintext** image bytes are copied to
     `avatars/profile-<own number>`;
   - removal: that file is deleted if present;
   - otherwise: untouched.
2. **Stored own profile** = the record from §3.1, with `avatarUrlPath` set
   to the form `key` **only for a new image**. Quirks:
   - `lastUpdateTimestamp` keeps the **old** value (it is not set to now),
     so the 6-hour clock is not restarted by a write;
   - after `--remove-avatar`, `avatarUrlPath` keeps the **old** path until
     the next successful fetch, so `listContacts` JSON may still show
     `hasAvatar: true` for the account itself;
   - `phoneNumberSharingMode` is null (§3.1 step 2).
3. Only for `updateProfile`: a **sync message** `fetchLatest { type =
   LOCAL_PROFILE (1) }` to the account's own linked devices
   (`spec/service/sync.md` §6.10). It is sent only when the account is
   multi-device, and its result is ignored.

`updateProfile` does **not** queue a storage-service sync, and does not
send the new profile to contacts. Peers see the change at their next fetch.

## 4. `updateProfile`

The command surface (options, help, parse errors) is in
`spec/surface/cli/updateProfile.md`.

### 4.1 Option mapping

| Option | Effect |
|--------|--------|
| `--given-name X`, `--name X` | given name := X |
| `--family-name X` | family name := X |
| `--about X` | about := X |
| `--about-emoji X` | about emoji := X |
| `--mobile-coin-address B64` | MobileCoin address := base64-decoded bytes (standard alphabet). Invalid base64 is an uncaught error (stack trace, exit 2) **before** anything is sent. |
| `--avatar PATH` | avatar mode "new image" |
| `--remove-avatar` | avatar mode "removal" |

- No option given: a write still happens, with all current values and avatar
  "unchanged". This renews the ciphertexts and hides badges (§3.4).
- There is no way to clear the MobileCoin address: an empty string decodes
  to an empty byte string, which is then signed and published as an empty
  address.
- There is no option for badges or phone-number sharing (sharing comes
  from the account setting, §1.2).

### 4.2 Sequence

1. Write the own profile (§3) with the values above.
2. Send the LOCAL_PROFILE fetch-latest sync (§3.6 step 3).

### 4.3 Output

Nothing on stdout, in both output modes, on success. Exit 0.

### 4.4 Errors

| Condition | stderr | Exit |
|-----------|--------|------|
| Avatar file cannot be opened | `Update profile error: <path> (No such file or directory)` (the JDK's `FileNotFoundException` text; other errno texts are possible) | 3 |
| Bad data URI | `Update profile error: <parser message>` | 3 |
| `PUT` 401 | `Update profile error: Authorization failed!` (Gap 2) | 3 |
| `PUT` 413 or 429 | `Update profile error: Rate Limited` (Gap 2) | 3 |
| `PUT` 403 (payments region) | `Update profile error: <message>` (Gap 2) | 3 |
| other status or network error, upload failure | `Update profile error: <message>` | 3 |
| name, about or emoji too long (profile-cipher.md §2.1) | uncaught error, stack trace | 2 |
| invalid base64 MobileCoin address | uncaught error, stack trace | 2 |

A failure of the own-profile fetch in §3.1 step 1 is **not** an error. It is
logged as `Failed to retrieve profile for <identifier>, ignoring: <msg>`.

### 4.8 Related warning on send

Every `send` (and the other single-message send commands that go through
the same path) reads the own profile (fetching it if 6 h old). If there is
no own profile, or its display name is empty, it logs at WARN level:

`No profile name set. When sending a message it's recommended to set a profile name with the updateProfile command. This may become mandatory in the future.`

In the default log format this goes to stderr; it does not change the exit
code.

## 5. Fetching profiles

### 5.1 When

Two entry points:

- **Lazy (6-hour rule):** if the stored profile is missing, or
  now − lastUpdateTimestamp ≥ 6 h (21 600 000 ms), fetch. Otherwise use the
  stored one. lastUpdateTimestamp = 0 (§2.1) always triggers a fetch.
- **Forced:** fetch regardless of age.

Who asks:

| Trigger | Mode | Target |
|---------|------|--------|
| 1:1 send: sealed-sender decision (`spec/service/send.md` §3) | lazy | each recipient, then self |
| own profile write (§3.1), send warning (§4.8) | lazy | self |
| `listContacts` without recipient arguments | lazy, up to 10 in parallel | every listed recipient that is not marked unregistered |
| `listContacts` with recipient arguments | **forced**, up to 10 in parallel | the given recipients |
| `getUserStatus` | lazy | each registered result (`spec/service/contacts.md` §8) |
| `getAvatar -p` | lazy | that recipient |
| sender name for received-message output (`spec/service/contacts.md` §3.3) | lazy | the sender, only if no nickname or contact name |
| incoming message that fails with an **untrusted identity** | **forced** | the sender |
| incoming sync `fetchLatest LOCAL_PROFILE` | **forced** | self |
| a send rejected for identity mismatch without a key in the response | **forced** | that recipient |
| group operations needing a profile-key credential (§5.6) | credential fetch if none is valid | each member concerned, up to 10 in parallel |

There is **no periodic background profile refresh**. The daily
`RefreshRecipientsJob` refreshes CDSI registrations, not profiles
(`spec/service/cdsi.md`).

### 5.2 Request

The request is a WebSocket `GET` with header `Accept-Language` in the
weighted form built from the JVM default locale
(`spec/service/api.md` §2.3), for example exactly `en;q=1` for an English locale.
The path depends on what is known:

| Known | Path |
|-------|------|
| no profile key | `/v1/profile/<serviceId>` |
| profile key, no credential needed | `/v1/profile/<aci>/<version>` |
| profile key, credential needed (§5.6) | `/v1/profile/<aci>/<version>/<request>?credentialType=expiringProfileKey` |

- `<serviceId>` is the target's service ID string: a bare UUID for an ACI,
  `PNI:<uuid>` for a PNI-only recipient. A recipient known only by phone
  number is first resolved through CDSI. If that fails, signal-cli uses the
  all-zero UUID `00000000-0000-0000-0000-000000000000`, and the server
  answers 404 or 400.
- `<version>`: the hex profile-key version of (their key, their ACI).
- `<request>`: the 329-byte `ProfileKeyCredentialRequest` in **lowercase
  hex** (zkgroup-credentials.md §8.1 and §8.2), from a fresh random request
  context per fetch.
- If a profile key is known but the target's service ID is a PNI, the fetch
  fails locally ("ServiceId must be an ACI if a profile key is available!")
  without any request, and is handled as a failure (§5.5).

**Socket choice.** If sealed-sender access to the target can be computed
**from stored data only** (no fetch), the request goes over the
**unauthenticated** WebSocket with `Unidentified-Access-Key` (the target's
UAK, or 16 zero bytes for unrestricted; `spec/service/api.md` §5.2).
Stored data means a stored target profile with mode `ENABLED` (and a stored
key) or `UNRESTRICTED`, plus a sender certificate, which may itself be
fetched. If that request returns **401**, the same request is repeated on
the authenticated WebSocket. With no such access, the request goes over the
authenticated WebSocket directly.

### 5.3 Response (P3)

On 200, a JSON object. Members read by signal-cli:

| Member | Encoding | Use |
|--------|----------|-----|
| `identityKey` | base64 (server sends it unpadded), 33 bytes | saved to the identity store (§5.4 step 6) and used to verify the payment address |
| `unidentifiedAccess` | padded base64, 32 bytes, or null | verifier (profile-cipher.md §7) |
| `unrestrictedUnidentifiedAccess` | boolean | access mode |
| `capabilities` | object of name → boolean | only `storage` and `ssre2` are used (§5.4). The JSON layer also parses `usernameChangeSyncMessage` but does not use it. Others are ignored. |
| `badges` | array | **ignored** |
| `uuid` | service ID string | the identity key is stored under this ID |
| `name`, `about`, `aboutEmoji` | padded base64 or absent | profile-cipher.md §2 |
| `avatar` | string or absent | CDN path |
| `paymentAddress` | padded base64 or absent | profile-cipher.md §4 |
| `phoneNumberSharing` | padded base64 or absent | profile-cipher.md §3 |
| `credential` | padded base64, 497 bytes, or null | only on credential requests (§5.6) |

Server facts that matter to the client:

- `GET /v1/profile/<aci>/<version>` for a version the server does not hold
  returns **200** with the base members only. `name`, `about`, `aboutEmoji`,
  `avatar`, `paymentAddress`, `phoneNumberSharing` are absent or null. This
  happens before the target's first profile write, or after it rotated its
  key and the requester holds an old key. It is **not** a 404.
- `paymentAddress` is returned only if `<version>` is the account's current
  profile version (or the account has none).
- `credential` is non-null only if `<version>` is the account's current
  version. It expires at the start of the UTC day 7 days after issue
  (truncated to days).
- For a PNI path, the server returns the PNI identity key, `unidentifiedAccess`
  null, `unrestrictedUnidentifiedAccess` false, the capabilities, empty
  `badges`, and `uuid` = `PNI:<uuid>`. Unidentified access is not accepted for
  PNI paths, so only the authenticated socket works there.
- 404: no such account. 401: neither valid authentication nor a valid
  access key. 429: rate limited (authenticated requests count against a
  per-requester profile limit). 400: malformed path or bad credential
  request.

### 5.4 Processing a successful response

In this order, for target recipient R:

1. **Credential.** For a credential request, or whenever R's stored
   credential is not currently valid (§5.6), store the credential from this
   response. For a non-credential response that value is "none", so an
   expired credential is cleared.
2. Remember R's currently stored profile P_old (may be none).
3. **With a profile key:**
   1. Avatar step (§6.1), using the response's `avatar` member.
   2. Decode the response (profile-cipher.md §6). On success, the new record
      P_new is: lastUpdateTimestamp = now; names, about, emoji, payment
      address, phone-number sharing and avatarUrlPath (= `avatar`) **all
      from the response**, nulls included; access mode and capabilities as
      below.
4. **Without a key, or if decoding failed** (a tag mismatch or a too-short
   field): P_new = a copy of P_old (or an empty record) with
   lastUpdateTimestamp = now, the access mode computed without a key
   (`UNRESTRICTED` or `DISABLED`), and the capabilities from the response.
   The copy keeps the old names, about and avatar path, but **drops
   phoneNumberSharingMode**. A failed decode is logged only at debug level.
   - Side effect: on a failed decode with a changed `avatar`, the avatar
     step 3.1 already ran and recorded the new path, and P_new then writes
     back P_old's path. The download is repeated on the next fetch.
5. **Self only:**
   - if `unrestrictedUnidentifiedAccess` differs from the local unrestricted
     flag, the local flag is set to the response value;
   - on a **primary** device, if P_old existed without
     `storageServiceEncryptionV2Capability` and P_new has it, a forced
     storage-service sync is queued.
6. **Identity.** Base64-decode `identityKey` and parse it as a 33-byte
   identity key. Save it into the identity store under `uuid` from the
   response:
   - no identity stored yet: store it. Trust is `TRUSTED_UNVERIFIED` under
     `--trust-new-identities on-first-use` or `always`, else `UNTRUSTED`.
     The added timestamp is now.
   - same key stored: nothing changes;
   - different key stored: replace it. Trust is `TRUSTED_UNVERIFIED` only
     under `always`, else `UNTRUSTED`. The added timestamp is now.

   An unparsable key logs `Got invalid identity key in profile for
   <identifier>` and is skipped. **A profile fetch can therefore introduce
   or change a peer's identity before any message is exchanged.** For self,
   this writes the account's own ACI identity entry.
7. Store P_new on R. Mark R **registered** (clear its unregistered
   timestamp).

**Access mode** (when a profile key is known and the decode succeeded):

- `UNRESTRICTED` if `unrestrictedUnidentifiedAccess` is true;
- else `ENABLED` if the verifier matches (profile-cipher.md §7);
- else `DISABLED`.

Without a key: `UNRESTRICTED` or `DISABLED` only. `UNKNOWN` comes only from
failures (§5.5) and from "never fetched".

**Capabilities:** `storage` = true adds `storage`; `ssre2` = true adds
`storageServiceEncryptionV2Capability`. A response without `capabilities`
cannot be processed and counts as a failure (Gap 4).

### 5.5 Failure handling

Any failure is handled the same way: a non-2xx status other than the 401
fallback of §5.2, a network error, an unparsable body, a credential that
fails verification, or the local PNI error. Steps:

- Log WARN `Failed to retrieve profile for <identifier>, ignoring: <message>`.
  `<identifier>` is the ACI if known, else the phone number, else the PNI.
- Store on R a copy of P_old (or an empty record) with **lastUpdateTimestamp
  = now**, access mode **`UNKNOWN`**, capabilities **empty**. The names and
  the avatar path stay, and phoneNumberSharingMode becomes null.
- **404 only:** mark R unregistered (`spec/service/contacts.md` §4.4). This
  sets the unregistered timestamp and clears "discoverable". If R has both
  an ACI and a PNI, R is split: the PNI and number move to a new recipient
  row.

Because lastUpdateTimestamp is set, **a failed fetch is not retried by lazy
callers for 6 hours**. Forced callers retry at once.

Callers never see the error. They read whatever is stored afterwards.

### 5.6 Expiring profile-key credentials

- A stored credential is **valid** if it exists and now (in whole seconds)
  is strictly less than its expiration time (in seconds).
- When a credential is needed (adding members to a group, own credential
  for joins and profile-key updates; `spec/service/groups.md`) and none is
  valid, a **credential fetch** is made (path 3 of §5.2). It needs R's
  profile key. Without a key, the unversioned path is used and no
  credential results.
- The response's `credential` is verified and turned into a credential as
  in zkgroup-credentials.md §8.4. If verification fails, the **whole fetch
  is a failure** (§5.5). A `credential` that does not deserialize is
  treated as absent.
- Everything else in §5.4 applies to credential fetches too.

## 6. Profile avatars of others

### 6.1 Download decision

This runs in §5.4 step 3.1, only when R's profile key is known:

- If R has no stored profile, or the response `avatar` differs from the
  stored `avatarUrlPath`:
  - `avatar` absent or null: delete `avatars/profile-<id>` if it exists
    (an error is logged and ignored);
  - otherwise: download and decrypt (§6.2) into `avatars/profile-<id>`.

  In both cases, the stored profile's avatarUrlPath is then set to the new
  value, **even if the download failed**.
- If the path is unchanged, nothing is downloaded, even if the local file
  was deleted in the meantime.

The `--ignore-avatars` receive option does **not** affect profile avatars
in 0.14.7. It applies only to contact-sync and group avatars.

### 6.2 Download

- `GET <CDN 0 base URL>/<avatar path>`, for example
  `https://cdn-staging.signal.org/profiles/…`. There is no authentication.
  The client and headers are as for CDN downloads in
  `spec/service/attachments.md` §5.2.
- Size cap **10 MiB** (10 485 760 bytes). A larger `Content-Length`, or more
  bytes received, fails the download.
- The encrypted body goes to a temporary file first. It is then decrypted
  as a stream into the avatar file (profile-cipher.md §5.2), and the
  temporary file is deleted.
- Any error (HTTP, size, I/O, tag mismatch) logs WARN `Failed to download
  profile avatar, ignoring: <message>` and is otherwise ignored. A partially
  written or unverified avatar file may remain.

### 6.3 Reading avatars

`getAvatar -p RECIPIENT` first reads R's profile lazily (§5.1), which may
download the avatar. It then returns the bytes of `avatars/profile-<id>`.
`getAvatar -c` returns `avatars/contact-<id>` (written only by contact
sync). Output formats and errors are in `spec/surface/cli/getAvatar.md`.

## 7. Profile-key rotation

Triggered by `block` when a newly blocked contact shares no group with the
account, or when any group is newly blocked (`spec/service/contacts.md`
§7). Steps:

1. Read the own profile (lazy fetch, §5.1).
2. Generate a new random 32-byte profile key and make it the account's key.
3. Update account attributes (`PUT /v1/accounts/attributes`), now carrying
   the UAK of the new key (attribute body as in `spec/service/registration.md`, UAK derivation in `spec/service/api.md` §5.1).
4. Write the own profile (§3) with no field changes, in rotation avatar
   mode (§3.2). The `version` and `commitment` change with the key, so this
   creates a **new profile version** on the server.
5. Mark the self recipient changed for the storage service and queue a
   storage sync.
6. Send a **profile-key update** (DataMessage `flags = PROFILE_KEY_UPDATE`,
   `profileKey` = new key; `spec/service/content.md` §4.4) to every recipient
   with profile sharing on that is not blocked. The blocked contact had
   profile sharing turned off by the block, so it is excluded.
7. For every v2 group where the account is a member and profile sharing is
   on, update the account's profile key in the group
   (`spec/service/groups.md` §11). Per-group failures log `Failed to update
   group profile key: <msg>`.

Errors in steps 1 to 4 propagate to the `block` command
(`spec/surface/cli/block.md`).

## 8. What other surfaces show from profiles

- `listContacts`: the profile record, plain and JSON
  (`spec/service/contacts.md` §6).
- `getUserStatus`: "unrestricted sealed sender" if the stored or fetched
  profile has mode `UNRESTRICTED` (`spec/service/contacts.md` §8).
- Received-message output: the sender name (`spec/service/contacts.md`
  §3.3).
- Sealed-sender decisions use the access mode and key
  (`spec/service/send.md` §3).

## 9. Corrections to other specs

1. `spec/service/profile.md` Gap 2 (the server's answer to a versioned GET
   before any upload) is answered by §5.3: 200 without the encrypted members.
2. `spec/service/attachments.md` §6.7 says the profile avatar is "fetched
   every time a profile is retrieved with a known profile key, unless
   `--ignore-avatars`". In fact it is fetched only when the path changed or
   no profile was stored (§6.1), and `--ignore-avatars` has no effect on it.
   (A pointer to this section was added to attachments.md §6.7.)

## Gaps

1. The exact `Accept-Language` value depends on the JVM default locale. A Go
   client should derive it from `LANG` the way `spec/service/api.md` §2.3
   specifies. It was not re-observed for profile requests.
2. The exact messages of the 403 payments-region error and of 413 or 429
   in `updateProfile` were not observed. They come from signal-service-java
   exception constructors, and only `Rate Limited` is visible in source.
3. The CDN response to the avatar form `POST`, and the server's answer when
   the policy is violated (for example an over-size avatar), were not
   observed. Expect S3-style 4xx XML. signal-cli prints `Update profile
   error: <message>` with the status in the message.
4. A response without `capabilities` was not tested against the reference.
   The source dereferences it unconditionally, so it would fail inside the
   success handler. Whether that ends in the §5.5 path or an uncaught error
   was not checked.
5. Storage-service interaction (the account record carrying the profile
   name and avatar URL, and contact records carrying profile keys) is out of
   scope here. It belongs to `spec/service/storage.md` (written in parallel).
6. Concurrency: lazy batch fetches run up to 10 at a time. The order of the
   resulting requests on the wire is not deterministic.
