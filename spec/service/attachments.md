# Attachments: pointer, cipher, CDN upload and download, local storage

Spec role: prose and tables only. This file covers how signal-cli 0.14.7
sends and receives attachments: the `AttachmentPointer` protobuf, the
attachment cipher and its padding, the upload form and the CDN upload
protocols (CDN 2 and CDN 3), downloads, where signal-cli keeps the
downloaded files, sticker packs and avatars as far as the CDN is
involved, and how `send -a` arguments are turned into attachments.

Related specs, not repeated here:

- `spec/service/transport.md`: hosts, TLS roots (§3.3, §5), the
  `User-Agent` / `X-Signal-Agent` strings (§4.2), proxies (§6).
- `spec/service/api.md`: WebSocket request framing, status-code
  conventions, rate limits.
- `spec/service/content.md`: `Content` / `DataMessage` and where field 2
  (`attachments`) sits; transport padding of the whole Content (a
  different padding from the one in §2.2 here).
- `spec/service/profile.md`: the profile `avatar` field.
- Being written in parallel by other spec agents, only cross-referenced
  here: data-message features (quotes, previews, shared contacts, stickers
  in a `DataMessage`, edits), sync and linked devices (contact/group sync
  blobs, sticker-pack operations), stories (`StoryMessage` file
  attachments, text attachments) and sender keys.
- CLI shapes of `send`, `receive`, `getAttachment`, `getSticker`,
  `uploadStickerPack`, `addStickerPack`: `spec/surface/cli/` and
  `spec/surface/jsonrpc/`.

## Sources

