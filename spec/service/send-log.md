# Message send log (resend log)

Spec role: prose and tables only. This file describes signal-cli 0.14.7's
**message send log** ("MSL", also called the resend log). It keeps a copy of
recently sent `Content` messages so signal-cli can answer a peer's retry
receipt (DecryptionErrorMessage) by sending the same message again.

Other specs mention the log but leave out its details:

- `pipe.md` §7.2 (building and sending retry receipts) and §7.3 (receiving
  one). This file refines §7.3 and corrects it in one place (§6.2 below).
- `content-features.md` (send-log rows in the command table; null-message
  trigger 2), `content.md` §"resend log", `groups.md` §12,
  `stories.md` §3.7 (sender-key shared-with resets), `sync.md` §3 item 7.

## Sources

| Ref | What | URL @ version |
|-----|------|---------------|
| M1 | signal-cli | https://github.com/AsamK/signal-cli @ tag `v0.14.7` = commit `b01b6b370dc063599a1a2b9fde0f5ff4e2d78fe8`: `lib/src/main/java/org/asamk/signal/manager/storage/sendLog/{MessageSendLogStore,MessageSendLogEntry}.java`, `.../manager/storage/{SignalAccount,AccountDatabase,Database}.java`, `.../manager/storage/sessions/SessionStore.java`, `.../manager/helper/{SendHelper,IncomingMessageHandler,ReceiveHelper,CallManager}.java`, `.../manager/actions/{ResendMessageAction,RenewSessionAction}.java`, `.../manager/internal/ManagerImpl.java` (`sendRemoteDeleteMessage`), `.../manager/Settings.java`, `src/main/java/org/asamk/signal/{App,GlobalConfig,ConfigLoader}.java` |
| M2 | signal-service-java (Turasa fork) | https://github.com/Turasa/libsignal-service-java @ tag `v2.15.3_unofficial_151` = commit `66956cb4a0394d50cee3590209baf58312084a44`: `lib/libsignal-service/src/main/java/org/whispersystems/signalservice/api/SignalServiceMessageSender.java` (`resendContent`, `sendContent`, the single- and multi-recipient `sendMessage`, `sendGroupMessage`, `transformGroupResponseToMessageResults`, `getEncryptedMessages`, `sendCallMessage`, `sendReceipt`, `sendRetryReceipt`), `.../api/messages/{SendMessageResult.java,SignalServiceEnvelope.kt,SignalServiceCallMessage.java}`, `.../api/crypto/ContentHint.java`, `core/models-jvm/src/main/java/org/signal/core/models/ServiceId.kt` |
| M3 | libsignal | https://github.com/signalapp/libsignal @ tag `v0.99.1` = commit `97801d22dcf9f5bf714f7b8fa3212cdc973ae1c8`: `rust/protocol/src/state/session.rs` (`current_ratchet_key_matches`) |
| M4 | signal-cli, for §4.2.1 | https://github.com/AsamK/signal-cli @ tag `v0.14.7` = commit `b01b6b370dc063599a1a2b9fde0f5ff4e2d78fe8`: `lib/src/main/java/org/asamk/signal/manager/helper/SendHelper.java` (`resendMessage`), `.../manager/actions/ResendMessageAction.java`, `.../manager/helper/IncomingMessageHandler.java` (`handleMessage`, `handleDecryptionErrorMessage`, `getSender`, `getGroupContext`, `isMessageBlocked`, `isNotAllowedToSendToGroup`), `.../manager/helper/GroupHelper.java` (`getGroup`), `.../manager/storage/groups/{GroupInfo,GroupInfoV1,GroupInfoV2,GroupMemberInfo,GroupMemberInfoV1,GroupMemberInfoV2}.java`, `.../manager/storage/recipients/{RecipientId,RecipientResolver,RecipientStore}.java` |
| M5 | signal-service-java (Turasa fork), for §4.2.1 | https://github.com/Turasa/libsignal-service-java @ tag `v2.15.3_unofficial_151` = commit `66956cb4a0394d50cee3590209baf58312084a44`: `lib/libsignal-service/src/main/java/org/whispersystems/signalservice/api/crypto/SignalServiceCipher.java` (plaintext-content envelopes, metadata group id) |

