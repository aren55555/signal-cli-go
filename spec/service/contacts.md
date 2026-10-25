# Contacts and recipients (signal-cli 0.14.7)

Spec role: prose only; no code. This file covers the local recipient and
contact store of signal-cli 0.14.7, the rules for resolving and merging
recipients (ACI, PNI, phone number, username) as far as contacts and
profiles depend on them, and the service-level behaviour of these
commands: `updateContact`, `removeContact`, `listContacts`, `block`,
`unblock`, `getUserStatus`, `listIdentities` and `getAvatar`.

Argument parsing, help texts and parse errors of these commands are in
`spec/surface/cli/<command>.md`; this file adds what happens underneath and
the exact output rules. Profiles are in `spec/service/profiles.md`. The
CDSI lookup itself is in `spec/service/cdsi.md` (written separately). Sync
message formats are in `spec/service/sync.md`.

## Sources

| Ref | What | URL @ version |
|-----|------|---------------|
| K1 | signal-cli | https://raw.githubusercontent.com/AsamK/signal-cli/b01b6b370dc063599a1a2b9fde0f5ff4e2d78fe8/ (tag v0.14.7): `lib/src/main/java/org/asamk/signal/manager/storage/recipients/{RecipientStore,MergeRecipientHelper,RecipientAddress,Recipient}.java`, `.../manager/storage/{SignalAccount,AvatarStore}.java`, `.../manager/storage/identities/IdentityKeyStore.java`, `.../manager/helper/{ContactHelper,RecipientHelper,SyncHelper,SendHelper,ProfileHelper,IncomingMessageHandler,IdentityHelper}.java`, `.../manager/internal/ManagerImpl.java`, `.../manager/api/{Contact,Profile,Recipient,RecipientAddress,UserStatus,Identity}.java`, `.../manager/config/ServiceConfig.java`; `src/main/java/org/asamk/signal/commands/{ListContactsCommand,UpdateContactCommand,RemoveContactCommand,BlockCommand,UnblockCommand,GetUserStatusCommand,GetAvatarCommand,ListIdentitiesCommand}.java`, `src/main/java/org/asamk/signal/json/JsonContact.java`, `src/main/java/org/asamk/signal/util/CommandUtil.java` |
| K2 | Reference distribution, offline | jshell over `/work/third_party/signal-cli-0.14.7/lib/*`: string forms of service IDs (`org.signal.core.models.ServiceId`): an ACI prints as the bare lowercase UUID, a PNI as `PNI:<uuid>`. `grep` over the decompiled `signal-cli-0.14.7.jar` for the callers of the sender-name lookup (`ReceiveMessageHandler`, `JsonMessageEnvelope`, D-Bus classes) |

## 1. Terms

- **Recipient**: one row of the local recipient store. A recipient has at
  least one of ACI, PNI, phone number (E.164) and username, and gets a
  stable numeric row ID.
- **Service ID string forms** (K2): ACI = bare lowercase UUID; PNI =
  `PNI:` + lowercase UUID. These forms appear in output (for example
  `listContacts` "PNI:"), in avatar file names and in request paths.
- **Contact**: a recipient for which the **is-contact predicate** holds:
  at least one of
  - given name, family name, nickname, nick given name, nick family name
    or note is set (not null; an empty string counts as set),
  - expiration time > 0,
  - profile sharing is on,
  - a colour is set,
  - blocked,
  - archived.

  **Hidden** is not part of the predicate, and neither are the profile,
  the profile key or mute. A recipient that is only hidden is not a
  contact.
- **Contact record**: the set of contact columns of a recipient (§2).
  Reading "the contact" of a recipient returns nothing unless the
  is-contact predicate holds. The listing in §6 is the exception: it always
  builds a record from the columns.

## 2. Recipient record

### 2.1 Logical fields