| Ref | What | URL @ version |
|-----|------|---------------|
| S1 | signal-cli | https://github.com/AsamK/signal-cli @ v0.14.7 (`b01b6b370dc063599a1a2b9fde0f5ff4e2d78fe8`): `lib/src/main/java/org/asamk/signal/manager/helper/{AttachmentHelper,IncomingMessageHandler,StickerHelper,ProfileHelper,GroupHelper}.java`, `.../manager/storage/{AttachmentStore,AvatarStore}.java`, `.../manager/storage/stickerPacks/{StickerPackStore,JsonStickerPack}.java`, `.../manager/util/{AttachmentUtils,MimeUtils,DataURI,Utils,IOUtils,StickerUtils}.java`, `.../manager/api/{StickerPackUrl,MessageEnvelope}.java`, `.../manager/internal/{ManagerImpl,PathConfig,SignalDependencies}.java`, `.../manager/jobs/RetrieveStickerPackJob.java`, `.../manager/config/{ServiceConfig,StagingConfig}.java`, `.../manager/storage/SignalAccount.java`; `src/main/java/org/asamk/signal/commands/{SendCommand,GetAttachmentCommand,GetStickerCommand}.java`, `src/main/java/org/asamk/signal/json/JsonAttachmentData.java` |
| S2 | signal-service-java (Turasa fork) | https://github.com/Turasa/libsignal-service-java @ v2.15.3_unofficial_151 (`66956cb4a0394d50cee3590209baf58312084a44`): `lib/libsignal-service/src/main/java/org/whispersystems/signalservice/api/crypto/{AttachmentCipherOutputStream,AttachmentCipherInputStream,AttachmentCipherStreamUtil,DigestingOutputStream,ProfileCipherInputStream}.*`, `.../internal/crypto/{PaddingInputStream,AttachmentDigest}.*`, `.../internal/push/{PushServiceSocket,AttachmentUploadForm,PushAttachmentData}.*`, `.../internal/push/http/{AttachmentCipherOutputStreamFactory,DigestingRequestBody,ResumableUploadSpec,StickerCipherOutputStreamFactory}.*`, `.../api/{SignalServiceMessageSender,SignalServiceMessageReceiver}.java`, `.../api/util/AttachmentPointerUtil.java`, `.../api/messages/{SignalServiceAttachment,SignalServiceAttachmentRemoteId,SignalServiceStickerManifestUpload,EnvelopeContentValidator}.*`, `lib/libsignal-service/src/main/protowire/{SignalService,StickerResources,ResumableUploads}.proto`; `lib/network/src/main/java/org/signal/network/{api/AttachmentApi,service/CdnService,rest/SignalRestClient,rest/RequestSpec}.kt`; `core/util-jvm/src/main/java/org/signal/core/util/{StringExtensions.kt,stream/LimitedInputStream.kt}`; tests `.../api/crypto/AttachmentCipherStreamUtilTest.kt`, `.../internal/crypto/PaddingInputStreamTest.kt` |
| S3 | libsignal | https://github.com/signalapp/libsignal @ v0.99.1 (`97801d22dcf9f5bf714f7b8fa3212cdc973ae1c8`): `rust/protocol/src/incremental_mac.rs` (with its tests), `rust/bridge/shared/src/incremental_mac.rs`, `rust/net/chat/src/ws/messages.rs` (`get_upload_form` and its test), `rust/net/chat/src/ws.rs` (upload-form JSON names), `rust/net/chat/src/api.rs`, `rust/net/chat/src/api/messages.rs`, `java/shared/java/org/signal/libsignal/protocol/incrementalmac/ChunkSizeChoice.kt`; tests `java/client/src/test/java/org/signal/libsignal/protocol/incrementalmac/IncrementalStreamsTest.java`, `node/ts/test/IncrementalMacTest.ts`, `swift/Tests/LibSignalClientTests/IncrementalMacTests.swift` |
| S4 | Signal-Server (server behaviour only; staging may run a different build) | https://github.com/signalapp/Signal-Server @ v20261022.0.0 (`8edf3488f7d97398db7ffa5184c6b769a272ff59`): `service/src/main/java/org/whispersystems/textsecuregcm/controllers/{AttachmentControllerV4,StickerController}.java`, `.../attachments/{GcsAttachmentGenerator,TusAttachmentGenerator,AttachmentUtil}.java`, `.../entities/StickerPackFormUploadAttributes.java`, `.../configuration/AttachmentsConfiguration.java` |
| S5 | Reference distribution, offline | `/work/third_party/signal-cli-0.14.7/lib/*.jar`: the bundled `SignalService.proto` and `StickerResources.proto` inside `signal-service-java-2.15.3_unofficial_151.jar` (field numbers identical to S2's source); `javap -c` of `AttachmentPointer$Companion$ADAPTER$1` (field emission order); offline `jshell` runs of public classes to compute the vectors (listed in the vectors file's `source` block) and to observe JDK MIME detection |
| S6 | Standards | RFC 2397 (data URLs), RFC 5869 (HKDF), FIPS 197 / NIST SP 800-38A (AES-CBC), RFC 2104 (HMAC), RFC 5652 §6.3 (PKCS#7 padding), tus resumable upload protocol 1.0.0 (tus.io), Google Cloud Storage XML-API resumable uploads (public docs), by reference |

Test-vector data: `testdata/vectors/attachments.json` (provenance in its
`source` block; summary in §9).

## 1. Answer in brief

- **Pointer.** An attachment travels as an `AttachmentPointer` (§3)
  inside the `DataMessage` (and in quotes, previews, stickers, stories).
  signal-cli always sends `cdnKey` (never `cdnId`), `cdnNumber` 2 or 3,
  a fresh random 64-byte `key`, plaintext `size`, `digest`, `flags`
  (0 unless voice note), `uploadTimestamp`, `clientUuid`, and
  `contentType`/`fileName`/`width`/`height` when known. It never sends
  `incrementalMac`, `chunkSize`, `thumbnail`, `caption` or `blurHash`.
- **Cipher.** Plaintext is zero-padded to a size bucket (§2.2), then
  encrypted as `IV(16) || AES-256-CBC-PKCS7 || HMAC-SHA256(32)`, with the
  64-byte key split as AES key ‖ HMAC key. `digest` = SHA-256 of that
  whole ciphertext.
- **Upload.** `GET /v4/attachments/form/upload?uploadLength=<ciphertext
  length>` on the authenticated chat WebSocket returns
  `{cdn, key, headers, signedUploadLocation}`. Then, over HTTPS to the
  configured CDN host (path and query of the signed URL grafted onto
  `cdn2-staging` / `cdn3-staging`): CDN 2 = Google-style resumable upload
  (POST create, PUT status probe, PUT data); CDN 3 = tus 1.0.0 (POST
  create with deferred length, HEAD, PATCH).
- **Download.** `GET https://cdn{,2,3}-staging.signal.org/attachments/<url-encoded
  cdnKey>` into a temp file, then length check, HMAC check, digest check,
  decrypt, truncate to `size`. Stored as
  `<config>/attachments/<sanitized cdnKey><.ext>`.
- signal-cli 0.14.7 does not use the incremental MAC in either direction,
  never resumes an upload or download across runs, and never retries a
  failed download.

## 2. Attachment encryption

### 2.1 Keys and IV

- **Attachment key**: 64 bytes from a CSPRNG, generated per attachment.
  Bytes 0–31 are the AES-256 key, bytes 32–63 the HMAC-SHA256 key. The
  whole 64 bytes go into `AttachmentPointer.key`.
- **IV**: 16 bytes from a CSPRNG, generated per attachment. It is not in
  the pointer; it is the first 16 bytes of the ciphertext.
- In signal-cli both are drawn when the upload is prepared (S2
  `CdnService.getResumableUploadSpec`), before the data is read.

A receiver must reject a key that is not exactly 64 bytes
("Invalid combined key material size").

### 2.2 Padding (before encryption)

Every attachment that goes through the normal upload path is padded with
**0x00 bytes** appended after the plaintext, up to a padded size `P(n)`
that depends only on the plaintext length `n` (S2 `PaddingInputStream`):

- `P(n) = 541` for `n ≤ 541`;
- otherwise `P(n)` is the smallest number of the form
  `floor(1.05^k)` (k a non-negative integer) that is `≥ n`.

Upstream computes this in IEEE double as
`max(541, floor(1.05 ^ ceil(ln n / ln 1.05)))`. The bucket formulation
above was checked to give identical results for every `n` from 0 to 2^24
and around every bucket boundary up to 2^31 (§9). An implementation
should use the explicit bucket table in the vectors file (312 values from
541 to 2,105,191,417) rather than re-derive it with floating point.

The first buckets are 541, 568, 596, 626, 657, 690, 725, 761, 799, 839,
881, 925, 972, 1020, ... Examples: `P(0) = P(541) = 541`, `P(542) = 568`,
`P(1000) = 1020`, `P(100000) = 100155`, `P(1048576) = 1093831`.

Notes:

- `n = 0` pads to 541 (upstream's logarithm of zero is −∞ and the `max`
  catches it).
- Upstream narrows the result to a signed 32-bit int; sizes above
  2^31 − 1 are not representable. Irrelevant in practice (§2.5).
- The pointer's `size` is the **unpadded** `n`. The receiver throws the
  padding away by truncating the decrypted stream to `size` (§5.3).
- Stickers uploaded by `uploadStickerPack` and their manifest are **not**
  padded (§6.6). Long-text attachments, voice notes, quote thumbnails and
  link-preview images are ordinary attachments and are padded.

### 2.3 Cipher layout

The uploaded blob is:

| Offset | Length | Content |
|--------|--------|---------|
| 0 | 16 | IV |
| 16 | `16·(floor(P/16) + 1)` | AES-256-CBC encryption of the padded plaintext (length `P`) with PKCS#7 padding, AES key = key bytes 0–31 |
| end − 32 | 32 | HMAC-SHA256, keyed with key bytes 32–63, over **IV ‖ CBC output** (everything before the MAC) |

PKCS#7 always adds 1–16 bytes, so a padded plaintext that is already a
multiple of 16 gets a full extra block. Total ciphertext length:

`C(P) = 16 + 16·(floor(P/16) + 1) + 32`

so `C(541) = 592`, `C(568) = 624`, `C(1020) = 1072`. signal-cli requests
the upload form with `uploadLength = C(P(n))` and the CDN body is exactly
that many bytes.

### 2.4 Digest

`AttachmentPointer.digest` = SHA-256 over the **entire uploaded blob**:
IV, CBC output and the trailing HMAC (S2 `DigestingOutputStream`, which
sits outside the cipher stream). It is not a hash of the plaintext.
(There is also a "plaintext hash" integrity mode in S2, used only for
backup media; signal-cli never uses it.)

### 2.5 Size limits

| Limit | Value | Where |
|-------|-------|-------|
| Download cap (ciphertext bytes) | 150 MiB = 157,286,400 | S1 `ServiceConfig.MAX_ATTACHMENT_SIZE`; enforced on `Content-Length` and on bytes actually read (§5.2) |
| Largest plaintext whose padded ciphertext fits the download cap | 151,032,162 bytes (itself a bucket value; ciphertext 151,032,224 bytes; the next bucket's ciphertext would exceed 150 MiB). Informative, from upstream's helper | S2 `getMaxPlaintextSizeForCiphertext` |
| Upload limit | none client-side. The server rejects `uploadLength` above its configured `maxAttachmentUploadSizeInBytes` with **413** (empty body); staging's value is not public | S4 `AttachmentControllerV4`, `AttachmentsConfiguration` |
| Sticker pack sticker / manifest downloads | 1 MiB each | S2 `PushServiceSocket.retrieveSticker`, `retrieveStickerManifest` |
| Profile and group avatar downloads | 10 MiB | S1 `ServiceConfig.AVATAR_DOWNLOAD_FAILSAFE_MAX_SIZE` |
| Text body before the long-text split | 2000 UTF-8 bytes | S1 `ServiceConfig.MAX_MESSAGE_SIZE_BYTES`, §6.5 |

### 2.6 Incremental MAC (defined, but unused by signal-cli 0.14.7)

Signal clients may add `incrementalMac` (field 19) and `chunkSize`
(field 17) so a receiver can verify a streaming download chunk by chunk.
**signal-cli 0.14.7 neither produces nor checks it**: every upload stream
is built with "faststart" off, which disables it (S2
`SignalServiceAttachment` builder default, `DigestingRequestBody`), and
downloads pass "no incremental digest, chunk size 0" (S2
`SignalServiceMessageReceiver.retrieveAttachment`). A compatible Go client
may ignore both fields on receipt and must not need them. For
completeness (S3 `incremental_mac.rs`, bridge `incremental_mac.rs`):

- **Input**: the attachment **ciphertext** blob of §2.3 (IV through MAC).
  Key: the HMAC half (bytes 32–63) of the attachment key.
- **Construction**: a single HMAC-SHA256 instance runs over the whole
  blob and is never reset. Each time another `chunkSize` bytes have been
  fed, the HMAC value of the prefix so far is emitted (32 bytes). When
  the stream ends, the HMAC value of the whole blob is emitted once more,
  **always**. The digest is the concatenation, so it has
  `floor(len / chunkSize) + 1` entries; if `len` is an exact non-zero
  multiple of `chunkSize`, the last two entries are equal; an empty input
  gives a single entry, `HMAC(key, "")`.
- **Validation**: entries are compared in order as chunks complete; the
  final entry must match the full-stream HMAC and there must be exactly
  one entry left at that point.
- **Chunk size** for a given data size `d` (the ciphertext length): the
  target is 8192 bytes of digest, i.e. 256 chunks. `chunkSize = 65536`
  if `d < 256·65536` (16 MiB); else `ceil(d/256)` if
  `d < 256·2 MiB` (512 MiB); else 2,097,152 (2 MiB).
- Field numbers 16 and 18 are reserved (older incremental-MAC variants).
- signal-service-java caps the number of attachments carrying an
  incremental MAC per envelope (signal-cli passes 10); irrelevant while
  signal-cli produces none.

## 3. `AttachmentPointer`

### 3.1 Fields

proto2. Field numbers and types from S5's bundled schema (identical to
S2's source). Listed in the order the Wire encoder writes them (S5
`javap`): Wire writes declared fields in declaration order and puts the
`oneof` members last. Byte identity is not needed for interoperability;
the order matters only when comparing against captured bytes.

| Order | # | Name | Type | Notes |
|-------|---|------|------|-------|
| 1 | 20 | clientUuid | bytes | 16-byte UUID (big-endian, RFC 4122 byte order) identifying this attachment within its message |
| 2 | 2 | contentType | string | MIME type |
| 3 | 3 | key | bytes | 64 bytes, §2.1 |
| 4 | 4 | size | uint32 | unpadded plaintext length |
| 5 | 5 | thumbnail | bytes | inline preview image bytes, **not encrypted** beyond the message itself |
| 6 | 6 | digest | bytes | 32 bytes, §2.4 |
| 7 | 19 | incrementalMac | bytes | §2.6 |
| 8 | 17 | chunkSize | uint32 | §2.6 |
| 9 | 7 | fileName | string | |
| 10 | 8 | flags | uint32 | bit 1 = VOICE_MESSAGE, bit 2 = BORDERLESS, bit 8 = GIF; value 4 is reserved |
| 11 | 9 | width | uint32 | pixels |
| 12 | 10 | height | uint32 | pixels |
| 13 | 11 | caption | string | |
| 14 | 12 | blurHash | string | |
| 15 | 13 | uploadTimestamp | uint64 | milliseconds since epoch |
| 16 | 14 | cdnNumber | uint32 | 0, 2 or 3 in practice |
| 17 | 1 | cdnId | fixed64 | oneof `attachment_identifier`: legacy numeric id (CDN 0) |
| 18 | 15 | cdnKey | string | oneof `attachment_identifier`: opaque key (CDN 2 and 3) |

Reserved: 16, 18. Next free number: 21.

### 3.2 What signal-cli sets when sending

For every attachment it uploads (S2 `AttachmentPointerUtil.createAttachmentPointer`,
`SignalServiceMessageSender.uploadAttachmentV4`; S1 `AttachmentUtils`):

| Field | Value |
|-------|-------|
| clientUuid | a fresh random (version 4) UUID per attachment |
| contentType | the detected or given MIME type (§7); for long text `text/x-signal-plain` |
| key | the 64-byte attachment key |
| size | plaintext length `n` |
| digest | §2.4 |
| fileName | basename of the file path, or the `filename` parameter of a data URI; **absent** for data URIs without it, for long-text attachments and for stickers |
| flags | **always present**: 1 if `--voice-note` was given (applied to *every* `-a` attachment of that message, not only audio), else **0** (explicitly encoded as `40 00`). Borderless and GIF are never set |
| width, height | only when > 0: from the JDK image probe (§7.3) |
| uploadTimestamp | wall-clock milliseconds when the attachment stream was created (before upload) |
| cdnNumber | the `cdn` from the upload form: 2 or 3 |
| cdnKey | the `key` from the upload form |
| not set | cdnId, thumbnail, incrementalMac, chunkSize, caption, blurHash |

Encoded examples are in the vectors file (`attachment_pointer_encoding`).

Uploads happen once per attachment, before the message is encrypted, so
the same pointer goes to every recipient and every linked device.

### 3.3 Handling on receipt

1. **Content validation** (S2 `EnvelopeContentValidator`, called by S1
   for every decrypted content): an `AttachmentPointer` in
   `DataMessage.attachments`, `DataMessage.sticker.data`, a shared
   contact's avatar or a preview image that has **neither** `cdnId` nor
   `cdnKey` makes the whole content invalid; signal-cli logs
   "Invalid content!" and drops it (see the data-message spec for what is
   reported). A `DataMessage` that carries a `text/x-signal-plain`
   attachment is allowed body ranges that run past the end of the
   (truncated) body.
2. **Conversion** (S2 `AttachmentPointerUtil.createSignalAttachmentPointer`):
   a pointer without `cdnNumber` or without `key` is a structure error
   for the content ("AttachmentPointer CDN number not set" / "key not
   set"). The remote id is `cdnKey` if the field is present (even if
   empty), else `cdnId` if it is > 0 (as a signed 64-bit value), else a
   structure error ("CDN location not set").
3. Other fields are taken as optional: missing `size` becomes "unknown"
   (and then decrypts to **zero bytes**, §5.3), `width`/`height`/
   `uploadTimestamp` default to 0, `flags` bits 1/2/8 become the voice
   note / borderless / GIF booleans (bit 4 ignored), `clientUuid` is kept
   if it parses as a UUID. `incrementalMac`/`chunkSize` are carried but
   not used.
4. For output (S1 `MessageEnvelope.Attachment`): `id` = the stored file
   name (§6.3), `file` = its path (reported whether or not the download
   succeeded), `uploadTimestamp`, `width`, `height` omitted when 0,
   `caption` omitted when empty, `size` only if present. Exact JSON shape:
   `spec/surface/cli/_received-message-output.md`.

## 4. Upload

### 4.1 Sequence

For `send -m TEXT -a A -a B ...` signal-cli does, in this order (S1
`ManagerImpl.applyMessage`, `AttachmentHelper`):

1. If the text is longer than 2000 UTF-8 bytes: prepare and upload the
   long-text attachment completely (form, create, upload) (§6.5).
2. For each `-a` argument in order: open the file or decode the data URI
   (§7), then fetch an upload form and create the resumable session
   (§4.2–§4.4). All forms and sessions are obtained **before** any data
   is uploaded.
3. For each attachment in order: upload the data (§4.5 / §4.6).
4. Build the `DataMessage` with the pointers: long-text pointer first,
   then the `-a` attachments in argument order.

Quote thumbnails (`--quote-attachment ...:previewFile`), link-preview
images (`--preview-image`) and stickers go through the same per-file
steps when they are attached (see the data-message spec). Any failure
(network, non-2xx, missing file) aborts the send before anything is sent
to recipients.

### 4.2 Upload form

| Item | Value |
|------|-------|
| Request | `GET /v4/attachments/form/upload?uploadLength=<C(P(n))>` (decimal) |
| Transport | **authenticated chat WebSocket** (libsignal typed API `AuthMessagesService.getUploadForm`; with signal-cli's empty remote config libsignal uses the WebSocket, not gRPC) (S2 `AttachmentApi`, S3 `ws/messages.rs`) |
| Headers, body | none beyond the socket's own |
| 200 | JSON object: `cdn` (integer), `key` (string), `headers` (object of string → string), `signedUploadLocation` (string, absolute URL) |
| 413 | upload too large; expected empty body; signal-cli fails the attachment |
| other | standard handling (`api.md` §4.2): 429 with `Retry-After`, 5xx = server error |

Server side (S4, informative): `cdn` is 3 for accounts enrolled in the
"cdn3" experiment, else 2. `key` is 15 random bytes encoded with the
URL-safe base64 alphabet (`A–Z a–z 0–9 - _`); 15 bytes give exactly 20
characters, so there is never a `=`. The server rate-limits by count and, when
`uploadLength` is given, by bytes; `uploadLength` must be positive.

What the form contains:

- **CDN 2** (Google Cloud Storage via a V4 signed URL): `headers` =
  `host: <GCS domain>`, `x-goog-content-length-range: 1,<uploadLength>`,
  `x-goog-resumable: start`; `signedUploadLocation` =
  `https://<GCS domain>/<path prefix>/<key>?<signed query>&X-Goog-Signature=<sig>`.
- **CDN 3** (tus server): `headers` = `Authorization: Bearer <JWT>` (the
  JWT carries the key and the max length) and
  `Upload-Metadata: filename <base64(key)>` (standard base64 of the key's
  UTF-8 bytes); `signedUploadLocation` = `<tus base URI>/attachments`.

Clients treat all of these as opaque except as noted below.

### 4.3 URL rewriting: everything goes to the configured CDN host

signal-service-java never connects to the host in `signedUploadLocation`
or in a returned `Location`. For each CDN request it takes the
**configured** base URL for that CDN number and appends the **path,
query and fragment** of the server-supplied URL, keeping them in their
already-encoded form (S2 `PushServiceSocket.buildConfiguredUrl`,
`SignalRestClient.computeRequestUrl`).

| CDN number | Configured base (staging) |
|-----------|----------------------------|
| 0 | `https://cdn-staging.signal.org` |
| 2 | `https://cdn2-staging.signal.org` |
| 3 | `https://cdn3-staging.signal.org` |

(S1 `StagingConfig`.) Example: a CDN 2 form location
`https://storage.example/attachments-bucket/AbC...?X-Goog-...` is posted
to `https://cdn2-staging.signal.org/attachments-bucket/AbC...?X-Goog-...`.
A relative `Location` header would fail to parse ("Malformed URL!").

### 4.4 Creating the resumable session (both CDNs)

Sent by `CdnService.getResumableUploadUrl` through `SignalRestClient`
(OkHttp, §4.7):

- `POST <configured base><path+query of signedUploadLocation>`, empty
  body.
- Headers: every form header **except** `host` (case-insensitive), then
  `Content-Length: 0`, then:
  - CDN 2: `Content-Type: application/octet-stream`;
  - CDN 3: `Upload-Defer-Length: 1` and `Tus-Resumable: 1.0.0`
    (no checksum header from signal-cli);
  - any other `cdn` value: client-side error "Unknown CDN version".
- Plus `X-Signal-Agent: <agent string>` (`transport.md` §4.2) and the
  OkHttp defaults of §4.7. No `Authorization` other than the form's.
- Success: any 2xx. The result is the response's `Location` header
  (required; missing → "Missing Location header in resumable-upload
  response"). Non-2xx fails the attachment.

The session URL is kept only in memory (with the key, IV and a nominal
7-day expiry). signal-cli never persists it, so there is no resumption
across runs.

### 4.5 CDN 2 upload (GCS-style resumable)

Both requests go to `<cdn2 base><path+query of Location>` (S2
`PushServiceSocket.uploadToCdn2`, `getResumeInfoCdn2`). The form headers
are **not** sent on these requests.

1. **Status probe**: `PUT`, empty body, `Content-Range: bytes */<N>`
   where `N` = ciphertext length.
   - 2xx: the upload is already complete. signal-cli then encrypts the
     data locally anyway (to compute the digest) and skips step 2.
   - **308**: read the `Range` header (form `bytes=0-<last>`); resume
     offset = `<last> + 1`, or 0 if there is no `Range` header.
   - 404: "resume location invalid"; other codes: failure.
2. **Data**: `PUT` with `Content-Range: bytes <offset>-<N−1>/<N>`,
   `Content-Type: application/octet-stream`, `Content-Length: N − offset`,
   body = the ciphertext from byte `offset` on. The whole ciphertext is
   regenerated with the same key and IV and the first `offset` bytes are
   discarded, so the digest still covers the full blob. Success = any
   2xx.

For a fresh session the probe answers 308 without `Range`, so in
practice: `PUT bytes */N`, then `PUT bytes 0-(N−1)/N` with the full body.

### 4.6 CDN 3 upload (tus 1.0.0)

Both requests go to `<cdn3 base><path+query of Location>` (S2
`uploadToCdn3`, `getResumeInfoCdn3`).

1. **Offset query**: `HEAD`, headers `Tus-Resumable: 1.0.0` then all form
   headers (`Authorization: Bearer …`, `Upload-Metadata: …`; a form
   `host` header, if one existed, would be sent too). Success = 2xx with
   an `Upload-Offset` header (decimal); any non-2xx = "resume location
   invalid".
2. If `Upload-Offset` equals `N`: already complete, digest computed
   locally, no PATCH.
3. **Data**: `PATCH` with `Upload-Offset: <offset>`, `Upload-Length: <N>`
   (this also resolves the deferred length), `Tus-Resumable: 1.0.0`, then
   all form headers, `Content-Type: application/offset+octet-stream`,
   `Content-Length: N − offset`, body = ciphertext from `offset`.
   Success = any 2xx (tus servers answer 204).

signal-service-java also has a single-request tus "creation with upload"
path (`POST` with body and `Upload-Length`) used by Signal-Android;
**signal-cli 0.14.7 does not use it** (it goes through
`SignalServiceMessageSender.uploadAttachment`, which always does
HEAD + PATCH after the POST of §4.4).

### 4.7 HTTP details common to all CDN traffic

| Item | Value |
|------|-------|
| Client | OkHttp 5.3.2, one client per configured URL; CDN requests from `PushServiceSocket` and from `SignalRestClient` use separate but identically configured clients |
| TLS | TLS 1.3 or 1.2 (`RESTRICTED_TLS`), ALPN as OkHttp offers it (`h2`, `http/1.1`) |
| Trust | the same BKS truststore as the REST side (`whisper.store`), i.e. effectively only the Signal RSA root (`transport.md` §5); default hostname verification. That the staging CDN hosts present chains to that root is inferred, not observed |
| `User-Agent` | the signal-cli agent string (`Signal-Android/8.21.1 signal-cli/0.14.7`), set by signal-cli's interceptor on every OkHttp request, replacing OkHttp's default |
| `X-Signal-Agent` | only on the session-creation POST (§4.4) |
| `Authorization` | none, except the CDN 3 form's Bearer token on POST/HEAD/PATCH |
| Other headers | OkHttp adds `Host`, `Accept-Encoding: gzip`, `Connection: Keep-Alive` on HTTP/1.1 |
| Timeouts | connect and read 30 s per request |
| Retries | OkHttp's transport-level retry on connection failure only; no retry on HTTP status codes |
| Proxy | the JVM default proxy selector, as for REST (`transport.md` §6). signal-cli's `SignalProxy` is unset for staging |

### 4.8 Result

On success the pointer of §3.2 is built from the form (`cdn` →
`cdnNumber`, `key` → `cdnKey`), the attachment key, and the digest
computed while uploading.

### 4.9 CDN 0 form posts (sticker packs)

Used by `uploadStickerPack` (and, outside this spec, group avatars). The
form comes from `GET /v1/sticker/pack/form/<count>` on the **REST** chat
host (`https://chat.staging.signal.org`, Basic auth, `X-Signal-Agent`),
`count` = number of stickers + 1 if there is a cover (server accepts
1–201). The JSON answer has `packId` (32 lowercase hex characters),
`manifest` and `stickers[]`, each item with `id`, `key`, `credential`,
`acl`, `algorithm`, `date`, `policy`, `signature` (S4 `StickerController`).

Each blob is uploaded as `POST https://cdn-staging.signal.org/` (path
empty) with a `multipart/form-data` body whose parts are, in order:
`acl`, `key`, `policy`, `Content-Type` (value `application/octet-stream`),
`x-amz-algorithm`, `x-amz-credential`, `x-amz-date`, `x-amz-signature`
(values from the item), then `file` with filename `file` and content
type `application/octet-stream` containing the encrypted blob (S2
`uploadToCdn0`). Success = 2xx.

## 5. Download

### 5.1 URL

`<configured base for pointer.cdnNumber>/attachments/<id>` (S2
`PushServiceSocket.retrieveAttachment`):

- `cdnKey` present: `<id>` = the key encoded with Java's
  `URLEncoder.encode(…, UTF-8)` (application/x-www-form-urlencoded rules:
  `A–Z a–z 0–9 . - * _` unchanged, space → `+`, everything else `%XX`
  of the UTF-8 bytes, upper-case hex). Server keys are URL-safe base64,
  which passes through unchanged.
- else `cdnId` > 0: `<id>` = its decimal value.
- A `cdnNumber` with no configured base (anything but 0, 2, 3) fails
  ("Attempted to download from unsupported CDN number").

Examples (staging): `https://cdn3-staging.signal.org/attachments/AbCdEfGhIjKlMnOpQrSt`,
`https://cdn-staging.signal.org/attachments/1234567890123`.

### 5.2 Request

- `GET`, no `Authorization`, `User-Agent` as in §4.7, no extra headers.
- signal-cli always downloads into a **fresh** temporary file
  (`signal-cli_tmp_*.tmp` in `java.io.tmpdir`, mode 0600), so the offset
  is 0 and no `Range` header is sent. (The library would send
  `Range: bytes=<existing length>-` for a non-empty destination and maps
  416 to a range error; unused.)
- Success = any 2xx. If the response `Content-Length` exceeds the cap
  (150 MiB for attachments), fail; while streaming, fail as soon as the
  bytes read exceed the cap.
- Non-2xx: failure with the status code. No retry.

### 5.3 Verification and decryption order

After the download completes (S1 `AttachmentHelper`, S2
`SignalServiceMessageReceiver`, `AttachmentCipherInputStream`):

1. signal-cli requires a `digest` in the pointer; without one the
   attachment fails ("Attachment pointer has no digest.") — checked
   **before** downloading. The library requires a `key`.
2. Ciphertext length must be **> 48** (16 IV + 32 MAC), else
   "Message shorter than crypto overhead!".
3. Compute HMAC-SHA256 (key bytes 32–63) over all bytes except the last
   32 and compare with the last 32 bytes (constant time). Mismatch:
   "MAC doesn't match!".
4. Compute SHA-256 over the **whole** file and compare with `digest`.
   Mismatch: "Digest doesn't match!".
5. Decrypt bytes `[16, len−32)` with AES-256-CBC (key bytes 0–31, IV =
   first 16 bytes) and deliver only the first `size` plaintext bytes
   (`size` absent → 0 bytes). PKCS#7 padding is checked only if the
   reader reaches the end of the ciphertext, which normally does not
   happen because `size` stops it earlier; a `size` larger than the
   padded plaintext yields the 0x00 padding bytes and then ends at the
   true plaintext end.

The whole file is verified before the first plaintext byte is
delivered, so a failed check writes nothing.

### 5.4 Failure handling

Failures are logged at WARN ("Failed to download attachment (<id>),
ignoring: <message>") and the message is still processed and reported.
Because the destination file is opened (created/truncated) **before**
the download starts (§6.3), a failed download leaves an **empty file**
at the attachment's path; a failure during decryption could leave a
partial file. The temp file is deleted in all cases.

## 6. Local storage in signal-cli

### 6.1 Directory layout

Under the signal-cli config directory (`--config`, default
`$XDG_DATA_HOME/signal-cli`, else `$HOME/.local/share/signal-cli`)
(S1 `PathConfig`):

| Directory | Contents |
|-----------|----------|
| `attachments/` | downloaded attachments and `.preview` files (§6.3) |
| `avatars/` | `profile-<id>`, `contact-<id>`, `group-<id>` (§6.7) |
| `stickers/<packIdHex>/` | `manifest.json` and one file per sticker id (§6.6) |
| `data/` | account data (not touched by this spec) |

These directories are **shared by all accounts** in the config directory.
They are created on first use with mode 0700 (POSIX); files are created
with default permissions.

### 6.2 When downloads happen

During `receive` (and the daemon's receive loop), for each decrypted
`DataMessage` (S1 `IncomingMessageHandler`), unless
`--ignore-attachments`:

- every pointer in `attachments` (and its inline `thumbnail`, §6.3);
- the avatar of each shared contact;
- the image of each link preview;
- the thumbnail of each quoted attachment;
- for a `StoryMessage`: the file attachment, and the preview image of a
  text attachment (see the stories spec).

Not downloaded into `attachments/`: `DataMessage.sticker.data` (the pack
is fetched instead, §6.6), contact/group sync blobs (fetched to a temp
file and parsed; see the sync spec).

With `--ignore-attachments`, nothing is stored, **except** that
`text/x-signal-plain` pointers in `attachments` are still downloaded
(temp file only) to recover the long text (§6.5).

Downloads run synchronously inside message handling, one at a time, in
the order above.

### 6.3 File names

The file for a pointer is `attachments/<sanitized id><ext>` (S1
`AttachmentStore`):

- `<id>`: the remote id as a string: the `cdnKey`, or the decimal
  `cdnId`.
- Sanitizing replaces every character outside `A–Z a–z 0–9 _ . -` with
  `_` (so `/`, `+`, `=` become `_`).
- `<ext>`:
  1. if the pointer's `fileName` contains a `.`: `.` plus everything after
     its **last** `.` (unchanged case; `photo.JPEG` → `.JPEG`,
     `archive.tar.gz` → `.gz`, `x.` → `.`);
  2. otherwise, from `contentType` by this fixed table:

     | contentType | ext |
     |-------------|-----|
     | `application/vnd.android.package-archive` | `apk` |
     | `application/json` | `json` |
     | `image/png` | `png` |
     | `image/jpeg` | `jpg` |
     | `image/heic` | `heic` |
     | `image/heif` | `heif` |
     | `image/webp` | `webp` |
     | `image/gif` | `gif` |
     | `audio/aac` | `aac` |
     | `video/mp4` | `mp4` |
     | `text/x-vcard` | `vcf` |
     | `text/plain`, `text/x-signal-plain` | `txt` |
     | `application/octet-stream` | `bin` |

     (exact, case-sensitive match);
  3. otherwise no extension.
- If the pointer has a `thumbnail`, its raw bytes are written first to
  the same name plus `.preview` (e.g. `AbC….jpg.preview`), regardless of
  whether the main download later succeeds.
- An existing file with the same name is overwritten. No sender or
  timestamp is part of the name.

The **attachment id** shown to users (`receive` output `id`) is this
file name, extension included.

### 6.4 `getAttachment --id ID`

Reads `attachments/<sanitized ID>` (same sanitizing as above, applied to
the whole given string, extension included) and prints its bytes base64
(standard alphabet, padded) — plain text: one line; JSON:
`{"data":"<base64>"}`. `--recipient` / `--group-id` are required by the
argument parser (one of them) but **ignored** for the lookup. A missing
file → user error "Could not find attachment with ID: <ID>". No network
access.

### 6.5 Long-text attachments (`text/x-signal-plain`)

**Send** (S1 `ManagerImpl.applyMessage`; S2 `splitByByteLength`): if the
message text is more than 2000 UTF-8 bytes,

- `body` = the longest prefix made of whole code points (never splitting
  a surrogate pair) whose UTF-8 encoding is ≤ 2000 bytes;
- an extra attachment is uploaded with contentType
  `text/x-signal-plain`, **no** file name, containing the **entire**
  message text in UTF-8 (not just the remainder), padded and encrypted
  like any attachment;
- that pointer comes first in `attachments`.

Text of exactly 2000 bytes or less is sent as `body` alone.

**Receive**: for each `text/x-signal-plain` pointer in `attachments`, the
file contents (UTF-8) replace the displayed `body` (if several, the last
one wins). With attachments enabled the text is read back from the stored
file (`<id>.txt`); with `--ignore-attachments` it is downloaded to a temp
file only. A failed download or read leaves the original `body`. The
long-text attachment is still listed among the message's attachments.

### 6.6 Sticker packs

**Identifiers.** A pack is identified by a 16-byte **pack id** and
encrypted with a 32-byte **pack key**. Both are written in lowercase hex
in paths and URLs. Share URL (S1 `StickerPackUrl`):
`https://signal.art/addstickers/#pack_id=<hex id>&pack_key=<hex key>`.
When parsing, the part after `#` is split on `&` and `=`, each name and
value is URL-decoded, and hex is accepted in either case; missing or
empty fragment / `pack_id` / `pack_key` are errors.

**Key derivation.** The 64-byte attachment-style key for every blob of a
pack = HKDF-SHA256 (RFC 5869) with **no salt** (equivalently 32 zero
bytes), IKM = pack key, info = the ASCII bytes `Sticker Pack`, output
length 64. Bytes 0–31 AES key, 32–63 HMAC key.

**Blob format.** Same layout as §2.3 (IV, CBC, HMAC over IV‖CBC), random
IV, **no** 0x00 size-bucket padding (the plaintext is the exact sticker
file or manifest). Decryption (S2 `createForStickerData`): length must be
> 48, HMAC verified (no digest exists), then the whole CBC part is
decrypted and PKCS#7 removed.

**CDN paths** (CDN 0, `https://cdn-staging.signal.org/`, no auth, 1 MiB
cap each):

- manifest: `stickers/<hex id>/manifest.proto`
- sticker: `stickers/<hex id>/full/<sticker id in decimal>`

**Manifest** plaintext is a `Pack` protobuf (proto2; S5
`StickerResources.proto`):

| Message | # | Field | Type |
|---------|---|-------|------|
| Pack | 1 | title | string |
| Pack | 2 | author | string |
| Pack | 3 | cover | Pack.Sticker |
| Pack | 4 | stickers | repeated Pack.Sticker |
| Pack.Sticker | 1 | id | uint32 |
| Pack.Sticker | 2 | emoji | string |
| Pack.Sticker | 3 | contentType | string |

**Download** (S1 `StickerHelper.retrieveStickerPack`): skipped if
`stickers/<hex id>/manifest.json` already exists. Otherwise fetch and
decrypt the manifest, then fetch each distinct sticker id among the cover
and the stickers, writing each decrypted sticker to
`stickers/<hex id>/<id>`, and finally write `manifest.json` (UTF-8 JSON,
keys in this order): `title`, `author`, `cover`, `stickers`, where cover
and each sticker are objects `{id, emoji, file, contentType}` with
`file` = the id as a decimal string; absent values are written as
`null`. Because `manifest.json` is written last, an interrupted download
is redone from scratch next time.

**When packs are downloaded**:

- a received `DataMessage.sticker`: the pack (id, key) is recorded in the
  account's sticker store if unknown, and, unless `--ignore-stickers`, a
  background job downloads it (failures logged, ignored);
- a sticker-pack-operation sync message of type INSTALL (or with no
  type) that has a pack id: recorded/updated and, unless
  `--ignore-stickers`, downloaded (see the sync spec);
- `addStickerPack --uri`: downloaded immediately (failure is an error);
- sending a sticker: downloaded on demand if `manifest.json` is missing;
- `getSticker`: if the sticker file is missing and the pack is in the
  store, the pack is downloaded, but this call still fails with
  "Could not find sticker with ID: …" (it does not re-check the file);
  a second call succeeds.

**Upload** (`uploadStickerPack PATH`; S1 `StickerUtils`,
`ManagerImpl.uploadStickerPack`; S2 `uploadStickerManifest`): `PATH` is a
`manifest.json` (its directory holds the files) or a `.zip` containing
`manifest.json`. The JSON has `title`, `author`, `cover`, `stickers`, each
sticker `{file, emoji, contentType}`; `stickers` must be non-empty and
every sticker and the cover need `file`. A new random 32-byte pack key
is generated. Stickers get ids 0..k−1 in list order; the cover, if given,
gets id k, else the cover entry in the manifest is a copy of sticker 0.
The uploaded manifest contains `title`, `author` (if given), `cover`
`{id, emoji}` and `stickers` `{id, emoji}` — **contentType is not
included**; emoji defaults to the empty string. Then (§4.9) the manifest
and each sticker (cover last) are uploaded to the form item with the
matching id. Result: the share URL; signal-cli also adds the pack to its
store and sends a sticker-pack sync message (sync spec).

**Sending a sticker** (`send --sticker PACKID:STICKERID`, see the
data-message spec): the pack must be in the account's sticker store
("Sticker pack not found" otherwise). signal-cli takes the emoji from the
`manifest.json` entry at **list index** `STICKERID` (not the entry whose
`id` equals it; an index past the end is "Sticker id not part of this
pack") and the local file `stickers/<hex>/STICKERID` (missing → "Missing
local sticker file"), and uploads that file as an ordinary attachment
(§4: padded, fresh key, no file name), content type detected as in §7.2
for a file without extension. Observed with the JDK in this environment, a WebP sticker file
sniffs as `audio/x-wav` (its first bytes are `RIFF`), an unrecognised one
falls back to `application/octet-stream`.

### 6.7 Avatars

| Kind | Source | Stored as | Format on CDN |
|------|--------|-----------|---------------|
| Profile | the profile's `avatar` path (e.g. `profiles/…`), fetched every time a profile is retrieved **with a known profile key**, unless `--ignore-avatars`; if the profile has no avatar the local file is deleted | `avatars/profile-<id>` | `GET https://cdn-staging.signal.org/<path>` (CDN 0, no auth, 10 MiB cap); body = 12-byte nonce ‖ AES-256-GCM ciphertext ‖ 16-byte tag, key = the 32-byte profile key, no associated data (S2 `ProfileCipherInputStream`) |
| Group v2 | `DecryptedGroup.avatar` (a CDN 0 key, e.g. `groups/…`) when group state is updated, unless `--ignore-avatars` | `avatars/group-<base64 group id with / → _>` | `GET https://cdn-staging.signal.org/<key>`, 10 MiB cap, decrypted with the group's secret params (groups spec, not covered here) |
| Group v1 / contact sync | embedded in sync blobs, not fetched by URL | `avatars/group-…`, `avatars/contact-<id>` | see the sync spec |

`<id>` is the recipient's phone number if known, else the ACI string,
else the PNI string. Avatar failures are logged and ignored. Note that
the profile avatar is decrypted and written as it streams and the GCM
tag is checked only at the end: on a tag failure the file already holds
the unverified bytes and only a warning is logged.

## 7. Turning `send -a` arguments into attachments

Each `-a` value is tried as a data URI first; if it does not parse as one
it is treated as a file path (S1 `Utils.createStreamDetails`). A value
that fails as a data URI only because of bad base64 is **not** retried
as a path; it is an error.

### 7.1 Data URIs

signal-cli's parser (S1 `DataURI`, documented as RFC 2397 plus a
`filename` parameter) behaves as follows; the grammar is looser than RFC
2397 in places:

- Case-insensitive `data:` prefix; the whole string must match.
- Optional media type: a non-empty run containing a `/` (shortest match).
  Absent → `text/plain`. Media-type case is preserved.
- Optional parameters: starting with `;`, `name=value` pairs. **Quirk:**
  the pairs are split greedily on the *last* `=`, so with two or more
  parameters the first key swallows the rest: `;filename=a.png;foo=bar`
  yields one parameter named `filename=a.png;foo` with value `bar`, and
  no `filename`. With a single parameter it works as expected. Values
  are URL-decoded (`+` → space, `%XX` UTF-8); keys are not.
- Optional `;base64` (case-insensitive) immediately before the comma.
- A `,` and then at least one data character (empty data is an error).
- Data: with `;base64`, standard-alphabet base64 (padding optional,
  invalid characters are an error); otherwise URL-decoded and re-encoded
  as UTF-8 (so `+` becomes a space).
- Only the `filename` parameter is used: it becomes the pointer's
  `fileName`. Other parameters (e.g. `charset`) are ignored. The media
  type becomes `contentType` unchanged. Length = decoded byte count.

Fourteen parse cases (successes, the multi-parameter quirk, errors) are
in the vectors file.

### 7.2 File paths

- The path is canonicalised (symlinks resolved); a file under the
  account data directory `<config>/data` is refused ("Attaching files
  from the signal-cli data directory is not allowed"). Files in
  `<config>/attachments` are allowed.
- `fileName` = the last path component as given.
- Length = the file size at open time.
- **contentType**: first the JDK's `Files.probeContentType` on the path
  (on Linux: the user's `~/.mime.types`, `/etc/mime.types`, then the
  JDK's built-in table, all keyed by file-name extension); if that
  yields nothing, the JDK's content sniffing of the first bytes
  (`URLConnection.guessContentTypeFromStream`); if that also yields
  nothing, `application/octet-stream`. Both steps are JDK behaviour and
  depend on the host. Observed with Temurin 25.0.5 and no MIME database
  files: `.png` image/png, `.jpg`/`.jpeg` image/jpeg, `.gif` image/gif,
  `.webp` image/webp, `.mp4` video/mp4, `.mov` video/quicktime, `.m4a`
  audio/mp4, `.mp3` audio/mpeg, `.ogg` audio/ogg, `.aac` audio/aac,
  `.txt` text/plain, `.pdf` application/pdf, `.json` application/json,
  `.zip` application/zip; `.heic`, `.vcf` and extension-less names fall
  through to sniffing (which recognises PNG, JPEG, GIF, HTML, XML, and
  `RIFF…` as audio/x-wav, but not plain text, PDF or ZIP). Full table in
  the vectors file. A Go implementation needs its own extension table;
  matching the JDK exactly is not required for interoperability.

### 7.3 Image dimensions

If `contentType` starts with `image/` and the length is between 1 byte
and 20 MiB, signal-cli decodes the image with the JDK's ImageIO (PNG,
JPEG, GIF, BMP, WBMP, TIFF readers; not WebP or HEIC) to fill `width` and
`height`. Undecodable images, other types, or larger files leave both
at 0, so the fields are omitted. EXIF orientation is not applied (raw
pixel dimensions).

## 8. Minimal behaviour for a compatible Go client

1. Send: pad (§2.2), encrypt (§2.3), digest (§2.4); get the form over the
   authenticated WebSocket with `uploadLength` = ciphertext length;
   create the session at the configured CDN host; upload per §4.5/§4.6;
   emit the pointer fields of §3.2 (always `cdnKey`, `cdnNumber`,
   `key`, `size`, `digest`).
2. Receive: accept pointers per §3.3; download per §5 with a 150 MiB cap;
   verify MAC then digest before writing anything; truncate to `size`.
3. Store per §6.3 if byte-compatible file names and `getAttachment`
   behaviour are wanted.
4. Ignore `incrementalMac`, `chunkSize`, `blurHash`, `caption` beyond
   reporting them.

## 9. Test vectors

`testdata/vectors/attachments.json`:

| Block | Count | Provenance |
|-------|-------|------------|
| `padding.buckets_from_541_to_2^31` | 312 values | computed with upstream's formula in the jar; bucket rule verified against `getPaddedSize` on 16,778,777 sizes, 0 mismatches |
| `padding.samples` | 27 (n → padded → ciphertext size) | jar |
| `attachment_cipher.cases` | 6 (fixed key 0x00..0x3f, IV 0x40..0x4f; unpadded and padded; full ciphertext hex for the small ones, SHA-256 for the 100,000-byte one) | jar (`AttachmentCipherOutputStream`), round-tripped through `AttachmentCipherInputStream`; one case cross-checked with openssl |
| `incremental_mac.upstream` | 2 | **upstream fixed vectors** (libsignal Java test; Node/Swift tests), reproduced with the jar |
| `incremental_mac.derived` | 3 (empty input; exact multiple showing the duplicated final entry; 200,000 bytes with inferred 64 KiB chunks) | jar, cross-checked by independent HMAC computation (openssl for the first two) |
| `incremental_mac.chunk_sizes` | 15 (12 are the upstream Rust test rows) | upstream table + jar |
| `sticker` | HKDF expansion, one encrypted sticker blob, one encrypted manifest (with its `Pack` bytes), share URL, CDN paths | jar; HKDF cross-checked with openssl |
| `data_uri` | 14 | jar (`DataURI.of`) |
| `long_text_split` | 6 | jar (`splitByByteLength`) |
| `stored_file_names` | 9 | jar (`AttachmentStore.getAttachmentFile`) |
| `attachment_pointer_encoding` | 2 | jar (`AttachmentPointerUtil.createAttachmentPointer(...).encode()`) |
| `mime_observations` | 27 extensions, 9 sniffed contents | JDK in this environment (informative only) |

Upstream has no fixed vectors for the attachment cipher, the padding
function (its tests are randomized or only check lengths), sticker key
derivation, or CDN protocols.

## Gaps

- **No server or CDN was contacted.** The staging CDN hosts' certificate
  chains (assumed to chain to the Signal RSA root in `whisper.store`,
  since signal-cli trusts nothing else), their HTTP versions, the actual
  `Location` values returned by `cdn2-staging` / `cdn3-staging` (assumed
  absolute URLs), and the status codes they use (GCS: 201 on create and
  308 on probe; tus: 201 on create, 200 on HEAD, 204 on PATCH) are from
  the client code and public protocol documentation, not observed.
- Whether staging enrols accounts in the "cdn3" experiment (so whether a
  test sees CDN 2 or CDN 3) is unknown; both paths must work.
- Staging's `maxAttachmentUploadSizeInBytes` is not public (the sample
  config's 1024 is a placeholder).
- How a receiver reports content dropped for an invalid pointer (§3.3)
  belongs to the data-message spec and was not traced here.
- Group v2 avatar decryption (`GroupSecretParams` blob cipher) and
  contact/group sync blob formats are out of scope (groups and sync
  specs).
- MIME detection in §7.2 is JDK- and host-dependent; the observations
  are from one environment.
- OkHttp default headers and ALPN (§4.7) are stated from general OkHttp
  behaviour, not re-read from OkHttp 5.3.2 source.
