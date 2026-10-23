# Profiles: the minimum for registration and a first 1:1 exchange

Spec role: prose only; no code. This file covers only what a fresh
signal-cli 0.14.7 account does with profiles during registration and the
first send:

- the profile key;
- the empty versioned profile uploaded right after registration;
- the profile fetches that feed the sealed-sender decision (`send.md` §3).

Avatars, badges, payments, names set by the user and profile-key
credentials are out of scope.

## Sources

| Ref | What | URL @ version |
|-----|------|---------------|
| F1 | signal-cli | https://github.com/AsamK/signal-cli @ v0.14.7 (b01b6b370dc063599a1a2b9fde0f5ff4e2d78fe8): `lib/src/main/java/org/asamk/signal/manager/helper/{ProfileHelper,UnidentifiedAccessHelper,SyncHelper,SendHelper}.java`, `.../manager/internal/{RegistrationManagerImpl,ManagerImpl}.java`, `.../manager/util/{ProfileUtils,KeyUtils}.java`, `.../manager/api/Profile.java`, `.../manager/SignalAccountFiles.java` |
| F2 | signal-service-java (Turasa fork) | https://github.com/Turasa/libsignal-service-java @ v2.15.3_unofficial_151 (66956cb4a0394d50cee3590209baf58312084a44): `lib/libsignal-service/src/main/java/org/whispersystems/signalservice/api/profiles/{ProfileApi.kt,SignalServiceProfileWrite.kt,SignalServiceProfile.java,AvatarUploadParams.java}`, `.../api/crypto/ProfileCipher.java`, `.../api/services/ProfileService.java`, `.../api/util/AcceptLanguagesUtil.java` (path as in the tree) |
| F3 | Reference distribution, offline | `jshell` over `/work/third_party/signal-cli-0.14.7/lib/*.jar`, no network: the serialized shape of a profile write body, plus the profile-key version and commitment length for a fixed key and ACI (§2.3) |

## 1. The profile key

- 32 bytes from a CSPRNG, created together with the local account before
  `register` (F1 `KeyUtils.createProfileKey`, `SignalAccountFiles`). It
  never changes during the flows in scope.
- Its UAK goes into account attributes (`registration.md` §4.2,
  `api.md` §5.1).
- It is put into every 1:1 DataMessage to a non-blocked contact
  (`content.md` §4.4). This is how the recipient learns it.

## 2. Initial profile upload after registration

Step 4 of `registration.md` §7 sets an empty profile: no name, about,
emoji or avatar. On the wire this is two requests, plus a no-op:

### 2.1 Own profile read first

The current own profile is read first, which fetches it if there is none
stored or the stored one is more than 6 hours old (§3):
`GET /v1/profile/{ownAci}/{version}` over the authenticated WebSocket.
For a brand-new account this most likely fails, since no versioned profile
exists yet. The failure is logged and ignored (§3.3).

### 2.2 `PUT /v1/profile`

Sent over the authenticated WebSocket with
`content-type:application/json`. The body has these members in this
order, with null members emitted (F3):

| Member | Value for the initial empty profile |
|--------|-------------------------------------|
| `version` | profile-key version for (profile key, own ACI): 64 lowercase hex characters (§2.3) |
| `name` | encrypt(UTF-8 of `""`, padded to **53** bytes): 81 bytes, standard base64 with padding |
| `about` | encrypt(`""`, padded to **128**): 156 bytes, padded base64 |
| `aboutEmoji` | encrypt(`""`, padded to **32**): 60 bytes, padded base64 |
| `paymentAddress` | `null` |
| `phoneNumberSharing` | encrypt(one byte: `0x01` if the sharing mode is "everybody", else `0x00`; padded to 1): 29 bytes. A fresh account sends `0x00`. |
| `avatar` | `true` |
| `sameAvatar` | `true` |
| `commitment` | profile-key commitment for (profile key, own ACI): 97 bytes, padded base64 (§2.3) |
| `badgeIds` | `[]` |

Notes:

- `avatar: true, sameAvatar: true` is what signal-cli sends when the
  avatar argument is "not given". It means "keep whatever is there", even
  on a fresh account (F1 `ProfileHelper.setProfile`, F2
  `AvatarUploadParams.unchanged(true)`).
- The name field is the given name, or given and family names joined by
  one `0x00` byte. Both are empty here.
- Padding targets for non-empty values, used only if names are set later:
  - name: 53 bytes if its UTF-8 length is at most 53, else 257;
  - about: 128 if its length is at most 128, 254 if under 254, else 512;
  - emoji: always 32.

**Field encryption** (F2 `ProfileCipher`):