Everything below was read from these sources. Nothing was observed on the
network.

---

## 1. What is stored

### 1.1 Logical model

One **log entry** is one sent `Content` plus the set of (recipient service
ID, device ID) pairs it was delivered to. An entry has:

| Field | Meaning |
|-------|---------|
| `timestamp` | The send timestamp: the envelope timestamp the message went out with. For data messages this equals `DataMessage.timestamp`. For receipts it is the receipt envelope's timestamp (see §2). |
| `content` | The **plaintext `Content` protobuf** exactly as handed to encryption (not padded, not encrypted), serialized. It includes everything that was in that Content at send time, for example a PNI signature (field 10) if one was added (`content-features.md` §4). It does **not** include padding or any sender-key distribution message that went out separately. |
| `content_hint` | The content hint the send used, as its integer: 0 DEFAULT, 1 RESENDABLE, 2 IMPLICIT. When read back, an unknown integer is treated as DEFAULT. |
| `urgent` | The `urgent` flag the send used (boolean). |
| `group_id` | Optional bytes, **derived from the Content** when the entry is written (not taken from the send call): if `Content.dataMessage` is present and has a v1 group context with an id, that id. Otherwise, if it has a `groupV2` context with a master key, the 32-byte v2 group identifier derived from that master key (`groups.md`). Otherwise absent. So **edits never get a group id**: their Content carries `editMessage`, not `dataMessage`, even when the edit went to a group. If the master key cannot be parsed, the group id is absent and a warning `Failed to parse groupId id from content` is logged. |
| recipients | Rows of (`address`, `device_id`). `address` is the recipient's service-ID string as it was used for the send: a plain lowercase UUID for an ACI, `PNI:` + UUID for a PNI. `device_id` is each device ID the message was encrypted for in the final successful attempt (after any 409/410 device-list corrections). |

There is no "sent at" column apart from `timestamp`. There is no column for
the sealed-sender decision, the envelope type, `online`, `story`, or the
destination identity (ACI or PNI) used.

### 1.2 Tables

The log lives in the account's SQLite database (`account.db`, opened with
foreign keys enabled). There are two `STRICT` tables:

**`message_send_log_content`**: one row per entry.

| Column | Type | Notes |
|--------|------|-------|
| `_id` | INTEGER PRIMARY KEY | entry id |
| `group_id` | BLOB, nullable | §1.1 |
| `timestamp` | INTEGER NOT NULL | ms since epoch |
| `content` | BLOB NOT NULL | serialized Content |
| `content_hint` | INTEGER NOT NULL | 0/1/2 |
| `urgent` | INTEGER NOT NULL | 0/1 |

Index on `timestamp`.

**`message_send_log`**: one row per (entry, recipient service ID, device).

| Column | Type | Notes |
|--------|------|-------|
| `_id` | INTEGER PRIMARY KEY | |
| `content_id` | INTEGER NOT NULL | references `message_send_log_content._id`, **ON DELETE CASCADE** |
| `address` | TEXT NOT NULL | service-ID string |
| `device_id` | INTEGER NOT NULL | |

Indexes on (`address`, `device_id`, `content_id`) and on (`content_id`).
There is no uniqueness constraint, so duplicate recipient rows are possible
(and do occur, see §2.3).

Schema history, in case old databases are read: older layouts keyed
recipient rows by a recipient id and later by a `uuid` BLOB. An account
database migration rewrote them to the `address` text form, dropping rows
whose address could not be resolved. The `urgent` column was added later
with default TRUE for existing rows.

### 1.3 One send, several recipients and devices

- A **1:1 send** that succeeds makes one content row and one recipient row
  per device ID it was encrypted for, all with the same `address`.
