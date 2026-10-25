# Profile cipher: encrypting and decrypting profile fields and avatars (signal-cli 0.14.7)

Spec role: prose only; no code. This file defines the byte-level encryption
of every encrypted member of a Signal profile, as signal-cli 0.14.7 writes
and reads it:

- the name (given and family name), about text, about emoji;
- the phone-number-sharing flag;
- the payment (MobileCoin) address;
- the avatar image;
- the unidentified-access verifier that is checked against the profile key.

How these values travel (the `PUT /v1/profile` body, the profile fetch, the
avatar upload form and CDN) is in `spec/service/profiles.md`. The
profile-key version and commitment are in `spec/crypto/zkgroup-profile.md`.

## Sources

| Ref | What | URL @ version |
|-----|------|---------------|
| C1 | signal-service-java (Turasa fork), bundled with signal-cli 0.14.7 | https://raw.githubusercontent.com/Turasa/libsignal-service-java/66956cb4a0394d50cee3590209baf58312084a44/ (tag v2.15.3_unofficial_151): `lib/libsignal-service/src/main/java/org/whispersystems/signalservice/api/crypto/{ProfileCipher,ProfileCipherInputStream,ProfileCipherOutputStream,UnidentifiedAccess}.java`, `.../api/profiles/ProfileApi.kt`, `.../internal/push/http/ProfileCipherOutputStreamFactory.java`, `lib/libsignal-service/src/main/protowire/SignalService.proto` (message `PaymentAddress`) |
| C2 | signal-cli | https://raw.githubusercontent.com/AsamK/signal-cli/b01b6b370dc063599a1a2b9fde0f5ff4e2d78fe8/ (tag v0.14.7): `lib/src/main/java/org/asamk/signal/manager/util/{ProfileUtils,PaymentUtils}.java`, `.../manager/api/Profile.java`, `.../manager/helper/ProfileHelper.java` |
| C3 | Signal-Server (server-side size checks only) | https://raw.githubusercontent.com/signalapp/Signal-Server/8edf3488f7d97398db7ffa5184c6b769a272ff59/ (tag v20261022.0.0): `service/src/main/java/org/whispersystems/textsecuregcm/entities/CreateProfileRequest.java`, `.../util/ProfileHelper.java` |
| C4 | Reference distribution, offline | jshell (Temurin 25.0.5) over `/work/third_party/signal-cli-0.14.7/lib/*`: every vector in `testdata/vectors/profile_cipher.json` was checked against the reference decoders (§10) |
| C5 | Standards | AES: FIPS 197; GCM: NIST SP 800-38D; base64: RFC 4648 §4; UTF-8: RFC 3629 |

## 1. Key and primitive

- The key is the **32-byte profile key** itself (its zkgroup serialization,
  which is the raw 32 bytes). No key derivation is applied.
- The cipher is **AES-256-GCM** with a **12-byte nonce**, a **128-bit tag**
  and **no associated data**.
- Every encrypted value has the layout

  `nonce (12 bytes) ‖ GCM ciphertext (same length as the plaintext) ‖ tag (16 bytes)`

  so it is 28 bytes longer than the (padded) plaintext.
- The nonce is 12 fresh bytes from a CSPRNG for every field and every
  upload. Nonces are never reused on purpose, and nothing is derived from
  them.

## 2. Fixed-size string fields: name, about, about emoji

### 2.1 Plaintext

The field value is a string, encoded as UTF-8. It is right-padded with
zero bytes (0x00) to a **target length** chosen from the UTF-8 byte length
`n` of the value:

| Field | Target length |
|-------|---------------|
| name | 53 if n ≤ 53; otherwise 257 |
| about | 128 if n ≤ 128; 254 if 128 < n < 254; otherwise 512 |
| about emoji | always 32 |

Notes:

- The about boundary is asymmetric: n = 254 already goes to 512.
- If n exceeds the largest target (name > 257, about > 512, emoji > 32), the
  encryption step fails with an "Input is too long" error. signal-cli does
  not check lengths before that; the failure surfaces from `updateProfile`
  as an uncaught error (see `spec/service/profiles.md` §4.4).
- An empty value is encrypted too: 53, 128 or 32 zero bytes.
- The resulting ciphertext lengths are therefore name 81 or 285, about 156,
  282 or 540, emoji 60. The server accepts exactly these lengths (name: 81
  or 285; about: 0, 156, 282 or 540; emoji: 0 or 60) and rejects others
  (C3).

### 2.2 The name value

The name field carries both name parts in one string:

- given name only (no family name): the given name;
- otherwise: given name, one 0x00 byte, family name. A missing given name
  counts as empty, so a family-only name is `0x00 ‖ family`.

When writing its own profile, signal-cli always has a value here (at least
the empty string).

### 2.3 Decrypting a string field

