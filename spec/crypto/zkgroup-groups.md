# zkgroup: group keys, member ciphertexts, blobs, server params (libsignal v0.99.1)

Spec role: prose only; no code. This file covers the half of libsignal's
zkgroup that Signal groups v2 needs **apart from** credentials, proofs and
endorsements:

- deriving everything from a 32-byte **group master key**: group secret
  params, group public params and the **group identifier** (group ID);
- encrypting and decrypting **service IDs** (ACI or PNI) into
  `UuidCiphertext`;
- encrypting and decrypting **profile keys** into `ProfileKeyCiphertext`;
- encrypting and decrypting **group blobs** (AES-256-GCM-SIV with a
  padding header);
- the layout of the **server public params** shipped in signal-cli, and
  verification of the server's **notary signature** (used on group
  changes);
- zkgroup's common serialization rules.

Auth credentials, profile-key credentials and presentations, group send
endorsements, and the proofs inside them are specified in
`spec/crypto/zkgroup-credentials.md`. The Sho construction, the three
ways of turning squeezed bytes into group values, the UUID byte order,
and the profile-key points M3 and M4 are specified in
`spec/crypto/zkgroup-profile.md` (cited below as **[PROF]**, with
section numbers). This file uses its notation and does not repeat it.
How the groups service uses these primitives (which protobuf fields hold
which ciphertext, and so on) belongs to the groups-service spec, not
here.

Every operation here is deterministic once its randomness input is
fixed. Service-ID and profile-key encryption take **no** randomness at
all.

## Sources

| Ref | What | URL @ version |
|-----|------|---------------|
| Z1 | libsignal zkgroup | https://github.com/signalapp/libsignal @ tag v0.99.1 = commit 97801d22dcf9f5bf714f7b8fa3212cdc973ae1c8: `rust/zkgroup/src/api/groups/{group_params,uuid_ciphertext,profile_key_ciphertext}.rs`, `rust/zkgroup/src/crypto/{uid_encryption,uid_struct,profile_key_encryption,profile_key_struct,signature,credentials}.rs`, `rust/zkgroup/src/api/{server_params,generic_server_params}.rs`, `rust/zkgroup/src/common/{sho,constants,serialization,simple_types}.rs`; tests `rust/zkgroup/tests/integration_tests.rs` and the unit tests in the files above |
| Z2 | zkcredential (same tree) | `rust/zkcredential/src/{attributes,sho,credentials,endorsements}.rs` |
| Z3 | poksho (same tree) | `rust/poksho/src/{sign,statement,proof,shohmacsha256,shoapi}.rs` |
| Z4 | libsignal-core (same tree) | `rust/core/src/address.rs` (service-ID kinds and binary forms) |
| Z5 | libsignal bridge and bindings (same tree) | `rust/bridge/shared/src/zkgroup.rs`, `rust/bridge/shared/types/src/zkgroup.rs`; Java `java/shared/java/org/signal/libsignal/zkgroup/{groups/ClientZkGroupCipher,groups/GroupSecretParams,ServerPublicParams,ServerSecretParams,GenericServerPublicParams}.java`; tests `java/client/src/test/java/org/signal/libsignal/zkgroup/integrationtests/ZkGroupTest.java`, `node/ts/test/ZKGroup-test.ts`, `swift/Tests/LibSignalClientTests/ZKGroupTests.swift` |
| D1 | curve25519-dalek 5.0.0 with feature `lizard` (as in [PROF]) | https://github.com/dalek-cryptography/curve25519-dalek @ tag curve25519-5.0.0 = commit 07bef73ff85998a206cd2cea7f2605c801d0d1c9: `curve25519-dalek/src/lizard/{lizard_ristretto,jacobi_quartic,lizard_constants,u64_constants}.rs`, `src/lizard/README.md`, `src/ristretto.rs`, `src/scalar.rs` (serde impls) |
| C1 | signal-cli | https://github.com/AsamK/signal-cli @ tag v0.14.7 = commit b01b6b370dc063599a1a2b9fde0f5ff4e2d78fe8: `lib/src/main/java/org/asamk/signal/manager/config/{StagingConfig,LiveConfig}.java`, `manager/groups/GroupUtils.java`, `manager/helper/{GroupV2Helper,GroupHelper}.java` |
| C2 | Turasa/libsignal-service-java | https://github.com/Turasa/libsignal-service-java @ tag v2.15.3_unofficial_151 = commit 66956cb4a0394d50cee3590209baf58312084a44: `lib/libsignal-service/src/main/java/org/whispersystems/signalservice/api/groupsv2/{GroupsV2Operations,ClientZkOperations}.java` |
| R1 | Reference distribution, offline | `jshell` over `/work/third_party/signal-cli-0.14.7/lib/*` (libsignal-client 0.99.1 with native library; bcprov-jdk18on 1.85 for crafting raw AES-GCM-SIV inputs), no network |
| S1 | ristretto255 | RFC 9496 |
| S2 | AES-GCM-SIV | RFC 8452 |
| S3 | UUID byte layout | RFC 9562 |

