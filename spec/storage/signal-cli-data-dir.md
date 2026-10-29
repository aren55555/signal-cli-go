# signal-cli 0.14.7 data directory: layout and JSON files

Spec role: prose and tables only; no code. This file describes **what is on
disk** in a data directory written by signal-cli 0.14.7, so that an
importer can read it without running signal-cli. It covers the directory
tree, `accounts.json`, the per-account JSON file, the per-account `.d/`
directory, media directories, locking, and which older layouts 0.14.7
still converts on load.

The SQLite database `account.db` is specified separately in
`signal-cli-account-db.md` (same directory). That file ends with an
importer checklist that points back into this one.

Related specs (not repeated here):

- Default data-directory path and the `-d/--data-dir/-c/--config` option:
  `spec/surface/cli/index.md` §6 (row 6) and `spec/surface/cli/argparse.md`.
- Received-envelope cache lifecycle and current file format:
  `spec/protocol/capture.md` §1 and §3. §5 below only adds what that
  file leaves out.
- Storage-service records and the manifest: `spec/service/storage.md`
  §4.
- Encrypted device name: `spec/service/linking.md` §4.5.
- PIN, master key, account entropy pool (AEP): `spec/service/svr.md`,
  `spec/crypto/pin.md`, `spec/service/storage.md` §2.

## Sources

| Ref | What | URL @ version |
|-----|------|---------------|
| D1 | signal-cli | https://github.com/AsamK/signal-cli @ tag `v0.14.7` = commit `b01b6b370dc063599a1a2b9fde0f5ff4e2d78fe8`, files under `lib/src/main/java/org/asamk/signal/manager/`: `SignalAccountFiles.java`, `internal/{PathConfig,AccountFileUpdaterImpl,ProvisioningManagerImpl,RegistrationManagerImpl,ManagerImpl}.java`, `storage/{SignalAccount,Utils,AttachmentStore,AvatarStore,AccountDatabase,Database}.java`, `storage/accounts/{AccountsStore,AccountsStorage}.java`, `storage/messageCache/{MessageCache,CachedMessage}.java`, `storage/stickerPacks/{StickerPackStore,JsonStickerPack}.java`, `storage/recipients/RecipientAddress.java`, `storage/{prekeys,sessions,identities,senderKeys,recipients,groups}/Legacy*.java`, `util/{MessageCacheUtils,IOUtils,KeyUtils}.java`, `helper/{AccountHelper,ReceiveHelper}.java`, `config/ServiceConfig.java`, `api/{ServiceEnvironment,GroupId,StickerPackId}.java` |
| D2 | signal-service-java (Turasa fork) | https://github.com/Turasa/libsignal-service-java @ tag `v2.15.3_unofficial_151` = commit `66956cb4a0394d50cee3590209baf58312084a44`: `lib/libsignal-service/src/main/java/org/whispersystems/signalservice/api/storage/SignalStorageManifest.kt` |
| D3 | Reference binary and its jars | `/work/third_party/signal-cli-0.14.7/lib/*.jar` (signal-cli 0.14.7 release; `libsignal-cli-0.14.7.jar` class list used to enumerate storage classes; `sqlite-jdbc-3.53.2.1.jar`). Offline `jshell` (JDK 25.0.5) runs against a **scratch** data directory under `~/.cache` (deleted afterwards): created an unregistered staging account with the bundled library's own account-creation routine (fictional number `+15555550100`, throwaway keys, no network), dumped the resulting files, took a Java file lock and read `/proc/locks`, and printed `ServiceId`, `Hex` and `GroupId` string forms. |

Everything below comes from these sources. No Signal server was contacted.

---

## 1. Directory tree

`<DATA>` is the data directory (default rules: `spec/surface/cli/index.md`
§6). signal-cli derives four fixed subdirectories from it (D1
`PathConfig`):

