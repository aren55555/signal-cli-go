# The SPQR protocol (spqr 1.5.3, protocol version V1)

Spec role: prose only. Source: P1 in `index.md` (SparsePostQuantumRatchet
@ v1.5.3, commit fd320484dcec89004021e6fdc7481825f5f261fa): `src/lib.rs`,
`src/chain.rs`, `src/authenticator.rs`, `src/kdf.rs`,
`src/v1/unchunked/*`, `src/v1/chunked/*`. The published ML-KEM Braid
document (P4) describes the same state machine at a higher level and uses
the same state names. Where this file is more specific, this file follows
the code.

SPQR has three layers, described in order below:

1. A **top-level** layer (section 2) that handles version negotiation and
   ties the other two together. Its operations are *initialize*, *send*
   and *receive*.
2. A **symmetric chain** (section 3) that turns epoch secrets into a
   32-byte key per message, with out-of-order support.
3. The **SCKA** (sections 5–7), an 11-state machine that runs one ML-KEM-768
   exchange per epoch, cut into 32-byte chunks spread over many messages.
   An **authenticator** (section 4) MACs the ML-KEM header and ciphertext.

All HKDF calls are HKDF-SHA256 (RFC 5869). All HMAC calls are
HMAC-SHA256 with full 32-byte output. A "zero salt" is 32 bytes of 0x00.

## 1. KDF and MAC catalogue

| Name | Function | Salt | IKM / key | Info (exact ASCII, then any suffix) | Output |
|------|----------|------|-----------|-------------------------------------|--------|
| Chain start | HKDF | zero salt | auth key (32) | `Signal PQ Ratchet V1 Chain  Start` (33 bytes; **two** spaces between `Chain` and `Start`) | 96 |
| Chain add epoch | HKDF | current `next_root` (32) | epoch secret (32) | `Signal PQ Ratchet V1 Chain Add Epoch` (36 bytes) | 96 |
| Chain next | HKDF | zero salt | current `next` value of that direction (32) | BE32(new counter) followed by `Signal PQ Ratchet V1 Chain Next` (4 + 31 bytes) | 64 |
| Authenticator update | HKDF | zero salt | current root_key (32) ‖ k (32) | `Signal_PQCKA_V1_MLKEM768:Authenticator Update` (45 bytes) followed by BE64(epoch) | 64 |
| Header MAC | HMAC | — | key = mac_key (32) | data = `Signal_PQCKA_V1_MLKEM768:ekheader` (33 bytes) ‖ BE64(epoch) ‖ hdr (64) | 32 |
| Ciphertext MAC | HMAC | — | key = mac_key (32) | data = `Signal_PQCKA_V1_MLKEM768:ciphertext` (35 bytes) ‖ BE64(epoch) ‖ ct1 (960) ‖ ct2 (128) | 32 |
| SCKA key (epoch secret) | HKDF | zero salt | ML-KEM-768 shared secret K (32) | `Signal_PQCKA_V1_MLKEM768:SCKA Key` (33 bytes) followed by BE64(epoch) | 32 |

Hex of the label bytes, to rule out typos:

| Label | Hex |
|-------|-----|
| Chain start | 5369676e616c205051205261746368657420563120436861696e20205374617274 |
| Chain add epoch | 5369676e616c205051205261746368657420563120436861696e204164642045706f6368 |
| Chain next | 5369676e616c205051205261746368657420563120436861696e204e657874 |
| Authenticator update | 5369676e616c5f5051434b415f56315f4d4c4b454d3736383a41757468656e74696361746f7220557064617465 |
| Header MAC prefix | 5369676e616c5f5051434b415f56315f4d4c4b454d3736383a656b686561646572 |
| Ciphertext MAC prefix | 5369676e616c5f5051434b415f56315f4d4c4b454d3736383a63697068657274657874 |
| SCKA key | 5369676e616c5f5051434b415f56315f4d4c4b454d3736383a53434b41204b6579 |

MAC comparisons are constant-time.

## 2. Top level: initialize, send, receive, version negotiation

