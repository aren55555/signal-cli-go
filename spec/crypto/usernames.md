# Usernames: syntax, hash, zero-knowledge proof, links (libsignal v0.99.1)

Spec role: prose only; no code. This file covers everything a client needs
to compute locally for Signal usernames:

- the syntax of a username (nickname + discriminator) and every validation
  error, with the exact text signal-cli surfaces;
- the 32-byte **username hash** (a ristretto255 point) and the three
  scalars it is built from;
- the **zero-knowledge proof** sent when confirming a username, as a
  poksho proof over that hash;
- **candidate generation** (how a bare nickname becomes 20 candidate
  usernames);
- **username links**: entropy, key derivation, the encrypted blob, and the
  `https://signal.me/#eu/…` URL.

The service side (which endpoint carries which of these values, status
codes, signal-cli behaviour) is in `spec/service/usernames.md`.

Companion specs, not repeated here:

- `spec/crypto/zkgroup-profile.md` (**ZP**): the Sho construction
  (HMAC-SHA256 variant), ristretto255 notation, the "point" and "scalar"
  squeeze mappings.
- `spec/crypto/zkgroup-credentials.md` (**ZC**): poksho — statements,
  statement description `D`, point list `A`, proving, proof encoding and
  verification (ZC §3). Usernames use that machinery unchanged; this file
  only gives the statement, the message and the inputs.

Vectors: `testdata/vectors/usernames.json`.

## Sources

| Ref | What | URL @ version |
|-----|------|---------------|
| U1 | libsignal usernames crate | https://github.com/signalapp/libsignal @ tag v0.99.1 = commit 97801d22dcf9f5bf714f7b8fa3212cdc973ae1c8: `rust/usernames/src/{lib, username, constants, error, username_links, proto}.rs`, `rust/usernames/src/proto/username.proto`, `rust/usernames/examples/{username_hash, username_links}.rs`, and the `#[cfg(test)]` modules in `username.rs`, `constants.rs`, `username_links.rs` |
| U2 | libsignal bridge and Java API (same tree) | `rust/bridge/shared/src/usernames.rs`, `rust/bridge/shared/src/net/chat.rs`, `rust/bridge/shared/types/src/jni/mod.rs` (exception mapping), `java/shared/java/org/signal/libsignal/usernames/*.java` |
| U3 | libsignal tests with fixed values (same tree) | `java/client/src/test/java/org/signal/libsignal/usernames/UsernamesTest.java`, `node/ts/test/UsernamesTest.ts`, `swift/Tests/LibSignalClientTests/UsernameTests.swift`, `rust/net/chat/src/ws/usernames.rs` (test constants) |
| U4 | poksho (same tree) | `rust/poksho/src/{statement, proof, args}.rs` — behaviour described in ZC §3 |
| C1 | signal-cli | https://github.com/AsamK/signal-cli @ v0.14.7 = commit b01b6b370dc063599a1a2b9fde0f5ff4e2d78fe8: `lib/src/main/java/org/asamk/signal/manager/api/UsernameLinkUrl.java`, `helper/AccountHelper.java` |
| J1 | Reference distribution, offline | `java` over `/work/third_party/signal-cli-0.14.7/lib/*.jar` (libsignal-client 0.99.1 with its native library, signal-cli 0.14.7 classes), no network; produced every `reference_jar` vector and the URL-parsing cases |
| S1 | ristretto255 | RFC 9496 |
| S2 | HMAC-SHA256, SHA-256, SHA-512 | RFC 2104, FIPS 180-4 |
| S3 | HKDF | RFC 5869 |
| S4 | AES-256-CBC, PKCS#7 padding | NIST SP 800-38A, RFC 5652 §6.3 |
| S5 | protobuf wire format | https://protobuf.dev/programming-guides/encoding/ |

## 1. Notation

ZP §1–§2 apply. In addition:

- `ℓ` is the ristretto255 group order
  `2^252 + 27742317777372353535851937790883648493`.
- A **scalar** is an integer mod `ℓ`, encoded as 32 bytes little-endian.
- `enc(P)` is the 32-byte ristretto255 encoding of point `P`.
- `be64(n)` is the 8-byte big-endian encoding of `n`.
- "base64url" means RFC 4648 §5 (`-` and `_`), **without** `=` padding,
  unless a section says otherwise.

