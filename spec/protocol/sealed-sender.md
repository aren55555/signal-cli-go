# Sealed sender v1 and v2 in libsignal v0.99.1, as used by signal-cli v0.14.7

Spec role: prose only. This file covers the byte layouts, KDF labels,
ciphers, certificate rules and the way signal-cli 0.14.7 uses sealed sender.
XEd25519 signature details are in `spec/crypto/xeddsa.md` and are not
repeated here. Nothing in this file is code.

## Sources

| Ref | What | URL @ version |
|-----|------|---------------|
| SS1 | libsignal | https://github.com/signalapp/libsignal @ tag v0.99.1 = commit 97801d22dcf9f5bf714f7b8fa3212cdc973ae1c8: `rust/protocol/src/sealed_sender.rs`, `rust/protocol/src/proto/sealed_sender.proto`, `rust/protocol/src/crypto.rs`, `rust/protocol/src/protocol.rs` (message-type numbers, PlaintextContent framing), `rust/core/src/address.rs` (ServiceId fixed-width form, DeviceId range), `rust/protocol/tests/sealed_sender.rs`, `java/client/src/main/java/org/signal/libsignal/metadata/SealedSessionCipher.java`, `java/shared/java/org/signal/libsignal/metadata/certificate/CertificateValidator.kt`, `java/client/src/test/java/org/signal/libsignal/metadata/{SealedSessionCipherTest.java,certificate/*}`, `java/server/src/test/java/org/signal/libsignal/protocol/SealedSenderMultiRecipientMessageTest.java`, `node/ts/test/protocol/SealedSenderTest.ts` |
| SS2 | signal-cli | https://github.com/AsamK/signal-cli @ tag v0.14.7 = commit b01b6b370dc063599a1a2b9fde0f5ff4e2d78fe8: `lib/src/main/java/org/asamk/signal/manager/config/{LiveConfig,StagingConfig,ServiceEnvironmentConfig}.java`, `lib/src/main/java/org/asamk/signal/manager/helper/{SendHelper,UnidentifiedAccessHelper,IncomingMessageHandler}.java` |
| SS3 | signal-service-java as shipped in the reference distribution | `/work/third_party/signal-cli-0.14.7/lib/signal-service-java-2.15.3_unofficial_151.jar`: the bundled `SignalService.proto` (Envelope), plus `javap -c` of `SignalServiceCipher`, `EnvelopeContent$Encrypted`, `EnvelopeContent$Plaintext`, `ContentHint`, `SignalServiceMessageSender` and `SignalServiceDataMessage`, and strings for HTTP paths |
| SS4 | Signal blog post "Technology preview: Sealed sender for Signal" (background only) | https://signal.org/blog/sealed-sender/ |

Signal publishes no formal specification of sealed sender. Everything below
comes from SS1 to SS3.

## 1. Where sealed sender sits

- A sealed-sender message travels as an `Envelope` (SS3 `SignalService.proto`)
  with `type` = **6** (`UNIDENTIFIED_SENDER`). Its `content` field (field 8,
  bytes) holds the sealed bytes: a v1 message, or the per-recipient
  *received* form of a v2 message. The envelope carries no
  `sourceServiceId` (11) or `sourceDeviceId` (7). The envelope's
  `serverTimestamp` (field 10, uint64 milliseconds) matters for certificate
  validation; see 4.4.
- Other Envelope types, for orientation: 1 `DOUBLE_RATCHET`, 3
  `PREKEY_MESSAGE`, 5 `SERVER_DELIVERY_RECEIPT`, 8 `PLAINTEXT_CONTENT`.
  Values 2 and 7 are reserved.
- **Inside** the seal there is a sender certificate, a message type, and
  the ciphertext of an ordinary 1:1 SignalMessage or PreKeySignalMessage, a
  group SenderKeyMessage, or a PlaintextContent. Sealed sender only wraps
  these. It does not touch the Double Ratchet, PQXDH, SPQR or sender-key
  state; see `double-ratchet.md`, `pqxdh.md` and `sender-keys.md`.
  **SPQR interface:** none. A SignalMessage carried inside a seal is
  byte-identical to one sent unsealed, including any `pq_ratchet` field.

