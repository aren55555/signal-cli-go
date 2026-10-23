# PQXDH as libsignal v0.99.1 runs it (signal-cli v0.14.7)

Spec role: prose only. Nothing in this file is code. Read it together with
Signal's published PQXDH specification (https://signal.org/docs/specifications/pqxdh/),
which the implementation role reads directly. This file records only the
concrete choices libsignal makes and the places where it departs from, or
goes beyond, the published document.

Related specs: `spec/crypto/kem.md` (Kyber1024 round 3, type byte `0x08`,
key and ciphertext encodings, prekey signatures), `spec/crypto/xeddsa.md`
(XEd25519, the 33-byte `0x05` public-key encoding, X25519 all-zero check,
base-key canonicity check), `spec/protocol/double-ratchet.md` (everything
after the handshake), `spec/protocol/spqr/` (the post-quantum ratchet; owned
by another agent).

## Sources

| Ref | What | URL @ version |
|-----|------|---------------|
| P1 | libsignal | https://github.com/signalapp/libsignal @ tag v0.99.1 = commit 97801d22dcf9f5bf714f7b8fa3212cdc973ae1c8: `rust/protocol/src/{pqxdh,handshake,ratchet,session,session_management,triple_ratchet,protocol,consts,identity_key}.rs`, `rust/protocol/src/ratchet/keys.rs`, `rust/protocol/src/state/{bundle,session}.rs`, `rust/protocol/src/storage/{traits,inmem}.rs`, `rust/protocol/src/proto/{wire,storage}.proto`, `rust/core/src/{lib,address}.rs`, `rust/protocol/tests/{ratchet,session}.rs` |
| P2 | signal-cli | https://github.com/AsamK/signal-cli @ tag v0.14.7 = commit b01b6b370dc063599a1a2b9fde0f5ff4e2d78fe8: `lib/src/main/java/org/asamk/signal/manager/storage/prekeys/{KyberPreKeyStore,PreKeyStore}.java`, `lib/src/main/java/org/asamk/signal/manager/storage/sessions/SessionStore.java` |
| P3 | Reference distribution | `/work/third_party/signal-cli-0.14.7/lib/libsignal-client-0.99.1.jar` and `signal-service-java-2.15.3_unofficial_151.jar` (`javap` signatures of `SessionCipher`, `SessionBuilder`, `SignalServiceCipher`) |
| P4 | Signal PQXDH specification | https://signal.org/docs/specifications/pqxdh/ (by reference; revision not pinned by this agent) |
| P5 | prost-derive (protobuf encoder used by libsignal; Cargo.lock pins 0.14.4) | https://github.com/tokio-rs/prost @ tag v0.14.1, `prost-derive/src/lib.rs` (fields are encoded sorted by tag number) |

## 1. Parameters (the choices PQXDH leaves open)

| PQXDH parameter | libsignal v0.99.1 value |
|-----------------|-------------------------|
| curve | X25519 (Curve25519), XEd25519 signatures |
| hash | SHA-256 |
| info | ASCII `WhisperText_X25519_SHA-256_CRYSTALS-KYBER-1024` (47 bytes, no terminator). **Confirmed** in P1 `pqxdh.rs`. |
| pqkem | CRYSTALS-Kyber1024, round 3 (`kem.md`). Not ML-KEM. |
| EncodeEC | 33 bytes: `0x05` then the 32-byte u-coordinate |
| EncodeKEM | 1569 bytes: `0x08` then the 1568-byte Kyber public key |
| AEAD / AD | Not an AEAD. The PQXDH "AD" role is played by the SignalMessage MAC, which covers both 33-byte identity keys (see `double-ratchet.md` §5). |
| KDF output | **96 bytes**, not 32 (see §3) |

