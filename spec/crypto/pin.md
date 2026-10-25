# PIN hashing, account entropy pool and master key (signal-cli 0.14.7)

Spec role: prose only; no code. This file covers the key material behind
the registration-lock PIN: the account entropy pool (AEP), the master key,
the keys derived from the master key, PIN normalization, the Argon2id PIN
hash, and the encryption of the master key for storage in SVR2. The SVR2
protocol and the signal-cli command flows that use this material are in
`spec/service/svr.md`. The storage-service key derived here is used in
`spec/service/storage.md`. The AEP's transfer to linked devices is in
`spec/service/sync.md` (keys sync) and `spec/service/linking.md`
(provisioning).

## Sources

| Ref | What | URL @ version |
|-----|------|---------------|
| P1 | libsignal | https://github.com/signalapp/libsignal @ v0.99.1 (97801d22dcf9f5bf714f7b8fa3212cdc973ae1c8): `rust/account-keys/{Cargo.toml,src/lib.rs,src/hash.rs,src/backup.rs,src/error.rs}`, `rust/bridge/shared/src/account_keys.rs`, `rust/attest/src/{svr2.rs,constants.rs}`, `java/client/src/main/java/org/signal/libsignal/svr2/{Pin,PinHash}.java`, `java/client/src/main/java/org/signal/libsignal/messagebackup/AccountEntropyPool.java`, `java/client/src/test/java/org/signal/libsignal/svr2/PinTest.java`, `java/client/src/test/java/org/signal/libsignal/messagebackup/AccountEntropyPoolTest.java` |
| P2 | signal-service-java (Turasa fork) | https://github.com/Turasa/libsignal-service-java @ v2.15.3_unofficial_151 (66956cb4a0394d50cee3590209baf58312084a44): `core/models-jvm/src/main/java/org/signal/core/models/{AccountEntropyPool,MasterKey}.kt`, `core/models-jvm/src/test/java/org/signal/core/models/AccountEntropyPoolTest.kt`, `lib/libsignal-service/src/main/java/org/whispersystems/signalservice/api/kbs/{PinHashUtil.kt,PinString.java}`, `.../api/svr/SecureValueRecoveryV2.kt` |
| P3 | signal-cli | https://github.com/AsamK/signal-cli @ v0.14.7 (b01b6b370dc063599a1a2b9fde0f5ff4e2d78fe8): `lib/src/main/java/org/asamk/signal/manager/storage/SignalAccount.java`, `.../manager/helper/{AccountHelper,PinHelper,StorageHelper,SyncHelper}.java`, `.../manager/internal/RegistrationManagerImpl.java` |
| P4 | RFC 9106 | Argon2 Memory-Hard Function for Password Hashing and Proof-of-Work Applications, https://www.rfc-editor.org/rfc/rfc9106.txt |
| P5 | RFC 7693 | The BLAKE2 Cryptographic Hash and Message Authentication Code (public standard; BLAKE2b is the hash inside Argon2) |
| P6 | RFC 5869, RFC 2104, FIPS 180-4 | HKDF, HMAC, SHA-256 (public standards) |
| P7 | Reference distribution, offline | `jshell` over `/work/third_party/signal-cli-0.14.7/lib/*.jar` (libsignal-client 0.99.1 with its native library, signal-service-java 2.15.3_unofficial_151, models-jvm, bcprov-jdk18on 1.85), no network: produced and cross-checked the vectors in `testdata/vectors/pin.json` and `testdata/vectors/rfc9106_argon2.json` |

## 1. Overview

There are three layers:

1. The **account entropy pool**: a 64-character string, the root secret.
2. The **master key** (libsignal calls it the "SVR key"): 32 bytes. It
   is either derived from the AEP or, after a PIN-protected
   re-registration, restored from SVR2.
3. Keys **derived from the master key** with HMAC-SHA256 and fixed
   labels: the registration lock token, the registration recovery
   password, the storage-service key, and the logging key.

The PIN never touches the master key derivations. It only protects the
copy of the master key that is stored in SVR2 (§6, §7).

## 2. Account entropy pool

### 2.1 Format

- Exactly 64 characters, each from the 36-symbol alphabet
  `0123456789abcdefghijklmnopqrstuvwxyz` (ASCII digits and lowercase
  letters).