| Group | Field | Notes |
|-------|-------|-------|
| address | number, aci, pni, username | each unique across the store, or null |
| registration | unregisteredTimestamp | null = registered/unknown; set = marked unregistered at that time (ms) |
| | discoverable | null (never checked), true or false (from CDSI) |
| profile | profileKey, profileKeyCredential, profile record | `spec/service/profiles.md` §1.1 |
| PNI signature | needsPniSignature, pniSignatureVerified | `spec/service/send.md` |
| contact | givenName, familyName | "system" contact name (from `updateContact`, contact sync, storage) |
| | nickName | legacy single nickname; never set by any command in 0.14.7, only read and shown |
| | nickNameGivenName, nickNameFamilyName | the user's nickname for this contact |
| | note | free text |
| | color | string; never set by any command in 0.14.7 (storage service only) |
| | expirationTime, expirationTimeVersion | disappearing-message timer in seconds (0 = off); version ≥ 1 |
| | muteUntil, hideStory, archived | from storage service; only `archived` is shown |
| | blocked | boolean |
| | profileSharing | boolean: "the user accepted this conversation"; controls profile-key sharing |
| | hidden | boolean |
| storage | storageId, storageRecord | storage-service bookkeeping (out of scope) |

### 2.2 On-disk form (for importing an existing data directory)