## 2. The version byte

Every sealed message starts with one byte: the high nibble is the version
the reader must support and the low nibble is the version the writer used.

| Byte | Meaning | Produced by libsignal v0.99.1 |
|------|---------|-------------------------------|
| `0x11` | v1 | Every v1 message |
| `0x22` | v2, recipients identified by bare 16-byte ACI UUIDs (SentMessage), **and** the only version byte used in a v2 *ReceivedMessage* | Received form only (the server writes it when fanning out) |
| `0x23` | v2 SentMessage with 17-byte ServiceIds | Every v2 SentMessage |

The decryptor (recipient side) looks **only at the high nibble**:
- 0 or 1: v1 layout. Accepting 0 is a libsignal quirk.
- 2: v2 received layout (the low nibble is ignored).
- Anything else fails with "unknown sealed sender version" (carrying the
  high nibble).
- An empty input fails with "invalid sealed sender message: Message was
  empty".

The SentMessage parser (server side, and needed only for fan-out tests)
accepts exactly `0x22` and `0x23`. `0x11`, `0x2F` and `0x77` are rejected
(vectors exist).

## 3. Protobufs (`sealed_sender.proto`, proto2, package `signal.proto.sealed_sender`)

All fields are `optional`. "Required" below means libsignal rejects the
message without it (error: invalid protobuf encoding). Unknown fields are
skipped. Signatures are always computed over the **raw bytes** of the
inner `certificate` field as received, so unknown fields inside it are
covered by the signature, and implementations must not re-encode before
verifying.

### 3.1 ServerCertificate

| Field | No. | Type | Notes |
|-------|----:|------|-------|
| certificate | 1 | bytes | Required. Serialized `ServerCertificate.Certificate`. |
| signature | 2 | bytes | Required. XEd25519 by a trust root over field 1's bytes. |

`ServerCertificate.Certificate`:

| Field | No. | Type | Notes |
|-------|----:|------|-------|
| id | 1 | uint32 | Required. The server key ID. |
| key | 2 | bytes | Required. 33-byte serialized Curve25519 public key (`0x05` prefix), parsed per `xeddsa.md`. |

### 3.2 SenderCertificate

| Field | No. | Type | Notes |
|-------|----:|------|-------|
| certificate | 1 | bytes | Required. Serialized `SenderCertificate.Certificate`. |
| signature | 2 | bytes | Required. XEd25519 by the server certificate's `key` over field 1's bytes. |

`SenderCertificate.Certificate`:

| Field | No. | Type | Notes |
|-------|----:|------|-------|
| senderE164 | 1 | string | Optional. |
| senderDevice | 2 | uint32 | Required, and must be a valid device ID, 1 to 127 inclusive. |
| expires | 3 | **fixed64** | Required. Milliseconds since the Unix epoch. |
| identityKey | 4 | bytes | Required. The sender's 33-byte identity public key. |
| *oneof signer:* certificate | 5 | bytes | A complete embedded serialized ServerCertificate (3.1). |
| *oneof signer:* id | 8 | uint32 | A reference to a built-in known server certificate (4.2). |
| *oneof senderUuid:* uuidString | 6 | string | The sender ACI as a hyphenated UUID string. |
| *oneof senderUuid:* uuidBytes | 7 | bytes | The sender ACI as 16 raw bytes. libsignal turns it into the lowercase hyphenated string. Any length other than 16 is an error. |

One member of `signer` and one of `senderUuid` must be present. Field
order on the wire is arbitrary. One upstream fixture encodes the fields in
the order 2, 3, 4, 8, 6. When libsignal creates a certificate itself it
writes `uuidString` and an embedded signer. Real servers may send either
signer form.

### 3.3 UnidentifiedSenderMessage (v1 outer envelope, after the version byte)

| Field | No. | Type | Notes |
|-------|----:|------|-------|
| ephemeralPublic | 1 | bytes | Required. 33-byte serialized ephemeral public key. |
| encryptedStatic | 2 | bytes | Required. 43 bytes: 33 bytes of AES-CTR ciphertext followed by a 10-byte MAC (5.2). |
| encryptedMessage | 3 | bytes | Required. Ciphertext of the USMC followed by a 10-byte MAC. |