## 2. Username syntax

A username is one string: `<nickname>.<discriminator>`.

### 2.1 Splitting

Split at the **last** `.` in the string (`rsplit_once`). Everything before
it is the nickname, everything after it the discriminator. If there is no
`.` at all, the username is invalid (`MissingSeparator`) — note that this
is also the error for the empty string.

Because the split is at the last dot, `name.with.dots.01` has nickname
`name.with.dots`, which then fails on the nickname character rule, and
`x.01.` has an empty discriminator.

### 2.2 Nickname rules, in the order they are checked

1. **Not empty**: if the nickname has no characters →
   `NicknameCannotBeEmpty`.
2. **First character is not an ASCII digit** (`0`–`9`) →
   `NicknameCannotStartWithDigit`. Note this is checked on the *original*
   string, before case folding, and that any non-digit first character
   (including a disallowed one such as a space or an emoji) passes this
   step.
3. **Character set**: after ASCII-lowercasing, every character must be one
   of `_`, `a`–`z`, `0`–`9`; otherwise `BadNicknameCharacter`. Only ASCII
   `A`–`Z` are folded; `É`, `ß`, `☃` and any non-ASCII digit are rejected.
4. **Hard length limit**: at most **48** characters, else
   `NicknameTooLong` (48 is the largest count that is guaranteed to fit in
   a scalar; see §3.3).
5. **Soft length limits** (a `[min, max]` range supplied by the caller;
   see §2.4): `< min` → `NicknameTooShort`, `> max` → `NicknameTooLong`.

All valid nicknames are ASCII, so "characters" and "bytes" coincide.

The nickname is **case-preserving but case-insensitive**: the string the
user typed is what is displayed and what goes into a username link, while
the hash (§3) is computed from the ASCII-lowercased form. `LOUD.700` and
`loud.700` have the same hash and therefore collide on the server.

### 2.3 Discriminator rules, in the order they are checked

The discriminator is parsed as an unsigned 64-bit integer.

1. Empty → `DiscriminatorCannotBeEmpty`.
2. First byte is not an ASCII digit → `BadDiscriminatorCharacter`. This is
   an explicit check so that `+1` and `-1` (which some integer parsers
   accept) are rejected here.
3. Parse the whole string as `u64` in base 10:
   - any non-digit byte → `BadDiscriminatorCharacter` (this covers
     `1a`, `1 `, `١٢`, and any other Unicode digit);
   - value above `2^64 − 1` → `DiscriminatorTooLarge`;
   - a parse result of zero is handled by step 4.
4. Value `0` → `DiscriminatorCannotBeZero`. This applies to `0`, `00` and
   any run of zeros, and it takes precedence over the leading-zero and
   single-digit rules.
5. Length rules on the *string*:
   - length 1 → `DiscriminatorCannotBeSingleDigit` (so `x.1` is invalid
     but `x.01` is fine);
   - length 2 → accepted, leading zero allowed (`01` … `99`);
   - length ≥ 3 → accepted only if the first character is `1`–`9`, else
     `DiscriminatorCannotHaveLeadingZeros`.

Consequences worth noting:

- The valid discriminator strings are exactly: `01`–`99` (two digits,
  leading zero allowed, but not `00`), and `100` … `18446744073709551615`
  with no leading zero.
- `jimio.18446744073709551615` (u64 max) is valid even though the server's
  candidate ranges never go above `999999999` (§4).
- A long run of zeros (`00000000000000000000000000`) is
  `DiscriminatorCannotBeZero`, but a long value that starts with `0` and
  is not all zeros (`0999999999999999999999`) overflows first and gives
  `DiscriminatorTooLarge`.

### 2.4 Nickname limits and where they come from

Validation has a hard limit (48) and a caller-supplied soft range
`[min, max]`:

- Creating a `Username` from a **whole string** (what signal-cli does for
  `-u name.01`, and what libsignal does for a username decrypted from a
  link) applies **no soft limit**: only the hard 48-character cap. So
  `ab.01` (2 characters) and a 48-character nickname both parse.
- Creating from **parts** or generating **candidates** applies the soft
  limits, which the caller passes in. libsignal's default is `[3, 32]`;
  signal-cli uses exactly `3` and `32`.
