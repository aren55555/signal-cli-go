# SPQR chunking: systematic erasure code over GF(2^16)

Spec role: prose only. Source: P1 in `index.md` (SparsePostQuantumRatchet
@ v1.5.3, commit fd320484dcec89004021e6fdc7481825f5f261fa):
`src/encoding.rs`, `src/encoding/gf.rs`, `src/encoding/polynomial.rs`,
and their use in `src/v1/chunked/*`. (`src/encoding/round_robin.rs`
exists in the crate but nothing in the protocol uses it.)

SPQR splits each of its four byte strings (Hdr, Ek, Ct1, Ct2;
`protocol.md`, section 5) into a stream of 32-byte chunks. The first n
chunks are the string itself. Later chunks are parity. Any n chunks with
distinct indices rebuild the string. This is a Reed–Solomon-style code: 16
independent polynomials over GF(2^16), each evaluated at the chunk index.

## 1. The field GF(2^16)

- An element is a 16-bit unsigned integer. Bit i is the coefficient of x^i
  (polynomial basis).
- Addition and subtraction are both bitwise XOR.
- Multiplication is carry-less polynomial multiplication followed by
  reduction modulo the degree-16 polynomial
  **x^16 + x^12 + x^3 + x + 1** (as an integer: `0x1100B`).
- Division a / b = a × b^(2^16 − 2), computed by repeated squaring (b must
  not be 0). The protocol never divides by zero (section 3).
- An integer index k in 0..65535 is used as the field element whose bit
  pattern is k. Distinct indices are distinct elements.

## 2. Encoding a byte string

Input: a byte string of even length L. All SPQR strings have even length;
an odd length is an error, and so is L > 16 × 65536.

1. Split it into W = L/2 words. Word k is bytes 2k (high) and 2k+1 (low):
   **big-endian**.
2. Word k belongs to polynomial j = k mod 16, at point x = ⌊k/16⌋.
3. Polynomial j therefore has n_j points, at x = 0, 1, …, n_j − 1, where
   n_j = ⌊W/16⌋, plus 1 if j < W mod 16. Let P_j be the unique polynomial
   over GF(2^16) of degree < n_j that passes through them, i.e.
   P_j(x) = word (16x + j) for x < n_j. If n_j = 0, P_j is the zero
   polynomial.
4. **Chunk c** (c = 0, 1, 2, …) has a 16-bit index c and 32 data bytes:
   the 16 values P_0(c), P_1(c), …, P_15(c), each written big-endian, in
   that order.

All SPQR strings have W divisible by 16, so every polynomial has the same
n = L/32 points:

| Stream | L | n (data chunks) |
|--------|--:|----------------:|
| Hdr (hdr ‖ MAC) | 96 | 3 |
| Ek | 1152 | 36 |
| Ct1 | 960 | 30 |
| Ct2 (ct2 ‖ MAC) | 160 | 5 |

So **chunk c for c < n is simply bytes 32c … 32c + 31 of the string.**
Parity chunks (c ≥ n) hold P_j(c), which can be computed by Lagrange
interpolation through the n data points:

P_j(c) = Σ_{i=0}^{n−1} y_i × Π_{m≠i} (c − m) / (i − m)

where y_i is word (16i + j), and all arithmetic, including the
subtractions of indices, is in GF(2^16). Any correct method gives the same
values; the upstream code precomputes the Lagrange basis polynomials for
n = 3, 5, 30, 34, 36 and evaluates by powers of c.

**Emission order.** Each stream's encoder has a counter starting at 0. Every
time the state machine emits a chunk of that stream, it emits chunk
(counter mod 65536) and increments the counter. The counter is a 32-bit
value, but only its low 16 bits become the index, so indices would wrap
after 65536 chunks. A stream is never resent from the start. Ek chunks
sent as "Ek" and later as "EkCt1Ack" come from **one** encoder, so their
indices simply continue.

## 3. Decoding

A decoder is created for a known length L (so n_j is known per polynomial)
and keeps, for each polynomial j, a set of points (x, y) with distinct x.

**Adding chunk c with data d.** For each j in 0..15, the candidate point is
x = c, y = big-endian word j of d. Store it in polynomial j's set only if

- c < n_j (a data point, always useful), **or**
- the set currently holds fewer than n_j points;

and in either case only if no point with x = c is stored yet. The first
point for an x wins; a later chunk with the same index is ignored.

**Completion.** The decoder is complete when every polynomial j holds at
least n_j points. SPQR checks this after each added chunk (except in the
two places noted in `protocol.md`, section 7.1, where completion is
impossible) and moves on as soon as it holds.

**Output.** For every word position k (polynomial j = k mod 16, x = ⌊k/16⌋):
- if a point with that x is stored for polynomial j, its y is the word;
- otherwise, interpolate polynomial j through its n_j stored points with
  the **smallest** x values and evaluate it at x.

The words, big-endian, give the L-byte string.

Practical consequence for SPQR, where every polynomial has the same n and
all points in a chunk share x: the string can be rebuilt as soon as n
distinct chunk indices have been received, in any order and with any
mixture of data and parity chunks. If the n received indices include
every data index 0..n−1, no interpolation is needed.

Integrity: chunks carry no checksum of their own. With honest data, which
points are chosen makes no difference. The choice rules above (first
point per x wins; interpolate through the smallest x values) matter only
for inconsistent input. In libsignal every SPQR message is covered by the
SignalMessage MAC, and the reassembled values are then checked by SPQR
itself: Hdr and Ct2 by their MACs, ct1 by the ciphertext MAC, Ek by the
hash in the header (`protocol.md`, `mlkem-incremental.md`).

## 4. Check cases from upstream tests (behaviour, not fixed bytes)

The upstream unit tests (P1 `src/encoding/polynomial.rs`) use random or
self-computed data, so they give behaviours to check rather than bytes:

- 10-byte message `abcdefghij`: five polynomials with one point each and
  eleven with none. Chunks 1 and 2 alone (no data chunk) decode it.
- 1088 bytes of `0x03`: n = 34. The test feeds parity chunks 35, 36, …,
  70 one at a time and stops at the first successful decode, which should
  come after the 34th chunk (index 68).
- Interpolating 30 random points at x = 0..29, evaluating at x = 30..59,
  and interpolating those again gives back the original values.
