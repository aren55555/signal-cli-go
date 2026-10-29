# signal-cli 0.14.7 `account.db` (SQLite schema and meaning)

Spec role: prose and tables only; no code, no copied SQL. This file
describes the per-account SQLite database
`<DATA>/data/<path>.d/account.db` as signal-cli 0.14.7 creates and
migrates it: the version mechanism, every table and column of the
current schema with its encoding and meaning, the `key_value` keys, and
the differences an old, migrated database can show. It ends with an
importer checklist (§15) that maps every piece of client state to its
location in this database or in the account JSON file.

Directory layout, the account JSON file, locking and the envelope cache
are in `signal-cli-data-dir.md` (same directory); references of the form
"data-dir §n" point there.

Related specs (not repeated here):

- Message send log semantics: `spec/service/send-log.md` (its §1.2 has
  the two tables from the behavioural side).
- libsignal record formats (`SessionRecord`, `PreKeyRecord`,
  `SignedPreKeyRecord`, `KyberPreKeyRecord`, `SenderKeyRecord`):
  `spec/protocol/capture.md` §2 ("Serialized record formats").
- `DecryptedGroup` (column `group_v2.group_data`): `spec/service/groups.md`
  (§"DecryptedGroup", around its line 294) and `spec/crypto/zkgroup-groups.md`.
- Storage-service records stored in the `storage_record` columns and the
  identifier type numbers used in `storage_id.type`:
  `spec/service/storage.md` §4.2–§4.3 and later sections.
- Recipient merging rules (why rows split and merge): `spec/service/contacts.md`.
- v1 → v2 group id derivation: `spec/service/groups.md` (around its line 74).

## Sources

| Ref | What | URL @ version |
|-----|------|---------------|
| B1 | signal-cli | https://github.com/AsamK/signal-cli @ tag `v0.14.7` = commit `b01b6b370dc063599a1a2b9fde0f5ff4e2d78fe8`, files under `lib/src/main/java/org/asamk/signal/manager/`: `storage/{AccountDatabase,Database,SignalAccount,Utils,UnknownStorageIdStore}.java`, `storage/keyValue/{KeyValueStore,KeyValueEntry}.java`, `storage/configuration/ConfigurationStore.java`, `storage/recipients/{RecipientStore,MergeRecipientHelper,RecipientAddress,CdsiStore}.java`, `storage/identities/{IdentityKeyStore,IdentityInfo}.java`, `storage/sessions/SessionStore.java`, `storage/prekeys/{PreKeyStore,SignedPreKeyStore,KyberPreKeyStore}.java`, `storage/senderKeys/{SenderKeyStore,SenderKeyRecordStore,SenderKeySharedStore}.java`, `storage/groups/{GroupStore,GroupInfoV1,GroupInfoV2}.java`, `storage/stickers/{StickerStore,StickerPack}.java`, `storage/sendLog/MessageSendLogStore.java`, `api/{Profile,PhoneNumberSharingMode,TrustLevel,Contact,StickerPackId}.java`, `config/ServiceConfig.java`, `util/KeyUtils.java`, `helper/{AccountHelper,ReceiveHelper,RecipientHelper,IncomingMessageHandler,GroupHelper,StorageHelper}.java`, `internal/ManagerImpl.java`, `syncStorage/{AccountRecordProcessor,ContactRecordProcessor,GroupV1RecordProcessor,GroupV2RecordProcessor,StickerPackRecordProcessor}.java` |
| B2 | Reference jars | `/work/third_party/signal-cli-0.14.7/lib/` (`libsignal-cli-0.14.7.jar`: searched all classes for key-value key definitions; `sqlite-jdbc-3.53.2.1.jar`). Offline `jshell` (JDK 25.0.5) on a **scratch** staging account created under `~/.cache` with the bundled library's own creation routine (fictional `+15555550100`, throwaway keys, no network; deleted afterwards): read `PRAGMA user_version`, `PRAGMA journal_mode`, every `sqlite_master` entry, and the initial rows; printed `ServiceId` string forms. |
| B3 | signal-cli, for §16 (listing order) | same tag and commit as B1. CLI/JSON-RPC commands under `src/main/java/org/asamk/signal/commands/`: `ListIdentitiesCommand`, `ListContactsCommand`, `ListGroupsCommand`, `ListStickerPacksCommand`, `ListDevicesCommand`, `ListAccountsCommand`; under `lib/src/main/java/org/asamk/signal/manager/`: `internal/ManagerImpl` (`getIdentities`, `getRecipients`, `getGroups`, `getStickerPacks`), `helper/GroupHelper`, `api/Group`, `storage/groups/{GroupInfo,GroupInfoV1,GroupInfoV2,GroupStore}`, `storage/identities/IdentityKeyStore`, `storage/recipients/{RecipientStore,MergeRecipientHelper}`, `storage/stickers/StickerStore`, `storage/{SignalAccount,AccountDatabase}`. Offline `jshell` against the bundled `sqlite-jdbc-3.53.2.1.jar` (SQLite 3.53.2) on a **scratch** database holding only the 0.14.7 `recipient`, `identity`, `group_v1`, `group_v1_member`, `group_v2`, `group_v2_member` and `sticker` tables (plus the migrated-db sticker index), with made-up rows: query plans of the listing queries and the resulting row order; deleted afterwards. |

Everything below comes from these sources. No Signal server was contacted.

---

## 1. Database file, connection settings, version

- One SQLite 3 database per account. signal-cli opens it with
  `foreign_keys=ON`, `journal_mode=wal`, busy timeout 60 000 ms and
  `BEGIN IMMEDIATE` transactions (B1 `Database`). B2 confirmed
  `journal_mode` = `wal` on disk. Committed data can sit in
  `account.db-wal` until a checkpoint (data-dir §4).
- Every table is declared `STRICT`, so stored values always have the
  declared storage class (or NULL); the `key_value.value` column is the
  only `ANY` column (§3).
