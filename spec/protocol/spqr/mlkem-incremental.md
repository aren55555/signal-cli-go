# ML-KEM-768 as SPQR uses it (incremental encapsulation)

Spec role: prose only. Sources: P1 `src/incremental_mlkem768.rs` and
`src/v1/unchunked/*`; P3 (libcrux-ml-kem @ libcrux-ml-kem-v0.0.10,
commit c5fb80f37530ee9b2df9501ae5ff8cb4a973a4bd) `src/mlkem.rs`,
`src/ind_cca/incremental.rs`, `src/ind_cca/incremental/{types.rs,README.md}`,
`src/ind_cca.rs`, `src/ind_cpa.rs`, `src/ntt.rs`, `src/polynomial.rs`,
`src/vector/portable/vector_type.rs`, `src/vector/avx2.rs`,
`tests/nistkats.rs`; FIPS 203 (P6) by reference. FIPS 203 algorithm
numbers below follow the final standard: Algorithm 14 = K-PKE.Encrypt,
16 = ML-KEM.KeyGen_internal, 17 = ML-KEM.Encaps_internal,
18 = ML-KEM.Decaps_internal.

## 1. Summary

SPQR uses **final FIPS 203 ML-KEM-768** (k = 3, η1 = η2 = 2, d_u = 10,
d_v = 4), not round-3 Kyber. Every byte string that goes on the wire is a
standard ML-KEM value or a slice of one:

| SPQR name | Size | FIPS 203 meaning | On the wire? |
|-----------|-----:|------------------|--------------|
| hdr | 64 | ρ (32) ‖ H(ek) (32): the last 32 bytes of ek, then SHA3-256 of the whole 1184-byte ek | yes (Hdr stream, with MAC) |
| ek payload | 1152 | ByteEncode12(t̂): the first 1152 bytes of ek | yes (Ek stream) |
| ct1 | 960 | c1 = ByteEncode10(Compress10(u)): the first 960 bytes of the ciphertext | yes (Ct1 stream) |
| ct2 | 128 | c2 = ByteEncode4(Compress4(v)): the last 128 bytes of the ciphertext | yes (Ct2 stream, with MAC) |
| shared secret | 32 | K | no (input to the SCKA-key HKDF) |
| dk | 2400 | the standard expanded decapsulation key dk_PKE ‖ ek ‖ H(ek) ‖ z | no (local state) |
| encapsulation state | 2080 | libcrux-specific: ŷ, e2 and m as 16-bit words (section 5) | no (local state) |

The full encapsulation key ek is t̂-encoding ‖ ρ (1184 bytes). The full
ciphertext is ct1 ‖ ct2 (1088 bytes), and the receiver decapsulates it with
**ordinary** FIPS 203 decapsulation. libcrux's KAT test runs all 100
ML-KEM-768 KATs both through the one-shot API and through this incremental
path and requires identical shared secrets (data in
`testdata/vectors/spqr_mlkem768_incremental_kat.json`).

**Can it be built from a standard FIPS 203 core?** Yes. Key generation and
decapsulation are completely standard. Encapsulation is standard
Encaps_internal, run in two parts: the first needs only ρ and H(ek), the
second needs t̂. That needs access to K-PKE.Encrypt's internal steps,
which the in-repo FIPS 203 core provides. Go's `crypto/mlkem` alone is not
enough for the encapsulating side: it cannot derive the coins from a
supplied H(ek) or produce c1 before t̂ is known.

## 2. Key generation (send_ek role, state KeysUnsampled)

- Draw 64 random bytes. The first 32 are **d** and the last 32 are **z**.
- Run ML-KEM.KeyGen_internal(d, z) (Algorithm 16) for ML-KEM-768. This is
  the FIPS 203 variant, with G(d ‖ 0x03) domain separation.
- From the results:
  - **hdr** = ρ ‖ H(ek), where ρ = ek[1152:1184] and H = SHA3-256 over all
    1184 bytes of ek;
  - **ek payload** = ek[0:1152];
  - **dk** = the standard 2400-byte decapsulation key:
    ByteEncode12(ŝ) (1152) ‖ ek (1184) ‖ H(ek) (32) ‖ z (32).
    SPQR stores this expanded form, never the seed.

In libcrux's "compressed key pair" layout, hdr is dk[2304:2368] and the ek
payload is dk[1152:2304].

A Go implementation may generate the key pair with `crypto/mlkem`
(`NewDecapsulationKey768` from the 64-byte d ‖ z seed, same order) and
get the same ek. But `crypto/mlkem` cannot export or import the 2400-byte
expanded dk. An implementation that keeps only its own state can store
the seed instead. An importer of libsignal state needs a decapsulation
routine that takes the expanded dk (`state.md`).

## 3. Encapsulation in two parts (send_ct role)

### 3.1 First part (state HeaderReceived; produces ct1 and K)

