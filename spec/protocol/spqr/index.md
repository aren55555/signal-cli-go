# SPQR (Sparse Post-Quantum Ratchet) in libsignal v0.99.1

Spec role: prose only. This directory describes what a byte-compatible
implementation of SPQR must do, as used by libsignal v0.99.1 (the
libsignal-client bundled with signal-cli v0.14.7). It is written from the
sources listed below. Nothing here is code. Constants, KDF labels, byte
layouts and protobuf field numbers are interoperability facts.

## Files

| File | Covers |
|------|--------|
| `index.md` (this file) | Sources, bottom line, glossary, test vectors, open gaps |
| `session.md` | How libsignal wires SPQR into a 1:1 session: seeding from PQXDH, mixing into message keys, the `pq_ratchet` field, version negotiation, libsignal-level error handling |
| `protocol.md` | The SPQR state machine: top-level send/receive with version negotiation, the symmetric key chain, the authenticator, the 11-state ML-KEM "braid", every KDF |
| `chunking.md` | The erasure code: GF(2^16), polynomial encoding, chunk layout, reassembly |
| `mlkem-incremental.md` | Which ML-KEM-768 pieces travel separately, in FIPS 203 terms; key and state layouts; the libcrux issue 1275 workaround |
| `wire.md` | Byte layout of an SPQR message; its place in SignalMessage |
| `state.md` | How SPQR state is persisted in libsignal's SessionRecord (summary, for data-dir import) |

Suggested reading order for an implementer: this file, `session.md`,
`protocol.md`, `wire.md`, `chunking.md`, `mlkem-incremental.md`, and
`state.md` only when importing an existing signal-cli data directory.

## Sources

| Ref | What | URL @ version |
|-----|------|---------------|
| P1 | SPQR crate `spqr` 1.5.3 | https://github.com/signalapp/SparsePostQuantumRatchet @ tag v1.5.3 = commit fd320484dcec89004021e6fdc7481825f5f261fa. Files read: `README.md`, `Cargo.toml`, `build.rs`, `src/lib.rs`, `src/chain.rs`, `src/authenticator.rs`, `src/authenticator/serialize.rs`, `src/kdf.rs`, `src/util.rs`, `src/serialize.rs`, `src/incremental_mlkem768.rs`, `src/encoding.rs`, `src/encoding/gf.rs`, `src/encoding/polynomial.rs`, `src/v1.rs`, `src/v1/unchunked/{send_ek,send_ct}.rs`, `src/v1/unchunked/*/serialize.rs`, `src/v1/chunked/{send_ek,send_ct,states}.rs`, `src/v1/chunked/*/serialize.rs`, `src/proto/pq_ratchet.proto`, `src/issue1275_{a,b}_state.in`; file listing of `src/test/` and `benches/` (searched for fixed vectors) |
| P2 | libsignal | https://github.com/signalapp/libsignal @ tag v0.99.1 = commit 97801d22dcf9f5bf714f7b8fa3212cdc973ae1c8. Files read: `Cargo.lock` (pins spqr 1.5.3 at the P1 commit, and libcrux-ml-kem 0.0.10), `rust/protocol/src/{ratchet.rs, ratchet/keys.rs, triple_ratchet.rs, protocol.rs, pqxdh.rs, consts.rs, identity_key.rs, session_management.rs, state/session.rs}`, `rust/protocol/src/proto/{wire.proto, storage.proto}`, `rust/protocol/cross-version-testing/tests/prespqr.rs`, `rust/protocol/tests/session.rs` (SPQR test only), `rust/core/src/lib.rs` (`derive_arrays`), `rust/bridge/shared/src/protocol.rs` (`SessionRecord_HasUsableSenderChain`) |
| P3 | libcrux-ml-kem 0.0.10 | https://github.com/cryspen/libcrux @ tag libcrux-ml-kem-v0.0.10 = commit c5fb80f37530ee9b2df9501ae5ff8cb4a973a4bd. Files read: `libcrux-ml-kem/Cargo.toml`, `src/mlkem.rs` (incremental API macro), `src/mlkem768.rs`, `src/ind_cca/incremental.rs`, `src/ind_cca/incremental/{types.rs, README.md}`, `src/ind_cca.rs` (`encaps_prepare`), `src/ind_cpa.rs` (`encrypt_c1`, `encrypt_c2`), `src/ntt.rs`, `src/polynomial.rs`, `src/vector/portable/vector_type.rs`, `src/vector/avx2.rs` (byte serialization of 16-bit words), `tests/nistkats.rs`, `tests/kats/{nistkats_mlkem_768.json, generate_mlkem_kats.py, README.md}`. libsignal's Cargo.lock pins the crates.io package with checksum 1d8160f7d64fd2716b4fd05cc886a042f8dcda18d9206c0d506e2c67bdf97daa; crates.io is blocked here, so this agent read the git tag and could not confirm that the tag and the package are byte-identical |
| P4 | Signal's published ML-KEM Braid specification | https://signal.org/docs/specifications/mlkembraid/ (Revision 1, 2025-02-21, last updated 2025-09-26). Skimmed for structure and state names only |
| P5 | Signal's published Double Ratchet specification, including SPQR and the Triple Ratchet | https://signal.org/docs/specifications/doubleratchet/ (Revision 4, 2025-11-04). Skimmed; libsignal's concrete choices in `session.md` take precedence over the document's recommendations |
| P6 | FIPS 203 (ML-KEM) | https://csrc.nist.gov/pubs/fips/203/final, by reference. The PDF host (nvlpubs.nist.gov) is blocked here, so it was not opened directly |
| P7 | RFC 5869 (HKDF) | https://datatracker.ietf.org/doc/html/rfc5869, by reference |