- A **group send** (sender key, fan-out, or both mixed in one send) makes
  **one** content row for the whole send. The first successful partial
  result creates it. Every later successful partial result (fan-out
  recipients complete one by one, while a sender-key batch reports all its
  recipients at once) appends recipient rows to that same content row. The
  stored Content is the one from the first successful result. All results
  of one group send carry the same Content, because the SKDM is never part
  of the logged Content.
- Failed results (unregistered, network failure, identity failure, rate
  limit, …) add nothing.
- If a send "succeeds" with an empty device list (for example the
  "no devices with valid sessions" shortcut), a content row with no
  recipient rows can be created. Nothing can match it. It disappears at the
  next orphan cleanup (§3.2) or by age (§3.3).

---

## 2. Which sends are logged

### 2.1 Logged

Only sends that pass through these paths are written, and only while
`--disable-send-log` is off (§2.4):

| Send | Path | `timestamp` | Hint / urgent stored |
|------|------|-------------|---------------------|
| 1:1 DataMessage: `send`, reactions, remote deletes, polls, pins/unpins, payment notifications, timer updates (`updateContact --expiration`), and every other command that uses the common 1:1 data-message path (`content-features.md` §1.3) | 1:1 data path | message timestamp | RESENDABLE / the send's urgent flag |
| 1:1 **edit** (`send --edit-timestamp` to an individual) | same path | the edit's own timestamp | RESENDABLE / urgent. `group_id` absent |
| Group DataMessage sent with `sendAsGroupMessage` (all group variants of the commands above) | group path, sender key and/or fan-out | message timestamp | RESENDABLE / urgent. `group_id` = the group |
| Group **edit** | group path | edit timestamp | RESENDABLE / urgent. **`group_id` absent** (§1.1) |
| Group update / create / quit messages (`groups.md`, "complete group message" sends) | group path | message timestamp | IMPLICIT / urgent true. `group_id` = the group |
| Receipts (automatic delivery receipts, automatic and explicit read/viewed receipts, `sendReceipt`) | receipt path | the receipt envelope's timestamp (`ReceiptMessage` "when"), **not** the timestamps being acknowledged | IMPLICIT / false |

A logged receipt can only be found again by a retry receipt that names the
receipt's own envelope timestamp.

### 2.2 Not logged

| Send | Why |
|------|-----|
| Typing indicators (1:1 and group, fan-out and sender key) | no logging on this path |
| Null messages (session renewals, the verification null message) | no logging |
| Retry receipts (DecryptionErrorMessage, plaintext content) | no logging. The send result also carries no Content |
| Profile-key update messages (`content.md` §4.4, `SendProfileKeyAction`) | sent without logging |
| Sync messages of every kind, including sent transcripts, note-to-self (sync-only) sends and the transcripts that follow 1:1 or group sends | not logged (`sync.md`) |
| Story sends (My Story and group stories) | no logging callback on these paths |
| Sender-key distribution messages sent ahead of a sender-key send | sent on their own path. Not part of the logged Content |
| Call messages (offer, answer, ICE, busy, hangup, opaque) | the send helper would log a call message only if it carried an explicit timestamp. None of the call messages signal-cli builds have one, and the `ManagerImpl` call APIs bypass the helper altogether. So in practice: never |
| Resends answering a retry receipt (§4) | the resend writes no new entry. The original entry stays as it was |

### 2.3 Sender-key quirk: duplicated recipient rows

For the sender-key part of a group send, the logging callback for a batch
result writes twice in a row: the first write creates the entry (or
appends, if a fan-out result already created it), and the second appends
the same recipients again. So every sender-key recipient device normally
has **two identical rows**. The duplicates have no observable effect: a
lookup returns the entry twice, and the two resend actions it produces are
equal and merge into one (§4.4). Deletion by receipt removes all matching
rows at once. A compatible implementation does not need to reproduce the
duplicates.

### 2.4 `--disable-send-log`

The global option `--disable-send-log` (or `"disableSendLog": true` in the
global `config.json`, which supplies the default; see
`spec/surface/cli/index.md`) changes exactly one thing: **nothing is
inserted** (no new content rows, no appended recipient rows). Everything
else keeps running:

- lookups for incoming retry receipts still query the table, so entries
  written by earlier runs without the flag (younger than one day) are still
  resent;
- deletions by delivery receipt and by remote delete, and age pruning,
  still run.

The option is read once when the account is opened and applies to every
account the process opens.

---

## 3. When entries are deleted

### 3.1 Delivery receipts

Two kinds of incoming event delete rows. Both match **exactly** on the
service-ID string and the device ID of the receipt's sender:

1. **A `ReceiptMessage` of type DELIVERY** inside a successfully decrypted
   Content. For each timestamp in its `timestamp` list, signal-cli deletes
   the recipient rows whose `address` is the sender's service ID, whose
   `device_id` is the sender's device, and whose entry has that
   `timestamp`. The sender is the envelope source (unsealed) or the
   sealed-sender certificate's sender and device. This happens only after
   the envelope passed the blocked-sender and group-permission checks, so a
   receipt from a blocked contact deletes nothing.
2. **An envelope of type `SERVER_DELIVERY_RECEIPT` (5)**. This envelope is
   not decrypted. signal-cli deletes the rows for (envelope
   `sourceServiceId`, envelope `sourceDevice`, envelope `timestamp`). This
   is done before the blocked check.

**Read and viewed receipts delete nothing.**

After either deletion, every content row that no longer has any recipient
row is deleted (a global orphan sweep, not only the affected entry). So an
entry lives until each of its recipient devices has sent a delivery receipt
or the entry ages out.

Because matching is by exact string, a receipt from a peer's ACI does not
remove rows written under that peer's PNI, and the other way round.

### 3.2 Remote delete

Before sending a remote delete for `targetSentTimestamp`
(`content-features.md` §3.12), for each recipient:

- **Individual** (given as ACI, PNI, or a number/username resolved to a
  service ID): delete every entry with that `timestamp`, **no** group id,
  and at least one recipient row whose `address` is that service ID. The
  whole entry goes, including rows for any other address (a 1:1 entry only
  ever has one address anyway). Then the orphan sweep. A recipient that
  cannot be resolved, or has no service ID, is skipped silently.
- **Group**: delete every entry with that `timestamp` and that `group_id`
  (cascade removes its recipient rows). There is no orphan sweep here.

Group edits are stored without a group id (§1.1), but this does not matter
here: the delete targets the original message's timestamp, and that entry
has the group id.

### 3.3 Age pruning

- Retention: **1 day**. A content row is deleted when its `timestamp` is
  older than now minus 86 400 000 ms. The comparison uses the message
  timestamp, not the time the row was written. Recipient rows go with it
  through the cascade.
- When it runs:
  - on a background daemon thread (named `msl-cleanup`), started when the
    account's send-log store is first used in the process. It prunes once
    at start and then every **1 hour** for as long as the account stays
    open. The thread stops when the account is closed. If one prune fails
    with a database error, the thread logs a warning and stops for the rest
    of the process;
  - **before every lookup** for a retry receipt (§4.1), so an expired
    entry is never resent, even within the hour.
- Logging: when rows were removed, `Removed <n> outdated entries from the
  message send log` at DEBUG. Otherwise a TRACE line. On a failure at the
  thread level: WARN `Deleting outdated entries failed` (cause at DEBUG).

### 3.4 Things that do **not** touch the log

- Session archiving or deletion, identity-key changes, 409/410 device-list
  changes, and session renewals (§4.3 archives sessions but leaves the log
  alone).
- Recipient merges or recipient deletion. Rows refer to service-ID
  strings, not recipient ids, so they keep their address.
- Blocking, leaving a group, or a group being deleted locally. The entries
  stay. A later resend for them is skipped (§4.2).
- Account deletion or unregistration: the log lives in the account database
  and is removed together with it when the account's data is removed. There
  is no separate step for the log.

---

## 4. Handling an incoming DecryptionErrorMessage

### 4.1 Preconditions and lookup