### 3.4 UnidentifiedSenderMessage.Message ("USMC", the sealed plaintext of both v1 and v2)

| Field | No. | Type | Notes |
|-------|----:|------|-------|
| type | 1 | enum Type | Required. It must be one of the four known values below; anything else is an invalid protobuf error. |
| senderCertificate | 2 | bytes | Required. Serialized SenderCertificate (3.2). |
| content | 3 | bytes | Required. The inner message bytes (see the table below). |
| contentHint | 4 | enum ContentHint | Optional. Absent means 0 ("default"). |
| groupId | 5 | bytes | Optional. libsignal never writes it as an empty value; an empty group id given to the encoder is omitted. |

`Type` values and what `content` holds:

| Proto value | Name | `content` holds | libsignal CiphertextMessageType |
|------------:|------|-----------------|---------------------------------|
| 1 | PREKEY_MESSAGE | serialized PreKeySignalMessage (version byte included) | PreKey = 3 |
| 2 | MESSAGE | serialized SignalMessage (version byte included) | Whisper = 2 |
| 7 | SENDERKEY_MESSAGE | serialized SenderKeyMessage | SenderKey = 7 |
| 8 | PLAINTEXT_CONTENT | serialized PlaintextContent: byte `0xC0`, then the Content protobuf, then a single `0x80` padding-boundary byte | Plaintext = 8 |

Values 3 to 6 are reserved. Note the PreKey mismatch: proto value 1
corresponds to the API type number 3, and Java's
`DecryptionResult.getCiphertextMessageType()` reports 3.

`ContentHint` values: 0 = default (reserved in the enum and written by
omitting the field), 1 = RESENDABLE, 2 = IMPLICIT. libsignal keeps any
other number as "unknown(n)" rather than failing. signal-service-java's
`ContentHint` enum uses the same numbers (0 DEFAULT, 1 RESENDABLE,
2 IMPLICIT).

## 4. Certificates and trust roots

### 4.1 Validation rule

Given a SenderCertificate, a set of trust-root public keys and a
validation time in milliseconds, libsignal decides "valid" as follows:

1. **Resolve the signer.** If the signer is embedded, use the embedded
   ServerCertificate (it was parsed when the SenderCertificate was parsed;
   a malformed embedded certificate makes the whole SenderCertificate
   unparseable). If the signer is a reference by `id`, look the id up in
   the known-certificate table (4.2). An unknown id is an **error**
   ("unknown sealed sender server certificate id"), not merely "invalid".
2. **Server certificate against the trust roots.** A server certificate
   whose key id is on the revocation list is invalid for every root. The
   list in v0.99.1 has a single entry, `0xDEADC357`, a test ID; no
   production certificate is revoked. Otherwise the server certificate is
   valid for a root if that root's XEd25519 signature over the server
   certificate's `certificate` bytes verifies. libsignal checks **every**
   configured root and combines the results without short-circuiting, so
   the time taken does not reveal which root matched. If no root matches,
   the certificate is invalid.
3. **Sender certificate signature.** It must verify under the server
   certificate's `key`, over the sender certificate's `certificate` bytes.
4. **Expiry.** The certificate is expired, and so invalid, only when the
   validation time is **strictly greater** than `expires`. A validation
   time equal to `expires` is still valid.

In the Java API an invalid result becomes `InvalidCertificateException`
("Validation failed"); inside `SealedSessionCipher.decrypt` it is wrapped
as `InvalidMetadataMessageException`. In Rust's own one-shot decrypt it
becomes "invalid sealed sender message: trust root validation failed".

### 4.2 Built-in known server certificates

A sender certificate may name its signer by `id` (field 8) instead of
embedding it, to save space. libsignal v0.99.1 carries this table:

| id | Trust root that signed it (base64 of the 33-byte key) | Server key in the certificate | Upstream label |
|----|------------------------------------|-------------------------------|----------------|
| 2 | `BYhU6tPjqP46KGZEzRs1OL4U39V5dlPJ/X09ha4rErkm` | `0539450d…631e` | staging |
| 3 | `BUkY0I+9+oPgDCn4+Ac6Iu813yvqkDr/ga8DzLxFxuk6` | `05bc9d1d…3a5a` | production |
| `0x7357C357` (1935131479) | `BS/lfaNHzWJDFSjarF+7KQcw//aEr8TPwu2QmV9Yyzt0`, the public key of the all-zero private key | `05847c0d…5a22`, the public key of the all-`0xFF` private key | test only |

The full serialized certificates are in
`testdata/vectors/sealed_sender_certificates.json`. A compatible decoder
must embed at least ids 2 and 3. Whether to include the test id is a
choice. libsignal includes it, but it can validate only under the test
root, which no real configuration trusts.

The "trust root" column is used only by upstream's self-test. During
validation the resolved certificate is checked against the
*caller's configured* roots (4.1, step 2), exactly like an embedded one.

### 4.3 Trust roots configured by signal-cli 0.14.7

signal-cli passes a **list of two roots per environment** (SS2
`LiveConfig`/`StagingConfig`, method `getUnidentifiedSenderTrustRoots`):

| Environment | Root | base64 | 33-byte key (hex) |
|-------------|------|--------|-------------------|
| production (`LIVE`) | 1 | `BXu6QIKVz5MA8gstzfOgRQGqyLqOwNKHL6INkv3IHWMF` | `057bba408295cf9300f20b2dcdf3a04501aac8ba8ec0d2872fa20d92fdc81d6305` |
| production (`LIVE`) | 2 | `BUkY0I+9+oPgDCn4+Ac6Iu813yvqkDr/ga8DzLxFxuk6` | `054918d08fbdfa83e00c29f8f8073a22ef35df2bea903aff81af03ccbc45c6e93a` |
| staging | 1 | `BbqY1DzohE4NUZoVF+L18oUPrK3kILllLEJh2UnPSsEx` | `05ba98d43ce8844e0d519a1517e2f5f2850facade420b9652c4261d949cf4ac131` |
| staging | 2 | `BYhU6tPjqP46KGZEzRs1OL4U39V5dlPJ/X09ha4rErkm` | `058854ead3e3a8fe3a286644cd1b3538be14dfd5797653c9fd7d3d85ae2b12b926` |

Root 2 of each environment is the root behind known certificate id 3
(production) or id 2 (staging), so referenced certificates validate in the
matching environment. A certificate from the other environment never
validates.

### 4.4 Validation time used by signal-cli

signal-service-java's `SignalServiceCipher` (SS3) passes the **envelope's
`serverTimestamp`** (Envelope field 10) as the validation time, not the
local clock. A captured envelope therefore still unseals later, even after
the sender certificate's expiry, provided the original server timestamp is
kept with it.

### 4.5 Sender certificates that signal-cli itself uses (sending side)

`UnidentifiedAccessHelper` (SS2) obtains certificates from the server:
`/v1/certificate/delivery` (with the E.164) or
`/v1/certificate/delivery?includeE164=false` (the "phone number privacy"
form). It uses the second form unless the phone-number-sharing mode says to
share the number with this recipient. Each certificate is cached and
replaced once less than one day remains before its `expires`. The
certificate's bytes are used unmodified as USMC field 2. HTTP details
belong to the transport spec.

## 5. Sealed sender v1

### 5.1 Primitives

- **X25519**, with libsignal's rejection of an all-zero shared secret
  (`xeddsa.md`).
- **HKDF-SHA256** (RFC 5869) with an **empty info** string and 96 bytes of
  output, split into three 32-byte keys in order.
- **AES-256-CTR** with an all-zero initial counter block. The counter is
  the last 4 bytes, incremented big-endian. For messages under 2^32 blocks
  this is identical to standard AES-256-CTR with a zero 128-bit IV.
  The key is never reused, because every message derives fresh keys.
- **MAC:** HMAC-SHA256 over the ciphertext only, **truncated to the first
  10 bytes**, appended after the ciphertext. On receipt the 10 bytes are
  compared in constant time before decrypting. Input shorter than 10 bytes
  fails as truncated.