- **Schema version**: SQLite's `PRAGMA user_version`. The current value
  in 0.14.7 is **30** (B1 `AccountDatabase`; confirmed in B2).
  - 0: empty database; signal-cli creates the current schema directly
    and sets 30.
  - 1–29: signal-cli runs every migration step above the stored value in
    one transaction, then sets 30.
  - greater than 30: signal-cli refuses (`Database has been updated by a
    newer signal-cli version`).
- Booleans are stored as INTEGER 0/1. Timestamps are **milliseconds since
  the Unix epoch** unless a column says otherwise. UUIDs stored as BLOB
  are 16 bytes, big-endian (most significant 8 bytes first, i.e. the
  usual textual order). Service ids stored as TEXT use the libsignal
  string form: an ACI is the lower-case UUID string
  (`00000000-0000-4000-8000-000000000001`), a PNI is `PNI:` followed by the
  lower-case UUID (B2).
- Columns are listed below in the order of a freshly created database.
  Migrated databases can have a different physical column order (§14);
  always address columns by name.

### 1.1 Account scope (`account_id_type`)

Tables that hold key material per local identity carry
`account_id_type` INTEGER (B1 `Utils.getAccountIdType`):

| Value | Local identity |
|-------|----------------|
| 0 | ACI |
| 1 | PNI |

Used by `session`, `pre_key`, `signed_pre_key`, `kyber_pre_key`. All
other tables are account-wide (not split by ACI/PNI). In particular
`identity`, `sender_key` and `sender_key_shared` are shared between the
two local identities.

### 1.2 Table list (fresh database, B2)

`recipient`, `identity`, `session`, `pre_key`, `signed_pre_key`,
`kyber_pre_key`, `sender_key`, `sender_key_shared`, `group_v1`,
`group_v1_member`, `group_v2`, `group_v2_member`, `sticker`,
`message_send_log`, `message_send_log_content`, `key_value`, `cdsi`,
`storage_id`, plus SQLite's internal `sqlite_sequence` (because
`recipient` uses AUTOINCREMENT). There are **no** job or queue tables;
signal-cli's job queue is in memory only.

Explicit indexes: `mslc_timestamp_index`, `msl_recipient_index`,
`msl_content_index` (§12); in migrated databases also
`sticker_storage_id_index` (§14). All other indexes are SQLite's automatic
ones for UNIQUE constraints.

---

## 2. `recipient`

One row per known person (including the account itself). Rows are
referenced by `_id` from `group_v1_member`, `group_v2_member`, and from
the envelope-cache directory names (data-dir §5). Sessions, identities and
sender keys are **not** linked by `_id`; they are keyed by service-id
string.

| Column | Type | Null | Default | Meaning / encoding |
|--------|------|------|---------|--------------------|
| `_id` | INTEGER | PK, AUTOINCREMENT | | Recipient id. Never reused (AUTOINCREMENT). |
| `storage_id` | BLOB | yes, UNIQUE | | 16-byte storage-service item id of this recipient's record (CONTACT type, or ACCOUNT type for the self row). Regenerated randomly whenever the row's synced data changes. |
| `storage_record` | BLOB | yes | | Last storage-service record seen for this recipient: the serialized **`ContactRecord`** protobuf, or for the self row the serialized **`AccountRecord`** (not the `StorageRecord` wrapper). Used to preserve unknown fields when re-uploading. |
| `number` | TEXT | yes, UNIQUE | | Phone number, E.164 with `+`. |
| `username` | TEXT | yes, UNIQUE | | Signal username (`nickname.discriminator`). |
| `aci` | TEXT | yes, UNIQUE | | ACI string (plain UUID). |
| `pni` | TEXT | yes, UNIQUE | | PNI string (`PNI:` + UUID). |
| `unregistered_timestamp` | INTEGER | yes | | ms when the recipient was found to be unregistered; NULL = registered or unknown. |
| `discoverable` | INTEGER | yes | | Boolean from contact discovery: 1 = number discoverable, 0 = not discoverable (or unregistered); NULL = unknown. |
| `profile_key` | BLOB | yes | | The recipient's 32-byte profile key. Ignored for the self row (the account file's `profileKey` is used instead). |
| `profile_key_credential` | BLOB | yes | | Serialized zkgroup `ExpiringProfileKeyCredential` for this recipient; cleared whenever the profile key changes. |
| `needs_pni_signature` | INTEGER | no | 0 | Boolean: outgoing messages to this recipient must carry a PNI signature (`spec/service/content-features.md` §4). |
| `pni_signature_verified` | INTEGER | no | 0 | Boolean: the ACI–PNI association of this row was verified by a PNI signature (or storage sync said so). |
| `given_name` | TEXT | yes | | Contact (system address book) given name. |
| `family_name` | TEXT | yes | | Contact family name. |
| `nick_name` | TEXT | yes | | Legacy single-field nickname. |
| `nick_name_given_name` | TEXT | yes | | Nickname given name (user-set local nickname). |
| `nick_name_family_name` | TEXT | yes | | Nickname family name. |
| `note` | TEXT | yes | | Free-text note about the contact. |
| `color` | TEXT | yes | | Conversation colour name (legacy). |
| `expiration_time` | INTEGER | no | 0 | Disappearing-message timer for the 1:1 conversation, in **seconds**; 0 = off. |
| `expiration_time_version` | INTEGER | no | 1 | Version counter of that timer (always ≥ 1 when written). |
| `mute_until` | INTEGER | no | 0 | ms until which the conversation is muted; 0 = not muted. |
| `blocked` | INTEGER | no | 0 | Boolean. |
| `archived` | INTEGER | no | 0 | Boolean. |
| `profile_sharing` | INTEGER | no | 0 | Boolean: our profile (key) is shared with this recipient ("message request accepted"). |
| `hide_story` | INTEGER | no | 0 | Boolean: hide this recipient's stories. |
| `hidden` | INTEGER | no | 0 | Boolean: contact hidden from the contact list (still known). |
| `profile_last_update_timestamp` | INTEGER | no | 0 | ms of the last profile fetch; 0 = never / must refetch. Reset to 0 when a new profile key is stored. |
| `profile_given_name` | TEXT | yes | | Decrypted profile given name. |
| `profile_family_name` | TEXT | yes | | Decrypted profile family name. |
| `profile_about` | TEXT | yes | | Decrypted "about" text. |
| `profile_about_emoji` | TEXT | yes | | Decrypted "about" emoji. |
| `profile_avatar_url_path` | TEXT | yes | | CDN path of the encrypted profile avatar, as returned by the server. |
| `profile_mobile_coin_address` | BLOB | yes | | Payment address bytes from the profile (opaque). |
| `profile_unidentified_access_mode` | TEXT | yes | | Enum name (§2.2). NULL or an unknown name reads as `UNKNOWN`. |
| `profile_capabilities` | TEXT | yes | | Comma-separated capability names (§2.2), no spaces; empty string = none. **NULL means "no profile stored"**: a profile is considered present iff this column is non-NULL. Unknown names are ignored. |
| `profile_phone_number_sharing` | TEXT | yes | | Enum name (§2.2) or NULL (unknown). |

