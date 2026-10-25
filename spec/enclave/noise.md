# Noise channel to CDSI and SVR2 enclaves

Spec role: prose only. This file covers what happens after the attestation
in `spec/enclave/attestation.md` has produced the enclave's X25519 static
key (claim `pk`): the Noise handshake over the WebSocket, then
encryption and framing of application messages.

## Sources

| Ref | What | URL @ version |
|-----|------|---------------|
| N1 | libsignal | https://github.com/signalapp/libsignal @ v0.99.1 = 97801d22dcf9f5bf714f7b8fa3212cdc973ae1c8: `rust/attest/src/{client_connection,enclave,sgx_session,cds2,svr2,snow_resolver,hsm_enclave,lib}.rs`, `rust/attest/Cargo.toml` (snow with feature `hfs`, libcrux-ml-kem `mlkem1024`), `rust/net/infra/src/ws/attested.rs`, `rust/bridge/shared/src/sgx_session.rs` |
| N2 | The Noise Protocol Framework, revision 34 (2018-07-11) | https://raw.githubusercontent.com/noiseprotocol/noise_spec/master/noise.md (noiseprotocol.org itself was unreachable from this sandbox; the GitHub copy states revision 34) |
| N3 | KEM-based Hybrid Forward Secrecy for Noise ("hfs" modifier), revision 1 (2018-11-17) | https://raw.githubusercontent.com/noiseprotocol/noise_hfs_spec/master/noise_hfs.md |
| N4 | RFC 8439 (ChaCha20-Poly1305), RFC 7748 (X25519), RFC 2104/5869 (HMAC/HKDF), FIPS 203 (ML-KEM) | public |
| N5 | Reference jar, offline | `/work/third_party/signal-cli-0.14.7/lib/libsignal-client-0.99.1.jar`: `Cds2Client`/`Svr2Client` on the captured staging data (initial-request size); `HsmEnclaveClient` (same Noise library and crypto resolver, pattern `Noise_NK_25519_ChaChaPoly_SHA256`) driven against the spec agent's own scratch Noise responder |
| N6 | signal-service-java (Turasa) | https://github.com/Turasa/libsignal-service-java @ 66956cb4a0394d50cee3590209baf58312084a44: `.../api/svr/Svr2Socket.kt` |

Vectors:
- `testdata/vectors/noise.json`: deterministic NK and NKhfs transcripts.
- `testdata/vectors/rfc8439_chacha20poly1305.json`: the full RFC 8439
  section 2 and appendix A vectors.

## 1. Which pattern

| Use | Noise protocol name | Notes |
|-----|---------------------|-------|
| CDSI (libsignal-net, signal-cli lookups) | `Noise_NKhfs_25519+Kyber1024_ChaChaPoly_SHA256` | post-quantum handshake type |
| SVR2 (`Svr2Client`, signal-cli PIN/registration lock) | `Noise_NKhfs_25519+Kyber1024_ChaChaPoly_SHA256` | post-quantum handshake type |
| (HSM enclave client and legacy pre-quantum mode; not used by signal-cli) | `Noise_NK_25519_ChaChaPoly_SHA256` | reference only |

- "Kyber1024" in the name is just a label. The KEM is **FIPS 203
  ML-KEM-1024**: libsignal plugs libcrux `mlkem1024` into the Noise
  library under that name.
- Sizes: encapsulation key 1568 bytes, ciphertext 1568 bytes, shared
  secret 32 bytes. The encapsulation-key check in FIPS 203 §7.2 is
  applied on encapsulation. The client never encapsulates.
- Go: `crypto/mlkem` (`GenerateKey1024`, `DecapsulationKey1024.Decapsulate`).

Primitives (Noise names → concrete):

| Noise | Concrete |
|-------|----------|
| DH `25519` | X25519 (RFC 7748). Private keys are 32 random bytes, clamped inside X25519. DHLEN = 32. libsignal does **not** reject an all-zero DH output |
| Cipher `ChaChaPoly` | AEAD_CHACHA20_POLY1305 (RFC 8439). The 96-bit nonce is 4 zero bytes followed by the 64-bit counter *n* in little-endian |
| Hash `SHA256` | SHA-256. HASHLEN = 32, BLOCKLEN = 64. HMAC = RFC 2104 HMAC-SHA256 |
| KEM (hfs) | ML-KEM-1024 as above |

**Go note.** The standard library has X25519 (`crypto/ecdh`), ML-KEM
(`crypto/mlkem`), SHA-256 and HMAC. It has **no public
ChaCha20-Poly1305**. That lives in `golang.org/x/crypto`, which this
project may not use. ChaCha20, Poly1305 and the AEAD must therefore be
implemented from RFC 8439. All RFC vectors are provided.

