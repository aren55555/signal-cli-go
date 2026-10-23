# SPQR state persistence (summary, for data-dir import)

Spec role: prose only. Sources: P1 `src/proto/pq_ratchet.proto` (proto3,
package `signal.proto.pq_ratchet`), `src/lib.rs`, `src/chain.rs`,
`src/authenticator/serialize.rs`, `src/encoding/polynomial.rs`,
`src/v1/**/serialize.rs`; P2 `rust/protocol/src/proto/storage.proto`,
`rust/protocol/src/state/session.rs`.

A Go implementation can store SPQR state however it likes. This file
matters only for reading SessionRecords written by signal-cli/libsignal.

## 1. Where it lives

`SessionStructure` (proto3, libsignal storage) field **15**
`pq_ratchet_state` (bytes) holds a serialized **PqRatchetState**. An empty
value means SPQR V0: a session created before SPQR. Every session state in
the record (current and archived) has its own field 15.

## 2. PqRatchetState and children

All messages are proto3, so zero or empty values are omitted when
written. Field types are protobuf types.

**PqRatchetState**

| # | Field | Type | Meaning |
|--:|-------|------|---------|
| 1 | version_negotiation | VersionNegotiation | The negotiation record; absent once a message has been received |
| 2 | chain | Chain | Absent until first created |
| 3 | v1 | V1State (only member of oneof `inner`) | Absent means V0 |

**VersionNegotiation** (nested in PqRatchetState): 1 auth_key bytes (32);
2 direction enum Direction; 3 min_version enum Version; 4 chain_params
ChainParams.

**Enums**: Version: V_0 = 0, V_1 = 1. Direction: A_2_B = 0, B_2_A = 1.

**ChainParams**: 1 max_jump uint32; 2 max_ooo_keys uint32. A value equal
to the library default (25000 / 2000) is **written as 0**, and 0 is read
back as the default. libsignal's self-session max_jump of 4294967295 is
written as is.

**Chain**: 1 direction Direction; 2 current_epoch uint64; 3 links repeated
Epoch (oldest first); 4 next_root bytes (32); 5 send_epoch uint64; 6 params
ChainParams (required when reading).

**Chain.Epoch**: 1 send EpochDirection; 2 recv EpochDirection (both
required when reading).

**Chain.Epoch.EpochDirection**: 1 ctr uint32; 2 next bytes (32, or empty
once erased; any other length is a decode error); 3 prev bytes, the
skipped-key store as a concatenation of 36-byte entries, each BE32(index)
‖ 32-byte key, in no particular order.

**Authenticator**: 1 root_key bytes (32); 2 mac_key bytes (32).

**V1State**: one oneof `inner_state` naming the SCKA state:

| # | State | Role |
|--:|-------|------|
| 1 | keys_unsampled | send_ek |
| 2 | keys_sampled | send_ek |
| 3 | header_sent | send_ek |
| 4 | ct1_received | send_ek |
| 5 | ek_sent_ct1_received | send_ek |
| 6 | no_header_received | send_ct |
| 7 | header_received | send_ct |
| 8 | ct1_sampled | send_ct |
| 9 | ek_received_ct1_sampled | send_ct |
| 10 | ct1_acknowledged | send_ct |
| 11 | ct2_sampled | send_ct |

Each of these "chunked" messages has field 1 `uc` (the core data, an
"unchunked" message) plus encoder/decoder fields:

| Chunked state | 1 uc (unchunked type) | Other fields |
|---------------|-----------------------|--------------|
| keys_unsampled | KeysUnsampled | — |
| keys_sampled | HeaderSent | 2 sending_hdr PolynomialEncoder |
| header_sent | EkSent | 2 sending_ek PolynomialEncoder; 3 receiving_ct1 PolynomialDecoder |
| ct1_received | EkSentCt1Received | 2 sending_ek PolynomialEncoder |
| ek_sent_ct1_received | EkSentCt1Received | 3 receiving_ct2 PolynomialDecoder (field 2 unused) |
| no_header_received | NoHeaderReceived | 2 receiving_hdr PolynomialDecoder |
| header_received | HeaderReceived | 2 receiving_ek PolynomialDecoder |
| ct1_sampled | Ct1Sent | 2 sending_ct1 PolynomialEncoder; 3 receiving_ek PolynomialDecoder |
| ek_received_ct1_sampled | Ct1SentEkReceived | 2 sending_ct1 PolynomialEncoder |
| ct1_acknowledged | Ct1Sent | 2 receiving_ek PolynomialDecoder |
| ct2_sampled | Ct2Sent | 2 sending_ct2 PolynomialEncoder |

Unchunked core messages (field 1 is always `epoch` uint64 and field 2
always `auth` Authenticator):

| Unchunked type | Further fields (number, name, length) |
|----------------|---------------------------------------|
| KeysUnsampled | — |
| HeaderSent | 3 ek (1152, the ek payload); 4 dk (2400) |
| EkSent | 3 dk (2400) |
| EkSentCt1Received | 3 dk (2400); 4 ct1 (960) |
| NoHeaderReceived | — |
| HeaderReceived | 3 hdr (64) |
| EkReceived | 3 hdr (64); 4 ek (1152) (declared, not used by the chunked states) |
| Ct1Sent | 3 hdr (64); 4 es (2080, encapsulation state); 5 ct1 (960) |
| Ct1SentEkReceived | 3 es (2080); 4 ek (1152); 5 ct1 (960) |
| Ct2Sent | — |

Lengths are checked when reading: a wrong length for ek, dk, hdr, es or
ct1 is a StateDecode error. dk is the standard 2400-byte ML-KEM-768
decapsulation key; es is the libcrux encapsulation state, which may need
the issue 1275 repair (`mlkem-incremental.md`, section 5).

**PolynomialEncoder** (`chunking.md`): 1 idx uint32, the next chunk
counter; then exactly one of these is non-empty:
- 2 pts, repeated bytes, exactly 16 entries. Entry j is polynomial j's
  data points y_0, y_1, … as 2-byte big-endian words. This is the state
  before any parity chunk has been produced; the words are the original
  string's words for polynomial j.
- 3 polys, repeated bytes, exactly 16 entries. Entry j is polynomial j's
  coefficients, lowest degree first, as 2-byte big-endian words. The
  encoder switches to this form the first time it produces a parity chunk.

**PolynomialDecoder**: 1 pts_needed uint32 (= string length / 2); 2 polys
uint32 (written as 16); 3 pts repeated bytes, exactly 16 entries, entry j
being polynomial j's stored points as 4-byte records BE16(x) ‖ BE16(y),
sorted by x; 4 is_complete bool (always written false in this version; a
true value makes the decoder refuse to output). When a stored decoder's
pts_needed does not match its state's string length (96/2, 1152/2,
960/2 or 160/2), reading fails with MsgDecode rather than StateDecode.
libsignal therefore reports it as an invalid message, not an invalid
state.

## 3. Reading semantics worth knowing

- An empty `pq_ratchet_state` is V0, with no negotiation record and no
  chain.
- A missing required sub-message (authenticator, chain params, epoch
  halves, the oneof) is a StateDecode error.
- `current_version` of a state is "still negotiating (version, minimum)"
  while field 1 is present, else "negotiation complete (version)". The
  version is V1 if field 3 is present, else V0.
- Examples: `testdata/vectors/spqr_issue1275_states.json` holds two real
  serialized states, both with negotiation already complete. One is in
  header_sent, the other in ct1_sampled with a byte-swapped es.