| Path | Scope | Content | Section |
|------|-------|---------|---------|
| `<DATA>/data/` | all accounts | account index and per-account state | §2–§4 |
| `<DATA>/data/accounts.json` | all accounts | account index | §2 |
| `<DATA>/data/<path>` | one account | account file (JSON) | §3 |
| `<DATA>/data/<path>.d/` | one account | account database, envelope cache, storage manifest | §4 |
| `<DATA>/attachments/` | **shared** by all accounts | downloaded attachments | §6.1 |
| `<DATA>/avatars/` | **shared** | contact, profile and group avatars | §6.2 |
| `<DATA>/stickers/` | **shared** | downloaded sticker packs | §6.3 |

Nothing else is created under `<DATA>`. The media directories are not
per account: two accounts in the same data directory write into the same
`attachments/`, `avatars/` and `stickers/`.

Permissions at creation (D1 `IOUtils`, confirmed in D3): directories that
signal-cli creates itself (`data/`, `<path>.d/`, `msg-cache/` and its
subdirectories, media directories) are mode `0700`; `accounts.json` and
the account file are created `0600`. `account.db` and its `-wal`/`-shm`
files are created by SQLite with the process umask (D3: `0644`).

`<path>` is chosen when an account is added (§2.3). It is opaque; an
importer must take it from `accounts.json`, never guess it from the
phone number.

---

## 2. `accounts.json`

### 2.1 Format

A single JSON object, written pretty-printed by Jackson (two-space
indent, `" : "` between key and value). Unknown keys are ignored when
read. D3 example (scratch account):

```
{
  "accounts" : [ {
    "path" : "616701",
    "environment" : "STAGING",
    "number" : "+15555550100",
    "uuid" : null
  } ],
  "version" : 2
}
```

| Key | JSON type | Meaning |
|-----|-----------|---------|
| `version` | number or absent/null | Index format version. **2** is current and is what 0.14.7 writes. Absent or null is read as **1**. A value above 2 makes every account operation fail with `Accounts file was created by a more recent version: <n>`; a value below 1 fails with `Accounts file was created by a no longer supported older version: <n>`. |
| `accounts` | array of objects | One entry per known account, in file order. May be empty. |

Each entry of `accounts`:

| Key | JSON type | Meaning |
|-----|-----------|---------|
| `path` | string, never null | Name of the account file inside `<DATA>/data/` (and, with `.d` appended, of its directory). See §2.3. |
| `environment` | string or null | `"LIVE"` or `"STAGING"` (service environment the account belongs to). **null** means "unknown" and the entry matches **any** environment. |
| `number` | string or null | The account's phone number, E.164 with leading `+`. null if the entry was displaced by another entry with the same number (§2.4) or before the number is known. |
| `uuid` | string or null | The account's ACI as a lower-case UUID string (no `PNI:` prefix). null until known (e.g. an account that has started but not finished registration). |

### 2.2 How entries are selected

For a run with service environment E (`LIVE` unless
`--service-environment staging`):

- An entry is **visible** if its `environment` is null or equals E.
- Lookup by number: the first visible entry (in file order) whose `number`
  equals the given number exactly. Lookup by ACI: the first visible entry
  whose `uuid` equals the ACI string.
- "All accounts" (multi-account daemon, `listAccounts`): every visible
  entry whose `number` is non-null. Entries with a null `number` are
  invisible there.
- An entry whose account file does not exist, is a directory, or is empty
  (length 0) counts as **not registered** (D1
  `SignalAccount.accountFileExists`).
- After loading, signal-cli checks that the account file's `number` (or
  ACI) matches the one used for the lookup; a mismatch is an error
  (`Number in account file doesn't match expected number: …`,
  `ACI in account file doesn't match expected ACI: …`). It also refuses an
  account file whose own `serviceEnvironment` differs from E
  (`Account is registered in another environment: …`).

### 2.3 How `path` is chosen

- **New accounts** (registration or linking): a random decimal integer in
  the range 100000–999999 inclusive (always six digits), retried until
  neither `<DATA>/data/<n>` nor `<DATA>/data/<n>.d` exists (D1
  `AccountsStore.generateNewAccountPath`).
