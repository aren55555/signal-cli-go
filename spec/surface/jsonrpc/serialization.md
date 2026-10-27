# Output serialization: collection order and JSON character escaping

Scope: (1) the element order of set-valued outputs, mainly the `listGroups` member arrays, and
(2) how JSON text is escaped and encoded on each output path. Key order inside objects is
covered elsewhere (`notifications.md`, the per-method files). Transport framing is in
`transports.md`.

## Sources

signal-cli v0.14.7 (tag `v0.14.7`, https://github.com/AsamK/signal-cli/tree/v0.14.7):

- `src/main/java/org/asamk/signal/commands/ListGroupsCommand.java` (S1)
- `lib/src/main/java/org/asamk/signal/manager/api/Group.java`, `api/GroupMember.java`, `api/RecipientAddress.java` (S2)
- `lib/src/main/java/org/asamk/signal/manager/storage/groups/GroupInfoV2.java`, `GroupInfoV1.java`,
  `GroupInfo.java`, `GroupMemberInfoV2.java` (S3)
- `lib/src/main/java/org/asamk/signal/manager/storage/recipients/RecipientId.java`, `RecipientAddress.java` (`toApiRecipientAddress`),
  `RecipientStore.java` (`getRecipients`, profile capabilities parsing) (S4)
- `lib/src/main/java/org/asamk/signal/manager/internal/ManagerImpl.java` (`getGroups`, `getUserStatus`,
  `getUsernameStatus`, `getRecipients`, `getIdentities`) (S5)
- `src/main/java/org/asamk/signal/commands/GetUserStatusCommand.java`, `ListContactsCommand.java`,
  `ListIdentitiesCommand.java`, `ListDevicesCommand.java`, `UpdateGroupCommand.java` (S6)
- `src/main/java/org/asamk/signal/util/Util.java` (`createJsonObjectMapper`), `output/JsonWriterImpl.java`,
  `App.java` (`getOutputWriter`), `util/IOUtils.java` (`getConsoleCharset`), `jsonrpc/SocketHandler.java`,
  `commands/JsonRpcDispatcherCommand.java`, `commands/DaemonCommand.java` (`addDefaultReceiveHandler`),
  `http/HttpServerHandler.java`, `http/ServerSentEventSender.java`, `json/JsonReceiveMessageHandler.java` (S7)
- `gradle/libs.versions.toml` (jackson-databind 2.22.1), `build.gradle.kts` (Java 25); the release
  distribution bundles `jackson-core-2.22.1.jar` and `jackson-databind-2.22.1.jar` (S8)

jackson-core 2.22.1 (tag `jackson-core-2.22.1`, https://github.com/FasterXML/jackson-core/tree/jackson-core-2.22.1):
`src/main/java/com/fasterxml/jackson/core/json/UTF8JsonGenerator.java`, `json/WriterBasedJsonGenerator.java`,
`json/JsonWriteFeature.java`, `io/CharTypes.java`, `JsonFactory.java` (J1).
jackson-databind 2.22.1 (tag `jackson-databind-2.22.1`): `ObjectMapper.java` (`writeValue(Writer,…)`,
`writeValueAsString`, `writeValueAsBytes`) (J2).

OpenJDK 25 (the reference runs on Temurin 25.0.5+8; sources from that JDK's `lib/src.zip`, equivalent to
https://github.com/openjdk/jdk25u at `jdk-25.0.5+8`): `java/util/HashMap.java`, `HashSet.java`,
`stream/Collectors.java`, `Optional.java`, `Objects.java`, `UUID.java`, `lang/Boolean.java`, `lang/Long.java`,
`lang/Enum.java`, `lang/runtime/ObjectMethods.java` (record `hashCode`), `io/Console.java`, `nio/channels/Channels.java`,
`sun/nio/cs/StreamEncoder.java` (K1).

Black-box runs of the 0.14.7 binary (zero accounts, scratch config, loopback only), 2026-10-27 (O1):
`jsonRpc` on stdio with `LANG` unset and no TTY; `daemon --http 127.0.0.1:P --tcp 127.0.0.1:Q` with
requests sent by `curl` and a bash `/dev/tcp` connection. Results are quoted below.

---

## Part 1: order of set-valued outputs

### 1.1 Common rule: how a Java `HashSet` / `HashMap` iterates (K1)

All hash-ordered outputs below come from `java.util.HashSet` (backed by `HashMap`) built by
`Collectors.toSet()`, by `Collectors.toMap(k, v)` (a `HashMap`), or by `new HashSet<>(collection)`.
Elements are added one at a time in the iteration order of the source.

- **Spread hash.** For element hash `h` (32-bit signed int), `s = h XOR (h >>> 16)` (logical shift).
  A null key would get 0 (does not occur here).
- **Bucket index.** `s AND (cap − 1)`, where `cap` is the table capacity (a power of two).
- **Capacity.** Empty table is created at 16 on first insert (`toSet`, `toMap`), or for
  `new HashSet<>(c)` at the smallest power of two ≥ ceil(max(|c|,12) / 0.75). Threshold is 0.75·cap. After an
  insert makes `size > threshold` the table doubles. In every case used here the final capacity is the
  smallest power of two `cap ≥ 16` with `n ≤ 0.75·cap`, for `n` distinct elements: 16 for n ≤ 12, 32 for
  n ≤ 24, 64 for n ≤ 48, 128 for n ≤ 96, and so on.
- **Within a bucket.** New nodes go at the tail of the bucket's list. Resizing splits each list into
  "low" and "high" lists and keeps relative order. A duplicate (by `equals`) is not inserted, so the first
  occurrence wins.
- **Iteration.** Buckets in ascending index; inside a bucket, list order.
- **Result.** The iteration order is a **stable sort of the distinct elements, in insertion order, by final
  bucket index**.
- **Tree bins (edge case).** A bucket is converted to a red-black tree only when it would reach 8 or more
  nodes *and* `cap ≥ 64` (with a smaller table it resizes instead). Iteration then still follows the
  node `next` links, but a node added to an existing tree is linked directly after its tree parent, not at
  the tail. The stable-sort rule then no longer holds for that bucket. This needs at least 9 elements with
  the same final bucket index in a set of at least 49 elements, which is practically unreachable for
  these outputs. An implementation may ignore it.

Hash functions needed (K1):

- `String.hashCode`: `h = 0`; for each UTF-16 code unit `u` in order, `h = 31·h + u` (mod 2³², signed).
  Non-BMP characters count as two code units (their surrogates). Empty string → 0.
- `Boolean.hashCode`: `true` → 1231, `false` → 1237.
- `Long.hashCode(v)`: `(int)(v XOR (v >>> 32))`.
- `Optional.hashCode`: hash of the contained value, or 0 if empty.
- `Objects.hashCode(x)`: `x.hashCode()`, or 0 for null.
- `Objects.hash(a)` with one argument: `31·1 + hash(a)` (the `Arrays.hashCode` rule: start at 1, then
  `r = 31·r + hash(e)` for each element).
- **Record `hashCode`** (Java records with generated `hashCode`, JDK 25 `ObjectMethods`): `r = 0`; for each
  component in declaration order, `r = 31·r + hash(component)`. Reference components use
  `Objects.hashCode` (null → 0); `boolean` components use `Boolean.hashCode`. All arithmetic is 32-bit
  wrapping. Note: the start value is 0, unlike `Arrays.hashCode`.
- `UUID.hashCode` (not used directly in group output, given for completeness):
  `Long.hashCode(msb XOR lsb)`.
- `Enum.hashCode`: identity hash, **not deterministic across JVM runs**.

### 1.2 `listGroups`: data flow and collection types (S1–S5)

`Manager.getGroups()` returns a `List<Group>` (group order is covered in `../cli/listGroups.md`). Each
`Group` (record, S2) is built from the stored group (`GroupInfoV2`, or legacy `GroupInfoV1`) like this:

| `Group` field | type | built from (insertion order) |
|---|---|---|
| `members` | `HashSet<GroupMember>` | `GroupInfo.getMembers()`. For V2 this is a `List` in the order of the stored decrypted group's `members` protobuf list. For V1 it is a `List` made by iterating the stored `HashSet<RecipientId>`. |
| `pendingMembers` | `HashSet<api.RecipientAddress>` | iteration of `GroupInfoV2.getPendingMembers()`, a `HashSet<RecipientId>` built from the protobuf `pendingMembers` list (service id → recipient id). Each id is resolved to an address. Empty for V1. |
| `requestingMembers` | `HashSet<api.RecipientAddress>` | same, from the protobuf `requestingMembers` list (by ACI). Empty for V1. |
| `bannedMembers` | `HashSet<api.RecipientAddress>` | same, from the protobuf `bannedMembers` list (by service id). Empty for V1. |

Element types and their hashes:

- `GroupMember` record, components in order: `recipientAddress` (api `RecipientAddress`), `isAdmin`
  (boolean; true iff the protobuf role is ADMINISTRATOR), `labelEmoji` (String, null when the protobuf field is
  empty), `label` (String, null when empty).
- api `RecipientAddress` record, components in order: `aci`, `pni`, `number`, `username`, each an
  `Optional<String>`. Their values come from the local recipient store:
  - `aci`: the ACI as a lowercase canonical UUID string.
  - `pni`: `PNI:` followed by the lowercase UUID (see `../../service/keys.md` for the ServiceId string forms).
  - `number`: the E.164 string.
  - `username`: the username string.
  Absent values are empty Optionals (hash 0).
- `RecipientId` (S4) is a plain class: `hashCode = Objects.hash(id)` = `31 + Long.hashCode(id)`, where `id` is
  the recipient's row id in the local store, after any merge redirection.

`ListGroupsCommand` then builds **new** sets for output (S1). Every one is a `Collectors.toSet()` `HashSet`,
filled by iterating the `Group` sets above.

JSON output (CLI `-o json` and JSON-RPC `listGroups`; the same command code):

| JSON key | Java type | element | filled by iterating |
|---|---|---|---|
| `members` | `HashSet<JsonGroupMember>` | record `(number, uuid, isAdmin, labelEmoji, label)` | `Group.members` |
| `pendingMembers` | `HashSet<JsonGroupMemberAddress>` | record `(number, uuid)` | `Group.pendingMembers` |
| `requestingMembers` | `HashSet<JsonGroupMemberAddress>` | record `(number, uuid)` | `Group.requestingMembers` |
| `admins` | `HashSet<JsonGroupMemberAddress>` | record `(number, uuid)` | an intermediate `HashSet<api.RecipientAddress>`, built by iterating `Group.members`, keeping those with `isAdmin` and taking their `recipientAddress` |
| `banned` | `HashSet<JsonGroupMemberAddress>` | record `(number, uuid)` | `Group.bannedMembers` |

- `number` is the address's `number` or null.
- `uuid` is the address's **ACI** as a lowercase UUID string, or null. A member known only by PNI therefore
  gets `uuid: null`.
- The `labelEmoji` and `label` components count in the hash even though JSON leaves them out when null
  (null hashes as 0).
- Because each stage deduplicates with record `equals`, two distinct members that map to equal output
  records collapse into one entry. Example: two pending invitees with no ACI and no number both become
  `(null, null)`.

Plain-text output (`listGroups -d`, S1): every list is a `HashSet<String>` printed in Java collection form
(`[a, b]`, see `../cli/listGroups.md`).

- `Members`: one string per element of `Group.members`, in that iteration order. The string is the legacy
  identifier (number if present, else ACI, else PNI string, else username), then `{ADMIN}` for admins, then
  `(` + emoji + label + `)` when either label part is set.
- `Pending members`, `Requesting members`, `Banned`: the legacy identifier of each element of the
  corresponding `Group` set.

**How to reproduce the order.** Simulate every stage with §1.1:

1. Start from the stored protobuf list order. For pending, requesting and banned members, first build the
   `RecipientId` set and iterate it.
2. Build the `Group` set (`GroupMember` or api `RecipientAddress` hashes).
3. For `admins`, build the intermediate api `RecipientAddress` set.
4. Build the final output set and iterate it.

Earlier stages matter only when two elements share a final bucket. The final order is a stable sort, by
final bucket index, of the order produced by the previous stage. So the bucket index of each final element
fixes the order in most cases, and tie-breaking needs the upstream order.

This explains the observation that `admins` can be `[B, A]` while `members` is `[A, B]` for the same two
people. `members` hashes the 5-component `JsonGroupMember` record (including `isAdmin` 1231/1237).
`admins` hashes the 2-component `JsonGroupMemberAddress` record, so the bucket indices differ.

### 1.3 Other list outputs

- **`getUserStatus`** (S5, S6): hash-ordered but deterministic.
  - Numbers: the recipients are copied into `new HashSet<String>(inputs)`, then `ManagerImpl.getUserStatus`
    builds a `HashMap<String, UserStatus>` keyed by the **raw input string** (not the normalized number) with
    `Collectors.toMap`, iterating that set.
  - Usernames: the same, `HashMap<String, UsernameStatus>` keyed by the raw input username.
  - Output: all number entries in that map's iteration order, then all username entries in theirs (keys
    hashed with `String.hashCode`, §1.1). Duplicate inputs collapse. This makes "unspecified" in
    `../cli/getUserStatus.md` precise.
- **`listContacts`**: list order is the SQLite result order of the recipient query, which has no
  `ORDER BY`. In practice that is ascending row id. It does not depend on hashing.
  - Exception: with `--internal`, the `capabilities` array (and the plain-text `Capabilities:` list) comes
    from a `HashSet<Profile.Capability>` (enum; values `storage`, `storageServiceEncryptionV2Capability`).
    Enum hashes are identity hashes, so with both capabilities present **the order can differ between
    runs of the reference**. Tests must not depend on it. An implementation may choose any fixed order
    (declaration order is the natural choice).
- **`listIdentities`**: SQLite result order of the identity query (no `ORDER BY`), then a `List`. Not hash
  dependent.
- **`listDevices`**: server response order, then a `List`. Not hash dependent.
- **`listAccounts`**: see `../cli/listAccounts.md`. No hash set is involved in the command itself.
- **Hash-ordered object keys** (deterministic, computed with §1.1 on the key strings):
  - The received-message wrapper (`JsonReceiveMessageHandler`, a `HashMap<String,Object>`) has order
    `exception`, `envelope`, `account`, as already stated in `notifications.md`.
  - The `updateGroup` JSON result is also a `HashMap` (S6). Bucket indices at cap 16: `groupId` → 7,
    `results` → 13, `timestamp` → 15. So the emitted key order is **`groupId`, `results`, `timestamp`**,
    for whichever of them are present.

---

## Part 2: JSON escaping and encoding per output path

### 2.1 The two Jackson generators (J1, J2)

Every path uses an `ObjectMapper` from `Util.createJsonObjectMapper()`. It changes visibility and disables
auto-close, and leaves every escaping-related feature at its default. Defaults in jackson-core 2.22.1:

- `ESCAPE_NON_ASCII` off.
- `ESCAPE_FORWARD_SLASHES` off.
- `WRITE_HEX_UPPER_CASE` on.
- `COMBINE_UNICODE_SURROGATES_IN_UTF8` off (since 2.18).
- No custom character escapes.

Which `ObjectMapper` method a path uses decides the generator:

- `writeValue(Writer, …)` and `writeValueAsString(…)` → **WriterBasedJsonGenerator** (produces
  characters; a separate Java `Writer` encodes them to bytes).
- `writeValueAsBytes(…)` → **UTF8JsonGenerator** (produces UTF-8 bytes itself).

Escaping shared by both generators (string values and object keys):

- `"` becomes `\"` and `\` becomes `\\`.
- U+0008 `\b`, U+0009 `\t`, U+000A `\n`, U+000C `\f`, U+000D `\r`.
- Every other code point U+0000–U+001F becomes `\u00XX` with **uppercase** hex (for example `\u001F`).
- `/` and U+007F are written unescaped. U+2028 and U+2029 are written unescaped.

Where they differ:

| | WriterBasedJsonGenerator | UTF8JsonGenerator |
|---|---|---|
| U+0080–U+FFFF other than surrogates | raw character | raw UTF-8 (2 or 3 bytes) |
| UTF-16 code unit U+D800–U+DFFF (paired **or** lone) | raw code unit, passed on to the `Writer` | **always** `\uXXXX` with uppercase hex, one escape per code unit |
| so a non-BMP character, e.g. U+1F6E0 | raw (the `Writer` encodes it, see 2.2) | `\uD83D\uDEE0` (12 ASCII bytes) |
| lone surrogate, e.g. U+D800 | raw (the `Writer` then replaces it or fails, see 2.2) | `\uD800` |

### 2.2 Per path (S7, K1)

| Path | Code path | Generator | Byte encoding of the generator's characters |
|---|---|---|---|
| CLI `-o json` (every command), and the `jsonRpc` command on **stdio** | `JsonWriterImpl` on `BufferedWriter(OutputStreamWriter(System.out, consoleCharset))` | Writer-based | console charset, with REPLACE on errors |
| `daemon` stdout printer for received messages (`-o json`) | same writer as above (`DaemonCommand` passes the `App` output writer to `JsonReceiveMessageHandler`) | Writer-based | console charset, with REPLACE |
| `daemon` stdout printer in plain-text mode | `PlainTextWriterImpl` on the same writer (no JSON) | — | console charset, with REPLACE |
| JSON-RPC over **UNIX socket** and **TCP** | `JsonWriterImpl` on `Channels.newWriter(channel, UTF_8)` | Writer-based | UTF-8, **REPORT** on errors |
| HTTP `POST /api/v1/rpc` response body | `objectMapper.writeValueAsBytes(response)` | **UTF8** | produced directly by the generator |
| HTTP `GET /api/v1/events` SSE `data:` line | `objectMapper.writeValueAsString(…)`, written by `ServerSentEventSender` via `BufferedWriter(OutputStreamWriter(out, UTF_8))` | Writer-based | UTF-8, with REPLACE |

Notes:

- Line endings and framing:
  - The writer-based paths end each JSON value with `System.lineSeparator()` (LF on Linux) and flush.
  - The HTTP RPC body has **no** trailing newline. Its `Content-Length` equals the byte length.
  - SSE framing is in `transports.md`. JSON escaping guarantees the `data:` value contains no raw newline.
- **Console charset** (`IOUtils.getConsoleCharset`): `System.console()` if non-null, else
  `Charset.defaultCharset()`.
  - On JDK 25 the default console provider is the one in `java.base`. That gives a `Console` only when
    **both stdin and stdout are terminals**. Its charset is the `stdout.encoding` property, which depends on
    the locale (for example US-ASCII under `LANG` unset/C).
  - Otherwise the default charset is used. That is UTF-8 on JDK 18+ regardless of locale, because the
    launcher passes no `file.encoding`.
  - So output that is **not on a TTY is always UTF-8**. This differs from argparse help text, which goes
    through `System.out`'s own `stdout.encoding` and shows `?` with `LANG` unset (see `../cli/daemon.md`).
- **REPLACE** (`OutputStreamWriter(…, Charset)`): characters the encoder cannot handle become the
  replacement byte `?` (0x3F).
  - With UTF-8, a lone surrogate becomes one `?`.
  - With a non-UTF-8 console charset, any unmappable character becomes one `?`. A valid surrogate pair
    (one non-BMP character) yields a single `?`.
  - stdin on stdio is decoded with the same console charset (`transports.md`).
- **REPORT** (sockets): `Channels.newWriter(ch, Charset)` uses a fresh encoder whose error action is
  REPORT. A lone surrogate makes the write fail partway. The stdio/HTTP rows below show what the binary
  does.

Observed (O1). Request ids are echoed in the error response for an unknown method:

- stdio, no TTY, `LANG` unset:
  - id `"’🛠"` comes back as raw UTF-8 bytes `E2 80 99 F0 9F 9B A0`.
  - id `"a\uD800b"` comes back as `"a?b"`.
  - id `"\u001f\u007f/"` + U+2028 comes back as `"\u001F`, raw 0x7F, `/`, raw `E2 80 A8`, then `"`.
- HTTP `/api/v1/rpc`:
  - id `"’🛠"` comes back as raw `E2 80 99`, then `\uD83D\uDEE0`.
  - id `"a\uD800b\uDC00"` comes back as `"a\uD800b\uDC00"`.
  - control characters: `\u001F`, raw 0x7F, `/`. `é` is raw `C3 A9`.
- TCP:
  - id `"’🛠"` comes back as raw UTF-8 `E2 80 99 F0 9F 9B A0`.
  - id `"a\uD800b"`: the response stops after `…,"id":"a`. The rest of that response line (including its
    newline) is never written, and the connection stays open. The next response follows directly on the
    same line, so the stream reads `…"id":"a{"jsonrpc":"2.0",…}`.
  - The UNIX socket uses the same code (`SocketHandler`), so the same behaviour is expected (not separately
    observed).

### 2.3 Implementation guidance (derived from the above)

- **HTTP `/api/v1/rpc` responses.** Escape every UTF-16 surrogate code unit as `\uXXXX` with uppercase hex.
  A non-BMP code point therefore becomes its two surrogates. Write all other non-ASCII characters as raw
  UTF-8.
- **Every other JSON path** (CLI `-o json`, stdio, socket, TCP, SSE, daemon stdout): write non-ASCII
  characters, including non-BMP ones, as raw UTF-8. When CLI or stdio output goes to a TTY with a non-UTF-8
  locale charset, write each unmappable code point as `?`.
- **Control characters and specials, all paths:** use the short escapes `\b \t \n \f \r \" \\`, and
  `\u00XX` with uppercase hex for the other C0 controls. Do not escape `/`, DEL, U+2028 or U+2029.
- **Lone surrogates.** A Go `string` cannot carry them the way a Java `String` can. In the reference they
  only arise from JSON input escapes such as `\uD800` echoed back. Treat byte-exact reproduction as best
  effort:
  - HTTP: `\uD800`.
  - stdio, CLI and SSE: `?`.
  - socket/TCP: a truncated line. A reimplementation need not copy this corruption.