Session version produced by this handshake: **4** (written into the session
record and into the high nibble of every message's version byte).
Version 3 (X3DH without Kyber) is no longer created and is rejected for new
incoming sessions (§6).

## 2. Shared-secret input (byte order)

The KDF input key material is the concatenation, with no length prefixes,
of the following pieces in this order. Each DH output is the raw 32-byte
X25519 result; any all-zero DH output aborts the handshake with an
"invalid key agreement" error (`xeddsa.md`).

| # | Piece | Initiator (Alice) computes | Responder (Bob) computes | Length |
|---|-------|---------------------------|--------------------------|-------:|
| 0 | discontinuity prefix | 32 bytes of `0xFF` | same | 32 |
| 1 | DH1 | X25519(Alice identity private, Bob signed-prekey public) | X25519(Bob signed-prekey private, Alice identity public) | 32 |
| 2 | DH2 | X25519(Alice base/ephemeral private, Bob identity public) | X25519(Bob identity private, Alice base public) | 32 |
| 3 | DH3 | X25519(Alice base private, Bob signed-prekey public) | X25519(Bob signed-prekey private, Alice base public) | 32 |
| 4 | DH4 (only if a one-time EC prekey is used) | X25519(Alice base private, Bob one-time prekey public) | X25519(Bob one-time prekey private, Alice base public) | 32 or absent |
| 5 | Kyber shared secret | from encapsulating to Bob's Kyber prekey | from decapsulating the received ciphertext with Bob's Kyber prekey secret | 32 |

So the input is 192 bytes with a one-time prekey and 160 bytes without one.
This order matches P4 (`F || DH1 || DH2 || DH3 [|| DH4] || SS`). "Identity
private key" means the account identity key pair of whichever identity
(ACI or PNI) the session is addressed to.

## 3. KDF and output split

- HKDF-SHA-256 (RFC 5869).
- **Salt: absent** (libsignal passes no salt, so RFC 5869's default of 32
  zero bytes applies).
- IKM: the bytes from §2.
- Info: `WhisperText_X25519_SHA-256_CRYSTALS-KYBER-1024`.
- Output length: **96 bytes**, split in order:

| Bytes | Use |
|-------|-----|
| 0..31 | initial **root key** RK |
| 32..63 | initial **chain key** CK (chain index 0) |
| 64..95 | **SPQR initial authentication key** ("PQR key"), handed to the SPQR ratchet as its `auth_key` seed. Interface only; see `spec/protocol/spqr/`. |

Deviation from P4: P4 describes a single 32-byte SK fed to the Double
Ratchet as its initial root key. libsignal instead derives RK, CK and the
SPQR seed in one 96-byte expansion. CK is used directly as a chain key
without a Diffie-Hellman ratchet step (next section).

## 4. Session initialization after the KDF

### Initiator (Alice), when processing a bundle

1. Alice's state starts with a **receiving chain** keyed by Bob's **signed
   prekey public key** (as the "remote ratchet key"), with chain key CK,
   index 0. In other words, Bob's signed prekey is Bob's first ratchet key.
2. Alice immediately generates a fresh X25519 ratchet key pair and performs
   one root-KDF step (`double-ratchet.md` §2) with the current root key RK
   and X25519(new ratchet private, Bob signed-prekey public). That yields
   (RK', CKs). Alice stores RK' as her root key and creates her **sending
   chain** = (new ratchet key pair, CKs, index 0).
3. The session state also records:
   - session version 4;
   - local identity public key (33 bytes) and remote identity public key;
   - `alice_base_key` = Alice's base public key (33 bytes);
   - previous counter 0;
   - the SPQR state, initialized with the PQR key, version V1, minimum
     version V1, direction "A to B" (Alice is A), and chain parameters
     (max jump 25 000, or unlimited for a self-session; max out-of-order
     keys 2 000);
   - **pending prekey** info: one-time prekey ID (optional), signed prekey
     ID, Alice's base public key, and the creation time in whole seconds
     since the Unix epoch;
   - **pending Kyber prekey** info: Kyber prekey ID and the 1569-byte Kyber
     ciphertext;
   - local and remote registration IDs (remote = the bundle's).
4. Alice saves Bob's identity key in her identity store and installs the new
   state as the current session, archiving any previous current state
   (§7).

Until Alice receives any message from Bob in this session, every message
she encrypts is wrapped in a PreKeySignalMessage built from the pending
info (§5). The pending info is cleared the first time any message from Bob
decrypts successfully in this session state.

If the pending info is older than **30 days**
(`MAX_UNACKNOWLEDGED_SESSION_AGE`, 2 592 000 s) when Alice tries to
encrypt, encryption fails with "session not found" (the caller is expected
to fetch a fresh bundle). The same age makes the session "stale" for the
sender-chain usability check.

### Responder (Bob), when processing a PreKeySignalMessage

1. Bob's state starts with root key RK (from §3, unchanged) and a
   **sending chain** = (his signed prekey key pair as ratchet key pair, CK,
   index 0). He has **no receiving chain** yet.
2. `alice_base_key` = the received base key; session version 4; identity
   keys; SPQR state initialized with the PQR key, version V1, minimum V1,
   direction "B to A", same chain parameters.
3. Local registration ID = own; remote registration ID = the value carried
   in the PreKeySignalMessage.
4. Decrypting the embedded SignalMessage then performs the normal receive
   path: Alice's new ratchet key is unknown, so Bob does a DH ratchet step
   using RK and X25519(signed-prekey private, Alice ratchet public). That
   gives the same (RK', CKs) Alice derived, as a receiving chain, and then a
   fresh sending chain for Bob.

Consequences worth noting:
- A receiver that tried to start Bob's state with a DH ratchet step (as the
  generic Double Ratchet initialization in signal.org's document does)
  would not interoperate. The signed prekey doubles as Bob's initial
  ratchet key, and CK from the handshake is a ready-made chain on it.
- Bob's initial sending chain (on the signed prekey) is normally never used,
  because receiving Alice's first message replaces it.

"Self-session": a session is treated as a self-session when the local and
remote identity public keys are equal **and** both protocol-address names
parse as the same service ID (ACI UUID string, or `PNI:` + UUID). This only
lifts the forward-jump limit (25 000 → unlimited) for the Double Ratchet and
SPQR.

## 5. Wire format: PreKeySignalMessage (ciphertext message type 3)

```
byte 0        : version byte = (session_version << 4) | 4
bytes 1..end  : protobuf "PreKeySignalMessage" (proto2)
```

- High nibble: the session version (4 for every session this build
  creates). Low nibble: always the constant 4 (the library's "current"
  version), even for a version-3 session. Parsers look only at the high
  nibble.
- There is **no MAC or signature on the outer message**. Integrity comes
  from the embedded SignalMessage's MAC, which covers both identity keys.

Protobuf fields (package `signal.proto.wire`):

| Field # | Name | Type | Content as sent by libsignal |
|--------:|------|------|------------------------------|
| 5 | registration_id | uint32 | sender's local registration ID |
| 1 | pre_key_id | uint32, optional | Bob's one-time EC prekey ID; omitted when no one-time prekey was in the bundle. The value 0 is a valid ID. |
| 6 | signed_pre_key_id | uint32 | Bob's signed EC prekey ID |
| 7 | kyber_pre_key_id | uint32 | Bob's Kyber prekey ID (one-time or last-resort) |
| 8 | kyber_ciphertext | bytes | 1569 bytes: `0x08` then the 1568-byte Kyber ciphertext |
| 2 | base_key | bytes | Alice's base (ephemeral) public key, 33 bytes |
| 3 | identity_key | bytes | **Alice's** identity public key, 33 bytes (the identity the session belongs to: ACI or PNI) |
| 4 | message | bytes | the complete serialized SignalMessage (its own version byte, protobuf and 8-byte MAC), see `double-ratchet.md` §4 |

The encoder writes the fields in ascending field-number order (1, 2, 3, 4,
5, 6, 7, 8) and writes every present field, including zero values. The
receiver must keep the received bytes and not re-encode them.

Parsing rules (receiver):
- Empty input: "ciphertext message too short".
- High nibble < 3: "legacy ciphertext version"; > 4: "unrecognized
  ciphertext version".
- Missing base_key, identity_key, message or signed_pre_key_id:
  "invalid protobuf encoding". A missing registration_id reads as 0.
- base_key and identity_key go through the 33-byte EC public-key decoder
  (`xeddsa.md`; trailing bytes are tolerated there).
- Kyber fields: both present → accepted. Both absent → accepted only if the
  version is ≤ 3; for version 4 the message is rejected as invalid ("Kyber
  pre key must be present"). Exactly one present → rejected as invalid.
- The embedded message is parsed as a SignalMessage immediately.

The same SignalMessage that is embedded here carries the optional
`addresses` field (field 6), which binds sender and recipient service IDs
and device IDs. libsignal fills it **only** for the SignalMessage inside a
PreKeySignalMessage (`double-ratchet.md` §4).

## 6. Prekey bundle: contents and validation order (initiator)

A bundle (built from the server's prekey response by signal-service-java)
must contain:

| Item | Required | Notes |
|------|----------|-------|
| registration ID | yes | |
| device ID | yes | 1..127 |
| identity public key | yes | 33 bytes |
| signed EC prekey: ID, public key, signature | yes | signature = XEd25519 by the identity key over the 33-byte serialized prekey |
| Kyber prekey: ID, public key, signature | **yes** | public key 1569 bytes with `0x08`; signature by the identity key over those 1569 bytes. Missing → "kyber_pre_key_* is required". |
| one-time EC prekey: ID and public key | no | unsigned; used only if both ID and key are present |

Processing order when Alice starts a session from a bundle:
1. Identity trust check for the remote address (direction "sending"). An
   untrusted identity aborts with "untrusted identity".
2. Verify the signed EC prekey signature. Failure → "signature validation
   failed".
3. Verify the Kyber prekey signature. Failure → "signature validation
   failed".
4. Load the existing session record for the address, or start an empty one.
5. Generate the base key pair, run §2–§4 (including Kyber encapsulation).
6. Save the remote identity key, install the new state as current (the old
   current state is archived), store the record.

No check is made that the bundle's keys differ from earlier ones, and the
one-time prekey is not validated beyond decoding.

## 7. Responder handling of an incoming PreKeySignalMessage

Order of operations in libsignal v0.99.1:

1. Load the session record for the sender's protocol address (or start an
   empty record).
2. **Identity trust check** on the identity key carried in the message
   (direction "receiving"). Untrusted → "untrusted identity".
3. **Existing-session match.** Look for a state, first the current one and
   then each archived one, whose session version equals the message
   version and whose stored `alice_base_key` equals the message's base key
   (33-byte serialized form, constant-time comparison). If an archived
   state matches, it is promoted to current. If any state matches, no
   prekeys are looked up or consumed, and processing continues at step 10
   with that state.
4. If the message version is 3: reject as **invalid message**,
   "X3DH no longer supported". This check comes *after* step 3, so a
   version-3 PreKeySignalMessage for an existing version-3 session still
   decrypts.
5. Load the signed EC prekey by ID. Missing → "invalid signed prekey ID"
   (Java: `InvalidKeyIdException`).
6. Require a Kyber prekey ID (missing → invalid message,
   "missing pq pre-key ID") and load it (missing → "invalid Kyber prekey
   ID"). Require the Kyber ciphertext (missing → invalid message,
   "missing pq ciphertext").
7. If the message names a one-time EC prekey, load it (missing → "invalid
   prekey ID"). Otherwise proceed without DH4 (libsignal logs a warning).
8. **Base-key canonicity check** on the initiator's base key, before any
   DH or KEM work (`xeddsa.md`). Failure → invalid message,
   "incoming base key is invalid".
9. Compute §2–§4 as responder (Kyber decapsulation included; a wrong-type
   or wrong-length ciphertext is an error, see `kem.md`). The new state
   becomes current; the previous current state is archived.
10. Decrypt the embedded SignalMessage with the **current state only**
    (no fallback to archived states for the inner message of a
    PreKeySignalMessage). Any failure other than "duplicated message" is
    reported as invalid message, "decryption failed".
11. Only after a successful decrypt:
    - save the sender's identity key;
    - if prekeys were used in step 9 (not for a step-3 match): call
      "mark Kyber prekey used" with (Kyber prekey ID, signed prekey ID,
      base key), then remove the one-time EC prekey;
    - store the session record.

If anything fails (steps 2–10), nothing is written: the session record,
identity store and prekey stores remain as they were. An upstream test
asserts this (`prekey_message_failed_decryption_does_not_update_stores`).

### Kyber prekey consumption in signal-cli 0.14.7 (P2)

- "Mark Kyber prekey used" in signal-cli's store deletes the Kyber prekey
  row **only if it is not a last-resort key**. A last-resort Kyber prekey
  is kept.
- libsignal's store contract says a last-resort Kyber key should
  additionally remember each (Kyber prekey ID, signed prekey ID, base key)
  tuple and reject a repeat as invalid message "reused base key".
  libsignal's in-memory test store does this. **signal-cli 0.14.7 does not
  implement it** (a TODO in its store); a replayed PreKeySignalMessage that
  uses a last-resort key and does not match an existing session (for
  example after the matching state aged out of the archive) is therefore
  processed again rather than rejected.
- One-time EC prekeys are deleted after successful use. Signed prekeys are
  not deleted on use.

### Duplicates and replays

- Same PreKeySignalMessage delivered twice: step 3 finds the session
  created the first time (same base key). Then the inner SignalMessage's
  counter has already been consumed, so the result is
  **"duplicated message"** (Java: `DuplicateMessageException`). No state is
  changed.
- A later PreKeySignalMessage from the same session (Alice keeps sending
  PreKeySignalMessages until she hears back) also matches at step 3 and
  decrypts normally with the existing state.
- A PreKeySignalMessage with the same base key but a different sender
  address finds no matching session for *that* address. With libsignal's
  reference store it would fail as "reused base key" when the Kyber key is
  marked used. With signal-cli's store, if the one-time prekey was already
  consumed, it fails at step 7; if only a last-resort key and no one-time
  key were used, it can succeed (see above).

### Archiving and promotion (session record)

- A session record holds one current state and up to **40 archived
  states**, newest first (`ARCHIVED_STATES_MAX_LENGTH`).
- Installing a new current state archives the old one: the old state's
  pending-prekey and pending-Kyber fields are cleared, it is serialized and
  inserted at the front of the archive list; if the list already holds 40,
  the oldest (last) entry is dropped first.
- Promoting an archived state (step 3 above, or a successful Whisper
  decrypt with an archived state, see `double-ratchet.md` §8) removes it
  from the list and installs it as current, which archives the previous
  current state at the front.
- Explicit archiving (signal-cli does this, for example, on identity
  change or session reset) moves the current state to the archive the same
  way and leaves the record with no current state.

### Session usability as signal-cli checks it

libsignal exposes a check "has a usable sender chain, with a required PQ
ratio". It is true when a current state exists, has a sending chain, is not
stale (pending prekey info at most 30 days old), and then either the state
is PQXDH-established (version > 3) with non-empty SPQR state, or the ratio
test passes. The ratio test reads the first 4 bytes of `alice_base_key` as
a big-endian unsigned integer x and accepts the non-PQ session when
floor((2^32 − 1) × ratio) ≤ x; ratio 0 always accepts and ratio ≥ 1 never
accepts. **signal-cli 0.14.7 always passes ratio 0.0**, so it treats any
non-stale state with a sending chain as active. Vectors:
`testdata/vectors/ratchet_records.json` (kind `session_usability`).

## 8. Error kinds (summary)

| Situation | libsignal error | Java exception (via P3 signatures) |
|-----------|-----------------|------------------------------------|
| bad version nibble | legacy / unrecognized ciphertext version | `LegacyMessageException` / `InvalidVersionException` |
| malformed protobuf, missing required field | invalid protobuf encoding | `InvalidMessageException` |
| v3 prekey message without existing session | invalid message "X3DH no longer supported" | `InvalidMessageException` |
| unknown signed / one-time / Kyber prekey ID | invalid signed prekey ID / invalid prekey ID / invalid Kyber prekey ID | `InvalidKeyIdException` |
| non-canonical base key | invalid message "incoming base key is invalid" | `InvalidMessageException` |
| untrusted identity | untrusted identity | `UntrustedIdentityException` |
| inner message already consumed | duplicated message | `DuplicateMessageException` |
| inner message fails MAC, SPQR or decryption | invalid message "decryption failed" | `InvalidMessageException` |

## 9. Interface points with SPQR (no internals)

- The SPQR initial authentication key is bytes 64..95 of the PQXDH KDF
  output (§3).
- Initiator SPQR direction is "A to B", responder "B to A"; both start at
  SPQR version V1 with minimum version V1 (libsignal v0.99.1 requires SPQR
  for new sessions).
- SPQR chain parameters: max jump = 25 000 (unlimited for a self-session),
  max out-of-order keys = 2 000.
- The serialized SPQR state lives in session-record field 15
  (`pq_ratchet_state`); per-message SPQR data travels in SignalMessage
  field 5 (`pq_ratchet`). How SPQR output enters message keys is in
  `double-ratchet.md` §3.

## 10. Test vectors

libsignal v0.99.1 has **no fixed PQXDH vectors**. Every PQXDH test in the
Rust, Java, Node and Swift suites (`tests/ratchet.rs`, `tests/session.rs`,
`SessionBuilderTest`, `SessionCipherTest`, `ProtocolTest.ts`,
`SessionTests.swift`) generates identity keys, prekeys, Kyber keys and base
keys at runtime and only checks that both sides agree. The one fixed item
relevant to this handshake is a Kyber1024 key pair that libsignal must
deserialize and re-serialize byte-identically; it is in
`testdata/vectors/pqxdh_kyber_prekey.json`. Base-key range rejection
vectors are already in `xeddsa.json` (kind `montgomery_u_range_check`).