- **Legacy installs**: if `accounts.json` does not exist when signal-cli
  starts, it creates it. Every regular file directly in `<DATA>/data/`
  whose **name** libphonenumber considers a "possible number" (in practice
  names like `+15555550100`) becomes an entry with `path` = that file
  name, `number` = that file name, `environment` = null, `uuid` = null,
  and the new file gets `version` 2. So on old installs `path` can be a
  phone number string.
- Linking first looks for an existing entry by ACI, then by number, and
  reuses its `path` (updating `number`/`uuid`/`environment`) instead of
  adding a new one (D1 `ProvisioningManagerImpl`).

### 2.4 Updates and duplicates

- Adding or updating an entry sets its `environment` to E and fills
  `number`/`uuid`. Any **other** visible entry with the same `number` gets
  its `number` set to null.
- Adding a new entry also nulls `uuid` on other visible entries with the
  same ACI. Updating an existing entry does **not** reliably do so (its
  ACI comparison never matches), so two entries can carry the same `uuid`.
  Lookup by ACI then takes the first in file order.
- Removing an account (`deleteLocalAccountData`, failed link cleanup)
  deletes its entry.
- Version 1 to 2 upgrade (done on first read): for each entry with null
  `environment`, signal-cli opens the account file and copies its
  `serviceEnvironment` into the entry (entries whose file cannot be
  loaded keep null). The upgraded file is written back with `version` 2.
  If the upgrade fails it is logged and the version-1 content is used
  as is.

### 2.5 Locking of `accounts.json`

Every read and every write opens `accounts.json` read-write and takes an
exclusive whole-file lock for the duration of that one operation only
(§7). The lock is **not** held while an account is in use. While waiting
signal-cli logs `Config file is in use by another instance, waiting…`
(INFO) and blocks.

---

## 3. The account file `<DATA>/data/<path>`

### 3.1 General

- UTF-8 JSON object, written pretty-printed by Jackson (same style as
  §2.1), rewritten in place (seek to 0, write, truncate, fsync) whenever
  any field changes. Unknown keys are ignored on read.
- Held under an exclusive lock for as long as signal-cli has the account
  open (§7).
- The two identity key pairs, the registration ids, the password, the
  device id, the prekey id counters, the profile key, and all PIN / master
  key / AEP material live **only** in this file, not in `account.db`.
- Layout is selected by the top-level `version` key:

| `version` | Layout | What 0.14.7 does on load |
|-----------|--------|--------------------------|
| absent or null | legacy flat layout (§3.4) | migrates (see §3.5), then rewrites as version 10 |
| 0 or negative | — | refuses: `Config file was created by a no longer supported older version: <n>` |
| 1 – 7 | legacy flat layout (§3.4) | migrates, rewrites as version 10 |
| 8, 9 | current structured layout (§3.2) | reads, rewrites as version 10 immediately; schedules server-side follow-ups on the next successful account check (§3.5) |
| **10** | current structured layout (§3.2) | current; what 0.14.7 writes |
| 11 or more | — | refuses: `Config file was created by a more recent version: <n>` |

### 3.2 Current layout (versions 8–10)

D3 example of a freshly created, not yet registered account (scratch
data; keys are throwaway):

```
{
  "version" : 10,
  "timestamp" : 1793292002700,
  "serviceEnvironment" : "STAGING",
  "registered" : false,
  "number" : "+15555550100",
  "username" : null,
  "encryptedDeviceName" : null,
  "deviceId" : 1,
  "isMultiDevice" : false,
  "password" : "UbffUm+Sy5hGlL2nQhUfLhvy",
  "aciAccountData" : {
    "serviceId" : null,
    "registrationId" : 6851,
    "identityPrivateKey" : "oPyqJvqfMWt6HWpUkhY1galZGhU63s0BrlhF0UicEmk=",
    "identityPublicKey" : "BWLEyxpz0VrY5SRJeG2p9YYAkHDxG9kDZVNva0Dgy9Uo",
    "nextPreKeyId" : 13517850,
    "nextSignedPreKeyId" : 16355817,
    "activeSignedPreKeyId" : -1,
    "nextKyberPreKeyId" : 3877776,
    "activeLastResortKyberPreKeyId" : -1
  },
  "pniAccountData" : { …same keys… },
  "registrationLockPin" : null,
  "pinMasterKey" : null,
  "storageKey" : null,
  "accountEntropyPool" : null,
  "mediaRootBackupKey" : null,
  "profileKey" : "3qk/OHF9Wnnkt1yzKXNWGKlI5wH4K2wyEFROn4Sy1sY=",
  "usernameLinkEntropy" : null,
  "usernameLinkServerId" : null
}
```