### 5.2 Key schedule and layout

Notation: `pub(X)` is the 33-byte serialized public key (`0x05` ‖ u). `‖`
is concatenation. "Sender identity" is the sender's own identity key pair:
signal-cli always sends sealed messages from its ACI identity. "Recipient
identity" is the identity key the sender has stored for the destination
address; if none is stored, encryption fails with "session not found".

1. Generate a fresh ephemeral X25519 key pair E.
2. **Ephemeral keys.**
   - salt = the ASCII bytes `UnidentifiedDelivery` (20 bytes)
     ‖ pub(recipient identity) ‖ pub(E). 86 bytes in total.
   - IKM = X25519(E private, recipient identity public).
   - The HKDF output, in order, is *e_chain* (32), *e_cipher* (32) and
     *e_mac* (32).
3. **encryptedStatic** = AES-CTR(e_cipher, pub(sender identity)), which is
   33 bytes, followed by the 10-byte truncated HMAC(e_mac) over those 33
   bytes. 43 bytes in total.
4. **Static keys.**
   - salt = e_chain ‖ encryptedStatic. All 43 bytes, so the MAC is
     included. 75 bytes in total.
   - IKM = X25519(sender identity private, recipient identity public).
   - Take 96 bytes of output. **Discard the first 32**; the next 32 are
     *s_cipher* and the last 32 are *s_mac*.
5. **encryptedMessage** = AES-CTR(s_cipher, serialized USMC), followed by
   the 10-byte truncated HMAC(s_mac) over that ciphertext.
6. **Output:** the byte `0x11`, then the protobuf of 3.3 with
   ephemeralPublic = pub(E) (33 bytes), encryptedStatic and
   encryptedMessage. libsignal writes the fields in the order 1, 2, 3.

### 5.3 Receiving v1

The recipient uses its own identity key pair. In signal-cli this is the
ACI identity for an ACI destination (see 8).

1. Parse the version byte (high nibble 0 or 1) and the protobuf. All
   three fields must be present, and ephemeralPublic must parse as a
   public key.
2. Derive the ephemeral keys as in 5.2 step 2. The salt order is the same:
   the recipient identity key first, then the ephemeral key. The IKM is
   X25519(recipient identity private, ephemeral public).
3. Check and decrypt encryptedStatic. A MAC failure gives "invalid sealed
   sender message: failed to decrypt sealed sender v1 message key". The
   33 plaintext bytes must parse as a public key, the claimed sender
   identity.
4. Derive the static keys with the claimed sender identity (5.2 step 4).
   The salt uses encryptedStatic exactly as received.
5. Check and decrypt encryptedMessage. A MAC failure gives "… failed to
   decrypt sealed sender v1 message contents". Then parse the USMC.
6. **Binding check.** The decrypted static-key bytes must equal, in
   constant time, the 33-byte serialization of the USMC sender
   certificate's `identityKey`. Otherwise: "sender certificate key does
   not match message key".
7. Certificate validation (4.1) follows as a separate step; see 7.

## 6. Sealed sender v2 (multi-recipient)

### 6.1 Constants

| Name | Value |
|------|-------|
| Label r | ASCII `Sealed Sender v2: r (2023-08)` |
| Label K | ASCII `Sealed Sender v2: K` |
| Label DH | ASCII `Sealed Sender v2: DH` |
| Label DH-sender | ASCII `Sealed Sender v2: DH-sender` |
| Message key length (M, C) | 32 bytes |
| Authentication tag length (AT) | 16 bytes |
| Public key on the wire (E.pub) | 32 bytes, raw u-coordinate, **no** `0x05` prefix |
| AEAD | AES-256-GCM-SIV (RFC 8452), 12-byte nonce of all zeros, empty associated data, 16-byte tag appended after the ciphertext |

All HKDFs in v2 are HKDF-SHA256 with **no salt** (RFC 5869: a salt of 32
zero bytes) and the label as the **info** string.

### 6.2 Sender computation

