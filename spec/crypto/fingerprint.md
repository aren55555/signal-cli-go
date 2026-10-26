# Safety numbers: numeric and scannable fingerprints (signal-cli 0.14.7)

Spec role: prose only; no code. This file covers how signal-cli 0.14.7
computes the safety number it shows for each stored identity key, how it
formats that number in `listIdentities`, what the `scannableSafetyNumber`
field contains, and how `trust -v` matches a value the user supplies.
The command surfaces (arguments, help text, other error rows) are in
`spec/surface/cli/listIdentities.md`, `spec/surface/cli/trust.md` and
the matching `spec/surface/jsonrpc/methods/*.md` files. This file is the
"fingerprint spec" that `spec/service/contacts.md` Gap 3 asks for.

Vectors: `testdata/vectors/fingerprint.json`.

## Sources

| Ref | What | URL @ version |
|-----|------|---------------|
| F1 | libsignal fingerprint core | https://github.com/signalapp/libsignal @ v0.99.1 (commit 97801d22dcf9f5bf714f7b8fa3212cdc973ae1c8): `rust/protocol/src/fingerprint.rs` (including its `#[cfg(test)]` module), `rust/protocol/src/proto/fingerprint.proto` |
| F2 | libsignal bridge and Java API (same tree) | `rust/bridge/shared/src/protocol.rs` (Fingerprint_New, ScannableEncoding, DisplayString, ScannableFingerprint_Compare), `java/shared/java/org/signal/libsignal/protocol/fingerprint/{NumericFingerprintGenerator,Fingerprint,DisplayableFingerprint,ScannableFingerprint,FingerprintParsingException,FingerprintVersionMismatchException}.java`, `java/shared/java/org/signal/libsignal/protocol/util/Hex.java` |
| F3 | libsignal tests with fixed values (same tree) | `java/client/src/test/java/org/signal/libsignal/protocol/fingerprint/NumericFingerprintGeneratorTest.java`, `node/ts/test/protocol/ProtocolTest.ts` (test "Fingerprint") |
| F4 | signal-service-java (Turasa fork) | https://github.com/Turasa/libsignal-service-java @ v2.15.3_unofficial_151 (commit 66956cb4a0394d50cee3590209baf58312084a44): `core/models-jvm/src/main/java/org/signal/core/models/ServiceId.kt` (binary form of a service ID, "unknown" test) |
| F5 | signal-cli | https://github.com/AsamK/signal-cli @ v0.14.7 (commit b01b6b370dc063599a1a2b9fde0f5ff4e2d78fe8): `lib/src/main/java/org/asamk/signal/manager/util/Utils.java`, `lib/.../manager/helper/IdentityHelper.java`, `lib/.../manager/internal/ManagerImpl.java` (getIdentities, toIdentity, trustIdentityVerified), `lib/.../manager/api/{Identity,IdentityVerificationCode}.java`, `lib/.../manager/storage/SignalAccount.java` (self-identity trust), `src/main/java/org/asamk/signal/commands/{ListIdentitiesCommand,TrustCommand}.java`, `src/main/java/org/asamk/signal/util/{Util,Hex}.java` |
| F6 | Reference distribution, offline | `javac`/`java` and `jshell` against `/work/third_party/signal-cli-0.14.7/lib/*.jar` (libsignal-client 0.99.1 with its native library, libsignal-cli 0.14.7, models-jvm 2.15.3_unofficial_151), no network. Produced the `reference_jar` vectors and confirmed the parsing behaviour in §6. A separate, independent SHA-512 computation (JDK `MessageDigest`) reproduced every per-side digest. |
| F7 | FIPS 180-4 | SHA-512 (public standard) |

## 1. Terms

- **Identity key**: a Curve25519 public key in its 33-byte serialized
  form, a type byte `0x05` followed by the 32-byte key. Everything below
  uses these 33 bytes, never the bare 32.
