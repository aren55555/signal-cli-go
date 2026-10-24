# zkgroup: credentials, proofs and group send endorsements (libsignal v0.99.1)

Spec role: prose only; no code. This file covers the credential and proof
half of libsignal's zkgroup, as a groups v2 **client** (signal-cli 0.14.7)
uses it:

- the two proof systems, poksho (Schnorr-style proofs of linear relations)
  and zkcredential (keyed-verification anonymous credentials);
- `AuthCredentialWithPni` (zkc variant): receiving and verifying the issued
  credential, and creating the presentation sent to the group server;
- `ExpiringProfileKeyCredential`: request context, request, response,
  verification, expiry rules and presentation;
- group send endorsements: receiving, combining, removing, tokens and
  full tokens;
- verifying server notary signatures (group changes);
- exactly how much randomness each operation consumes and how it is
  expanded.

Companion specs. This file does not repeat them:

- `spec/crypto/zkgroup-profile.md` (**ZP**): Sho (HMAC-SHA256),
  ristretto255 notation, "point", "single-Elligator point" and "scalar"
  squeezes, the profile key points M3/M4, the commitment (J1, J2, J3), the
  nonce j3, the generators G_j1..G_j3 and the profile-key version.
- `spec/crypto/zkgroup-groups.md` (**ZG**, written in parallel):
  `GroupSecretParams`/`GroupPublicParams` and their derivation from the
  master key, `UuidCiphertext`, `ProfileKeyCiphertext`, the uid and
  profile-key encryption generators, blob encryption, and the full contents
  of `ServerPublicParams`. Where this file needs one of those values it
  says which field, and gives byte offsets only to help cross-checking.
- The JSON and protobuf envelopes (the `/v1/certificate/auth/group` JSON,
  the `Authorization` header, protobuf field numbers) belong to the groups
  service and profile specs. This file covers only the binary zkgroup
  values inside them.

Everything in sections 2 to 10 was checked byte for byte against the
reference jar (see section 12). A correct implementation reproduces every
vector exactly, including all randomized outputs, because randomness is an
explicit input.

## Sources

| Ref | What | URL @ version |
|-----|------|---------------|
| Z1 | libsignal zkgroup | https://github.com/signalapp/libsignal @ tag v0.99.1 = commit 97801d22dcf9f5bf714f7b8fa3212cdc973ae1c8: `rust/zkgroup/src/api/auth/{auth_credential_with_pni.rs, auth_credential_with_pni/zkc.rs, auth_credential_presentation.rs}`, `rust/zkgroup/src/api/profiles/{expiring_profile_key_credential, expiring_profile_key_credential_response, profile_key_credential_presentation, profile_key_credential_request, profile_key_credential_request_context}.rs`, `rust/zkgroup/src/api/groups/{group_send_endorsement, group_params}.rs`, `rust/zkgroup/src/api/{server_params, endorsement_expiration}.rs`, `rust/zkgroup/src/crypto/{credentials, proofs, profile_key_credential_request, profile_key_commitment, profile_key_encryption, profile_key_struct, uid_encryption, uid_struct, timestamp_struct, signature}.rs`, `rust/zkgroup/src/common/{constants, serialization, simple_types, sho, array_utils}.rs` |
| Z2 | zkcredential (same tree) | `rust/zkcredential/src/{lib, sho, credentials, attributes, issuance, presentation, endorsements}.rs` |
| Z3 | poksho (same tree) | `rust/poksho/src/{statement, proof, args, sign, shoapi, shohmacsha256, shosha256}.rs` |
| Z4 | libsignal bridge and Java API (same tree) | `rust/bridge/shared/src/zkgroup.rs`; `rust/core/src/address.rs`; `java/shared/java/org/signal/libsignal/zkgroup/**` (`auth/ClientZkAuthOperations`, `profiles/ClientZkProfileOperations`, `groupsend/*`, `ServerPublicParams`, `ServerSecretParams`, server operation classes) |
| Z5 | libsignal tests searched for fixed values (same tree) | `rust/zkgroup/tests/integration_tests.rs` (fixed vectors used), `rust/zkgroup/tests/group_send_endorsements.rs`, `java/client/src/test/java/org/signal/libsignal/zkgroup/**`, `node/ts/test/ZKGroup-test.ts`, `swift/Tests/LibSignalClientTests/ZKGroupTests.swift`; unit tests inside Z2/Z3 |
| D1 | curve25519-dalek 5.0.0 with `lizard` | https://github.com/dalek-cryptography/curve25519-dalek @ tag curve25519-5.0.0 = commit 07bef73ff85998a206cd2cea7f2605c801d0d1c9: `curve25519-dalek/src/lizard/lizard_ristretto.rs`, `src/ristretto.rs` (batch double-and-compress) |
| C1 | signal-cli | https://github.com/AsamK/signal-cli @ v0.14.7 = commit b01b6b370dc063599a1a2b9fde0f5ff4e2d78fe8: `lib/src/main/java/org/asamk/signal/manager/helper/{GroupV2Helper, GroupHelper, SendHelper, ProfileHelper}.java`, `storage/groups/GroupStore.java` |
| C2 | libsignal-service-java fork | https://github.com/Turasa/libsignal-service-java @ v2.15.3_unofficial_151 = commit 66956cb4a0394d50cee3590209baf58312084a44: `lib/libsignal-service/src/main/java/org/whispersystems/signalservice/api/groupsv2/{GroupsV2Api.java, GroupsV2ApiHelper.kt, GroupsV2Operations.java, GroupsV2AuthorizationString.java, GroupSendEndorsements.kt, ReceivedGroupSendEndorsements.kt}`, `api/crypto/SealedSenderAccess.kt`, `api/profiles/{ProfileApi.kt, SignalServiceProfile.java}`, `core/util-jvm/.../ExpiringProfileCredentialUtil.java`, `core/util-jvm/.../Hex.java` |
| J1 | Reference distribution, offline | `jshell` over `/work/third_party/signal-cli-0.14.7/lib/*.jar` (libsignal-client 0.99.1 with its native library), no network; produced every `reference_jar` vector |
| S1 | ristretto255 | RFC 9496 |
| S2 | HMAC-SHA256, SHA-256 | RFC 2104, FIPS 180-4 |
| P1 | Chase, Perrin, Zaverucha, "The Signal Private Group System and Anonymous Credentials Supporting Efficient Verifiable Encryption" | https://eprint.iacr.org/2019/1416 (background only; this spec is authoritative for the byte level) |

Vectors: `testdata/vectors/zkgroup_credentials.json`.

## 1. Notation and conventions

ZP §1 and §2 apply: `‖`, `be64`, `ℓ`, point encodings, little-endian
scalars, `Sho(label, data)`, and the three squeeze mappings ("point",
"single-Elligator point", "scalar"). In addition:

- `G` is the ristretto255 base point (encoding
  `e2f2ae0a6abc4e71a884a961c500515f58e30b6aa582dd8db6a65945e08d2d76`).
- `le64(n)` is the 8-byte **little-endian** encoding of `n`.
- `O` is the identity point (encodes as 32 zero bytes).
- `x⁻¹` for a scalar is its inverse mod `ℓ`.
- `enc(P)` is the 32-byte encoding of point P. `dbl(P)` is `enc(P + P)`.
- "Wide scalar from 64 bytes" means the scalar mapping of ZP §2.4: read
  64 bytes little-endian and reduce mod ℓ.