0.14.7 always writes **all** keys below, in this order, using JSON `null`
for absent values. When reading, a missing key is treated like Jackson's
default for the type: missing string → null, missing number → **0**,
missing boolean → false. (Files at version 8 or 9 may lack keys that were
added later; the zero defaults then apply. In particular a missing
`nextKyberPreKeyId` reads as 0 and a missing
`activeLastResortKyberPreKeyId` as 0, not −1.)

"base64" below always means standard base64 (RFC 4648 §4, alphabet with
`+` and `/`) **with** `=` padding, unless stated otherwise.

Top-level keys:

| Key | JSON type | Encoding / range | Meaning |
|-----|-----------|------------------|---------|
| `version` | number | 10 | Account-file format version (§3.1). |
| `timestamp` | number | ms since epoch | Wall-clock time of the last write. Informational; never read back. |
| `serviceEnvironment` | string | `"LIVE"` or `"STAGING"` | Environment the account is registered in. Must be one of these two names; anything else makes loading fail. |
| `registered` | boolean | | True once registration or linking completed. The account counts as registered only if this is true **and** `deviceId` > 0. Set to false when the server rejects the credentials with an authorization failure (during the account check or while receiving), and after `unregister` or account deletion. |
| `number` | string or null | E.164 with `+` | Own phone number. |
| `username` | string or null | e.g. `name.42` | Own Signal username (nickname plus discriminator), if one is set. An empty string is read as null. |
| `encryptedDeviceName` | string or null | base64 **without** padding | Encrypted `DeviceName` message (format: `spec/service/linking.md` §4.5) for this device. Normally set on linked devices and when the device name is changed; null on a primary after registration. |
| `deviceId` | number | 1 = primary; ≥ 2 linked; 0 = not yet assigned | This device's id. A primary is exactly `deviceId` 1. |
| `isMultiDevice` | boolean | | True if the account has (or, for a linked device, is one of) several devices. Always true on linked devices. |
| `password` | string | opaque ASCII | Password for HTTP basic auth to the chat service. Primaries generate it as base64 of 18 random bytes (24 characters); linked devices get theirs during provisioning. |
| `aciAccountData` | object | §3.3 | Identity data for the ACI. |
| `pniAccountData` | object | §3.3 | Identity data for the PNI. |
| `registrationLockPin` | string or null | the PIN as typed | The **plaintext** Signal PIN, if the user set one with signal-cli (`setPin`, or `register`/`verify` with `--pin`). null if no PIN is known. |
| `pinMasterKey` | string or null | base64 of 32 bytes | Legacy master key (from PIN-based SVR, or received from the primary on a linked device). Cleared when an AEP is set. |
| `storageKey` | string or null | base64 of 32 bytes | Storage-service key received from the primary (linked devices only; a primary never stores one here, it derives it). Cleared when a master key or AEP arrives. |
| `accountEntropyPool` | string or null | 64 characters `0-9a-z` | The account entropy pool. The master key (and from it the storage key) is derived from it when present (`spec/service/storage.md` §2). |
| `mediaRootBackupKey` | string or null | base64 of 32 bytes | Media root backup key. Generated lazily or received during linking. |
| `profileKey` | string or null | base64 of 32 bytes | Own profile key. This is the authoritative copy; the self row in `recipient.profile_key` is not used for self. If null on load of a legacy file, a new random key is generated. An undecodable value makes loading fail (`Config file contains an invalid profileKey, …`). |
| `usernameLinkEntropy` | string or null | base64 of 32 bytes | Username link entropy (`spec/service/usernames.md`). Only used if `usernameLinkServerId` is also non-null. |
| `usernameLinkServerId` | string or null | UUID string | Server id of the username link. |

