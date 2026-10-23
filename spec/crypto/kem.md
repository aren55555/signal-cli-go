# KEMs in libsignal v0.99.1 (as bundled by signal-cli v0.14.7)

Spec role: prose only. Implementers should read this together with the
published standards cited below. Byte values and lengths here are
interoperability facts. Nothing in this file is code.

## Sources

| Ref | What | URL @ version |
|-----|------|---------------|
| S1 | signal-cli build files | https://github.com/AsamK/signal-cli @ tag v0.14.7 = commit b01b6b370dc063599a1a2b9fde0f5ff4e2d78fe8: `gradle/libs.versions.toml`, `build.gradle.kts`, `lib/build.gradle.kts`, `lib/src/main/java/org/asamk/signal/manager/util/KeyUtils.java`, `.../helper/PinHelper.java`, `.../helper/RecipientHelper.java`, `CHANGELOG.md` |
| S2 | Reference binary distribution | `/work/third_party/signal-cli-0.14.7/lib/*.jar` (jar listings, manifests, strings in the bundled `libsignal_jni_amd64.so`) |
| S3 | libsignal | https://github.com/signalapp/libsignal @ tag v0.99.1 = commit 97801d22dcf9f5bf714f7b8fa3212cdc973ae1c8: `Cargo.toml`, `Cargo.lock`, every `Cargo.toml` in the workspace, `rust/protocol/src/kem.rs`, `rust/protocol/src/kem/{kyber1024,kyber768,mlkem1024}.rs`, `rust/protocol/src/{pqxdh,session,ratchet,protocol,triple_ratchet,sealed_sender}.rs`, `rust/protocol/src/state/{kyber_prekey,bundle,session}.rs`, `rust/bridge/shared/src/protocol.rs`, `rust/attest/src/{client_connection,enclave,snow_resolver,cds2,svr2,sgx_session,hsm_enclave}.rs`, `rust/crypto/src/hpke.rs`, `java/shared/java/org/signal/libsignal/protocol/kem/KEMKeyType.kt` |
| S4 | libcrux-ml-kem 0.0.10 (the KEM implementation libsignal links) | https://github.com/cryspen/libcrux (now served as celabshq/libcrux) @ tag libcrux-ml-kem-v0.0.10 = commit c5fb80f37530ee9b2df9501ae5ff8cb4a973a4bd: `libcrux-ml-kem/src/{variant,ind_cca,mlkem1024}.rs`, `libcrux-ml-kem/tests/kats/{README.md,generate_kyber_kats.py,nistkats_kyber_1024.json}`. Cargo.lock in S3 pins crates.io checksum 1d8160f7d64fd2716b4fd05cc886a042f8dcda18d9206c0d506e2c67bdf97daa |
| S5 | SPQR | https://github.com/signalapp/SparsePostQuantumRatchet @ tag v1.5.3 = commit fd320484dcec89004021e6fdc7481825f5f261fa: `Cargo.toml`, file list |
| S6 | Kyber round-3 NIST submission package (spec v3.0 + KATs) | https://csrc.nist.gov/CSRC/media/Projects/post-quantum-cryptography/documents/round-3/submissions/Kyber-Round3.zip (SHA-256 a68b152c7cf70a32bab1abaf2ab5a21423f33b0ac11ca8db9b9dd4864007319b; top folder `NIST-PQ-Submission-Kyber-20201001`) |
| S7 | Kyber prose specification matching round-3 v3.02 | draft-cfrg-schwabe-kyber-04, https://datatracker.ietf.org/doc/html/draft-cfrg-schwabe-kyber-04 (sections 6, 10, 11, 12, appendix B.4). It cites the v3.02 spec (https://pq-crystals.org/kyber/data/kyber-specification-round3-20210804.pdf) |
| S8 | ML-KEM | FIPS 203, https://csrc.nist.gov/pubs/fips/203/final (Appendix C lists the differences from round-3 Kyber) |
| S9 | NIST DRBG | SP 800-90A Rev. 1, https://csrc.nist.gov/pubs/sp/800/90/a/r1/final, section 10.2.1 (CTR_DRBG) |
| S10 | Go standard library | https://pkg.go.dev/crypto/mlkem |