### 1.1 Sho state machine, precisely

ZP §2 describes one-call absorbs. The credential code also absorbs
several pieces before a single ratchet, and sometimes ratchets twice in a
row. The exact rule:

1. An **absorb** appends bytes to a pending buffer. The first absorb after
   a ratchet or squeeze starts a new buffer, even if it appends zero bytes.
2. A **ratchet** does nothing if no absorb has happened since the last
   ratchet or squeeze. Otherwise it sets `cv ← HMAC(cv, buffer ‖ 0x00)`
   and clears the buffer.
3. A **squeeze** requires that nothing is pending (always true below). It
   works as in ZP §2.2.
4. "Absorb-and-ratchet x" is an absorb of x followed by a ratchet. It
   always changes `cv`, even for empty x. A further ratchet right after it
   is a no-op.
5. A squeeze of **0 bytes** produces no output but still replaces `cv`
   with `HMAC(cv, be64(0) ‖ 0x02)`. This happens for one-member endorsement
   responses (§9.4).

`Sho(label)` means "create with label" (one absorb-and-ratchet of the
label). `Sho(label, data)` additionally absorbs-and-ratchets `data`.

### 1.2 ShoSha256 (only for zkcredential system parameters)

zkcredential's fixed generators (§4.1) use a second Sho variant built on
plain SHA-256 instead of HMAC. Its state is also a 32-byte `cv`, starting
at 32 zero bytes, with the same mode rules as §1.1:

- **Ratchet** after absorbing `data`:
  `cv ← SHA256(SHA256(0x00^64 ‖ cv_old ‖ data))`, where `data` is
  everything absorbed since the last ratchet or squeeze. Unlike the HMAC
  variant there is **no** trailing `0x00`, and the hash is applied twice.
- **Squeeze n bytes**: block `i` is `SHA256(0x00^63 ‖ 0x01 ‖ cv ‖ be64(i))`.
  The output is the first n bytes of the blocks. Then
  `cv ← SHA256(0x00^63 ‖ 0x02 ‖ cv_old ‖ be64(n))`.
- Creating with a label is an absorb-and-ratchet of the label.

The three upstream ShoSha256 test vectors are in the vector file
(`sho_sha256`).

### 1.3 Serialization conventions ("zkgroup encoding")

Every zkgroup value in this file is a fixed sequence of fields, encoded
as follows, with no other framing:

| Field type | Encoding |
|------------|----------|
| version byte or reserved byte | 1 byte with a fixed value; parsing rejects any other value |
| point | 32-byte encoding. Parsing must decode it (RFC 9496 decode) and rejects invalid encodings. The exception is §9.2, where endorsement points are stored "compressed" and are not checked at parse time. |
| scalar | 32 bytes little-endian. Parsing rejects values ≥ ℓ (non-canonical). |
| fixed byte array (UUID 16, profile key 32, ...) | raw bytes |
| u64 (timestamps) | `le64` |
| variable-length byte string or list (poksho proofs, point lists, token bytes) | `le64(count)` followed by the elements; for bytes the count is the byte length, for points the number of points |

Parsing rejects trailing bytes. A poksho proof parsed this way must
additionally satisfy §3.6.

### 1.4 Service IDs and UUIDs

- A UUID's 16 bytes are in RFC 9562 order (ZP §3.1).
- The **service-ID binary** form (`sid`) is the 16 UUID bytes for an ACI,
  and `0x01 ‖ 16 UUID bytes` (17 bytes) for a PNI.
- The bridge APIs take a 17-byte fixed-width form (`0x00` or `0x01`,
  then the UUID); this only matters for API plumbing, not for any hash.

### 1.5 Timestamps

All zkgroup timestamps are whole **seconds** since the Unix epoch, held as
u64. "Day-aligned" means divisible by 86400.

## 2. Labels used in this file

Every label is ASCII, with no terminator, absorbed exactly as written.

| Label | Variant | Used for |
|-------|---------|----------|
| `POKSHO_Ristretto_SHOHMACSHA256` | HMAC | poksho transcript (§3) |
| `Signal_ZKCredential_ConstantSystemParams_generate_20230410` | SHA256 | zkcredential generators (§4.1) |
| `Signal_ZKGroup_20200424_Constant_Credentials_SystemParams_Generate` | HMAC | zkgroup credential generators (§4.2) |
| `Signal_ZKGroup_20200424_UID_CalcM1` | HMAC | uid point M1 (§5.1) |
| `Signal_ZKGroup_20220524_Timestamp_Calc_m` | HMAC | expiration scalar m5 (§5.3) |
| `20240222_Signal_AuthCredentialZkc` | HMAC | auth credential public-attribute hash (§7) |
| `Signal_ZKCredential_Issuance_20230410` | HMAC | zkcredential issuance randomness (server, §6.4) |
| `Signal_ZKCredential_Presentation_20230410` | HMAC | zkcredential presentation randomness (§6.6) |
| `Signal_ZKCredential_CredentialPrivateKey_generate_20230410` | HMAC | zkcredential key generation (server, §11) |
| `Signal_ZKGroup_20200424_Random_ServerPublicParams_CreateProfileKeyCredentialRequestContext` | HMAC | request context (§8.1) |
| `Signal_ZKGroup_20220508_Random_ServerSecretParams_IssueExpiringProfileKeyCredential` | HMAC | profile credential issuance (server, §8.3) |
| `Signal_ZKGroup_20220508_Random_ServerPublicParams_CreateExpiringProfileKeyCredentialPresentation` | HMAC | profile presentation (§8.5) |
| `20240215_Signal_GroupSendEndorsement` | HMAC | endorsement tag info (§9.3) |
| `Signal_ZKCredential_Endorsements_EndorsementResponse_ProofWeights_20240207` | HMAC | endorsement batch weights (§9.4) |
| `Signal_ZKCredential_Endorsements_ServerRootKeyPair_generate_20240207` | HMAC | endorsement root key (server, §11) |
| `Signal_ZKGroup_20200424_Random_ServerSecretParams_Generate` | HMAC | server key generation (server, §11) |
| `Signal_ZKGroup_20200424_Random_ServerSecretParams_Sign` | HMAC | notary signing randomness (server, §10) |

Domain identifiers used by zkcredential for encryption keys (they only
serve to recognize "the same key" inside one proof; they are never hashed):
`Signal_ZKGroup_20230419_UidEncryption` (uid key) and
`Signal_ZKGroup_20231011_ProfileKeyEncryption` (profile-key key).

## 3. poksho: proofs of knowledge of linear relations

poksho proves knowledge of secret scalars satisfying a fixed system of
equations over ristretto255 points. It uses Fiat-Shamir with a synthetic
nonce, both through Sho (HMAC variant).

### 3.1 Statements

A **statement** is an ordered list of 1 to 255 equations. Each equation
has the form

  `LHS = s_1·P_1 + s_2·P_2 + … + s_k·P_k` (1 ≤ k ≤ 255)

where LHS and every `P_j` are named **points** and every `s_j` is a named
**scalar** (a secret, the "witness"). Names are only bookkeeping. What
matters on the wire is the **index** each name receives:

- Point index 0 is reserved for the base point `G`. The name `G` always
  means index 0, including when `G` appears as a LHS.
