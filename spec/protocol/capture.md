# Offline interop testing against signal-cli 0.14.7: where the data lives

Spec role: prose only; no code. This file documents the on-disk formats a
signal-cli 0.14.7 instance uses, so that the main session can run two of
its **own** reference instances on Signal staging and later replay what one
of them received into the Go implementation, offline. It covers only data
that the operator's own accounts own. It proposes no binary patching, no
TLS interception and no access to anyone else's data.

## Sources

| Ref | What | URL @ version |
|-----|------|---------------|
| C1 | signal-cli | https://github.com/AsamK/signal-cli @ tag v0.14.7 = commit b01b6b370dc063599a1a2b9fde0f5ff4e2d78fe8: `lib/src/main/java/org/asamk/signal/manager/storage/{SignalAccount,AccountDatabase,Database,Utils}.java`; `.../storage/{sessions/SessionStore,prekeys/PreKeyStore,prekeys/SignedPreKeyStore,prekeys/KyberPreKeyStore,identities/IdentityKeyStore,identities/SignalIdentityKeyStore,senderKeys/SenderKeyRecordStore,senderKeys/SenderKeySharedStore,keyValue/KeyValueStore,recipients/RecipientStore}.java`; `.../storage/messageCache/{MessageCache,CachedMessage}.java`; `.../util/MessageCacheUtils.java`; `.../helper/{ReceiveHelper,IncomingMessageHandler}.java`; `.../internal/PathConfig.java`; `.../api/TrustLevel.java`; `src/main/java/org/asamk/signal/{App,commands/ReceiveCommand,commands/TrustCommand,logging/LogConfigurator}.java` |
| C2 | libsignal | https://github.com/signalapp/libsignal @ tag v0.99.1 = commit 97801d22dcf9f5bf714f7b8fa3212cdc973ae1c8: `rust/protocol/src/proto/storage.proto`; `rust/protocol/src/state/{session,kyber_prekey,signed_prekey,prekey}.rs`; `rust/protocol/src/{session,session_management}.rs`; `rust/protocol/src/consts.rs` |
| C3 | Reference distribution | `/work/third_party/signal-cli-0.14.7/`: launcher `bin/signal-cli`; `lib/signal-service-java-2.15.3_unofficial_151.jar` (the Wire-generated `…internal.push.Envelope` class; field numbers read from its `@WireField` annotations with `javap -v`); class-file string search across the signal-cli and signal-service jars |
| C4 | Local black-box check | The bundled `sqlite-jdbc-3.53.2.1.jar` opened a scratch SQLite file (WAL, STRICT table) from `jshell` with no network access, confirming the driver alone suffices to read this schema. No real account database existed in the sandbox, since creating one needs registration. |

See also `spec/crypto/kem.md` (Kyber key byte layouts) and
`spec/crypto/xeddsa.md` (Curve25519 key encodings).

## 1. Data-directory layout

The data root is `--data-dir` (alias `--config`), defaulting to
`$XDG_DATA_HOME/signal-cli`, else `$HOME/.local/share/signal-cli`
(C1 `App`, `PathConfig`, `SignalAccount`).

| Path under the data root | Content |
|--------------------------|---------|
| `data/accounts.json` | Index: `version` 2 and an `accounts` array of `{path, environment, number, uuid}`. `environment` is `LIVE` or `STAGING`. `path` is an opaque short string. |
| `data/<path>` | Account file: JSON, storage `version` 10, held under an advisory file lock while signal-cli runs. |
| `data/<path>.d/account.db` | SQLite, schema `user_version` 30, opened **WAL** with `foreign_keys=ON`; `account.db-wal` and `-shm` sit beside it. |
| `data/<path>.d/msg-cache/` | Received-envelope cache (section 3). |
| `data/<path>.d/storage-manifest` | Storage-service manifest; irrelevant here. |
| `attachments/`, `avatars/`, `stickers/` | Media. |

The account file carries, per identity (`aciAccountData` and
`pniAccountData`): `serviceId`, `registrationId`, `identityPublicKey` and
`identityPrivateKey` (both base64 of the libsignal serialized forms, so 33
and 32 bytes), `nextPreKeyId`, `nextSignedPreKeyId`,
`activeSignedPreKeyId`, `nextKyberPreKeyId`, `activeLastResortKyberPreKeyId`.
Top level additionally has `serviceEnvironment`, `registered`, `number`,
`deviceId`, `isMultiDevice`, `password`, profile and backup keys. The two
identity key pairs are **only** in this file, not in `account.db`.