## 2. Keys and roles

- The client is the **initiator**. The enclave is the **responder**.
- Responder static public key `rs` = the attested claim `pk` (32 bytes).
  It is never taken from the unauthenticated protobuf field 1
  (`attestation.md` §3).
- The client creates a fresh X25519 ephemeral and a fresh ML-KEM-1024 key
  pair for every connection. There is no client static key.
- **Prologue: empty.** Under Noise rules it is still hashed:
  MixHash of zero bytes.
- Pre-message pattern `<- s`: MixHash(rs) after the prologue.

## 3. Handshake

All operations below are the standard Noise rev 34 SymmetricState and
CipherState operations (N2 §5), with the HFS tokens defined in N3 §4.

**Initialization.**
- The protocol name `Noise_NKhfs_25519+Kyber1024_ChaChaPoly_SHA256` is
  46 bytes, longer than HASHLEN, so h = SHA-256(name).
- For the pre-quantum `Noise_NK_25519_ChaChaPoly_SHA256`, which is
  exactly 32 bytes, h = the name bytes themselves.
- ck = h, and there is no cipher key yet.

**Message 1, client → server.** Token sequence `e, es, e1`, then the
payload. N3's table gives NKhfs as `-> e, es, e1 / <- e, ee, ekem1`.

1. `e`: generate the ephemeral key pair. Write its 32-byte public key in
   clear, then MixHash(e.pub).
2. `es`: MixKey(X25519(e.priv, rs)). After this a cipher key exists.
3. `e1`: generate an ML-KEM-1024 key pair. Write EncryptAndHash(ek):
   1568 bytes of ciphertext plus a 16-byte tag, 1584 bytes in total. It is
   encrypted because it comes after `es`.
4. Payload: the client sends an **empty** payload, which gives
   EncryptAndHash(empty) = a 16-byte tag.

Total: **1632 bytes** = 32 + 1584 + 16. libsignal's internal buffer
constant is "64 + 1568". The reference jar's `Cds2Client` and
`Svr2Client` both produced 1632-byte initial requests on the captured
staging data (N5).

**Message 2, server → client.** Token sequence `e, ee, ekem1`, then the
payload.

1. `e`: read the server ephemeral `re` (32 bytes), then MixHash(re).
2. `ee`: MixKey(X25519(e.priv, re)).
3. `ekem1`: DecryptAndHash the next 1584 bytes to get the 1568-byte KEM
   ciphertext. Compute ss = ML-KEM-1024.Decaps(dk, ciphertext), then
   MixKey(ss).
4. Payload: DecryptAndHash of the remainder. libsignal reads it into a
   zero-length buffer, so the payload must be empty. The only valid
   message-2 length is therefore **1632 bytes**: 32 + 1584 + 16.

**Split.** (k_c2s, k_s2c) = the two HKDF outputs of Split():
- client → server uses the first key;
- server → client uses the second;
- both nonces start at 0.

The final h is the handshake hash. libsignal computes it and exposes it,
but neither CDSI nor SVR2 uses it.

**Errors.** Any AEAD failure in message 2 fails the connection, as does
a wrong length or a KEM failure. Java surfaces this as
`SgxCommunicationFailureException`. libsignal-net reports a Noise
handshake error.

Reminders of the Noise building blocks (N2), so the ordering is
unambiguous:
- **MixHash(d):** h = SHA-256(h ‖ d).
- **MixKey(ikm):** let o1, o2 be the two outputs of the Noise HKDF with
  chaining key ck and input ikm. ck = o1, k = o2, n = 0. The Noise HKDF
  is: t = HMAC(ck, ikm), o1 = HMAC(t, 0x01), o2 = HMAC(t, o1 ‖ 0x02).
- **EncryptAndHash(p):**
  - with no key, the output is p, followed by MixHash(p);
  - otherwise the output is c = AEAD(k, n, ad = h, p), followed by
    n += 1 and MixHash(c).
- **Split:** the two outputs of the HKDF with ck and an empty ikm.

**What was cross-checked.**
- The pre-quantum NK pattern was cross-checked end to end against
  libsignal's own Noise stack. A scratch responder written from N2
  completed handshakes and exchanged transport messages with
  `HsmEnclaveClient`, which uses the same library and resolver. That
  covers name handling, empty-prologue hashing, HKDF, nonce encoding
  and chunked framing (N5).
- For the hfs part, only the message-1 layout was confirmed (its size
  proves `e1` is encrypted, i.e. placed after `es`).