- **Stable identifier**: a byte string that names one party. signal-cli
  uses one of two forms (§3).
- **Per-side digest**: a 64-byte value computed from one party's stable
  identifier and identity key (§2.1).
- **Per-side digits**: a 30-digit decimal string derived from a per-side
  digest (§2.2).
- **Safety number** (libsignal: "displayable fingerprint"): the 60-digit
  string made of both parties' per-side digits (§2.3).
- **Scannable safety number** (libsignal: "scannable fingerprint"): a
  small protobuf holding a version number and the first 32 bytes of both
  per-side digests (§4). The apps put it in a QR code.

## 2. Numeric safety number

### 2.1 Per-side digest

Inputs: an iteration count N, a stable identifier `id`, and a 33-byte
identity key `K`. signal-cli always uses **N = 5200**.

1. Compute SHA-512 over the concatenation of: the two bytes `0x00 0x00`
   (a fixed fingerprint-format marker, independent of the "version"
   in §3), then `K`, then `id`, then `K` again.
2. Then, N − 1 more times: replace the current 64-byte value with
   SHA-512 of (current value followed by `K`).

So there are N SHA-512 calls in total, and the result is the 64-byte
output of the last one. libsignal rejects N ≤ 1 and N > 1 000 000
("Invalid fingerprint iterations"); with 5200 this never fires.

The identifier length is not encoded anywhere; the bytes are simply
concatenated. The "version" number of §3 does **not** enter the digest.
That is why the numeric safety number is the same for version 1 and
version 2 when the same identifiers are used (libsignal tests F3 check
this).

### 2.2 Per-side digits

Take the first 30 bytes of the 64-byte digest as six chunks of 5 bytes.
Read each chunk as a 40-bit unsigned big-endian integer, reduce it
modulo 100 000, and write it as exactly five decimal digits with leading
zeros. Concatenate the six groups in order: 30 ASCII digits. Bytes 30–63
of the digest are not used here.

### 2.3 Combining both sides

Compute the per-side digits for the local party (own identifier, own
identity key) and for the remote party (their identifier, their identity
key). The 60-digit safety number is the two 30-digit strings
concatenated with the **smaller one first**. "Smaller" is plain
character-by-character comparison; since both strings have the same
length and contain only digits, this is the same as numeric order. If
the two strings are equal, the order does not matter.

Because of this sort, both parties compute the same 60 digits, no matter
who is "local". There are no separators in the computed value.

## 3. Which identifier and version signal-cli 0.14.7 uses

signal-cli computes a safety number for one stored identity at a time.
Each stored identity is keyed by a service ID (an ACI, or a PNI). The
choice depends only on whether that service ID is the "unknown" one,
meaning the all-zero UUID `00000000-0000-0000-0000-000000000000`.

### 3.1 Normal case: service ID is known (version 2)