### 2.1 Row semantics

- **Identifiers**: a row has any subset of `aci`, `pni`, `number`,
  `username`. Rows in which `aci`, `pni` and `number` are all NULL are
  ignored by every listing and are leftovers of merges (identifiers are
  cleared before a merged row is deleted, and a storage record whose
  address turned out invalid also gets its identifiers, including
  `username` and `storage_id`, cleared). A number-only row (no `aci`/`pni`)
  is a phone number whose service id is not known yet.
- **Self row**: the row whose `aci`/`pni`/`number` match the account's own
  (account file). Always present once the account has a number (B2: a
  fresh unregistered account has exactly one row, `_id` 1, with only
  `number` set). Its `profile_key` column is not authoritative; its
  `storage_record` holds the `AccountRecord`.
- **Is a contact**: signal-cli treats a row as a contact iff at least one
  of these holds: any of `given_name`, `family_name`, `nick_name`,
  `nick_name_given_name`, `nick_name_family_name`, `note`, `color` is
  non-NULL; `expiration_time` > 0; `profile_sharing`, `blocked` or
  `archived` is 1. `listContacts`-style views additionally require
  `hidden` = 0 (and, for some views, `unregistered_timestamp` NULL).
- **Unregistered**: `unregistered_timestamp` set (ms) and `discoverable`
  = 0. If such a row had both an ACI and a PNI, signal-cli **splits** it:
  the existing row keeps the ACI (and username) and loses `pni` and
  `number`; a new row is inserted with the PNI and number. Re-registration
  clears `unregistered_timestamp`.
- **Merged**: when one person turns out to have several rows, one row
  survives and receives the union of identifiers; the others are deleted
  after their contact data, profile key, credential and profile are
  copied into the survivor where the survivor had none, their group
  memberships are re-pointed to the survivor (duplicates dropped), and
  their envelope-cache files are moved. Merged `_id`s therefore simply
  disappear.

### 2.2 Enumerations stored as TEXT

| Column | Values |
|--------|--------|
| `profile_unidentified_access_mode` | `UNKNOWN`, `DISABLED`, `ENABLED`, `UNRESTRICTED` |
| `profile_capabilities` (list elements) | `storage`, `storageServiceEncryptionV2Capability` |
| `profile_phone_number_sharing`, key_value `config-phone-number-sharing-mode` | `EVERYBODY`, `CONTACTS`, `NOBODY` |

---

## 3. `key_value`

| Column | Type | Null | Meaning |
|--------|------|------|---------|
| `_id` | INTEGER | PK | row id |
| `key` | TEXT | no, UNIQUE | key name (table below) |
| `value` | ANY | yes | value; storage class depends on the key |

Encoding per value type (B1 `KeyValueStore`): integers and longs as
INTEGER; booleans as INTEGER 0/1; strings as TEXT; byte arrays as BLOB;
enums as TEXT holding the enum constant name (an unknown name reads as
NULL). "Unsetting" a key writes a row with `value` NULL rather than
deleting it; a missing row and a NULL value both mean "use the default".

This is the complete list of keys 0.14.7 reads or writes (B2: these are
the only key definitions in the library):

| Key | Value type | Default when absent/NULL | Meaning |
|-----|-----------|--------------------------|---------|
| `verification-session-id` | TEXT | NULL | Id of the registration verification session in progress (`spec/service/registration.md`). Cleared after registration. |
| `verification-session-number` | TEXT | NULL | Phone number (E.164) that session belongs to; the session id is only reused for the same number. |
| `last-receive-timestamp` | INTEGER (ms) | 0 | Time of the last successful message receive. 0 = never. Reset to 0 on registration/linking. Used for the "messages have been last received N days ago" warning and receive pacing. |
| `last-applied-pni-change-server-timestamp` | INTEGER (ms) | 0 | Server timestamp of the last applied PNI change-number sync message; older/equal ones are ignored as replays. |
| `retry-failed-messages` | INTEGER bool | true | Set when a receive starts (and when trust changes during a receive); while true the next receive first retries cached envelopes from `msg-cache/`; cleared after a receive loop ends. |
| `cdsi-token` | BLOB | NULL | Opaque token from the last full contact-discovery (CDSI) request, sent with the next incremental request. Cleared on an invalid-token error. |
| `last-recipients-refresh` | INTEGER (ms) | NULL | Time of the last full CDSI refresh. NULL or older than 1 day triggers a refresh at startup. Cleared on registration/linking. |
| `storage-manifest-version` | INTEGER | −1 | Version of the storage-service manifest last synced; −1 = none (forces a full sync). |
| `unrestricted-unidentified-access` | INTEGER bool | false | Own setting: accept sealed-sender messages from anyone (sent in account attributes). |
| `config-read-receipts` | INTEGER bool | NULL | Own setting: send read receipts. NULL = never set. |
| `config-unidentified-delivery-indicators` | INTEGER bool | NULL | Own setting: show sealed-sender indicators. |
| `config-typing-indicators` | INTEGER bool | NULL | Own setting: send typing indicators. |
| `config-link-previews` | INTEGER bool | NULL | Own setting: generate link previews. |
| `config-phone-number-unlisted` | INTEGER bool | NULL | Own setting: number not discoverable. NULL or 0 = discoverable. |
| `config-phone-number-sharing-mode` | TEXT enum | NULL | Own setting: `EVERYBODY`, `CONTACTS` or `NOBODY` (§2.2). |
| `username-link-color` | TEXT | NULL | Colour name of the own username link QR code (from/to storage sync). |

