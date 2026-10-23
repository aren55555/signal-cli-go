# zkgroup: profile-key version and profile-key commitment (libsignal v0.99.1)

Spec role: prose only; no code. This file closes the gap named in
`spec/service/profile.md` §2.3. It covers exactly two values that
signal-cli 0.14.7 needs for a fixed (profile key, ACI) pair:

- the **profile-key version**: 64 lowercase hex characters, used in
  `GET /v1/profile/{aci}/{version}` and as the `version` member of
  `PUT /v1/profile`;
- the **profile-key commitment**: 97 bytes, sent base64-encoded as the
  `commitment` member of `PUT /v1/profile`.

Credentials, credential requests, presentations, group features and
`ProfileKey` generation from randomness are out of scope.

Both values are deterministic. Neither involves randomness, so a correct
implementation reproduces the test vectors byte for byte.

## Sources

| Ref | What | URL @ version |
|-----|------|---------------|
| Z1 | libsignal zkgroup | https://github.com/signalapp/libsignal @ tag v0.99.1 = commit 97801d22dcf9f5bf714f7b8fa3212cdc973ae1c8: `rust/zkgroup/src/api/profiles/{profile_key,profile_key_version,profile_key_commitment}.rs`, `rust/zkgroup/src/crypto/{profile_key_struct,profile_key_commitment}.rs`, `rust/zkgroup/src/common/{sho,constants,simple_types,serialization}.rs`, `rust/zkgroup/Cargo.toml`, `Cargo.toml`, `Cargo.lock`; tests searched for fixed values: `rust/zkgroup/tests/integration_tests.rs`, `java/client/src/test/java/org/signal/libsignal/zkgroup/**`, `node/ts/test/**`, `swift/Tests/**`; Java API `java/shared/java/org/signal/libsignal/zkgroup/profiles/{ProfileKey,ProfileKeyVersion,ProfileKeyCommitment}.java` |
| Z2 | poksho (in the libsignal tree, same commit) | `rust/poksho/src/{shohmacsha256,shoapi,lib}.rs` |
| Z3 | curve25519-dalek 5.0.0, the version zkgroup resolves (crates.io; Z1 `Cargo.lock`), with feature `lizard` | https://github.com/dalek-cryptography/curve25519-dalek @ tag curve25519-5.0.0 = commit 07bef73ff85998a206cd2cea7f2605c801d0d1c9: `curve25519-dalek/src/ristretto.rs`, `src/ristretto/elligator.rs`, `src/lizard/lizard_ristretto.rs`, `src/field.rs`, `Cargo.toml` |
| Z4 | Reference distribution, offline | `jshell` over `/work/third_party/signal-cli-0.14.7/lib/*` (libsignal-client 0.99.1), no network: version and commitment for the nine cases in the vector file |
| S1 | ristretto255 | RFC 9496, https://www.rfc-editor.org/rfc/rfc9496 |
| S2 | HMAC | RFC 2104; SHA-256 per FIPS 180-4 |
| S3 | UUID byte layout | RFC 9562 (formerly RFC 4122) |

Vectors: `testdata/vectors/zkgroup_profile.json`.

## 1. Notation

- `‖` is byte concatenation. `0xNN` is one byte.
- `HMAC(k, m)` is HMAC-SHA256 (S2) with key `k` and message `m`. Its
  output is 32 bytes.
- `be64(n)` is the 8-byte big-endian encoding of the unsigned integer `n`.
- `p = 2^255 − 19`. `ℓ = 2^252 + 27742317777372353535851937790883648493`
  is the order of the ristretto255 group.
- "Encoding" a point means the 32-byte ristretto255 encoding of RFC 9496
  §4.3.2. Scalars are written as 32-byte little-endian integers in
  `[0, ℓ)`.
- All label strings below are ASCII, have no terminator, and are given
  byte-exact between backticks.

## 2. The Sho construction (HMAC-SHA256 variant)