- Other points get indices 1, 2, 3, … in order of **first appearance**.
  The scan goes equation by equation; inside an equation it takes the LHS
  first, then for each term the scalar, then the point.
- Scalars get indices 0, 1, 2, … in order of first appearance, in the same
  scan.
- A name that appears again reuses its index. Two different equations may
  share a LHS name; the identity point is used that way in §8.5.

At most 256 points (including G) and 256 scalars are allowed.

### 3.2 Statement description D

`D` is the byte string:

- 1 byte: number of equations;
- for each equation in order: 1 byte LHS point index, 1 byte number of
  terms k, then for each term 1 byte scalar index followed by 1 byte point
  index.

Upstream examples (vector file, `poksho.statement_encodings`):
`A = a·G` is `01 01 01 00 00`; `A = a·G + b·H` is `01 01 02 00 00 01 02`.

### 3.3 Point list A

`A` is the concatenation of the encodings of **all** points in index
order, **including `enc(G)` first** (index 0). The prover and verifier
supply concrete points for every name except G. The count must match
exactly: no missing names, no extra names.

### 3.4 Proving

Inputs: the statement, a value for every scalar, a value for every point,
a message `M` (every use in this file has an empty message), and exactly
32 bytes of randomness `Z`.

1. `sho` = Sho(`POKSHO_Ristretto_SHOHMACSHA256`). Absorb `D`, then absorb
   `A` (as one or many pieces; they are concatenated), then ratchet.
2. **Synthetic nonce.** Copy the state into `sho2`. Absorb `Z`, then the
   32-byte encodings of all witness scalars in scalar-index order, then
   ratchet. Absorb-and-ratchet `M` (this ratchets even if M is empty).
   Squeeze `64·n` bytes, n = number of scalars. Nonce `r_i` is the wide
   scalar of bytes `64i … 64i+63`.
3. **Commitment.** For each equation e, `R_e = Σ r_{s_j}·P_j` over its
   terms.
4. **Challenge.** Back in `sho` (the state after step 1): absorb `enc(R_e)`
   for every equation in order, then absorb `M`, then ratchet. Squeeze 64
   bytes; `c` = wide scalar.
5. **Response.** `s_i = r_i + c·w_i` (mod ℓ) for each scalar i, where `w_i`
   is the witness.

libsignal verifies the proof before returning it. That does not change the
output.

### 3.5 Proof encoding

`enc_scalar(c) ‖ enc_scalar(s_0) ‖ … ‖ enc_scalar(s_{n−1})`: 32·(n+1)
bytes. The commitments are not sent.

### 3.6 Verification

1. Reject unless the length is a non-zero multiple of 32, every 32-byte
   chunk is a canonical scalar, there is at least one response and at
   most 256, and the number of responses equals the number of scalars in
   the statement.
2. Rebuild `D` and `A` from the statement and the verifier's points, and
   compute step 1 of §3.4.
3. For each equation e, `R_e = Σ s_{s_j}·P_j − c·LHS_e`.
4. Compute step 4 of §3.4 with these `R_e`. Accept if and only if the
   squeezed challenge equals `c`.

### 3.7 Signatures

A poksho signature is a proof for the one-equation statement
`public_key = private_key·G` (D = `01 01 01 00 00`) over a message. It is
exactly 64 bytes (challenge plus one response). The upstream unit-test
signature is in the vector file (`poksho.signature`); the upstream
multi-equation proof is in `poksho.complex_statement`.

## 4. Fixed generators

### 4.1 zkcredential system parameters

ShoSha256 (§1.2) created with `Signal_ZKCredential_ConstantSystemParams_generate_20230410`
(no further absorb). Then 13 consecutive **point** squeezes (64 bytes each,
ZP §2.4), in this order:

`G_w, G_w', G_x0, G_x1, G_V, G_z, G_y0, G_y1, G_y2, G_y3, G_y4, G_y5, G_y6`

The concatenated encodings are an upstream constant (vector file,
`system_params.zkcredential_…`). `G_z` is unused here.

### 4.2 zkgroup ("classic") credential system parameters

Used by the profile key credential. Sho(`Signal_ZKGroup_20200424_Constant_Credentials_SystemParams_Generate`,
empty data). The empty data is still absorbed (ZP §2.2). Then 17 point
squeezes in **generation order**:

`G_w, G_w', G_x0, G_x1, G_y1, G_y2, G_y3, G_y4, G_m1, G_m2, G_m3, G_m4, G_V, G_z, G_y5, G_y6, G_m5`

libsignal hardcodes them as a 544-byte constant in a different
**storage order**:

`G_w, G_w', G_x0, G_x1, G_y1 … G_y6, G_m1 … G_m5, G_V, G_z`

(vector file, `system_params.zkgroup_credentials_…`). Only `G_w, G_w',
G_x0, G_x1, G_y1 … G_y5, G_m5, G_V` are used in this file. These are
**different points** from §4.1 despite the shared names. Below, `Gz_*`
refers to the zkgroup set and `Gk_*` to the zkcredential set wherever both
could be meant.

### 4.3 Encryption and commitment generators

- `G_a1, G_a2` (uid encryption) and `G_b1, G_b2` (profile key encryption):
  defined in ZG; repeated in the vector file. Rule: Sho(label, empty) and
  two point squeezes, with labels
  `Signal_ZKGroup_20200424_Constant_UidEncryption_SystemParams_Generate` and
  `Signal_ZKGroup_20200424_Constant_ProfileKeyEncryption_SystemParams_Generate`.
- `G_j1, G_j2, G_j3`: ZP §4.1.

## 5. Attribute points and encryption

### 5.1 Uid attribute (M1, M2)

For a service ID with binary form `sid` and UUID bytes `u`:

- `M1` = the **point** squeezed from Sho(`Signal_ZKGroup_20200424_UID_CalcM1`, `sid`).
  An ACI and a PNI with the same UUID have different M1.
- `M2` = Lizard encoding of `u` (ZG has the authoritative definition):
  take `SHA256(u)`, overwrite bytes 8..23 with `u`, clear bit 0 of byte 0
  and the top two bits of byte 31, then apply the RFC 9496 MAP **once**
  (the single-Elligator mapping of ZP §2.4). M2 depends only on the UUID,
  not on ACI/PNI.

Vector `auth_credential_with_pni[0].intermediate` has M1/M2 for an ACI and
a PNI.

### 5.2 Profile key attribute (M3, M4)

ZP §3.2, using the 48-byte `K ‖ U` of the profile key and the **ACI**.

### 5.3 Expiration scalar

`m5(T)` = the scalar squeezed from Sho(`Signal_ZKGroup_20220524_Timestamp_Calc_m`, `be64(T)`).
Note the **big-endian** encoding here, unlike the little-endian
serialization. `M5 = m5(T)·Gz_m5`.

### 5.4 Attribute encryption