Changing any `config-*` key or `username-link-color` also gives the self
recipient a new `storage_id`, so the change is uploaded at the next
storage sync. A freshly created account has no `key_value` rows (B2).

---

## 4. `identity`

Remote identity keys and their trust (B1 `IdentityKeyStore`). One row per
service id; **not** scoped by `account_id_type`.

| Column | Type | Null | Meaning |
|--------|------|------|---------|
| `_id` | INTEGER | PK | row id |
| `address` | TEXT | no, UNIQUE | Service-id string of the peer: ACI (plain UUID) or PNI (`PNI:` + UUID). |
| `identity_key` | BLOB | no | libsignal serialized identity public key: 33 bytes, `0x05` followed by the 32-byte Curve25519 point. |
| `added_timestamp` | INTEGER | no | ms when this key was first stored for the address (kept when only the trust level changes; reset when the key changes). |
| `trust_level` | INTEGER | no | Trust level, enum ordinal (table below). |

| `trust_level` | Name | Trusted for sending | Verified-state equivalent |
|---------------|------|---------------------|---------------------------|
| 0 | `UNTRUSTED` | no | UNVERIFIED |
| 1 | `TRUSTED_UNVERIFIED` | yes | DEFAULT |
| 2 | `TRUSTED_VERIFIED` | yes | VERIFIED |

Rules: a first key seen for an address is stored as 1 under the default
trust mode (`--trust-new-identities on-first-use`), as 1 always under
`always`, and as 0 under `never`; a **changed** key is stored as 0 unless
the mode is `always`. The account's own ACI and PNI public keys are stored
here with level 2 (written at registration, linking and whenever the PNI
key changes). Rows whose key fails to parse are skipped on read.

---

## 5. Prekey tables

All three are scoped by `account_id_type` (§1.1). Key ids are in
[0, 16777215) (24-bit, see data-dir §3.3); the next ids to use are in the
account file, not here.

### 5.1 `pre_key` (one-time EC prekeys)

| Column | Type | Null | Meaning |
|--------|------|------|---------|
| `_id` | INTEGER | PK | row id |
| `account_id_type` | INTEGER | no | 0 ACI, 1 PNI |
| `key_id` | INTEGER | no | prekey id; UNIQUE together with `account_id_type` |
| `public_key` | BLOB | no | 33-byte serialized Curve25519 public key (`0x05` + 32 bytes) |
| `private_key` | BLOB | no | 32-byte Curve25519 private key |
| `stale_timestamp` | INTEGER | yes | NULL = current. Set to the time (ms) at which a newer batch was uploaded; stale keys are deleted later once older than an age threshold, always keeping a minimum number of most recent keys. |

Not a serialized `PreKeyRecord`: the record is rebuilt from `key_id` and
the two key columns. A one-time prekey row is deleted once a session is
established with it.

### 5.2 `signed_pre_key`

| Column | Type | Null | Meaning |
|--------|------|------|---------|
| `_id` | INTEGER | PK | row id |
| `account_id_type` | INTEGER | no | 0 ACI, 1 PNI |
| `key_id` | INTEGER | no | signed prekey id; UNIQUE with `account_id_type` |
| `public_key` | BLOB | no | 33-byte serialized public key |
| `private_key` | BLOB | no | 32-byte private key |
| `signature` | BLOB | no | 64-byte XEdDSA signature by the identity key over the 33-byte public key |
| `timestamp` | INTEGER | yes (default 0) | ms creation time of the signed prekey (part of the libsignal record) |

The **active** signed prekey is the one whose `key_id` equals
`activeSignedPreKeyId` in the account file. Non-active keys are kept for
a while after rotation (rows older than 30 days are deleted, except the
newest of them) so late messages still decrypt.

### 5.3 `kyber_pre_key`

| Column | Type | Null | Meaning |
|--------|------|------|---------|
| `_id` | INTEGER | PK | row id |
| `account_id_type` | INTEGER | no | 0 ACI, 1 PNI |
| `key_id` | INTEGER | no | Kyber prekey id; UNIQUE with `account_id_type` |
| `serialized` | BLOB | no | complete serialized libsignal **`KyberPreKeyRecord`** (contains id, ML-KEM/Kyber key pair, signature, timestamp; format in `spec/protocol/capture.md` §2) |
| `is_last_resort` | INTEGER | no | Boolean: 1 = last-resort key (never deleted on use), 0 = one-time key (deleted when used) |
| `stale_timestamp` | INTEGER | yes | as in `pre_key`; only ever set on one-time keys |
| `timestamp` | INTEGER | yes (default 0) | copy of the record's own creation timestamp (ms) |

The active last-resort key is the row with `is_last_resort` = 1 whose
`key_id` equals `activeLastResortKyberPreKeyId` in the account file; older
last-resort keys are kept and pruned like signed prekeys.

---

## 6. `session`

| Column | Type | Null | Meaning |
|--------|------|------|---------|
| `_id` | INTEGER | PK | row id |
| `account_id_type` | INTEGER | no | Which local identity the session belongs to: 0 ACI, 1 PNI |
| `address` | TEXT | no | Peer service-id string (ACI plain UUID, or `PNI:` + UUID) |
| `device_id` | INTEGER | no | Peer device id (1 = primary) |
| `record` | BLOB | no | serialized libsignal **`SessionRecord`** (current state plus archived previous states) |