Blocked hosts: `pq-crystals.org`, `nvlpubs.nist.gov`, `www.ietf.org` and
`crates.io` were refused by the egress proxy. This agent therefore did not
read the v3.02 Kyber PDF or the FIPS 203 PDF directly. No PDF renderer was
available, so `kyber.pdf` inside S6 could not be read either. The Kyber
algorithm description below comes from S7, which states that it matches
v3.02 and was checked against the NIST vectors. The ML-KEM side comes from
S8 by reference, corroborated by S4's own documentation of the
Kyber/ML-KEM differences ("cf. FIPS 203, Appendix C").

## Q0. Exact libsignal version

**libsignal v0.99.1**, tag commit `97801d22dcf9f5bf714f7b8fa3212cdc973ae1c8`.

Evidence:
- The distribution ships `lib/libsignal-client-0.99.1.jar`, which bundles
  `libsignal_jni_amd64.so` and other native libraries (S2).
- signal-cli does not declare libsignal directly. It depends on
  `com.github.turasa:signal-network` version `2.15.3_unofficial_151`
  (`gradle/libs.versions.toml`), and libsignal-client arrives through it
  transitively. `lib/build.gradle.kts` excludes `org.signal:libsignal-client`
  only when a local `libsignal_client_path` override is given. The root
  `build.gradle.kts` has a `writeLibsignalVersion` task that records whatever
  version of `org.signal:libsignal-client` gets resolved. So the jar version
  in the distribution is the authoritative answer, and it is 0.99.1 (S1, S2).
- The native library's embedded crate paths name `libcrux-ml-kem-0.0.10`,
  `libcrux-sha3-0.0.10` and `spqr`. These match the `Cargo.lock` of
  libsignal v0.99.1 (S2, S3).

## Bottom line

1. **The 1:1 session prekeys (PQXDH) use pre-standard CRYSTALS-Kyber1024,
   round 3, not ML-KEM.** In the shipped build it is the only KEM key type
   accepted. Its type byte is `0x08`. It is the "Kyber" variant of
   libcrux-ml-kem 0.0.10. Its outputs match all 100 entries of the official
   NIST round-3 Kyber1024 KAT file byte for byte (pk, sk, ct, ss; checked by
   this agent). Go's `crypto/mlkem` is **not** compatible with it, so the
   implementation needs its own Kyber round-3 KEM.
2. **FIPS 203 ML-KEM is used elsewhere:** ML-KEM-1024 in the Noise "hfs"
   handshake to the CDSI and SVR2 enclaves, and ML-KEM-768 inside the SPQR
   post-quantum ratchet. signal-cli 0.14.7 exercises all three paths (PQXDH,
   SPQR and CDSI/SVR2).
3. ML-KEM-1024 *as a libsignal prekey type* (`0x0A`) exists in the source but
   is compiled out of the shipped build.

## Q1. KEM key types in libsignal v0.99.1

libsignal-protocol defines a one-byte KEM type registry (S3 `kem.rs`):

| Type byte | Name in libsignal | Algorithm | In shipped build? |
|-----------|-------------------|-----------|-------------------|
| `0x07` | Kyber768 | round-3 Kyber768 | **No**. Behind Cargo feature `kyber768`, which no workspace crate enables. |
| `0x08` | Kyber1024 | round-3 Kyber1024 (libcrux "Kyber" variant) | **Yes**. Always compiled. |
| `0x0A` | MLKEM1024 | FIPS 203 ML-KEM-1024 | **No**. Behind Cargo feature `mlkem1024` of libsignal-protocol, which no crate in the workspace enables (checked every `Cargo.toml` at S3). The protocol crate's Cargo.toml says this feature "may still change incompatibly". |

The Java API mirrors this. `KEMKeyType` has exactly one member, `KYBER_1024`
with value 8 (S3 `KEMKeyType.kt`). The bridge hard-codes Kyber1024 as the
type for key generation (S3 `rust/bridge/shared/src/protocol.rs`).