1. Base64-decode the member (standard alphabet). If it is **not valid
   base64**, the field is treated as absent (null) and decoding of the other
   fields continues.
2. If the decoded value is shorter than **29 bytes** (12 + 16 + 1), it is a
   decryption failure ("Too short").
3. Split off the 12-byte nonce, decrypt and verify with AES-256-GCM. A tag
   mismatch is a decryption failure.
4. Strip **all trailing 0x00 bytes** (scan from the end to the last non-zero
   byte). Zero bytes before the last non-zero byte are kept.
5. Decode the rest as UTF-8. Malformed sequences are **replaced** with
   U+FFFD; they are not an error.

Consequences of step 4: a value that itself ends in 0x00 cannot be
distinguished from padding. A name written as given "Bob" with an empty
family name (`Bob 0x00`) reads back as just "Bob".

### 2.4 Splitting the name

After decryption the name string is split on every 0x00 character, with
**trailing empty parts discarded** (the split rule of Java's `String.split`):

| Decrypted name | Given name | Family name |
|----------------|-----------|-------------|
| (member absent or not base64) | null | null |
| `""` | `""` | null |
| `Alice` | `Alice` | null |
| `Alice␀Smith` | `Alice` | `Smith` |
| `␀Smith` | `""` | `Smith` |
| `a␀b␀c` | `a` | `b` (the rest is dropped) |
| `A␀␀B` | `A` | `""` |

(␀ = the 0x00 character.) Note that an empty name yields an empty given
name, not null.

## 3. The boolean field: phone-number sharing

- Plaintext: **one byte**, 0x01 for true and 0x00 for false. No padding;
  the target length is 1. Ciphertext: 29 bytes.
- Decryption: same as §2.3 steps 1 to 3, then the value is **true if the
  first plaintext byte is non-zero**. Other bytes, if any, are ignored.
- An absent member gives "unknown" (null). Invalid base64 also gives null.
- signal-cli maps true to sharing mode `EVERYBODY` and false to `NOBODY`.

## 4. The payment address

### 4.1 Inner message

The payment address is a protobuf message (field numbers are wire facts):

`PaymentAddress`:

| # | Name | Type |
|---|------|------|
| 1 | mobileCoin | message `MobileCoin` (the only member of a oneof) |

`PaymentAddress.MobileCoin`:

| # | Name | Type |
|---|------|------|
| 1 | publicAddress | bytes |
| 2 | signature | bytes |

- `publicAddress` is the MobileCoin public address bytes as given by the
  user (signal-cli treats them as opaque; `updateProfile
  --mobile-coin-address` takes them base64-encoded).
- `signature` is a 64-byte **XEdDSA** signature over exactly the
  `publicAddress` bytes, made with the account's **ACI identity private key**
  (`spec/crypto/xeddsa.md`; the signature uses a random nonce, so it differs
  on every write).

### 4.2 Encryption

1. Serialize the `PaymentAddress` message.
2. Prefix it with its length L as a **4-byte little-endian** signed integer.
3. Right-pad with zero bytes to **554 bytes**. (554 = 776 · 6 / 8 − 28: the
   field was sized so that its base64 form is 776 characters.) If 4 + L > 554
   the encryption fails.
4. Encrypt as in §1. The result is always **582 bytes**; the server accepts
   only 0 or 582 (C3).

### 4.3 Decryption and verification

1. If the member is absent, or the profile's identity key cannot be parsed,
   the address is null; nothing else is attempted. Unlike the string fields,
   the member is decoded as base64 by the JSON layer, so invalid base64 here
   makes the whole response unparsable.
2. Decrypt as in §2.3 steps 2 and 3. A value shorter than 29 bytes or a tag
   mismatch is a **decryption failure of the whole profile** (§6).
3. Read L as a 4-byte little-endian signed integer. If L is negative or
   greater than (plaintext length − 4), the address is null (only this field
   is dropped).
4. Parse the L bytes after the prefix as `PaymentAddress`. A parse error
   makes the address null.
5. If `mobileCoin`, `publicAddress` or `signature` is missing, the address is
   null. If the signature is not 64 bytes, or does not verify (XEdDSA) under
   the **identity key from the same profile response**, the address is null.
6. Otherwise the result is the `publicAddress` bytes. Only these are stored
   and shown (base64 in `listContacts` JSON).

## 5. The avatar

### 5.1 Encryption (upload)

The avatar image bytes, exactly as read from the file or data URI, are
encrypted as one stream:

`nonce (12, random) ‖ AES-256-GCM(profile key, nonce, image bytes) ‖ tag (16)`

- **No padding** is added, unlike attachments.
- No associated data.
- The ciphertext length, which is the upload length, is image length + 28.
- An empty image would give a 28-byte ciphertext. The server's upload policy
  requires 1 byte to 10 MiB (10 485 760 bytes) of ciphertext
  (`spec/service/profiles.md` §4.4).

### 5.2 Decryption (download)

1. Read the first 12 bytes as the nonce.
2. Treat the **last 16 bytes** of the download as the tag, and everything
   between nonce and tag as ciphertext.
3. Decrypt.

signal-cli **decrypts and writes the plaintext as it streams, and verifies
the tag only at end of stream**. On a tag mismatch, the plaintext already
written stays on disk (the avatar file then holds unverified bytes). The
error ("authentication of decrypted data failed") is logged as a warning and
otherwise ignored (`spec/service/profiles.md` §6.3). A compatible client
should verify before writing. It may keep or delete a partial file; the
reference keeps it.

## 6. Decoding a whole profile response

When a profile response is decoded with the profile key, the encrypted
members are processed in this order: name, about, about emoji, phone-number
sharing, payment address. Then:

- If **any** of name, about, about emoji, phone-number sharing or payment
  address fails **decryption** (too short, or a tag mismatch), the whole
  decode fails. No field from this response is used, as if the profile key
  were unknown (`spec/service/profiles.md` §5.4).
- Invalid base64 in name, about, about emoji or phone-number sharing nulls
  only that field.
- The payment address has the softer failures of §4.3 steps 3 to 5.
- `avatar` is a plain string (a CDN path such as `profiles/…`), not
  encrypted. It is taken as is.
- If the server does not hold the requested profile version, all encrypted
  members are absent. The decode then succeeds with every field null
  (`spec/service/profiles.md` §5.3).

## 7. Unidentified-access verifier

The profile response member `unidentifiedAccess` (base64) is compared with

`HMAC-SHA256(key = UAK, message = 32 zero bytes)` (32 bytes)

where UAK is the 16-byte unidentified-access key derived from the profile
key (`spec/service/api.md` §5.1). The comparison is in constant time.

- An absent or empty verifier never matches.
- Only a match yields access mode `ENABLED`. The full mode rules
  (`UNRESTRICTED`, `DISABLED`, `UNKNOWN`) are in
  `spec/service/profiles.md` §5.4.

## 8. Display name

This is not part of the cipher, but it is used wherever a profile name is
printed (`listContacts` "Profile name", sender names, `--name` matching):

- Given and family name both empty or null: `""`.
- Only one non-empty: that one.
- Both non-empty: given, one space (U+0020), family.

"Empty" means null or the empty string.

## 9. What a compatible implementation must match

- Byte-exact: padding targets, zero padding, nonce ‖ ciphertext ‖ tag layout,
  the 4-byte little-endian length prefix and 554-byte content size of the
  payment address, the 1-byte boolean, and unpadded avatar encryption.
  Peers (official apps, signal-cli) and the server's size checks depend on
  these.
- Decode leniency: §2.3 to §2.4, §4.3 and §6. Matching them makes stored and
  printed values identical to the reference for malformed peer profiles.
  Stricter behaviour is interoperable but changes output.

## 10. Test vectors

`testdata/vectors/profile_cipher.json` (C4) contains:

- `padding_targets`: name and about targets at and around every boundary.
- `string_fields`: name, about and emoji encryptions with fixed nonces
  (including a non-ASCII name with family part, the 53/54-byte name
  boundary, and 129- and 254-byte about texts).
- `reference_encrypt_string`: outputs of the reference encryptor
  (random nonce), re-derivable from key, nonce and plaintext.
- `decrypt_string_quirks`: trailing-NUL stripping, U+FFFD replacement,
  inner NULs kept, too-short input, wrong key.
- `boolean_fields`: true, false, and a 0x02 byte read as true.
- `payment_address`: identity key, inner proto, length-prefixed plaintext
  and 582-byte ciphertext.
- `unidentified_access`: UAK and verifier for key k1.
- `avatar`: three stream encryptions (100 bytes, a 1×1 PNG, empty), and the
  tampered-tag behaviour (all 100 plaintext bytes emitted before the error).
- `decode_profile`: 18 complete profile response JSONs with the field values
  the reference decoder produces. These cover the split table, tamper and
  short-input failures, invalid base64, missing version, access modes,
  capability mapping, phone-number sharing false, and payment address
  rejection cases.
- `put_profile_body`: the reference JSON serialization of a profile write
  body with fixed ciphertexts.

No upstream repository publishes vectors for these primitives. All entries
are reference-derived (see the file's `source.provenance`).

## Gaps

1. The behaviour for over-long input (name > 257, about > 512, emoji > 32
   UTF-8 bytes) is source-derived. The exact stderr text of the resulting
   uncaught error was not observed (it needs a registered account).
2. Java's `new String(bytes)` uses the JVM default charset. It is UTF-8 on
   Java 18 and later (signal-cli 0.14.7 requires Java 25), so §2.3 step 5
   assumes UTF-8. A JVM started with a different `file.encoding` would
   decode differently. This is not considered relevant.