Blocked hosts: `crates.io` (see P3) and `nvlpubs.nist.gov` (see P6). The
GitHub REST API hit its anonymous rate limit when this agent tried to read
libcrux issue 1275, so that issue's text was not read; the description of
the issue here comes from the SPQR source (P1).

## Bottom line

- **SPQR can be built on a standard FIPS 203 ML-KEM-768 core** that
  exposes the internal K-PKE steps. Every ML-KEM value on the wire is
  standard: the ciphertext halves ct1 and ct2 concatenate to the standard
  1088-byte ciphertext, the header carries ρ and H(ek), and the second key
  part is the standard 1152-byte encoding of t̂. The peer decapsulates with
  plain FIPS 203 decapsulation. The only non-standard step is that the
  encapsulator must produce ct1 (the u part) knowing only ρ and H(ek), and
  produce ct2 (the v part) later once t̂ arrives. See `mlkem-incremental.md`.
- **The libcrux-specific encodings never reach the wire.** The 2080-byte
  "encapsulation state" and the issue 1275 byte-order repair are local
  state. They matter only when importing a libsignal SessionRecord that was
  saved in the middle of an epoch (`state.md`).
- **The hardest parts** are (1) getting the 11-state machine and its
  tolerance for out-of-order messages exactly right (`protocol.md`), (2) the
  Reed–Solomon-style erasure code over GF(2^16) (`chunking.md`), and (3)
  matching libsignal's out-of-order key store limits in the symmetric chain.
  None of these needs anything beyond SHA-256, HMAC, HKDF, SHA-3 and the
  ML-KEM core.
- libsignal v0.99.1 **requires** SPQR: every new session starts at SPQR
  version 1 with minimum version 1. A peer that sends no SPQR data cannot
  exchange messages with it (`session.md`).

## Glossary