Consequences for a compatible implementation:
- Deserializing a KEM public key, KEM secret key or KEM ciphertext whose
  first byte is anything other than `0x08` is an error ("bad KEM key type").
  This includes `0x0A`.
- An empty input is a separate error ("no key type identifier").
- The length must match **exactly**: type byte plus raw length. Trailing
  bytes are rejected. This differs from EC public keys; see `xeddsa.md`.

### Which Kyber exactly

- **Round 3**, the CRYSTALS-Kyber specification as submitted to round 3 of
  the NIST PQC process. It is the standard SHAKE/SHA-3 instantiation, not
  the "90s" AES/SHA-2 variant.
- libsignal calls libcrux-ml-kem's `kyber1024` functions. Internally these
  use a "Kyber" parameterization of the shared ML-KEM code that differs from
  ML-KEM in exactly three places (S4 `variant.rs`; details below).
- **Revision:** the outputs equal the KATs in the 2020-10-01 round-3 package
  (spec v3.0, S6). They also equal libcrux's own Kyber KATs, which come from
  the draft-cfrg-schwabe-kyber model. That draft declares itself to match
  v3.02 (S7). So for Kyber1024 the v3.0 package KATs and v3.02 behaviour are
  the same at the byte level, and either document describes the algorithm in
  use. This is an inference from matching vectors; the 2021 v3.01 and v3.02
  revisions are not recorded as changing Kyber1024's input/output behaviour.
- It is **not** the NIST "draft ML-KEM" (FIPS 203 ipd, August 2023), and it
  is not final FIPS 203. libcrux also carries ipd KATs (`nistkats_mlkem_ipd_*`);
  those belong to a different variant.

### Serialized forms and lengths (type `0x08`)

| Object | Raw length | Serialized in libsignal | Notes |
|--------|-----------:|-------------------------|-------|
| Public key | 1568 | 1569 = `0x08` followed by 1568 | Raw layout as in Kyber: 4×384-byte encoded NTT-domain vector t̂, then the 32-byte seed ρ. Byte-identical layout to an ML-KEM-1024 encapsulation key. |
| Secret key | 3168 | 3169 = `0x08` followed by 3168 | **Expanded form**, not a seed: 1536-byte encoded ŝ, then the 1568-byte public key, then the 32-byte SHA3-256 of the public key, then the 32-byte rejection value z. This is the Kyber round-3 secret-key layout, which has the same shape as the FIPS 203 expanded decapsulation key. libsignal never stores the 64-byte generation seed. |
| Ciphertext | 1568 | 1569 = `0x08` followed by 1568 | **Ciphertexts carry the type prefix too.** Decapsulation first checks the prefix is a known type, then that the total length is right, then that the ciphertext's type equals the secret key's type. A mismatch gives the error "wrong KEM key type". |
| Shared secret | 32 | 32, no prefix | |

Where these bytes appear:
- **Kyber prekey records** (local storage): a KyberPreKeyRecord reuses the
  signed-prekey storage structure. Its public-key field holds the 1569-byte
  serialized public key and its private-key field holds the 3169-byte
  serialized secret key. An implementation that has to read existing
  signal-cli account databases must therefore parse the 3168-byte expanded
  key. It cannot assume a seed.
- **Uploaded prekey entities / bundles**: the public key in serialized
  1569-byte form. The signature over it is described below.
- **PreKeySignalMessage**: the Kyber ciphertext field carries the 1569-byte
  serialized ciphertext, including `0x08`.

### Generation and signing of Kyber prekeys

- signal-cli 0.14.7 generates every Kyber one-time prekey and the
  last-resort Kyber prekey with key type `KYBER_1024`, so type `0x08`
  (S1 `KeyUtils.java`). The last-resort key is generated the same way as
  the one-time keys; the only difference is how it is registered with the
  server.
- Key generation draws 64 random bytes. The first 32 are the CPA seed d and
  the last 32 are z (S4 `ind_cca.rs`; same ordering as S7 section 11.1).
- The prekey signature is an XEd25519 signature by the account's identity
  private key over the **1569-byte serialized public key, including the
  `0x08` byte** (S1 `KeyUtils.java`; S3 `kyber_prekey.rs`). See `xeddsa.md`
  for the signature scheme.