UNIQUE (`account_id_type`, `address`, `device_id`). Sessions with the
account's own other devices are stored under the own ACI. "Archiving" a
session keeps the row but moves the current state into the previous
states inside `record`; a session is considered active iff its current
state has a sender chain. Rows whose record fails to parse are ignored.

---

## 7. Sender keys

### 7.1 `sender_key`

| Column | Type | Null | Meaning |
|--------|------|------|---------|
| `_id` | INTEGER | PK | row id |
| `address` | TEXT | no | Service-id string of the sender (a peer, or the account's own ACI for our own outgoing sender keys) |
| `device_id` | INTEGER | no | Sender's device id |
| `distribution_id` | BLOB | no | 16-byte distribution id (UUID) |
| `record` | BLOB | no | serialized libsignal **`SenderKeyRecord`** |
| `created_timestamp` | INTEGER | no | ms when the row was **first** inserted; not changed when `record` is updated. For our own key it is the key's age, used to decide rotation. |

UNIQUE (`address`, `device_id`, `distribution_id`). Not scoped by
`account_id_type`. All sender keys are deleted on registration and
linking.

### 7.2 `sender_key_shared`

Which recipient devices already received our sender-key distribution
message for a distribution id.

| Column | Type | Null | Meaning |
|--------|------|------|---------|
| `_id` | INTEGER | PK | row id |
| `address` | TEXT | no | Recipient service-id string |
| `device_id` | INTEGER | no | Recipient device id |
| `distribution_id` | BLOB | no | 16-byte distribution id |
| `timestamp` | INTEGER | no | ms when it was (last) marked shared |

UNIQUE (`address`, `device_id`, `distribution_id`). Rows are removed when
a session with that device is archived, when the recipient's identity key
changes, after a retry receipt from that device, and when our own key for
the distribution id is discarded, so the key is re-sent next time.

---

## 8. `group_v2` and `group_v2_member`

### 8.1 `group_v2`

| Column | Type | Null | Default | Meaning |
|--------|------|------|---------|---------|
| `_id` | INTEGER | PK | | internal group id (referenced by `group_v2_member.group_id`) |
| `storage_id` | BLOB | yes, UNIQUE | | 16-byte storage item id (GROUPV2 type). Regenerated on every local update of the group row. |
| `storage_record` | BLOB | yes | | last seen serialized **`GroupV2Record`** protobuf |
| `group_id` | BLOB | no, UNIQUE | | 32-byte group id (derived from the master key) |
| `master_key` | BLOB | no | | 32-byte `GroupMasterKey` |
| `group_data` | BLOB | yes | | serialized **`DecryptedGroup`** protobuf (latest known group state). NULL = group known only by its master key (state not fetched yet, or fetch not permitted). |
| `distribution_id` | BLOB | no, UNIQUE | | 16-byte sender-key distribution id used for this group; random, chosen when the row is first created |
| `endorsement_expiration_time` | INTEGER | no | 0 | ms expiration of the stored group-send endorsements; 0 = none |
| `blocked` | INTEGER | no | 0 | Boolean |
| `profile_sharing` | INTEGER | no | 0 (fresh) / 1 (migrated, §14) | Boolean: our profile key is shared with the group ("message request accepted"). signal-cli always writes this column explicitly. |
| `permission_denied` | INTEGER | no | 0 | Boolean: the last attempt to fetch the group failed with "not a member / forbidden" |

### 8.2 `group_v2_member`

Group-send endorsements per member (only the endorsement data; the
member list itself is inside `group_data`).

| Column | Type | Null | Meaning |
|--------|------|------|---------|
| `_id` | INTEGER | PK | row id |
| `group_id` | INTEGER | no | → `group_v2._id`, ON DELETE CASCADE |
| `recipient_id` | INTEGER | no | → `recipient._id`, ON DELETE CASCADE |
| `endorsement` | BLOB | no | serialized zkgroup `GroupSendEndorsement` for that member |

UNIQUE (`group_id`, `recipient_id`). Replaced as a whole whenever new
endorsements are received; cleared when they are dropped.

## 9. `group_v1` and `group_v1_member` (legacy groups)

| Column | Type | Null | Default | Meaning |
|--------|------|------|---------|---------|
| `_id` | INTEGER | PK | | internal id |
| `storage_id` | BLOB | yes, UNIQUE | | 16-byte storage item id (GROUPV1 type); regenerated on every local update |
| `storage_record` | BLOB | yes | | last seen serialized **`GroupV1Record`** protobuf |
| `group_id` | BLOB | no, UNIQUE | | 16-byte v1 group id |
| `group_id_v2` | BLOB | yes, UNIQUE | | 32-byte v2 group id this v1 group would migrate to (derived from the v1 id) |
| `name` | TEXT | yes | | group title |
| `color` | TEXT | yes | | colour name (legacy) |
| `expiration_time` | INTEGER | no | 0 | disappearing-message timer in **seconds** |
| `blocked` | INTEGER | no | 0 | Boolean |
| `archived` | INTEGER | no | 0 | Boolean |

`group_v1_member`: `_id` INTEGER PK; `group_id` INTEGER NOT NULL →
`group_v1._id` (cascade); `recipient_id` INTEGER NOT NULL →
`recipient._id` (cascade); UNIQUE (`group_id`, `recipient_id`). The member
list of the v1 group. When a v2 group with the matching id is learned,
the v1 row is deleted (its blocked flag carries over).

---

## 10. `storage_id` (unknown storage records)

Storage-service items signal-cli does not process but must keep in the
manifest.

| Column | Type | Null | Meaning |
|--------|------|------|---------|
| `_id` | INTEGER | PK | row id |
| `type` | INTEGER | no | `ManifestRecord.Identifier` type number (`spec/service/storage.md` §4.2; e.g. 5 story distribution list, 7 call link, 8 chat folder, 9 notification profile, 0 unknown, ≥ 10 future types) |
| `storage_id` | BLOB | no, UNIQUE | raw storage item id (16 bytes as generated by Signal clients) |

Items of processed types (contacts, account, groups, sticker packs) are
not here; their ids are in the `storage_id` columns of `recipient`,
`group_v1`, `group_v2` and `sticker`.

---

## 11. `sticker`

Sticker packs known to the account (installed, or removed but still
tracked for storage sync). The images themselves are under
`<DATA>/stickers/` (data-dir §6.3).

| Column | Type | Null | Default | Meaning |
|--------|------|------|---------|---------|
| `_id` | INTEGER | PK | | row id |
| `pack_id` | BLOB | no, UNIQUE | | 16-byte sticker pack id |
| `pack_key` | BLOB | no | | 32-byte sticker pack key (decrypts the pack manifest and stickers) |
| `installed` | INTEGER | no | 0 | Boolean |
| `position` | INTEGER | no | 0 | order among installed packs (next free position when installed); 0 when not installed |
| `deleted_timestamp` | INTEGER | no | 0 | ms when the pack was uninstalled (0 = not deleted); kept so the deletion syncs |
| `storage_id` | BLOB | yes, UNIQUE | | 16-byte storage item id (STICKER_PACK type); NULL for packs that were never installed or synced |
| `storage_record` | BLOB | yes | | last seen serialized **`StickerPackRecord`** protobuf |

---

## 12. Message send log: `message_send_log_content`, `message_send_log`

Behaviour: `spec/service/send-log.md`. Layout:

`message_send_log_content` (one row per logged send):

| Column | Type | Null | Meaning |
|--------|------|------|---------|
| `_id` | INTEGER | PK | content id |
| `group_id` | BLOB | yes | group id of a group send (16 bytes v1 / 32 bytes v2), else NULL |
| `timestamp` | INTEGER | no | send timestamp (ms), indexed (`mslc_timestamp_index`) |
| `content` | BLOB | no | plaintext serialized `Content` protobuf as sent |
| `content_hint` | INTEGER | no | 0 DEFAULT, 1 RESENDABLE, 2 IMPLICIT |
| `urgent` | INTEGER | no (fresh: no default; migrated: default 1) | Boolean urgent flag of the send |

`message_send_log` (one row per recipient device of a logged send):

| Column | Type | Null | Meaning |
|--------|------|------|---------|
| `_id` | INTEGER | PK | row id |
| `content_id` | INTEGER | no | → `message_send_log_content._id`, ON DELETE CASCADE; indexed (`msl_content_index`) |
| `address` | TEXT | no | recipient service-id string |
| `device_id` | INTEGER | no | recipient device id |

Index `msl_recipient_index` on (`address`, `device_id`, `content_id`).
This table is a short-lived resend cache (entries are pruned after a
day); it is not needed to operate an account.

---

## 13. `cdsi`

Phone numbers sent to contact discovery (CDSI), for incremental queries.

| Column | Type | Null | Meaning |
|--------|------|------|---------|
| `_id` | INTEGER | PK | row id |
| `number` | TEXT | no, UNIQUE | E.164 number |
| `last_seen_at` | INTEGER | no | ms when the number was last part of a query result set / still present locally |

Rewritten after each full CDSI query and cleared together with
key_value `cdsi-token` when the server rejects the token. Only meaningful
together with that token.

---

## 14. Fresh vs. migrated databases

A database migrated from an older schema has the same tables and columns
as a fresh one (B1 `AccountDatabase` migrations 2–30), with these
differences a reader must tolerate:

| Area | Fresh (created by 0.14.7) | Migrated |
|------|---------------------------|----------|
| Column order | as listed in this file | columns added by later migrations come last: `recipient` ends with `needs_pni_signature`, `nick_name_given_name`, `nick_name_family_name`, `note`, `discoverable`, `profile_phone_number_sharing`, `expiration_time_version`, `pni_signature_verified` (in that order of addition); `group_v1`/`group_v2` have `storage_id`, `storage_record` after the original columns, then (v2) `profile_sharing`, `endorsement_expiration_time`; `sticker` has `position`, `deleted_timestamp`, `storage_id`, `storage_record` last; `pre_key.stale_timestamp`, `kyber_pre_key.stale_timestamp`, `kyber_pre_key.timestamp`, `message_send_log_content.urgent` last |
| `group_v2.profile_sharing` default | 0 | 1 (rows that existed before the column was added read as sharing enabled) |
| `message_send_log_content.urgent` | NOT NULL, no default | NOT NULL DEFAULT 1 |
| `sticker.storage_id` uniqueness | inline UNIQUE (automatic index) | separate unique index `sticker_storage_id_index` |
| Dropped data | — | migrations to key-by-service-id (schema 10, 15) and to TEXT recipient ids (22) dropped identity, session, sender-key, shared-sender-key and send-log rows that had no service id; schema 13 dropped identity / shared-sender-key rows for the all-zero ACI; schema 22 dropped a `pni` value that was shared by several old rows |

Older schemas also had columns that no longer exist (e.g. a BLOB `uuid`
on `recipient`, `recipient_id` keys on `identity`/`session`/`sender_key`);
they are all gone at version 30, so a database at `user_version` 30 has
exactly the column set described here.

---

## 15. Importer checklist: where each piece of state lives

"JSON" = the account file `<DATA>/data/<path>` (data-dir §3); `aci.`/`pni.`
= its `aciAccountData`/`pniAccountData` objects. "DB" = `account.db`.

| State | Location | Notes |
|-------|----------|-------|
| Which accounts exist, their file names, environment | `<DATA>/data/accounts.json` `accounts[]`: `path`, `environment`, `number`, `uuid` | data-dir §2 |
| Account file / DB format versions | JSON `version` (expect 10); DB `PRAGMA user_version` (expect 30) | data-dir §3.1, §1 |
| Service environment | JSON `serviceEnvironment` | `LIVE` / `STAGING` |
| Registered flag | JSON `registered` and `deviceId` > 0 | data-dir §3.2 |
| Own phone number | JSON `number` | E.164 |
| Own ACI | JSON `aci.serviceId` (also `accounts.json` `uuid`) | plain UUID |
| Own PNI | JSON `pni.serviceId` | `PNI:` + UUID (plain UUID accepted) |
| ACI identity key pair | JSON `aci.identityPublicKey` (base64, 33 B) and `aci.identityPrivateKey` (base64, 32 B) | not in the DB |
| PNI identity key pair | JSON `pni.identityPublicKey`, `pni.identityPrivateKey` | not in the DB |
| Registration ids | JSON `aci.registrationId`, `pni.registrationId` | |
| Device id | JSON `deviceId` | 1 = primary |
| Multi-device flag | JSON `isMultiDevice` | |
| Chat-service password | JSON `password` | HTTP basic auth with ACI (or number) and device id |
| Encrypted device name | JSON `encryptedDeviceName` | base64 without padding |
| One-time EC prekeys | DB `pre_key` (per `account_id_type`) | raw key columns |
| Signed prekeys | DB `signed_pre_key`; active id: JSON `aci.activeSignedPreKeyId` / `pni.activeSignedPreKeyId` | |
| Kyber prekeys (one-time and last-resort) | DB `kyber_pre_key` (`is_last_resort`); active last resort: JSON `aci.activeLastResortKyberPreKeyId` / `pni.…` | serialized `KyberPreKeyRecord` |
| Next prekey ids | JSON `aci.`/`pni.` `nextPreKeyId`, `nextSignedPreKeyId`, `nextKyberPreKeyId` | 24-bit counters |
| Sessions | DB `session` (`account_id_type`, `address`, `device_id`, `record`) | `SessionRecord` |
| Sender keys (received and own) | DB `sender_key`; who has our key: DB `sender_key_shared` | `SenderKeyRecord`; 16-byte distribution ids |
| Remote identity keys and trust | DB `identity` (`address`, `identity_key`, `trust_level` 0/1/2, `added_timestamp`) | own keys appear with level 2 |
| Recipients / contacts / profiles | DB `recipient` | contact rule §2.1; profile present iff `profile_capabilities` non-NULL |
| Recipients' profile keys and credentials | DB `recipient.profile_key`, `recipient.profile_key_credential` | |
| Blocked contacts | DB `recipient.blocked` | |
| Groups v2 | DB `group_v2` (`group_id`, `master_key`, `group_data` = `DecryptedGroup`, `distribution_id`, `blocked`, `profile_sharing`, `permission_denied`); endorsements: DB `group_v2_member` + `group_v2.endorsement_expiration_time` | |
| Groups v1 | DB `group_v1` + `group_v1_member` | legacy |
| Own profile key | JSON `profileKey` (base64, 32 B) | self `recipient.profile_key` is not used |
| Own profile (name, about, avatar path, …) | DB self row of `recipient`, `profile_*` columns | |
| Storage key | JSON `storageKey` (linked devices only); otherwise derived from the master key | data-dir §3.2 |
| Master key | JSON `pinMasterKey`, or derived from JSON `accountEntropyPool` when that is set | AEP wins: setting an AEP clears `pinMasterKey` and `storageKey` |
| Account entropy pool | JSON `accountEntropyPool` | 64 chars `0-9a-z` |
| Media root backup key | JSON `mediaRootBackupKey` | base64, 32 B |
| PIN | JSON `registrationLockPin` (plaintext) | registration lock token and recovery password are derived from the master key, not stored |
| Storage-sync state | DB key_value `storage-manifest-version`; file `<path>.d/storage-manifest`; DB `storage_id` table; `storage_id`/`storage_record` columns of `recipient`, `group_v1`, `group_v2`, `sticker` | data-dir §4.1 |
| Own username | JSON `username` | |
| Own username link | JSON `usernameLinkEntropy` (base64, 32 B), `usernameLinkServerId` (UUID); colour: DB key_value `username-link-color` | |
| Others' usernames | DB `recipient.username` | |
| Account settings (read receipts, typing, link previews, sealed-sender indicators, number sharing, discoverability, unrestricted UA) | DB key_value `config-*` keys and `unrestricted-unidentified-access` | §3 |
| Sticker packs | DB `sticker` (`pack_id`, `pack_key`, `installed`, `position`, `deleted_timestamp`); images and manifest: `<DATA>/stickers/<hex pack id>/` | |
| Last receive time | DB key_value `last-receive-timestamp` | |
| PNI change-number replay guard | DB key_value `last-applied-pni-change-server-timestamp` | |
| Registration session in progress | DB key_value `verification-session-id`, `verification-session-number` | only during registration |
| Contact discovery state | DB `cdsi` + key_value `cdsi-token`, `last-recipients-refresh` | |
| Undecrypted cached envelopes | `<path>.d/msg-cache/` | data-dir §5 |
| Resend log | DB `message_send_log_content`, `message_send_log` | short-lived |
| Downloaded attachments and avatars | `<DATA>/attachments/`, `<DATA>/avatars/` (shared by all accounts) | data-dir §6 |

---

## 16. Listing order

Sources: B3. None of the listing queries below has an ORDER BY clause, and
none of the commands sorts its result afterwards. The order in the output
is therefore whatever SQLite returns. For every query here, SQLite 3.53.2
(the version bundled with 0.14.7) plans a **full scan of the table's
rowid b-tree**, so rows come out in **ascending `_id` order**. The probe
confirmed this. No query can be answered from an index alone: each one
reads columns that no index holds. signal-cli never runs `ANALYZE`, so no
statistics can change the plan. An implementation that wants
byte-identical output should sort by `_id` explicitly and not rely on its
own engine's unordered scan order.

One trap the probe showed: a query that reads **only** `identity.address`
is answered from the UNIQUE index on `address` and comes back in address
order. The real identity listing also reads the key, timestamp and trust
level, so it scans the table and comes back in `_id` order.

CLI plain-text, CLI `--output=json` and JSON-RPC all go through the same
command code, so they share one order.

### 16.1 `listIdentities`

- **Without `-n`**: every `identity` row in ascending `_id` order. Rows
  whose key fails to parse are skipped (§4). Then each row's address is
  resolved to a recipient, and a row is **dropped** when that recipient
  has a service id that differs from the row's address. In practice this
  hides the PNI row of anyone whose ACI is known, including the account's
  own PNI row. The account's own ACI row is listed, at whatever position
  it was first inserted. Resolving the address can create a `recipient`
  row as a side effect. That does not change the identity order.
- **With `-n <recipient>`**: at most one entry: the identity row for that
  recipient's service id (its ACI if known, else its PNI). An empty list
  is returned if the recipient has no service id, is unregistered, or has
  no identity row.

### 16.2 `listContacts`

All forms read `recipient` in ascending `_id` order. Rows with none of
`number`, `pni`, `aci` are always excluded (§2.1).

- **Default** (contacts only): additionally requires
  `unregistered_timestamp` NULL, the contact rule of §2.1, and `hidden` =
  0.
- **`-a` / `--all-recipients`**: no contact filter. Every row with an
  identifier, including the self row, unregistered rows and hidden rows,
  in `_id` order.
- **`--blocked true|false`**: an extra filter on `blocked`. Same order.
- **`--name`**: applied after the query, keeping the row order. It keeps
  rows whose contact name or profile display name equals the argument
  exactly.
- **Positional recipients**: each is resolved to a recipient id, and the
  ids become an `_id IN (…)` filter. SQLite then walks the requested ids
  in **ascending `_id` order** (probe: ids given as 5, 2, 4 returned 2, 4,
  5). So the output does **not** follow argument order, and a recipient
  named twice, or two arguments that resolve to the same row, appear
  once. Without `-a`, the contact filter still applies to them. Recipients
  that fail to resolve (unregistered) are left out. If none resolves, the
  result is empty.
- Before the final query, signal-cli refreshes profiles: those of the
  listed registered recipients, or of the named recipients. That refresh
  can merge or split rows (§16.4), and the final listing reflects the
  state after it.

### 16.3 `listGroups`

- **Without `-g`**: all `group_v2` rows in ascending `_id` order,
  **followed by** all `group_v1` rows in ascending `_id` order. Rows that
  fail to decode are skipped.
- **With `-g <id>…`**: the same full list, filtered to the requested ids.
  It keeps store order, not argument order, and duplicates collapse.
  Unknown ids simply produce no entry.
- **Member lists inside a group** (`members`, `pendingMembers`,
  `requestingMembers`, `admins`, `banned`, in plain text and JSON) are
  built as hash sets. Their order is hash-iteration order. It comes from
  neither the database nor the `DecryptedGroup` member order, so a
  client must not depend on it. For v1 groups, the member ids are read in
  ascending `recipient_id` order (via the `(group_id, recipient_id)`
  unique index), but they then go through the same hash sets.

### 16.4 When `_id`s change (what moves a row in the order)

- **`identity`**: one row per address, written by an upsert keyed on
  `address`. A **key change** and a **trust-level change** both update
  the existing row **in place**: `_id` and position stay the same (probe
  confirmed). A key change resets `added_timestamp` (§4); a trust change
  keeps it. A row is deleted only when its recipient is removed, or for
  the account's old PNI when the PNI changes. `identity._id` is a plain
  INTEGER PRIMARY KEY **without** AUTOINCREMENT. A new row therefore gets
  the current maximum `_id` + 1, so the id of a deleted *highest* row
  can be **reused** (probe: delete the top row, insert another, and it
  takes the freed id). A re-added identity always goes to the end.
  Migrations from older schema versions copy `_id` unchanged.
- **`recipient`**: `_id` is AUTOINCREMENT, so ids are never reused and a
  new row always goes **last**. Updates to identifiers, contact, profile,
  storage or registration fields keep `_id`. The ordering effects of
  merges and splits (details in §2.1):
  - A **merge** keeps the row whose ACI matches the incoming address,
    otherwise the row whose PNI matches. The survivor keeps its `_id` and
    position, and the merged rows disappear. When no existing row matches
    by ACI or PNI (for example, a number-only row and a PNI-only row
    being joined), all the matching rows are emptied and a **new row is
    appended**.
  - If the incoming address's ACI differs from the ACI of the single row
    that matched by number, PNI or username, that row loses those
    identifiers in place, and a **new row** holding the incoming address
    is appended.
  - A **split** (an unregistered row that had both an ACI and a PNI)
    keeps the ACI on the existing row and **appends** a new row holding
    the PNI and number.
  - Rows emptied of all identifiers stay in the table but never appear in
    a listing.
- **`group_v2` / `group_v1`**: updating a known group looks up its `_id`
  by group id and upserts on that `_id`, so `_id` and position stay. The
  ids are plain INTEGER PRIMARY KEYs (no AUTOINCREMENT, so max + 1, with
  the same reuse caveat as `identity`). A group that is deleted and later
  re-added gets a new id at the end. **Local v1→v2 migration** (a v2
  message arrives for a known v1 group) deletes the v1 row and inserts a
  new `group_v2` row, so that group moves from the v1 tail to the end of
  the v2 part. A v2 group known only by master key is not stored until
  its first update, so its position is set by that first write.
- **`sticker`**: rows are inserted once per pack and afterwards only
  updated in place (install/uninstall, storage sync), so `_id` stays.
  Rows are deleted only by the one-time migration from the legacy JSON
  store, which rewrites the table in the legacy list order.

### 16.5 Other database-backed output

- **`listStickerPacks`**: `sticker` rows in ascending `_id` order (first
  insertion order). The order is **not** by `position`, and uninstalled
  (still tracked) packs are included. The stickers within a pack follow
  the locally stored manifest file, not the database.
- **`listAccounts`** lists the accounts loaded by the multi-account manager (not from `account.db`), and
  **`listDevices`** returns the server's device list in the server's
  order. Neither touches `account.db`.
- No other command output exposes database row order.