The DecryptionErrorMessage arrives as Content field 8, normally as
plaintext content (`pipe.md` §7.2). It is handled like any decrypted
Content: only if the envelope passed the destination checks, the sender is
not blocked, and the group-permission check passed. On arrival signal-cli
logs, at DEBUG,
`Received a decryption error message from RecipientId[id=<n>].<senderDevice> (resend request for <timestamp>)`.

- If the message's `device_id` is **not** this account's device ID:
  DEBUG `Request is for another one of our devices`, and nothing else
  happens. This is the only check on `device_id`.
- Otherwise the **lookup key** is: the sender's service ID (envelope source,
  or sealed-sender certificate sender), the sender's **device ID** (from the
  envelope or certificate, not from the message), and the message's
  `timestamp`. Age pruning (§3.3) runs first. The query returns every entry
  that has a recipient row with exactly that `address` and `device_id` and
  whose `timestamp` equals the requested one.
- If the message has **no** `ratchet_key` (the peer failed a sender-key
  message), only entries **with** a group id are kept. Entries without a
  group id (1:1 messages, receipts, group edits) are dropped from the result.
- An entry whose stored Content can no longer be parsed is skipped (WARN
  `Failed to parse content from message send log`). A database error makes
  the result empty (WARN `Failed read from message send log`).

For **each** entry found, one **resend action** is queued (§4.4), keyed by
(sender recipient, requested timestamp, entry).

### 4.2 What a resend sends

Each resend is an ordinary **1:1 send** to the requesting peer's service ID
(`send.md`), never a multi-recipient sender-key send:

- **Same timestamp:** the envelope timestamp is the requested `timestamp`,
  which equals the original entry's `timestamp`. The Content is the stored
  one, so `DataMessage.timestamp` and everything else inside are unchanged.
- **Content hint and urgency** come from the entry. `online = false`, not a
  story.
- **Devices:** the send goes to **all** of the peer's devices, like any 1:1
  send: every device with an active session, plus device 1, corrected by
  the server's 409/410 answers. It is **not** limited to the device that
  asked.
- **Sealed sender:** the normal per-recipient decision of `send.md` §3
  (unidentified access key, with its 401 fallback). A group-send token is
  never used. If the peer is our own account (an entry written for our own
  devices, for example by a group send that included self), the resend is
  always unsealed.
- **No PNI signature is added.** A PNI signature is present only if it was
  already part of the stored Content.
- **No sent transcript** is sent to our own devices, and the resend is
  **not logged** again.
- **Entry without group id**: the Content is sent unchanged. The sealed-sender
  USMC carries no group ID.
- **Entry with group id**:
  1. The group is looked up locally. If it does not exist: DEBUG
     `Could not find a matching group for the groupId <base64>! Skipping message send.`,
     and nothing is sent.
  2. If the requesting recipient is not a current member of the group:
     WARN `The target user is no longer in the group <base64>! Skipping message send.`,
     and nothing is sent.
  3. Otherwise signal-cli gets the current sender-key distribution message
     for the group's distribution ID, creating a new sender key if none
     exists. It sends a copy of the stored Content with field 7
     (`senderKeyDistributionMessage`) set to that SKDM, next to the
     original fields. The USMC carries the 32-byte group ID (as raw bytes).
  4. If the send succeeds, every device the resend reached is marked as
     "sender key shared" for that distribution ID (`stories.md` §3.7).
  This holds whether the original went out by sender key or by fan-out.
- Send results of a resend are not shown to the user. Unlike normal sends,
  a resend does not run the post-send bookkeeping that downgrades the
  unidentified-access mode after an unsealed success or handles identity
  failures. Exceptions are caught and logged by the action runner
  (WARN `Message action failed.`).

Because the bytes are re-encoded, the peer sees the same plaintext Content
(plus field 7 for group entries) at the same timestamp. Encryption, padding,
envelope type, sealed-sender wrapping, and the PreKey-or-Whisper choice
come from the new send.

### 4.2.1 The membership check, precisely

