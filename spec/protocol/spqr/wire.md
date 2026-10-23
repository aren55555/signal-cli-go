# SPQR wire format

Spec role: prose only. Sources: P1 `src/lib.rs` (version detection),
`src/v1/chunked/states/serialize.rs` (message layout), `src/encoding.rs`
(chunk type); P2 `rust/protocol/src/proto/wire.proto` and
`rust/protocol/src/protocol.rs`.

## 1. Carrier

The SPQR message is the content of `SignalMessage.pq_ratchet` (field 5,
bytes). An empty SPQR message means the field is left out. See
`session.md`, section 5. An SPQR message is **not protobuf**: it has its
own byte layout. (P1's `pq_ratchet.proto` declares a `V1Msg` message, but
nothing uses it. Do not use it on the wire.)

## 2. Version detection

| SPQR message bytes | Treated as |
|--------------------|-----------|
| empty | V0 |
| first byte `0x00` | V0 (the rest is ignored) |
| first byte `0x01` | V1, parsed as below |
| first byte `0x02`–`0xFF` | unknown future version: the receiver ignores the message without error (`protocol.md`, section 2.4) |

## 3. V1 message layout

| # | Field | Encoding | Notes |
|---|-------|----------|-------|
| 1 | version | 1 byte, `0x01` | |
| 2 | epoch | varint, 1–10 bytes | The sender's SCKA epoch, u64, must be ≥ 1 |
| 3 | key index | varint, 1–5 bytes | Index of the sender's chain key for chain epoch (epoch − 1); u32. 0 means "no key" (only possible in epoch 1 without a chain) |
| 4 | payload type | 1 byte | See below |
| 5 | chunk index | varint, 1–3 bytes | Only for chunk-carrying types; 0…65535 |
| 6 | chunk data | 32 bytes | Only for chunk-carrying types |

Payload types:

| Byte | Name | Has chunk (fields 5–6)? | Meaning |
|-----:|------|:-----------------------:|---------|
| 0 | None | no | nothing to send in this state |
| 1 | Hdr | yes | chunk of hdr ‖ header MAC |
| 2 | Ek | yes | chunk of the ek payload |
| 3 | EkCt1Ack | yes | chunk of the ek payload, plus "I have all of ct1" |
| 4 | Ct1Ack | no | "I have all of ct1" (send_ek, after ek is fully sent and ct2 has started arriving) |
| 5 | Ct1 | yes | chunk of ct1 |
| 6 | Ct2 | yes | chunk of ct2 ‖ ciphertext MAC |

Which state sends which type is in `protocol.md`, section 7.

**Varint**: unsigned base-128, least-significant 7-bit group first, high
bit set on every byte except the last (the protobuf varint). The sender
always writes the shortest form. The receiver reads at most 10 bytes, and
does not reject non-minimal forms. It fails if the input ends before a
byte with the high bit clear, or if no such byte appears within 10 bytes.
Vectors: `testdata/vectors/spqr_varint.json`.

**Sizes**: a message without a chunk is 1 + |epoch| + |index| + 1 bytes
(4 bytes when both varints are short). A chunk message adds |chunk index|
+ 32 bytes (37 bytes when all varints are short; at most 1 + 10 + 5 + 1 +
3 + 32 = 52).

## 4. Parsing rules (receiver)

Parsing fails with MsgDecode when:
- the message is empty or the first byte is not `0x01` (these cases never
  reach the V1 parser; see section 2);
- the epoch varint is malformed or decodes to 0;
- the index varint is malformed or exceeds 2^32 − 1;
- the input ends before the type byte;
- the type byte is greater than 6;
- for chunk types: the chunk-index varint is malformed, the index exceeds
  65535, or fewer than 32 bytes remain.

**Trailing bytes after the last field are allowed and ignored**, to leave
room for future extensions. A Ct1Ack message always means "acknowledged";
it has no flag byte.

## 5. Example layout

An Ek chunk with epoch 3, key index 17, chunk index 200 and 32 data bytes
D is encoded as:

`01` · `03` · `11` · `02` · `c8 01` · D

That is the version, epoch 3, index 17, type Ek, chunk index 200 as the
varint `c8 01`, then the data, 38 bytes in total. This example only shows
the layout; it is not an upstream vector.