- **Version**: 2.
- **Local identifier**: the account's own **ACI**, as its 16-byte binary
  UUID (the UUID's 16 bytes in the usual big-endian order, no prefix).
- **Local key**: the account's **ACI** identity public key (33 bytes).
- **Remote identifier**: the service ID under which the identity is
  stored, in libsignal's "service-ID binary" form:
  - ACI: the 16 UUID bytes, no prefix;
  - PNI: one byte `0x01` followed by the 16 UUID bytes (17 bytes total).
- **Remote key**: the stored identity key (33 bytes).

Consequences:

- The local side is always ACI + ACI identity key, even when the remote
  identity is a PNI identity. signal-cli never uses its own PNI or its
  own PNI identity key here.
- An identity stored under a PNI gets a safety number (it is **not**
  null), but the number differs from the one computed for the same key
  under that contact's ACI, because the remote identifier bytes differ
  (vector `v2_aci_to_pni`).
- No phone number is involved when the service ID is known, even if
  the contact's number is known.

### 3.2 Fallback: service ID is the unknown (all-zero) UUID (version 1)

signal-cli looks up the recipient for that service ID and its phone
number.

- If the recipient has no phone number, there is **no safety number**:
  both `safetyNumber` and `scannableSafetyNumber` are null (§5).
- Otherwise:
  - **Version**: 1.
  - **Local identifier**: the account's own phone number in E.164 form
    (`+` and digits), as UTF-8 bytes.
  - **Local key**: the account's ACI identity public key.
  - **Remote identifier**: the recipient's E.164 number as UTF-8 bytes.
  - **Remote key**: the stored identity key.

In practice, signal-cli 0.14.7 stores identities only under real service
IDs, so this branch is close to unreachable. It is specified because the
code path exists and because libsignal's published vectors (F3) use
E.164 identifiers.

## 4. Scannable safety number

### 4.1 Protobuf layout

The scannable form is a protobuf (proto2 syntax) message, here called
CombinedFingerprints. Field tables (names are descriptive):

CombinedFingerprints:

| Field | Number | Wire type | Type | Cardinality | Meaning |
|-------|--------|-----------|------|-------------|---------|
| version | 1 | varint | uint32 | optional | 1 or 2, per §3 |
| local_fingerprint | 2 | length-delimited | LogicalFingerprint | optional | the computing party's side |
| remote_fingerprint | 3 | length-delimited | LogicalFingerprint | optional | the other party's side |

LogicalFingerprint:

| Field | Number | Wire type | Type | Cardinality | Meaning |
|-------|--------|-----------|------|-------------|---------|
| content | 1 | length-delimited | bytes | optional | first 32 bytes of that side's per-side digest (§2.1) |

Field number 2 inside LogicalFingerprint is reserved (an old
"identifier" field, now unused). Never emit it.

### 4.2 Encoding produced

- `content` is bytes 0–31 of the per-side digest. Note that the 30-digit
  string of §2.2 is computed from bytes 0–29 of the same digest.
- The encoder writes all three top-level fields, in field-number order:
  version, then local, then remote. Each LogicalFingerprint holds just
  its `content`.
- With 32-byte contents and a version below 128, the result is always
  exactly 74 bytes:
  `08 VV 12 22 0a 20 <local 32 bytes> 1a 22 0a 20 <remote 32 bytes>`,
  where `VV` is `01` or `02`.
- Unlike the 60-digit number, the scannable form is **not** symmetric:
  "local" is whoever computed it. Alice's code is Bob's code with the
  two 32-byte halves swapped (vectors show both sides).
- The version **is** part of the encoding, so the v1 and v2 encodings of
  the same digests differ in byte 1 only.

### 4.3 Comparison (verifying a scanned code)

To check a code scanned from the other party ("theirs") against our own
fingerprint ("ours"):

1. Decode theirs as CombinedFingerprints. If decoding fails, the result
   is a parsing error. Unknown fields are ignored.
2. Their version is taken as 0 if the field is absent. If it differs
   from our version, the result is a version-mismatch error.
3. Their `local_fingerprint.content` must be present; otherwise a
   parsing error ("missing their local fingerprint"). Same for their
   `remote_fingerprint.content` ("missing their remote fingerprint").
4. The code matches only if their **local** content equals our
   **remote** content and their **remote** content equals our **local**
   content. Comparisons are constant-time byte-string equality. Contents
   of different lengths are unequal.

Consequence: our own code compared with itself does **not** match
(unless both sides happen to be identical, as for the self-identity in
§5.3). The code that matches is the one the *other* party's device
shows.

## 5. What `listIdentities` shows

### 5.1 Which identities are listed

Without `-n`, every identity in the store is considered. For each one,
signal-cli resolves the recipient for the identity's service ID and its
current address. If that address has a service ID and it is **not** the
identity's service ID, the identity is skipped entirely (no line, no
JSON element). The usual case: an identity stored under a contact's PNI
after the contact's ACI became known. The contact's address now carries
the ACI, so the PNI-keyed identity is hidden. A PNI identity is shown
only while the recipient's address has no ACI.