An attribute is a pair of points `(M_a, M_b)`. An encryption key pair is
two scalars `(a1, a2)` with public key `A = a1·G_a1 + a2·G_a2` (the
generators of that key's domain). The ciphertext is

  `E1 = a1·M_a`, `E2 = a2·E1 + M_b`

and is serialized as `enc(E1) ‖ enc(E2)` (64 bytes; the 65-byte
`UuidCiphertext`/`ProfileKeyCiphertext` add a leading `0x00`, see ZG).
The uid key pair `(a1, a2, A)` and the profile-key key pair `(b1, b2, B)`
come from `GroupSecretParams` (ZG). In the 289-byte serialization they sit
at offsets 97 (a1), 129 (a2), 161 (A), 193 (b1), 225 (b2), 257 (B).

## 6. zkcredential: keyed-verification credentials

### 6.1 Keys

A server credential private key holds scalars `w, w', x0, x1, y0 … y6`
and the point `W = w·Gk_w`. The public key, which clients know from
`ServerPublicParams`, is:

- `C_W = W + w'·Gk_w'`;
- for each attribute-point count `N` from 2 to 7,
  `I_N = Gk_V − x0·Gk_x0 − x1·Gk_x1 − Σ_{i<N} y_i·Gk_yi`.

The serialized public key is `enc(C_W) ‖ enc(I_2) ‖ … ‖ enc(I_7)` (224
bytes). In `ServerPublicParams` it is the "generic credential public key"
at byte offset 417 (ZG). A credential with `N` attribute points uses
`I_N`. ZG §8.2 numbers the same six points `I_1 … I_6` by position; this
file's `I_N` is ZG's `I_(N−1)`.

### 6.2 Credentials

A credential is `(t, U, V)`: scalar t, points U and V, with

  `V = W + (x0 + x1·t)·U + Σ_i y_i·M_i`

over its attribute points `M_0 … M_{N−1}`. It serializes as
`enc_scalar(t) ‖ enc(U) ‖ enc(V)` (96 bytes).

### 6.3 Attribute points of a credential

- `M_0` encodes **all public attributes** together. Start with
  Sho(credential label). For each public attribute in order, absorb its
  bytes and ratchet. For a timestamp the bytes are `be64(seconds)`. Then
  squeeze one **point**. A credential with no public attributes squeezes
  right after creating the Sho.
- Then each hidden attribute contributes its two points, in the order the
  attributes are listed: `M_1, M_2` for the first, `M_3, M_4` for the
  second, and so on. At most 3 hidden attributes fit (N ≤ 7).

### 6.4 Issuance (server side, informational)

With 32 bytes of randomness R: `sho` = Sho(`Signal_ZKCredential_Issuance_20230410`, R).
Then `t` = scalar squeeze, `U` = point squeeze, V computed as in §6.2.
The proof randomness is a 32-byte squeeze from the same `sho`. The issuance
proof (message empty) proves the statement of §6.5.

The **issuance proof** is serialized as the credential followed by the
poksho proof as a byte string: `enc_scalar(t) ‖ enc(U) ‖ enc(V) ‖ le64(len) ‖ proof`.

### 6.5 Issuance proof statement and client verification

For N attribute points, the statement is three equations, in this order:

1. `C_W = w·G_w + w'·G_w'`
2. `G_V−I = x0·G_x0 + x1·G_x1 + y0·G_y0 + … + y_{N−1}·G_y{N−1}`
3. `V = w·G_w + x0·U + x1·tU + y0·M0 + … + y_{N−1}·M{N−1}`

Point values supplied by the client:

| Name | Value |
|------|-------|
| C_W | from the server public key |
| G_w, G_w', G_x0, G_x1, G_y0 … | §4.1 points |
| G_V−I | `Gk_V − I_N` |
| V, U | from the received credential |
| tU | `t·U` with t from the received credential |
| M0 … | the attribute points of §6.3, computed by the client from its own expected values |

So the point indices are: G 0, C_W 1, G_w 2, G_w' 3, G_V−I 4, G_x0 5,
G_x1 6, G_y0 … G_y{N−1}, V, U, tU, M0 … M{N−1}. The scalars are w, w', x0,
x1, y0 … y_{N−1}. For N = 5, D is
`03010200020103040702050306040705080609070a080b0c080002020d030e040f0510061107120813`.

The client accepts the credential `(t, U, V)` only if this proof verifies
(§3.6) with an empty message.

### 6.6 Presentation (client)

The client presents a credential while encrypting every hidden attribute
under a key pair whose public key the verifier knows. With 32 bytes of
randomness R:

1. `sho` = Sho(`Signal_ZKCredential_Presentation_20230410`, R); `z` =
   scalar squeeze.
2. Commitments, with `M'_0 = O` and `M'_i = M_i` for hidden attribute points:
   - `C_yi = z·Gk_yi + M'_i` for i = 0 … N−1;
   - `C_x0 = z·Gk_x0 + U`;
   - `C_x1 = z·Gk_x1 + t·U`;
   - `C_V = z·Gk_V + V`.
3. `I = I_N` from the public key; `Z = z·I`; `z0 = −z·t`.
4. Distinct encryption keys, in order of first use (attributes encrypted
   under the same key share one key entry). For each key k: `z1_k = −z·a1_k`.
5. For each hidden attribute with first point index f and second index
   f+1 (f = 1, 3, 5) and key k:
   `E_Af = a1_k·M_f`, `E_A(f+1) = a2_k·E_Af + M_(f+1)` (§5.4).
6. The proof randomness is a 32-byte squeeze from `sho`; the message is
   empty.

**Statement**, equations in this order:

1. `Z = z·I`
2. `C_x1 = t·C_x0 + z0·G_x0 + z·G_x1`
3. for each key k in order: `0 = z1_k·I + a1_k·Z` (LHS: the identity point,
   one shared point named "0")
4. one equation `sumA = Σ_k (a1_k·G_a1_k + a2_k·G_a2_k)` over the keys in
   order, where `G_a1_k, G_a2_k` are that key's domain generators
5. for each hidden attribute in order (f as above, key k):
   - `E_Af = a1_k·C_yf + z1_k·G_yf`
   - `C_y(f+1)−E_A(f+1) = z·G_y(f+1) + a2_k·(−E_Af)`
6. `C_y0 = z·G_y0`

Point values: `I`, `Z`, `C_x0`, `C_x1`, `C_yi` as above; `G_x0, G_x1,
G_yi` from §4.1; "0" = `O`; `sumA` = sum of the keys' public points;
`E_Af`; `C_y(f+1) − E_A(f+1)`; `−E_Af` (the negation of `E_Af`).

Witness: `z, t, z0`, and per key `z1_k, a1_k, a2_k`.

**Serialization** of the presentation proof:
`enc(C_x0) ‖ enc(C_x1) ‖ enc(C_V) ‖ le64(N) ‖ enc(C_y0) ‖ … ‖ enc(C_y{N−1}) ‖ le64(len) ‖ proof`.

### 6.7 Presentation verification (server side, informational)

The verifier recomputes `M_0` from the public attributes and computes
`Z = C_V − W − x0·C_x0 − x1·C_x1 − Σ y_i·C_yi − y0·M_0`. It takes `E_Af`
and `E_A(f+1)` from the ciphertexts sent alongside, and verifies the
statement of §6.6 with the same index layout.

## 7. AuthCredentialWithPni (zkc)

### 7.1 Where it comes from (context)

signal-cli (C1) computes `today` = the current Unix time, rounded down to
a multiple of 86400. It requests
`GET /v1/certificate/auth/group?redemptionStartSeconds=<today>&redemptionEndSeconds=<today+604800>`
(C2 `GroupsV2ApiHelper.kt`).

libsignal-service-java v2.15.3_unofficial_151 sends **no**
`zkcCredential` query parameter. The server returns zkc credentials
anyway: version 3 (below) is the only response format libsignal 0.99.1
accepts. The JSON envelope (a list of `{credential, redemptionTime}`,
credential in base64) is specified by the groups-service spec.

signal-cli keeps the parsed responses in memory, keyed by redemption time,
and uses the one whose redemption time equals `today`. If verification
fails (§7.3), it re-fetches once.

### 7.2 Response format (425 bytes)

| Offset | Len | Field |
|--------|-----|-------|
| 0 | 1 | version `0x03` (the only accepted value; anything else is a parse error) |
| 1 | 96 | credential `t ‖ U ‖ V` (§6.2) |
| 97 | 8 | `le64(320)` |
| 105 | 320 | poksho issuance proof (challenge + 9 responses) |

### 7.3 Receive and verify

Inputs: the response, the client's own ACI, own PNI, and the redemption
time T (seconds, from the JSON). Steps:

1. Reject unless T is day-aligned.
2. Attribute points (N = 5):
   - `M0` from §6.3 with label `20240222_Signal_AuthCredentialZkc` and one
     public attribute `be64(T)`;
   - `M1, M2` = the uid points of the ACI (§5.1, sid = 16 bytes);
   - `M3, M4` = the uid points of the PNI (sid = `0x01 ‖ uuid`).
3. Verify the issuance proof (§6.5) against the generic credential public
   key from `ServerPublicParams`.

The resulting `AuthCredentialWithPni` (265 bytes), which signal-cli does
not persist:

| Offset | Len | Field |
|--------|-----|-------|
| 0 | 1 | version `0x03` |
| 1 | 96 | `t ‖ U ‖ V` |
| 97 | 16 | ACI UUID bytes |
| 113 | 32 | ACI M1 |
| 145 | 32 | ACI M2 |
| 177 | 16 | PNI UUID bytes (raw UUID, no prefix byte) |
| 193 | 32 | PNI M1 (computed with the `0x01` prefix) |
| 225 | 32 | PNI M2 |
| 257 | 8 | `le64(T)` |

Bound values: the ACI and PNI the client passes in (the account's own),
and T. A wrong ACI, wrong PNI, swapped ACI/PNI, wrong T or unaligned T all
make verification fail (vector `receive_negative`).

### 7.4 Presentation (633 bytes)

Inputs: the credential, `GroupSecretParams` of the target group, and 32
bytes of randomness. Apply §6.6 with N = 5, `I = I_5`, and two hidden
attributes (ACI uid points, then PNI uid points), both encrypted under the
group's **uid** key pair (one key entry). The statement therefore has 9
equations, and D is
`0901010002030301040205000607020302040108020409050a0b02040c030d0e02000f051011020412031314020015051617010018`
(point order: G, Z, I, C_x1, C_x0, G_x0, G_x1, 0, sumA, G_a1, G_a2, E_A1,
C_y1, G_y1, C_y2−E_A2, G_y2, −E_A1, E_A3, C_y3, G_y3, C_y4−E_A4, G_y4,
−E_A3, C_y0, G_y0; scalar order: z, t, z0, z1, a1, a2). Here `sumA` is the
uid public key A.

| Offset | Len | Field |
|--------|-----|-------|
| 0 | 1 | version `0x03` |
| 1 | 32 | C_x0 |
| 33 | 32 | C_x1 |
| 65 | 32 | C_V |
| 97 | 8 | `le64(5)` |
| 105 | 160 | C_y0 … C_y4 |
| 265 | 8 | `le64(224)` |
| 273 | 224 | poksho proof (challenge + 6 responses) |
| 497 | 64 | ACI ciphertext `E_A1 ‖ E_A2` |
| 561 | 64 | PNI ciphertext `E_A3 ‖ E_A4` |
| 625 | 8 | `le64(T)` |

The ciphertexts are exactly the group's `UuidCiphertext`s of the ACI and
PNI without their leading `0x00` (ZG). The first byte `0x03` is
libsignal's "presentation version 4". Versions 0x00–0x02 are obsolete and
rejected.

Use (C2 `GroupsV2AuthorizationString`, details in the groups-service spec):
HTTP Basic credentials with user = lowercase hex of the 97-byte
`GroupPublicParams` and password = lowercase hex of these 633 bytes.
signal-cli creates a fresh presentation, with fresh randomness, for every
authorization.

### 7.5 Server acceptance window (informational)

The server accepts a presentation when its current time is between
`T − 86400` and `T + 2·86400`, both inclusive.

## 8. ExpiringProfileKeyCredential

This credential uses zkgroup's older proof layout (§4.2 generators),
not zkcredential.

### 8.1 Request context (client)

Inputs: the target's ACI (UUID bytes `U`), their profile key `K`, and 32
bytes of randomness R.

1. Compute M3, M4 (ZP §3.2) and the commitment J1, J2, J3 with nonce j3
   (ZP §4).
2. `sho` = Sho(`Signal_ZKGroup_20200424_Random_ServerPublicParams_CreateProfileKeyCredentialRequestContext`, R).
3. In order from `sho`: `y` = scalar, `r1` = scalar, `r2` = scalar.
   `Y = y·G`.
4. Request ciphertext: `D1 = r1·G`, `E1 = r2·G`, `D2 = r1·Y + M3`,
   `E2 = r2·Y + M4`.
5. Proof (message empty, randomness = 32-byte squeeze from `sho`) of:
   1. `Y = y·G`
   2. `D1 = r1·G`
   3. `E1 = r2·G`
   4. `J3 = j3·G_j3`
   5. `D2−J1 = r1·Y + j3·(−G_j1)`
   6. `E2−J2 = r2·Y + j3·(−G_j2)`

   Point values: `D2 − J1`, `E2 − J2`, `−G_j1`, `−G_j2` are the computed
   differences and negations. D = `0601010000020101000301020004010305060201010307080202010309`.
   Scalars: y, r1, r2, j3.

Request context (473 bytes; client-local, kept until the response
arrives):

| Offset | Len | Field |
|--------|-----|-------|
| 0 | 1 | reserved `0x00` |
| 1 | 16 | ACI UUID |
| 17 | 32 | profile key |
| 49 | 32 | y |
| 81 | 32 | Y |
| 113 | 32 | r1 |
| 145 | 32 | r2 |
| 177 | 32 | D1 |
| 209 | 32 | D2 |
| 241 | 32 | E1 |
| 273 | 32 | E2 |
| 305 | 8 | `le64(160)` |
| 313 | 160 | proof (challenge + 4 responses) |

### 8.2 Request (329 bytes)

`0x00 ‖ Y ‖ D1 ‖ D2 ‖ E1 ‖ E2 ‖ le64(160) ‖ proof`: the context without
the UUID, the profile key and the secret scalars.

It is sent as **lowercase hex** (658 characters) in
`GET /v1/profile/{aci}/{version}/{request-hex}?credentialType=expiringProfileKey`,
where `{version}` is the profile-key version of ZP §5 (C2 `ProfileApi.kt`).
The server checks the request proof against the commitment stored with
the profile.

### 8.3 Response (497 bytes)

The response arrives base64-encoded in the JSON member `credential` of the
profile response (profile spec). Layout:

| Offset | Len | Field |
|--------|-----|-------|
| 0 | 1 | reserved `0x00` |
| 1 | 32 | t |
| 33 | 32 | U |
| 65 | 32 | S1 |
| 97 | 32 | S2 |
| 129 | 8 | `le64(expiration)` |
| 137 | 8 | `le64(352)` |
| 145 | 352 | issuance proof (challenge + 10 responses: w, w', x0, x1, y1 … y5, r') |

Server side (informational): `sho` = Sho(`…IssueExpiringProfileKeyCredential`, R).
It draws t (scalar) and U (point), computes `V' = W + (x0 + x1·t)·U +
y1·M1 + y2·M2 + y5·M5`, where M1, M2 are the ACI uid points and
`M5 = m5(expiration)·Gz_m5`. Then it draws r' (scalar) and computes
`S1 = r'·G + y3·D1 + y4·E1` and `S2 = r'·Y + V' + y3·D2 + y4·E2`. The
proof randomness is a 32-byte squeeze.

### 8.4 Receive and verify (client)

Inputs: the stored context, the response, and the current time `now`
(seconds).

1. Verify the issuance proof (message empty). Statement, in order:
   1. `C_W = w·G_w + w'·G_w'`
   2. `G_V−I = x0·G_x0 + x1·G_x1 + y1·G_y1 + y2·G_y2 + y3·G_y3 + y4·G_y4 + y5·G_y5`
   3. `S1 = y3·D1 + y4·E1 + r'·G`
   4. `S2 = y3·D2 + y4·E2 + r'·Y + w·G_w + x0·U + x1·tU + y1·M1 + y2·M2 + y5·M5`

   Point values: `C_W` and `I` are the "expiring profile key credentials"
   public key of `ServerPublicParams` (offsets 289 and 321; ZG).
   `G_V−I = Gz_V − I`. The generators are the §4.2 set. S1, S2, U, t come
   from the response, with `tU = t·U`. D1, D2, E1, E2, Y come from the
   context. M1, M2 are the uid points of the context's ACI, and M5 uses
   the response's expiration. D =
   `04010200020103040702050306040705080609070a080b0c03060d070e09000f09061007110912000202130314041505160817`.
2. Reject unless the expiration is day-aligned.
3. Let `days = floor(max(0, expiration − now) / 86400)`. Reject if
   `days = 0` or `days > 7`. So an expiration less than one full day away,
   or in the past, fails, and so does one 8 or more full days away (vector
   `receive_at`).
4. `V = S2 − y·S1`. The credential is `(t, U, V)`.

ExpiringProfileKeyCredential (153 bytes, persisted by signal-cli per
recipient):

| Offset | Len | Field |
|--------|-----|-------|
| 0 | 1 | reserved `0x00` |
| 1 | 32 | t |
| 33 | 32 | U |
| 65 | 32 | V |
| 97 | 16 | ACI UUID |
| 113 | 32 | profile key |
| 145 | 8 | `le64(expiration)` |

signal-cli treats a stored credential as usable while `now < expiration`
(C2 `ExpiringProfileCredentialUtil`). Otherwise it fetches the profile
again with a new request before adding that member.

### 8.5 Presentation (721 bytes)

Inputs: the credential, the target group's `GroupSecretParams` (uid key
`a1, a2, A`; profile-key key `b1, b2, B`), and 32 bytes of randomness R.
Used in group changes that add or invite members, join requests, and
profile-key updates (groups spec).

1. `sho` = Sho(`Signal_ZKGroup_20220508_Random_ServerPublicParams_CreateExpiringProfileKeyCredentialPresentation`, R);
   `z` = scalar.
2. M1, M2 = uid points of the credential's ACI; M3, M4 = profile-key
   points of (profile key, ACI).
3. `C_y1 = z·Gz_y1 + M1`, `C_y2 = z·Gz_y2 + M2`, `C_y3 = z·Gz_y3 + M3`,
   `C_y4 = z·Gz_y4 + M4`, `C_y5 = z·Gz_y5`; `C_x0 = z·Gz_x0 + U`,
   `C_x1 = z·Gz_x1 + t·U`, `C_V = z·Gz_V + V`.
4. `I` = the expiring-profile-key public key's I; `Z = z·I`;
   `z0 = −z·t`, `z1 = −z·a1`, `z2 = −z·b1`.
5. Ciphertexts (§5.4): `E_A1 = a1·M1`, `E_A2 = a2·E_A1 + M2`,
   `E_B1 = b1·M3`, `E_B2 = b2·E_B1 + M4`. These equal the group's
   `UuidCiphertext` of the ACI and `ProfileKeyCiphertext` of the profile
   key (ZG), minus the leading `0x00`.
6. Proof (message empty, randomness = 32-byte squeeze from `sho`).
   Statement, in order:
   1. `Z = z·I`
   2. `C_x1 = t·C_x0 + z0·G_x0 + z·G_x1`
   3. `A+B = a1·G_a1 + a2·G_a2 + b1·G_b1 + b2·G_b2`
   4. `C_y2−E_A2 = z·G_y2 + a2·(−E_A1)`
   5. `E_A1 = a1·C_y1 + z1·G_y1`
   6. `C_y4−E_B2 = z·G_y4 + b2·(−E_B1)`
   7. `E_B1 = b1·C_y3 + z2·G_y3`
   8. `0 = z1·I + a1·Z`
   9. `0 = z2·I + b1·Z` (same point "0" = O as equation 8)
   10. `C_y5 = z·G_y5`

   `A+B` is the sum of the two public keys; the generators are the §4.2
   set and `G_a*`, `G_b*`. Scalar order: z, t, z0, a1, a2, b1, b2, z1, z2.
   D = `0a010100020303010402050006070403080409050a060b0c02000d040e0f02031007111202001306141502051608171802070203011802080205011901001a`.

| Offset | Len | Field |
|--------|-----|-------|
| 0 | 1 | version `0x03` |
| 1 | 256 | C_x0, C_x1, C_y1, C_y2, C_y3, C_y4, C_y5, C_V (this order) |
| 257 | 8 | `le64(320)` |
| 265 | 320 | proof (challenge + 9 responses) |
| 585 | 64 | `E_A1 ‖ E_A2` |
| 649 | 64 | `E_B1 ‖ E_B2` |
| 713 | 8 | `le64(expiration)` |

The server (informational) verifies with
`Z = C_V − W − x0·C_x0 − x1·C_x1 − y1·C_y1 − … − y4·C_y4 − y5·(C_y5 + M5)`
and rejects when `expiration ≤ now`.

## 9. Group send endorsements

### 9.1 Where they come from

The group server attaches a `GroupSendEndorsementsResponse` (bytes) to
group fetches, group creation, group changes and group history pages
(the `group_send_endorsements_response` bytes field of the respective
protobuf responses; field numbers are in the groups-service spec). Empty
bytes mean "none".

signal-cli (C1 `GroupHelper`, C2 `GroupsV2Operations.receiveGroupSendEndorsements`)
receives it with:

- the member list = the **ACIs of all full members** of the decrypted
  group state, in the order they appear in that state;
- the local user = its own ACI. If the local ACI is not a member, signal-cli
  skips receiving.

It stores, per group, each member's individual endorsement (33-byte
serialization, §9.6) and the expiration in milliseconds.