1. Draw 32 random bytes M.
2. Derive two values from M, using M as the IKM with no salt:
   - r = 32 bytes with info Label r;
   - K = 32 bytes with info Label K.

   Both come from the same HKDF extract.
3. The ephemeral private key is r, imported as a libsignal private key,
   which means it is **clamped** (`xeddsa.md`). E is the resulting key
   pair. E is deterministic given M.
4. For each recipient i, with identity public key R_i:
   - C_i = (HKDF output of 32 bytes over the IKM X25519(E private, R_i)
     ‖ pub(E) ‖ pub(R_i), with info Label DH) XOR M.
     The keys in this IKM are the 33-byte prefixed forms. The total IKM
     length is 32 + 33 + 33 = 98 bytes.
   - AT_i = HKDF output of 16 bytes over the IKM X25519(S private, R_i)
     ‖ pub(E) ‖ C_i ‖ pub(S) ‖ pub(R_i), with info Label DH-sender. S is
     the sender's identity key pair. The total IKM length is
     32 + 33 + 32 + 33 + 33 = 163 bytes.
5. The ciphertext is AES-256-GCM-SIV under K (zero nonce, no AAD) of the
   serialized USMC, followed by the 16-byte tag. It is computed once for
   all recipients.

C_i and AT_i are computed **once per recipient identity** (per service
ID), not per device.

### 6.3 SentMessage layout (client → server, `0x23`)

Fixed-width integers are big-endian and unaligned. "varint" is the
protobuf unsigned varint.

| Part | Size | Content |
|------|------|---------|
| version | 1 | `0x23`. (`0x22` is the legacy form where each recipient is a bare 16-byte ACI UUID; libsignal still parses it but does not produce it.) |
| count | varint | The number of recipient entries that follow, included and excluded together. |
| entries | variable | `count` entries, as below. |
| E.pub | 32 | Raw ephemeral public key. |
| ciphertext | the rest | AEAD ciphertext followed by its 16-byte tag. |

An **included recipient entry** consists of:
- the ServiceId in fixed-width binary form, 17 bytes. The first byte is
  the kind (`0x00` for an ACI, `0x01` for a PNI) and the remaining 16
  bytes are the UUID. Any other kind byte is a parse error.
- one or more device records of 3 bytes each: the device ID (1 byte,
  1 to 127), then a 2-byte big-endian word. Bit 15 of that word is
  "has more", which is set on every device record of the entry except the
  last. Bit 14 is unused and is 0 when written. Bits 0 to 13 are the
  device's registration ID.
- C_i (32 bytes) and AT_i (16 bytes).

An **excluded recipient entry** consists of the 17-byte ServiceId followed
by a single `0x00` byte. Device ID 0 is never valid, so it serves as the
marker. An excluded recipient carries no C or AT.