### Processing a peer's prekey bundle (PQXDH)

- A bundle **must** contain a Kyber prekey ID, public key and signature.
  Without them, bundle construction fails with "kyber_pre_key_* is required"
  (S3 `state/bundle.rs`).
- Both the signed EC prekey signature and the Kyber prekey signature are
  verified against the peer identity key. Each is computed over the
  respective *serialized* (type-prefixed) public key. Either failure is a
  signature-validation error (S3 `session.rs`).
- **Accepted KEMs:** only what deserializes, which in the shipped build
  means `0x08` Kyber1024 only.
- The initiator encapsulates to the peer's Kyber prekey and puts the
  prefixed ciphertext in the PreKeySignalMessage. The responder looks up the
  Kyber prekey by ID and decapsulates.
- The PQXDH KDF's HKDF info label is the ASCII string
  `WhisperText_X25519_SHA-256_CRYSTALS-KYBER-1024` (S3 `pqxdh.rs`). Its
  output also seeds SPQR. The full PQXDH key schedule is out of scope here
  and needs its own spec.
- PreKeySignalMessages with message version 3 (pre-Kyber X3DH) are
  rejected as an invalid message ("X3DH no longer supported"), unless they
  match an already established session (S3 `session.rs`).
- The responder rejects the initiator's EC base key unless it is
  "canonical"; see `xeddsa.md`.
- No FIPS-203-style public-key validation (the coefficient modulus check)
  is done on Kyber public keys before encapsulation. libsignal checks only
  the length (S3 `kem/kyber1024.rs`). The round-3 Kyber spec defines no such
  check either. This agent did not determine how libcrux treats an encoded
  coefficient ≥ q in that case. Honestly generated keys never contain one.

### Other KEM uses in libsignal v0.99.1

| Use | KEM | Exercised by signal-cli 0.14.7? |
|-----|-----|-------------------------------|
| PQXDH session setup (above) | Kyber1024 round 3, type `0x08` | Yes, for every new 1:1 session, both sending and receiving. |
| **SPQR** ("Sparse Post-Quantum Ratchet", the PQ half of the "triple ratchet"; crate `spqr` v1.5.3, S5) | **FIPS 203 ML-KEM-768**, through libcrux-ml-kem 0.0.10's *incremental* API (crate features `incremental`, `mlkem768`) | **Yes.** libsignal v0.99.1 starts every new session with SPQR version 1 and sets the minimum version to 1 as well ("Require that all clients speak SPQR"; S3 `ratchet.rs`). Signal messages carry a `pq_ratchet` field (S3 `protocol.rs`). SPQR splits ML-KEM-768 encapsulation keys and ciphertexts into chunks and uses libcrux-specific incremental state encodings. Symbol names in the bundled native library show a byte-order fix-up and a workaround for "state incorrectly encoded by libcrux issue 1275". **SPQR needs its own spec before implementation.** |
| CDSI (contact discovery) and SVR2 (PIN / registration-lock backup) enclave channels | **FIPS 203 ML-KEM-1024** in the Noise pattern `Noise_NKhfs_25519+Kyber1024_ChaChaPoly_SHA256`. The name says "Kyber1024", but the implementation calls libcrux `mlkem1024`, which is final FIPS 203 (S3 `client_connection.rs`, `enclave.rs`, `snow_resolver.rs`). CDSI and SVR2 select the post-quantum handshake type; the handshake overhead constant is 64 + 1568 bytes. | **Yes.** CDSI is used for recipient lookups (S1 `RecipientHelper.java`, `CdsiV2Service`); SVR2 is used by PIN handling (S1 `PinHelper.java`). |
| HSM enclave client | none (plain `Noise_NK_25519_ChaChaPoly_SHA256`) | n/a |
| Sealed sender v1/v2 | none. Its "KEM" is X25519-based; the source uses the word only for the construction. | n/a |
| HPKE helper (`rust/crypto/src/hpke.rs`) | DHKEM(X25519, HKDF-SHA256) only, AES-256-GCM, base mode | Not determined. No signal-cli call site was looked for. |