signal-cli keeps these fields in table `recipient` of the SQLite database
`<config>/data/<account path>.d/account.db`. Column names, as stored:
`_id` (integer primary key, autoincrement), `storage_id`, `storage_record`,
`number`, `username`, `aci`, `pni`, `unregistered_timestamp`,
`discoverable`, `profile_key`, `profile_key_credential`,
`needs_pni_signature`, `pni_signature_verified`, `given_name`,
`family_name`, `nick_name`, `nick_name_given_name`,
`nick_name_family_name`, `note`, `color`, `expiration_time` (default 0),
`expiration_time_version` (default 1), `mute_until`, `blocked`, `archived`,
`profile_sharing`, `hide_story`, `hidden`,
`profile_last_update_timestamp` (default 0), `profile_given_name`,
`profile_family_name`, `profile_about`, `profile_about_emoji`,
`profile_avatar_url_path`, `profile_mobile_coin_address`,
`profile_unidentified_access_mode` (enum name as text),
`profile_capabilities` (enum names joined by `,`; **null means "no profile
stored"**), `profile_phone_number_sharing` (enum name or null).

Encodings:

- `aci` is the bare UUID and `pni` is `PNI:<uuid>`.
- Booleans are integers 0/1.
- `profile_key` holds the 32 raw bytes, and `profile_key_credential` the
  serialized credential.
- An unreadable `profile_key` or `profile_key_credential` is treated as
  absent.
- Unknown enum names read as `UNKNOWN` (access mode) or are dropped
  (capabilities).

A Go implementation does not have to use this format. It matters only for
the data-directory import goal.

### 2.3 How contact columns are written

Every change to a contact rewrites **all** contact columns at once, from a
record built by copying the current contact record (or an empty one) and
changing one or more fields. Consequences:

- The record read for copying is "nothing" when the is-contact predicate
  is false, so the copy starts empty. For such a recipient this **clears
  `unregisteredTimestamp`** (the record carries it), which marks the
  recipient registered again. Affected writes: `updateContact`, `block`,
  `--hide`, first send, and others.
- An empty record has expirationTimeVersion 1. The stored version is never
  below 1.
- Deleting the contact (`removeContact` without flags) writes an empty
  record: all names, note and colour become null; expiration 0 (version
  reset to 1); mute 0; hideStory, profileSharing, blocked, archived and
  hidden false; unregisteredTimestamp null.
- Each write also marks the recipient as changed for the storage service
  (a new storage ID). Commands then queue a storage sync. The storage
  protocol is out of scope.

If a write stores a non-null unregisteredTimestamp, the recipient is also
processed as "marked unregistered" (§4.4).

## 3. Names

### 3.1 Contact name

Built from givenName and familyName, where "empty" means null or `""`:

- both empty: `""`;
- only one non-empty: that one;
- both non-empty: given + one space + family.

### 3.2 Nickname

Built the same way from nickNameGivenName and nickNameFamilyName, except
that "both empty" yields **null** (no nickname). The legacy `nickName`
column takes no part in this.

### 3.3 Sender name in received-message output

When received-message output needs a name for a recipient (plain-text
`“NAME” IDENT`, JSON `sourceName`, D-Bus), it is chosen as follows:

1. If the recipient identifier cannot be resolved: null.
2. If a contact record exists and its nickname (§3.2) is non-empty: the
   nickname.
3. If a contact record exists and its contact name (§3.1) is non-empty:
   the contact name.
4. Otherwise the recipient's **profile, read lazily**: a fetch if missing or
   6 h old (`spec/service/profiles.md` §5.1). Since a profile is stored even
   after a failed fetch, this is nearly always a profile, and the result is
   its display name (`spec/crypto/profile-cipher.md` §8). That may be `""`.
5. Null only if no profile could be stored at all.

So JSON `sourceName` can be `""` (profile without a name), not just null.
Plain-text output treats an empty name as "no name".

## 4. Resolving and merging recipients

### 4.1 Recipient arguments of commands

A command argument is first parsed into an identifier: a phone number
(canonicalized to E.164 using the account's own number for the country
context), an ACI UUID, `PNI:<uuid>`, or `u:<username>`
(`spec/surface/cli/index.md`, recipient forms). Parsing errors give
`Invalid phone number '<input>': <reason>` (exit 1). The identifier is then
resolved to a recipient:

| Identifier | Resolution |
|------------|------------|
| ACI | the recipient with that ACI; created with only that ACI if none exists. Never fails, no network. |
| PNI | the same, by PNI. |
| number | if a recipient with that number exists **and has a service ID**: that recipient. Otherwise a **CDSI lookup** of the one number (`spec/service/cdsi.md`). If CDSI returns a service ID, the recipient for that service ID (the CDSI result was already stored as a trusted ACI+PNI+number link, §4.3). If CDSI finds nothing, or fails, **"not registered"**. |
| username or username link | if a recipient with that username exists and has a service ID: it. Otherwise a username lookup on the server (`spec/service/usernames.md`, written in parallel); a found ACI is linked to the username as a trusted pair. Not found or any error: **"not registered"**. |

"Not registered" surfaces as `The user <identifier> is not registered.`
(exit 1). `<identifier>` is the number or username as given (canonical
form for numbers).

**Optional resolution** (used by `removeContact`): as above, but a number
that resolves to "not registered" falls back to a local lookup by number
only. If that finds nothing too, the command does nothing.

### 4.2 Low-trust resolution (one identifier, or untrusted sources)

Used when only a single identifier is at hand, or when the source does not
vouch for the link between identifiers. Lookup order:

1. by ACI;
2. by PNI;
3. by number.

The first hit wins and nothing is linked or changed. If nothing is found, a
new recipient is created with **only the service ID** (ACI if present, else
PNI) if the address has one. Otherwise it is created with only the number.
Note that a number given together with a service ID is **dropped** in that
case.

### 4.3 High-trust resolution and merging

Used when a source vouches that identifiers belong together. The sources
in scope:

- CDSI results (ACI, PNI, number);
- contact-sync records (ACI, number);
- username lookups (ACI, username);
- storage records;
- PNI signatures;
- the account's own address (for self, the merge below always runs; for
  other addresses that match the account's own address, the low-trust path
  is used instead).

If the address has a single identifier, low-trust resolution (§4.2) is
used. Otherwise, let A be the address with its identifiers (ACI, PNI,
number, username), and let **M** be the set of existing recipients that
match A on any one identifier:

1. **M empty:** create a recipient with all identifiers of A.
2. **M has one recipient R:**
   - if R already has every identifier of A: use R unchanged;
   - else, if R has no ACI, or A has no ACI and (A has no PNI or R's PNI
     equals A's PNI), or R's ACI equals A's ACI: **update R** to the union
     (A's identifiers win where both have one);
   - otherwise (R has a different ACI): **strip** from R every identifier it
     shares with A, then create a new recipient with A.
3. **M has several recipients:**
   1. The **target** T is the member whose ACI equals A's ACI. If there is
      none and A has a PNI, it is the member whose PNI equals A's PNI. If
      neither exists, there is no T yet.
   2. Each other member O is classified:
      - **merge** if O has no identifier beyond A's (every identifier O has
        equals A's value for that kind), or if O has only a PNI and a
        number (the server links those two);
      - otherwise **strip**.
   3. The final address starts from T's address (if any), adds the
      identifiers of every merge member, and then is overridden by A (A's
      identifiers win). Each merge member's address is cleared (all four
      identifiers set to null).
   4. Each strip member loses every identifier it shares with the final
      address.
   5. If T exists, it gets the final address. Otherwise a new recipient with
      the final address is created and becomes T.
   6. T is **marked registered**.
   7. Each merge member's data is merged into T (below), and the merge
      member's row is deleted.

**Merging data of O into T:**

- the contact record is copied only if T has **no** contact record
  (is-contact false for T);
- the profile key is copied only if T has none. This copy does not reset
  the profile timestamp;
- the credential is copied only if T has none;
- the profile record is copied only if T has none;
- "PNI signature verified" becomes true on T if O had it;
- group membership and cached messages that refer to O are moved to T
  (`spec/service/groups.md`).

Address updates keep `pniSignatureVerified` only if both ACI and PNI are
unchanged. Every address update marks the recipient changed for storage.

### 4.4 Registered and unregistered

- **Mark registered:** clears unregisteredTimestamp. Done after every
  successful profile fetch and every high-trust merge.
- **Mark unregistered:** unregisteredTimestamp = now, discoverable = false.
  If the recipient has both an ACI and a PNI, it is **split**: the recipient
  keeps its ACI (and username), and a new recipient is created with the PNI
  and the number. Done after a profile fetch answered 404
  (`spec/service/profiles.md` §5.5), and through CDSI.
- **CDSI outcome** (summary; details in `spec/service/cdsi.md`): every
  number found gets a high-trust ACI+PNI+number link and discoverable =
  true. Every number queried but not found gets discoverable = false. Such
  a recipient is also marked unregistered (with split) if it has no ACI, or
  if it was already marked unregistered.

## 5. Other places that change contact fields

These are not commands of this spec, but they explain what the listing
shows:

| Event | Change |
|-------|--------|
| any 1:1 send through the single-recipient path (`spec/service/content.md` §4.4) | if there is no contact record, or profile sharing is off, or the recipient is hidden: profileSharing := true, hidden := false |
| incoming 1:1 DataMessage with a body or the EXPIRATION_TIMER_UPDATE flag (from the peer, or a sent transcript to the peer) | expiration timer := the message's `expireTimer` (0 if absent) with its `expireTimerVersion`. Skipped if a contact record exists and either the timer is already equal or the stored version ≥ the incoming version. |
| contact sync (`spec/service/sync.md` §5.1) | names, timer, contact avatar |
| blocked-list sync received (`spec/service/sync.md` §6.4) | blocked := true (and profileSharing := false) for each listed recipient |
| message request response sent (`block`, `unblock`, `sendMessageRequestResponse`) | profileSharing := true for ACCEPT and UNBLOCK_AND_ACCEPT, false otherwise. This happens **even on a single-device account** (the sync send itself is skipped there). |
| storage-service records | everything (out of scope) |

## 6. `listContacts`

### 6.1 Selection

Inputs: `-a` (all recipients), `--blocked BOOL`, `--name NAME`, positional
recipients. For the `--blocked` value quirk see
`spec/surface/cli/listContacts.md`.

1. Each positional recipient is resolved as in §4.1. Recipients that are
   "not registered" are **silently dropped**. A number can cost a CDSI
   lookup. If recipients were given and **all** were dropped, the output is
   empty (`[]` in JSON, nothing in plain text) and nothing else happens.
2. Rows are selected with these conditions joined by AND:
   - the row has a number, PNI or ACI (username-only rows are never listed);
   - without `-a`: unregisteredTimestamp is null **and** is-contact
     **and** not hidden;
   - with `--blocked`: blocked equals the value;
   - with positional recipients: the row is one of the resolved
     recipients. Without `-a` these must **also** pass the contact
     condition. A given non-contact is not listed unless `-a` is used.
3. `--name NAME` keeps rows where NAME **equals** (exact, case-sensitive)
   the contact name (§3.1) or the profile display name. Nicknames, partial
   matches and the empty-name corner case do not get special handling.
   `--name ""` matches every row whose contact name or profile name is
   empty.
4. Order: database order (ascending row ID in practice; not guaranteed).

### 6.2 Profile refresh before listing

- Without positional recipients: the same selection (steps 2 and 3) is run
  once first. Every selected row that is **not marked unregistered** gets a
  **lazy** profile read (`spec/service/profiles.md` §5.1), up to 10
  fetches in parallel. Errors are logged (`Failed to refresh profiles for
  recipients`) and ignored. A large contact list can therefore cause many
  profile requests.
- With positional recipients: a **forced** profile fetch for each resolved
  recipient, whether or not it is a contact.
- Then the rows are selected again and printed with the updated data.

### 6.3 What each listed row carries

For every listed row, a contact record and a profile record are **always
built from the columns**, even if the recipient is not a contact or has no
profile. Missing values are null, false or 0. A never-fetched profile has
lastUpdateTimestamp 0, access mode `UNKNOWN` and no capabilities. So
`--detailed` and `--internal` never fail for lack of a record, and JSON
`profile` is never null.

For the account's own row, the account's own profile key is used. This is
not printed.

### 6.4 Plain-text output

Line 1, per row (`<…>` = value; booleans print as `true`/`false`):

`Number: <number> ACI: <aci> Name: <contact name> Profile name: <profile display name> Username: <username> Color: <color> Blocked: <blocked> Message expiration: <exp>`

- number, ACI, username and colour print as `""` (nothing) when absent.
  The ACI is the bare UUID.
- `<exp>`: `disabled` if the timer is 0, else `<seconds>s` (e.g. `3600s`).

With `--detailed`, line 2, indented by **two spaces**:

`PNI: <pni> Given name: <given> Family name: <family>, Nick name: <nickName> Nick given name: <nickGiven> Nick family name <nickFamily> Note: <note> Archived: <archived> Hidden: <hidden> Profile sharing: <profileSharing> About: <about> About Emoji: <aboutEmoji> Unregistered: <bool>`

- `<pni>` is in the `PNI:<uuid>` form, so the line starts
  `PNI: PNI:…`. Absent values print as nothing.
- Note the comma after the family name and the missing colon after
  `Nick family name`. Both are verbatim.
- `About` and `About Emoji` come from the profile.
- `Unregistered` is true when unregisteredTimestamp is set.

With `--internal`, another line, also indented by two spaces:

`Capabilities: [<cap>, <cap>] Unidentified access mode: <mode> Shares number: <shares> Discoverable by number: <disc>`

- Capabilities use list rendering: `[]`, `[storage]`, or both names
  separated by `, `. With two capabilities the **order is not
  deterministic** in the reference (a hash set of enum values). Tests
  should accept both orders.
- `<mode>`: `DISABLED`, `ENABLED` or `UNRESTRICTED`; nothing for `UNKNOWN`.
- `<shares>`: `true` if the profile's sharing mode is `EVERYBODY`, `false`
  if `NOBODY`, nothing if unknown.
- `<disc>`: `true`, `false` or nothing (never checked).

`--detailed` and `--internal` are independent: `--internal` alone prints
lines 1 and 3. Without rows, nothing is printed.

### 6.5 JSON output

One JSON array (one line), one object per row. Members in this order:

| Member | Value |
|--------|-------|
| `number` | string or null |
| `uuid` | ACI (bare UUID) or null. A PNI-only row has null here, and its PNI is **not** in the JSON at all. |
| `username` | string or null |
| `name` | contact name (§3.1); `""` when none |
| `givenName`, `familyName`, `nickName`, `nickGivenName`, `nickFamilyName`, `note`, `color` | string or null |
| `isArchived`, `isBlocked`, `isHidden` | boolean |
| `messageExpirationTime` | integer seconds |
| `profileSharing` | boolean |
| `unregistered` | boolean |
| `profile` | object, never null: `lastUpdateTimestamp` (integer ms, 0 if never), `givenName`, `familyName`, `about`, `aboutEmoji` (string or null), `hasAvatar` (true if avatarUrlPath is non-null), `mobileCoinAddress` (standard base64 with padding, or null) |
| `internal` | **only with `--internal`**; otherwise the member is omitted. Object: `capabilities` (array of capability names, order as in §6.4), `unidentifiedAccessMode` (string, or null for UNKNOWN), `sharesPhoneNumber` (boolean or null), `discoverableByPhonenumber` (boolean or null) |

`--detailed` has no effect on JSON. Example for a contact with only a
nickname and a never-fetched profile:

```
[{"number":"+15555550123","uuid":"00000000-0000-4000-8000-000000000002","username":null,"name":"","givenName":null,"familyName":null,"nickName":null,"nickGivenName":"Al","nickFamilyName":null,"note":null,"color":null,"isArchived":false,"isBlocked":false,"isHidden":false,"messageExpirationTime":0,"profileSharing":false,"unregistered":false,"profile":{"lastUpdateTimestamp":0,"givenName":null,"familyName":null,"about":null,"aboutEmoji":null,"hasAvatar":false,"mobileCoinAddress":null}}]
```

(Constructed from the rules above, not captured. Running `listContacts`
needs a registered account; see Gap 1.)

## 7. `block` and `unblock`

### 7.1 Contacts

For `block` (blocked = true) or `unblock` (blocked = false) with recipients
R1…Rn:

1. Resolve all recipients (§4.1). The first "not registered" aborts the
   command before any change (`The user <id> is not registered.`, exit 1).
   Duplicates collapse.
2. For each recipient whose blocked state **differs** from the requested
   one, in unspecified order (the set order of the resolved IDs):
   1. Write the contact record with blocked := value. **Blocking also sets
      profileSharing := false.** Unblocking leaves it as is.
   2. Send a message-request-response sync (`spec/service/sync.md` §6.9):
      type BLOCK for block, UNBLOCK_AND_ACCEPT (sent on the wire as 0) for
      unblock. This sets profileSharing := false (block) or **true**
      (unblock) locally, even on single-device accounts. A recipient
      without a service ID is skipped here.
   3. Block only: decide whether to rotate. Rotation is needed if, **for any
      newly blocked contact**, there is no group in which both the account
      and that contact are members. All stored groups count, v1 included.
3. If rotation is needed: the profile-key rotation of
   `spec/service/profiles.md` §7.
4. Send the full blocked list as a sync message (`spec/service/sync.md`
   §6.4).
5. Queue a storage sync.

Steps 3 to 5 run whenever at least one recipient was given, **even if no
state changed** (the blocked-list sync is sent anyway). With no recipients,
nothing happens.

### 7.2 Groups

Same structure per group ID: the group's blocked flag is set, and a
message-request-response sync with `groupId` is sent. Rotation is needed if
**any** group was newly blocked, regardless of membership. Then come the
blocked-list sync and the storage sync. An unknown group ID logs `Unknown
group id: …` as a warning; the remaining handling is in
`spec/surface/cli/block.md`.

Contacts are processed fully (including their blocked-list sync) before
groups. With both recipients and groups, two blocked-list syncs are sent.

### 7.3 Effects of being blocked

- Messages from a blocked contact or group are ignored on receipt.
- No profile key goes into messages to a blocked contact.
- Blocked recipients do not get profile-key updates after a rotation.

The receive-side rules are in the receive spec, not repeated here.

### 7.4 Output

Nothing on success. Errors are as in `spec/surface/cli/block.md` and
`unblock.md`.

## 8. `getUserStatus`

1. Each phone-number argument is canonicalized to E.164 using the
   account's number. Arguments that fail are kept with an empty canonical
   form: they are not looked up, and are reported as unregistered without a
   `number`.
2. All distinct canonical numbers are looked up through CDSI in one
   operation (`spec/service/cdsi.md`). If there are more than **3**, the
   lookup becomes a full refresh that also includes every number already
   stored. The CDSI result updates the recipient store as in §4.4.
   CDSI resource exhaustion becomes the `Rate limit reached` error
   (`spec/surface/cli/getUserStatus.md`).
3. For each number found, the reported UUID is the **ACI if CDSI returned
   one, otherwise the PNI's raw UUID** (without `PNI:`). The recipient's
   profile is then read **lazily** (`spec/service/profiles.md` §5.1). The
   "unrestricted sealed sender" flag is true if that profile's access mode
   is `UNRESTRICTED`.
4. Each `--username` is resolved with a **forced** server lookup (not the
   local table). Not found or failed means unregistered. A found user gets a
   lazy profile read as in step 3.
5. Output formats, ordering and errors: `spec/surface/cli/getUserStatus.md`.

## 9. `updateContact`

Arguments: one recipient; `-n/--name`, `--given-name`, `--family-name`,
`--nick-given-name`, `--nick-family-name`, `--note`, `-e/--expiration`.

1. Resolve the recipient (§4.1). "Not registered" gives `The user <id> is
   not registered.` (exit 1).
2. If `-e N` was given:
   1. Expiration timer: if a contact record exists and its timer already
      equals N, nothing is stored. Otherwise store timer N with version :=
      1 for a new contact record, else stored version + 1 (it stays at
      2 147 483 647 once there).
   2. **Always** send an expiration-timer update to the recipient, even if
      the timer was unchanged. This is a DataMessage with flags =
      EXPIRATION_TIMER_UPDATE, `expireTimer` = the contact's timer (omitted
      when 0), `expireTimerVersion` = the contact's version, and
      `profileKey` unless blocked, through the normal 1:1 send path
      (`spec/service/content.md` §4.4, `spec/service/send.md`). This also
      sets profileSharing := true and hidden := false. Send failures are
      **not reported**: the command neither prints nor fails on them.
   3. Queue a storage sync.