How libsignal's encoder fills these fields:
- Each destination address must already have a session. The registration
  ID comes from that session's stored remote registration ID. Without one,
  encoding fails with an invalid-state error ("maybe it was recently
  archived"). A value above `0x3FFF` fails with "invalid registration ID".
- The destination's name must parse as a ServiceId string; otherwise
  encoding fails with an invalid-argument error.
- Consecutive destinations with the same name are grouped into one entry,
  using one identity lookup, which must succeed or "session not found"
  results. If a name appears again non-contiguously, it simply produces
  another entry for the same service ID; that output is valid but larger.
- `count` = number of grouped entries + number of excluded recipients. The
  included entries come first, in input order, then the excluded entries.

### 6.4 SentMessage parsing and server fan-out

Rules the parser enforces (SS1; vectors in
`sealed_sender_v2_sent_message.json`):
- **Version byte.** It must be `0x22` or `0x23`. With `0x22`, each
  recipient is a bare 16-byte UUID, treated as an ACI.
- **Device records.** Records are read until one has "has more" clear. A
  device byte of 0 is allowed only as the *first* byte of an entry (an
  excluded entry). A 0 after a "has more" flag is an error. A device byte
  above 127 is an error. Registration IDs are read as the low 14 bits.
- **Repeated service IDs.** When an included entry repeats a service ID,
  its devices are appended to the earlier entry's list. Duplicate devices
  are not diagnosed. The **first** occurrence's C‖AT is the one used.
  Combining an excluded entry with any other entry for the same service ID,
  in either order, is an error ("recipient redundantly encoded as empty").
  So is excluding the same service ID twice.
- **Shared tail.** At least 32 bytes must remain after the last entry.
  These remaining bytes are the shared tail E.pub ‖ ciphertext.
- **Truncation.** Running out of bytes anywhere is an error. An empty
  input is an error.

The **ReceivedMessage** that the server delivers to each device of an
included recipient, as Envelope type 6 `content`, is:

| Part | Size | Content |
|------|------|---------|
| version | 1 | **always `0x22`**, whichever version byte the SentMessage had |
| C | 32 | That recipient's C_i |
| AT | 16 | That recipient's AT_i |
| E.pub | 32 | Copied from the SentMessage |
| ciphertext | the rest | Copied from the SentMessage (AEAD ciphertext followed by its tag) |

The registration IDs and device IDs exist for the server, which checks
them against the account's devices. They never reach the recipient.

### 6.5 Receiving v2

The recipient uses its own identity key pair, with pub(R) as its 33-byte
public key.

1. Split the input as in the table above. Fewer than 81 bytes is an
   invalid protobuf encoding error. E.pub is read as a raw 32-byte key.
2. M = (HKDF output of 32 bytes over the IKM X25519(R private, E.pub)
   ‖ pub(E) ‖ pub(R), with info Label DH) XOR C. pub(E) here is the
   33-byte prefixed form of the received raw key.
3. Re-derive r and K from M (6.2 step 2) and the key pair from r. The
   derived public key must equal the received E.pub; the comparison is
   constant-time. Otherwise: "derived ephemeral key did not match key
   provided in message".
4. Decrypt the ciphertext with AES-256-GCM-SIV under K (zero nonce, no
   AAD). On failure: "failed to decrypt inner message: …". Then parse the
   USMC.
5. Recompute AT using the sender certificate's `identityKey` as pub(S),
   with the received C. The input is laid out as in 6.2 step 4, with
   X25519(R private, S public) as the DH value and pub(S) placed before
   pub(R). It must equal the received AT; the comparison is constant-time.
   Otherwise: "sender certificate key does not match authentication tag".
6. Certificate validation (4.1) follows as a separate step; see 7.

## 7. After unsealing (receiver)

In this order (SS1 Java `SealedSessionCipher.decrypt`, which signal-cli
uses through signal-service-java):

1. Unseal v1 or v2 (5.3 or 6.5), then validate the sender certificate
   against the configured trust-root list at the envelope's server
   timestamp (4.1, 4.4). Any failure in this step, whether parsing,
   cryptography or certificate validation, becomes a single
   `InvalidMetadataMessageException`. The caller learns nothing about the
   sender.
2. **Self-send check.** The message is from the local account if either
   the certificate's sender UUID string equals the local ACI UUID string,
   or both E.164 values are present and equal. If, in addition, the
   certificate's device equals the local device ID, the result is
   `SelfSendException`. signal-cli catches this and **silently drops** the
   message (debug log "Dropping unidentified message from self.").
3. **Dispatch on the USMC type.** The sender's protocol address is the
   certificate's UUID string and device.

   | Type | Handling |
   |------|----------|
   | MESSAGE (2) | Session decrypt of a SignalMessage |
   | PREKEY_MESSAGE (1) | Session decrypt of a PreKeySignalMessage |
   | SENDERKEY_MESSAGE (7) | Group-cipher decrypt of a SenderKeyMessage |
   | PLAINTEXT_CONTENT (8) | Strip the `0xC0` marker; the content is returned unencrypted |

   Errors from this step are re-thrown as `Protocol*Exception` and **carry
   the sender's identity, device, content hint and group id**, so the
   client can send a retry request. Rust's own one-shot decrypt accepts
   only types 1 and 2.
4. The result gives the sender UUID, the optional E.164, the device, the
   message type, the optional group id and the padded plaintext. Removing
   the padding is the job of the Content layer, not of sealed sender.

## 8. How signal-cli 0.14.7 sends and receives

**Receiving** (SS2 `IncomingMessageHandler`, SS3 `SignalServiceCipher`):
- The identity used to unseal belongs to the envelope's destination
  service ID: the ACI identity when the destination is absent or equals
  the account ACI, otherwise the PNI identity.
- An envelope of type `UNIDENTIFIED_SENDER` addressed to the account's
  **PNI** is rejected before decryption ("Got a sealed sender message to
  our PNI? …").
- Both v1 and v2 received forms are accepted, because libsignal decides by
  the version byte.

**Sending, 1:1** (SS3 `EnvelopeContent$Encrypted`, `$Plaintext`):
- When unidentified access is available, meaning a sender certificate was
  obtained and an access key or group send token is usable, the message is
  sealed as **v1** and submitted with Envelope type 6.
- The USMC carries the session ciphertext type (1 or 2) and the content
  hint chosen by the caller. signal-cli uses IMPLICIT for receipts, typing
  indicators and similar, and RESENDABLE for data messages. The optional
  group id is included when the content belongs to a group.
- Retry-request PlaintextContent is sealed with type 8 and hint IMPLICIT.
- Without unidentified access, the message goes unsealed as type 1 or 3.

**Sending, groups** (SS2 `SendHelper.sendGroupMessageInternal`, SS3
`SignalServiceMessageSender`/`SignalServiceCipher.encryptForGroup`):
- Sender key with **v2** is used when all of these hold:
  - the group has a distribution ID;
  - group send endorsements are available;
  - at least **2** recipients are sender-key capable.
- The sender-key ciphertext goes into a USMC of type 7, with the caller's
  content hint and group id = the **32-byte GV2 group identifier**
  (derived from the master key through zkgroup's group public params). It
  is then sealed with v2 (version `0x23`) and uploaded to
  `/v1/messages/multi_recipient`.
- Any remaining recipients get per-recipient v1 (or unsealed) sends.

## 9. Test vectors

- `testdata/vectors/sealed_sender_certificates.json` (11 entries):
  - the 4 signal-cli trust roots and the test root;
  - the 3 known server certificates, parsed;
  - the 3 fixed SenderCertificate fixtures from libsignal's unit tests.
    These cover an embedded signer with a `uuidString` and an unknown
    field 999, an embedded signer with `uuidBytes`, and a signer referenced
    by id `0x7357C357`. Each comes with its expected parsed fields and
    validity at 31336 ms. The outcomes at 31337 and 31338 ms are derived
    from the expiry rule and marked as such.
- `testdata/vectors/sealed_sender_v2_sent_message.json` (16 entries): the
  SentMessage parsing and fan-out cases from libsignal's server-side Java
  test (layout only, with filler key material), including all rejection
  cases, plus the empty-input case.
- `testdata/vectors/sealed_sender_primitives.json` (1 entry): libsignal's
  AES-256-CTR unit vector (zero counter block).

**No fixed end-to-end vectors exist upstream.** Every v1/v2 encrypt and
decrypt test in the Rust, Java, Node and Swift suites generates its keys,
M and ephemeral keys at runtime. None of them pins HKDF outputs, C, AT or
GCM-SIV ciphertexts. The Java `multiRecipientEncrypt` tests check only
that the excluded ServiceIds appear followed by `0x00`. Implementations
should therefore rely on round trips, the vectors above, standard
RFC 5869 and RFC 8452 vectors for the primitives, and messages captured
from the reference binary.

## 10. Gaps

- The HTTP layer for sealed sending is not specified here: the
  `Unidentified-Access-Key` and `Group-Send-Token` headers, the
  `/v1/messages/multi_recipient?ts=…&online=…&urgent=…&story=…` query, and
  the response handling for 409/410 device mismatches. Only the path
  strings were observed (SS3).
- The full rules signal-cli uses for "sender-key capable"
  (`isSenderKeyCapable`) and for group send endorsements were not
  examined.
- The signatures in the sender-certificate fixtures were not re-verified
  by this agent; upstream asserts them.
