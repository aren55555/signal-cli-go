# SPQR inside a libsignal v0.99.1 session

Spec role: prose only. Sources: P1, P2, P5 in `index.md`. Item numbers
in brackets refer to P2 files: [ratchet] `rust/protocol/src/ratchet.rs`,
[keys] `ratchet/keys.rs`, [tr] `triple_ratchet.rs`, [proto]
`protocol.rs`, [pqxdh] `pqxdh.rs`, [sm] `session_management.rs`,
[state] `state/session.rs`, [wire] `proto/wire.proto`, [bridge]
`rust/bridge/shared/src/protocol.rs`, [core] `rust/core/src/lib.rs`.

This file covers only the interface between SPQR and the rest of the
session. PQXDH itself and the Double Ratchet are specified elsewhere. The
SPQR operations named here ("initialize", "send", "receive") are defined
in `protocol.md`.

## 1. Seeding SPQR from PQXDH

The PQXDH key schedule ends with one HKDF call [pqxdh]:

- HKDF-SHA256 with **no salt** (which RFC 5869 treats as 32 zero bytes),
  IKM = the PQXDH secret input (defined in the PQXDH spec), info = the
  46-byte ASCII string `WhisperText_X25519_SHA-256_CRYSTALS-KYBER-1024`,
  output length **96** bytes.
- The 96 bytes are cut in order into three 32-byte values [core,
  `derive_arrays`]:

| Bytes | Use |
|-------|-----|
| 0–31 | Double Ratchet initial root key |
| 32–63 | Double Ratchet initial chain key (index 0) |
| 64–95 | **SPQR auth key** |

Both parties derive the same 96 bytes, so both get the same SPQR auth key.
No further label is applied before SPQR sees it; SPQR's own KDFs
(`protocol.md`) take the 32 bytes as they are.

## 2. Initial SPQR parameters

Each new session state gets a fresh SPQR state with these parameters
[ratchet]:

| Parameter | Initiator (sent the PreKeySignalMessage; PQXDH "Alice") | Responder (PQXDH "Bob") |
|-----------|------------------|-----------------|
| auth key | bytes 64–95 above | bytes 64–95 above |
| direction | A2B | B2A |
| version | V1 | V1 |
| minimum version | V1 | V1 |
| chain max_jump | 25000, or 4294967295 (2^32 − 1) for a self session | same rule |
| chain max_ooo_keys | 2000 | 2000 |

"Self session" means the remote identity key equals the local identity key
and the remote address's service ID equals the local one, so the session
is between two devices of the same account. The flag is fixed when the
session is created and stored inside the SPQR state. 25000 and 2000 are
libsignal's `MAX_FORWARD_JUMPS` and `MAX_MESSAGE_KEYS`.

The SPQR state is opaque bytes to the rest of the session. It is stored
in `SessionStructure.pq_ratchet_state` (field 15); see `state.md`.

## 3. Encrypting one message

Order of operations for every outgoing SignalMessage, whether or not it is
wrapped in a PreKeySignalMessage [tr, `OutgoingTripleRatchet::encrypt`]:

1. Run SPQR **send** on the session's SPQR state. It returns a new state,
   an SPQR message (bytes) and an optional 32-byte SPQR message key. SPQR
   send consumes randomness (key generation or encapsulation) in some
   states. An SPQR error aborts encryption ("post-quantum ratchet send
   error", reported as an invalid-state error).
2. Take the Double Ratchet sending chain key CK with its index N. Compute
   the message-key seed MKS = HMAC-SHA256(key = CK, data = the single
   byte `0x01`). This is the normal Double Ratchet step.
3. Derive the message keys (section 4) from MKS and the SPQR key.
4. AES-256-CBC encrypt and build the SignalMessage. Its `pq_ratchet`
   field holds the SPQR message bytes (section 5). The MAC covers the
   whole serialized SignalMessage, `pq_ratchet` included.
5. Only after success: advance CK (HMAC with byte `0x02`) and store the new
   SPQR state.

With libsignal's parameters (V1, minimum V1) the SPQR key is present on
every message, from the very first PreKeySignalMessage on. The SPQR
message is never empty.

## 4. Message-key derivation (the hybrid KDF)

[keys, `MessageKeys::derive_keys` and `MessageKeyGenerator::generate_keys`]

- HKDF-SHA256 with
  - **salt = the 32-byte SPQR message key** if SPQR returned one;
    otherwise no salt (32 zero bytes);
  - **IKM = MKS** (the 32-byte Double Ratchet message-key seed);
  - **info = `WhisperMessageKeys`** (18 ASCII bytes);
  - output length **80** bytes.
- Bytes 0–31 are the AES-256 key, bytes 32–63 the HMAC-SHA256 key used for
  the 8-byte SignalMessage MAC, and bytes 64–79 the CBC IV.

So SPQR changes only the HKDF salt. With no SPQR key, the result is exactly
the pre-SPQR libsignal derivation.

Skipped (out-of-order) Double Ratchet keys are stored as their seed MKS
plus index, not as finished keys. The SPQR salt is applied when such a
message finally arrives, using the SPQR key that SPQR returns for that
message. Old stored entries that hold finished keys (no seed) come from
pre-SPQR sessions; using one together with an SPQR key is treated as an
internal error (assertion).

## 5. The `pq_ratchet` field

SignalMessage [wire]:

| Field | Number | Type | Notes |
|-------|-------:|------|-------|
| ratchet_key | 1 | bytes | unchanged |
| counter | 2 | uint32 | unchanged |
| previous_counter | 3 | uint32 | unchanged |
| ciphertext | 4 | bytes | unchanged |
| **pq_ratchet** | **5** | **bytes (proto2 optional)** | The SPQR message, layout in `wire.md` |
| addresses | 6 | bytes | unchanged |

- The sender **omits** field 5 when the SPQR message is empty (an SPQR V0
  state, section 7) and otherwise writes it. libsignal's encoder writes
  fields in number order, so field 5 sits between ciphertext and
  addresses.
- The receiver treats a missing field 5 as an empty byte string.
- The SignalMessage version nibble stays 4 (current). SPQR has no
  separate version on the outer message; its own first byte carries the
  SPQR version (`wire.md`).
- **PreKeySignalMessage** gets no new field. Its embedded SignalMessage
  (field 4) carries `pq_ratchet` like any other. Nothing about SPQR is
  sent in clear PQXDH fields: the auth key comes from the PQXDH KDF, and
  both sides start their SPQR state when they create the session.

## 6. Decrypting one message

[tr, `TripleRatchet::decrypt`; sm, `try_decrypt_with_state` and the
session-trial loop]

For each candidate session state (the current one first, then archived
ones), libsignal works on a **copy** and does:

1. Double Ratchet: find or create the receiver chain for the sender's
   ratchet key and get the message-key seed for the counter. Duplicates
   are detected here, before SPQR runs.
2. SPQR **receive** on that candidate's SPQR state with the message's
   `pq_ratchet` bytes. It returns a new state and an optional SPQR key.
3. Derive the message keys (section 4), verify the SignalMessage MAC and
   the optional address binding, then AES-CBC decrypt.
4. Only if all of steps 1–3 succeed: write back the Double Ratchet state
   and the new SPQR state, and clear the "unacknowledged PreKey" marker.
   If any step fails, the candidate is left exactly as it was and the next
   candidate is tried.

If no candidate succeeds, the result is the usual "decryption failed"
invalid-message error. A "duplicated message" error stops the search
immediately, as before.

Error mapping for step 2: if SPQR reports that its **stored state** cannot
be decoded, libsignal reports an invalid-state error for that candidate.
Any other SPQR error (unparseable message, MAC failure inside SPQR, epoch
out of range, key already used, key too far ahead, version mismatch,
minimum version not met, data inconsistent with the header) is reported
as an invalid message for that candidate. Either way the candidate is
skipped and nothing is saved. The full list of SPQR errors is in
`protocol.md`, section 9.

Since the SPQR message sits inside the MAC'd SignalMessage, a forged or
corrupted SPQR message cannot pass step 3 unless the attacker also knows
the message keys. libsignal therefore never commits SPQR state derived
from unauthenticated bytes.

## 7. Version negotiation and peers without SPQR

SPQR has two versions: V0 (disabled: empty messages, no keys) and V1. The
full negotiation rules are in `protocol.md`, section 2. Their effect in
libsignal v0.99.1:

| Situation | Result |
|-----------|--------|
| Both sides on v0.99.1 (V1, minimum V1) | Every message carries SPQR data and an SPQR key |
| Peer sends a message with no `pq_ratchet` (an SPQR V0 message) to a v0.99.1 session | SPQR receive fails with "minimum version" (while the negotiation record still exists) or "version mismatch" (after the first successful receive). That message fails to decrypt (P2 `prespqr.rs`, `test_alice_lacks_spqr`) |
| v0.99.1 sends to a peer that ignores `pq_ratchet` | The peer derives message keys without the salt, its MAC check fails, and the message fails to decrypt on its side (P2 `prespqr.rs`, `test_bob_lacks_spqr`) |
| SPQR message whose first byte is greater than 1 (a future version) | Ignored without error: SPQR state unchanged, no SPQR key. In a v0.99.1 session the sender of such a message would have used a key, so the MAC check then fails |
| Stored session created before SPQR existed (`pq_ratchet_state` empty) | SPQR runs as V0: send produces an empty message and no key, so field 5 is omitted and no salt is used; receive returns no key whatever arrives (V1 data is accepted and ignored). Such sessions keep working with peers in the same situation |

The minimum version is V1 on both sides, so libsignal v0.99.1 never
downgrades a new session to V0.

**Session usability.** [state, bridge] A session "uses SPQR" if its stored
SPQR state is non-empty. The bridge function behind Java's
`SessionRecord.hasSenderChain(now, requirePqRatio)` first requires a
usable, not-stale sender chain. If the session was also established with
PQXDH and uses SPQR, it is usable. Otherwise it is usable when the ratio
is 0. For a ratio above 0 (values outside 0–1 are clamped) the bridge
decides from the ratio and the session's stored base key, so the answer
is fixed for a given session rather than random. This agent did not read
that helper (`should_use_nonpq_session`) in detail. Which ratio
signal-cli's Java layer passes was not checked (`index.md`, open gaps).
This only decides whether to keep sending on a pre-SPQR session or start a
new one; it does not affect the SPQR wire protocol.

## 8. Summary of SPQR-related constants used by libsignal

| Constant | Value |
|----------|-------|
| PQXDH KDF info | `WhisperText_X25519_SHA-256_CRYSTALS-KYBER-1024` (46 bytes), output 96, SPQR auth key = bytes 64–95 |
| Message keys info | `WhisperMessageKeys` (18 bytes), output 80, salt = SPQR key |
| SignalMessage field | 5, `pq_ratchet`, bytes |
| SessionStructure field | 15, `pq_ratchet_state`, bytes |
| SPQR version / minimum | V1 / V1 |
| SPQR max_jump | 25000 (self session: 4294967295) |
| SPQR max_ooo_keys | 2000 |