1. Right-pad the plaintext with zero bytes to the target length. The
   plaintext must not be longer than the target.
2. Pick a fresh random 12-byte nonce.
3. Encrypt with AES-256-GCM, using the 32-byte profile key as the key, a
   128-bit tag and no associated data.
4. The field value is nonce ‖ ciphertext ‖ tag, which is target + 28
   bytes.

Decryption reverses this. For strings, trailing zero bytes are then
stripped. For the boolean, the first byte is tested for non-zero.

The response is a JSON string body. It is ignored here: it matters only
when a new avatar is uploaded.

### 2.3 Profile-key version and commitment (zkgroup)

Both values come from libsignal's zkgroup:

- the version is a 32-byte value, hex-encoded;
- the commitment is a 97-byte serialized group-element commitment.

Their derivations are **not specified in this spec set** (see Gaps).
Reference values, computed offline with the bundled jar (F3):

- profile key: 32 zero bytes;
- ACI: `00000000-0000-4000-8000-000000000001`;
- version: `fcd0715a47cd726483fb0ff70d03b3f4c7329fe444d21cd61e36723f971b3b32`;
- commitment length: 97 bytes.

### 2.4 Afterwards

The profile is stored locally as the own profile. A "fetch profile" sync
message to linked devices would follow, but it is skipped because a fresh
account is not multi-device.

## 3. Fetching a profile

### 3.1 When

signal-cli keeps one stored profile per recipient, with a last-update
time. A fetch happens when the stored profile is missing or more than
**6 hours** old, at the moment the profile is needed. For a 1:1 send, the
recipient's profile is needed, and then the account's own (`send.md` §3).

### 3.2 Request

`GET` over the authenticated WebSocket. The path depends on what is known
(F2 `ProfileService.getProfile`):

| Known | Path |
|-------|------|
| no profile key for the target | `/v1/profile/{serviceId}` |
| profile key known (target must be an ACI) | `/v1/profile/{aci}/{version}`, where `version` is the profile-key version of (their profile key, their ACI) |

- A profile-key credential variant (`…/{version}/{request}?credentialType=expiringProfileKey`)
  exists, but signal-cli uses it only when it needs a credential, which is
  for groups. A 1:1 send does not.
- Header: `Accept-Language` in the weighted form (`api.md` §2.3), for
  example `en;q=1`.
- If signal-cli already holds sealed-sender access for the target from
  stored data, the GET goes over the unauthenticated WebSocket with
  `Unidentified-Access-Key` instead, and is repeated on the authenticated
  socket if that returns 401. For a first contact there is no stored data,
  so it is authenticated.

### 3.3 Response handling

The members used for this scope:

| Member | Use |
|--------|-----|
| `identityKey` | base64 33-byte identity key. **Saved into the identity store** for the target's service ID, so a profile fetch can introduce or change a peer identity before any message is exchanged. |
| `unrestrictedUnidentifiedAccess` | true gives mode *unrestricted*. For the account's own profile, the local "unrestricted" setting is also updated to match. |
| `unidentifiedAccess` | base64 verifier. Mode *enabled* if it matches HMAC-SHA256(UAK(their profile key), 32 zero bytes) (`api.md` §5.1). Otherwise *disabled*. Without a profile key the mode can only be unrestricted or disabled. |
| `capabilities` | stored |
| `name`, `about`, `aboutEmoji`, `avatar`, `paymentAddress`, `phoneNumberSharing`, `badges`, `credential` | decrypted or stored only when the profile key is known; not needed here |

On success, the profile is stored with last-update = now, and the target
is marked registered.

On **any** failure, a profile is stored with:

- last-update = now;
- access mode *unknown*;
- empty capabilities.

A 404 additionally marks the target unregistered. Because last-update is
set on failure too, **a failed fetch is not retried for 6 hours**, and
sends during that window are unsealed (`send.md` §7).

## 4. Is the upload needed for a first exchange?

Not strictly, for message delivery:

- peers can still fetch the unversioned profile (identity key and access
  flags);
- messages flow regardless.

But signal-cli always does it, and it is what makes the own profile
fetchable by version. A byte-compatible client should send the same
`PUT /v1/profile` after registration. That requires the zkgroup version
and commitment (§2.3).

## Gaps

- **zkgroup derivations** (profile-key version and commitment) are not
  specified anywhere in `spec/` yet. A narrower spec request is needed:
  libsignal `rust/zkgroup/src/api/profiles/` and its `crypto/` modules at
  v0.99.1.
- The server's answer to `GET /v1/profile/{aci}/{version}` before any
  profile is uploaded (expected 404) was not observed.