Vectors: `testdata/vectors/zkgroup_groups.json` (§11).

## 1. Notation

Everything in [PROF] §1 applies: `‖`, `HMAC`, `p`, `ℓ`, point encodings,
little-endian scalars, and the label convention. In addition:

- "Sho(label, data)", "squeeze n", and the three mappings **point**
  (64-byte squeeze), **single-Elligator point** (32-byte squeeze) and
  **scalar** (64-byte squeeze) are as in [PROF] §2. When one Sho
  instance produces several values, they are consecutive squeezes from
  the same state, in the order listed.
- `MAP(r)` is the RFC 9496 §4.3.4 MAP function applied to field element
  `r`. `enc(P)` is the 32-byte encoding of point `P`. `B` is the
  ristretto255 base point (`e2f2ae0a…2d76`, RFC 9496).
- `i` is `SQRT_M1` of RFC 9496. A field element is **negative** if its
  canonical little-endian encoding has the low bit set, otherwise
  **non-negative** ("positive" in dalek's wording). `|x|` is the
  non-negative one of `x` and `−x`.
- `SQRT_RATIO_M1(u, v)` is the RFC 9496 §4.2 function; it returns a flag
  and a non-negative field element.
- Point equality always means ristretto255 group equality (equal
  encodings), never equality of internal coordinates.
- For a scalar `k`, `k⁻¹` is its inverse mod `ℓ`.

## 2. Common serialization rules

zkgroup serializes every structure with bincode 1.x options "fixed-int,
little-endian". For the structures here this means:

- Fields are written in declaration order with **no** length prefixes,
  tags or padding.
- A fixed-size byte array (master key, group ID, AES key, UUID bytes) is
  written as its raw bytes.
- A point is written as its 32-byte encoding; a scalar as its 32-byte
  little-endian canonical encoding.
- A **reserved byte** is written as `0x00`. Most top-level structures
  start with one. libsignal calls the same mechanism a "version byte"
  where the value is not zero; every structure in this file uses `0x00`.

Parsing (deserialization) rules:

- The leading reserved byte must be exactly `0x00`; any other value is a
  deserialization failure.
- Every point must be a valid RFC 9496 encoding (canonical, decodes); a
  scalar must be canonical (value `< ℓ`). Anything else is a
  deserialization failure.
- Input that is too short is a failure.
- **Trailing bytes are not rejected by zkgroup's own deserializer.**
  Upstream intends to reject them, but the way it drives bincode skips
  the final end-of-input check. The bindings add an exact length check
  for every fixed-size type (all types in §3–§6), so those reject wrong
  lengths in practice. The variable-size handle types
  (`ServerPublicParams`) and `GenericServerPublicParams` accept extra
  trailing bytes. The reference jar accepted 674- and 678-byte server
  params and a 226-byte generic params (R1, vectors). signal-cli only
  parses its own hardcoded constants, so an implementation may require
  exact lengths everywhere.
- Parsing never checks consistency between fields (for example, that a
  public key matches its secret scalars or that a group ID matches its
  master key). The reference jar accepted a group secret params with a
  flipped group-ID byte and reported the altered identifier.

zkgroup reports two error kinds: **deserialization failure** (bad
encoding or length) and **verification failure** (any cryptographic
check failed: decryption, blob authentication, signature). The Java API
maps them to `InvalidInputException` and `VerificationFailedException`.

## 3. Group master key and group params

### 3.1 Group master key

The group master key is 32 bytes. It is the only secret a group member
stores. Its serialization is the 32 raw bytes (no reserved byte, length
exactly 32).

**Generating a new key.** libsignal does not use the 32 random bytes
directly. It computes Sho(`Signal_ZKGroup_20200424_Random_GroupSecretParams_Generate`,
`R`) for 32 random bytes `R`, and the master key is the first 32-byte
squeeze. signal-cli uses this path when it creates a group (C1
`GroupV2Helper`). Since `R` is secret and random, an implementation that
draws the 32 master-key bytes straight from a CSPRNG is
indistinguishable. Use the Sho path only to reproduce the vectors.

### 3.2 Derivation of the group secret params

Compute Sho(`Signal_ZKGroup_20200424_GroupMasterKey_GroupSecretParams_DeriveFromMasterKey`,
`master_key`). Then take these consecutive squeezes from that one state:

| # | Value | Squeeze | Meaning |
|---|-------|---------|---------|
| 1 | `group_id` | 32 bytes, raw | the group identifier (§3.5) |
| 2 | `blob_key` | 32 bytes, raw | AES-256 key for blobs (§6) |
| 3 | `a1` | scalar (64 bytes) | service-ID encryption key, first scalar |
| 4 | `a2` | scalar (64 bytes) | service-ID encryption key, second scalar |
| 5 | `b1` | scalar (64 bytes) | profile-key encryption key, first scalar |
| 6 | `b2` | scalar (64 bytes) | profile-key encryption key, second scalar |

Then the two public keys:

- `A_uid = a1·G_a1 + a2·G_a2`
- `A_pk  = b1·G_b1 + b2·G_b2`

with the system generators of §3.4.

(libsignal names `b1`, `b2` "a1", "a2" of the profile-key key pair; the
renaming here only avoids ambiguity.)

### 3.3 Serialized layouts

**GroupSecretParams**, 289 bytes:

| Offset | Len | Field |
|--------|-----|-------|
| 0 | 1 | reserved `0x00` |
| 1 | 32 | `master_key` |
| 33 | 32 | `group_id` |
| 65 | 32 | `blob_key` |
| 97 | 32 | `a1` |
| 129 | 32 | `a2` |
| 161 | 32 | `enc(A_uid)` |
| 193 | 32 | `b1` |
| 225 | 32 | `b2` |
| 257 | 32 | `enc(A_pk)` |

**GroupPublicParams**, 97 bytes:

| Offset | Len | Field |
|--------|-----|-------|
| 0 | 1 | reserved `0x00` |
| 1 | 32 | `group_id` |
| 33 | 32 | `enc(A_uid)` |
| 65 | 32 | `enc(A_pk)` |

A client never needs to parse either structure from the network.
signal-cli re-derives them from the master key each time (C1). Both
layouts matter only where the public params are sent to the server
(groups-service spec) and for the vectors.

### 3.4 System generators

| Name | Derivation | Encoding |
|------|------------|----------|
| `G_a1`, `G_a2` | Sho(`Signal_ZKGroup_20200424_Constant_UidEncryption_SystemParams_Generate`, empty data), two consecutive **point** squeezes | `a6324c368df734691147981348b6e7eb42c3307e711b6c7eccd3032d45693f5a`, `048013525b76124bf2640c5e9369c76efbe80aba2a24aa5d8e18a98eba14f837` |
| `G_b1`, `G_b2` | Sho(`Signal_ZKGroup_20200424_Constant_ProfileKeyEncryption_SystemParams_Generate`, empty data), two consecutive **point** squeezes | `f6baa317ce1839c93d617e0cd837d19da9c8a4c520bf7c51b1e6c2cb2a049c61`, `2e0175894c8730b203ab3bd98ecb2d81abacb65f8a6124f49771d14a9852120c` |

As with the commitment generators ([PROF] §4.1), libsignal ships these
as hardcoded constants, and its tests assert that they equal the
derivation. The empty data is still absorbed. The spec agent reproduced
both derivations.

### 3.5 Group identifier

The group identifier is `group_id` (squeeze #1 of §3.2): 32 bytes,
serialized raw. It is a pure function of the master key. signal-cli's
v2 group ID is exactly these 32 bytes (C1 `GroupUtils`), and its
base64 is the group ID users see.

## 4. Service-ID encryption (`UuidCiphertext`)

### 4.1 Plaintext points

A service ID has a kind (ACI or PNI) and a 16-byte UUID `U` in RFC 9562
order ([PROF] §3.1). Its **binary form** (Z4) is:

- ACI: the 16 bytes of `U` (no prefix);
- PNI: `0x01 ‖ U` (17 bytes).

(The fixed-width form, always 17 bytes with prefix `0x00` for ACI and
`0x01` for PNI, is what the Java API passes across JNI and what
decryption returns. It is not hashed anywhere.)

Two points represent the service ID:

- **M1** = the **point** squeezed (64 bytes) after
  Sho(`Signal_ZKGroup_20200424_UID_CalcM1`, binary form). M1 depends on
  the kind, so an ACI and a PNI with the same UUID give different M1.
- **M2** = Lizard encoding of `U` (§4.3). M2 depends only on the UUID.

### 4.2 Encryption

With the group's `a1`, `a2`:

- `E1 = a1·M1`
- `E2 = a2·E1 + M2`

There is no randomness: the same service ID in the same group always
gives the same ciphertext. (The groups server relies on this to match
members.)

**UuidCiphertext**, 65 bytes: reserved `0x00`, `enc(E1)` (32),
`enc(E2)` (32). Parsing follows §2: reserved byte `0x00`, two valid
points, length exactly 65.

### 4.3 Lizard encoding and decoding

**Encode** 16 bytes `U` to a point:

1. `h = SHA-256(U)` (32 bytes).
2. Build 32 bytes `s = h[0..8] ‖ U ‖ h[24..32]`. That is, bytes 8–23 of
   the hash are replaced by `U`.
3. Clear bit 0 of `s[0]` (AND `0xFE`) and the top two bits of `s[31]`
   (AND `0x3F`).
4. Read `s` as a little-endian integer (it is now `< 2^254`, so no
   reduction occurs) and output `MAP(s)`.

**Decode** a point `P`:

1. Compute the eight **preimage slots** of `P` (§4.4).
2. For every defined slot value `c`, take its 32-byte canonical encoding
   `cb`, and let `V = cb[8..24]`. The slot **passes** if `cb` equals the
   bytes that steps 1–3 of encoding produce for `V`, that is
   `SHA-256(V)[0..8] ‖ V ‖ SHA-256(V)[24..32]` with the same two masks.
3. If exactly one slot passes, the result is its `V`. If none or more
   than one passes, decoding fails. (A legitimate encoding has a second
   passing preimage with probability about 2^−122; such inputs cannot be
   decoded, and libsignal accepts this.)

Upstream vectors for Lizard encoding: four from D1 and one round trip
from Z1, in the vectors file.

### 4.4 Preimage slots of MAP

MAP is not injective. Every point has at most eight non-negative field
elements `r` with `MAP(r) = P` (and their negatives). Decryption needs
them in a specific form: **eight slots**, each either undefined or
holding a non-negative field element that maps to `P`.

**Normative definition.**

- If `P` is **not the identity**, the defined slots hold exactly the
  distinct non-negative field elements `r ∈ [0, p)` with `MAP(r) = P`,
  each once. Their number is between 0 and 8, and the order does not
  matter. Any correct method of finding all such preimages is
  acceptable.
- If `P` is **the identity**, all eight slots are defined and hold this
  fixed multiset (little-endian hex), in which `0` and `SQRT_ID` each
  appear **twice**:
  `SQRT_ID = a81b5c4acb2a3075aa6dea0e2da9bccd156eeb739954347597eb7bf45855b305`,
  `0`, `SQRT_ID`, `0`,
  `bced0d08ab13994b901f939d3d6f164d7d1f1b7860d5ccdbca27c4b705f23d02`,
  `a2ce3011a78ef573098dfffa7c6319a2973184a23f8c7e4fb28a95abee32615f`,
  `068c5b499d860a72cba9a67991e93e203e07f2cecac35ff78ba286490a9d647f`,
  `2a6382d98d9b4fe0526bf32f2e36b1c6fb594edaadaa30f35a29566c59adfe4c`.
  Each of these maps to the identity. The repetition is observable: see
  §5.3.

**A method (informative)** that produces exactly these slots. It follows
the inversion of Elligator through the Jacobi quartic described in D1's
`lizard/README.md`, written here for affine coordinates. Constants (field
elements; `d` is the Edwards constant and the RFC 9496 constants are used
with the RFC's signs):

- `SQRT_ID` = the non-negative square root of `i·d` (value above);
- `K1 = (d + 1)/(d − 1)`;
- `K2 = −2·INVSQRT_A_MINUS_D`;
- `K3 = −1/√(1 + d)`, with `1/√(1 + d)` the non-negative root that
  SQRT_RATIO_M1(1, 1 + d) returns.

Let `(x, y)` be the affine coordinates of any Edwards representative of
`P`.

1. **Special case.** If `x = 0` or `y = 0` (this happens exactly for the
   identity), use these four Jacobi points: `(0, 1)`, `(0, 1)`,
   `(1, K2·i)`, `(−1, K2·i)`.
2. Otherwise let `g` = the element returned by
   SQRT_RATIO_M1(1, y⁴·x²·(1 − y²)), `e = g·y²` and `e' = (1 − y²)·K3·g`.
   The four Jacobi points `(s, t)` are:
   - `(e·(1 − y)·x, K2·e·(1 − y))`
   - `(−e·(1 + y)·x, K2·e·(1 + y))`
   - `(e'·(i − x)·y, K2·i·e'·(i − x))`
   - `(−e'·(i + x)·y, K2·i·e'·(i + x))`
3. For each of the four points `(s, t)`, in order, form two slots: one
   from `(s, t)` and one from its dual `(−s, −t)`. That gives 8 slots.
4. The slot value for a Jacobi point `(s, t)`:
   - if `s = 0`: defined, with value `SQRT_ID` when `t = 1`, otherwise `0`;
   - otherwise let `a = (t + 1)·K1` and compute
     SQRT_RATIO_M1(1, i·(s⁴ − a²)) → (flag, `w`). If the flag is false,
     the slot is undefined. Otherwise the value is
     `|(a + s²)·w|` if `s` is non-negative, or `|(a − s²)·w|` if `s` is
     negative.

The spec agent implemented this method. For 300 random points
`P = MAP(r)` it produced only distinct non-negative preimages, always
including `|r|`. It reproduced every decryption outcome of the reference
jar, including the failures in §5.3. Projective formulas that differ by
the representative or the sign of `g` swap a point with its dual. That
changes slot order but not the multiset, and only the multiset matters.

### 4.5 Decryption

Input: the group's `a1`, `a2`, and a parsed ciphertext `(E1, E2)`.

1. If `E1` equals the base point `B`, fail.
2. `M2' = E2 − a2·E1`.
3. Lizard-decode `M2'` (§4.3). On failure, fail. Otherwise call the 16
   bytes `U`.
4. `M1' = a1⁻¹·E1`.
5. If `M1'` equals M1 of the **ACI** `U`, the result is ACI `U`.
   Otherwise, if it equals M1 of the **PNI** `U`, the result is PNI `U`.
   Otherwise fail.

All failures are verification failures. Upstream does both M1
comparisons in constant time and branches only at the end. The two
candidates can never both match. Wrong-group ciphertexts, swapped
halves, `E1 = B` and `E1 = identity` all fail (vectors).

## 5. Profile-key encryption (`ProfileKeyCiphertext`)

### 5.1 Plaintext points

The plaintext is a 32-byte profile key `K` together with the owner's ACI
UUID `U` (16 bytes; only ACIs are used here). The two points are exactly
**M3** and **M4** of [PROF] §3.2:

- `M3` = single-Elligator point from
  Sho(`Signal_ZKGroup_20200424_ProfileKeyAndUid_ProfileKey_CalcM3`, `K ‖ U`);
- `M4` = single-Elligator mapping of `K` with `K[0] &= 0xFE` and
  `K[31] &= 0x3F`.

### 5.2 Encryption and format

With the group's `b1`, `b2`: `E1 = b1·M3`, `E2 = b2·E1 + M4`. No
randomness is used.

**ProfileKeyCiphertext**, 65 bytes: reserved `0x00`, `enc(E1)`,
`enc(E2)`. The parsing rules are the same as for `UuidCiphertext`.

### 5.3 Decryption

Input: `b1`, `b2`, the parsed ciphertext and the **ACI UUID `U` of the
member the key belongs to**. The caller must know `U`. In groups it is
the member's decrypted service ID (§4.5).

1. If `E1 = B`, fail.
2. `M4' = E2 − b2·E1`; `T = b1⁻¹·E1` (this should equal M3).
3. Compute the eight preimage slots of `M4'` (§4.4).
4. For every defined slot value `c` (32-byte encoding `cb`), form eight
   **variants** of `cb` by optionally setting each of the three masked
   bits: bit 0 of byte 0 (`|= 0x01`), bit 7 of byte 31 (`|= 0x80`) and
   bit 6 of byte 31 (`|= 0x40`). For each variant `K'`, compute
   `M3(K', U)` and compare it with `T`.
5. Count the matches over **all** slots and variants. If the count is
   exactly 1, the matching `K'` is the profile key. Any other count
   fails.

Consequences (all confirmed with the reference jar; vectors):

- Decryption with any ACI other than the one used to encrypt fails.
- A profile key whose masked form (`K[0] & 0xFE`, `K[31] & 0x3F`) is all
  zero, or equals `SQRT_ID`, **encrypts fine but can never be
  decrypted**. Its M4 is the identity, the true key matches in two slots,
  and the count is 2. Examples: the all-zero key, `01 00…00`,
  `00…00 c0`, and `SQRT_ID` itself. The other identity preimage that is
  a valid masked key (`bced0d08…f23d02`) decrypts normally, and so do
  its variants.

## 6. Group blob encryption

zkgroup encrypts arbitrary group attributes ("blobs") under the group's
`blob_key`. The bindings, and therefore every client including
signal-cli, always use the **padded** form (§6.2). The unpadded form
exists only in the Rust API.

### 6.1 Unpadded encryption (inner layer)

Inputs: `blob_key` (§3.2), 32 bytes of randomness `R`, plaintext `X`.

1. `nonce` = the first 12-byte squeeze of
   Sho(`Signal_ZKGroup_20200424_Random_GroupSecretParams_EncryptBlob`, `R`).
2. `C = AES-256-GCM-SIV(key = blob_key, nonce, plaintext = X, no
   associated data)` per RFC 8452. `C` is the ciphertext followed by the
   16-byte tag, so `|C| = |X| + 16`.
3. Output `C ‖ nonce ‖ 0x00`. Total length `|X| + 29`.

The trailing byte is a reserved byte, but it sits at the **end** and is
**not checked** on decryption (see below).

A random `R` makes the nonce random. An implementation may draw the
12-byte nonce from a CSPRNG directly, and must use the Sho path only to
reproduce vectors.

**Unpadded decryption** of `Y`:

1. If `|Y| < 13`, fail.
2. Drop the last byte of `Y`, **whatever its value**. The reference jar
   decrypted a blob whose last byte was `0x7f`.
3. The last 12 remaining bytes are the nonce. The rest is `C`.
4. If `|C| < 16`, fail.
5. AES-256-GCM-SIV-decrypt `C` with `blob_key` and the nonce. An
   authentication failure fails.

All failures are verification failures.

### 6.2 Padded form (what clients use)

**Encryption** with padding length `n` (an unsigned 32-bit integer):

1. Build `Xp = be32(n) ‖ X ‖ 0x00 × n`, where `be32` is the 4-byte
   big-endian encoding.
2. Encrypt `Xp` with §6.1.

The output length is `|X| + n + 33`. The Java, Node and Swift
`encryptBlob` always use `n = 0`, so signal-cli's blobs are
`0x00000000 ‖ X` inside, `|X| + 33` bytes outside. Decryption accepts
any `n`. The groups-service spec says which fields are encrypted this
way, since zkgroup does not know.

**Decryption:**

1. Unpadded-decrypt (§6.1) to `Xp`.
2. If `|Xp| < 4`, fail.
3. `n` = the first 4 bytes read as a big-endian unsigned integer.
4. If `|Xp| − 4 < n`, fail.
5. The plaintext is `Xp` without its first 4 bytes and without its last
   `n` bytes.

The padding bytes are **not** checked to be zero. The crafted vectors
include a non-zero padding byte that decrypts fine, a padding length
equal to the remaining length (empty plaintext), a padding length one
too large, an inner plaintext shorter than 4 bytes, and `n = 0xFFFFFFFF`.

## 7. Notary signatures (server signatures)

### 7.1 Where signal-cli uses them

The groups server signs each group change. A `GroupChange` carries the
serialized actions bytes and a `serverSignature`. When signal-cli
receives a signed group change embedded in a group message, it verifies
it (C1 `GroupV2Helper.getDecryptedGroupChange` → C2
`GroupsV2Operations.decryptChange` in verify mode):

- the signature must parse as a 64-byte notary signature, else the
  change is rejected;
- it is verified with the server public params' `sig_public_key` (§8)
  over the **exact actions bytes as received**, not a re-encoding;
- the decoded actions' group ID must then equal the group identifier;
- changes whose epoch is above 7 (C2 `HIGHEST_KNOWN_EPOCH`) are ignored,
  without an error.

Any failure discards the embedded change. The groups-service spec covers
what happens next. Nothing else in this file's half carries a server
signature.

### 7.2 Format

A notary signature is 64 bytes: `c ‖ s`, two 32-byte little-endian
scalars. Both must be **canonical** (`< ℓ`). A non-canonical value fails
verification. It is not reduced.

### 7.3 Verification

The signature is a poksho (Z3) proof of knowledge of `x` with
`PK = x·B`. Given the public key `PK` (a point), the message `m`
(arbitrary bytes, possibly empty) and the signature `c ‖ s`:

1. If either scalar is non-canonical, fail.
2. `R = s·B − c·PK`.
3. Sho chain (HMAC-SHA256 Sho of [PROF] §2):
   - create with label `POKSHO_Ristretto_SHOHMACSHA256`;
   - absorb and ratchet the single byte string
     `01 01 01 00 00 ‖ enc(B) ‖ enc(PK)`
     (5 + 64 bytes). The first five bytes describe the statement "one
     equation; left side point #1; one term; scalar #0 times point #0";
   - absorb and ratchet `enc(R) ‖ m`;
   - squeeze 64 bytes and reduce them to a scalar `c'` ([PROF] §2.4
     **scalar** mapping).
4. The signature is valid if and only if `c' = c`.

Each "absorb and ratchet" is one HMAC over the concatenation followed by
`0x00`, exactly as in [PROF] §2.2. libsignal absorbs the pieces
separately before a single ratchet, which gives the same result.

**Signing** (servers only; for the vectors): the nonce scalar comes from
a separate Sho branch keyed with 32 bytes of randomness and the secret.
signal-cli never signs. A verifier need not reproduce signing, and the
vectors give the signatures directly.

## 8. Server public params

### 8.1 What signal-cli ships

signal-cli embeds three public-parameter blobs per environment (C1
`StagingConfig`, `LiveConfig`), as standard base64. They are passed to
libsignal-service-java as `zkGroupServerPublicParams`,
`genericServerPublicParams` and `backupServerPublicParams`.

| Environment | Blob | Bytes | Starts with (base64) |
|-------------|------|-------|----------------------|
| production | `zkGroupServerPublicParams` | 673 | `AMhf5ywVwITZ…` |
| production | `genericServerPublicParams` | 225 | `AByD873dTilm…` |
| production | `backupServerPublicParams` | 225 | `AJwNSU55fsFC…` |
| staging | `zkGroupServerPublicParams` | 673 | `ABSY21VckQcb…` |
| staging | `genericServerPublicParams` | 225 | `AHILOIrFPXX9…` |
| staging | `backupServerPublicParams` | 225 | `AHYrGb9IfugA…` |

The full values, and each blob split into its components, are in the
vectors file (`server_public_params.signal_cli_config`). They are copied
verbatim from C1 and are interop data. All six parse in the reference
jar.

### 8.2 `ServerPublicParams` layout (673 bytes)

| Offset | Len | Component | Content | Used by a client for |
|--------|-----|-----------|---------|----------------------|
| 0 | 1 | reserved | `0x00` | — |
| 1 | 64 | auth credentials public key | 2 points (`C_W`, `I`) | no longer used |
| 65 | 64 | profile-key credentials public key | 2 points | no longer used |
| 129 | 32 | `sig_public_key` | 1 point | **notary signature verification (§7)** |
| 161 | 64 | receipt credentials public key | 2 points | receipt credentials |
| 225 | 64 | PNI credentials public key | 2 points | no longer used |
| 289 | 64 | expiring profile-key credentials public key | 2 points | profile-key credentials (credentials spec) |
| 353 | 64 | auth-credential-with-PNI public key (old format) | 2 points | no longer used |
| 417 | 224 | generic credential public key | 7 points: `C_W`, then `I_1 … I_6` | zkcredential-based credentials, such as group auth (credentials spec) |
| 641 | 32 | endorsement public key | 1 point | group send endorsements (credentials spec) |

The "no longer used" slots must still be present and parse (valid
points), because the layout is positional. A client parses the blob once
using §2: reserved byte `0x00`, then every point must decode. libsignal
tolerates trailing bytes (§2).

### 8.3 Generic and backup params (225 bytes each)

Both use the same layout, **GenericServerPublicParams**: reserved
`0x00`, then a generic credential public key of 7 points (`C_W`,
`I_1 … I_6`; 224 bytes). signal-cli uses them for credentials outside
this file's scope (call links, backups; see the credentials spec).

### 8.4 Verification of the params themselves

None. The params are trusted because they are compiled into the client.
No signature or hash check is applied.

## 9. Constants

| Name | Value |
|------|-------|
| Group master key length | 32 |
| GroupSecretParams length | 289 |
| GroupPublicParams length | 97 |
| Group identifier length | 32 |
| UuidCiphertext / ProfileKeyCiphertext length | 65 |
| Blob overhead (padded, `n = 0`) | 33 bytes = 4 header + 16 tag + 12 nonce + 1 reserved |
| Notary signature length | 64 |
| ServerPublicParams length | 673 |
| GenericServerPublicParams length | 225 |
| Randomness input length (all Sho "Random_" labels) | 32 |

Sho labels used in this file, each byte-exact ASCII:

- `Signal_ZKGroup_20200424_Random_GroupSecretParams_Generate`
- `Signal_ZKGroup_20200424_GroupMasterKey_GroupSecretParams_DeriveFromMasterKey`
- `Signal_ZKGroup_20200424_Constant_UidEncryption_SystemParams_Generate`
- `Signal_ZKGroup_20200424_Constant_ProfileKeyEncryption_SystemParams_Generate`
- `Signal_ZKGroup_20200424_UID_CalcM1`
- `Signal_ZKGroup_20200424_ProfileKeyAndUid_ProfileKey_CalcM3` (from [PROF])
- `Signal_ZKGroup_20200424_Random_GroupSecretParams_EncryptBlob`
- `POKSHO_Ristretto_SHOHMACSHA256`

The encryption domains also carry IDs (`Signal_ZKGroup_20230419_UidEncryption`,
`Signal_ZKGroup_20231011_ProfileKeyEncryption`). They do not enter any
computation in this file. They appear only in the proofs (credentials
spec).

## 10. Implementation checklist

- Derive all six values of §3.2 from one Sho state, in order.
- M1 hashes the **binary form**: 16 bytes for an ACI, `0x01 ‖ UUID` for
  a PNI. M2 encodes only the UUID.
- Decryption of service IDs tries ACI before PNI. Exactly one can match.
- Profile-key decryption needs the owner's ACI and must count matches
  across all slots, including the duplicated identity slots.
- Blob decryption ignores the trailing reserved byte and the padding
  contents. It checks lengths in the order: total ≥ 13, inner ciphertext
  ≥ 16, decrypted ≥ 4, padding ≤ remaining.
- Notary signatures cover the raw actions bytes and need canonical
  scalars.

## 11. Test vectors

`testdata/vectors/zkgroup_groups.json`. Every entry has a `provenance`
(`upstream`, `jar`, `jar+bouncycastle`, `derived`, `config`, explained in
its `source` block).

| Section | Count | Provenance | Content |
|---------|-------|------------|---------|
| `constants` | — | upstream | test arrays, both system-generator pairs, base point, `SQRT_ID` (derived) |
| `lizard` | 4 | upstream (D1) | 16 bytes → point |
| `identity_preimage_slots` | 8 slots | derived | §4.4 identity multiset |
| `group_params` | 5 master keys | jar | full 289/97-byte params, group ID, every field split out |
| `generate_from_randomness` | 3 | jar | randomness → master key (§3.1) |
| `uuid_ciphertexts` | 20 | jar (+derived M1, M2) | 2 groups × 5 UUIDs × {ACI, PNI}, with decryption result |
| `uuid_ciphertext_errors` | 8 | jar | wrong group, reserved byte, lengths, invalid point, `E1 = B`, `E1 = identity`, swapped halves |
| `profile_key_ciphertexts` | 34 | jar (+derived M3, M4, match count) | 18 decrypt successfully, 16 fail to decrypt (identity M4, count 2), and all fail with a wrong ACI. Includes the `bced…` identity-slot key and one of its variants, which succeed |
| `domain_level_upstream` | 2 | upstream (+derived) | libsignal's 64-byte ciphertext unit vectors with their Sho-derived keys |
| `blob.aes_gcm_siv_rfc8452` | 2 | upstream | raw AES-256-GCM-SIV (RFC 8452) |
| `blob.upstream` | 5 | upstream | unpadded Rust vector, padded `n=0` vector of the bindings, `n=257` decrypt-only, two `"secret team"` vectors (`n = 0, 8`) |
| `blob.generated` | 40 | jar | 5 plaintexts × `n ∈ {0,1,8,257}` × 2 randomness values |
| `blob.errors` | 8 | jar | non-zero reserved byte (still decrypts), tamper, short lengths, wrong group |
| `blob.crafted_padding` | 9 | jar+bouncycastle | hand-built inner plaintexts testing §6.2 rules |
| `server_public_params` | 6 + 1 + 3 | config / jar | signal-cli staging and production blobs with components, a test-generated params, parse edge cases |
| `notary_signatures` | 6 + 4 results | upstream / jar / derived | libsignal's server-signature, poksho and zkgroup signature vectors (public keys derived or from jar), empty and 100-byte messages, altered message, jar verification outcomes |
| `serialization_errors` | 6 | jar | reserved byte, non-canonical scalar, wrong lengths, unchecked group-ID tamper |

libsignal publishes **no** fixed vectors for group params, group
identifiers or service-ID/profile-key ciphertexts at the API level
(the Java test that would check a group identifier is commented out).
Those all come from the jar. The upstream ciphertext vectors exist only
at the domain level (`domain_level_upstream`), with keys from test Sho
labels instead of a master key. One upstream presentation vector embeds
the ACI ciphertext of TEST_UUID under TEST_ARRAY_32_1 (`765de9df…`), and
it matches the jar's `uuid_ciphertexts` entry.

## Gaps

- The normative definition of the preimage slots for non-identity
  points (§4.4: "exactly the distinct non-negative preimages") is
  supported by D1's README argument and by the spec agent's checks (300
  random points; agreement with every jar outcome), not by a proof. The
  identity multiset was computed with the informative method. The jar
  confirmed its observable effect (keys with masked form `0` or
  `SQRT_ID` fail, `bced…` succeeds).
- Intermediates (M1, M2, M3, M4, identity slots, derived keys) come from
  the spec agent's scratch implementation. libsignal does not expose
  them, but every final output they feed was matched against the jar or
  upstream data.
- The trailing-byte leniency (§2) was observed on the variable-length
  types only. For fixed-length types, the Java layer's length check hides
  the deserializer's behavior. Other bindings were not examined.
- Signing (§7.3) is described only in outline. Its exact nonce
  derivation is server-side and not needed by a client.
- The usage of blob encryption by specific group fields, and the
  handling of a rejected signed group change, are left to the
  groups-service spec.
- No server-signed group change from a real server was available, so
  the signature vectors are libsignal test vectors and jar-generated
  ones.