- The soft-limit constructor rejects `min ≥ max` and `max > 48` by
  aborting (programming error, not a username error).

So a 2-character nickname can be *used* if the server already granted it,
but cannot be *requested* through the nickname path.

### 2.5 Error strings

Each error has one fixed message. libsignal's JNI layer maps each to its
own exception class; signal-cli surfaces the message plus the simple class
name (see `spec/service/usernames.md` §7).

| Error | Message | Java exception class (`org.signal.libsignal.usernames.`) |
|---|---|---|
| MissingSeparator | `Username must contain a '.'` | `MissingSeparatorException` |
| NicknameCannotBeEmpty | `Name cannot be empty` | `CannotBeEmptyException` |
| NicknameCannotStartWithDigit | `Name cannot start with a digit` | `CannotStartWithDigitException` |
| BadNicknameCharacter | `Nickname contains disallowed character` | `BadNicknameCharacterException` |
| NicknameTooShort | `Nickname is too short` | `NicknameTooShortException` |
| NicknameTooLong | `Nickname is too long` | `NicknameTooLongException` |
| DiscriminatorCannotBeEmpty | `Discriminator cannot be empty` | `DiscriminatorCannotBeEmptyException` |
| DiscriminatorCannotBeZero | `Discriminator cannot be zero` | `DiscriminatorCannotBeZeroException` |
| DiscriminatorCannotBeSingleDigit | `Discriminator cannot be a single digit` | `DiscriminatorCannotBeSingleDigitException` |
| DiscriminatorCannotHaveLeadingZeros | `Discriminator cannot have leading zeros unless it would otherwise be a single digit` | `DiscriminatorCannotHaveLeadingZerosException` |
| BadDiscriminatorCharacter | `Discriminator must only be made up of digits` | `BadDiscriminatorCharacterException` |
| DiscriminatorTooLarge | `Value is too large to be a username discriminator` | `DiscriminatorTooLargeException` |
| (proof) ProofVerificationFailure | `Username could not be verified` | `ProofVerificationFailureException` |
| (link) InputDataTooLong | `The combined length of all input data is too long` | `UsernameLinkInputDataTooLong` |
| (link) InvalidEntropyDataLength | `Invalid size of the entropy data` | `UsernameLinkInvalidEntropyDataLength` |
| (link) UsernameLinkDataTooShort | `Username link data size is too short: must contain IV, ciphertext, and HMAC` | `UsernameLinkInvalidLinkData` |
| (link) HmacMismatch | `HMAC on username link doesn't match the one calculated with the given entropy input` | `UsernameLinkInvalidLinkData` |
| (link) BadCiphertext | `Ciphertext in the username link can't be decrypted` | `UsernameLinkInvalidLinkData` |
| (link) InvalidDecryptedDataStructure | `Data decrypted from the username link is of invalid structure` | `UsernameLinkInvalidLinkData` |

There is also an unused `BadDiscriminatorException` class in the Java
package; nothing in 0.99.1 throws it.

### 2.6 Display form

A username is displayed as `<nickname>.<discriminator>` with the
discriminator printed in decimal, **zero-padded to at least two digits**
(`8` prints as `08`, `700` as `700`). Parsing preserves the nickname's
case, so round-tripping `He110.01` yields `He110.01`.

## 3. Username hash

The hash is a ristretto255 point, encoded as 32 bytes, that commits to the
lowercased nickname and the discriminator. It is what the server stores
and what other clients look up; the username itself never reaches the
server except inside the encrypted link blob.

### 3.1 The three generators

Three fixed ristretto255 points `G1`, `G2`, `G3` are derived, once, as
three consecutive "point" squeezes from one Sho (HMAC variant, ZP §2):

- create the Sho with label
  `Signal_Username_20230130_Constant_Points_Generate` (ASCII, no
  terminator) and absorb-and-ratchet an **empty** data string — i.e. the
  two-argument `Sho(label, "")` form of ZP §2.3, which ratchets twice;
- squeeze a point, then a second, then a third, from the same Sho state
  (each point is 64 squeezed bytes mapped with the ristretto255
  one-way map; ZP §2.4 "point").

Their encodings (also in the vector file, `constants.base_points`):