### 2.1 Top-level state

The top-level state has three optional parts:

- **Negotiation record**: auth key, direction, minimum version, chain
  parameters. Present from initialization until the first successful
  receive.
- **Chain** (section 3): absent at first. It is created on demand from
  the negotiation record.
- **SCKA state** (sections 5–7) for V1, or nothing for V0.

The "state version" is V1 if an SCKA state is present, else V0. An empty
serialized state means V0 with no negotiation record and no chain.

### 2.2 Initialize

Inputs: auth key, direction, version, minimum version, chain parameters.

- If version is V0, the state is empty.
- Otherwise, store a negotiation record and no chain. The SCKA starts in
  **KeysUnsampled, epoch 1** for A2B and in **NoHeaderReceived, epoch 1**
  for B2A. In both cases the authenticator is initialized from the auth key
  for epoch 1 (section 4).

libsignal always initializes with version V1 and minimum version V1
(`session.md`).

### 2.3 Send

1. No SCKA state (V0): return an empty message, no key, and an empty
   state.
2. Run the SCKA send step for the current SCKA state (section 7). It gives
   an SCKA message with epoch E and a payload, possibly an **epoch secret**
   (epoch number plus 32 bytes), and the next SCKA state.
3. Get the chain:
   - if a chain is stored, use it;
   - else, if a negotiation record exists and its minimum version is at
     least V1, create the chain from the auth key, direction and chain
     parameters in that record (section 3.2);
   - else, if a negotiation record exists with minimum version V0, use **no
     chain**;
   - else (neither chain nor negotiation record) fail with "chain not
     available".
4. With no chain: key index = 0 and no message key. With a chain: if
   step 2 produced an epoch secret, first add it to the chain (section
   3.3). Then take the next sending key for chain epoch **E − 1** (section
   3.4). That gives (index, 32-byte key).
5. Serialize the message (`wire.md`) with epoch E, the index and the
   payload.
6. The new state keeps the negotiation record unchanged (sending never
   alters it), stores the chain if there is one, and stores the new SCKA
   state. The SPQR message key is the 32-byte key, or none if there was no
   chain.

### 2.4 Receive

1. **Message version**: an empty message is V0. Otherwise the first byte
   decides: 0 is V0, 1 is V1, and anything else is an unknown future
   version. For an unknown version, return the state **unchanged**, no key
   and **no error**.
2. Compare the message version with the state version:
   - message version ≥ state version: continue with the state as it is;
   - message version < state version:
     - no negotiation record: fail with **VersionMismatch**;
     - message version below the record's minimum version: fail with
       **MinimumVersion**;
     - otherwise **downgrade**: replace the SCKA state with the initial
       state for the message's version (for V0: none), keep the chain or
       create it from the record, and delete the negotiation record so no
       further negotiation can happen.
3. If the (possibly downgraded) state has no SCKA state (V0): return an
   empty state and no key. Any V1 data in the message is ignored.
4. Parse the message as a V1 message (`wire.md`). Parse errors fail with
   **MsgDecode**.
5. Run the SCKA receive step (section 7) with the message's epoch and
   payload. It may fail; it may produce an epoch secret.
6. Get the chain: the stored one, else create it from the negotiation
   record, else fail with "chain not available". Note that, unlike in
   send, the record's minimum version does not matter here.
7. If step 5 produced an epoch secret, add it to the chain.
8. Let e = message epoch − 1 and i = the message's key index. If e = 0 and
   i = 0, there is no key (that only happens when the sender had no chain,
   which needs a minimum version of V0). Otherwise take the receiving key
   for (e, i) (section 3.5); that can fail.
9. The new state drops the negotiation record (**receiving ends
   negotiation**), stores the chain and the new SCKA state. Return the
   32-byte key (or none).

A failed receive must leave the stored state untouched. libsignal achieves
that by discarding the result unless the whole message decrypts
(`session.md`, section 6).

### 2.5 What libsignal's settings imply

