# Attested enclave channel (CDSI and SVR2): transport, handshake message, SGX DCAP attestation

Spec role: prose only. This file covers what a client does **before** the
Noise session exists: how it opens the WebSocket, what the server's first
frame contains, and how libsignal v0.99.1 decides whether to trust it. The
Noise handshake and transport framing are in `spec/enclave/noise.md`. The
request/response protocols that run inside the channel are in
`spec/service/cdsi.md` and `spec/service/svr.md`.

## Sources

| Ref | What | URL @ version |
|-----|------|---------------|
| A1 | libsignal attestation crate | https://github.com/signalapp/libsignal @ tag v0.99.1 = commit 97801d22dcf9f5bf714f7b8fa3212cdc973ae1c8: `rust/attest/Cargo.toml`, `rust/attest/build.rs`, `rust/attest/src/{lib,dcap,enclave,sgx_session,cds2,svr2,constants,client_connection,cert_chain,util,endian,expireable,error,hsm_enclave,snow_resolver}.rs`, `rust/attest/src/dcap/{evidence,sgx_quote,sgx_report_body,ecdsa,endorsements,revocation_list,sgx_x509,fakes}.rs`, `rust/attest/src/proto/{cds2,svr}.proto` |
| A2 | libsignal attestation test data | same commit, `rust/attest/tests/data/{cdsi,svr2}.{handshakestart,mrenclave,timestamp,advisories}`, `cdsi.pubkey`, `svr2.group_config`, `dcap.{evidence,endorsements,pubkey}`, `cds2_test.{evidence,endorsements,mrenclave,privatekey}`, `tcb_info_v2.json`, `tcb_info_v3.json`, `sgx_x509_extension.der` |
| A3 | libsignal-net | same commit: `rust/net/src/{lib,enclave,svr,auth,env,cdsi,connect_state}.rs`, `rust/net/infra/src/{lib,ws,timeouts}.rs`, `rust/net/infra/src/ws/{attested,connection}.rs`; bridge: `rust/bridge/shared/src/{cds2,svr2,sgx_session}.rs` |
| A4 | signal-cli | https://github.com/AsamK/signal-cli @ v0.14.7 = b01b6b370dc063599a1a2b9fde0f5ff4e2d78fe8: `lib/src/main/java/org/asamk/signal/manager/config/{StagingConfig,LiveConfig,ServiceEnvironmentConfig}.java`, `.../helper/{PinHelper,RecipientHelper}.java`; and `javap -c` of `SignalDependencies` in `/work/third_party/signal-cli-0.14.7/lib/libsignal-cli-0.14.7.jar` |
| A5 | signal-service-java (Turasa fork, a signal-cli dependency) | https://github.com/Turasa/libsignal-service-java @ 66956cb4a0394d50cee3590209baf58312084a44: `lib/libsignal-service/src/main/java/org/whispersystems/signalservice/api/svr/{Svr2Socket,SecureValueRecoveryV2}.kt`, `.../api/cds/CdsiV2Service.java` |
| A6 | Signal-Server (server behaviour only) | https://github.com/signalapp/Signal-Server @ v20261022.0.0 = 8edf3488f7d97398db7ffa5184c6b769a272ff59: `service/src/main/java/org/whispersystems/textsecuregcm/controllers/{SecureValueRecovery2Controller,DirectoryV2Controller}.java` (only the `@Path` annotations) |
| A7 | Reference jar, offline | `/work/third_party/signal-cli-0.14.7/lib/libsignal-client-0.99.1.jar` (native libsignal 0.99.1) driven through its public Java API (`Cds2Client`, `Svr2Client`) with fixed `Instant`s against A2 data; no network |
| A8 | Public standards | Intel SGX DCAP: "Intel SGX ECDSA Quote Library API" (quote v3 layout, section A.4) and Intel PCS API documentation for TCB Info v3 / QE Identity v2 (as cited by A1; the Intel hosts were not contacted); RFC 5280 (certificate path validation and CRLs); FIPS 186-4 / SEC1 for ECDSA P-256 |

All captured attestation data is reproduced in
`testdata/vectors/enclave_attestation.json` with expected results.

## 1. Summary

- Both services: a TLS WebSocket whose path contains the hex MRENCLAVE. The
  client sends no subprotocol. HTTP Basic auth uses a username and password
  that the chat service issues.
- The server speaks first. Its first binary frame is a protobuf
  `ClientHandshakeStart` that carries the SGX DCAP **evidence** (a quote v3
  followed by Open Enclave custom claims) and the **endorsements** (the
  Open Enclave collateral blob).
- The client checks the evidence against Intel's root key, which libsignal
  pins. If the checks pass, it reads the claim `pk`, the enclave's X25519
  static key, and starts a Noise NKhfs handshake toward that key
  (`noise.md`).
- No step needs non-standard-library cryptography. A scratch program using
  only the Go standard library reproduced every signature, hash and chain
  check on the captured staging data (§9). ChaCha20-Poly1305, needed later
  for Noise, is the one primitive Go does not export; see `noise.md`.

## 2. Transport

### 2.1 Endpoints (staging)