### 9.2 Response format

| Field | Encoding |
|-------|----------|
| reserved | `0x00` |
| endorsements R | `le64(n)` then n × 32-byte point encodings, in **sorted** order (§9.4), not checked at parse time |
| proof | `le64(64)` then a 64-byte poksho proof |
| expiration | `le64(seconds)` |

Total `1 + 8 + 32n + 8 + 64 + 8` bytes (217 for 4 members).

### 9.3 Key derivation

The server's endorsement root public key `PK` is the last field of
`ServerPublicParams` (offset 641; ZG). For an expiration E:

- `tag` = Sho(`20240215_Signal_GroupSendEndorsement`, `be64(E)`), then
  `τ` = scalar squeeze;
- derived public key `PK' = PK + τ·G`.

(Server: root secret sk; derived secret `sk' = (sk + τ)⁻¹`; endorsement of
point P is `sk'·P`.)

### 9.4 Receive (client)

Inputs: the response, the ordered member service IDs, the current time
`now`, the group's `GroupSecretParams` (uid scalar `a1`), and the server
public params.

1. **Expiration check** on the response's expiration E. Reject unless E is
   day-aligned. Let `rem = max(0, E − now)`. Reject if `rem < 7200` (two
   hours, which also covers past expirations) or `rem > 7·86400`.