- The message-2 order (`ee` before `ekem1`, MixKey(ss) after the
  ciphertext is decrypted) follows N3 and could not be checked against
  libsignal, because no enclave private key matching a valid attestation
  exists (Gap G1).

## 4. Transport limits and nonces

- One Noise transport message carries at most **65 535** bytes:
  plaintext ≤ 65 519 bytes plus a 16-byte tag.
- Associated data for transport messages is empty.
- Each direction has its own key and its own 64-bit counter. The counter
  starts at 0 and increases by one per Noise message, not per WebSocket
  frame.
- There is no rekey. A connection carries only a handful of messages.

## 5. Framing over the WebSocket

**Sending an application message (client).**
- Split the plaintext into consecutive chunks of 65 519 bytes. Only the
  last chunk may be shorter.
- Seal each chunk as its own Noise transport message, using successive
  nonces.
- Concatenate the ciphertexts and send them as **one** binary frame.
  The frame's size is the plaintext length + 16 × (number of chunks).
- **Empty plaintext gives zero chunks**, so the frame is empty: no tag at
  all. Observed with the jar: sending 0 bytes returned a 0-byte frame.
  libsignal-net still hands that empty frame to the socket.
  `Svr2Socket` never sends an empty request.

**Receiving (client).**
- Split the received frame into consecutive 65 535-byte pieces. The last
  piece may be shorter.
- Open each piece as one transport message with successive nonces.
- Concatenate the plaintexts.
- The server must therefore chunk the same way: every message except the
  last exactly 65 535 bytes.
- A final piece shorter than 16 bytes, or any tag failure, is an error.

Observed with the jar against the scratch responder:

| Plaintext | Frame size |
|-----------|------------|
| 3 bytes | 19 bytes |
| 65 519 bytes | 65 535 bytes |
| 65 520 bytes | 65 552 bytes (2 messages) |
| 200 000 bytes | 200 064 bytes (4 messages) |

All of these round-tripped correctly in both directions.

**Message content.** Each decrypted frame is one serialized protobuf of
the service protocol (`spec/service/cdsi.md`, `spec/service/svr.md`). In
libsignal-net, a protobuf that fails to decode is a protocol error.

## 6. Session lifecycle

**CDSI** (libsignal-net):
1. Frame 1 is the attestation.
2. The client sends handshake message 1 and reads message 2.
3. Request/response frames follow, as in `cdsi.md`.
4. The server ends with Close 1000 or with an error code
   (`attestation.md` §2.5).

**SVR2** (signal-cli, N6): exactly one request per connection.
1. On the first frame, the client constructs `Svr2Client` with the
   current time (attestation) and sends the initial request.
2. On the second frame, it completes the handshake and sends one
   encrypted request.
3. On the third frame, it decrypts the response and closes with
   1000 "OK".

## 7. Test vectors (`testdata/vectors/noise.json`)

Two deterministic transcripts, NK and NKhfs, use these fixed byte
strings:

| Value | Bytes |
|-------|-------|
| responder static private | 0x01…0x20 |
| initiator ephemeral private | 0x21…0x40 |
| responder ephemeral private | 0x41…0x60 |
| ML-KEM seed d‖z | 0x61…0xa0 (Go `mlkem.NewDecapsulationKey1024`) |
| encapsulation randomness m | 0xa1…0xc0 (Go `mlkemtest.Encapsulate1024`) |

Each transcript includes:
- the initial h;
- message 1 and message 2 (both 1632 bytes for NKhfs, 48 bytes for NK);
- the KEM public key, ciphertext and shared secret;
- the handshake hash;
- both split keys;
- three transport messages with direction and nonce. These are raw Noise
  messages, not libsignal frames.

Use them to test both roles:
- a client implementation must reproduce message 1 given the
  initiator-side fixed values;
- it must accept message 2 and decrypt the responder-to-initiator
  transport messages.

## Gaps

- **G1.** The NKhfs second message (`<- e, ee, ekem1`) and the
  MixKey(kem output) placement come from N3. They agree with every
  observable size, but were not confirmed against libsignal's live
  behaviour, because doing so needs a valid attestation whose private key
  is known. The first live staging handshake will settle it. If it fails
  with a decrypt error on message 2, re-check this ordering first.
- **G2.** Noise's documented rule (N2 §5.1) reserves nonce 2^64−1, and the
  Noise library libsignal uses refuses to encrypt at that point. This is
  unreachable in practice.
- **G3.** The behaviour if the server sends a non-empty message-2 payload
  is inferred from libsignal passing a zero-length output buffer. It is
  expected to be an error, but this was not observed.