Noise hfs detail relevant to Go (from the Noise KEM-extension semantics and
S3 `snow_resolver.rs`): the client is the initiator. It generates an
**ephemeral** ML-KEM-1024 key pair from 64 random bytes, sends the
1568-byte encapsulation key in its first message, and decapsulates the
1568-byte ciphertext in the responder's reply. When a party acts as
encapsulator, the received public key goes through the FIPS 203 validity
check first. The KEM keys are ephemeral, so the expanded-versus-seed storage
question never comes up on this path.

### Go `crypto/mlkem` compatibility

Go's `crypto/mlkem` implements final FIPS 203 (S10): 1568-byte
encapsulation key, 1568-byte ciphertext, 32-byte shared key, and the
decapsulation key held and exported only as a 64-byte seed `d || z`.

- **Kyber1024 (`0x08`, PQXDH prekeys): not compatible.** Key generation,
  encapsulation and decapsulation all produce different bytes; see the next
  section. Even the public-key bytes from the same 64-byte seed differ,
  because key generation hashes the seed differently. Go's API also cannot
  import a 3168-byte expanded secret key, and libsignal and signal-cli
  storage hold only the expanded form. The implementation needs its own
  Kyber round-3 KEM, validated with `testdata/vectors/kyber1024_kat.json`.
- **ML-KEM-1024 in the CDSI/SVR2 Noise handshake: compatible.** The
  encapsulation-key encoding, the ciphertext and the 32-byte shared secret
  are standard FIPS 203. libcrux's 64-byte key-generation input is `d || z`
  in the same order as Go's seed. libcrux internally holds the expanded
  3168-byte FIPS 203 decapsulation key, but that is never serialized on this
  path.
- **ML-KEM-768 in SPQR:** the KEM math is FIPS 203, but SPQR uses
  libcrux's incremental, split-key API and its encodings. Go's
  `crypto/mlkem` has no such API. Whether SPQR can be built on Go's ML-KEM
  is a question for the SPQR spec.
- **ML-KEM-1024 as prekey type `0x0A`:** not reachable in this build. If it
  were enabled, libsignal would serialize the secret as the 3168-byte
  expanded key, not the seed.

## Round-3 Kyber1024 compared with ML-KEM-1024, at the byte level

Sources: S7 sections 6, 10.2 and 11, plus S8 Appendix C by reference,
corroborated by S4 `variant.rs`.

**Shared by both.** The parameters are the same: n = 256, q = 3329, k = 4,
η1 = η2 = 2, d_u = 11, d_v = 5. The sizes are the same: pk 1568, sk 3168,
ct 1568, ss 32. The NTT, sampling, compression and encodings are the same,
and so is the inner CPA public-key encryption scheme. The symmetric
primitives are the same: H = SHA3-256, G = SHA3-512 split into two 32-byte
halves, PRF(s, b) = SHAKE-256(s || b), XOF = SHAKE-128. The secret-key
layout is the same (ŝ, pk, H(pk), z). In both, encapsulation derives
(K̄, r) as G(m || H(pk)) and encrypts m with coins r.

**Kyber also defines** KDF(x) = the first 32 bytes of SHAKE-256(x).

**The differences:**

1. **Key-generation seed domain separation.** Kyber expands the 32-byte
   CPA seed d with G(d), giving (ρ, σ). ML-KEM expands G(d || k), where k is
   a single byte holding the module rank (`0x04` for the 1024 parameter
   set). Everything downstream is identical, so the same d gives different
   keys.
2. **Encapsulation randomness pre-hash.** Kyber samples 32 random bytes and
   uses m = H(random) as the message. ML-KEM uses the 32 random bytes
   directly as m.
3. **Shared-secret derivation.** Kyber's shared secret is
   KDF(K̄ || H(c)), that is the first 32 bytes of SHAKE-256 over the 32-byte
   K̄ followed by the 32-byte SHA3-256 of the ciphertext. ML-KEM's shared
   secret is K̄ itself.