## 2. `account.db` tables that matter for decryption

All are `STRICT` tables (C1). `account_id_type` distinguishes the local
identity: **0 = ACI, 1 = PNI** (C1 `storage/Utils.getAccountIdType`).

| Table | Columns relevant here | Notes |
|-------|----------------------|-------|
| `session` | `account_id_type`, `address` TEXT, `device_id` INTEGER, `record` BLOB; unique on the triple | `address` is the peer's service-ID UUID string (ACI or PNI form). `record` is a libsignal `SessionRecord`. |
| `pre_key` | `account_id_type`, `key_id`, `public_key` BLOB, `private_key` BLOB, `stale_timestamp` | One-time EC prekeys; raw serialized key bytes, not a record protobuf. |
| `signed_pre_key` | `account_id_type`, `key_id`, `public_key`, `private_key`, `signature`, `timestamp` | Same style; signature over the 33-byte public key. |
| `kyber_pre_key` | `account_id_type`, `key_id`, `serialized` BLOB, `is_last_resort` INTEGER, `stale_timestamp`, `timestamp` | `serialized` is a whole libsignal `KyberPreKeyRecord`. |
| `identity` | `address` TEXT UNIQUE, `identity_key` BLOB, `added_timestamp`, `trust_level` | Peers' identity keys. **Not** scoped by `account_id_type`. `trust_level` is the ordinal of UNTRUSTED=0, TRUSTED_UNVERIFIED=1, TRUSTED_VERIFIED=2. |
| `sender_key` | `address`, `device_id`, `distribution_id` BLOB (16-byte UUID), `record` BLOB, `created_timestamp` | Receiving sender-key states; `record` is a libsignal `SenderKeyRecord`. |
| `sender_key_shared` | `address`, `device_id`, `distribution_id`, `timestamp` | Bookkeeping for outgoing distribution. |
| `recipient` | `_id`, `aci`, `pni`, `number`, `profile_key`, … | Maps recipient ids (used in msg-cache directory names) to service IDs. |
| `key_value` | `key` TEXT, `value` ANY | Misc. account flags. |

### Serialized record formats (C2 `proto/storage.proto`)

- `SessionRecord` = `RecordStructure`: field 1 `current_session`
  (`SessionStructure`), field 2 repeated `bytes previous_sessions`, each
  itself an encoded `SessionStructure`, newest first, capped at 40.
- `SessionStructure`: 1 `session_version` uint32, 2 `local_identity_public`
  bytes, 3 `remote_identity_public` bytes, 4 `root_key` bytes,
  5 `previous_counter` uint32, 6 `sender_chain` Chain, 7 repeated
  `receiver_chains` Chain, 9 `pending_pre_key` PendingPreKey,
  10 `remote_registration_id` uint32, 11 `local_registration_id` uint32,
  13 `alice_base_key` bytes, 14 `pending_kyber_pre_key`
  PendingKyberPreKey, 15 `pq_ratchet_state` bytes (SPQR; field 12 is
  reserved).
  - `Chain`: 1 `sender_ratchet_key`, 2 `sender_ratchet_key_private`,
    3 `chain_key` {1 `index` uint32, 2 `key` bytes}, 4 repeated
    `message_keys` {1 `index`, 2 `cipher_key`, 3 `mac_key`, 4 `iv`,
    5 `seed`}.
  - `PendingPreKey`: 1 optional `pre_key_id` uint32, 2 `base_key` bytes,
    3 `signed_pre_key_id` int32, 4 `timestamp` uint64.
  - `PendingKyberPreKey`: 1 `pre_key_id` uint32, 2 `ciphertext` bytes.
- `PreKeyRecordStructure`: 1 `id` uint32, 2 `public_key`, 3 `private_key`.
- `SignedPreKeyRecordStructure`: 1 `id` uint32, 2 `public_key`,
  3 `private_key`, 4 `signature`, 5 `timestamp` fixed64 (ms).
  A `KyberPreKeyRecord` reuses this same message: `public_key` holds the
  1569-byte serialized Kyber public key and `private_key` the 3169-byte
  serialized secret key (see `kem.md`).