With V1 and minimum V1 on both sides:
- The chain is created on the first send or receive, and every message has
  a key index of at least 1 and a 32-byte key.
- A V0 (empty) message is always rejected: with **MinimumVersion** before
  the first successful receive, with **VersionMismatch** after it.
- Downgrade never happens.

## 3. The symmetric chain

### 3.1 State

- direction (A2B or B2A), current epoch (u64, starts at 0), send epoch (u64,
  starts at 0), `next_root` (32 bytes), parameters max_jump and
  max_ooo_keys;
- a list of **links**, one per retained epoch, oldest first. Each link has
  a send half and a receive half. Each half has a counter (u32, starts at 0),
  a 32-byte `next` value, and (receive half only in practice) a store of
  skipped keys, each an (index, 32-byte key) pair.

### 3.2 Creation (epoch 0)

Run **Chain start** (section 1) on the auth key, giving 96 bytes:
- bytes 0–31 become `next_root`;
- bytes 32–63 seed the **A2B** half; bytes 64–95 seed the **B2A** half.

The epoch-0 link's send half starts from the seed for the party's own
direction and its receive half from the seed for the other direction. So
the A2B party sends with bytes 32–63 and receives with bytes 64–95, and
the B2A party the reverse. Both counters start at 0.

### 3.3 Adding an epoch

An epoch secret for epoch n must have n = current epoch + 1 (anything else
is a fatal internal error). Run **Chain add epoch** with salt = `next_root`
and IKM = the 32-byte epoch secret, giving 96 bytes. Split them exactly as
in 3.2: new `next_root`, then the A2B and B2A seeds. Append a new link for
epoch n and set the current epoch to n.

### 3.4 Next key in a half; sending

**Next key in a half**: add 1 to the half's counter (call the new value c),
run **Chain next** with IKM = the half's current `next` and info = BE32(c)
‖ label, giving 64 bytes. Bytes 0–31 replace `next`; bytes 32–63 are the
message key for index c. So the first key in every half has index 1.

**Sending key for chain epoch E**:
1. If E < send epoch: fail with **SendKeyEpochDecreased**.
2. Find the link for E. If E > current epoch, or the link was already
   dropped: fail with **EpochOutOfRange**.
3. If E differs from the send epoch: set the send epoch to E, drop links
   from the front until at most **one** link older than E is left, and
   erase the send-half `next` value of every remaining link older than E.
   (Links are dropped only here.)
4. Take the next key in that link's send half and return (index, key).

### 3.5 Receiving key for (epoch E, index i)

Find the link for E (failing with **EpochOutOfRange** as in 3.4, step 2),
then work on its receive half, whose counter is c:

- **i > c**:
  - if i − c > max_jump: fail with **KeyJump**;
  - if i > c + max_ooo_keys: empty the skipped-key store first (every key
    in it is now too old);
  - step the half forward through indices c+1 … i−1. Store each such
    (index, key) only if index + max_ooo_keys ≥ i;
  - garbage-collect (below), then take one more step and return the key
    for index i. The counter is now i.
- **i < c**:
  - if i + max_ooo_keys < c: fail with **KeyTrimmed**;
  - if the store holds index i: return its key and remove the entry;
  - otherwise fail with **KeyAlreadyRequested**.
- **i = c**: fail with **KeyAlreadyRequested**. This includes i = 0 on a
  fresh half: index 0 never carries a key.

**Garbage collection**: when the store holds at least
trim_size = ⌊max_ooo_keys × 11 / 10⌋ + 1 entries (2201 with libsignal's
2000), remove every entry whose index is below c − max_ooo_keys.

Parameter defaults: a stored max_jump or max_ooo_keys of 0 means the
library default, which is 25000 and 2000 respectively.

## 4. The authenticator

State: `root_key` (32 bytes) and `mac_key` (32 bytes).

- **Initialization for epoch 1**: start with root_key = 32 zero bytes, then
  run Update(epoch 1, k = auth key).
