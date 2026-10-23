# XEd25519 and X25519 in libsignal v0.99.1 (as bundled by signal-cli v0.14.7)

Spec role: prose only. The algorithm itself is defined in Signal's public
XEdDSA specification. This file records only how libsignal departs from
it, where libsignal uses it, and which fixed vectors exist. Nothing in this
file is code.

## Sources

| Ref | What | URL @ version |
|-----|------|---------------|
| X1 | XEdDSA/VXEdDSA specification, Revision 1, 2016-10-20 | https://signal.org/docs/specifications/xeddsa/ (fetched 2026-10-23) |
| X2 | libsignal | https://github.com/signalapp/libsignal @ tag v0.99.1 = commit 97801d22dcf9f5bf714f7b8fa3212cdc973ae1c8: `rust/core/src/curve.rs`, `rust/core/src/curve/curve25519.rs`, `rust/core/examples/{xed,ed_to_xed}.rs`, `rust/protocol/src/{identity_key,sealed_sender,protocol,session,pqxdh,session_management,group_cipher}.rs`, `rust/protocol/src/state/kyber_prekey.rs`, `rust/keytrans/src/vrf.rs`, `java/client/src/test/java/org/signal/libsignal/protocol/ecc/Curve25519Test.java`, `node/ts/test/protocol/ProtocolTest.ts`, `swift/Tests/LibSignalClientTests/PublicAPITests.swift` |
| X3 | curve25519-dalek 5.0.0 (the version libsignal-core resolves) | https://github.com/dalek-cryptography/curve25519-dalek @ tag curve25519-5.0.0 = commit 07bef73ff85998a206cd2cea7f2605c801d0d1c9: `curve25519-dalek/src/montgomery.rs`, `curve25519-dalek/src/field.rs` |
| X4 | signal-cli | https://github.com/AsamK/signal-cli @ tag v0.14.7 = commit b01b6b370dc063599a1a2b9fde0f5ff4e2d78fe8: `lib/src/main/java/org/asamk/signal/manager/util/KeyUtils.java`; class-file string search in the distribution jars `libsignal-cli-0.14.7.jar`, `signal-service-java-2.15.3_unofficial_151.jar` and `signal-network-2.15.3_unofficial_151.jar` |
| X5 | Ed25519 and X25519 | RFC 8032 (https://www.rfc-editor.org/rfc/rfc8032) and RFC 7748 (https://www.rfc-editor.org/rfc/rfc7748) |

## Which variant is used

- **XEd25519 (XEdDSA on Curve25519, SHA-512)** is the only signature
  scheme on Curve25519 identity and prekey material.
- **VXEdDSA is not used anywhere** in libsignal v0.99.1, and therefore not
  by signal-cli 0.14.7. A search of the Rust, Java, Node and Swift sources
  finds no VXEdDSA. The only VRF in the tree is in the key-transparency
  crate, and it is ECVRF-EDWARDS25519-SHA512-TAI from RFC 9381, not
  VXEdDSA. signal-cli 0.14.7 does not reference key transparency at all.

## Where XEd25519 is used, and whether signal-cli 0.14.7 exercises it

In every case below, "message" means the exact bytes that are signed. Each
signature is 64 bytes.

| Use | Signer key / verifier key | Signed bytes | signal-cli 0.14.7 |
|-----|---------------------------|--------------|-------------------|
| Signed EC prekey | identity key | the 33-byte serialized prekey public key (`0x05` followed by u) | Signs its own prekeys (`KeyUtils`); verifies peers' prekeys when processing bundles. |
| Kyber prekey (one-time and last-resort) | identity key | the 1569-byte serialized KEM public key (`0x08` followed by 1568 bytes); see `kem.md` | Signs and verifies, as above. |
| Sealed sender: server certificate | trust root / trust root public key | the `certificate` field (field 1, bytes) of the ServerCertificate protobuf; the signature is field 2 | Verifies, when receiving sealed-sender messages. |
| Sealed sender: sender certificate | server certificate key | the `certificate` field of the SenderCertificate | Verifies. |
| Sender-key (group) message | the sender's sender-key signing key | the whole serialized SenderKeyMessage up to the signature: one version byte followed by the protobuf body. The 64-byte signature is appended at the end. | Signs and verifies (group messaging). |
| Alternate-identity ("PNI") signature | one identity key signs the other identity key | 32 bytes of `0xFF`, then the 20 ASCII bytes `Signal_PNI_Signature`, then the 33-byte serialized other identity key. All are concatenated with no length prefixes. | Verifies (signal-cli `IncomingMessageHandler` calls verifyAlternateIdentity). Signs through signal-service-java's message sender (`signAlternateIdentity` appears in `SignalServiceMessageSender`). |
| Payment address signature | identity key | MobileCoin address bytes (layout not examined) | Present in signal-cli `PaymentUtils` (signs and verifies). |
| Backup/archive credential presentation | backup-derived EC key | not examined | A `calculateSignature` call exists in signal-network `ArchiveApi`. This agent did not determine whether signal-cli reaches it. |

Not XEd25519:
- **Device linking/provisioning** carries no XEd25519 signature of its own.
  A linking device produces the signed EC prekeys and Kyber prekeys above
  for both its ACI and PNI identities.
- **Usernames** use zero-knowledge proofs (poksho), not XEd25519.
- **zkgroup** server signatures use their own scheme (for example,
  GroupsV2Operations calls a zkgroup `verifySignature`).

libsignal's signing and verification calls accept a message given in
several parts. The parts are simply concatenated; there is no framing.

## Key encodings and validation

- **Public key, serialized:** 33 bytes: the type byte `0x05` ("DJB" type),
  then the 32-byte little-endian Montgomery u-coordinate. This form is used
  everywhere a "PublicKey"/"IdentityKey" is serialized, signed or put on the
  wire.
- **Deserialization quirks:**
  - Empty input is rejected ("no key type identifier").
  - A first byte other than `0x05` is rejected ("bad key type").
  - Fewer than 32 bytes after the type byte is rejected.
  - **More than 33 bytes is accepted**: trailing bytes are ignored, with a
    logged warning, and re-serializing yields the 33-byte form.
  - The Java `ECPublicKey` follows the same rules (KEY_SIZE 33).
  - A separate constructor takes exactly 32 raw bytes, with no type byte.
- **Private key:** exactly 32 bytes, any other length rejected. On import
  libsignal **clamps** the bytes as in RFC 7748: it clears the low 3 bits
  of byte 0, clears bit 7 of byte 31 and sets bit 6 of byte 31. The
  *clamped* value is what gets stored, re-serialized, and used for both
  X25519 and signing. Freshly generated keys are 32 random bytes, clamped
  the same way.
- **X25519 agreement:** plain RFC 7748 X25519 with no input validation.
  libsignal then rejects an **all-zero shared secret** with an "invalid key
  agreement" error, compared in constant time.
- **PQXDH base-key check** (responder side; X2 `pqxdh.rs`, `curve.rs`):
  before any computation, the initiator's EC base key must be "canonical".
  Two conditions apply:
  - **Range:** bit 255 of u must be clear and u must be below 2^255 − 19.
  - **Torsion-free:** convert u to an Edwards point with sign 0 (as in the
    verification step below). The conversion must succeed and the point
    must have no small-order component.

  A failing key gives the error "incoming base key is invalid". The range
  half has fixed vectors in `xeddsa.json` (kind
  `montgomery_u_range_check`); the torsion half has only randomized
  upstream tests.

## Deviations from the signal.org XEdDSA specification

The libsignal source says it "varies slightly from that paper". The
example tool `ed_to_xed.rs` says libsignal "accommodates Ed25519 signing
keys with a sign bit of 1 by expecting the sign bit of the signature to be
set to 1 as well". In detail:

### 1. The sign bit travels in the signature

- X1 forces the Edwards public key's sign bit to 0. When kB has sign bit 1,
  it negates the private scalar (calculate_key_pair).
- **libsignal never negates.** It signs with a = k mod q, where k is the
  clamped 32-byte private key read as a little-endian integer. It uses
  A = the true compressed Edwards encoding of kB, including that point's
  real sign bit.
- The signer copies A's sign bit into **bit 7 of the last signature byte**
  (byte 63, the top bit of S). S is always below q < 2^253, so that bit is
  otherwise always 0.
- The verifier reads bit 7 of byte 63 as the Edwards sign bit when
  converting the Montgomery u to an Edwards point. It then clears that bit
  before treating bytes 32 to 63 as S.
- Consequence: every libsignal signature from a key whose kB has sign bit 1
  **fails** a verifier that follows X1 literally (sign bit forced to 0). It
  also fails a verifier that does not clear the bit, because S then looks
  ≥ 2^255. About half of all keys are affected. The positive vectors in
  `xeddsa.json` include both sign values.

### 2. Nonce hash input

- X1 computes r = hash_1(**a** || M || Z) mod q, where **a** is the
  32-byte encoding of the (possibly negated) reduced scalar.
- libsignal hashes the **clamped private key bytes k as stored**: not
  reduced mod q, never negated. Clamping sets bit 254, so k > q always, and
  the bytes therefore always differ from the encoding of k mod q.
- The hash_1 prefix is X1's, unchanged: one byte `0xFE` followed by 31
  bytes `0xFF`. The full input is that prefix, then the 32 key bytes, then
  the message, then the 64 random bytes Z.
- The 512-bit SHA-512 output, read as a little-endian integer, is reduced
  mod q.
- Consequence: for the same k, M and Z, libsignal's (R, S) differs from an
  X1-literal signer's, even when the sign bit is 0. Both verify. This
  matters only for byte-exact reproduction of signatures with a fixed Z.
  Upstream has no such vector.

### 3. Challenge hash and response

- h = SHA-512(R || A || M) mod q, where A is the 32-byte Edwards encoding
  **with its real sign bit**.
- S = r + h·a mod q, encoded as 32 bytes little-endian, and then bit 7 of
  byte 63 is overwritten by A's sign bit.

These steps are as in X1 and Ed25519, apart from which A is used.

### 4. Random input Z

64 bytes from the OS CSPRNG on every signature, as X1 requires. It is never
exposed through the API.

### 5. Verification checks, compared with X1

| Check | X1 | libsignal v0.99.1 |
|-------|----|-------------------|
| Signature length | 2b bits = 64 bytes | any length other than 64 bytes → invalid (returns false, no error) |
| u ≥ p | reject | **not rejected.** Bit 255 of u is ignored; X1's convert_mont also masks it. A value in [p, 2^255) is used reduced mod p (X3 field decoding). |
| u = p − 1 (that is, −1) | (not on curve) | reject: the birational map is undefined |
| A on curve | reject if not | reject if Edwards decompression of (y = (u−1)/(u+1), sign = signature bit) fails |
| R encoding | reject R.y ≥ 2^255 | R is never decoded. The verifier compares the *canonical* encoding of sB − hA byte for byte with the received R, so any non-canonical R fails. |
| S range | reject S ≥ 2^253 | the same: after clearing bit 7 of byte 63, reject if any of bits 5 to 7 of byte 63 are set. **S is not required to be < q.** It is reduced mod q before use. |
| Equation | cofactorless, R == sB − hA | the same (cofactorless, byte comparison) |

Notes for a Go implementation:
- A libsignal XEd25519 signature verifies as a standard Ed25519 signature
  (RFC 8032) under these conditions:
  - The 32-byte Ed25519 public key is built from u as
    y = (u − 1)/(u + 1) mod p, encoded little-endian, with bit 255 set to
    bit 7 of signature byte 63.
  - The signature passed to the Ed25519 verifier has that bit cleared.

  This agent checked every positive vector in `xeddsa.json` that way with
  OpenSSL's Ed25519 verifier.
- Go's `crypto/ed25519` is **stricter** than libsignal in one respect: it
  rejects S ≥ q. libsignal accepts S in [q, 2^253). Honest signers never
  produce such S, so this matters only for adversarial or contrived inputs.
  To match libsignal exactly, an implementation has to accept an unreduced
  S below 2^253.

## Test vectors (`testdata/vectors/xeddsa.json`)

The file has 94 entries:

- **xeddsa_verify, 68 entries.**
  - 1 positive vector from `curve25519.rs` `test_signature`, the same data
    as Java `Curve25519Test.testSignature`. Its message is a 33-byte
    serialized public key and its sign bit is 1.
  - 64 negative vectors that upstream derives in the same test by flipping
    bit 0 of each signature byte in turn. All must be rejected.
  - 3 positive vectors from the constant `KNOWN_SERVER_CERTIFICATES` in
    `sealed_sender.rs`: certificate id 2 (staging trust root, sign bit 0),
    id 3 (production trust root, sign bit 1) and the test certificate
    0x7357C357 (sign bit 1). For the test certificate, an upstream comment
    gives the trust root's private key as 32 zero bytes.
  - **No vector fixes the random input Z**; upstream signs only with fresh
    randomness. The vectors therefore test verification only. Signing can
    be tested by round trip, and against an independent Ed25519 verifier
    using the conversion described above.
- **x25519_agreement, 1 entry**, from `test_agreement` and Java
  `testAgreement` (same data).
- **x25519_public_key_derivation, 2 entries**: the all-zero and all-`0xFF`
  private keys behind the sealed-sender test certificate. The relationship
  is stated upstream; the values were confirmed with OpenSSL X25519.
- **montgomery_u_range_check, 23 entries**, from `curve.rs`
  `keys_with_the_high_bit_set_are_out_of_range` and
  `keys_above_the_prime_modulus_are_out_of_range`.

The signal.org XEdDSA document contains no test vectors. The Node and Swift
test suites contain no fixed XEd25519 vectors; their signature tests use
freshly generated keys. Upstream negative cases beyond the bit-flip loop
(wrong message, wrong key) are also randomized, so they are not included.