- libsignal's validity check (P1 `AccountEntropyPool` parsing) requires
  exactly 64 **bytes** and rejects any byte that is not an ASCII digit or
  lowercase ASCII letter. It rejects uppercase. The two error kinds are
  "wrong length" (with the byte length) and "invalid character" (with the
  first bad character).
- signal-cli's wrapper (P2 `AccountEntropyPool` model) lower-cases the
  string when it is constructed. Every AEP signal-cli stores or derives
  from is therefore lowercase. The wrapper also has display-form helpers
  (`O` shown as `#`, `0` shown as `=`) that signal-cli never uses.

### 2.2 Generation

Each of the 64 characters is chosen independently and uniformly from the
36-symbol alphabet using a cryptographically secure random source (P1
`AccountEntropyPool::generate`). Any unbiased method is acceptable, for
example rejection sampling of random bytes (accept a byte b < 252 and
take b mod 36). Do not take a raw byte mod 36: that is biased.

### 2.3 Master key from the AEP

    master_key = HKDF-SHA256(salt = none, IKM = the 64 ASCII bytes of the AEP,
                             info = "20240801_SIGNAL_SVR_MASTER_KEY", L = 32)

"Salt none" means the RFC 5869 default: 32 zero bytes. Since L = 32 this is
one HMAC for extract and one for expand:
`PRK = HMAC-SHA256(32 zero bytes, AEP)`,
`master_key = HMAC-SHA256(PRK, info || 0x01)`.

(For completeness only: libsignal derives the message-backup key from the
same AEP with info `20240801_SIGNAL_BACKUP_KEY`. signal-cli 0.14.7 never
uses it.)

## 3. Keys derived from the master key

Each derived key is `HMAC-SHA256(key = master_key, message = label)`, where
the label is the ASCII string below without a terminator (P1 `SvrKey`,
P2 `MasterKey`). All outputs are 32 bytes.

| Label | Name | Wire / storage form used by signal-cli |
|-------|------|----------------------------------------|
| `Registration Lock` | registration lock token | **lowercase hex**, 64 characters, no separators. Sent as `registrationLock` in account attributes, in `PUT /v1/accounts/registration_lock`, and in the registration retry after a 423 (`svr.md`). |
| `Registration Recovery` | registration recovery password | **standard base64 with padding**, 44 characters. Sent as `recoveryPassword` (account attributes and the top level of `POST /v1/registration`). |
| `Storage Service Encryption` | storage-service key | 32 raw bytes; see `storage.md`. |
| `Logging Key` | logging key | not used by signal-cli 0.14.7. |

Known answers: master key = 32 bytes of `0x2a` gives registration lock
`3a40e258…e10d1` (full values in `pin.json` → `master_key_derivations`).

## 4. Local storage and lifecycle in signal-cli

### 4.1 Stored fields

The account file (JSON, P3 `SignalAccount`) has these members related to
this spec. All are nullable.

| Member | Content |
|--------|---------|
| `registrationLockPin` | the PIN exactly as the user typed it (**not** normalized) |
| `pinMasterKey` | a master key restored from SVR2, standard base64 with padding of the 32 bytes |
| `accountEntropyPool` | the 64-character lowercase AEP |
| `storageKey` | a storage key received from a primary (linked devices only; `storage.md`) |
| `mediaRootBackupKey` | 32 bytes, standard base64 (not related to the PIN; `sync.md`) |

### 4.2 Which master key is current

- **Current master key**: `pinMasterKey` if set; otherwise the
  AEP-derived key (§2.3) if an AEP is stored; otherwise none.
- **Get-or-create master key**: the current master key; if there is none,
  generate an AEP (§2.2), store it and save the account file at once, and
  derive from it.
- **PIN-backed master key**: none if `registrationLockPin` is null.
  Otherwise, on a linked device, the current master key (possibly none).
  Otherwise (primary), the get-or-create master key.

Every upload of account attributes (`registration.md` §4.2, and the
`PUT /v1/accounts/attributes` of every account check, `api.md` §4.3) fills
two members from the PIN-backed master key:

- `registrationLock` = its registration lock token, or null if there is
  no PIN-backed master key;
- `recoveryPassword` = its recovery password, or null likewise.

So with no PIN set, both are null, and the server clears any registration
lock it had (`svr.md` §2.3).