- **Update(epoch e, k)**: run **Authenticator update** with IKM =
  root_key ‖ k and info = label ‖ BE64(e), giving 64 bytes. Bytes 0–31 are
  the new root_key and bytes 32–63 the new mac_key.
- **Header MAC(e, hdr)** and **Ciphertext MAC(e, ct1‖ct2)**: section 1,
  keyed with the current mac_key.

When updates happen (both parties do the same updates in the same order):
- once at initialization with the auth key (epoch 1);
- in each epoch e, once with that epoch's SCKA key S_e. The send_ct party
  updates when it encapsulates. The send_ek party updates when it has all
  of ct2, just **before** checking the ciphertext MAC.

So the header MAC of epoch e uses the mac_key from the update made at the
end of epoch e − 1 (or from initialization when e = 1). The ciphertext MAC
of epoch e uses the mac_key from the update with S_e.

## 5. SCKA payloads and roles

Per epoch, one party has the **send_ek** role and the other the
**send_ct** role. In epoch 1 the A2B party (initiator) is send_ek. The
roles swap at every epoch change, so the initiator is send_ek in odd epochs
and the responder in even epochs.

Four byte strings travel per epoch, each as a stream of 32-byte chunks
from the erasure code in `chunking.md`:

| Stream | Content | Length | Data chunks needed (n) | Sent by |
|--------|---------|-------:|-----------------------:|---------|
| Hdr | hdr (64) ‖ Header MAC(e, hdr) (32) | 96 | 3 | send_ek |
| Ek | ek payload (t̂ encoding) | 1152 | 36 | send_ek |
| Ct1 | ct1 | 960 | 30 | send_ct |
| Ct2 | ct2 (128) ‖ Ciphertext MAC(e, ct1‖ct2) (32) | 160 | 5 | send_ct |

The ML-KEM values are defined in `mlkem-incremental.md`. Each stream has
its own encoder, whose chunk counter starts at 0 for that stream.

Message payload kinds (`wire.md` gives the type bytes): None, Hdr chunk,
Ek chunk, EkCt1Ack chunk (an Ek chunk that also acknowledges that all of
ct1 arrived), Ct1Ack (no chunk; acknowledges ct1), Ct1 chunk, Ct2 chunk.

The SCKA epoch secret for epoch e is S_e = **SCKA key** applied to the
ML-KEM shared secret K with BE64(e) (section 1).

## 6. Epoch numbering on messages

- Every SCKA message carries the sender's current SCKA epoch E.
- The message key always comes from **chain epoch E − 1** (section 2.3,
  step 4; section 2.4, step 8). So messages labelled epoch 1 use the chain
  seeded straight from the auth key (chain epoch 0), and the secret S_e is
  first used by messages labelled e + 1.
- A party adds S_e to its chain at the moment its SCKA produces it: the
  send_ct party when it sends its first Ct1 chunk, the send_ek party when it
  receives the last needed Ct2 chunk.

## 7. The SCKA state machine

Notation: e is the state's epoch, M.epoch the incoming message's epoch.
"Ignore" means the state is unchanged and no error is raised. The message
key for an ignored SCKA message is still computed by the top level.

**Common receive rule for every state except Ct2Sampled:** if M.epoch > e,
fail with **EpochOutOfRange**. If M.epoch < e, ignore. Only M.epoch = e is
processed as listed. Payload kinds not listed for a state are ignored.

Decoders (from `chunking.md`) are fed one chunk at a time. "Complete"
means the decoder can now return the whole stream.

### 7.1 send_ek role