- `SenderKeyRecordStructure`: 1 repeated `sender_key_states`
  (`SenderKeyStateStructure`: 5 `message_version`, 1 `chain_id`,
  2 `sender_chain_key` {1 `iteration`, 2 `seed`}, 3 `sender_signing_key`
  {1 `public`, 2 `private`}, 4 repeated `sender_message_keys`
  {1 `iteration`, 2 `seed`}).

## 3. The envelope cache (`msg-cache`)

This is the only place signal-cli keeps **raw received envelopes**, and it
is the heart of any offline replay (C1 `ReceiveHelper`, `MessageCache`,
`CachedMessage`, `MessageCacheUtils`).

Lifecycle during `receive`:

1. A batch is read from the authenticated websocket. For each envelope,
   signal-cli writes it to disk **before** acknowledging it to the server.
2. File path: `msg-cache/<recipientId>/<nowMillis>_<envelopeTimestamp>`,
   where `<recipientId>` is the numeric `recipient._id` of the sender, or
   the `msg-cache` directory itself when the envelope carries no source
   service ID (which is the normal case for sealed sender).
3. The envelope is then decrypted and handled.
4. **On success the cache file is deleted**, and its parent directory too
   if it becomes empty.
5. **On an untrusted-identity failure the file is kept**, and moved into
   the sender's directory once the sender is known. On the next `receive`,
   signal-cli retries every cached file first; a retry that still fails
   with untrusted identity keeps the file until the envelope is older than
   14 days. Any other outcome deletes it.
6. If the process is killed between the write and the delete, the file
   survives and is retried on the next run.

File format: four bytes big-endian version (currently **9**), then eight
bytes big-endian `serverDeliveredTimestamp` (Java `DataOutputStream`
ordering), then the Wire-encoded `Envelope` protobuf, to end of file.
Versions below 9 use an older `DataOutputStream` field-by-field layout that
0.14.7 can still read but never writes.

`Envelope` field numbers (C3, read from the Wire annotations):
1 `type` enum, 5 `clientTimestamp` uint64, 7 `sourceDeviceId` uint32,
8 `content` bytes, 9 `serverGuid` string, 10 `serverTimestamp` uint64,
11 `sourceServiceId` string, 12 `ephemeral` bool, 13
`destinationServiceId` string, 14 `urgent` bool, 15 `updatedPni` string,
16 `story` bool, 17 `report_spam_token` bytes, 19
`sourceServiceIdBinary` bytes, 20 `destinationServiceIdBinary` bytes,
21 `serverGuidBinary` bytes, 22 `updatedPniBinary` bytes. The `type` enum
values are UNKNOWN 0, DOUBLE_RATCHET 1, PREKEY_MESSAGE 3,
SERVER_DELIVERY_RECEIPT 5, UNIDENTIFIED_SENDER 6, PLAINTEXT_CONTENT 8.
Field 8 `content` is exactly the ciphertext the protocol specs in this
directory describe.

## 4. Logging

`-v/--verbose` is cumulative: 1 raises the root logger to INFO and
`org.asamk` to DEBUG, 2 raises `org.asamk` to TRACE, 3 raises the root
logger and libsignal's bridge loggers to TRACE (C1 `LogConfigurator`).
`--log-file` writes the detailed layout to a file and filters the console.
`--scrub-log` masks numbers and UUIDs.

Even at the highest level **no envelope or ciphertext bytes are logged**.
A string search of all signal-cli, libsignal-cli and signal-service classes
found only descriptive envelope messages ("Retrieved {} envelopes!",
"Storing new message from {}", cache-version lines). libsignal logs a
decryption-failure summary that names session state, not ciphertext.
Logging is therefore useless as a capture channel.

## 5. Flags that affect what is left behind

- `receive` has `--timeout`, `--max-messages`, `--ignore-attachments`,
  `--ignore-stories`, `--ignore-avatars`, `--ignore-stickers`,
  `--send-read-receipts`. None of them stops decryption or preserves an
  envelope: a decryptable envelope is always decrypted and its cache file
  deleted.