2. Derive `PK'` for E (§9.3).
3. Member points: for member i, `E_i = a1·M1(member_i)` (§5.1, with that
   member's service-ID binary form). `E_i` is exactly the first point of
   the member's `UuidCiphertext` in this group.
4. **Sort.** The order of the R list is the members sorted by `dbl(E_i)`
   (the 32-byte encoding of `2·E_i`), compared as unsigned byte strings,
   lexicographically ascending. Ties cannot happen for distinct members.
   Below, `σ(0), σ(1), …` are the member indices in sorted order. `R_j`
   (the j-th list entry) is the endorsement of member σ(j).
5. Reject if the number of members differs from n.
6. **Weights.** `h` = Sho(`Signal_ZKCredential_Endorsements_EndorsementResponse_ProofWeights_20240207`).
   Absorb `enc(PK')`, then `dbl(E_σ(j))` for j = 0 … n−1, then the n
   32-byte R entries as received, then ratchet. Squeeze `16·(n−1)` bytes
   (zero bytes when n = 1). For j = 1 … n−1, weight `ω_j` is the 16-byte
   chunk j−1, with the top bit of its last byte (bit 7 of byte 15)
   cleared, read as a little-endian integer (< 2^127).
7. `ΣE = E_σ(0) + Σ_{j≥1} ω_j·E_σ(j)`. `ΣR = R_0 + Σ_{j≥1} ω_j·R_j`,
   where an R entry that fails to decode counts as `O`; the proof check
   then fails.
8. Verify the poksho proof (message empty) of the two-equation statement:
   1. `ΣR = sk'·ΣE`
   2. `G = sk'·PK'` (the LHS is the base point itself)

   D = `020101000200010003` (points: G, ΣR, ΣE, PK'; scalar: sk').
9. Output: for each member i in the **original member order**, its
   endorsement is the R entry at its sorted position, kept as the exact 32
   bytes received.

The libsignal Java API (`receive(groupMembers, localUser, now, groupParams, serverParams)`)
also returns the **combined endorsement of all members except the local
user** (sum of the decoded R points; identity if the local user is the
only member). The local user must be in the list. Otherwise the call
fails with an assertion, not a verification failure.

`receive(ciphertexts, localCiphertext, …)` is the same algorithm with
`E_i` taken from the members' `UuidCiphertext`s; the vectors confirm
identical results.

### 9.5 Combine and remove

Endorsements are points. Combining a set is their sum, and the order does
not matter; the empty set gives `O`. Removing is subtraction. All must come
from the same response (same expiration and group key) to produce a valid
token.

### 9.6 Serialized forms

| Value | Encoding | Size |
|-------|----------|------|
| GroupSendEndorsement | `0x00 ‖ enc(R)` | 33 |
| GroupSendToken | `0x00 ‖ le64(16) ‖ token(16)` | 25 |
| GroupSendFullToken | `0x00 ‖ le64(16) ‖ token(16) ‖ le64(expiration)` | 33 |

Parsing a GroupSendEndorsement decodes (validates) the point.

### 9.7 Tokens

For an endorsement (individual or combined) R, in a group with uid scalar
a1:

- `P = a1⁻¹·R` (this removes the group-specific blinding, so P equals
  `sk'·Σ M1(member)` and is the same in every group);
- `token = first 16 bytes of SHA256(enc(P))` (plain SHA-256, no Sho);
- full token = token plus the expiration of the response it came from. The
  expiration is **not** checked when building a full token. A wrong value
  only makes the server reject it.

Consequence (visible in the vectors): the same member, with the same
expiration, gets the same token in two different groups.

### 9.8 Use in signal-cli

- Before a group send, signal-cli loads the stored endorsements and
  expiration. If there are none, or if `expiration − 2 h < now`, it
  re-fetches the group (which delivers a new response) and tries again. If
  that still fails, it sends without endorsements (C1 `SendHelper`).
- Multi-recipient (sender-key) sends: the token is the combination of the
  endorsements of exactly the sender-key recipients of that send (never
  the local user), as a full token.
- Individual fallback sends: a full token from that recipient's single
  endorsement.
- Header: `Group-Send-Token: <standard base64 with padding of the 33-byte full token>`
  (C2 `SealedSenderAccess.kt`). Request routing is in the stories/send
  specs.

### 9.9 Server verification (informational)

The server checks `now ≤ expiration`, derives `sk'` for the token's
expiration, computes `SHA256(enc(sk'·Σ_i M1(member_i)))[:16]` over the
claimed recipients, and compares in constant time. The server's default
expiration is the end of the next UTC day, or one day later if that is
less than 25 hours away.

## 10. Server notary signatures (group changes)

A signed group change carries `serverSignature` (64 bytes) over the
`actions` field. The client verifies:

- message = the bytes of the serialized `actions` field **exactly as
  received** (never re-encoded);
- signature = the 64-byte poksho signature (§3.7), parsed as challenge
  plus one response (both canonical);
- public key = the notary signature public key of `ServerPublicParams`
  (offset 129, the "sig public key"; ZG).

The statement is `public_key = private_key·G` (§3.7), verified with the
message M (§3.4) set to the actions bytes. A
signature that is not exactly 64 bytes is rejected before verification.

Server signing (informational): `sho` = Sho(`Signal_ZKGroup_20200424_Random_ServerSecretParams_Sign`, R);
poksho randomness = 32-byte squeeze. Vector `notary_signature` (upstream
`EXPECTED_SIGNATURE`) can be verified with the vector server's public
params.

## 11. Server parameters needed for tests

Clients only parse `ServerPublicParams` (ZG). To use the vectors, a test
needs the following offsets in the 673-byte public params:

| Offset | Content |
|--------|---------|
| 0 | reserved `0x00` |
| 129 | notary signature public key (32) |
| 289 | expiring profile key credential public key: C_W (32), I (32) |
| 417 | generic (zkcredential) credential public key: C_W, I_2 … I_7 (224) |
| 641 | endorsement root public key PK (32) |

The 2721-byte `ServerSecretParams` in the vectors exist only to let a test
harness run the server role. Its layout, with all keys in order:

- reserved byte;
- seven classic credential/signature keys: auth (352), profile (352),
  signature (`sk ‖ pk`, 64), receipt (352), pni (416), expiring profile
  key (384), auth-with-pni (384). Each classic credential key is
  `w ‖ w' ‖ W ‖ x0 ‖ x1 ‖ y_1 … y_k ‖ C_W ‖ I`, with k = 4, 4, 4, 6, 5, 5
  respectively; the expiring profile key key starts at offset 1537;
- the zkcredential private key `w ‖ w' ‖ W ‖ x0 ‖ x1 ‖ y0 … y6` (384,
  offset 2305);
- the endorsement root secret scalar (32, offset 2689).

Generation from 32 bytes R:

- the seven classic keys come from one Sho(`Signal_ZKGroup_20200424_Random_ServerSecretParams_Generate`, R),
  drawing for each key, in order, w, w', x0, x1, then its stored y
  scalars as consecutive scalar squeezes; the signature key draws one
  scalar. Each classic key's `I` uses only its first NUM y's: 3 for auth,
  4 for profile, 2 for receipt, 6 for pni, 5 for expiring profile key, 5
  for auth-with-pni. Here `I = Gz_V − x0·Gz_x0 − x1·Gz_x1 − Σ y_i·Gz_yi`
  and `C_W = w·Gz_w + w'·Gz_w'`;
- the zkcredential key comes from Sho(`Signal_ZKCredential_CredentialPrivateKey_generate_20230410`, R):
  w, w', x0, x1, y0 … y6 (§6.1);
- the endorsement root secret = scalar from
  Sho(`Signal_ZKCredential_Endorsements_ServerRootKeyPair_generate_20240207`, R).

The spec agent confirmed the zkcredential key, the endorsement key, the
signature key and the expiring-profile-key `C_W`/`I` against the jar.

## 12. Randomness summary

Every randomized operation takes exactly **32 bytes**. The Java API draws
them with one `nextBytes(byte[32])` call when given a `SecureRandom`. No
other randomness is used: everything else is derived through the Sho
instance seeded with those bytes, or (for endorsement issuance) passed
directly to poksho.

| Operation | Who | Expansion of the 32 bytes R |
|-----------|-----|-----------------------------|
| AuthCredentialWithPni presentation | client | Sho(`…ZKCredential_Presentation_20230410`, R): z (64-byte scalar), then 32 bytes poksho randomness |
| Profile credential request context | client | Sho(`…CreateProfileKeyCredentialRequestContext`, R): y, r1, r2 (scalars), then 32 bytes poksho randomness |
| Profile credential presentation | client | Sho(`…CreateExpiringProfileKeyCredentialPresentation`, R): z, then 32 bytes poksho randomness |
| Auth credential issuance | server | Sho(`…ZKCredential_Issuance_20230410`, R): t, U (point), then 32 bytes poksho randomness |
| Profile credential issuance | server | Sho(`…IssueExpiringProfileKeyCredential`, R): t, U, r', then 32 bytes poksho randomness |
| Endorsement issuance | server | R used directly as poksho randomness |
| Notary signature | server | Sho(`…ServerSecretParams_Sign`, R): 32 bytes poksho randomness |
| ServerSecretParams | server | §11 |

Receiving and verifying, combining, removing and token building are
deterministic.

## 13. Size summary

| Value | Bytes | First byte |
|-------|-------|------------|
| AuthCredentialWithPniResponse | 425 | 0x03 |
| AuthCredentialWithPni | 265 | 0x03 |
| AuthCredentialPresentation (zkc) | 633 | 0x03 |
| ProfileKeyCredentialRequestContext | 473 | 0x00 |
| ProfileKeyCredentialRequest | 329 | 0x00 |
| ExpiringProfileKeyCredentialResponse | 497 | 0x00 |
| ExpiringProfileKeyCredential | 153 | 0x00 |
| ProfileKeyCredentialPresentation (expiring) | 721 | 0x03 |
| GroupSendEndorsementsResponse | 89 + 32n | 0x00 |
| GroupSendEndorsement | 33 | 0x00 |
| GroupSendToken | 25 | 0x00 |
| GroupSendFullToken | 33 | 0x00 |
| NotarySignature | 64 | (raw) |
| ServerPublicParams | 673 | 0x00 |
| ServerSecretParams | 2721 | 0x00 |

## 14. Test vectors

`testdata/vectors/zkgroup_credentials.json`, with provenance in its
`source.provenance` block:

- **Upstream data** (copied from libsignal Z3/Z2/Z1/Z5):
  - 3 ShoSha256 vectors;
  - 4 poksho statement encodings, 1 poksho signature, 1 multi-equation
    proof;
  - the zkcredential (13 points) and zkgroup credential (17 points)
    generator constants, plus G_a, G_b, G_j;
  - the auth presentation `AUTH_CREDENTIAL_PRESENTATION_V4_RESULT`
    (= `auth_credential_with_pni[0].presentation`), the profile
    presentation `PROFILE_KEY_CREDENTIAL_PRESENTATION_V4_RESULT`
    (= `expiring_profile_key_credential[0].presentation`) and the notary
    signature `EXPECTED_SIGNATURE`.

  Each of these was reproduced by the jar.
- **Reference jar** (J1): server params from T32; two groups; two auth
  credential cases (response, credential, presentation, ciphertexts, 4
  negative receive checks each); two profile credential cases (commitment,
  version, context, request, response, credential, presentation,
  ciphertexts, 9 receive-time checks each); four endorsement cases with
  4, 2, 7 and 1 members (member ciphertexts, response, per-member
  endorsements, tokens and full tokens, combined-without-self, combine-all,
  remove, 6 receive-time checks each).
- **Derived by the spec agent**: the seven statement descriptions, and
  intermediates for the first case of each kind (M0, M1, M2, I_5, z,
  poksho randomness; y, r1, r2, m5; τ, PK', E_i, sort keys, sorted order,
  weight bytes, a1⁻¹).

To test client code byte for byte:

1. Parse `server.public_params` and the group's `secret_params`.
2. Receive `response` with the listed inputs; compare to `credential`.
3. Present with `present_randomness`; compare to `presentation`.

For the profile credential, build the context from
`request_context_randomness` and compare `request_context` and `request`.
Then receive `response` at `receive_time` and present. For endorsements,
receive `response` at `receive_time` with `members_aci` and the local
index, then compare endorsements, tokens (`full_tokens` use `expiration`)
and the combined values.

libsignal publishes no fixed vectors for endorsements, request contexts,
issued responses or received credentials. Those come from the jar.

## Gaps

- **Server-side issuance** (§6.4, §8.3, §9.3) and **ServerSecretParams
  generation** (§11) are informational. They were checked only where §11
  says, not by re-issuing responses with a scratch implementation.
- **Lizard (M2)** is summarized in §5.1; ZG is authoritative. The summary
  was confirmed against the dalek test vector and through every
  M2-dependent proof in the vectors.
- **Obsolete formats** (auth presentations v1–v3, profile presentations
  v1–v3, non-zkc auth credentials, receipt credentials, call link, backup
  and avatar credentials) are out of scope. libsignal 0.99.1 rejects the
  obsolete versions when parsing or verifying.
- **Server behaviour** that clients cannot observe (rate limits, which
  members the server endorses, caching) was not examined. The
  `zkcCredential` query parameter mentioned in the task is not sent by the
  pinned libsignal-service-java (§7.1). Whether the deployed server still
  requires or ignores it was not checked against Signal-Server.
- signal-cli's exact retry behaviour for profile credential fetch failures
  beyond what §8.4 states, and its database schema for stored endorsements,
  belong to the storage/groups specs.