| Service | Client stack in signal-cli 0.14.7 | URL | MRENCLAVE used |
|---------|----------------------------------|-----|----------------|
| CDSI | libsignal-net (`Network.cdsiLookup`, environment STAGING) | `wss://cdsi.staging.signal.org/v1/<mrenclave-hex>/discovery` | `6d9b9649fa3a337754a98059c66d48ac77aaca5299d3b27d6ed1e646c7c81c0a` (libsignal's built-in staging CDSI enclave) |
| SVR2 | OkHttp WebSocket in `Svr2Socket` (A5); libsignal is used only for attestation and Noise (`Svr2Client`) | `wss://svr2.staging.signal.org/v1/<mrenclave-hex>` | signal-cli tries `3c699f4975aaa3d172c0aad042f94f031b2b03e10b9c19a45116a01693d83302` (current) and `97f151f6ed078edbbfd72fa9cae694dcc08353f1f5e8d9ccd79a971b10ffc535` (legacy), each as a separate connection (A4 `StagingConfig.SVR2_MRENCLAVE[_LEGACY]`) |

- `<mrenclave-hex>` is the 32-byte MRENCLAVE as 64 **lowercase** hex
  characters. libsignal uses `hex::encode`. signal-cli passes its configured
  string through unchanged, and those strings are lowercase.
- signal-cli's config also holds a `CDSI_MRENCLAVE` (`0f6fd79c…4a57`), but
  nothing reads it (A4 `javap`: `SignalDependencies` reads only
  `svr2Mrenclaves`). CDSI always uses libsignal's constant.
- Production hosts, for reference: `cdsi.signal.org`, `svr2.signal.org`.
- libsignal-net connection settings for both hosts: TLS 1.3 minimum,
  HTTP/1.1, Signal's pinned root certificates (see
  `spec/service/transport.md`), no confirmation header. The OkHttp path for
  SVR2 uses signal-cli's TrustStore for `svr2.staging.signal.org`.
- Both stacks also support domain-fronting proxies. signal-cli does not
  use them.

### 2.2 Authentication and headers

- Header `Authorization: Basic base64(username ":" password)`.
- The username/password pair comes from the chat service:
  - CDSI: `GET /v2/directory/auth`.
  - SVR2: `GET /v2/backup/auth`. The server also accepts the alias
    `/v2/svr/auth` (A6 `@Path("/v2/{name: backup|svr}")`). During
    registration lock, the 423 body's `svr2Credentials` field supplies the
    pair instead.
  - Formats are specified in `spec/service/cdsi.md` and
    `spec/service/svr.md`. A3 `auth.rs` documents the shape: username is
    hex, and password is `"<unix-seconds>:<hex>"`.
- No `Sec-WebSocket-Protocol` header is sent, and none is expected.
- libsignal-net adds its usual User-Agent header to the upgrade request.
  OkHttp adds its own. Neither is part of the enclave protocol (Gap G1).

### 2.3 Frame usage and sequence

Every application frame is **binary**. In libsignal-net, a text frame from
the server is a protocol error ("received a text websocket frame").

| # | Direction | Content |
|---|-----------|---------|
| 1 | server → client | `ClientHandshakeStart` protobuf (§3), sent unprompted right after the upgrade |
| 2 | client → server | Noise handshake message 1 (1632 bytes, `noise.md` §3) |
| 3 | server → client | Noise handshake message 2 (1632 bytes) |
| 4… | either | one frame = one or more concatenated Noise transport messages carrying one application message (`noise.md` §5) |

A Close in place of frame 1 or frame 3 is an "unexpected close" error.

### 2.4 Keep-alive and timeouts (libsignal-net, CDSI)

`RECOMMENDED_WS_CONFIG` (A3 `infra/src/lib.rs`, `timeouts.rs`):

- The client sends a WebSocket Ping after 31 s with nothing sent, or after
  31 s with nothing received.
- It disconnects after 45 s with nothing received.
- At most one WebSocket handshake is in flight per enclave connection
  attempt.
- The single-route connection timeout is 60 s.

OkHttp's defaults apply to SVR2 in signal-cli.

### 2.5 Close codes

**CDSI.** The server closes with a code after the final response or on
error (A3 `cdsi.rs`):

| Code | Meaning | Client handling |
|------|---------|-----------------|
| 1000 (Normal) | Lookup finished | Normal end of stream |
| 4003 | Invalid argument | Error "invalid argument". The close reason text is the server's explanation |
| 4008 | Rate limit exceeded | The close reason is JSON `{"retry_after": <u32 seconds>}`. If it does not parse, the close is treated as unexpected |
| 4013 | Server internal error | Server error |
| 4014 | Server unavailable | Server error |
| 4101 | Invalid token | Error "invalid token". signal-cli then clears its stored CDSI token and number store (A4 `RecipientHelper`) |
| other, or no close frame | — | Unexpected close |

An HTTP 429 on the upgrade carries a `Retry-After` header, which becomes a
rate-limit error.