```
G1 = 60b993663a3daecc4c852f533547e305388c2a50a58393ea277de4abf3de543a
G2 = f2b6f1c826fa3640206f3b58b2286bdefdfda6a54ff902f204a72de737d26157
G3 = 0606bd3abfce4e9617d448fb2caeb6cc028ec9a2b62b10b3d9eb2948da6f3f53
```

An implementation may hard-code these three encodings; upstream does, and
asserts in a unit test that they match the derivation above.

### 3.2 Scalar 1: `username_sha_scalar`

Let `n` be the **ASCII-lowercased** nickname (UTF-8 bytes, which are ASCII
here) and `d` the discriminator as a u64. Compute

```
SHA-512( n ‖ 0x00 ‖ be64(d) )
```

and map the 64-byte digest to a scalar by reading it **little-endian** and
reducing mod `ℓ` (the standard "from wide bytes" reduction; ZP §2.4
"scalar", but applied to a SHA-512 digest rather than squeezed bytes).

### 3.3 Scalar 2: `nickname_scalar`

Map each character of the lowercased nickname to a small integer:

| Character | Value |
|---|---|
| `_` | 1 |
| `a` … `z` | 2 … 27 |
| `0` … `9` | 28 … 37 |

(Any other character is `BadNicknameCharacter`. Value 0 is deliberately
unused so that trailing positions cannot be forged.)

Let the values be `v[0] … v[k−1]`, `1 ≤ k ≤ 48`. The scalar is

```
s = v[0] + 27 · ( v[1] + 37·v[2] + 37²·v[3] + … + 37^(k−2)·v[k−1] )
```

computed mod `ℓ`. Equivalently, and this is how upstream computes it:
start from 0; for `i` from `k−1` down to `1`, set `s ← 37·s + v[i]`; then
`s ← 27·s + v[0]`. The first character uses radix 27 because a nickname
cannot start with a digit, so `v[0] ∈ [1, 27]`.

Upstream unit-test value: for `_z9_` the result is
`1 + 27·(27 + 37·37 + 37²·1) = 74656`.

48 is the length cap because `27 · 37^47 < ℓ`, while a 49th character
could overflow.

### 3.4 Scalar 3: `discriminator_scalar`

The discriminator as a u64, converted to a scalar directly (`d mod ℓ`;
`d < 2^64 < ℓ`, so it is just the integer).

### 3.5 The hash

```
H = username_sha_scalar·G1 + nickname_scalar·G2 + discriminator_scalar·G3
```

The **username hash** is `enc(H)`: 32 bytes. On the wire it is always
base64url without padding (43 characters).

Example (upstream, `he110.42`):
`f63f0521eb3adfe1d936f4b626b89558483507fbdb838fc554af059111cf322e`.

The vector file's `hashes` array gives 18 usernames with their hash, the
base64url form, and the three scalars (little-endian hex), so an
implementation can check each step separately. Note `LOUD.700` and
`loud.700` have identical scalars and hash.

## 4. Candidate generation

Given a bare nickname and the soft limits `[min, max]` (signal-cli:
`[3, 32]`), a client produces **20** candidate usernames:

1. Validate the nickname as in §2.2 (hard rules, then the soft limits).
   Note that the character check here is applied to the lowercased string,
   and the length compared is the number of mapped characters, which for
   valid nicknames equals the character count.
2. For each of eight discriminator ranges (half-open, `[start, end)`),
   draw the given number of **distinct** values uniformly at random
   without replacement:

   | Range | Count |
   |---|---|
   | 1 … 99 | 4 |
   | 100 … 999 | 3 |
   | 1 000 … 9 999 | 3 |
   | 10 000 … 99 999 | 2 |
   | 100 000 … 999 999 | 2 |
   | 1 000 000 … 9 999 999 | 2 |
   | 10 000 000 … 99 999 999 | 2 |
   | 100 000 000 … 999 999 999 | 2 |

3. Concatenate them in range order (the four small ones first), and format
   each as `<nickname>.<discriminator>` with the discriminator zero-padded
   to at least two digits. So a drawn value of `8` becomes `nickname.08`,
   which is a valid username; `0` is never drawn because the first range
   starts at 1.

The resulting list is ordered, and the server tries it in order (§
`spec/service/usernames.md` §3). The exact random sampling algorithm is
not observable from the outside and is not part of the contract; only the
counts, the ranges, the distinctness within a range and the ordering are.