With `-n RECIPIENT`, the recipient is resolved (an unregistered or
unresolvable recipient gives an empty list, not an error), its address's
service ID (the ACI if known, otherwise the PNI) is looked up in the
identity store, and at most one identity is listed.

### 5.2 Fields derived from the key and the safety number

For each listed identity:

| Output | Content |
|--------|---------|
| Fingerprint (plain) / `fingerprint` (JSON) | the stored 33-byte identity key, each byte as two lowercase hex digits, bytes separated by one ASCII space (so 98 characters, starting `05 `) |
| Safety Number (plain) / `safetyNumber` (JSON) | the 60-digit number of §2.3, formatted as below, or null |
| `scannableSafetyNumber` (JSON only) | standard base64 (RFC 4648 alphabet with `+` and `/`, with `=` padding, no line breaks) of the 74-byte encoding of §4.2, computed from **this account's** perspective (local = own ACI side, remote = the stored identity's side), or null. For a v2 code this is always 100 characters ending in one `=`. |

Safety-number formatting: the digit string is cut into 12 equal parts of
(length ÷ 12, rounded down) characters each, joined by single ASCII
spaces. For the 60-digit number that is 12 groups of 5 digits, 71
characters in total, for example
`30035 44776 92869 39689 28698 76765 45825 75691 62576 84344 09180 79131`.
There are no other separators and no line breaks. The plain-text line
prints the literal word `null` when there is no safety number. JSON
writes a JSON `null`. The plain-text output does not include the
scannable form.

### 5.3 When values are null, and special identities

- `safetyNumber` and `scannableSafetyNumber` are null together, and
  only in the §3.2 case: the identity's service ID is the all-zero UUID
  and the recipient has no phone number. Nothing else makes them null.
  In particular, PNI identities (when listed) and contacts with no known
  phone number still get both values (version 2).
- The account's **own ACI identity** is stored at registration or
  linking (trust level TRUSTED_VERIFIED) and is listed like any other.
  Its safety number has the same 30 digits twice, and its scannable code
  has identical local and remote halves (vector `v2_self`). The own
  **PNI** identity is also stored, but it is hidden by the rule in §5.1
  because the own address carries the ACI.

## 6. `trust -v`: accepted inputs and how they are matched

The same rules apply to the JSON-RPC `trust` method's
`verifiedSafetyNumber`.

### 6.1 Classifying the input

1. Remove every ASCII space (U+0020) anywhere in the string. No other
   whitespace is removed: tabs, newlines, non-breaking spaces and so on
   stay in place.
2. Classify by the remaining length, counted in UTF-16 code units:
   - **66** → legacy hex fingerprint. The string is lowercased
     (locale-independent) and read as 33 bytes, two characters per byte,
     high nibble first. **This step never fails.** A character that is
     not a hex digit does not raise an error. It behaves as the value
     −1 in Java's `Character.digit`, which gives these results:
     - an invalid **low** (second) character makes the byte `0xff`;
     - an invalid **high** (first) character with a valid low
       character `d` makes the byte `0xf0 | d`.

     Example: `zz06…d8g8` decodes to `ff06…d8f8` (vector). Unicode
     decimal digits from other scripts count as their digit value.
     Upper- and lowercase hex are both accepted.
   - **60** → numeric safety number. The 60 characters are kept as they
     are, with no check for digits.
   - **any other length** (including 0) → base64 of a scannable code.
     It is decoded with Java's basic base64 decoder:
     - only the standard alphabet (`A–Z a–z 0–9 + /`) is accepted;
     - `=` padding is optional, but if present it must be correct;
     - the URL-safe characters `-` and `_` are rejected, and so are
       tabs, newlines and any other character;
     - a final group of a single character is rejected;
     - the empty string decodes to zero bytes and is accepted at this
       stage.

     Short strings can decode successfully: `abc` gives `69 b7`, and
     `123` gives `d7 6d`.