| State (stored data) | On send | On receive with M.epoch = e |
|---------------------|---------|-----------------------------|
| **KeysUnsampled** (e, authenticator) | Draw 64 random bytes and generate an ML-KEM-768 key pair (`mlkem-incremental.md`). Build hdr, compute Header MAC(e, hdr), and start the Hdr encoder over hdr ‖ MAC. Emit **Hdr chunk** #0. Go to KeysSampled. | Ignore everything. |
| **KeysSampled** (e, auth, ek payload, dk, Hdr encoder) | Emit the next **Hdr chunk**. | **Ct1 chunk**: start a Ct1 decoder (960 bytes) and add the chunk. Start the Ek encoder over the 1152-byte ek payload; ek no longer needs to be stored separately. Go to HeaderSent. (No completion check here; one chunk can never complete ct1.) |
| **HeaderSent** (e, auth, dk, Ek encoder, Ct1 decoder) | Emit the next **Ek chunk**. | **Ct1 chunk**: add it. If complete, store ct1 and go to Ct1Received; else stay. |
| **Ct1Received** (e, auth, dk, ct1, Ek encoder) | Emit the next chunk of the same Ek encoder as an **EkCt1Ack chunk**. | **Ct2 chunk**: start a Ct2 decoder (160 bytes), add the chunk, go to EkSentCt1Received (no completion check). |
| **EkSentCt1Received** (e, auth, dk, ct1, Ct2 decoder) | Emit **Ct1Ack** (no chunk). | **Ct2 chunk**: add it. If not complete, stay. If complete: split into ct2 (first 128 bytes) and MAC (last 32); K = ML-KEM-768 decapsulation of ct1 ‖ ct2 with dk; S_e = SCKA key(K, e); authenticator Update(e, S_e); check Ciphertext MAC(e, ct1 ‖ ct2) against the received MAC, failing with **MacVerifyFailed** on mismatch. On success, output the **epoch secret (e, S_e)** and go to **NoHeaderReceived with epoch e + 1** (send_ct role), with a fresh Hdr decoder (96 bytes). |

### 7.2 send_ct role