The vector file's `candidates` array has reference-jar samples (20 per
nickname, including one with a zero-padded two-digit value) plus the
rejection cases.

## 5. The zero-knowledge proof

When confirming a reserved username the client proves, without revealing
the username, that it knows a `(username_sha_scalar, nickname_scalar,
discriminator_scalar)` triple that produces the hash it is claiming. The
server verifies the proof against the hash it already holds. This stops a
client from claiming a hash it did not construct (e.g. one copied from
another user's link lookup).

### 5.1 Statement

One poksho equation (ZC §3.1), with the names used upstream:

```
username_hash = username_sha_scalar·G1 + nickname_scalar·G2 + discriminator_scalar·G3
```

Index assignment follows ZC §3.1: point index 0 is the ristretto255 base
point `G` (unused here but always present), then in order of first
appearance `username_hash` = 1, `G1` = 2, `G2` = 3, `G3` = 4; scalars
`username_sha_scalar` = 0, `nickname_scalar` = 1,
`discriminator_scalar` = 2.

Statement description `D` (ZC §3.2), 9 bytes:

```
01  01  03  00 02  01 03  02 04
```

(one equation; LHS point index 1; three terms; (scalar 0, point 2),
(scalar 1, point 3), (scalar 2, point 4)).

Point list `A` (ZC §3.3), 160 bytes, in index order:

```
enc(G) ‖ enc(username_hash) ‖ enc(G1) ‖ enc(G2) ‖ enc(G3)
```

where `enc(G)` is the ristretto255 base point
`e2f2ae0a6abc4e71a884a961c500515f58e30b6aa582dd8db6a65945e08d2d76`.

### 5.2 Message

Unlike the zkgroup proofs in ZC, this proof is over a **non-empty**
message: the 32-byte username hash itself (`enc(H)`, the same bytes that
are sent in the request). Both proving and verifying absorb it.

### 5.3 Proving and encoding

Run ZC §3.4 with that statement, the three witness scalars (in scalar-index
order: sha, nickname, discriminator), the point values above, message =
the 32-byte hash, and 32 bytes of randomness `Z`.

The proof is encoded as in ZC §3.5: challenge scalar then the three
response scalars, 32 bytes each — **128 bytes total**, base64url without
padding (171 characters) on the wire.

The proof is fully deterministic given `Z`: the reference jar returns
identical bytes for repeated calls with the same randomness (checked;
`proofs[*].deterministic` in the vector file). Upstream picks `Z` from a
CSPRNG for each confirm call (libsignal's Java helper fills 32 bytes from
`SecureRandom`; the service layer in signal-cli's fork does the same
before calling `generateProofWithRandomness`).

The randomness argument must be exactly 32 bytes; a different length is an
argument error from the bridge (`expected array with length 32`), not a
username error.

### 5.4 Verification

Given a candidate proof and a 32-byte hash:

1. The hash must decode as a ristretto255 point; if not, fail.
2. Build the point list with that point as `username_hash`.
3. Run ZC §3.6 with message = the 32 hash bytes.

Any failure (bad length, non-canonical scalar, wrong challenge,
undecodable hash) is reported as the single error
`ProofVerificationFailure` / `Username could not be verified`; there is no
distinction between "malformed" and "wrong". Reference-jar checks that all
produce it: flipped proof byte, proof for a different hash, proof
truncated to 96 bytes, hash of 32 `0xff` bytes, hash of 31 bytes.

The server runs exactly this verification before accepting a confirm
(`spec/service/usernames.md` §3.2).

### 5.5 Vectors

`testdata/vectors/usernames.json` → `proofs`: five (username, hash,
randomness, proof) tuples with fixed randomness, produced by the reference
jar, including the all-zero and all-`0xff` randomness and the u64-max
discriminator. Reproducing these byte-for-byte proves both the hash and
the poksho implementation.

## 6. Username links

A username link lets someone open a URL and learn the username (and from
it the hash, and from that the ACI) without the server learning the
username. The server stores only an opaque ciphertext under a random
handle; the decryption key lives in the URL fragment.

### 6.1 Pieces

| Piece | Size | Who makes it |
|---|---|---|
| entropy | 32 bytes | client (CSPRNG), or reused from a previous link |
| IV | 16 bytes | client (CSPRNG), fresh on **every** encryption |
| encrypted username ("link blob") | 112 bytes in practice; server accepts 1…128 | client |
| server ID / link handle | UUID (16 bytes) | server |

Re-encrypting the same username with the same entropy yields a *different*
blob (new IV), which is exactly what the "keep the link handle, rotate the
ciphertext" flow does.

### 6.2 Key derivation

Two 32-byte keys, both HKDF-SHA256 with **no salt** (RFC 5869 with a salt
of 32 zero bytes), IKM = the 32 entropy bytes, L = 32, and these info
strings (ASCII, no terminator):

| Key | Info |
|---|---|
| AES key | `Signal Username Link Encryption Key` |
| MAC key | `Signal Username Link Authentication Key` |

### 6.3 Plaintext

A protobuf message (proto3) with two fields:

| Field | Number | Type | Content |
|---|---|---|---|
| username | 1 | string | the username, exactly as the user wrote it (case preserved), UTF-8 |
| padding | 2 | bytes | `max(0, 48 − len(username in bytes))` zero bytes |

Encoded in field-number order, with proto3's "skip empty" rule: if the
username is exactly 48 bytes or longer the padding field is omitted
entirely. Concretely, for a username of `u` bytes with `u < 48` the
plaintext is `0x0a, u, <username>, 0x12, 48−u, <48−u zero bytes>` and is
always **52 bytes**; for `u ≥ 48` it is `0x0a, u, <username>`, i.e. `u + 2`
bytes (the length is a single varint byte for `u < 128`).

Length rule: the encoded plaintext must be **more than 48 and fewer than
64 bytes**, so that the ciphertext is exactly four AES blocks. If it is 64
or more the operation fails with `InputDataTooLong`. In practice this caps
the username string at **61 bytes** (62 fails), which is more than any
legal username needs except a 48-character nickname with a discriminator
longer than 12 digits.

### 6.4 Encryption

```
ciphertext = AES-256-CBC-PKCS#7( key = AES key, iv = IV, plaintext )
blob       = IV ‖ ciphertext ‖ HMAC-SHA256( MAC key, IV ‖ ciphertext )
```

With a 52-byte or 50-byte plaintext the ciphertext is 64 bytes and the
blob is `16 + 64 + 32 = 112` bytes.

The 112-byte blob is what goes to the server (`encryptedUsername` /
`usernameLinkEncryptedValue`); the entropy never leaves the client.

### 6.5 Decryption

1. If the entropy is not exactly 32 bytes → `InvalidEntropyDataLength`.
2. If the blob length is **≤ 48** → `UsernameLinkDataTooShort`.
3. Split off the last 32 bytes as the tag; recompute
   `HMAC-SHA256(MAC key, everything before the tag)` and compare in
   constant time; mismatch → `HmacMismatch`.
4. First 16 bytes are the IV, the rest is the ciphertext; AES-256-CBC
   decrypt and strip PKCS#7; failure (bad padding, length not a multiple
   of 16) → `BadCiphertext`.