### 3.3 `aciAccountData` / `pniAccountData`

Both objects have the same keys.

| Key | JSON type | Encoding / range | Meaning |
|-----|-----------|------------------|---------|
| `serviceId` | string or null | ACI: plain lower-case UUID. PNI: written as `PNI:` + lower-case UUID; a plain UUID is also accepted on read. | The account's ACI or PNI. null before registration/linking finished. An unparsable value makes loading fail (`Config file contains an invalid serviceId, …`). |
| `registrationId` | number | 1 – 16380 (libsignal registration id, 14 bits) | Local registration id for this identity, generated at account creation. 0 if unknown. |
| `identityPrivateKey` | string or null | base64 of 32 bytes | Curve25519 private key (raw scalar as libsignal serializes it). |
| `identityPublicKey` | string or null | base64 of 33 bytes | libsignal serialized public key: type byte `0x05` followed by the 32-byte Curve25519 point. The pair is used only if both keys are non-null. |
| `nextPreKeyId` | number | 0 – 16777214 | Id the next one-time EC prekey will get. |
| `nextSignedPreKeyId` | number | 0 – 16777214 | Id the next signed prekey will get. |
| `activeSignedPreKeyId` | number | −1 or a key id | Id of the signed prekey currently uploaded; −1 = none. |
| `nextKyberPreKeyId` | number | 0 – 16777214 | Id the next Kyber prekey (one-time or last-resort; they share one counter) will get. |
| `activeLastResortKyberPreKeyId` | number | −1 or a key id | Id of the Kyber last-resort prekey currently uploaded; −1 = none. |

Prekey id rules (D1 `SignalAccount`, `ServiceConfig`): the counters start
at a uniformly random value in [0, 16777215) when the account is created
or the prekeys are reset, and advance by one per key generated, modulo
16777215 (the largest 24-bit value). The keys themselves are in
`account.db` (`signal-cli-account-db.md` §5); these counters are not.
After a fresh registration or link, the signed prekey and the last-resort
Kyber prekey received in the registration/link step get the current
counter values and the counters move past them.

PNI on primaries: if a primary account (`deviceId` 1) has no PNI identity
key pair on load, 0.14.7 generates a new one, trusts it, and saves. A
linked device without a PNI key pair is rejected at the account check
(`Missing PNI identity key, relinking required`).

### 3.4 Legacy flat layout (version absent or 1–7)

0.14.7 can still load these; they come from signal-cli releases before
the structured layout. Top-level keys it reads:

| Key | Type | Becomes |
|-----|------|---------|
| `username` | string, **required** | the account's phone number (yes, the legacy key for the number is `username`) |
| `password` | string | `password`; generated if missing |
| `serviceEnvironment` | string | `serviceEnvironment`; `LIVE` if missing |
| `registered` | boolean, **required** | `registered` |
| `usernameIdentifier` | string | own `username` (empty string → null) |
| `uuid` | UUID string | ACI |
| `pni` | UUID string | PNI |
| `sessionId`, `sessionNumber` | string | key_value `verification-session-id` / `verification-session-number` |
| `deviceName` | string | `encryptedDeviceName` |
| `deviceId` | number | `deviceId` |
| `isMultiDevice` | boolean | `isMultiDevice` |
| `lastReceiveTimestamp` | number | key_value `last-receive-timestamp` |
| `registrationId` | number | ACI registration id |
| `pniRegistrationId` | number | PNI registration id (random if missing) |
| `identityKey`, `identityPrivateKey` | base64 | ACI identity key pair |
| `pniIdentityKey`, `pniIdentityPrivateKey` | base64 | PNI identity key pair |
| `registrationLockPin` | string | `registrationLockPin` |
| `pinMasterKey`, `storageKey` | base64 | same names |
| `storageManifestVersion` | number | key_value `storage-manifest-version` |
| `preKeyIdOffset`, `nextSignedPreKeyId`, `activeSignedPreKeyId` | number | ACI prekey counters (random / −1 if missing) |
| `pniPreKeyIdOffset`, `pniNextSignedPreKeyId`, `pniActiveSignedPreKeyId` | number | PNI prekey counters |
| `kyberPreKeyIdOffset`, `activeLastResortKyberPreKeyId` | number | ACI Kyber counters |
| `pniKyberPreKeyIdOffset`, `pniActiveLastResortKyberPreKeyId` | number | PNI Kyber counters |
| `profileKey` | base64 | `profileKey` (new random key if missing) |
| `axolotlStore` | object | old JSON protocol store: identity key pair and registration id (override the values above), prekeys, signed prekeys, sessions, identities |
| `recipientStore`, `contactStore`, `profileStore`, `groupStore`, `stickerStore`, `configurationStore`, `threadStore` | objects | old JSON stores, migrated into `account.db` |