**SVR2 (signal-cli's OkHttp path, A5).**
- After reading the single response, the client closes with 1000 "OK".
- After an attestation-data error, it closes with 1007 "OK".
- After any other client-side failure, it closes with 1000 "OK".
- If the server closes with 1000 before a response arrives, the request
  fails with an I/O error.
- If the server closes with any other code, the client replies with a 1000
  close and reports `NonSuccessfulResponseCodeException(code)`.
- If the upgrade fails and the failure message contains "404" or "429",
  the client reports those status codes. Anything else is an I/O error.

## 3. The server's first frame: `ClientHandshakeStart`

Proto3, standard protobuf encoding. Unknown fields are ignored. Two
definitions exist, and they are wire-compatible for the fields the client
reads.

CDSI (package `org.signal.cds2`):

| Field | No. | Type | Meaning |
|-------|-----|------|---------|
| pubkey | 1 | bytes | Enclave public key. **The client ignores it** and uses the attested `pk` claim instead. In the captured data it equals the claim |
| evidence | 2 | bytes | Evidence (§4) |
| endorsement | 3 | bytes | Endorsements (§5) |

SVR2 (package `org.signal.svr`):

| Field | No. | Type | Meaning |
|-------|-----|------|---------|
| (reserved) | 1 | — | Test-only public key, never used by clients. The captured staging message still carries 32 bytes here; ignore it |
| evidence | 2 | bytes | Evidence |
| endorsement | 3 | bytes | Endorsements |

Related SVR messages, used for the attested Raft configuration (§7.3):

`RaftGroupConfig`

| Field | No. | Type |
|-------|-----|------|
| group_id | 1 | fixed64 |
| min_voting_replicas | 2 | uint32 |
| max_voting_replicas | 3 | uint32 |
| super_majority | 4 | uint32 |
| db_version | 5 | enum `DatabaseVersion`: 0 UNKNOWN, 2 SVR2, 3 SVR3, 4 SVR4 |
| attestation_timeout | 6 | uint32 (seconds) |
| simulated | 7 | bool |

`AttestationData` (`public_key` = 1, bytes; `group_config` = 2,
`RaftGroupConfig`) exists in the proto file. The SGX path does not use it.

Rejected before any parsing (data errors):
- evidence of length 0 ("Evidence does not fit expected format");
- endorsements of length 0;
- an MRENCLAVE argument whose length is not 32;
- a protobuf that fails to decode.

## 4. Evidence layout

Evidence is the concatenation, with nothing after it:

**quote body (432 bytes) ‖ signature-data length (u32 LE) ‖ signature data ‖ custom claims**

All integers are little-endian. Offsets are from the start of the
evidence.

### 4.1 Quote header (offsets 0–47)

| Off | Len | Field | Check |
|-----|-----|-------|-------|
| 0 | 2 | version | Must be **3**. Quote v4 and v5 are rejected ("unsupported quote version"). This is checked before reading further |
| 2 | 2 | attestation key type | Must be **2** (ECDSA-256 with P-256) |
| 4 | 4 | reserved | Covered by the signature |
| 8 | 2 | QE SVN | Not checked |
| 10 | 2 | PCE SVN | Not checked (the PCE SVN comes from the PCK certificate) |
| 12 | 16 | QE vendor ID | Must equal Intel's UUID `939a7233-f79c-4ca9-940a-0db3957f0607`, i.e. bytes `939a7233f79c4ca9940a0db3957f0607` in this order |
| 28 | 20 | user data | Not checked |

If the buffer is shorter than 432 bytes, the error is "incorrect buffer
size".

### 4.2 ISV enclave report body (offsets 48–431; field offsets below are relative to 48)

The same 384-byte report-body layout is reused for the QE report in §4.3.

| Rel off | Len | Field | Use |
|---------|-----|-------|-----|
| 0 | 16 | CPUSVN | — |
| 16 | 4 | MISCSELECT (u32 LE) | QE check only |
| 20 | 12 | reserved | — |
| 32 | 16 | ISV extended product ID | — |
| 48 | 16 | ATTRIBUTES | The first 8 bytes are the flags as a u64 LE. Flag bit 0x02 = DEBUG |
| 64 | 32 | MRENCLAVE | Compared with the expected value (§7.2) |
| 96 | 32 | reserved | — |
| 128 | 32 | MRSIGNER | **Not checked for the ISV enclave.** QE check only |
| 160 | 32 | reserved | — |
| 192 | 64 | CONFIGID | — |
| 256 | 2 | ISVPRODID (u16 LE) | QE check only |
| 258 | 2 | ISVSVN (u16 LE) | QE check only |
| 260 | 2 | CONFIGSVN | — |
| 262 | 42 | reserved | — |
| 304 | 16 | ISV family ID | — |
| 320 | 64 | REPORT DATA | For the ISV report, §6 step 8 binds it to the custom claims |

ISV enclave MRENCLAVE is at absolute offset 112, and ISV REPORT DATA at
absolute 368.

### 4.3 Signature data (starting at absolute offset 432)

| Off (abs.) | Len | Field |
|------------|-----|-------|
| 432 | 4 | signature data length L (u32 LE) |
| 436 | 64 | ISV signature: ECDSA P-256, raw r‖s, each 32-byte big-endian |
| 500 | 64 | attestation public key: raw x‖y, each 32-byte big-endian (uncompressed point without the 0x04 prefix) |
| 564 | 384 | QE report body (layout §4.2) |
| 948 | 64 | QE report signature, raw r‖s |
| 1012 | 2 | QE authentication data size A (u16 LE) |
| 1014 | A | QE authentication data |
| 1014+A | 2 | certification data type (u16 LE). Must be **5** (PCK certificate chain) |
| 1016+A | 4 | certification data size C (u32 LE) |
| 1020+A | C | certification data: a PEM chain of the PCK leaf, the PCK Platform CA and the Intel SGX Root CA, in that order. In the captured data it ends with a NUL byte; PEM parsing ignores it |

Notes on the length checks:
- libsignal requires only that at least L bytes remain after the length
  field. It does **not** require the structure above to end exactly at
  436+L. Parsing simply continues, and the claims start wherever the
  certification data ends.
- In all captured data, 436+L equals the end of the certification data
  (cdsi: L = 4298, end = 4734; svr2: L = 4294, end = 4730).
- An implementation may additionally require equality. It is not known
  whether the server ever sends anything else (Gap G2).
- A must fit in the remaining bytes, and so must C.
- Errors: "buffer underflow"; "unsupported certification key type".

Fixed-size part: the four fields from 436 to 1013 are the 578-byte
"ECDSA signature header", and A is its last field.

The PCK leaf must carry the SGX extension (§5.4). Otherwise the error is
"PCK certificate is missing SGX extension".

### 4.4 Custom claims (Open Enclave `oe_custom_claims`, rest of the evidence)

| Field | Encoding |
|-------|----------|
| version | u64 LE, must be **1** |
| count | u64 LE, at most **256** |
| entries × count | each: name_size u64 LE (≤ 1024), value_size u64 LE (≤ 1 048 576), name bytes, value bytes |

- If both sizes exceed the remaining bytes, the error is "underflow".
- One trailing NUL is removed from the name if present. The name must
  then be valid UTF-8.
- If a name repeats, the later value wins.
- Any bytes left after the last entry are an error.
- The claims "blob" is the byte range from the version field to the end
  of the evidence. Its SHA-256 is bound into the report (§6 step 8).

Claim names in use:
- `pk`: the 32-byte X25519 enclave static key. Required.
- `config`: a serialized `RaftGroupConfig`. Present for SVR2.
- `minimum_limits`: empty in the captured SVR2 data and ignored.

Captured examples:
- CDSI: 1 claim, `pk\0` → `81815830…135d`.
- SVR2: 3 claims, `pk\0`, `config\0`, `minimum_limits\0`.

## 5. Endorsements layout (Open Enclave `oe_endorsements_t`, SGX)

### 5.1 Container

| Off | Len | Field | Check |
|-----|-----|-------|-------|
| 0 | 4 | version (u32 LE) | Must be 1 |
| 4 | 4 | enclave type (u32 LE) | Must be **2** (SGX) |
| 8 | 4 | buffer size (u32 LE) | Not checked (captured: total length − 16) |
| 12 | 4 | number of elements N (u32 LE) | ≥ 9 required |
| 16 | 4·N | offsets, u32 LE each | Relative to the **data area**, which starts at 16+4·N |
| 16+4·N | … | data area | |

Offset rules:
- The offsets must be strictly increasing.
- The last offset must be less than the length of the data area.
- Field *i* spans from offset[i] to offset[i+1]. The last field (index
  N−1) runs to the end of the data. Only fields 0–8 are used.

Captured CDSI offsets: `[0, 4, 9351, 11244, 14601, 14896, 16801, 18182, 20075]`,
with the data area at byte 52.

| Index | Field | Format |
|-------|-------|--------|
| 0 | version | Exactly 4 bytes, u32 LE, must be 1 |
| 1 | TCB info | JSON, one trailing NUL stripped (§5.2) |
| 2 | TCB info issuer chain | PEM: Intel SGX TCB Signing, then Intel SGX Root CA |
| 3 | PCK CRL (issued by Intel SGX PCK Platform CA) | DER X.509 CRL **followed by one NUL byte**; see note |
| 4 | Root CA CRL (issued by Intel SGX Root CA; Open Enclave calls it "CRL PCK proc CA") | DER CRL + NUL |
| 5 | PCK CRL issuer chain | PEM: PCK Platform CA, Root CA |
| 6 | QE identity | JSON, trailing NUL stripped (§5.3) |
| 7 | QE identity issuer chain | PEM: TCB Signing, Root CA |
| 8 | creation datetime | ISO 8601 text plus NUL, e.g. `2026-06-05T19:52:04Z`. Untrusted; must be UTF-8; otherwise unused |

- **CRL note.** libsignal passes the field, NUL included, to BoringSSL's
  DER parser, which accepts the trailing byte. Strip one trailing 0x00
  before parsing. Go's `x509.ParseRevocationList` happened to accept the
  captured fields either way.
- Each CRL must contain the Authority Key Identifier extension and the
  CRL Number extension. Otherwise the error is "CRL missing required
  extension".
- **PEM chains.** Every PEM certificate in the field is parsed, and an
  empty chain is an error. The chain is then put in leaf→root order:
  - The root is the last self-issued certificate in the list. "Self-issued"
    means it verifies with its own key and its issuer name equals its
    subject.
  - Working down from the root, each certificate must have issued exactly
    the next one.
  - Any missing or extra link is an error ("Invalid certificate chain").
- "Leaf" means the first certificate after sorting, and "root" the last.

### 5.2 TCB info JSON (Intel PCS TCB Info v3; v2 is also accepted)

Shape: `{"tcbInfo": <object>, "signature": "<128 hex chars>"}`.

- **Signature.** ECDSA P-256 over SHA-256 of the **exact bytes** of the
  `tcbInfo` value as transmitted, from `{` to the matching `}`, without
  re-serialization. The signature is raw r‖s, hex. The key is the TCB
  issuer chain leaf. The signature is checked while the endorsements are
  parsed, before the inner object is interpreted.
- **Hex.** Both upper- and lower-case hex are accepted.

Inner object fields (unknown fields such as `"id":"SGX"` are ignored; all
fields listed are required unless marked otherwise):

| Field | Type | Check |
|-------|------|-------|
| version | int | 2 or 3, otherwise a parse error |
| issueDate | RFC 3339 UTC | Parsed, ignored |
| nextUpdate | RFC 3339 UTC | Verification time must be ≤ nextUpdate |
| fmspc | hex, 6 bytes | Must equal the PCK extension FMSPC |
| pceId | hex, 2 bytes | Must equal the PCK extension PCE-ID |
| tcbType | int | Must be 0 |
| tcbEvaluationDataNumber | u16 | Must be **≥ 21** (`SGX_TCB_EVALUATION_DATA_NUMBER_MIN`). A lower number makes the collateral "not valid" (the expiry error) |
| tcbLevels | array | Ordered as Intel publishes them (newest first). libsignal does not re-sort |

Each TCB level:

| Field | Type | Notes |
|-------|------|-------|
| tcb | object | v3: `{"sgxtcbcomponents": [16 objects, each with "svn": u8; other keys ignored], "pcesvn": u16}`. v2: `{"sgxtcbcomp01svn"…"sgxtcbcomp16svn": u8, "pcesvn": u16}`. Every level must use the format that matches `version`, otherwise "mismatched tcb info versions" |
| tcbDate | RFC 3339 | Parsed, ignored |
| tcbStatus | string | One of `UpToDate`, `OutOfDate`, `ConfigurationNeeded`, `SWHardeningNeeded`, `ConfigurationAndSWHardeningNeeded`, `OutOfDateConfigurationNeeded`, `Revoked`. Any other string is a parse error |
| advisoryIDs | array of strings | Optional (default empty) |

### 5.3 QE identity JSON (Intel PCS Enclave Identity v2)

Shape: `{"enclaveIdentity": <object>, "signature": "<hex r‖s>"}`. The
signature covers the exact raw bytes of the `enclaveIdentity` value. The
key is the QE identity issuer chain leaf.

| Field | Type | Check |
|-------|------|-------|
| id | "QE" or "QVE" | Must be "QE" (checked in §6 step 4) |
| version | u16 | Must be 2 |
| issueDate | RFC 3339 | Ignored |
| nextUpdate | RFC 3339 | Verification time must be ≤ nextUpdate |
| tcbEvaluationDataNumber | u16 | Required, **not** checked |
| miscselect, miscselectMask | hex, 4 bytes each | See the byte-order note below |
| attributes, attributesMask | hex, 16 bytes | — |
| mrsigner | hex, 32 bytes | — |
| isvprodid | u16 | — |
| tcbLevels | array of `{"tcb": {"isvsvn": u16}, "tcbDate": RFC 3339, "tcbStatus": "UpToDate"\|"OutOfDate"\|"Revoked"}` | `advisoryIDs` is ignored. Any other status string is a parse error |

**Byte order of miscselect.** libsignal decodes the 4 hex bytes and reads
them as a **little-endian** u32. The report's MISCSELECT is also read
little-endian. Intel publishes the hex string big-endian, so for
non-symmetric values this mismatches Intel's intent. Reproduce libsignal's
behaviour. Captured data has `"00000000"` / `"FFFFFFFF"`, where the order
does not matter (Gap G3).

### 5.4 PCK certificate SGX extension

- Extension OID `1.2.840.113741.1.13.1`, non-critical, on the PCK leaf.
- The extnValue is a DER `SEQUENCE OF SEQUENCE { OBJECT IDENTIFIER, value }`.
- Allowed value types: OCTET STRING, SEQUENCE (nested list of the same
  shape), INTEGER (must fit u64, then the target width), ENUMERATED,
  BOOLEAN.
- **Exactly** the following top-level OIDs (prefix `1.2.840.113741.1.13.1`)
  must appear, each once:
  - an unknown OID is an error ("unexpected extension");
  - a duplicate is an error;
  - a missing one is an error.

| OID suffix | Name | Type |
|------------|------|------|
| .1 | PPID | OCTET STRING, 16 bytes |
| .2 | TCB | SEQUENCE of exactly: .2.1–.2.16 COMPnnSVN (INTEGER 0–255), .2.17 PCESVN (INTEGER 0–65535), .2.18 CPUSVN (OCTET STRING 16) |
| .3 | PCE-ID | OCTET STRING, 2 bytes |
| .4 | FMSPC | OCTET STRING, 6 bytes |
| .5 | SGX type | ENUMERATED 0 (Standard) or 1 (Scalable). Anything else is an error |
| .6 | Platform instance ID | OCTET STRING, 16 bytes |
| .7 | Configuration | SEQUENCE of exactly .7.1 dynamic platform, .7.2 cached keys, .7.3 SMT enabled (BOOLEAN each) |

- Because .6 and .7 are mandatory, only PCK certificates from the
  **Platform CA** (multi-package platforms) parse.
- Processor-CA PCK certificates lack .6 and .7 and would be rejected.
  Staging and production enclaves use Platform CA certificates.
- Only TCB, PCE-ID and FMSPC feed decisions. The rest is parsed and
  ignored.

Captured CDSI PCK extension:
- TCB COMPSVN = `[16,16,3,3,255,255,1,0,0,0,0,0,0,0,0,0]`, PCESVN 13,
  CPUSVN `10100303ffff01000000000000000000`;
- PCE-ID `0000`, FMSPC `00606a000000`;
- SGX type 1;
- configuration (true, false, false).

## 6. Verification procedure (order matters for which error is reported)

Inputs: evidence, endorsements, the expected MRENCLAVE (32 bytes), the
acceptable advisory list for that MRENCLAVE (§7.1), and the current time.

**Skew.** Every time-based check uses **T = now + 86 400 s** (24 hours,
"to adjust for clock skew"). The window therefore moves one day earlier:
- data that becomes valid at instant X is accepted from X − 1 day;
- data that expires at Y stops being accepted at Y − 1 day.

The order below matters only for which error is reported.

1. **Parse** the evidence (§4), then the endorsements (§5). Parsing
   includes verifying the TCB info signature and the QE identity
   signature, each with its own issuer chain's leaf key. Both are checked
   here, before any certificate-chain check, so a bad signature fails
   early.
2. **Validity windows at T.** All must hold:
   - every certificate in the PCK chain, the TCB issuer chain, the PCK CRL
     issuer chain and the QE identity issuer chain has
     notBefore ≤ T ≤ notAfter (inclusive);
   - TCB info: T ≤ nextUpdate, and tcbEvaluationDataNumber ≥ 21;
   - QE identity: T ≤ nextUpdate;
   - both CRLs: T < nextUpdate (strict).

   Failure is reported as "attestation is not valid for <T in unix
   seconds>", prefixed "(evidence)" or "(endorsements)".
3. **Certificates and revocation** (RFC 5280 path validation at time T,
   with CRL checking for every certificate in each path):
   1. Take the root = the root of the TCB issuer chain. It must be
      self-issued, and its signature must verify with the **pinned Intel
      key** (§8). Note what is not checked: the certificate bytes are not
      compared with anything, and the root carried in the other three
      chains is not compared. Those roots must still chain to this trust
      anchor.
   2. The root CRL's signature must verify with the pinned key.
   3. Trust set = {root} with CRLs {root CRL}. Validate the PCK CRL issuer
      chain (Platform CA → root) against this set, passing the PCK CRL as
      an extra CRL. This proves the Platform CA is not revoked by the root
      CRL and that the PCK CRL is properly issued.
   4. Trust set = {root} with CRLs {root CRL, PCK CRL}. Validate, in this
      order:
      - the TCB issuer chain;
      - the PCK chain (leaf → Platform CA → root);
      - the QE identity issuer chain.
   - **CRL coverage.** Each non-anchor certificate needs a valid CRL from
     its issuer, with thisUpdate ≤ T < nextUpdate:
     - the PCK leaf needs the PCK CRL;
     - the Platform CA and TCB Signing certificates need the root CRL.
   - A certificate whose serial is listed in its issuer's CRL fails.
   - BoringSSL's `X509_STRICT` profile applies.
   - libsignal checks no chain lengths and no subject names. The only
     anchor is the pinned key.
4. **Quoting-enclave identity**, comparing the QE report body (§4.3)
   with the QE identity:
   1. The quote header QE vendor ID equals Intel's UUID.
   2. QE MRSIGNER equals identity `mrsigner`.
   3. QE ISVPRODID equals identity `isvprodid`.
   4. (QE MISCSELECT AND `miscselectMask`) = `miscselect`, using the u32
      values described in §5.3.
   5. For each of the 16 ATTRIBUTES bytes, (QE byte AND mask byte) =
      identity byte.
   6. Identity `id` is "QE".
   7. QE TCB status: find the **first** level in `tcbLevels` (listed order)
      whose `isvsvn` ≤ QE ISVSVN. Its status must be `UpToDate`. If no
      level matches, the status counts as Revoked. Anything other than
      UpToDate is rejected ("Enclave version tcb not up to date").
5. **Signatures.** Each ECDSA check is P-256 over SHA-256 of the bytes
   named:
   1. The QE report signature verifies over the 384 bytes of the QE report
      body, with the PCK leaf certificate's public key.
   2. QE REPORT DATA[0..32] = SHA-256(attestation public key (64 bytes,
      x‖y) ‖ QE authentication data), and QE REPORT DATA[32..64] is all
      zero.
   3. The ISV signature verifies over the first **432** bytes of the
      evidence (header plus ISV report body), with the attestation public
      key taken as the affine point (x, y).
6. **Platform TCB level:**
   - PCK FMSPC = TCB info `fmspc`, and PCK PCE-ID = `pceId`.
   - Walk `tcbLevels` in listed order. Take the **first** level where all
     16 PCK COMPSVN values are ≥ the level's components (compared
     position by position), and PCK PCESVN ≥ the level's `pcesvn`. CPUSVN
     is not used.
   - If no level matches: error "Unsupported TCB in pck extension".
   - Status `UpToDate`: standing "up to date".
   - Status `SWHardeningNeeded`: standing "SW hardening needed", carrying
     that level's `advisoryIDs`.
   - Any other status, including `ConfigurationAndSWHardeningNeeded`,
     `ConfigurationNeeded`, `OutOfDate…` and `Revoked`: error "invalid tcb
     status".