### 4.3 When keys are created, replaced or dropped

| Event | Effect |
|-------|--------|
| New account file (`register` for an unknown number) | no AEP, no `pinMasterKey`, no PIN |
| `setPin` | get-or-create master key (may create the AEP), then the SVR2 backup with that key; on success the PIN is stored (`svr.md` §4.1) |
| `removePin` | only `registrationLockPin` becomes null; AEP and `pinMasterKey` are kept |
| `addDevice` (linking a new device) | get-or-create master key (may create the AEP); the AEP is sent in the provisioning message (`linking.md`) |
| Sending a keys sync message | the AEP is created if absent (`sync.md`) |
| Successful registration or re-registration (`verify`, or `register` via recovery password) | `pinMasterKey` := the master key passed in (the SVR2-restored key, the PIN-backed key for a recovery-password re-registration, or none), **`accountEntropyPool` := none**, `registrationLockPin` := the PIN passed in (none unless a master key was passed in) |
| Storage sync on a **primary** while `pinMasterKey` or `storageKey` is set ("storage key migration") | the AEP is fetched or created, then stored as the AEP, and `pinMasterKey` and `storageKey` are cleared. A keys sync message is sent and the storage manifest is force-pushed under the new storage key (`storage.md`, `sync.md`) |
| Keys sync message received with an AEP (any device) | AEP stored; `pinMasterKey` and `storageKey` cleared (`sync.md`) |
| Restored master key on a linked device (`setMasterKey`) | ignored on a primary. On a linked device it sets `pinMasterKey` and clears `storageKey` |

Consequence, reproduced as is: after a PIN-protected re-registration
restores master key M from SVR2, the next storage sync on the primary
replaces M by a key derived from a freshly generated AEP. From then on,
account-attribute uploads carry the registration lock and recovery
password of the new key, while SVR2 still holds M. Nothing re-uploads the
PIN to SVR2 automatically (see Gap 2).

## 5. PIN normalization

signal-cli passes the user's PIN string (from `setPin PIN` or
`verify --pin PIN`) to signal-service-java, which normalizes it before
hashing (P2 `PinHashUtil.normalize`). The steps, in order:

1. **Trim.** Remove leading and trailing characters that are whitespace
   in Kotlin's sense, over UTF-16 code units. A unit is whitespace if it
   is U+0009..U+000D, U+001C..U+001F, or in Unicode general category Zs,
   Zl or Zp. This includes U+0020, U+00A0 (no-break space), U+2003 and
   U+3000. It does not include U+0000, U+0085, U+200B or U+FEFF.
2. **Digit mapping.** If **every** UTF-16 code unit of the trimmed string
   is a decimal digit (general category Nd, judged per code unit, so
   characters outside the BMP never count as digits), replace each one by
   the ASCII digit with the same numeric value. Examples: Arabic-Indic
   `١٢٣٤`, Extended Arabic-Indic `۱۲۳۴`, fullwidth `１２３４`, Devanagari
   `१२३४` all become `1234`. The empty string counts as all-digit and
   stays empty. If any unit is not a digit, nothing is mapped in this
   step.
3. **NFKD.** Apply Unicode Normalization Form KD (compatibility
   decomposition followed by canonical ordering), as in Java 25, which
   implements Unicode 16.0.
4. **UTF-8.** Encode the result as UTF-8. These bytes are the Argon2
   password.

Notes:

- Inner whitespace is kept: `12 34` stays `12 34`.
- Step 3 still turns many non-ASCII digits into ASCII when step 2 did not
  apply: `１２a４` becomes `12a4`, `①②` becomes `12`, and the
  mathematical bold digits U+1D7CF U+1D7D0 become `12`.
- Letters are case-sensitive and are not folded. `Café` (precomposed é)
  becomes `Cafe` followed by U+0301.
- There is no length or character-class validation. The empty PIN is
  accepted and hashed as zero bytes.
- The **stored** `registrationLockPin` is the raw string; normalization
  happens each time the PIN is hashed.
- Go's standard library has neither NFKD nor Unicode 16.0 tables. An
  implementation needs its own decomposition data (UnicodeData.txt
  decomposition mappings, canonical combining classes, Hangul syllable
  arithmetic). For an ASCII-only PIN, steps 2 and 3 are the identity.