Legacy files and directories inside `<path>.d/` that the same migration
consumes and then **deletes** (files first, then the directory):
`recipients-store` (only when the previous version < 5), `pre-keys/`,
`pre-keys-pni/`, `signed-pre-keys/`, `signed-pre-keys-pni/`, `sessions/`,
`identities/`, `sender-keys/`, `shared-sender-keys-store`, `group-cache/`
(the latter only together with a `groupStore` object). The nested formats
of these legacy stores are out of scope for this spec (see §8).

### 3.5 Migrations 0.14.7 performs on load

| Starting point | Migration | When |
|----------------|-----------|------|
| account file version absent or 1–7 | whole legacy conversion of §3.4 into the version-10 JSON plus `account.db` rows; self recipient marked trusted (version < 6); file rewritten as version 10 | during load, before any network use |
| account file version 8 | rewritten as 10 on load; on the next successful account check: **all prekeys force-refreshed** (new signed, last-resort Kyber and one-time keys uploaded), and the step for version 9 | first run of an account command |
| account file version 9 (or 8) | on the next successful account check, if this is a primary with a non-null `registrationLockPin`: the PIN is re-registered with SVR under the (possibly newly generated) master key and registration lock is re-enabled | first run of an account command |
| any version | primary without a PNI identity key pair gets one (§3.3) | during load |
| `account.db` `user_version` 1–29 | schema migrated to 30 (`signal-cli-account-db.md` §1) | when the database is first opened, which happens before the network check |
| `accounts.json` version 1 | §2.4 | first read |

Because the account file is rewritten as version 10 during load (before
the network check runs), a version-8/9 file that was opened once but whose
account check failed (no network) loses the information that the
follow-up steps are still pending.

### 3.6 Oldest layout 0.14.7 can open

0.14.7 accepts any account file it can parse as JSON whose `version` is
absent or 1–10: there is no lower cut-off besides `version` ≤ 0. The
oldest layout it still converts is therefore the legacy flat layout with
the old JSON `axolotlStore` (pre-SQLite, pre-per-file stores). Its
`account.db` migrations reach back to schema version 1.

Recommendation for the importer (facts, not code): only the version-10
JSON layout together with an `account.db` at `user_version` 30 is fully
described by these specs. For account files below version 10, or
databases below 30, the robust path is to refuse with a message telling
the user to run any account command once with signal-cli 0.14.7 (which
performs every local migration in §3.5 during load, even if the
subsequent network check fails), then retry the import. Versions 8 and 9
are structurally identical to 10 apart from possibly missing keys, so
accepting them is trivial; the pending server-side steps of §3.5 (prekey
refresh, PIN re-registration) would then be the importer's
responsibility.

---

## 4. The per-account directory `<DATA>/data/<path>.d/`

Created (mode `0700`) on first access. Contents in a 0.14.7 directory:

| Entry | Kind | Content |
|-------|------|---------|
| `account.db` | file | SQLite 3 database, schema `user_version` 30 (`signal-cli-account-db.md`). |
| `account.db-wal` | file | SQLite write-ahead log. signal-cli opens the database with `journal_mode=wal` and `foreign_keys=ON`, so committed data may still be **only in this file** until a checkpoint. Present while any connection is open; normally removed by SQLite when the last connection closes cleanly. A leftover non-empty `-wal` after a crash is part of the database. |
| `account.db-shm` | file | SQLite WAL shared-memory index. Same lifetime as `-wal`. |
| `msg-cache/` | directory | Received-envelope cache (§5). May be absent. |
| `storage-manifest` | file | Last storage-service manifest seen (§4.1). May be absent. |
| legacy names of §3.4 | files/dirs | Only if a legacy migration failed to delete them. Not read again by 0.14.7 once the account file is version ≥ 8. |

Nothing else is stored here; in particular sessions, prekeys, identities,
sender keys, recipients, groups, sticker pack keys and settings are all in
`account.db`.

Opening a WAL database, even read-only, normally creates `-wal` and
`-shm` beside it if they are missing and the directory is writable (D3:
reading the scratch database with the bundled sqlite-jdbc driver in
read-only mode created both).

### 4.1 `storage-manifest`

The raw bytes of a protobuf `StorageManifest` message
(`spec/service/storage.md` §4.1): field 1 `version` (uint64, the manifest
version) and field 2 `value` (bytes). Unlike on the wire, `value` here is
the **plaintext** serialized `ManifestRecord` (`spec/service/storage.md`
§4.2), not an encrypted blob. In the stored `ManifestRecord` the inner
`version` field is not set (0); identifiers, `sourceDevice` and
`recordIkm` (empty if none) are. signal-cli deletes this file whenever the
storage key changes (registration, linking, new master key or AEP) and
then sets key_value `storage-manifest-version` to −1 (D1 `SignalAccount`,
D2 `SignalStorageManifest`).

---

## 5. Envelope cache `<path>.d/msg-cache/`

Lifecycle, path scheme (`msg-cache/<recipient _id>/<nowMillis>_<envelope
timestamp>`, or directly in `msg-cache/` when the sender is unknown) and
the current file format (version 9) are in `spec/protocol/capture.md` §3.
Additional facts:

- `<recipient _id>` is the decimal `recipient._id` from `account.db`.
  When recipients are merged, files move into the surviving recipient's
  directory. Empty recipient directories are removed when encountered.
- The 8-byte number written after the version in version-9 files is the
  envelope's **server delivered** timestamp, but when read back it is
  used as the **server received** timestamp.
- Readers must skip files whose leading version is greater than 9.
- Files with version 1–8 use a field-by-field Java `DataOutputStream`
  layout (big-endian integers; strings as 2-byte big-endian length plus
  modified UTF-8). Field order:

| # | Field | Type | Present in versions |
|---|-------|------|---------------------|
| 1 | version | int32 | all |
| 2 | envelope type | int32 | all |
| 3 | source (E.164 or empty) | string | all |
| 4 | source service id | string | ≥ 3 |
| 5 | source device id | int32 | all |
| 6 | legacy relay (ignored) | string | = 1 only |
| 7 | destination service id | string | ≥ 5 |
| 8 | client timestamp | int64 | all |
| 9 | content length L, then L bytes (absent if L ≤ 0) | int32 + bytes | all |
| 10 | legacy message length M, then M bytes (ignored) | int32 + bytes | all |
| 11 | server received timestamp | int64 | ≥ 2 |
| 12 | server GUID (empty = none) | string | ≥ 2 |
| 13 | server delivered timestamp | int64 | ≥ 4 |
| 14 | urgent | boolean byte | ≥ 6 (default true) |
| 15 | story | boolean byte | ≥ 7 (default true) |
| 16 | updated PNI | string | ≥ 8 |

The cache holds envelopes that failed to decrypt with an untrusted
identity (or that were being processed when signal-cli was killed). They
are encrypted; importing them is only useful if the sessions and
identities are imported too.

---

## 6. Shared media directories

### 6.1 `<DATA>/attachments/`

Flat directory of downloaded attachments (D1 `AttachmentStore`).

