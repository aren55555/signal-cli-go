# Storage service (signal-cli 0.14.7)

Spec role: prose and tables only; no code. This file covers the encrypted
per-account key/value store that Signal clients share through the storage
service: keys, encryption, wire messages, HTTP endpoints, and the
synchronisation algorithm that signal-cli 0.14.7 runs ("storage sync job").

Not repeated here (see the referenced files):

- Account entropy pool format, master key, SVR/PIN, registration lock:
  `svr.md` (parallel). Only the derivations the storage service needs are
  restated in §2.
- The `keys`, `fetchLatest` and `request` sync messages: `sync.md` §6.2,
  §6.7, §6.10. The provisioning message that carries the pool to a new
  linked device: `linking.md` §4.3.
- Host, TLS, OkHttp client, `User-Agent`: `transport.md` and
  `groups.md` §3.1 (the groups API uses the same host and HTTP client).
- Recipient resolution and merging ("trusted" address resolution):
  `contacts.md` (parallel). Profile fetch and profile-key handling:
  `profiles.md` / `profile.md`. Usernames and username links:
  `usernames.md` (parallel). Group state: `groups.md`.
- CLI surfaces: `spec/surface/cli/{listContacts,listGroups,
  listIdentities,listStickerPacks,updateConfiguration,updateAccount,
  block,unblock,sendSyncRequest}.md`. Log line format:
  `spec/surface/cli/index.md` §8.

## Sources