| State (stored data) | On send | On receive with M.epoch = e |
|---------------------|---------|-----------------------------|
| **NoHeaderReceived** (e, auth, Hdr decoder) | Emit **None**. | **Hdr chunk**: add it. If complete: split into hdr (first 64) and MAC (last 32), check Header MAC(e, hdr), failing with **MacVerifyFailed** on mismatch. On success go to HeaderReceived with a fresh Ek decoder (1152 bytes). |
| **HeaderReceived** (e, auth, hdr, Ek decoder) | Draw 32 random bytes m, run the first encapsulation half on hdr (`mlkem-incremental.md`), giving ct1, the shared secret K and an encapsulation state. S_e = SCKA key(K, e); authenticator Update(e, S_e). Start the Ct1 encoder over ct1 and emit **Ct1 chunk** #0. Output the **epoch secret (e, S_e)**. Go to Ct1Sampled. | Ignore everything (the send_ek side sends Ek chunks only after it has seen a Ct1 chunk). |
| **Ct1Sampled** (e, auth, hdr, encapsulation state, ct1, Ct1 encoder, Ek decoder) | Emit the next **Ct1 chunk**. | **Ek chunk** (ack = no) or **EkCt1Ack chunk** (ack = yes): add it to the Ek decoder. If complete: check the ek payload against hdr (`mlkem-incremental.md`), failing with **ErroneousDataReceived** if they do not match; then, with ack, finish encapsulation (ct2), compute Ciphertext MAC(e, ct1‖ct2), start the Ct2 encoder over ct2 ‖ MAC and go to **Ct2Sampled**; without ack go to **EkReceivedCt1Sampled**. If not complete: with ack go to **Ct1Acknowledged**; without ack stay. |
| **EkReceivedCt1Sampled** (e, auth, encapsulation state, ek payload, ct1, Ct1 encoder) | Emit the next **Ct1 chunk**. | **Ct1Ack** or **EkCt1Ack chunk** (the chunk's data is not used): finish encapsulation, MAC, start the Ct2 encoder, go to Ct2Sampled. |
| **Ct1Acknowledged** (e, auth, hdr, encapsulation state, ct1, Ek decoder) | Emit **None**. | **Ek chunk** or **EkCt1Ack chunk**: add it. If complete: check against hdr (**ErroneousDataReceived** on mismatch), finish encapsulation, MAC, start the Ct2 encoder, go to Ct2Sampled. Else stay. |
| **Ct2Sampled** (e, auth, Ct2 encoder) | Emit the next **Ct2 chunk**. | Special epoch rule: **M.epoch = e + 1** (any payload) moves to **KeysUnsampled with epoch e + 1** (send_ek role), keeping the authenticator. The payload of that message is not otherwise processed. M.epoch > e + 1 fails with **EpochOutOfRange**. M.epoch ≤ e is ignored. |

"Finish encapsulation" means computing ct2 from the stored encapsulation
state and the ek payload (`mlkem-incremental.md`). Encoders keep
producing chunks for as long as the state keeps sending: after the data
chunks come parity chunks (`chunking.md`).

### 7.3 A typical epoch (lockstep, one message each way)

Initiator I (send_ek, KeysUnsampled e = 1), responder R (send_ct,
NoHeaderReceived e = 1):

1. I sends Hdr #0, #1, #2 (one per message). After the third, R's Hdr
   decoder completes and R checks the MAC.
2. R's next send encapsulates. It sends Ct1 #0 and adds S_1 to its chain.
   I, on Ct1 #0, starts sending Ek chunks.
3. R keeps sending Ct1 #1 … #29 while I sends Ek #0, #1, …. When I has 30
   Ct1 chunks, it switches to sending the remaining Ek chunks as
   EkCt1Ack.
4. When R has 36 Ek chunks and an acknowledgement, it validates ek,
   finishes encapsulation and sends Ct2 #0 … #4.
5. When I has 5 Ct2 chunks, it decapsulates, checks the MAC, adds S_1 to
   its chain and becomes send_ct with epoch 2, sending None payloads
   labelled epoch 2.
6. When R sees a message labelled epoch 2, it becomes send_ek with epoch 2
   and starts over with roles swapped.

Lost or reordered messages only delay this. Any n distinct chunks of a
stream are enough (`chunking.md`), and old-epoch messages are ignored by
the SCKA while still yielding their message keys from the chain.

## 8. Randomness consumed

| Where | Bytes | Use |
|-------|------:|-----|
| KeysUnsampled send | 64 | ML-KEM-768 key generation seed d ‖ z (d first) |
| HeaderReceived send | 32 | ML-KEM-768 encapsulation message m (used directly, no hash) |

Nothing else in SPQR is random.

## 9. Error list

All errors below abort the operation; the caller must not keep any state
change. "libsignal effect" is from `session.md`, section 6.

| Error | Raised when | libsignal effect |
|-------|-------------|------------------|
| StateDecode | stored state bytes cannot be parsed or have wrong field sizes (`state.md`) | invalid state (candidate session skipped) |
| MsgDecode | message parse failure (`wire.md`) | invalid message |
| VersionMismatch | V0 message on a V1 state without negotiation record | invalid message |
| MinimumVersion | V0 message on a V1 state whose minimum version is V1 | invalid message |
| EpochOutOfRange(e) | SCKA: message epoch ahead of the state (section 7); chain: epoch ahead of current or already dropped | invalid message |
| MacVerifyFailed | header MAC or ciphertext MAC mismatch | invalid message |
| ErroneousDataReceived | reassembled ek payload does not match hdr | invalid message |
| KeyJump(c, i) | receive index more than max_jump ahead | invalid message |
| KeyTrimmed(i) | receive index older than max_ooo_keys | invalid message |
| KeyAlreadyRequested(i) | receive index already used, or index equal to the counter (including 0) | invalid message |
| SendKeyEpochDecreased | sending key requested for an older epoch than before | (cannot happen with a consistent state) |
| ChainNotAvailable | no chain and no negotiation record | (cannot happen with a consistent state) |

A message with an **unknown version byte** (greater than 1) is not an
error (section 2.4, step 1).

Persistent failure case: if the ciphertext MAC check fails at
EkSentCt1Received, the state is not saved. Every later message with a
Ct2 chunk for that epoch will hit the same failure, because the decoder
already had everything it needed. With honest peers this cannot happen,
since the outer SignalMessage MAC authenticates every chunk.