4. **Implicit rejection on decapsulation failure** (the re-encrypted
   ciphertext differs from the received one). The Kyber spec (S7 section
   11.3) outputs KDF(z || H(c)). ML-KEM outputs J(z || c), the first 32
   bytes of SHAKE-256(z || c). On success Kyber outputs KDF(K̄′ || H(c)) and
   ML-KEM outputs K̄′. The comparison and selection must be constant-time in
   both.
   - **libcrux deviation (observed in S4 `ind_cca.rs`, not a spec
     difference):** for the Kyber variant, libcrux computes the rejection
     output as KDF(J(z || c) || H(c)). That is SHAKE-256 over [the first 32
     bytes of SHAKE-256(z || c)] followed by SHA3-256(c), truncated to 32
     bytes. The Kyber spec says KDF(z || H(c)). The KATs cover only valid
     ciphertexts, so they cannot tell the two apart. **Interop impact:
     none.** The rejection value is pseudorandom and unknown to the sender
     either way, so the session keys never match and the message fails
     authentication. A spec-conformant implementation interoperates. Only a
     test that feeds deliberately corrupted ciphertexts and compares outputs
     with libsignal would notice.
5. **Input validation.** FIPS 203 requires an encapsulation-key modulus
   check and a decapsulation-key hash check. Round-3 Kyber has neither, and
   libsignal performs neither on `0x08` keys; it checks lengths only.

## Reproducing the KATs (`testdata/vectors/kyber1024_kat.json`)

The upstream file and its hashes are recorded in the JSON `source` block.
The full `.rsp` holds 100 entries, counts 0 to 99. Each gives `seed` (48
bytes), `pk`, `sk`, `ct` and `ss`. The seeds are listed explicitly, so
reproducing an entry needs only the per-entry DRBG below. The master DRBG
that produced the seed list does not need to be reproduced.

**DRBG.** NIST's PQC KAT procedure uses the CTR_DRBG of SP 800-90A Rev. 1
(S9, section 10.2.1) with these choices:
- AES-256 as the block cipher, so a 256-bit key and a 128-bit block.
- **No derivation function**, so seedlen = 384 bits = 48 bytes.
- No prediction resistance, no personalization string, no additional input.
- No reseeding during a KAT entry.

**Instantiate** with entropy_input = the entry's 48-byte `seed`. Start
from Key = 32 zero bytes and V = 16 zero bytes, then apply the CTR_DRBG
Update function with provided_data = seed. Update works as follows:
- Repeat three times: increment V as a 128-bit big-endian integer (mod
  2^128), encrypt V under Key, and append the 16-byte output.
- XOR the resulting 48 bytes with provided_data.
- Take the first 32 bytes as the new Key and the last 16 as the new V.

**Generate(n)**, one call per randomness request:
- Repeatedly increment V (big-endian, mod 2^128), encrypt V under Key and
  append the output until at least n bytes exist. Return the first n bytes.
- Then apply Update with provided_data = 48 zero bytes. With no additional
  input, the Update step still runs.

Each request is its own Generate call. Two separate 32-byte requests
therefore yield different bytes from one 64-byte request.

**Order of randomness requests per entry, for Kyber.** This follows the
algorithm order in the round-3 specification (S6/S7): the CCA key
generation runs the CPA key generation first, then samples z, and
encapsulation samples one 32-byte value.
1. Key generation makes two separate 32-byte requests, first d (the CPA
   seed), then z. The resulting (pk, sk) must equal the entry's `pk` and
   `sk`.
2. Encapsulation to `pk` makes one 32-byte request. Kyber hashes this value
   with SHA3-256 to get m (difference 2 above). The resulting ciphertext
   and shared secret must equal `ct` and `ss`.
3. Decapsulating `ct` with `sk` must give `ss`.

Corroboration: libcrux's KAT generator (S4 `generate_kyber_kats.py`) draws
the key-generation randomness as two 32-byte reads followed by one 32-byte
read for encapsulation. Its resulting JSON agrees with all 100 entries of
the NIST file.

The NIST KAT harness does not define a derived 64-byte "seed" form for
Kyber. The 64-byte value d || z, built from the first two Generate outputs,
is what libcrux takes as its key-generation input.
