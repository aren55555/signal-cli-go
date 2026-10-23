# Sender keys (group messaging) in libsignal v0.99.1 (signal-cli v0.14.7)

Spec role: prose only. Nothing in this file is code. Signal publishes no
specification for sender keys, so this file describes the whole
construction as libsignal implements it.

Related: `spec/crypto/xeddsa.md` (the XEd25519 signature on every
SenderKeyMessage), `spec/protocol/double-ratchet.md` (AES-256-CBC with
PKCS#7, the 1:1 channel that carries distribution messages),
`spec/protocol/sealed-sender.md` (sender-key messages are only sent inside
sealed sender, as content type 7, alone or in multi-recipient v2 form).

## Sources

| Ref | What | URL @ version |
|-----|------|---------------|
| K1 | libsignal | https://github.com/signalapp/libsignal @ tag v0.99.1 = commit 97801d22dcf9f5bf714f7b8fa3212cdc973ae1c8: `rust/protocol/src/{group_cipher,sender_keys,protocol,consts,crypto}.rs`, `rust/protocol/src/proto/{wire,storage}.proto`, `rust/crypto/src/aes_cbc.rs`, `rust/protocol/tests/groups.rs`, Java `GroupCipherTest.java`, Swift `PublicAPITests.swift` (`testGroupCipher*`) |
| K2 | Reference distribution | `/work/third_party/signal-cli-0.14.7/lib/signal-service-java-2.15.3_unofficial_151.jar` (`javap` of `SignalServiceCipher`: a `GroupCipher` is built from the sender-key store and the local protocol address) |
| K3 | prost-derive (encoder) | https://github.com/tokio-rs/prost @ tag v0.14.1, `prost-derive/src/lib.rs` (fields encoded in tag order) |

## 1. Concepts

- A **sender key** belongs to one sending device (protocol address =
  service-ID string + device ID) and one **distribution ID** (a UUID that
  names the audience, e.g. a group). Receivers store it under
  (sender address, distribution ID).
- A sender-key **state** consists of: a **chain ID** (uint32), a **chain
  key** (32 bytes) with an **iteration** counter, a **signing key pair**
  (X25519/XEd25519; receivers hold only the public half), the message
  version (3), and a list of skipped message keys.
- A **record** holds up to **5** states (`MAX_SENDER_KEY_STATES`), newest
  first. The sender always uses the first (newest) state. A receiver picks
  the state whose chain ID matches the message.

## 2. Key derivation

Chain step, with CK the current 32-byte chain key at iteration n:
- **message-key seed** = HMAC-SHA-256(key = CK, data = single byte `0x01`);
- **next chain key** = HMAC-SHA-256(key = CK, data = single byte `0x02`),
  iteration n + 1. Advancing past iteration 2^32 − 1 is an error
  ("sender chain is too long").

Message key from the seed:
- HKDF-SHA-256, **no salt** (RFC 5869 default), **IKM = seed**,
  **info = ASCII `WhisperGroup`** (12 bytes), output **48 bytes**.
- Split: bytes **0..15 = IV**, bytes **16..47 = AES-256 key**. Note the
  order is the reverse of the 1:1 message keys, where the IV comes last.
- Cipher: AES-256-CBC with PKCS#7 padding, same rules as 1:1
  (`double-ratchet.md` §3). There is **no MAC**; integrity comes from the
  signature (§4).

## 3. SenderKeyDistributionMessage (SKDM)

Carried inside the 1:1-encrypted `Content` message, field 7
(`sender_key_distribution_message`, bytes), to each member device.

```
byte 0       : version byte = (3 << 4) | 3 = 0x33
bytes 1..end : protobuf "SenderKeyDistributionMessage" (proto2)
```

| Field # | Name | Type | Content |
|--------:|------|------|---------|
| 1 | distribution_uuid | bytes | 16 bytes, the UUID in standard big-endian order |
| 2 | chain_id | uint32 | sender's chain ID |
| 3 | iteration | uint32 | the sender's **current** iteration (the next one it will use) |
| 4 | chain_key | bytes | the chain key **at that iteration**, 32 bytes |
| 5 | signing_key | bytes | signing public key, 33 bytes (`0x05` + u) |

No signature or MAC; authenticity comes from the 1:1 session.

Parsing:
- total length < 65 → "ciphertext message too short";
- high nibble ≠ 3 → legacy (< 3) or unrecognized (> 3) version;
- any of the five fields missing, or a distribution_uuid that is not 16
  bytes → "invalid protobuf encoding";
- chain_key must be exactly 32 bytes and signing_key exactly 33 bytes,
  else "invalid protobuf encoding" (stricter than the general key decoder,
  which tolerates trailing bytes).

Creating an SKDM (sender side): if no record exists for (own address,
distribution ID), create one state with
- chain ID = a random 32-bit value shifted right by one (so 0..2^31 − 1),
- iteration 0,
- 32 random bytes as chain key,
- a fresh signing key pair,
and store it. Then emit an SKDM from the newest state's current iteration,
chain key and signing public key. A recipient therefore cannot derive keys
for iterations before the one in the SKDM.

Processing an SKDM (receiver side), into the record for
(sender address, distribution ID):
1. If a state with the same chain ID **and** the same signing public key
   exists, it is kept unchanged (its chain position and skipped keys are
   preserved; the SKDM's iteration and chain key are ignored) and moved to
   the front.
2. Otherwise any state with that chain ID but a different signing key is
   discarded, and a new state is built from the SKDM.
3. Before inserting at the front, states are dropped from the back while
   the record holds 5 or more, so at most 5 remain.

## 4. SenderKeyMessage (SKM, ciphertext message type 7)

```
byte 0             : version byte = (3 << 4) | 3 = 0x33
bytes 1 .. n-65    : protobuf "SenderKeyMessage" (proto2)
bytes n-64 .. n-1  : XEd25519 signature, 64 bytes
```

| Field # | Name | Type | Content |
|--------:|------|------|---------|
| 1 | distribution_uuid | bytes | 16 bytes |
| 2 | chain_id | uint32 | |
| 3 | iteration | uint32 | iteration whose message key encrypted this message |
| 4 | ciphertext | bytes | AES-256-CBC output |

- The signature is made with the state's signing **private** key over the
  version byte plus the protobuf, i.e. everything before the signature
  (`xeddsa.md`, row "Sender-key (group) message").
- Fields are encoded in ascending order with every field present.

Parsing: total length < 65 → too short; high nibble ≠ 3 → legacy /
unrecognized version; missing field or bad UUID → invalid protobuf.

## 5. Encrypting (sender)

1. Load the record for (own address, distribution ID); none → "no sender
   key state".
2. Use the newest state. Derive the message key for the current iteration,
   encrypt, build and sign the SKM with that iteration.
3. Advance the chain by one and store the record.

## 6. Decrypting (receiver)

1. Parse the SKM.
2. Load the record for (sender address, distribution ID from the message);
   none → **"no sender key state"** (Java: `NoSessionException`).
3. Find the state with the message's chain ID; none → "no sender key
   state".
4. The message version must equal the state's version (a stored version 0
   counts as 3); else "unrecognized message version".
5. Verify the signature with the state's signing public key; failure →
   **"signature validation failed"** (Java: presumably `InvalidMessageException`, the only fitting exception `GroupCipher.decrypt` declares; mapping not traced).
6. Select the message key for the iteration j, with i = the state's
   current iteration:
   - j < i: use and remove a stored skipped key for j; if none →
     **"duplicated message"** (Java: `DuplicateMessageException`).
   - j − i > **25 000** (`MAX_FORWARD_JUMPS`) → invalid message,
     "message from too far into the future". Unlike 1:1 sessions there is
     no self-session exception.
   - otherwise store the seeds for i .. j − 1 as skipped keys, use the key
     for j, and set the state's chain to iteration j + 1.
7. Decrypt with AES-256-CBC; bad padding → invalid message
   "decryption failed".
8. Store the record **only after success**. A failure at any step leaves
   the stored record unchanged.

Skipped-key list per state: entries hold (iteration, 32-byte seed); new
entries are **appended at the end**, and when there are more than **2 000**
(`MAX_MESSAGE_KEYS`) the **first** (oldest) is dropped. (This is the
opposite list order from the 1:1 ratchet; it only matters for byte-exact
record reproduction.)

## 7. Storage format (for reading signal-cli data)

`SenderKeyRecordStructure` (proto3, package `signal.proto.storage`):
1 sender_key_states (repeated SenderKeyStateStructure, newest first).

`SenderKeyStateStructure`:

| Field # | Name | Type |
|--------:|------|------|
| 5 | message_version | uint32 (0 means 3) |
| 1 | chain_id | uint32 |
| 2 | sender_chain_key | message: 1 iteration (uint32), 2 seed (bytes, the 32-byte chain key) |
| 3 | sender_signing_key | message: 1 public (bytes, 33), 2 private (bytes, 32; empty on receivers) |
| 4 | sender_message_keys | repeated message: 1 iteration (uint32), 2 seed (bytes, 32) |

## 8. Distribution ID in signal-cli

signal-cli passes a `DistributionId` (UUID) to signal-service-java's
group encryption, and stores sender keys in its account database keyed by
address, device and distribution ID (see `capture.md`). The UUID's 16
bytes travel in field 1 of both messages in standard big-endian order.

## 9. Test vectors

libsignal v0.99.1 contains **no fixed sender-key vectors**. The Rust
(`tests/groups.rs`), Java (`GroupCipherTest`), Node and Swift group tests
create sender keys and signing keys at runtime and only round-trip them;
the only constant is the distribution ID
`d1d1d1d1-7000-11eb-b32a-33b8a8a487a6`, which has no expected output tied
to it. No `sender_key_*.json` file was therefore written. The chain-step
HMAC constants are the same as in the 1:1 ratchet, and AES-256-CBC can be
checked with `testdata/vectors/ratchet_kdf.json`.