| Ref | What | URL @ version |
|-----|------|---------------|
| S1 | signal-cli | https://github.com/AsamK/signal-cli @ v0.14.7 (`b01b6b370dc063599a1a2b9fde0f5ff4e2d78fe8`): `lib/src/main/java/org/asamk/signal/manager/helper/{StorageHelper,AccountHelper,GroupHelper,ProfileHelper,IncomingMessageHandler,SyncHelper}.java`; `.../manager/syncStorage/{AccountRecordProcessor,ContactRecordProcessor,DefaultStorageRecordProcessor,GroupV1RecordProcessor,GroupV2RecordProcessor,StickerPackRecordProcessor,StorageRecordProcessor,StorageRecordUpdate,StorageSyncModels,StorageSyncValidations,WriteOperationResult}.java`; `.../manager/jobs/SyncStorageJob.java`; `.../manager/actions/SyncStorageDataAction.java`; `.../manager/internal/{ManagerImpl,RegistrationManagerImpl,ProvisioningManagerImpl,JobExecutor}.java`; `.../manager/storage/{SignalAccount,UnknownStorageIdStore}.java`; `.../manager/storage/recipients/RecipientStore.java`; `.../manager/storage/groups/GroupStore.java`; `.../manager/storage/stickers/StickerStore.java`; `.../manager/storage/configuration/ConfigurationStore.java`; `.../manager/storage/identities/IdentityKeyStore.java`; `.../manager/util/{KeyUtils,ProfileUtils}.java`; `.../manager/api/Profile.java` |
| S2 | signal-service-java (Turasa fork, bundled as `signal-service-java-2.15.3_unofficial_151.jar`, `signal-network-…`, `models-jvm-…`) | https://github.com/Turasa/libsignal-service-java @ v2.15.3_unofficial_151 (`66956cb4a0394d50cee3590209baf58312084a44`): `lib/libsignal-service/src/main/java/org/whispersystems/signalservice/api/storage/{SignalStorageCipher,RecordIkm,SignalStorageManifest,StorageId,SignalStorageRecord,SignalRecord,SignalAccountRecord,StorageServiceApi,StorageRecordConverters,AccountRecordExtensions,ManifestRecordIdentifierExtensions,StorageRecordProtoUtil,StorageAuthResponse,IAPSubscriptionId}.{kt,java}`; `.../api/crypto/Crypto.kt`; `.../api/payments/PaymentsConstants.java`; `.../api/profiles/SignalServiceProfile.java`; `.../internal/push/PushServiceSocket.java` (storage request path); `lib/network/src/main/java/org/signal/network/service/StorageServiceService.kt`; `core/models-jvm/src/main/java/org/signal/core/models/{AccountEntropyPool,MasterKey,ServiceId}.kt`, `.../storageservice/{StorageKey,StorageManifestKey,StorageItemKey,StorageCipherKey}.kt` |
| S3 | Reference jar | `/work/third_party/signal-cli-0.14.7/lib/signal-service-java-2.15.3_unofficial_151.jar`: bundled Wire-formatted `StorageService.proto` (field numbers below were read from it and agree with S2's generated classes). Offline `jshell` runs against `lib/*.jar` produced the vectors in `testdata/vectors/storage.json` (§10). |
| S4 | libsignal | https://github.com/signalapp/libsignal @ v0.99.1 (`97801d22dcf9f5bf714f7b8fa3212cdc973ae1c8`): `rust/account-keys/src/lib.rs` (pool alphabet and length, SVR-key derivation, `derive_storage_service_key`, published known-answer test) |
| S5 | storage-service (server behaviour only) | https://github.com/signalapp/storage-service @ v20260713.0.2 (`0da239ba7ddda05306ed7b0c4b81fd627055d888`): `src/main/java/org/signal/storageservice/controllers/StorageController.java`, `.../storage/{StorageManager,StorageManifestsTable,StorageItemsTable}.java`, `.../auth/UserAuthenticator.java`. The deployed server may differ. |
| S6 | Signal-Server (server behaviour only) | https://github.com/signalapp/Signal-Server @ v20261022.0.0 (`8edf3488f7d97398db7ffa5184c6b769a272ff59`): `service/src/main/java/org/whispersystems/textsecuregcm/controllers/SecureStorageController.java`, `.../storage/DeviceCapability.java` |
| S7 | Standards | RFC 5869 (HKDF), RFC 2104 (HMAC), NIST SP 800-38D (GCM), RFC 7617 (Basic auth), proto3 encoding |

## 1. Overview

The storage service holds, per account (keyed by ACI), one **manifest**
and a set of **items**. Each item is an opaque 16-byte **storage ID**
(the "raw ID") and an encrypted value. The manifest is an encrypted list
of `(raw ID, record type)` pairs plus a version number. Clients never
modify an item in place: a changed record is uploaded under a **new**
random raw ID and the old ID is deleted, in the same write that bumps the
manifest version by exactly one.

signal-cli keeps, for each local entity that has a remote counterpart
(each contact, the account itself, each group, each installed or deleted
sticker pack), the raw ID it believes the remote record has, plus the
plaintext of the last remote record it saw (to keep fields it does not
understand). Any local change to such an entity gives it a fresh random
raw ID. A sync then compares the two ID sets (§6).

## 2. Keys

### 2.1 Hierarchy

| Key | Size | Derivation |
|-----|------|------------|
| Account entropy pool (AEP) | 64 ASCII characters from `0-9a-z` | random (each character uniform over the 36-character alphabet `0123456789abcdefghijklmnopqrstuvwxyz`); stored lower-case (S2 lower-cases on construction) (S4) |
| Master key (libsignal: "SVR key") | 32 bytes | HKDF-SHA256 (RFC 5869) with **no salt** (equivalent to 32 zero bytes), IKM = the 64 ASCII bytes of the AEP, info = ASCII `20240801_SIGNAL_SVR_MASTER_KEY`, L = 32 (S4). An account may instead hold a legacy 32-byte "PIN master key" that is not derived from an AEP (see §2.3). |
| Storage service key ("storage key") | 32 bytes | HMAC-SHA256(key = master key, message = ASCII `Storage Service Encryption`) (S2 `MasterKey`, S4) |
| Manifest key for version V | 32 bytes | HMAC-SHA256(key = storage key, message = ASCII `Manifest_` followed by V in decimal, no padding or sign, e.g. `Manifest_2`) (S2 `StorageKey`) |
| Item key, legacy scheme | 32 bytes | HMAC-SHA256(key = storage key, message = ASCII `Item_` followed by the **standard base64 with padding** of the 16-byte raw ID, e.g. `Item_EBESExQVFhcYGRobHB0eHw==`) (S2 `StorageKey`) |
| Item key, recordIkm scheme | 32 bytes | HKDF-SHA256 with no salt, IKM = the manifest's `recordIkm` (32 bytes), info = ASCII `20240801_SIGNAL_STORAGE_SERVICE_ITEM_` followed by the 16 raw ID bytes (binary, not encoded), L = 32 (S2 `RecordIkm`) |

Which item-key scheme applies is decided per manifest: if the manifest
that lists the item has a non-empty `recordIkm` (§4.2 field 4), every item
it lists is encrypted with the recordIkm scheme; otherwise with the legacy
scheme. Reads use the `recordIkm` of the manifest being read; writes use
the `recordIkm` of the manifest being written (S2 `StorageServiceService`).

Vectors: `storage.json` `keys.*` (including libsignal's published
known-answer value for the storage key of a master key of 32 × `0x2a`).

### 2.2 Which storage key signal-cli uses

signal-cli stores up to three of these per account: a PIN master key, a
storage key, and an AEP (S1 `SignalAccount`). The storage key used for
sync is chosen in this order:

1. an explicitly stored storage key (0.14.7 itself only stores one on a
   linked device from a legacy `keys` message, which its parser never
   produces, see `sync.md` §6.7; account files written by older versions
   may still contain one, on any device);
2. else, if a PIN master key is stored: its derived storage key;
3. else, if an AEP is stored: storage key of the AEP's master key;
4. else, on a **linked** device, or on a primary that is **not**
   multi-device: **none** (no sync happens, §6.1);
5. else (multi-device primary without any key): a new AEP is generated and
   persisted, and its derived storage key is used.

### 2.3 How each device obtains the keys

- **Primary, new registration**: registration stores the master key that
  was recovered with `--pin` through SVR (`svr.md`), or none. The AEP is
  cleared, the storage key cleared, the local manifest deleted and the
  local manifest version reset to −1 (S1 `SignalAccount.finishRegistration`).
  An AEP is created lazily (rule 5 above, or when linking a device or
  answering a KEYS request).
- **Linked device**: receives the AEP in the provisioning message
  (`linking.md` §4.3 field 15). The PIN master key and the storage key are
  cleared, the local manifest deleted, version reset to −1. If the
  provisioning message had no AEP the device has no key; each sync attempt
  then sends a KEYS request to the primary (`sync.md` §6.2) and stops.
- **Keys sync message** (`sync.md` §6.7) on any device: a received AEP is
  stored, and the stored PIN master key and storage key are discarded;
  then a sync is queued.

### 2.4 Key change / migration (primary only)

A primary "needs storage key migration" when it holds a stored storage
key or a PIN master key (i.e. any key not derived from an AEP). The sync
job then (§6.4 step 6):

1. keeps the existing AEP, or generates a new one; stores it, which
   discards the PIN master key and storage key;
2. derives the new storage key from the AEP;
3. sends a `keys` sync message (AEP and media root backup key) to its own
   devices (`sync.md` §6.7; skipped when not multi-device);
4. force pushes (§6.6) everything under the new storage key.

Consequences of any storage-key change (by signal-cli or another
client): the remote manifest no longer decrypts with the old key. A
primary that cannot decrypt the manifest force pushes with its own key; a
linked device that cannot decrypt it sends a KEYS request and does
nothing else until it receives a `keys` message. Note that migrating away
from a PIN master key changes the master key and therefore the
registration lock and SVR material (`svr.md`).

## 3. Encryption format

All encrypted values (the manifest's `value`, and each item's `value`) are:

| Bytes | Content |
|-------|---------|
| 0–11 | IV: 12 random bytes (fresh per encryption) |
| 12–(n−17) | AES-256-GCM ciphertext of the plaintext |
| last 16 | GCM tag (128 bits) |

Key: the manifest key for the manifest's version (§2.1), or the item key
for the item's raw ID. **No additional authenticated data.** The plaintext
is the serialized `ManifestRecord` (manifest) or the serialized
`StorageRecord` (item).

Decryption failures (bad tag, value shorter than 12 + 16 bytes) are
"decryption errors" (§6.4, §6.5). A plaintext that is not valid protobuf
is an I/O error instead.

**Storage IDs** (raw IDs) are 16 bytes from a cryptographically secure
random generator, generated fresh for every new or changed record
(S1 `KeyUtils.createRawStorageId`). They carry no structure; the record
type is only known from the manifest.

Vectors: `storage.json` `encryption.*` (fixed IV; the GCM output was
checked by decrypting with the jar's own cipher).

## 4. Protobuf messages

All messages are **proto3** (package `signalservice` in upstream; names
here are descriptive, only numbers and types matter). Unless noted, a
field is singular; proto3 defaults (0, false, empty) are not emitted.
"optional" marks proto3 explicit-presence fields (emitted even when
zero if set).

**Unknown fields and enum values.** The decoder used by signal-cli
(Wire) keeps unknown fields and re-emits them after the known fields on
re-encoding; an unknown value in a known enum field is likewise kept as
an unknown field and reads as the enum's 0 value. signal-cli relies on
this to round-trip records it only partly understands (§5.1).

**Field order on the wire.** Wire emits known fields in the order they
are declared upstream, not by number (e.g. `ManifestRecord` emits 1, 3,
2, 4; `ContactRecord` emits 15 after 2). Any order is valid protobuf; the
vectors in `storage.json` give the exact plaintext bytes they encrypt so
that encryption vectors do not depend on field order.

### 4.1 Transport messages

**StorageManifest** (body of `GET /v1/storage/manifest…`, part of
`WriteOperation`, body of a 409)

| # | Name | Type | Meaning |
|---|------|------|---------|
| 1 | `version` | uint64 | manifest version; selects the manifest key; the server compares it (§5.3) |
| 2 | `value` | bytes | encrypted `ManifestRecord` (§3) |

**StorageItem**

| # | Name | Type | Meaning |
|---|------|------|---------|
| 1 | `key` | bytes | raw storage ID (16 bytes) |
| 2 | `value` | bytes | encrypted `StorageRecord` (§3) |

**StorageItems** (response of `PUT /v1/storage/read`)

| # | Name | Type | Meaning |
|---|------|------|---------|
| 1 | `items` | repeated StorageItem | items found; missing keys are silently omitted; order unspecified |

**ReadOperation** (body of `PUT /v1/storage/read`)

| # | Name | Type | Meaning |
|---|------|------|---------|
| 1 | `readKey` | repeated bytes | raw IDs to fetch |

**WriteOperation** (body of `PUT /v1/storage`)

| # | Name | Type | Meaning |
|---|------|------|---------|
| 1 | `manifest` | StorageManifest | new manifest; required (server answers 400 without it) |
| 2 | `insertItem` | repeated StorageItem | items to store |
| 3 | `deleteKey` | repeated bytes | raw IDs to delete |
| 4 | `clearAll` | bool | delete **all** existing items first; signal-cli never sends `deleteKey` together with `clearAll` |

### 4.2 ManifestRecord (plaintext of the manifest)

| # | Name | Type | Meaning |
|---|------|------|---------|
| 1 | `version` | uint64 | same value as `StorageManifest.version`; signal-cli takes the version of a decrypted manifest from **this** field (the outer one only selects the key) |
| 3 | `sourceDevice` | uint32 | device ID of the writer; signal-cli writes its own device ID; never interpreted |
| 2 | `identifiers` | repeated Identifier | every item of the account |
| 4 | `recordIkm` | bytes | empty, or 32 bytes enabling the recordIkm item-key scheme (§2.1) |

**ManifestRecord.Identifier**

| # | Name | Type | Meaning |
|---|------|------|---------|
| 1 | `raw` | bytes | raw storage ID |
| 2 | `type` | enum Type (varint) | record type; values outside the table are kept as-is (the integer is carried through unchanged when re-writing the manifest) |

| Type | Value | signal-cli 0.14.7 |
|------|-------|-------------------|
| UNKNOWN | 0 | not processed (ID kept) |
| CONTACT | 1 | processed |
| GROUPV1 | 2 | processed |
| GROUPV2 | 3 | processed |
| ACCOUNT | 4 | processed |
| STORY_DISTRIBUTION_LIST | 5 | fetched but not processed (ID kept) |
| STICKER_PACK | 6 | processed |
| CALL_LINK | 7 | fetched but not processed (ID kept) |
| CHAT_FOLDER | 8 | fetched but not processed (ID kept) |
| NOTIFICATION_PROFILE | 9 | fetched but not processed (ID kept) |
| any other | ≥ 10 | never fetched (ID kept) |

"Known types" in §6 means {1, 2, 3, 4, 6}.

### 4.3 StorageRecord (plaintext of an item)

A oneof `record`; exactly one field is set.

| # | Name | Type |
|---|------|------|
| 1 | `contact` | ContactRecord |
| 2 | `groupV1` | GroupV1Record |
| 3 | `groupV2` | GroupV2Record |
| 4 | `account` | AccountRecord |
| 5 | `storyDistributionList` | StoryDistributionListRecord |
| 6 | `stickerPack` | StickerPackRecord |
| 7 | `callLink` | CallLinkRecord |
| 8 | `chatFolder` | ChatFolderRecord |
| 9 | `notificationProfile` | NotificationProfile |

signal-cli dispatches on which of 1, 2, 3, 4, 6 is present (checked in
the order account, groupV1, groupV2, contact, stickerPack); a record with
none of them (5, 7, 8, 9, empty, or a future field) is "unknown" (§6.4).
Note the dispatch is by **content**, not by the manifest type.

**Shared enums.**

| OptionalBool | Value |
|---|---|
| UNSET | 0 |
| ENABLED | 1 |
| DISABLED | 2 |

AvatarColor: A100 = 0, A110 = 1, … A210 = 11 (step 1 per 10). Never
interpreted by signal-cli; carried through.

### 4.4 ContactRecord

| # | Name | Type | Meaning / signal-cli use |
|---|------|------|--------------------------|
| 1 | `aci` | string | ACI, lower-case UUID string |
| 2 | `e164` | string | phone number `+…` |
| 15 | `pni` | string | PNI as a plain UUID string (no `PNI:` prefix) |
| 3 | `profileKey` | bytes | 32-byte profile key |
| 4 | `identityKey` | bytes | 33-byte serialized identity public key (type byte 0x05 + 32) |
| 5 | `identityState` | enum IdentityState | DEFAULT 0, VERIFIED 1, UNVERIFIED 2 |
| 6 | `givenName` | string | profile given name |
| 7 | `familyName` | string | profile family name |
| 8 | `username` | string | username |
| 9 | `blocked` | bool | |
| 10 | `whitelisted` | bool | profile sharing enabled |
| 11 | `archived` | bool | |
| 12 | `markedUnread` | bool | carried through only |
| 13 | `mutedUntilTimestamp` | uint64 | ms |
| 14 | `hideStory` | bool | |
| 16 | `unregisteredAtTimestamp` | uint64 | ms; 0 = registered |
| 17 | `systemGivenName` | string | "contact name" given part |
| 18 | `systemFamilyName` | string | "contact name" family part |
| 19 | `systemNickname` | string | local "nick name" |
| 20 | `hidden` | bool | |
| 21 | `pniSignatureVerified` | bool | |
| 22 | `nickname` | Name | user-set nickname; `Name` = {1 `given` string, 2 `family` string} |
| 23 | `note` | string | |
| 24 | `avatarColor` | optional AvatarColor | carried through only |
| 25 | `aciBinary` | bytes | ACI as 16 raw UUID bytes |
| 26 | `pniBinary` | bytes | PNI as 16 raw UUID bytes (no type prefix) |

Parsing an ACI or PNI from a record: the binary field is preferred; the
string is used only if the binary field is empty or unparsable. A 17-byte
binary value is read as a prefixed service ID and accepted only if it is
of the right kind.

### 4.5 GroupV1Record

| # | Name | Type | Meaning |
|---|------|------|---------|
| 1 | `id` | bytes | 16-byte v1 group ID |
| 2 | `blocked` | bool | |
| 3 | `whitelisted` | bool | signal-cli always writes true |
| 4 | `archived` | bool | |
| 5 | `markedUnread` | bool | carried through |
| 6 | `mutedUntilTimestamp` | uint64 | carried through |

### 4.6 GroupV2Record

| # | Name | Type | Meaning |
|---|------|------|---------|
| 1 | `masterKey` | bytes | 32-byte group master key |
| 2 | `blocked` | bool | |
| 3 | `whitelisted` | bool | profile sharing |
| 4 | `archived` | bool | carried through |
| 5 | `markedUnread` | bool | carried through |
| 6 | `mutedUntilTimestamp` | uint64 | carried through |
| 7 | `dontNotifyForMentionsIfMuted` | bool | carried through |
| 8 | `hideStory` | bool | carried through |
| 9 | — | — | reserved |
| 10 | `storySendMode` | enum | DEFAULT 0, DISABLED 1, ENABLED 2; carried through |
| 11 | `avatarColor` | optional AvatarColor | carried through |
| 12 | `verifiedNameHash` | bytes | SHA-256 of the verified title; carried through (as an ordinary field of the stored record) |

### 4.7 AccountRecord

Reserved numbers: 9, 19, 28, 31, 36, 37, 38, 39. "Used" = read or
written by signal-cli; everything else is carried through (§5.1, §6.4).

| # | Name | Type | Used |
|---|------|------|------|
| 1 | `profileKey` | bytes | yes |
| 2 | `givenName` | string | yes |
| 3 | `familyName` | string | yes |
| 4 | `avatarUrlPath` | string | yes |
| 5 | `noteToSelfArchived` | bool | merge only |
| 6 | `readReceipts` | bool | yes |
| 7 | `sealedSenderIndicators` | bool | yes (unidentified-delivery indicators) |
| 8 | `typingIndicators` | bool | yes |
| 10 | `noteToSelfMarkedUnread` | bool | merge only |
| 11 | `linkPreviews` | bool | yes |
| 12 | `phoneNumberSharingMode` | enum | UNKNOWN 0, EVERYBODY 1, NOBODY 2; yes |
| 13 | `unlistedPhoneNumber` | bool | yes |
| 14 | `pinnedConversations` | repeated PinnedConversation | merge only |
| 15 | `preferContactAvatars` | bool | merge only |
| 16 | `payments` | Payments {1 `enabled` bool, 2 `entropy` bytes} | merge only (§6.4.3) |
| 17 | `universalExpireTimer` | uint32 | merge only |
| 18 | `primarySendsSms` | bool | carried |
| 20 | `preferredReactionEmoji` | repeated string | merge only |
| 21 | `subscriberId` | bytes | merge only |
| 22 | `subscriberCurrencyCode` | string | merge only |
| 23 | `displayBadgesOnProfile` | bool | merge only |
| 24 | `subscriptionManuallyCancelled` | bool | merge only |
| 25 | `keepMutedChatsArchived` | bool | merge only |
| 26 | `hasSetMyStoriesPrivacy` | bool | merge only |
| 27 | `hasViewedOnboardingStory` | bool | merge only |
| 29 | `storiesDisabled` | bool | merge only |
| 30 | `storyViewReceiptsEnabled` | OptionalBool | merge only |
| 32 | `hasSeenGroupStoryEducationSheet` | bool | merge only |
| 33 | `username` | string | yes |
| 34 | `hasCompletedUsernameOnboarding` | bool | merge only |
| 35 | `usernameLink` | UsernameLink | yes |
| 40 | `backupTier` | optional uint64 | carried |
| 41 | `backupSubscriberData` | IAPSubscriberData | merge only |
| 42 | `avatarColor` | optional AvatarColor | merge only (remote wins) |
| 43 | `backupTierHistory` | BackupTierHistory {1 optional uint64 `backupTier`, 2 optional uint64 `endedAtTimestamp`} | carried |
| 44 | `notificationProfileManualOverride` | NotificationProfileManualOverride | carried |
| 45 | `notificationProfileSyncDisabled` | bool | carried |
| 46 | `automaticKeyVerificationDisabled` | bool | carried |
| 47 | `hasSeenAdminDeleteEducationDialog` | bool | carried |
| 48 | `releaseNotesChatArchived` | optional bool | carried |
| 49 | `releaseNotesChatMutedUntilTimestamp` | optional uint64 | carried |
| 50 | `releaseNotesChatBlocked` | optional bool | carried |
| 51 | `releaseNotesChatMarkedUnread` | optional bool | carried |

Sub-messages:

- **PinnedConversation**: oneof `identifier` {1 `contact` Contact, 3
  `legacyGroupId` bytes, 4 `groupMasterKey` bytes, 5 `releaseNotes`
  (empty message)}. Contact = {1 `serviceId` string, 2 `e164` string, 3
  `serviceIdBinary` bytes (16 bytes for an ACI, 17 bytes with type prefix
  for a PNI)}.
- **UsernameLink**: 1 `entropy` bytes (32), 2 `serverId` bytes (16-byte
  UUID), 3 `color` enum: UNKNOWN 0, BLUE 1, WHITE 2, GREY 3, OLIVE 4,
  GREEN 5, ORANGE 6, PINK 7, PURPLE 8.
- **IAPSubscriberData**: 1 `subscriberId` bytes; oneof
  `iapSubscriptionId` {2 `purchaseToken` string, 3 `originalTransactionId`
  uint64}.
- **NotificationProfileManualOverride**: oneof `override` {1
  `disabledAtTimestampMs` uint64, 2 `enabled` ManuallyEnabled {1 `id`
  bytes, 3 `endAtTimestampMs` uint64}}.

### 4.8 StickerPackRecord

| # | Name | Type | Meaning |
|---|------|------|---------|
| 1 | `packId` | bytes | 16 bytes |
| 2 | `packKey` | bytes | 32 bytes; empty when deleted |
| 3 | `position` | uint32 | display order among installed packs; 0 when deleted |
| 4 | `deletedAtTimestamp` | uint64 | ms; non-zero = pack was uninstalled (tombstone) |

### 4.9 Record types signal-cli does not process

Listed so an implementation can decode the oneof; signal-cli never reads
or writes their contents (§6.4: their IDs are kept and the remote data is
left untouched, except by a force push, §6.6).

- **StoryDistributionListRecord**: 1 `identifier` bytes, 2 `name`
  string, 3 `recipientServiceIds` repeated string, 4
  `deletedAtTimestamp` uint64, 5 `allowsReplies` bool, 6 `isBlockList`
  bool, 7 `recipientServiceIdsBinary` repeated bytes.
- **CallLinkRecord**: 1 `rootKey` bytes, 2 `adminPasskey` bytes, 3
  `deletedAtTimestampMs` uint64; 4 reserved.
- **ChatFolderRecord**: 1 `identifier` bytes, 2 `name` string, 3
  `position` uint32, 4 `showOnlyUnread` bool, 5 `showMutedChats` bool, 6
  `includeAllIndividualChats` bool, 7 `includeAllGroupChats` bool, 8
  `folderType` enum (UNKNOWN 0, ALL 1, CUSTOM 2), 9 `includedRecipients`
  repeated Recipient, 10 `excludedRecipients` repeated Recipient, 11
  `deletedAtTimestampMs` uint64.
- **NotificationProfile**: 1 `id` bytes, 2 `name` string, 3 `emoji`
  optional string, 4 `color` fixed32 (0xAARRGGBB), 5 `createdAtMs`
  uint64, 6 `allowAllCalls` bool, 7 `allowAllMentions` bool, 8
  `allowedMembers` repeated Recipient, 9 `scheduleEnabled` bool, 10
  `scheduleStartTime` uint32, 11 `scheduleEndTime` uint32, 12
  `scheduleDaysEnabled` repeated enum DayOfWeek (UNKNOWN 0, MONDAY 1 …
  SUNDAY 7), 13 `deletedAtTimestampMs` uint64.
- **Recipient**: oneof `identifier` {1 `contact` Contact (as in
  PinnedConversation), 2 `legacyGroupId` bytes, 3 `groupMasterKey`
  bytes}.

## 5. Transport

### 5.1 Credentials: `GET /v1/storage/auth`

Sent on the **authenticated chat connection** (libsignal chat WebSocket,
`transport.md`), as a plain request without body. Response 200, JSON
(S6 `SecureStorageController`, S2 `StorageAuthResponse`):

| Key | Type | Meaning |
|-----|------|---------|
| `username` | string | the account's ACI as a UUID string |
| `password` | string | opaque token (server-generated; currently `<username>:<epoch seconds>:<hex signature>`); valid about 24 h |

Unknown keys are ignored. signal-cli does not cache the credentials: a
new `GET /v1/storage/auth` precedes **every** storage operation (each
manifest fetch, each read call, each write; one per read call even when
the read is split into batches). A failure of this request fails the
operation like any other I/O error.

The storage requests carry `Authorization: Basic <base64(username ":"
password)>` (RFC 7617; standard base64 with padding; ISO-8859-1 bytes).
Vector: `storage.json` `http.authorization_header`.

### 5.2 Endpoints (storage host)

Host `https://storage-staging.signal.org` (production
`https://storage.signal.org`), over signal-cli's OkHttp client: TLS,
proxy and `User-Agent` exactly as for the groups API (`groups.md` §3.1);
connect and read timeout 30 s; one of the configured storage URLs is
picked at random per request (there is only one). Request bodies are
`Content-Type: application/x-protobuf`; response bodies are raw protobuf.
No `X-Signal-Timestamp` check is applied to storage responses (unlike the
groups endpoints).

| Purpose | Method and path | Body | Success | Other statuses (server, S5) |
|---------|-----------------|------|---------|-----------------------------|
| Current manifest | `GET /v1/storage/manifest` | — | 200 StorageManifest | 404 if the account has no manifest |
| Manifest if changed | `GET /v1/storage/manifest/version/<V>` (V decimal, may be `-1`) | — | 200 StorageManifest if the stored version ≠ V | **204** if the stored version equals V **or no manifest exists** |
| Read items | `PUT /v1/storage/read` | ReadOperation | 200 StorageItems | 400 if no keys; 413 if more than 5120 keys |
| Write | `PUT /v1/storage` | WriteOperation | 200, empty body | 400 without manifest; 409 with body = current StorageManifest (§5.3); 413 if 2 × inserts + deletes > 1 000 000 |
| (unused) | `DELETE /v1/storage`, `GET /ping` | — | — | never called by signal-cli |

Response mapping on the client (S2 `PushServiceSocket`): a 2xx other than
204 is success. Otherwise: 204 "no content", 401/403 "authorization
failed", 404 "not found", 409 "conflict", 429 "rate limit", 499
"deprecated version", anything else a generic non-success error. Each is
an I/O error carrying the status code; the storage layer then maps 204 on
the manifest-if-changed call to "same version", 404 on the plain manifest
call to "no manifest", and 409 on a write to "conflict". The 409 body is
**not** used by signal-cli.

signal-cli reads at most **1000** IDs per `PUT /v1/storage/read`
request; larger sets are split into consecutive batches of 1000. A read
of an empty set sends no request. Only IDs whose type is 1–9 are sent;
IDs of other types are never requested (they become empty "unknown"
placeholders, §6.4).

### 5.3 Server write semantics (S5)

- If `clearAll` is set, all items of the account are deleted first
  (before the version check).
- The new manifest is stored **iff** the account has no manifest yet (any
  version is then accepted) **or** the new `StorageManifest.version` is
  exactly the stored version + 1. Otherwise nothing further happens and
  the answer is 409 with the current stored StorageManifest as body.
  (With `clearAll` the items are already gone at that point.)
- When the manifest was stored, the inserts and deletes are applied.
  Items are keyed by (ACI, raw ID); inserting an existing raw ID
  overwrites it.
- The server never decrypts anything and does not check that the manifest
  lists exactly the stored items.

### 5.4 Retry

There is no retry anywhere in the storage path: a conflict on write
abandons the current sync silently (§6.4 step 7, §6.6); network and status
errors abort the job, which logs a warning (§7). The next sync happens
only when something queues one again.

## 6. signal-cli's sync algorithm

### 6.1 When a sync runs

A sync is a job ("storage sync job") on the account's job queue. Jobs run
one at a time on a background thread in FIFO order; there is no
de-duplication (two enqueues → two syncs). When the account is closed
(end of a CLI command, daemon shutdown) the queue is drained: pending and
running jobs finish before the process exits. A job enqueued while
shutting down is dropped.

A normal sync job is enqueued by (S1):

| Trigger | Where |
|---------|-------|
| Registration finished and the server's verify response had `storageCapable: true` | `registration.md` §7 step 3 (after the prekey refresh, before the initial profile upload) |
| Device linked (`addDevice`) | after the device is linked and the multi-device flag is set |
| Own number/ACI/PNI changed (whoami differs from stored values) | account helper |
| Received sync message: any `request` (on the primary; one per request message); `fetchLatest` type STORAGE_MANIFEST; `keys` with an AEP (or legacy storage key); `keys` with a media root backup key | `sync.md` §6.2, §6.7, §6.10 (as a message action, i.e. after the received batch is handled) |
| `sendSyncRequest` (and the request sent right after linking: `requestAllSyncData`) | after sending the requests |
| `updateConfiguration` | after storing the settings and sending the configuration sync |
| `updateContact` (contact/nick names, note), `removeContact` (`--hide` hides; `--forget` deletes the recipient; plain removes the contact) | after the local change |
| `block`/`unblock` (contacts and groups) | after the local change, message-request-response and blocked-list syncs |
| Expiration-timer change for a contact (`updateContact -e`) | after sending the expiration update |
| Profile-key rotation (e.g. after blocking) | after rotating and re-uploading the profile |
| Group operations: create, update, join, quit-and-delete (`quitGroup --delete`), set blocked, and any refresh that stores new group state from the server (`getOrMigrateGroup`) | group helper |

A **force-push** job (§6.6 directly, without the read step) is enqueued
only when the account's own profile, fetched on a primary, newly gains
the `storageServiceEncryptionV2Capability` (`ssre2`) capability. The
current server (S6) no longer defines that capability, so this never
happens in practice.

There is **no** sync at program start, on `receive`, or periodically.

### 6.2 Local state

| What | Where (signal-cli) | Notes |
|------|--------------------|-------|
| Local manifest version | key/value entry `storage-manifest-version`, integer, **default −1** | reset to −1 on registration and on linking |
| Local copy of the last manifest | file `<data>/<account path>.d/storage-manifest` | serialized StorageManifest whose `value` is the **unencrypted** ManifestRecord (identifiers, sourceDevice, recordIkm; the inner `version` is not set) and whose outer `version` is the version. Deleted on registration/linking. Missing or unreadable → treated as the empty manifest (version 0, no IDs, no recordIkm; a read failure logs "Failed to read local storage manifest."). |
| Raw ID per contact and for the account | recipient table, column `storage_id` (unique, nullable); the self row's ID is the ACCOUNT record's ID | |
| Last remote record per contact / account | recipient table, `storage_record`: serialized **ContactRecord** / **AccountRecord** (inner message, not the StorageRecord wrapper) | |
| Raw ID and last record per group | v1 and v2 group tables, `storage_id`, `storage_record` (GroupV1Record / GroupV2Record) | |
| Raw ID and last record per sticker pack | sticker table, `storage_id`, `storage_record` (StickerPackRecord), plus `installed`, `position`, `deleted_timestamp` | |
| IDs of records signal-cli does not process | table `storage_id` (`type` integer, `storage_id` blob unique) | "unknown IDs" |

The **local ID set** is: all unknown IDs (with their stored type); every
v1 and v2 group with an ID (types 2, 3); every recipient **other than
self** that has an ID and an ACI or PNI (type 1, including unregistered
ones); every sticker pack with an ID (type 6); and the self row's ID as
type 4 (created on the spot if the self row has none).

### 6.3 What gives a local entity a new raw ID

Every one of these writes a fresh random 16-byte ID into the entity's
`storage_id` (the old one thereby becomes "remote only" and is deleted at
the next write):

- **Contacts and self** (recipient row): storing contact data (any of
  names, nick names, note, blocked, profile sharing, archived, hidden,
  mute, hide-story, expiration timer, color, unregistered timestamp —
  the ID is rotated on every store, changed or not); storing profile data
  (every profile fetch that stores a profile, including routine
  refreshes); storing a different profile key (or the same key while the
  stored profile's unidentified-access mode is unknown/disabled); any
  change of the row's number/ACI/PNI/username (address update or merge);
  newly marking the PNI signature verified; saving an identity key or
  trust level for the recipient's ACI (identity store write).
- **Self only**: changing read receipts, unidentified-delivery
  indicators, typing indicators, link previews, phone-number unlisted,
  phone-number sharing mode, or username-link color (only when the value
  actually changes); confirming or losing the username; rotating the
  profile key.
- **Groups**: every save of a group (any state change, blocked flag,
  profile sharing, refresh from the server).
- **Sticker packs**: install or uninstall (`installed` changes; uninstall
  also sets `deleted_timestamp` = now and `position` = 0; install sets
  `deleted_timestamp` = 0 and `position` = next free position = 1 +
  highest position among installed packs, or 0); adding a pack that is
  installed or carries a deletion timestamp.
- **Missing IDs** are filled in at every sync (§6.4 step 5) for: recipients
  with ACI or PNI, no ID and no unregistered timestamp; v2 groups without
  ID; sticker packs without ID that are installed or deleted. (v1 groups
  are meant to be included too, but signal-cli 0.14.7 never executes that
  update, so v1 groups without ID stay without one.)

An entity loses its ID (→ remote delete) when its row is deleted
(recipient forgotten, group deleted) or its address is removed.

### 6.4 The sync job

Inputs: local manifest version L (default −1), local manifest M_L (or
empty), device role (primary = device 1).

1. **Key.** Determine the storage key (§2.2). If there is none: on a
   linked device send a KEYS request (`sync.md` §6.2); stop.
2. **Fetch.** `GET /v1/storage/manifest/version/<L>` (after
   `GET /v1/storage/auth`).
   - 200: decrypt with the manifest key of the outer version. On success
     the result is the remote manifest R; **store R locally immediately**
     (version and file), before anything else, regardless of whether R is
     newer or older.
   - 200 but decryption fails: log WARN "Manifest couldn't be
     decrypted."; R = none; on a primary remember "force push needed", on
     a linked device send a KEYS request.
   - 204: R = M_L (the local copy; the empty manifest if none is stored).
     This includes "no manifest on the server": a fresh account then sees
     R = empty manifest with version 0.
   - any other status or network error: the job fails (§7).
3. **Read** (only if R exists and R.version > L): diff R's IDs against
   M_L's IDs (**not** against the local database), as in §6.4.1, and
   process the remote-only records (§6.4.2). A type mismatch on a primary
   sets "force push needed". If R.version < M_L.version only a debug line
   is logged ("Remote storage manifest version was older. User might have
   switched accounts.").
4. **Previously unknown types** (only if R exists): for every stored
   unknown ID whose type is now a known type, read those records from R's
   items and process them; remove the processed ones from the unknown-ID
   table. (Only matters after an upgrade that adds known types.)
5. **Fill missing IDs** (§6.3 last bullet).
6. **Decide** (first matching rule):
   1. primary needs key migration (§2.4): migrate, send keys, force push;
   2. R is none (manifest undecryptable): primary → force push; linked →
      nothing;
   3. R has no `recordIkm` and the account's own stored profile has the
      `ssre2` capability: force push (does not occur with the current
      server, §6.1; note that a missing own profile makes this check fail
      with an internal error, see Gap 6);
   4. otherwise: **write** (§6.5), with "force push pending" = the flag
      from steps 2–3.
   Then, if "force push needed" is set (from step 2 or 3, or rule 1–3),
   force push (§6.6). Note that under rule 4 a write is attempted
   **before** a pending force push.
7. **Notify other devices.** If a write (§6.5) or a force push succeeded,
   send a `fetchLatest` sync message of type STORAGE_MANIFEST
   (`sync.md` §6.10; only sent when the account is multi-device). If a
   write or force push hit 409, the job ends here silently (no sync
   message, no retry, local manifest unchanged).

#### 6.4.1 ID difference

Compare two lists of (raw ID, type) by raw ID:

- remote-only = raw IDs present only in the remote list; local-only = raw
  IDs present only in the local list;
- if a raw ID is in both lists with **different types**, and the local
  type is a known type, the ID is in neither result and a **type
  mismatch** is flagged; with an unknown local type the ID is treated as
  shared (no mismatch);
- a type mismatch is also flagged if either list contains the same raw ID
  twice.

#### 6.4.2 Processing remote-only records (read step)

1. Fetch all remote-only IDs (§5.2 read), decrypt each with the item key
   of R's scheme (§2.1). Items the server did not return are simply
   missing (debug log; they will be deleted by the next write). If **any**
   item of a read batch fails to decrypt, the whole call yields **no
   records** (WARN "Failed to read storage records, ignoring.") and the
   sync continues as if those records were missing — so the subsequent
   write deletes them remotely (Gap 4). Protobuf parse errors abort the
   job.
2. Process each record, in the order returned, by its content type (§4.3)
   with the per-type rules of §6.4.3. Records of unprocessed types are
   collected as "unknown inserts".
3. Let U = local-only IDs (M_L minus R) minus the IDs the processors
   matched/stored. For each recipient whose ID is in U **and** that has
   an unregistered timestamp, clear its ID; for each sticker pack whose ID
   is in U and that is uninstalled with a deletion timestamp, clear its
   ID. If any were cleared, log WARN "Found <n> recipients and <m> sticker
   packs that were deleted remotely but only marked deleted locally.
   Removed those from local store."
4. Add the unknown inserts to the unknown-ID table (replacing same raw
   IDs) with their manifest type; delete from that table the local-only
   IDs of unknown type.
5. All database changes of steps 2–4 are one transaction.

Every processor follows the same frame (S1
`DefaultStorageRecordProcessor`):

- If the remote record is **invalid** (per type), it is ignored (debug
  "Found invalid key! Ignoring it."). Its ID stays remote-only and is
  deleted by the next write.
- Find the matching local entity. If none: **insert** locally (the
  remote ID becomes the entity's ID).
- If the matching local entity was already matched by an earlier remote
  record in this batch: ignore this one ("Multiple remote records map to
  the same local record"); its ID will be deleted by the next write.
- Otherwise **merge** remote R and local Lc into M (per-type rules). The
  merge result is **R itself** (with R's ID) if M encodes identically to
  R; else **Lc itself** (with Lc's ID) if M encodes identically to Lc;
  else M with a **new random raw ID**. "Encodes identically" means equal
  field values including unknown fields.
- If the result differs from Lc (ID or content), update the local entity
  from the result and store the result's ID and record bytes as the
  entity's `storage_id`/`storage_record` (clearing that ID from any other
  row first).

Outcomes for the next write: result = R → IDs agree, nothing to write;
result = Lc → Lc's ID is local-only (inserted) and R's ID remote-only
(deleted); result = new → same, with the new ID.

#### 6.4.3 Per-type rules

**Building the local record ("local → remote")** always starts from the
entity's stored `storage_record` (all fields of the last remote record,
including unknown ones; empty if none) and overwrites the fields below.

#### ContactRecord

Local → remote (recipient row + identity):

| Field | Value |
|-------|-------|
| `e164` | number or "" |
| `username` | username or "" |
| `pniSignatureVerified` | stored flag |
| `profileKey` | 32 bytes or empty |
| `aci` / `aciBinary` | ACI string / 16 bytes, or ""/empty |
| `pni` / `pniBinary` | PNI without prefix / 16 bytes, or ""/empty |
| `givenName`, `familyName` | profile names ("" for unset) — only if a profile is stored |
| `systemGivenName`, `systemFamilyName`, `systemNickname`, `note` | contact given/family/nick name, note ("" for unset) — only if contact data is stored (same for the rest of this group) |
| `nickname` | Name{given, family} from the nick given/family names; **unset** if both are empty |
| `blocked`, `whitelisted` (profile sharing), `mutedUntilTimestamp`, `hideStory`, `archived`, `hidden` | contact flags |
| `unregisteredAtTimestamp` | unregistered timestamp or 0 |
| `identityKey`, `identityState` | only if an identity is stored for the ACI and the recipient has an ACI: 33-byte key; trust TRUSTED_VERIFIED → VERIFIED, UNTRUSTED → UNVERIFIED, anything else → DEFAULT |

Invalid remote record: neither a valid ACI nor a valid PNI; or the ACI,
PNI or e164 equals the account's own; or a non-empty e164 that does not
match `^\+[1-9][0-9]{6,18}$`.

Matching: resolve (ACI, PNI, e164, username) of the remote record to a
local recipient, **creating one if none exists** (so contacts always go
through merge, never insert). Its ID is taken, or created if absent.

Merge (start from all remote fields, then set):

| Field(s) | Rule |
|----------|------|
| `givenName` + `familyName` | the remote pair if either remote value is non-empty, else the local pair |
| `identityKey`, `identityState` | if the remote key is non-empty and (device is linked, or the states differ, or the local key is empty): remote key and state. Otherwise: local state, and key = local key if the local record has an ACI or PNI and both keys are non-empty, else empty |
| (side effect) | if the local record has an ACI and both keys are non-empty and differ: queue a profile fetch for that contact |
| `pni`, `e164` | on a primary, if (e164s equal and non-empty but both PNIs present and different) or (PNIs equal but both e164s non-empty and different): local PNI and local e164, and queue a recipient refresh. Otherwise (and always on a linked device): PNI = remote PNI if present else local; e164 = remote if non-empty else local |
| `systemGivenName`, `systemFamilyName` | primary: local; linked: remote |
| `systemNickname` | remote |
| `profileKey` | remote if non-empty and a valid profile key, else local |
| `username` | remote if non-empty, else local |
| `blocked`, `whitelisted`, `archived`, `markedUnread`, `mutedUntilTimestamp`, `hideStory`, `unregisteredAtTimestamp`, `hidden`, `nickname`, `note`, `avatarColor` | remote |
| `pniSignatureVerified` | remote OR local |
| `aci`, `aciBinary` | if the remote record has neither: both from the local ACI (or stay empty). Otherwise each form that the remote record has is replaced by the local ACI's form when a local ACI exists |
| `pni`, `pniBinary` | same pattern with the merged PNI (no prefix; empty if none) |

Local update from a contact record (insert or update):

1. Resolve the recipient for (ACI, PNI, e164, username) of the record as
   a **trusted** address (may merge recipients, `contacts.md`).
2. If any of blocked, profile sharing (`whitelisted`), archived, hidden,
   hide-story, mute-until, unregistered timestamp, contact given/family
   name (`systemGivenName`/`systemFamilyName`), nick name
   (`systemNickname`), nick given/family (`nickname`), note differs from
   the stored contact: store the contact with those values (empty strings
   → unset; unregistered 0 → unset); other contact fields (expiration
   timer, color) are kept.
3. If profile given/family name differ: store the profile with the
   record's names (other profile fields kept).
4. Non-empty `profileKey`: store it (an invalid key logs WARN "Received
   invalid contact profile key from storage").
5. Non-empty `identityKey` and the address has an ACI: save it as the
   ACI's identity, then set its trust level: DEFAULT → TRUSTED_UNVERIFIED,
   VERIFIED → TRUSTED_VERIFIED, UNVERIFIED → UNTRUSTED (an invalid key logs
   WARN "Received invalid contact identity key from storage").
6. Store `pniSignatureVerified`, then the record's ID and bytes.

#### AccountRecord

Local → remote (self row, configuration):

| Field | Value |
|-------|-------|
| `profileKey` | own profile key, if any |
| `givenName`, `familyName`, `avatarUrlPath` | from the own stored profile ("" for unset), if a profile is stored |
| `typingIndicators`, `readReceipts`, `sealedSenderIndicators`, `linkPreviews` | configuration value; **true** if never set |
| `unlistedPhoneNumber` | configuration value; false if never set |
| `phoneNumberSharingMode` | EVERYBODY → EVERYBODY; CONTACTS or NOBODY → NOBODY; unset → UNKNOWN |
| `username` | own username or "" |
| `usernameLink` | only if a username link is stored: entropy, server ID (16 bytes), color = stored color name or UNKNOWN |

Never invalid. The match is always the local account record (one per
sync); a second remote account record in the same batch is ignored and
later deleted. Never inserted.

Merge (start from all remote fields):

| Field(s) | Rule |
|----------|------|
| `givenName` + `familyName` | remote pair if either is non-empty, else local pair |
| `avatarUrlPath`, `profileKey`, `subscriberCurrencyCode` | remote if non-empty, else local |
| `preferredReactionEmoji` | remote list if non-empty, else local |
| `noteToSelfArchived`, `noteToSelfMarkedUnread`, `readReceipts`, `typingIndicators`, `sealedSenderIndicators`, `linkPreviews`, `unlistedPhoneNumber`, `phoneNumberSharingMode`, `pinnedConversations`, `preferContactAvatars`, `universalExpireTimer`, `displayBadgesOnProfile`, `subscriptionManuallyCancelled`, `keepMutedChatsArchived`, `hasSetMyStoriesPrivacy`, `storiesDisabled`, `username`, `usernameLink`, `avatarColor` | remote |
| `hasViewedOnboardingStory`, `hasSeenGroupStoryEducationSheet`, `hasCompletedUsernameOnboarding` | remote OR local |
| `storyViewReceiptsEnabled` | remote unless UNSET, then local |
| `payments` | take remote `payments` if present with non-empty entropy, else local's (may be absent). Then **always** write a Payments message: `enabled` = (taken enabled) AND entropy is exactly 32 bytes; `entropy` only if exactly 32 bytes. An absent source yields an empty Payments message (field 16 present with length 0). |
| `subscriberId` + `subscriberCurrencyCode` | remote pair if remote `subscriberId` non-empty, else local pair; kept only if the ID is 32 bytes and the currency is not blank, otherwise both cleared |
| `backupSubscriberData` | remote's if its `subscriberId` is non-empty, else local's; kept (as subscriberId + purchaseToken or originalTransactionId) only if the ID is 32 bytes and there is a non-blank purchase token or a transaction ID; otherwise the field is removed |

(Because of the payments rule, a remote account record without field 16
never merges as "equal to remote"; signal-cli then re-uploads the account
record once with an empty Payments message.)

Local update from an account record:

1. Configuration: read receipts, typing indicators, unidentified-delivery
   indicators (`sealedSenderIndicators`), link previews, unlisted phone
   number set from the record; phone-number sharing mode set to
   EVERYBODY/NOBODY, or **unset** for UNKNOWN.
2. Own username = record's `username`, or **cleared** if empty.
3. If `usernameLink` is present: store link (entropy, server ID parsed as
   a 16-byte UUID) and the color name.
4. Non-empty valid `profileKey`: store as the own profile key and queue a
   download of the own avatar from `avatarUrlPath` (empty → none). An
   invalid key is ignored (debug log).
5. Store the own profile with the record's given and family names (empty
   strings stored as such), keeping other profile fields.
6. Store the record's ID and bytes on the self row.

#### GroupV1Record

Local → remote: `id` = group ID (16 bytes), `blocked`, `archived` from
the group, `whitelisted` = **true**.

Invalid: the ID is not a 16-byte v1 ID, or a v2 group derived from this
v1 ID already exists locally. Matching: the local v1 group with that ID.
Merge: all remote fields (no local field survives). Insert/update: get
the v1 group (creating it if unknown; skipped if a migrated v2 group
exists), set `blocked`, save, store ID and record. Nothing else (e.g.
archived) is applied locally.

#### GroupV2Record

Local → remote: `masterKey` (32 bytes), `blocked`, `whitelisted` =
profile sharing.

Invalid: `masterKey` is not 32 bytes. Matching: the local v2 group whose
ID is derived from the master key (`groups.md`). Merge: all remote
fields. Insert/update: get the group, or create a new local v2 group from
the master key (partially migrating a matching v1 group, `groups.md`);
set blocked and profile sharing from the record; save; store ID and
record. Group state is **not** fetched by the sync; a new group initially
has only its master key (`groups.md` for how such groups are listed and
later refreshed).

#### StickerPackRecord

Local → remote: `packId`; if the pack has a deletion timestamp:
`packKey` empty, `position` 0, `deletedAtTimestamp` = timestamp;
otherwise `packKey`, `position`, `deletedAtTimestamp` 0.

Invalid: `packId` not 16 bytes, or not deleted (`deletedAtTimestamp` 0)
and `packKey` not 32 bytes. Matching: a local pack with that ID that is
installed or has a deletion timestamp (an ID is assigned if it has none).
Merge: if both are deleted and the local deletion timestamp is later, the
local record; otherwise the remote record (never a new ID).
Insert/update: upsert the pack: installed = not deleted; position =
remote position (0 if deleted); deletion timestamp = remote; pack key =
remote key if non-empty (a deleted remote record keeps the local key);
store ID and record. The sticker manifest is **not** downloaded by the
sync.

### 6.5 Write

1. Local ID set (§6.2). Diff against R's IDs (§6.4.1).
2. Local-only IDs of **unknown** type are removed from the unknown-ID
   table, and the diff is recomputed.
3. Deletes = all remote-only raw IDs. Inserts = for each local-only ID,
   the local record built per §6.4.3 (contact, v1 group, v2 group,
   account, sticker pack).
4. New manifest: version = R.version + 1; sourceDevice = own device ID;
   `recordIkm` = R's `recordIkm` (kept, possibly empty); identifiers =
   the whole local ID set (with unknown IDs keeping their numeric type).
5. If there are no inserts and no deletes: nothing is sent (debug "No
   remote writes needed"); the job continues to step 6.7 with "nothing
   written".
6. Validate (below). Then `PUT /v1/storage` with the manifest encrypted
   under the manifest key of the new version, the inserts encrypted with
   the item keys of the new manifest's scheme, and `deleteKey` = deletes
   (`clearAll` false).
7. 200: store the new manifest locally (version + file); "written".
   409: abandon silently. Other errors: job fails.

Validation (an internal error aborts the job, §7): exactly one ACCOUNT ID
in the manifest; no duplicate (type, raw) and no raw ID used twice; no
duplicate inserts; every insert is in the manifest and is of a processed
type; no insert is a contact matching the own address; no delete is in the
manifest. If R.version > 0: the new version must be R.version + 1, and,
unless a force push is pending, the inserts must be exactly the IDs added
relative to R and the deletes exactly the IDs removed.

### 6.6 Force push (primary only)

1. `GET /v1/storage/manifest` → current version C (404 → C = 0; the
   manifest is not decrypted). New version N = C + 1.
2. Assign **new random IDs to everything** and build records:
   - every recipient with an ACI or PNI (including unregistered ones):
     the self row as ACCOUNT, all others as CONTACT;
   - every v1 group, every v2 group;
   - every sticker pack that is installed or has a deletion timestamp.
3. `recordIkm`: a new random 32-byte value **only if** the own profile has
   the `ssre2` capability (never with the current server); otherwise
   none, i.e. items use the legacy item keys.
4. Manifest: version N, own device ID, that recordIkm, all new IDs.
   Validate as in §6.5 (structural checks only).
5. If N > 1: `PUT /v1/storage` with `clearAll` = true and all records as
   inserts. If N = 1 (no manifest existed): same without `clearAll`.
6. 200: store the manifest locally; write all new IDs into the local
   rows; **delete all unknown IDs** from the local table. 409: abandon
   silently (local IDs unchanged). Other errors: job fails.

Consequence: a force push destroys every remote record signal-cli does
not process (story distribution lists, call links, chat folders,
notification profiles, future types) and drops a remote `recordIkm`.

### 6.7 Messages to other devices

- After a successful write or force push: `fetchLatest`
  STORAGE_MANIFEST (§6.4 step 7).
- After key migration: `keys` (§2.4).
- On a linked device without key, or with an undecryptable manifest:
  `request` KEYS.

All three go through the normal sync-message path and are only sent on a
multi-device account (`sync.md` §1, §9).

## 7. Visible effects and log lines

The sync job produces **no stdout output** and never changes a command's
exit status; it runs after (or alongside) the command. Its effects become
visible through other commands:

| Remote change | Visible via |
|---------------|-------------|
| Contact blocked/unblocked | `listContacts` `Blocked:` / `--blocked` filter; blocked list |
| Contact profile sharing, archived, hidden, unregistered | `listContacts --detailed` (`Profile sharing:`, `Archived:`, `Hidden:`, `Unregistered:`) |
| Contact names (system given/family, nick name, nickname, note) | `listContacts` `Name:`; `--detailed` given/family/nick fields, `Note:` |
| Contact profile names, profile key | `listContacts` `Profile name:`; enables profile fetch/sealed sender |
| Contact username, e164, PNI | `listContacts` `Username:`/`Number:`/`PNI:` (after address merge) |
| Contact identity key and verification state | `listIdentities` trust level |
| Account settings (read receipts, typing, sealed-sender indicators, link previews) | used by later sends; `updateConfiguration` state |
| Phone-number sharing / unlisted | account settings (`updateAccount`) |
| Own username and link, own profile names and key, avatar | own profile, `updateAccount`/`listAccounts`-related output (`usernames.md`) |
| v2 group blocked / profile sharing, new v2 group | `listGroups` (`Blocked:`; new group appears, initially without state) |
| v1 group blocked | `listGroups` |
| Sticker pack installed/removed | `listStickerPacks` (installed flag; title unknown until the pack is fetched) |

Log lines (stderr; format `spec/surface/cli/index.md` §8; at default
verbosity only WARN and above appear):

| Level | Text | When |
|-------|------|------|
| WARN | `Failed to sync storage data` (+ exception) | any I/O or status error inside the job (logger `SyncStorageJob`) |
| WARN | `Job SyncStorageJob failed` (+ exception) | any other error, e.g. a validation failure or internal error |
| WARN | `Manifest couldn't be decrypted.` | §6.4 step 2 |
| WARN | `Failed to read storage records, ignoring.` | §6.4.2 step 1 |
| WARN | `Found <n> recipients and <m> sticker packs that were deleted remotely but only marked deleted locally. Removed those from local store.` | §6.4.2 step 3 |
| WARN | `Received invalid contact profile key from storage` / `Received invalid contact identity key from storage` | contact update |
| WARN / ERROR | `Failed to read local storage manifest.` / `Failed to store local storage manifest.` | local file I/O |
| DEBUG | `Storage key unknown, requesting from primary device.`, `Doing a force push.`, `Force pushing local state to remote storage`, `Storage needs force push due to new account entropy pool`, `Done syncing data with remote storage`, `No remote writes needed. Still at version: <v>`, `Saved new manifest. Now at version: <v>`, `Force push succeeded. Updating local manifest version to: <v>`, `Hit a conflict when trying to resolve the conflict! Retrying.` (409 on write; despite the text nothing is retried), `Hit a conflict. Trying again.` (409 on force push; idem) | |

Logs are not part of the compatibility contract (`index.md`).

## 8. Implementation notes (normative for interoperability)

- The server enforces only "version = stored + 1 or no manifest". Every
  other invariant (each listed ID exists, exactly one account record, no
  self contact) is a client convention that other clients rely on;
  §6.5's validation keeps it.
- Writes must always contain the **full** ID set in the manifest, not a
  delta.
- An implementation must carry through all fields it does not interpret,
  including unknown field numbers and unknown enum values, when
  re-uploading a record it previously downloaded (§6.4.3 "starts from the
  stored record").
- Byte-exact equality of protobuf encodings is only used to decide merge
  outcomes; an implementation may compare decoded messages (including
  unknown fields) instead.

## 9. Gaps

1. **`ssre2` capability**: Signal-Server @ v20261022.0.0 defines no
   `ssre2` device capability, so profiles never report it and signal-cli
   never generates a `recordIkm`. If a deployed server still returns
   `capabilities.ssre2: true` in profiles, rule 6.4 step 6.3 and the
   force-push `recordIkm` generation would activate. Not observed live.
2. **Server version skew**: storage-service tag v20260713.0.2 was used;
   the deployed server may differ (limits 5120 / 1 000 000, the 204 for a
   missing manifest).
3. **Credential password format** (§5.1) is opaque to clients and was
   only read from the server source.
4. **Destructive quirks reproduced from signal-cli**: a single
   undecryptable item makes a whole read batch empty and the following
   write deletes those items remotely; a stored-then-failed read (manifest
   stored before records are processed) can likewise lead to remote
   deletes; a force push wipes unprocessed record types. Whether the Go
   implementation should replicate these or be more careful is a product
   decision; this spec describes 0.14.7.
5. **Local manifest missing but version stored**: the 204 path then uses
   the empty manifest (version 0); the write proposes version 1, gets 409
   and the sync silently does nothing until the version changes. Taken
   from the source, not observed.
6. **Missing own profile**: if the own profile has never been stored
   (no capabilities row), the `ssre2` checks in §6.4 step 6.3 (reached
   when R has no `recordIkm`) and §6.6 step 3 fail with an internal error
   ("Job SyncStorageJob failed"), so no write happens. An implementation
   may treat a missing profile as "no capability"; this deviates from
   0.14.7 only in that the sync then proceeds.
7. **Identifier order** in written manifests: signal-cli emits unknown
   IDs (hash-set order), then v1 and v2 groups, contacts, sticker packs,
   and the account ID last (force push: contacts/account in recipient
   order, then groups, then stickers). No reader depends on the order;
   treated as unspecified.
8. **Recipient resolution side effects** of matching a remote contact
   (creating recipients, trusted merges) are specified in `contacts.md`;
   not repeated here.
9. **No live observation**: nothing in this file was checked against the
   staging server (not allowed for spec agents); the black-box binary
   cannot exercise storage without an account. Wire-level facts were
   checked with the reference jar (§10).

## 10. Vectors

`testdata/vectors/storage.json`, generated offline with `jshell` against
the reference jar (S3) except `keys.kat_svrkey_2a`, which is libsignal's
published known answer (S4). Contents: AEP → master key → storage key;
manifest keys for versions 0, 1, 2, 12345; legacy and recordIkm item
keys; serialized ContactRecord / AccountRecord / GroupV1Record /
GroupV2Record / StickerPackRecord inside StorageRecord; AES-256-GCM
encryption of items and of a ManifestRecord (with an identifier of
unknown type 42) under a fixed IV; StorageManifest, ReadOperation,
StorageItem, WriteOperation (with deletes, and with `clearAll`); the
Basic authorization header. Key derivations were cross-checked with an
independent HMAC/HKDF computation (javax.crypto), and every ciphertext
was decrypted with the jar's own storage cipher.