Sources: M4, M5. This subsection refines step 2 of §4.2. The short
version: **in 0.14.7 the check fails for every group entry, members
included.** The WARN line is printed for any requester, member or not, so
signal-cli 0.14.7 never resends a logged group message.

**Who the "requesting recipient" is.** It is a local **RecipientId** (a row
in the account's recipient table). It is taken from the sender of the
envelope that carried the DecryptionErrorMessage, not from the send-log
row. For an unsealed envelope with a source service ID, signal-cli resolves
the envelope's source service ID to a RecipientId. For a sealed-sender
envelope it resolves the sender address from the sealed-sender certificate
(the sender's ACI, plus the certificate's E.164 if present) to a
RecipientId. That resolution can create or merge recipient rows, as any
recipient resolution can. The same RecipientId goes into the resend action
(§4.4) and is what the check later tests. The send-log lookup itself
(§4.1) uses the sender's service-ID string and device, not the
RecipientId. The send-log row contributes only the group id and the
Content. Its `address` column is not consulted again.

**How "current member" is meant to be evaluated.** The group is the local
group record for the entry's stored group id (§1.1), loaded through the
normal group lookup. For a v2 group whose decrypted state is not cached,
that lookup can fetch it from the group server first. The intended member
list is the group's **full members** only. Pending (invited) members,
requesting members and banned members are not part of it. For a v2 group,
each full member's ACI from the decrypted group state is resolved to a
RecipientId. A v2 group with no decrypted state, for example because
permission was denied, has an empty member list. For a v1 group it is the
stored member list. PNIs play no part: v2 full members are listed by ACI
only.

**Why it always fails.** The member collection holds **member-info
objects** (one per full member, wrapping the member and able to report the
member's RecipientId and role). It does not hold RecipientIds. The check
asks whether that collection contains the requester's RecipientId object
itself, not whether any member's RecipientId equals it. A RecipientId
counts as equal only to another RecipientId. The member-info classes (v1
and v2) do not define their own equality, so they are equal only to
themselves. So the containment test is false for every requester and every
group, and execution always takes the "no longer in the group" branch.
Other membership questions in the same codebase (the incoming
group-permission check, the admin check) compare RecipientIds correctly
through each member's RecipientId. Only this resend path has the type
mismatch.

So none of the following is the cause: the retry receipt's envelope type or
source, a send-log entry stored under another address, a wrong group id,
ACI versus PNI, an E.164 on the recipient row, split or merged recipient
rows, or the pending-member list. If one of those were wrong, the visible
result would differ:

- a blocked sender means no handling
  at all (§4.1);
- a different service ID or device in the lookup means no entry found, so
  no resend action and no WARN (only the §4.3 renewal or reset);
- a group id with no local group gives the DEBUG "Could not find a matching
  group" line, not the WARN.

The WARN shows that the entry was found, the group exists locally, and the
check ran. It says nothing about whether the requester is a member.

**Side effects that still happen.** These are unchanged by the skipped
resend (§4.3). With a `ratchet_key` that matches our current session and
at least one entry found, the sessions with the sender are still archived.
No NullMessage is queued in that case, because entries were found. So the
peer receives nothing at all until our next message to it, which then goes
out as a PreKey message. Without a `ratchet_key`, the sender's "shared
with" rows for that group's distribution ID are still deleted. The next
ordinary group send redistributes the SKDM to that device, but the missed
message itself is never delivered again. Entries without a group id (1:1
messages, receipts, group edits) do not go through this check, and their
resend works as §4.2 describes.

**What the retry receipt must contain for handling to reach this point.**

- Any envelope form that decrypts to a `Content` with field 8
  (`decryptionErrorMessage`) works. In practice that is a PLAINTEXT_CONTENT
  envelope (type 8), or a sealed-sender envelope whose inner message type
  is plaintext. signal-cli does not check the envelope type, the
  sealed-sender version or the content hint.
- The envelope must pass the destination check (destination is our ACI or
  PNI), and the sender must not be blocked.
- The **USMC group id is ignored** by signal-cli on receipt. The group
  checks on incoming messages (group blocked, sender not allowed to send to
  the group) look only at a group context inside a DataMessage or
  StoryMessage. A Content that carries only a DecryptionErrorMessage has
  neither, so those checks never apply, whatever group id the USMC
  carries. The group used for the resend comes only from the send-log
  entry.
- The DecryptionErrorMessage's `device_id` must equal our device ID. Its
  `timestamp` must equal the entry's `timestamp`. The sender's (service ID,
  device) taken from the envelope or certificate must match a recipient row
  of the entry exactly (§4.1). For a sender-key failure, the message must
  have **no** `ratchet_key`, which limits matches to group entries.

**For a compatible implementation:** reproducing 0.14.7 exactly means
skipping every group-entry resend with that WARN, keeping the §4.3 side
effects. The evident intent is different: resend when the requester's
recipient is among the group's full members, resolved by ACI. An
implementation that follows the intent sends resends that 0.14.7 does not.
Which of the two to follow is a project decision. This spec records the
0.14.7 behaviour above. Note that §4.2 step 2 and §5 describe the intent.

### 4.3 Session handling: archive vs renewal

This runs **synchronously while the DecryptionErrorMessage is handled**,
right after the lookup and before any queued action executes:

- **`ratchet_key` present** (a 1:1 decryption failure):
  - signal-cli checks whether the key equals the **sender ratchet key of
    our current session** with (sender service ID, sender device), in the
    session store of the identity the DecryptionErrorMessage was addressed
    to (ACI or PNI destination). There is no match if no session exists
    or if it is already archived (M3).
  - If it matches and **no** entries were found: a **session renewal
    action** is queued (DEBUG `Renewing the session with sender`). When it
    runs, it archives all sessions with that service ID in that identity's
    store and sends a NullMessage (`content-features.md` null-message
    trigger 1: IMPLICIT, not urgent, PreKey message). Renewals are
    deduplicated per sender recipient.
  - If it matches and entries **were** found: all sessions with that
    service ID in that identity's store are archived **immediately** (TRACE
    `Archiving the session with sender, a resend message has already been queued`).
    No NullMessage is sent. The queued resend then finds no session and
    builds a new one from a fresh prekey bundle, so the resend arrives as a
    PreKey message.
  - If it does not match: no session change. The resend (if any) uses the
    existing sessions.
  - With a ratchet key, the sender-key state is **not touched**.
- **`ratchet_key` absent** (a sender-key failure):
  - No session check, no archiving, no renewal.
  - For each found entry (all of which have a group id, §4.1) whose group
    exists locally: delete the "shared with" rows for (sender service ID,
    sender device, that group's distribution ID). TRACE
    `Deleting shared sender key with RecipientId[id=<n>] (<device>): <distributionId>`.
  - If no such entry was found (no entries, or only groups that no longer
    exist locally): delete **all** "shared with" rows for the sender's
    service ID, across all distributions and devices. DEBUG
    `Reset all shared sender keys with this recipient, no related message found in send log`.
    Story sends are never logged, so a sender-key retry for a story always
    takes this branch.
  - The resend itself then re-marks the reached devices as shared (§4.2
    step 4). For a group that is not resent, the next group send
    redistributes the SKDM to the affected devices.

`stories.md` §3.7 lists the shared-with deletion as happening for every
retry request. It actually happens only when `ratchet_key` is absent.

Edge case: archiving uses the destination identity's session store, but
every resend and renewal NullMessage is sent through the normal (ACI)
sending path. For a DecryptionErrorMessage addressed to our PNI, the
archive therefore hits the PNI-side sessions while the resend uses the
ACI-side ones.

### 4.4 Timing, ordering, deduplication, rate limiting

- Resend and renewal actions are not executed inline. They go into the
  receive loop's action queue (`pipe.md` §7, steps 3 to 5). While the initial backlog is
  drained, actions accumulate. After the queue-empty signal, the queued
  actions run after each handled envelope. Remaining actions also run when
  `receive` ends. Retries of cached envelopes run their actions as a batch.
- The queue is keyed by action equality. Two resend actions are equal when
  the sender recipient, requested timestamp and entry (group id, Content,
  hint, urgent) are equal. Equal actions in the same batch run once. This
  merges the duplicated sender-key rows (§2.3), and also repeated or
  per-device retry receipts from one peer for the same timestamp. Since a
  resend goes to all of the peer's devices anyway, one resend covers them.
- Execution order within a batch is **unspecified** (hash order). That
  includes resends relative to each other and to other actions such as
  renewals or receipts. The session archiving of §4.3 always comes before
  the resends, because it happens at handling time.
- There is **no other rate limiting and no persistent deduplication.** A
  retry receipt that arrives in a later batch for the same timestamp causes
  another resend, for as long as the entry exists. The entry is not removed
  by resending, only by a delivery receipt from that device, a remote
  delete, or age.

---

## 5. Observable behaviour summary

**Peer (requesting device and its siblings):**

- The resent envelope(s) carry the original timestamp and the original
  plaintext Content, with field 7 (SKDM) added for group entries. Content
  hint and `urgent` are as originally sent. The USMC group ID is present
  for group entries and absent otherwise (including group edits).
- They usually arrive as PreKey messages (after the §4.3 archive) and go
  to every device of the peer, not only the requester.
- If the log had nothing, the peer gets a NullMessage instead (only when
  the ratchet key matched our current session), or nothing at all.
- No resend happens for: typing, receipts other than via their own
  timestamp, sync messages, stories, profile-key updates, null messages,
  call messages, messages older than one day, messages already
  delivery-receipted by that device, remotely deleted messages, or groups
  the peer is no longer in. For a sender-key retry, 1:1 entries and group
  edits are not resent either.

**Local user:** nothing appears in normal output. The only traces are log
lines at the levels shown above (DEBUG/TRACE need `-v`/`-vv`/`-vvv`):

| Level | Text |
|-------|------|
| DEBUG | `Received a decryption error message from RecipientId[id=<n>].<device> (resend request for <timestamp>)` |
| DEBUG | `Request is for another one of our devices` |
| DEBUG | `Renewing the session with sender` |
| TRACE | `Archiving the session with sender, a resend message has already been queued` |
| TRACE | `Deleting shared sender key with RecipientId[id=<n>] (<device>): <distributionId>` |
| DEBUG | `Reset all shared sender keys with this recipient, no related message found in send log` |
| DEBUG | `Executing action ResendMessageAction` (or `RenewSessionAction`), preceded by `Handling message actions` |
| TRACE | `Resending message <timestamp> to RecipientId[id=<n>]` |
| DEBUG | `Could not find a matching group for the groupId <base64>! Skipping message send.` |
| WARN | `The target user is no longer in the group <base64>! Skipping message send.` |
| DEBUG | `Removed <n> outdated entries from the message send log` |
| WARN | `Deleting outdated entries failed` |
| WARN | `Failed read from message send log` / `Failed delete from message send log` / `Failed to insert into message send log` / `Failed to insert message send log content` / `Failed to append recipients to message send log` / `Failed to parse content from message send log` |

The library also logs `[<timestamp>] Resending content.` at debug level
when a resend is sent (M2). Exact log text is informative only. No test or
client depends on it.

---

## 6. Notes for a compatible implementation

1. Store the plaintext Content, hint, urgent flag, timestamp, derived
   group id, and the (service ID, device) pairs from successful sends of
   the kinds in §2.1. Prune after one day and on every lookup. Delete on
   DELIVERY receipts and server delivery receipts matching (service ID,
   device, timestamp), and on remote delete.
2. **Correction to `pipe.md` §7.3:** `--disable-send-log` stops writing,
   not looking up (§2.4). An implementation that never writes the log gets
   the same behaviour, apart from entries left over from earlier runs.
3. Answering with a resend to all of the peer's devices at the original
   timestamp is what signal-cli does. An implementation that has no log
   entry should still do the §4.3 renewal (archive, then NullMessage) when
   the ratchet key matches, and the sender-key shared-with reset when it
   is absent.