Inputs: hdr = ρ ‖ h (h is the sender's claimed H(ek)), and 32 fresh random
bytes **m**.

1. (K, r) = G(m ‖ h): SHA3-512, K = first 32 bytes, r = last 32 bytes.
   This is Encaps_internal's first step, using h from hdr instead of
   hashing an ek you do not have yet. m is used as is, with no pre-hash
   (unlike round-3 Kyber).
2. Run the parts of K-PKE.Encrypt (Algorithm 14) that need only ρ and r:
   - expand Â from ρ exactly as FIPS 203 does;
   - sample the vector y with PRF_η1(r, N) for N = 0, 1, 2; sample e1 with
     PRF_η2(r, N) for N = 3, 4, 5; sample e2 with PRF_η2(r, 6);
   - ŷ = NTT(y); u = NTT⁻¹(Âᵀ ∘ ŷ) + e1;
   - **ct1** = c1 = ByteEncode10(Compress10(u)) (960 bytes).
3. Keep, for the second part: ŷ, e2 and m (the "encapsulation state"),
   plus hdr and ct1 (SPQR keeps those too).
4. The shared secret is **K**. It is available now, before t̂ is known, and
   SPQR immediately derives the epoch secret from it (`protocol.md`).

### 3.2 Header check (before the second part)

When the 1152-byte ek payload has been rebuilt from chunks, SPQR checks it
against hdr and fails with ErroneousDataReceived if any check fails:
- rebuild ek = ek payload ‖ ρ (ρ = hdr[0:32]) and require
  SHA3-256(ek) = hdr[32:64];
- require ek to pass the FIPS 203 encapsulation-key modulus check: every
  12-bit value decoded from the 1152 bytes is below q = 3329. Equivalently,
  decoding and re-encoding gives the same bytes.

Since hdr itself is authenticated by the header MAC, this check
authenticates the ek payload.

### 3.3 Second part (produces ct2)

Using the stored ŷ, e2, m and t̂ = ByteDecode12(ek payload):
- μ = Decompress1(ByteDecode1(m));
- v = NTT⁻¹(t̂ᵀ ∘ ŷ) + e2 + μ;
- **ct2** = c2 = ByteEncode4(Compress4(v)) (128 bytes).

ct1 ‖ ct2 is exactly the ciphertext that ML-KEM.Encaps_internal(ek, m)
would output, and K is exactly its shared key. An implementation can
therefore simply keep (m, hdr) and, once t̂ arrives, rerun the standard
deterministic encapsulation on the full ek. That gives the same K and the
same ct1 already sent, plus ct2. Holding ŷ and e2 is an optimization, not
a requirement, except when importing libsignal state that lacks hdr
(section 5).

## 4. Decapsulation (send_ek role, state EkSentCt1Received)

Run ML-KEM.Decaps_internal(dk, ct1 ‖ ct2) (Algorithm 18) with the stored
2400-byte dk, including FIPS 203 implicit rejection (K̄ = J(z ‖ c) when the
re-encryption check fails). No input checks beyond the fixed sizes are
done on dk or the ciphertext. On a mismatch the result is a pseudorandom
K, so the ciphertext MAC check that follows fails (`protocol.md`,
section 7.1).

## 5. The libcrux encapsulation state and the issue 1275 workaround (local only)

libsignal persists the send_ct side's encapsulation state in states
Ct1Sampled, EkReceivedCt1Sampled and Ct1Acknowledged (`state.md`). It is
2080 bytes:

| Offset | Length | Content |
|-------:|-------:|---------|
| 0 | 1536 | ŷ: 3 polynomials × 256 coefficients, each a signed 16-bit integer, **little-endian**, polynomial 0 first and coefficients in FIPS 203 index order |
| 1536 | 512 | e2: 256 coefficients, signed 16-bit little-endian, each in −2…2 |
| 2048 | 32 | m (raw bytes, no byte order) |

The ŷ values are congruent mod q to NTT(y) in the standard domain (not
Montgomery form). libcrux's formal-verification contract for its NTT says
so, and the code ends with a Barrett reduction. They are signed and not
necessarily in 0…q−1, so an importer must reduce each value mod 3329. This
range detail was not tested.

**Issue 1275.** According to SPQR's source comments, libcrux versions
before libcrux PR 1276 serialized these 16-bit words **big-endian** in the
portable (non-SIMD) backend. In 0.0.10 the portable backend writes
little-endian and the AVX2 backend uses a native (little-endian) store.
Saved states from affected builds can therefore hold the first 2048 bytes
byte-swapped. Before the second encapsulation part, SPQR
inspects and, if needed, repairs the state:

- Walk the e2 region (offsets 1536 … 2047) as 256 words read
  little-endian, in order. For each word:
  - 0x0000 or 0xFFFF (0 or −1, which read the same either way): keep
    going;
  - 0x0001, 0x0002 or 0xFFFE (1, 2, −2 read correctly): the state is fine;
    stop and use it unchanged;
  - 0x0100, 0x0200 or 0xFEFF (1, 2, −2 byte-swapped): the state is
    swapped; stop, swap the two bytes of every 16-bit word in offsets
    0 … 2047 (leave the final 32 bytes of m alone), and use the result;
  - any other value: the state is not recognizable; stop and use it
    unchanged (upstream logs a warning).
- If all 256 words are 0 or −1, use it unchanged.

The repaired state is used only to compute ct2. The stored copy is not
rewritten by this step.

**Wire impact: none.** The workaround only fixes how a local state is
read. Without it, a swapped state would give a wrong ct2. The peer's
decapsulation would then return a pseudorandom key and the ciphertext MAC
check would fail. With it, the bytes on the wire are standard. A Go
implementation that writes its own state never needs this logic. It
matters only when importing a libsignal SessionRecord saved by an
affected build in one of the three states above.
`testdata/vectors/spqr_issue1275_states.json` contains such a state.

Importing: Ct1Sampled and Ct1Acknowledged also store hdr, so an importer
could recompute everything from m (the last 32 bytes) and hdr (section
3.3). EkReceivedCt1Sampled stores the ek payload but not hdr, so neither ρ
nor H(ek) is available there. In that state the importer must use ŷ and e2
from the stored state (with the issue 1275 check) to compute ct2.
