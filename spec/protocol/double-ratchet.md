# Double Ratchet as libsignal v0.99.1 runs it (signal-cli v0.14.7)

Spec role: prose only. Nothing in this file is code. Read it together with
Signal's published Double Ratchet specification
(https://signal.org/docs/specifications/doubleratchet/) and Sesame
specification (https://signal.org/docs/specifications/sesame/), which the
implementation role reads directly. This file fixes every parameter those
documents leave open and lists where libsignal differs from them.

Related: `pqxdh.md` (how the first root key, chain key and ratchet keys
come about), `spec/crypto/xeddsa.md` (key encodings, X25519 all-zero check),
`spec/protocol/spqr/` (the post-quantum ratchet; another agent's spec).
libsignal calls the combination of this ratchet and SPQR the "triple
ratchet".

## Sources

| Ref | What | URL @ version |
|-----|------|---------------|
| D1 | libsignal | https://github.com/signalapp/libsignal @ tag v0.99.1 = commit 97801d22dcf9f5bf714f7b8fa3212cdc973ae1c8: `rust/protocol/src/{double_ratchet,triple_ratchet,ratchet,session_management,protocol,consts,crypto,identity_key}.rs`, `rust/protocol/src/ratchet/keys.rs`, `rust/protocol/src/state/session.rs`, `rust/protocol/src/proto/{wire,storage,service}.proto`, `rust/crypto/src/aes_cbc.rs`, `rust/core/src/{lib,address}.rs`, `rust/bridge/shared/src/protocol.rs`, `rust/protocol/tests/session.rs`, Java `SessionRecordTest.java`, `PlaintextContentTest.java` |
| D2 | signal-cli | https://github.com/AsamK/signal-cli @ tag v0.14.7 = commit b01b6b370dc063599a1a2b9fde0f5ff4e2d78fe8: `lib/src/main/java/org/asamk/signal/manager/storage/sessions/SessionStore.java` |
| D3 | Reference distribution | `/work/third_party/signal-cli-0.14.7/lib/libsignal-client-0.99.1.jar`, `signal-service-java-2.15.3_unofficial_151.jar` (`javap` of `SessionCipher`, `SessionRecord`, `SignalServiceCipher`) |
| D4 | prost-derive (protobuf encoder; Cargo.lock pins 0.14.4) | https://github.com/tokio-rs/prost @ tag v0.14.1, `prost-derive/src/lib.rs` |
| D5 | Signal Double Ratchet and Sesame specifications | https://signal.org/docs/specifications/doubleratchet/, https://signal.org/docs/specifications/sesame/ (by reference) |

## 1. Primitives

| Double Ratchet function | libsignal v0.99.1 |
|-------------------------|-------------------|
| GENERATE_DH | X25519 key pair; private key 32 random bytes, clamped (`xeddsa.md`) |
| DH | X25519; an all-zero result is an error |
| KDF_RK | HKDF-SHA-256, §2 |
| KDF_CK | HMAC-SHA-256 with one-byte inputs, §3 |
| ENCRYPT | AES-256-CBC with PKCS#7 padding, then a separate truncated HMAC over the whole serialized message, §3 and §5 |
| HEADER | not encrypted; the protobuf fields of §4 |
| MAX_SKIP | 25 000 per step (§6) |

## 2. KDF_RK (root-key step)

- HKDF-SHA-256 with **salt = the current 32-byte root key**,
  **IKM = the 32-byte X25519 output** (our ratchet private key with their
  ratchet public key), **info = ASCII `WhisperRatchet`** (14 bytes),
  output **64 bytes**.
- Bytes 0..31 = new root key; bytes 32..63 = new chain key, index 0.

A full DH ratchet step on receiving an unknown remote ratchet key R:
1. (RK1, CKr) = KDF_RK(RK, DH(our current sending ratchet private, R)).
   CKr becomes a new receiving chain keyed by R, index 0.
2. Generate a new sending ratchet key pair N.
3. (RK2, CKs) = KDF_RK(RK1, DH(N private, R)). RK2 is the new root key;
   (N, CKs, index 0) is the new sending chain.
4. previous counter := (old sending chain index − 1), floored at 0. See the
   note below.
5. The receiving chain is appended at the end of the receiving-chain list.
   If the list then holds more than **5** chains, the oldest (first) is
   dropped (`MAX_RECEIVER_CHAINS`).

Note on the previous counter (PN): the signal.org document sets PN to the
number of messages sent in the previous sending chain. libsignal stores the
index of the last key used, i.e. that number minus one (and 0 if nothing was
sent). The receiver ignores PN entirely: it is parsed but never used to
pre-compute skipped keys. So this only matters for byte-exact
reproduction of libsignal's messages.

## 3. KDF_CK and message keys

Chain step, with CK the current 32-byte chain key:
- **message-key seed** = HMAC-SHA-256(key = CK, data = the single byte `0x01`);
- **next chain key** = HMAC-SHA-256(key = CK, data = the single byte `0x02`),
  index + 1.

Message keys from the seed:
- HKDF-SHA-256 with **IKM = the 32-byte seed**, **info = ASCII
  `WhisperMessageKeys`** (18 bytes), output **80 bytes**.
- **Salt = the SPQR message key for this message**, if SPQR supplied one;
  otherwise no salt (RFC 5869's default, 32 zero bytes). This is the only
  point where SPQR output enters 1:1 message encryption. In sessions without
  SPQR state the salt is always absent, which reproduces the classic
  libsignal derivation.
- Split: bytes 0..31 = **AES-256 cipher key**, 32..63 = **HMAC-SHA-256 MAC
  key**, 64..79 = **AES-CBC IV** (16 bytes).

Encryption: AES-256-CBC with that key and IV, PKCS#7 padding (a full block
of padding when the plaintext is a multiple of 16). Decryption rejects a
ciphertext that is empty or not a multiple of 16 bytes, and rejects bad
padding. The plaintext handed to this layer is already the padded Signal
`Content` (padding of the Content is a higher-layer matter and is not
specified here).

Order of operations when sending one message:
1. Ask SPQR for (new SPQR state, optional 32-byte key, SPQR message bytes).
2. Derive the message keys from the current sending chain key's seed, with
   the SPQR key as salt.
3. Encrypt, build and MAC the SignalMessage (§4, §5) with the current chain
   index as counter.
4. Advance the sending chain (next chain key) and store the new SPQR
   state. The state is committed only if the whole encrypt succeeded and
   the remote identity is still trusted.

Order when receiving (one session state):
1. Find the receiving chain for the message's ratchet key, or perform the DH
   ratchet step of §2.
2. Obtain the message-key seed for the counter (§6): a stored skipped seed,
   or by advancing the chain.
3. Feed the message's `pq_ratchet` bytes to SPQR, getting (new SPQR state,
   optional key). A malformed SPQR message is "invalid message"; an
   undecodable stored SPQR state is "invalid state".
4. Derive the message keys with the SPQR key as salt.
5. Verify the MAC (§5), including the address binding if present.
6. AES-CBC decrypt.
7. Only if all steps succeed: commit the ratchet changes and the new SPQR
   state, and clear the pending-prekey information (`pqxdh.md` §4).

Skipped message keys are stored as **(counter, 32-byte seed)**, not as
expanded keys, because the SPQR salt is only known when the message
arrives. Old records may hold entries with explicit cipher key, MAC key and
IV and an empty seed; such entries are used as is and are only valid for
messages that carry no SPQR key.

Fixed vector: `testdata/vectors/ratchet_kdf.json`.

## 4. Wire format: SignalMessage (ciphertext message type 2)

```
byte 0              : version byte = (session_version << 4) | 4
bytes 1 .. n-9      : protobuf "SignalMessage" (proto2)
bytes n-8 .. n-1    : MAC, 8 bytes
```

- High nibble: the session version (4 for PQXDH sessions; 3 for old X3DH
  sessions). Low nibble: always 4. Receivers read only the high nibble.
- Minimum total length 9 bytes; shorter → "ciphertext message too short".
- High nibble < 3 → "legacy ciphertext version"; > 4 → "unrecognized
  ciphertext version".

Protobuf fields (package `signal.proto.wire`), encoded in ascending field
order, every present field written, zero values included:

| Field # | Name | Type | Content |
|--------:|------|------|---------|
| 1 | ratchet_key | bytes | sender's current ratchet public key, 33 bytes (`0x05` + u). Required. |
| 2 | counter | uint32 | index of the sending chain key used (first message on a chain = 0). Required. |
| 3 | previous_counter | uint32 | see §2 note. Absent reads as 0. |
| 4 | ciphertext | bytes | AES-256-CBC output. Required. |
| 5 | pq_ratchet | bytes | SPQR message bytes; **omitted when empty**. Contents are defined in `spec/protocol/spqr/`. Absent reads as empty. |
| 6 | addresses | bytes | optional address binding, 36 bytes, see below |

Missing field 1, 2 or 4 → "invalid protobuf encoding". The ratchet key goes
through the normal 33-byte EC key decoder.

### Address binding (field 6), new in this libsignal generation

Layout, 36 bytes, no separators:

| Bytes | Content |
|-------|---------|
| 0..16 | sender service ID, fixed-width binary: 1 kind byte (`0x00` ACI, `0x01` PNI) then the 16 UUID bytes in standard (big-endian, RFC 4122) order |
| 17 | sender device ID (1..127) |
| 18..34 | recipient service ID, same 17-byte form |
| 35 | recipient device ID |

- **Sending:** libsignal fills field 6 only for the SignalMessage embedded
  in a PreKeySignalMessage, i.e. while the session is unacknowledged.
  Ordinary SignalMessages omit it. It is also omitted if either protocol
  address's name does not parse as a service ID (an ACI is a hyphenated
  UUID string; a PNI is `PNI:` followed by one).
- **Receiving:** after the MAC verifies, if field 6 is present it must equal
  the 36 bytes computed from (sender = the remote address the message came
  from, recipient = our local address). A mismatch, or local addresses that
  are not service IDs, makes the MAC check fail. If field 6 is absent the
  message is accepted.
- signal-service-java builds the local address from the local account's
  service-ID string and local device ID.

## 5. MAC

- HMAC-SHA-256 keyed with the 32-byte MAC key from §3 (any other key length
  is an error).
- Input, concatenated in this order with no length prefixes:
  1. the **sender's** identity public key, 33-byte serialized form
     (`0x05` + u);
  2. the **receiver's** identity public key, 33 bytes;
  3. the serialized message from the version byte through the end of the
     protobuf (everything except the trailing 8 bytes), exactly as
     received.
- Truncated to the **first 8 bytes**.
- When sending, "sender" is our identity and "receiver" the remote one; when
  verifying, the roles swap. The identity keys come from the session state
  (the identity the session was established with, ACI or PNI).
- Compared in constant time. A bad MAC is not an error by itself in the
  per-state attempt; it makes that state's decrypt fail (see §8).

## 6. Counters and limits

| Constant | Value | Meaning |
|----------|------:|---------|
| `MAX_FORWARD_JUMPS` | 25 000 | maximum (counter − current receiving-chain index) accepted in one message. A jump of exactly 25 000 is allowed; 25 001 fails. **Unlimited for a self-session** (`pqxdh.md` §4). |
| `MAX_MESSAGE_KEYS` | 2 000 | maximum skipped-key entries kept **per receiving chain** |
| `MAX_RECEIVER_CHAINS` | 5 | receiving chains kept per session state |
| `ARCHIVED_STATES_MAX_LENGTH` | 40 | archived session states kept per record |
| `MAX_UNACKNOWLEDGED_SESSION_AGE` | 30 days | after this, an unacknowledged session can no longer send (`pqxdh.md` §4) |

Receiving a counter c on a chain whose next index is i:
- **c < i:** look for a stored skipped seed with index c on that chain. If
  found, it is removed and used. If not found → **"duplicated message"**
  (carries i and c). This happens *before* any MAC check, and it stops the
  whole decrypt: archived states are not tried.
- **c − i > max jump:** → invalid message, "message from too far into the
  future".
- **otherwise:** for every index from i up to c − 1, store its seed as a
  skipped key, then use the seed at index c; the chain's next index becomes
  c + 1.
- Skipped-key list order: each new entry is inserted at the **front**;
  when the list exceeds 2 000 entries, the **last** (oldest) entry is
  dropped. Lookup takes the first entry whose index matches.
- If the ratchet key is unknown, the DH ratchet step of §2 runs first, so
  the jump is measured from index 0 of the new chain. Keys skipped on the
  *previous* receiving chain because of PN are not pre-computed (PN is
  ignored).

## 7. Failure behaviour and state mutation

- Per-state decryption works on a copy. **A failed decrypt never mutates
  the stored session**: not the root key, chains, skipped keys, SPQR state
  or pending info. This holds for bad MAC, bad padding, SPQR errors,
  too-far-future counters and duplicates.
- Error kinds visible to callers:

| Situation | libsignal error | Java exception |
|-----------|-----------------|----------------|
| no session record / no current state when sending | session not found | `NoSessionException` |
| no session record when receiving a SignalMessage | session not found | `NoSessionException` |
| version nibble out of range | legacy / unrecognized ciphertext version | `LegacyMessageException` / `InvalidVersionException` |
| message version ≠ state version (per state) | unrecognized message version (recorded; next state tried) | (folded into the final error) |
| counter already used | duplicated message | `DuplicateMessageException` |
| every state failed (bad MAC, too far ahead, SPQR, bad padding, address mismatch) | invalid message "decryption failed" | `InvalidMessageException` |
| session state structurally broken (bad key lengths, missing chain) | invalid session structure | (Java mapping not determined by this agent) |
| remote identity no longer trusted (checked **after** a successful decrypt) | untrusted identity; the new state is not stored | `UntrustedIdentityException` |

## 8. Session selection (Sesame-related behaviour)

For an incoming SignalMessage from address A (one record per
(service ID, device ID)):
1. Try the current state, if its session version equals the message's
   version. Success → store it as current and finish.
2. Otherwise try each archived state, newest first, skipping undecodable
   ones and ones with a different version. The first success is
   **promoted** to current (the old current state is archived; see
   `pqxdh.md` §7). This is Sesame's "activate the matching session".
3. "Duplicated message" from any state ends the search immediately with
   that error.
4. If nothing worked → invalid message "decryption failed". Nothing is
   written.

The inner SignalMessage of a PreKeySignalMessage is tried against the
current state only (`pqxdh.md` §7).

Encrypting always uses the current state. If that state still has pending
prekey info, the output is a PreKeySignalMessage (`pqxdh.md` §5), else a
SignalMessage.

signal-cli treats a record as active when the current state has a
non-stale sending chain (it passes PQ ratio 0.0; `pqxdh.md` §7).

## 9. Session versions accepted

- **Version 4** (PQXDH, Kyber1024): the only version created; normal case.
- **Version 3** (X3DH): SignalMessages with high nibble 3 still parse and
  decrypt if a matching version-3 state exists in the record (old
  sessions). New version-3 sessions are refused (`pqxdh.md` §7).
- Versions below 3 are rejected at parse time; above 4 likewise.
- A stored state with session version 0 reads as version 2 (very old),
  which can never match a parsable message.

## 10. Session record storage format (for reading signal-cli data)

`RecordStructure` (proto3, package `signal.proto.storage`):

| Field # | Name | Type |
|--------:|------|------|
| 1 | current_session | SessionStructure (message) |
| 2 | previous_sessions | repeated bytes, each a serialized SessionStructure, newest first |

`SessionStructure`:

| Field # | Name | Type | Notes |
|--------:|------|------|-------|
| 1 | session_version | uint32 | 4 for PQXDH sessions |
| 2 | local_identity_public | bytes | 33 bytes |
| 3 | remote_identity_public | bytes | 33 bytes |
| 4 | root_key | bytes | 32 |
| 5 | previous_counter | uint32 | |
| 6 | sender_chain | Chain | |
| 7 | receiver_chains | repeated Chain | oldest first; the newest is appended |
| 9 | pending_pre_key | PendingPreKey | present while unacknowledged |
| 10 | remote_registration_id | uint32 | |
| 11 | local_registration_id | uint32 | |
| 12 | (reserved) | | |
| 13 | alice_base_key | bytes | 33; the initiator's base key, on both sides |
| 14 | pending_kyber_pre_key | PendingKyberPreKey | present while unacknowledged |
| 15 | pq_ratchet_state | bytes | serialized SPQR state (opaque here) |

`Chain`: 1 sender_ratchet_key (bytes, 33), 2 sender_ratchet_key_private
(bytes, 32; only in the sending chain), 3 chain_key (ChainKey), 4
message_keys (repeated MessageKey; only in receiving chains).
`ChainKey`: 1 index (uint32, the **next** index to use), 2 key (bytes, 32).
`MessageKey`: 1 index (uint32), 2 cipher_key, 3 mac_key, 4 iv (bytes;
legacy only), 5 seed (bytes, 32; current form).
`PendingPreKey`: 1 pre_key_id (optional uint32), 3 signed_pre_key_id
(int32), 2 base_key (bytes, 33), 4 timestamp (uint64, seconds since the
Unix epoch).
`PendingKyberPreKey`: 1 pre_key_id (uint32), 2 ciphertext (bytes, 1569).

Fixed parse vectors: `testdata/vectors/ratchet_records.json`.

## 11. PlaintextContent and DecryptionErrorMessage (type 8)

When a message cannot be decrypted, clients send a DecryptionErrorMessage
inside an unencrypted **PlaintextContent** (ciphertext message type 8).
Layout: one byte `0xC0`, then a serialized `Content` protobuf whose only
field is 8 (`decryption_error_message`, bytes), then one byte `0x80`
(padding boundary; no further padding). Parsing requires a non-empty input
whose first byte is `0xC0` (else "unrecognized message version").
`DecryptionErrorMessage` (proto2): 1 ratchet_key (bytes, 33; the failed
1:1 message's ratchet key, absent for sender-key messages), 2 timestamp
(uint64, ms), 3 device_id (uint32). Fixed vector in
`testdata/vectors/ratchet_messages.json`.

## 12. Interface with SPQR (summary, no internals)

- Session-record field 15 holds the SPQR state; SignalMessage field 5
  carries SPQR's per-message output; both are opaque to this layer.
- Per message, SPQR is called once (send or receive) and may return a
  32-byte key, used as the HKDF salt for the message keys (§3).
- SPQR receive runs after the Double Ratchet chose the message-key seed and
  before the MAC check; its state is committed only on full success.
- SPQR is initialized from bytes 64..95 of the PQXDH output with the
  parameters listed in `pqxdh.md` §9.

## 13. Test vectors

- `ratchet_kdf.json`: the only fixed Double Ratchet KDF vector in libsignal
  (Rust `ratchet/keys.rs` test): chain key → cipher key, MAC key, next chain
  key, with no SPQR salt. The spec agent recomputed it with OpenSSL (HMAC
  and HKDF) and it matched; the seed and IV values in the file are marked as
  derived by that recomputation, since upstream does not assert them. The
  file also has libsignal's AES-256-CBC vector.
- `ratchet_records.json`: SessionRecord, PreKeyRecord, SignedPreKeyRecord
  and KyberPreKeyRecord parse vectors from Java/Swift tests, plus the
  PQ-ratio decision vectors.
- `ratchet_messages.json`: the PlaintextContent / DecryptionErrorMessage
  bytes from Java `PlaintextContentTest`.
- No upstream test contains a fixed SignalMessage or PreKeySignalMessage
  with keys that decrypt it. All session tests generate keys at runtime.