29 worked examples are in `pin.json` → `normalization`.

## 6. PIN hash (Argon2id)

### 6.1 Parameters

| Parameter | Value |
|-----------|-------|
| Variant | Argon2id (type 2) |
| Version | 0x13 (19) |
| Memory | 16384 KiB (16 MiB, m = 16384 blocks of 1 KiB) |
| Iterations (passes) | t = 32 |
| Parallelism (lanes) | p = 1 |
| Tag length | 64 bytes |
| Secret K | empty |
| Associated data X | empty |
| Password | the normalized PIN bytes (§5) |
| Salt | 32 bytes (§6.2) |

The 64-byte tag is split: bytes 0..31 are the **encryption key**, bytes
32..63 are the **access key**.

libsignal uses the RustCrypto `argon2` crate with exactly these
parameters (P1 `hash.rs`). There is nothing beyond RFC 9106: the same
tags come out of BouncyCastle's independent Argon2 implementation for
every vector checked (P7). So an implementation follows RFC 9106 §3 to the
letter:

- H0 is BLAKE2b-512 over the little-endian 32-bit fields p, T, m, t, v,
  y, then length-prefixed P, S, K, X, as in RFC 9106 §3.2.
- H' is the variable-length hash of RFC 9106 §3.3, used for the first two
  blocks of each lane and for the final 64-byte tag. Since the tag is
  64 bytes, H' for the tag is a single BLAKE2b-512 of
  LE32(64) || final block.
- The compression function G uses the BLAKE2b round function with the
  added 2·trunc(a)·trunc(b) multiplication (RFC 9106 §3.5, §3.6).
- Argon2id indexing: data-independent addressing for the first two
  slices of pass 0, data-dependent otherwise (RFC 9106 §3.4.1.3). With
  p = 1 there are 4 slices of 4096 blocks each, and the reference set is
  always the single lane.

BLAKE2b (RFC 7693, unkeyed, with digest lengths up to 64 bytes) must be
implemented as well; Go's standard library has neither BLAKE2b nor
Argon2. Test data: `rfc9106_argon2.json` has the RFC 9106 §5 vectors for
Argon2d, Argon2i and Argon2id (with H0 and the first and last block of
each pass), BLAKE2b digests, and 64-byte Argon2id tags with the PIN
parameters.

Cost: one hash fills 16 MiB and makes 32 passes. signal-cli computes it
once per enclave per operation (§6.3), so `setPin` and a locked `verify`
each compute up to two hashes with different salts.

### 6.2 Salt for SVR2

    salt = HKDF-SHA256(salt = BE64(group_id), IKM = UTF-8(username),
                       info = empty, L = 32)

- `username` is the SVR2 Basic-auth username in use for that request:
  from `GET /v2/svr/auth`, or from `svr2Credentials` in a 423 body
  (`svr.md` §3.1). It is 32 lowercase hex characters, hashed as text.
- `group_id` is the 64-bit Raft group ID of the enclave being talked to,
  written as 8 bytes big-endian. libsignal looks it up from a built-in
  table keyed by the enclave's MRENCLAVE (P1 `constants.rs`). For the
  enclaves signal-cli uses:

| Environment | MRENCLAVE (hex) | group_id (unsigned decimal) |
|-------------|-----------------|-----------------------------|
| staging, current | `3c699f4975aaa3d172c0aad042f94f031b2b03e10b9c19a45116a01693d83302` | 5138641357881452604 |
| staging, legacy | `97f151f6ed078edbbfd72fa9cae694dcc08353f1f5e8d9ccd79a971b10ffc535` | 2330628069874851020 |
| live, current | `ced8217b26228e4b210c985786999d095c4958a94faf37b14acaf25c4cbb02a4` | 11311619198250676136 |
| live, legacy | `1240acbd4aa26974184844c8a46b1022d3957ac8a76c1fd8f5b1a15141ee0708` | 2076725645304009823 |

  An MRENCLAVE not in the table is an error ("Error looking up
  mrenclave"); this cannot happen with these constants.

So the same PIN gives different keys per enclave and per account (the
username is derived by the server from the ACI, `svr.md` §3.1).

Published check: username `username` with group 2330628069874851020
(the staging legacy enclave) gives salt `70dd66bd…7a5f83`, and with group
3862621253427332054 gives `d6159ba3…3f8a5e` (`pin.json` →
`published_salt`).

### 6.3 Which hash is used where

| Operation | Hash input | What is sent to SVR2 |
|-----------|-----------|----------------------|
| backup (`setPin`) | normalized PIN, salt for (username, enclave) | access key as `pin`; encrypted master key as `data` |
| expose (`setPin`) | same | encrypted master key as `data` |
| restore (`verify --pin`) | same, with the username from the 423 body | access key as `pin`; the returned `data` is decrypted with the encryption key |

## 7. Encrypting the master key: HMAC-SHA256-SIV

The 32-byte master key m is encrypted under the 32-byte encryption key k
(§6.1) into a 48-byte blob `IV || C` (P1 `hash.rs`). This is a
deterministic, fixed-length construction built from HMAC-SHA256. It is
not RFC 5297 SIV.

Encrypt:

1. `Ka = HMAC-SHA256(k, "auth")` and `Ke = HMAC-SHA256(k, "enc")`, labels
   in ASCII.
2. `IV = first 16 bytes of HMAC-SHA256(Ka, m)`.
3. `Kx = HMAC-SHA256(Ke, IV)` (32 bytes).
4. `C = Kx XOR m` (32 bytes).
5. Output `IV || C` (48 bytes).

Decrypt a 48-byte blob:

1. Split into IV (16 bytes) and C (32 bytes). Derive Ka and Ke as above.
2. `m = HMAC-SHA256(Ke, IV) XOR C`.
3. Recompute the first 16 bytes of `HMAC-SHA256(Ka, m)`. Compare them with
   IV in constant time. On a mismatch the blob is invalid. signal-service-java
   reports this as an invalid-ciphertext error, which signal-cli treats as
   an application error for that enclave (`svr.md` §5.3).

The same key and master key always give the same blob. signal-cli relies
on this: the expose request recomputes the blob and the enclave compares
it with the stored one (`svr.md` §3.3).

Known answer: key `000102…1f`, m `202122…3f` → `f2703691…f5c5a342`
(`pin.json` → `hmac_sha256_siv`). Flipping byte 0 or byte 47 makes
decryption fail.

## 8. Things libsignal offers that signal-cli 0.14.7 does not use

- The local PIN hash (Argon2i, m = 512 KiB, t = 64, p = 1, 32-byte tag,
  16-byte random salt, PHC string such as
  `$argon2i$v=19$m=512,t=64,p=1$…`). signal-cli never stores or checks a
  local PIN hash.
- The "SVR1" PIN hash with a caller-supplied salt (same Argon2id
  parameters as §6.1). It appears here only because the published vectors
  use it.
- The logging key and the message-backup key family.

## 9. Test vectors

- `testdata/vectors/pin.json`:
  - published libsignal PIN-hash vectors (pin, salt → encryption key,
    access key, encrypted master key) and published salts;
  - HMAC-SHA256-SIV known answers and two tamper cases;
  - 40 SVR2 PIN hashes: 4 enclaves × 2 usernames × 5 PIN inputs, with
    salt, keys and the encrypted master key;
  - normalization cases;
  - AEP → master key → derived keys, AEP validity cases;
  - master key → derived keys, including libsignal's published 0x2a
    known answers.
- `testdata/vectors/rfc9106_argon2.json`: RFC 9106 §5 vectors, BLAKE2b
  digests, and Argon2id with the PIN parameters.

## Gaps

1. **Unicode version drift.** The reference runs on Java 25 (Unicode
   16.0) for NFKD, the Nd category and the whitespace categories. A
   different Unicode version only matters for exotic PIN characters. The
   vectors cover common cases only.
2. **Master key divergence after re-registration** (§4.3). This is
   upstream behaviour: after a storage-key migration, the registration
   lock sent in account attributes no longer matches the master key held
   in SVR2. A later PIN-protected re-registration elsewhere would restore
   the old key, derive a token the server rejects, and hit the "reg lock
   still failed" assertion (`svr.md` §5.2). Whether to reproduce this
   faithfully is a product decision. A drop-in replacement reproduces it.
3. Argon2 performance in pure Go (16 MiB, 32 passes, up to twice per
   command) was not measured. The reference computes it in native Rust.