zkgroup derives every value in this file from one keyed hash
construction that libsignal calls "Sho" (Z1 `common/sho.rs`, which wraps
Z2's `ShoHmacSha256`). It is built **only on HMAC-SHA256**. No other
hash, and no SHA-256 without HMAC, is involved. zkgroup does not use the
SHA-256-only Sho variant anywhere in scope.

### 2.1 State

The state is a 32-byte **chaining value** `cv`.

### 2.2 Operations

**Create with a label.** Start with `cv` = 32 zero bytes. Then absorb the
label exactly as in "absorb and ratchet" below. So after creation,
`cv = HMAC(0^32, label ‖ 0x00)`.

**Absorb and ratchet `data`** (any length, including empty):

`cv ← HMAC(cv, data ‖ 0x00)`

- The trailing `0x00` is always appended.
- Absorbing an empty string is **not** a no-op: it still replaces `cv`
  with `HMAC(cv, 0x00)`. The system parameters (§4.1) depend on this.
- Every absorb used in this file is a single call with the full input.
  Upstream can absorb several pieces before one ratchet, which is the
  same as absorbing their concatenation. Nothing in scope does this.

**Squeeze and ratchet `n` bytes** (`n ≥ 1`):

1. For `i = 0, 1, 2, …` while `32·i < n`, compute the block
   `B_i = HMAC(cv, be64(i) ‖ 0x01)`. Here `cv` is the value **before**
   this squeeze.
2. The output is the first `n` bytes of `B_0 ‖ B_1 ‖ …`.
3. Then set `cv ← HMAC(cv_old, be64(n) ‖ 0x02)`, where `cv_old` is the
   pre-squeeze value and `n` is the requested length.

Consecutive squeezes therefore give different outputs. The second
squeeze uses the ratcheted `cv`.

### 2.3 The one-shot form

Every derivation below starts with "Sho(label, data)": create with
`label`, then absorb and ratchet `data` once. Squeezes follow.

### 2.4 Mapping squeezed bytes to group values

zkgroup turns squeezed bytes into group values in three ways:

| Name used here | Squeeze | Mapping |
|----------------|---------|---------|
| **point** | 64 bytes | The RFC 9496 §4.3.4 one-way map ("element derivation") applied to the 64 bytes. Each 32-byte half has bit 255 cleared, is read as a little-endian integer reduced mod `p`, and goes through MAP. The two resulting points are added. |
| **single-Elligator point** | 32 bytes | Clear bit 255 (the `0x80` bit of the last byte). Read the result as a little-endian integer and reduce it mod `p`. Values in `[p, 2^255)` are accepted and reduced, not rejected. Apply the RFC 9496 MAP function **once** (one half of the one-way map, with no addition). |
| **scalar** | 64 bytes | Read all 64 bytes as a little-endian integer (512 bits) and reduce it mod `ℓ`. |

The same single-Elligator mapping also turns the profile key itself into
a point (§3.2). In that case no Sho is involved.

The MAP of the field element 0 is the identity point, which encodes as 32
zero bytes. The vectors include such a case.

**Caution for implementers:** the RFC 9496 constants `SQRT_AD_MINUS_ONE`
and `INVSQRT_A_MINUS_D` must be used **exactly as the RFC lists them**.
In particular, the RFC's `SQRT_AD_MINUS_ONE` is the root whose
lowest bit is 1 (a "negative" field element in RFC 9496 terms).
Recomputing it as the non-negative square root gives wrong points. The RFC 9496 Appendix A
vectors in `testdata/vectors/rfc9496_ristretto255.json` (the
`from_uniform_bytes` set) catch this.

## 3. Common inputs

### 3.1 Byte encodings

- **Profile key** `K`: the raw 32 bytes. No masking is applied, except
  for M4 (§3.2).
- **ACI** `U`: the 16 bytes of the UUID in RFC 9562 order. This is the
  order the canonical string shows, read left to right, most significant
  byte first (big-endian). For example, `00000000-0000-4000-8000-000000000001`
  becomes `00 00 00 00 00 00 40 00 80 00 00 00 00 00 00 01`. There is no
  service-ID prefix byte: ACIs carry none, and only ACIs are valid here.
- **Combined input** `KU = K ‖ U`: 48 bytes. Every derivation that "uses
  the profile key and ACI" absorbs exactly these 48 bytes.

### 3.2 The profile key as two points (M3, M4)

The commitment commits to two points derived from (K, U):

- **M3** = the single-Elligator point squeezed (32 bytes) from
  Sho(`Signal_ZKGroup_20200424_ProfileKeyAndUid_ProfileKey_CalcM3`, `KU`).
- **M4** = the single-Elligator mapping (§2.4) applied directly to a
  masked copy of `K`:
  - byte 0 ANDed with `0xFE` (clear the lowest bit);
  - byte 31 ANDed with `0x3F` (clear the top two bits);
  - all other bytes unchanged.

  M4 therefore ignores those three bits of K. Two keys that differ only
  in them have the same M4, but different M3, j3, version and commitment.
  The vectors include such a pair.

## 4. Profile-key commitment

### 4.1 System parameters

The commitment uses three fixed generator points `G_j1`, `G_j2`, `G_j3`.
They are defined as:

- Sho(`Signal_ZKGroup_20200424_Constant_ProfileKeyCommitment_SystemParams_Generate`,
  empty data). The empty data is still absorbed (§2.2).
- Then three consecutive **point** squeezes (64 bytes each), in this
  order: `G_j1`, `G_j2`, `G_j3`.

At run time libsignal does not derive these. It loads a hardcoded 96-byte
constant, the three encodings concatenated in the order `G_j1 ‖ G_j2 ‖ G_j3`,
and its unit test asserts that the constant equals the derivation. An
implementation may either hardcode or derive them. The constant (data,
needed for interop) is:

- `G_j1` = `a8ca0bbd1148c466725860640ac53d2772b14eeae0170a38c62c7b3dd29c3e4a`
- `G_j2` = `14b9462d948f059450799f4cc2a06e55dec807735670b94a5ce80f59f1950861`
- `G_j3` = `b0c0f7b91f6ef9c7556093d8930a86bd36188cec740554657d92dcd86aad251c`

### 4.2 The commitment nonce j3

`j3` = the **scalar** squeezed (64 bytes) from
Sho(`Signal_ZKGroup_20200424_ProfileKeyAndUid_ProfileKeyCommitment_Calcj3`, `KU`).

It is derived from the inputs, not drawn at random. This is why the
commitment is deterministic.

### 4.3 The commitment

The commitment is three points, computed in the ristretto255 group:

- `J1 = j3·G_j1 + M3`
- `J2 = j3·G_j2 + M4`
- `J3 = j3·G_j3`

### 4.4 Serialization (97 bytes)

| Offset | Length | Content |
|--------|--------|---------|
| 0 | 1 | reserved byte, always `0x00` |
| 1 | 32 | encoding of `J1` |
| 33 | 32 | encoding of `J2` |
| 65 | 32 | encoding of `J3` |

- There are no length prefixes or other framing. zkgroup's serializer
  writes a fixed-size point as its 32 raw encoding bytes.
- When parsing, libsignal rejects a first byte other than `0x00`, invalid
  point encodings, and any length other than 97. signal-cli only produces
  this value and never parses one.
- In `PUT /v1/profile`, the `commitment` member is these 97 bytes in
  standard base64 with padding: 132 characters (`profile.md` §2.2).

## 5. Profile-key version

1. Compute Sho(`Signal_ZKGroup_20200424_ProfileKeyAndUid_ProfileKey_GetProfileKeyVersion`, `KU`).
2. Squeeze **32** bytes.
3. Hex-encode the 32 bytes as **lowercase** ASCII, two characters per
   byte, most significant nibble first. This gives 64 characters.

This 64-character string is the version itself; no other form is sent:

- the `{version}` path segment of `GET /v1/profile/{aci}/{version}`, as
  is;
- the string value of the `version` member of `PUT /v1/profile`.

The Java API (`ProfileKeyVersion.serialize()`) returns exactly this
string.

The version identifies a (profile key, ACI) pair. It does not change when
the profile content changes.

## 6. Test vectors

`testdata/vectors/zkgroup_profile.json` contains:

- **`sho_hmac_sha256`**: three Sho vectors. Their outputs are libsignal's
  own poksho unit-test data. For the first vector, the spec agent also
  added the chaining values after each step, so the absorb, ratchet and
  squeeze steps can each be checked on their own.
- **`system_params`**: the chaining value after the label and after the
  empty absorb, the three 64-byte squeeze outputs, and the three points.
  The concatenation equals the upstream hardcoded constant.
- **`cases`**: nine (profile key, ACI) pairs, each with:
  - the version and the full 97-byte commitment, both produced by the
    reference jar (Z4);
  - intermediates: the 48-byte `KU`, M3's 32-byte squeeze and point, the
    masked key and M4, j3's 64-byte squeeze and the reduced scalar.

  The first case is the reference value in `profile.md` §2.3. Other cases
  cover libsignal's test constants, all-`0xFF` inputs (exercising the
  masking), a pair that differs only in masked bits, and a key whose
  masked form is zero (M4 = identity).

libsignal itself has no fixed vectors for either value. Its tests only
check determinism. See the `source` block of the JSON for the provenance
of each field.

## Gaps

- None known for the two values in scope.
- The intermediates (M3, M4, j3) are not exposed by libsignal's API. The
  spec agent computed them with a throwaway implementation of this
  document. That implementation reproduced the jar's version and
  commitment exactly for all nine cases, but the intermediates
  themselves were not checked against libsignal internals.
- Whether the server validates the commitment at `PUT /v1/profile`
  beyond its length and encoding was not examined; it is a server-side
  matter.