3. Names. If `--given-name` is absent, the given name comes from `-n`, and
   if `-n` is used without `--family-name`, the family name becomes `""`.
4. **Always** (even with only `-e`): write the contact record with
   **hidden := false**, plus each of given name, family name, nick given
   name, nick family name and note that was supplied (a value `""` is
   supplied and stored as `""`, which makes the recipient a contact). Then
   queue a storage sync.
5. No sync message is sent to linked devices for the name change. The man
   page suggests `sendContacts` for that.

Output: nothing. Errors: `spec/surface/cli/updateContact.md` (I/O errors
during the timer send are not raised in practice, see step 2.2).

## 10. `removeContact`

The recipient is resolved **optionally** (§4.1). If it cannot be resolved,
the command does nothing and still exits 0.

| Flags | Effect |
|-------|--------|
| none | write an empty contact record (§2.3): names, nickname, note, timer, blocked, archived, profile sharing, hidden, mute and colour are all reset, and the unregistered mark is cleared. Profile, profile key, identity and sessions stay. Queue a storage sync. |
| `--hide` | write the contact record with hidden := true (all else kept). The recipient disappears from the default `listContacts` and from contact sync (`spec/service/sync.md` §5.1). Queue a storage sync. A later `updateContact` or 1:1 send unhides it. |
| `--forget` | delete the recipient entirely: contact, profile, profile key and credential, then the row itself. Also its cached messages, all sessions (ACI and PNI) and sender keys for its ACI and PNI, and its identity keys. Queue a storage sync. On the account's own recipient this is refused with an internal error (`Can't delete self recipient`, uncaught: stack trace, exit 2). |

Output: nothing on success.

## 11. `listIdentities`

Reads the identity store; no network. Per stored identity (keyed by
service ID string):

1. The service ID is resolved to a recipient, creating one if needed (§4.2).
2. If that recipient's **primary service ID** (its ACI if it has one, else
   its PNI) is **not** this identity's service ID, the identity is **not
   listed**. As a result, the PNI identity of a recipient that also has an
   ACI is hidden.
3. Otherwise it is listed with the recipient's number and ACI, the
   fingerprint (the 33-byte serialized public key as spaced hex), the safety
   number (needs a fingerprint spec, see Gap 3), the trust level and the
   added timestamp.

With `-n RECIPIENT`, only the identity under that recipient's primary
service ID is shown, if stored. An unresolvable recipient gives an empty
result, not an error. Formats: `spec/surface/cli/listIdentities.md`.

How identities enter the store with which trust level (including from
profile fetches): `spec/service/profiles.md` §5.4 step 6.

## 12. `getAvatar`

- `-c RECIPIENT`: the file `avatars/contact-<id>` (written only by contact
  sync), no network.
- `-p RECIPIENT`: lazy profile read first (may download), then
  `avatars/profile-<id>` (`spec/service/profiles.md` §6.3).
- `-g GROUP`: reads `avatars/group-<id>` **before** refreshing the group.
  If the refresh downloads a new avatar, this call still returns the old
  file, or "Could not find avatar" if there was none.

`<id>` as in `spec/service/profiles.md` §1.3. Output and errors:
`spec/surface/cli/getAvatar.md`.

## 13. Corrections to other specs

1. `spec/surface/cli/listContacts.md` says `--detailed`/`--internal` fail
   with an internal error for recipients without contact or profile
   records, and that JSON `profile` is null when no profile is known. Both
   are wrong for 0.14.7: the listing always builds both records (§6.3), so
   JSON `profile` is always an object. That spec also did not mention the
   `PNI:` prefix of the printed PNI value. (Short corrections pointing here
   were added to listContacts.md in place.)
2. `spec/surface/cli/_received-message-output.md` (and
   `spec/surface/jsonrpc/notifications.md` `sourceName`) omit the nickname,
   which has priority over the contact name, and the fact that
   `sourceName` can be `""` (§3.3).
3. `spec/surface/cli/block.md` and `unblock.md`: the blocked-list sync and
   storage sync happen even when no state changed (§7.1). Unblock sets
   profile sharing to true (§7.1 step 2.2).

## Gaps

1. No output in this file was captured from the reference binary with real
   data: every command here needs a registered account, and spec agents must
   not use one. Formats come from source. The JSON example in §6.5 is
   constructed.
2. The username lookup (`u:` recipients, `getUserStatus --username`,
   username links) is specified in `spec/service/usernames.md` (written in parallel). Only its
   outcome (found ACI or not registered) is described here.
3. Safety numbers and the scannable fingerprint in `listIdentities` need a
   fingerprint spec (noted in PLAN.md milestone 5); not covered here.
4. Which recipient merges the storage service and PNI-signature handling
   trigger, and what the storage sync writes, is out of scope.
5. `removeContact --forget` deletes the recipient row. Whether this
   conflicts with other tables that refer to the row ID (for example group
   membership rows) was not checked. The reference deletes the row without
   reassigning references.
6. Row order of `listContacts` is SQLite's natural order for a query
   without `ORDER BY`. That is ascending row ID in practice, but not
   guaranteed.