5. Parse the plaintext as the §6.3 message; a parse failure →
   `InvalidDecryptedDataStructure`. Unknown fields are ignored and a
   missing field 1 yields an empty username (which then fails username
   validation).
6. Return field 1. Callers validate it as a username; when the string came
   from a link lookup libsignal deliberately collapses *any* username
   syntax error into `InvalidDecryptedDataStructure` rather than leaking
   which rule failed (the user never typed it).

Note that step 2 accepts a 49-byte blob even though the ciphertext would
then be 1 byte; such a blob fails at step 4.

### 6.6 The `signal.me` URL

Serialization (signal-cli, C1):

```
https://signal.me/#eu/ + base64url_nopad( entropy(32) ‖ serverId(16) )
```

The 48 bytes always encode to exactly 64 base64 characters with no
padding. The server ID is the link handle UUID in RFC 9562 byte order
(most-significant 8 bytes first).

Parsing, exactly as signal-cli does it:

1. Match the whole input against the regular expression
   `(https://)?signal.me/?#eu/([a-zA-Z0-9+\-_/]+)` (full match, no
   surrounding whitespace allowed). Quirks that follow from it:
   - the scheme is optional, and the `/` before `#` is optional, so
     `signal.me#eu/<data>` is accepted;
   - the `.` in `signal.me` is an unescaped regex dot, so `signalxme`
     is also accepted (observed);
   - matching is case-sensitive: `HTTPS://…` is rejected;
   - `http://`, `sgnl://`, `www.signal.me` and a trailing `/` after the
     data are rejected;
   - the character class allows **both** base64 alphabets (`+`, `/`, `-`,
     `_`) but **not** `=`, so a padded link is rejected.