- File name: the attachment's remote id (CDN key or numeric id as
  string) with every character outside `A-Z a-z 0-9 _ . -` replaced by
  `_`, followed by an extension: the part after the last `.` of the
  attachment's file name if it has one, otherwise an extension guessed
  from the content type, otherwise nothing (no dot).
- Previews (quote/link thumbnails) use the same name plus `.preview`.
- Content: the **decrypted** attachment bytes.
- Not referenced from `account.db`; the received-message JSON output
  carries the id.

### 6.2 `<DATA>/avatars/`

Flat directory (D1 `AvatarStore`). Content: decrypted image bytes.

| File name | Owner |
|-----------|-------|
| `contact-<id>` | contact avatar from a contacts sync |
| `profile-<id>` | profile avatar |
| `group-<groupIdBase64>` | group avatar |

`<id>` is the recipient's phone number (E.164 with `+`) if known, else
its ACI string, else `PNI:<uuid>`. `<groupIdBase64>` is the group id
(16 bytes for v1, 32 bytes for v2) in standard base64 with padding, with
every `/` replaced by `_` (D3: an all-zero v1 id gives
`group-AAAAAAAAAAAAAAAAAAAAAA==`).

### 6.3 `<DATA>/stickers/`

One subdirectory per downloaded sticker pack (D1 `StickerPackStore`):

- Directory name: the 16-byte pack id as **lower-case hex** without
  separators (32 characters).
- `manifest.json`: JSON object written with Jackson defaults (compact):
  `title` (string), `author` (string), `cover` (sticker object or null),
  `stickers` (array of sticker objects). A sticker object has `id`
  (number or null), `emoji` (string or null), `file` (string: the file
  name of the image inside the directory) and `contentType` (string or
  null). If `id` is null the id is the integer value of `file`.
- `<stickerId>`: one file per sticker, named by its decimal id, holding
  the decrypted image bytes.

The pack **key** is not stored here; it is in `account.db` table
`sticker` (`signal-cli-account-db.md` §11).

---

## 7. Locking: detecting a running signal-cli

| File | Lock | Held |
|------|------|------|
| `<DATA>/data/<path>` (account file) | exclusive lock over the whole file | from account load until the account is closed: for one-shot commands, the command's lifetime; for `daemon`/`jsonRpc`, the daemon's whole lifetime |
| `<DATA>/data/accounts.json` | exclusive lock over the whole file | only during each single read or update of the index |

Mechanism: Java `FileChannel.tryLock()` / `lock()` on a channel opened
read-write. On Linux this is a **POSIX advisory record lock** (`fcntl`
`F_SETLK`/`F_SETLKW`, type write lock) covering offset 0 to end of file
(D3: `/proc/locks` showed `POSIX ADVISORY WRITE <pid> … 0 EOF` for a
lock taken this way on the scratch account file). It is not a `flock(2)`
lock; on Linux the two kinds do not conflict with each other.

Behaviour of signal-cli itself:

- Normal commands try the account lock; if it is taken they log
  `Config file is in use by another instance, waiting…` (INFO), block
  until it is free, then log `Config file lock acquired.`.
- When probing accounts for the `accounts.json` version-1 upgrade it
  does not wait; a held lock makes that account count as unloadable.
- The SQLite database is additionally protected by SQLite's own locking
  (busy timeout 60 s, `IMMEDIATE` transactions), but signal-cli relies on
  the account-file lock to keep a single process per account.

So a running signal-cli for an account is detectable by testing for a
conflicting POSIX write lock on that account's file (for example with
`fcntl` `F_GETLK` or a non-blocking `F_SETLK` attempt). The `-wal` file
alone is not a reliable indicator.

---

## 8. Not covered

- Nested formats of the legacy JSON stores and legacy per-file stores
  (§3.4): `axolotlStore` and the other `*Store` objects, `recipients-store`,
  the per-file `pre-keys`, `sessions`, `identities`, `sender-keys`,
  `group-cache` formats. 0.14.7 converts them on load; this spec only
  names them.
- signal-cli's global configuration file (`config.json`): see
  `spec/surface/cli/index.md` §7. It is not inside the data directory.