| Term | Meaning here |
|------|--------------|
| A2B / B2A | SPQR direction. The session initiator (PQXDH "Alice") is A2B, the responder is B2A |
| auth key | The 32-byte value SPQR is seeded with; libsignal takes it from the PQXDH KDF output |
| SCKA | Sparse continuous key agreement: the ML-KEM part of SPQR, which produces one fresh 32-byte secret per epoch |
| epoch | SCKA round number, starting at 1. Each completed ML-KEM exchange yields the secret for one epoch |
| send_ek role | The party that generates the ML-KEM key pair in the current epoch (Alice in odd epochs, Bob in even epochs) |
| send_ct role | The party that encapsulates in the current epoch |
| hdr | The 64-byte ML-KEM "header": ρ followed by H(ek) |
| ek payload | The 1152-byte encoding of t̂ (the first 1152 bytes of the FIPS 203 encapsulation key) |
| ct1, ct2 | The first 960 and last 128 bytes of the FIPS 203 ML-KEM-768 ciphertext |
| chain | SPQR's symmetric KDF chain that turns epoch secrets into per-message 32-byte keys |
| message key (SPQR) | The 32-byte per-message output of SPQR, mixed into the Double Ratchet message keys |
| BE32 / BE64 | Unsigned integer as 4 / 8 bytes, big-endian |
| HKDF(salt, ikm, info, L) | HKDF-SHA256 (RFC 5869), extract then expand, L output bytes |

## Test vectors

All files are data only, with a `source` block giving upstream URL, tag,
commit, path and SHA-256.

| File | Entries | What it pins |
|------|--------:|--------------|
| `testdata/vectors/spqr_mlkem768_incremental_kat.json` | 100 | libcrux's ML-KEM-768 KATs: 64-byte key-generation seed, 32-byte encapsulation message m, SHA3-256 of ek, of the 2400-byte dk and of the 1088-byte ciphertext, and the shared secret. libcrux checks every entry through the same incremental API SPQR uses, and gets the same shared secret. The `source.description` explains how each field maps onto SPQR's hdr, ek payload, dk and ct1‖ct2 |
| `testdata/vectors/spqr_issue1275_states.json` | 1 (two states) | Two serialized SPQR states from SPQR's regression test for libcrux issue 1275, one of them holding a big-endian encapsulation state. Inputs for a state-import test and a self-consistency test; there are no fixed outputs |
| `testdata/vectors/spqr_varint.json` | 3 | The varint encode/decode cases from SPQR's message serializer tests |

**No fixed SPQR protocol vectors exist upstream.** Every SPQR test in P1
(state machine, chain, encoder/decoder, the top-level ratchet tests) draws
fresh OS randomness and only checks that the two parties agree. The same is
true of libsignal's SPQR tests (P2 `tests/session.rs` checks only that
`pq_ratchet` is non-empty; `prespqr.rs` checks that SPQR and pre-SPQR
versions cannot talk). The chain tests in P1 `chain.rs` use fixed inputs
(initial key `"1"`, epoch secret `0x02`) but compare only the two sides with
each other, never with fixed bytes. This agent did not make up expected
outputs. libsignal's Double Ratchet message-key test
(`rust/protocol/src/ratchet/keys.rs`, `test_chain_key_derivation`) is
fixed, but it covers only the no-SPQR-key case and belongs to the Double
Ratchet spec.

Self-tests an implementer can still run without upstream outputs:
- Two instances in lockstep, at random, and out of order, checking that the
  sender's message key always equals the receiver's (the upstream tests do
  exactly this).
- The ML-KEM-768 KATs, checking that the incremental split reproduces the
  one-shot results (`mlkem-incremental.md`).
- The erasure code: decode from any n distinct chunk indices
  (`chunking.md`).
- Loading the issue 1275 states and running 20 lockstep steps.

## Open gaps

- Whether signal-cli's Java layer (via `SessionRecord.hasSenderChain`)
  passes a nonzero "require PQ ratio" was not checked. That parameter
  decides whether a pre-SPQR stored session is still used for sending; see
  `session.md`.
- Whether the libcrux git tag is byte-identical to the crates.io package
  libsignal links was not checked (crates.io blocked).
- The exact signed range of the r̂ coefficients inside libcrux's
  encapsulation state was taken from libcrux's formal-verification
  postconditions, not tested. An importer should reduce them mod q
  (`mlkem-incremental.md`).
- No black-box capture of SPQR bytes from the reference binary was made.
  That would need a live peer, and Signal servers must not be contacted.