7. (No further collateral checks. libsignal does not implement the
   "TCB info id = SGX", tcbDate or QE-advisory steps.)
8. **Claims binding:**
   - ISV REPORT DATA[32..64] must be all zero ("report data hash had
     unexpected data").
   - ISV REPORT DATA[0..32] must equal SHA-256 of the custom-claims blob
     (§4.4).
   - That hash must not be all zero.
9. **Debug.** If ATTRIBUTES flag bit 0x02 (DEBUG) is set in the ISV
   report, the error is "Application enclave in debug mode".
10. **Advisories.** If the standing is "SW hardening needed", **every**
    advisory ID on the matched level must be in the acceptable list for
    the expected MRENCLAVE. Otherwise the error is "TCB contains
    unmitigated unaccepted advisory ids: [...]".
11. **MRENCLAVE.** The ISV MRENCLAVE must equal the expected value, else
    "expected mrenclave …, was …".

    Steps 10 and 11 are reversed relative to intuition: with an
    *unknown* expected MRENCLAVE the advisory list is empty, so a
    hardening-needed platform reports the advisory error, not the
    MRENCLAVE error. A7 observed exactly this.
12. Success returns the claims map.

**Error classes** as surfaced through the Java API (A7), which an
implementation may mirror:
- *attestation data* errors: malformed protobuf, empty evidence or
  endorsements, wrong MRENCLAVE length, missing `pk`, bad or missing raft
  config, unknown SVR2 MRENCLAVE;
- *attestation failed*: everything in steps 1–11.

## 7. Enclave measurements, policy and claims use

### 7.1 Known enclaves in libsignal 0.99.1 (A1 `constants.rs`)

Every listed enclave accepts the SW advisories `INTEL-SA-00615` and
`INTEL-SA-00657`. Any MRENCLAVE not in the table gets an empty advisory
list.

| Name | MRENCLAVE |
|------|-----------|
| CDSI staging | `6d9b9649fa3a337754a98059c66d48ac77aaca5299d3b27d6ed1e646c7c81c0a` |
| CDSI prod | `15637fa1e54fe655176d3df1a9f94b87c01ed377acaa570682dc5d72c95ef07b` |
| SVR2 2026Q2 staging (current) | `3c699f4975aaa3d172c0aad042f94f031b2b03e10b9c19a45116a01693d83302` |
| SVR2 2026Q1 staging (legacy in signal-cli) | `97f151f6ed078edbbfd72fa9cae694dcc08353f1f5e8d9ccd79a971b10ffc535` |
| SVR2 2026Q2 prod | `ced8217b26228e4b210c985786999d095c4958a94faf37b14acaf25c4cbb02a4` |
| SVR2 2026Q1 prod | `1240acbd4aa26974184844c8a46b1022d3957ac8a76c1fd8f5b1a15141ee0708` |
| SVR2 2025Q3 prod | `29cd63c87bea751e3bfd0fbd401279192e2e5c99948b4ee9437eafc4968355fb` |
| SVR-B 2026Q2 / 2026Q1 staging | same values as SVR2 2026Q2 / 2026Q1 staging |
| SVR-B 2026Q2 / 2026Q1 prod | `2048e20fcd07d0992c4907e8e04c5a85f1f993d195004c7342675343ca2e524b` / `bee62050df1072e3d9fdf7660bfaf4e4b71f5622db9de8b30fc5f4b9852d8359` |

- libsignal-net's STAGING environment uses CDSI staging, and SVR2 2026Q2
  staging as "current" with no "previous".
- signal-cli's own SVR2 list adds 2026Q1 staging as legacy.
- **Expiry dates.** libsignal 0.99.1 encodes none for old enclaves. An
  enclave stays acceptable until a new libsignal drops it. The 604 800 s
  `attestation_timeout` is a Raft parameter checked for equality, not a
  client-side expiry.

### 7.2 Expected Raft configurations for SVR2 (equality on all seven fields)

| MRENCLAVE | group_id | min | max | super_majority | db_version | attestation_timeout | simulated |
|-----------|----------|-----|-----|----------------|------------|---------------------|-----------|
| 3c699f… (2026Q2 staging) | 5138641357881452604 | 3 | 9 | 0 | 2 | 604800 | false |
| 97f151… (2026Q1 staging) | 2330628069874851020 | 3 | 9 | 0 | 2 | 604800 | false |
| ced821… (2026Q2 prod) | 11311619198250676136 | 4 | 13 | 2 | 2 | 604800 | false |
| 1240ac… (2026Q1 prod) | 2076725645304009823 | 4 | 13 | 2 | 2 | 604800 | false |
| 29cd63… (2025Q3 prod) | 10263621230883829694 | 4 | 13 | 2 | 2 | 604800 | false |

The SVR-B configurations (db_version 4) exist in the source but are not
used by signal-cli.

The captured staging `config` claim is
`093ca8a7dc161f50471003180928023080f524`, which decodes to the first row.

### 7.3 Per-service rules after attestation

- **CDSI.** Take claim `pk`; a missing `pk` is a data error. If a `config`
  claim is present it is decoded, and a decode failure is a data error,
  but it is not compared. Other claims are ignored. The handshake type is
  post-quantum.
- **SVR2** (`Svr2Client`, which is what signal-cli uses):
  - The expected Raft config is looked up by MRENCLAVE in §7.2 **before
    anything else**. An unknown MRENCLAVE is a data error ("unknown
    mrenclave").
  - After attestation, the `config` claim must be present ("Claims must
    contain a raft group config") and must equal the expected config on
    all seven fields.
  - The handshake type is post-quantum.
- The `pk` value becomes the Noise responder static key (`noise.md` §2).
  libsignal does not check its length up front. A non-32-byte value fails
  inside the Noise layer (Gap G4).

## 8. Intel SGX Root CA pinning

- libsignal pins the root **public key only**, a 65-byte SEC1 uncompressed
  P-256 point:
  `040ba9c4c0c0c86193a3fe23d6b02cda10a8bbd4e88e48b4458561a36e705525f567918e2edc88e40d860bd0cc4ee26aacc988e505a953558c453f6b0904ae7394`
- The certificate travels inside the endorsements. In the captured data
  it is:
  - subject = issuer = `C=US, ST=CA, L=Santa Clara, O=Intel Corporation, CN=Intel SGX Root CA`;
  - serial `22650cd65a9d3489f383b49552bf501b392706ac`;
  - validity 2018-05-21T10:45:10Z – 2049-12-31T23:59:59Z;
  - DER SHA-256 `44a0196b2b99f889b8e149e95b807a350e7424964399e885a7cbb8ccfab674d3`.
- The DER is in the vectors file (`intel_sgx_root.certificate_der_base64`),
  copied from the captured endorsements. libsignal's sources do not
  contain it.
- An implementation may pin either the key, as libsignal does, or this
  exact certificate. They are equivalent for the current root.

Other Intel certificates seen in the captured chains:

| Certificate | Validity | Note |
|-------------|----------|------|
| Intel SGX PCK Platform CA | 2018-05-21 – 2033-05-21 | serial `956f5dcd…0bde54` |
| Intel SGX TCB Signing | 2025-05-06 – 2032-05-06 | serial `7e3882d5…bdf455`; the 2023 data has the older instance with the same serial and key, valid 2018-05-21 – 2025-05-21 |
| PCK leaf (CDSI capture) | 2025-07-29 – 2032-07-29 | — |

## 9. Go standard-library feasibility

All of these are in the standard library:
- `crypto/ecdsa` with `elliptic.P256` (raw r‖s converted to big integers);
- `crypto/sha256`;
- `crypto/x509` (PEM certificates, `ParseRevocationList`,
  `CheckSignatureFrom`, `Verify` with a custom root pool and
  `ExtKeyUsageAny`);
- `encoding/asn1` (the SGX extension; ENUMERATED is supported);
- `encoding/json` with `json.RawMessage`, which keeps the exact signed
  bytes of `tcbInfo` / `enclaveIdentity`;
- `encoding/binary`.

A scratch Go program run on the CDSI and SVR2 captures (not committed)
confirmed all of the following:
- the ISV signature;
- the QE report signature;
- the QE report-data hash;
- the claims hash;
- both JSON signatures;
- the root key equals the pinned key;
- both CRL signatures;
- `x509.Verify` of the PCK and TCB chains at the capture time plus one
  day.

Differences an implementation must cover by hand:
1. `x509.Verify` does no CRL checking. Check each certificate's serial
   against its issuer's CRL, check the CRL validity window, and check
   that the CRL is signed by the issuer.
2. Go's path validation is not BoringSSL's `X509_STRICT`, so edge-case
   acceptance may differ (Gap G5).
3. RFC 3339 timestamps parse with `time.RFC3339`.

## 10. Test data (see `testdata/vectors/enclave_attestation.json`)

**`cdsi_staging_2026-06-05`**
- Real staging handshake start, MRENCLAVE `6d9b…1c0a`, captured at unix
  1780689322 (2026-06-05T19:55:22Z).
- Advisories {00615, 00657}.
- Expected `pk` = `81815830c9bbe0a32c97343ed165693acb6cd83873be4e201c8ebbad3dea135d`.
- Matched platform level: level 0, SWHardeningNeeded, advisory list
  [INTEL-SA-00615].
- The QE (ISVSVN 11) matches the QE identity level isvsvn 8, UpToDate.
- With the reference jar (skew included), it is accepted for given times
  2026-06-04T19:19:17Z … 2026-07-04T19:19:06Z:
  - lower bound: PCK CRL thisUpdate minus one day;
  - upper bound: QE identity nextUpdate minus one day, inclusive.
- Rejected 2 years later, and today (2026-10-25), as "not valid".

**`svr2_staging_2026-06-05`**
- MRENCLAVE `3c69…3302`, same capture time.
- Expected raft config is the §7.2 first row.
- Accepted 2026-06-04T18:19:11Z … 2026-07-04T18:19:03Z.

**`dcap_2023_expired`, `cds2_test_2022_noise_key`**
- Older captures. They parse but fail production policy (evaluation data
  number 14 / 12, both < 21) and are long expired.
- `cds2_test` includes the enclave's X25519 private key. Upstream uses it
  to drive a Noise responder in test builds.
- `dcap` metrics:
  - `tcb_info_expiration_ts` 1676670969;
  - `tcb_signer_not_before_ts` 1526899810;
  - `tcb_signer_not_after_ts` 1747824610.

Plus the TCB info v2/v3 JSON samples and a PCK extension DER, with
expected parsed values.

To test the time logic without live data, run the verifier with an
injected clock set to the capture time.

## Gaps

- **G1.** The exact User-Agent / extra headers on the CDSI and SVR2
  upgrade requests were not traced. They are believed irrelevant to the
  server.
- **G2.** libsignal does not require the signature-data length to match
  the structure exactly (§4.3). Behaviour with a server that pads it is
  unknown. Captured data always matches exactly.
- **G3.** The miscselect byte-order quirk (§5.3) only matters for
  non-palindromic values. Intel's current QE identity uses
  `00000000`/`FFFFFFFF`.
- **G4.** A `pk` claim that is not 32 bytes: libsignal reports a Noise
  error (not a data error) when the handshake starts. Treat it as an
  attestation-data error.
- **G5.** BoringSSL `X509_STRICT` and CRL-processing details (e.g. delta
  CRLs, critical CRL extensions, issuing-distribution-point handling)
  were not reproduced rule by rule. The captured chains pass both
  BoringSSL (via the jar) and Go's `x509.Verify`.
- **G6.** Intel's PCS documentation (TCB Info v3 / QE Identity v2 field
  definitions) was not fetched (Intel hosts are off-limits). Field
  meanings above come from libsignal's parser and the captured JSON.
- **G7.** Quote v4 and v5 (TDX-era formats) are unsupported by libsignal
  0.99.1. If staging enclaves ever switch, this spec is insufficient.