- The global `--trust-new-identities {on-first-use,always,never}` is the
  one lever that reliably **leaves envelopes undecrypted**. With `never`,
  an unknown peer's identity is stored as UNTRUSTED, decryption of its
  messages ends in untrusted-identity, and by section 3 step 5 the cache
  file is kept and retried rather than deleted. Note that a retry pass sets
  an internal "retrying decryption" flag that suppresses identity-store
  writes, so repeated retries do not churn the `identity` table.
- `--disable-send-log` affects only resend support.
- There is no supported option to dump envelopes, and none is needed.

## 6. Recommended approach

The reliable method is a two-phase file capture around a single `receive`
run of the operator's own staging recipient instance, with no changes to
the binary and no network interception.

1. **Before** receiving, stop signal-cli and copy the whole
   `data/<path>` account file and `data/<path>.d/` directory. Copy
   `account.db` together with its `-wal` and `-shm` files, or checkpoint
   first; a WAL database copied without its sidecar files can be stale.
   This snapshot holds the pre-receive session state, the unconsumed
   one-time EC prekeys and the Kyber prekeys — exactly the state the
   envelopes about to arrive were encrypted against.
2. Run `receive` with `--trust-new-identities never` so that envelopes are
   not consumed, then stop the process and copy `msg-cache/` again. The
   retained files are the raw envelopes. (If full normal processing is
   wanted instead, run with default trust and take the cache copy from a
   run that is interrupted before it acknowledges handling; that is racier,
   so the untrusted-identity route is preferred.)
3. **After** receiving, snapshot the data directory again. Comparing the
   two snapshots shows which one-time prekey and Kyber prekey rows the
   reference consumed and how the session record advanced, which is itself
   a strong conformance oracle for the Go implementation.

Scope of the untrusted-identity route (parent agent's addition, from C1
`IdentityKeyStore.isTrustedIdentity`): with `never`, a peer identity seen
for the first time is stored as UNTRUSTED and every later trust check for
it fails, in both directions. Consequences:
- Only the **recipient** instance may run with `never`; the sender needs
  the default setting, or its own send fails as untrusted.
- It works only while the recipient does not yet trust the sender's
  identity (first contact, or after an identity change). Once the peer is
  trusted, envelopes decrypt and are deleted as usual.
- libsignal checks trust **before** touching any state for a
  PreKeySignalMessage (`spec/protocol/pqxdh.md` §7), so nothing is
  consumed: the pre- and post-receive snapshots hold the same prekeys and
  sessions.
- To capture a sequence (a PreKeySignalMessage followed by SignalMessages,
  group messages, and so on), let the sender send everything while the
  recipient is not receiving, then do the single `receive`. All of those
  envelopes stay cached. The capture is one-directional: the recipient
  never decrypts, so it cannot reply within that session.

A replay tool therefore needs to read, from the pre-receive snapshot only:

- the account file: both identity key pairs, both registration IDs, the
  ACI and PNI service IDs, the device ID;
- `pre_key`, `signed_pre_key`, `kyber_pre_key` rows for the right
  `account_id_type`, including `is_last_resort`;
- `session` rows (peer UUID, device id, `SessionRecord` blob);
- `sender_key` rows plus `recipient` for id-to-service-ID mapping;
- `identity` rows, if peer-identity checks are to be reproduced;

and from the post-receive snapshot the `msg-cache` files.

Two practical notes. First, decryption **mutates** state in a fixed order
(C2 `session_management`): the identity is saved, then the Kyber prekey is
marked used (a last-resort key is kept, a one-time Kyber key is deleted),
then the one-time EC prekey is removed, then the session record is stored.
Running the Go implementation against a post-receive snapshot will fail,
so the pre-receive snapshot is mandatory. Second, `account.db` needs no
sqlite3 CLI: the bundled `sqlite-jdbc-3.53.2.1.jar` on the JVM classpath
reads this schema directly (verified locally, C4), which lets the main
session export rows to JSON fixtures without adding a dependency. Reading a
copy, never the live file, also avoids the advisory lock and WAL races.

Values in these snapshots are live private keys for the operator's own test
accounts. They belong in a scratch directory, not in the repository;
only derived fixtures should be committed, and only for throwaway
staging accounts.