2. Base64-decode the captured group. signal-cli's decoder adds `=`
   padding if needed and chooses the URL-safe alphabet if (and only if)
   the string contains `-` or `_`, otherwise the standard alphabet.
   Consequence: a mixed string containing `_` *and* `+` fails with
   `Invalid base64 encoding`, while a pure standard-alphabet string
   (`+`, `/`) decodes fine. Decode failure → `Invalid base64 encoding`.
3. The decoded length must be exactly **48**; otherwise
   `Invalid username link`.
4. First 32 bytes are the entropy, last 16 the server ID, read as a UUID.
   (The null check on the UUID can never fire for 16 bytes.)
5. Re-serializing always produces the canonical base64url form, so parsing
   a standard-alphabet link and printing it normalizes `+` → `-`,
   `/` → `_`.

Both error messages are exactly `Invalid username link` and
`Invalid base64 encoding` (a third, `Invalid serverId`, is unreachable).

`testdata/vectors/usernames.json` → `link_url_cases` (21 accepted and
rejected inputs with the parsed entropy, server ID and canonical URL) and
`link_url_construction` (three entropy/UUID → URL pairs including all-zero
and all-`0xff`).

### 6.7 Vectors

- `links`: three (username, entropy, IV) triples with the derived AES and
  MAC keys, the plaintext, the full blob in hex and base64url. These were
  encrypted by an independent implementation of §6.2–§6.4 and then
  **decrypted by the reference jar** back to the original username, so
  they pin every intermediate value.
- `link_upstream`: libsignal's own two test blobs — `moxie.01`, and one
  that decrypts to `testtest` (a valid protobuf whose string is not a
  valid username, used to exercise the error path).
- `link_errors`, `link_length_boundary`: the failure cases and the
  61/62-byte cut-off.

## 7. Size summary

| Value | Size |
|---|---|
| username hash | 32 bytes (43 chars base64url) |
| proof | 128 bytes (171 chars base64url) |
| link entropy | 32 bytes |
| link IV | 16 bytes |
| link blob | 112 bytes (server limit 128) |
| link URL data | 48 bytes → 64 base64 characters |
| nickname | 1…48 characters (3…32 when requested by nickname) |
| discriminator | 1 … 2^64−1, as a 2…20 character string |

## Gaps

1. **Candidate sampling algorithm.** Upstream draws distinct values per
   range with a library sampler whose exact sequence depends on the RNG;
   only counts, ranges, distinctness and ordering are specified here. If a
   test ever needs byte-identical candidate lists it cannot get them from
   this spec.
2. **`Sho` double-ratchet for the generator label.** §3.1 states the
   derivation as `Sho(label, "")`, i.e. the empty-data two-argument form.
   This was not re-derived here (no ristretto255 tooling was available in
   the scratch environment); it is taken from upstream's own assertion.
   The three encodings in §3.1 are authoritative either way — implementers
   should hard-code them and treat the derivation as documentation.
3. **Proof vectors are reference-jar outputs, not published vectors.**
   libsignal publishes no fixed proof bytes (its tests only round-trip).
   The five tuples in the vector file were generated offline from the
   distribution jar with fixed randomness; they are correct for libsignal
   0.99.1 but are not an upstream-blessed KAT.
4. **HKDF salt.** Upstream passes "no salt"; this spec reads that as RFC
   5869's all-zero salt of hash length (32 bytes). Not independently
   confirmed against a vector with a non-trivial entropy — though the
   three `links` vectors do confirm it, since they were produced with an
   explicit 32-byte zero salt and the jar decrypts them.
5. **Protobuf plaintext for a username containing invalid UTF-8** cannot
   arise from a valid username, so the behaviour is unspecified here.
6. **`BadDiscriminatorException`** exists as a Java class with no thrower
   in 0.99.1; if a later version uses it the mapping table in §2.5 needs
   an entry.