3. Only a base64 decoding failure produces the format error, with exact
   text:
   `Safety number has invalid format, either specify the old hex fingerprint or the new safety number`
   (exit 1 on the CLI). Length 66 and length 60 inputs never produce it.

Classification is purely by length. A 60-character string that happens
to be valid base64, or a 66-character string that is really a base64
code, is still treated as numeric or hex.

### 6.2 Matching against the recipient's identity

The recipient is resolved to an address. Its service ID is the ACI if
known, otherwise the PNI. If the address has no service ID, or there is
no stored identity under that service ID, the result is "no match". If
there is an identity, the check depends on the input type:

- **Hex fingerprint**: match if the 33 decoded bytes equal the stored
  33-byte serialized identity key. This is the same value as the
  `fingerprint` field of `listIdentities` with the spaces removed.
- **Numeric**: compute the 60-digit safety number for that identity
  exactly as in §3 (same version and identifier choice) and compare it
  with the input for exact string equality. The input's spaces were
  already removed, so the 12-group form from `listIdentities` and the
  bare 60 digits are both accepted. If the computed value is null
  (§3.2), there is no match.
- **Scannable**: compute this account's own scannable fingerprint for
  that identity (§3, §4.2) and run the comparison of §4.3 with the
  decoded input as "theirs". A version mismatch or a parsing error
  counts as "no match"; it does not produce a different message.
  Because of §4.3, the accepted code is the one the **contact's**
  device shows (local and remote halves swapped relative to our
  `scannableSafetyNumber`). Pasting signal-cli's own
  `scannableSafetyNumber` back into `trust -v` does **not** match,
  except for the self-identity, whose two halves are identical. The
  contact's version must also be equal to ours (2 in the normal case).

On a match, the identity's trust level becomes TRUSTED_VERIFIED, and
signal-cli sends a "verified" sync message to its linked devices (see
`spec/service/sync.md`). A recipient that cannot be resolved raises the
not-registered error from `spec/surface/cli/trust.md`. On no match, the
error is, exactly:
`Failed to set the trust for this number, make sure the number and the fingerprint/safety number are correct.`
(exit 1).

### 6.3 Corrections to the surface specs

- `spec/surface/cli/trust.md` and `spec/surface/jsonrpc/methods/trust.md`
  say that a "bad hex" value gives the format error. It cannot: length-66
  input always decodes (§6.1). Only base64 failures produce that error.
- Inputs of other lengths that decode as base64, such as `123`, are
  accepted at the format stage and then fail matching. The result is the
  "Failed to set the trust…" error, not the format error.

## 7. Test vectors

`testdata/vectors/fingerprint.json` contains:

- `published`: libsignal's fixed vectors (F1 tests, F3), with
  identifiers `+14152222222` / `+14153333333` and the "Alice" and "Bob"
  keys. The 60-digit number is the same for versions 1 and 2. The
  scannable encodings are given for both versions and both
  perspectives, plus the encoding-only vector (contents `0x12`×32 and
  `0xBA`×32, version 2).
- `per_side`: per-side 64-byte digests and 30-digit strings for each
  identifier/key pair used, so implementations can test §2.1 and §2.2
  separately.
- `signal_cli_cases`: fingerprints computed through signal-cli's own
  path (version 2 with ACI binary identifiers; ACI to PNI; version 1
  with E.164; self). Each gives the 60-digit string, the
  `listIdentities` grouped form, the scannable hex and the base64 for
  both perspectives.
- `scannable_compare`: comparison outcomes (match, self-compare,
  version mismatch, parse failures).
- `trust_input_parsing`: how `trust -v` strings are classified and
  decoded.

All UUIDs in the jar-generated cases are made up. All phone numbers are
fictional (+1 555-01xx), except libsignal's published identifiers
`+14152222222` and `+14153333333`, which are copied as-is.
