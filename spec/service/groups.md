# Groups v2, client side (signal-cli 0.14.7)

Spec role: prose and tables only; no code. This file covers what signal-cli
0.14.7 does with Signal groups v2:

- the wire formats of the group protobufs it sends, receives and stores;
- the groups service (storage service host): authentication, endpoints,
  status codes, paging, group send endorsements in responses;
- how group state is created, changed, fetched, verified, applied and
  stored, including the stored record and its distribution ID;
- the user-facing behaviour of `updateGroup`, `joinGroup`, `quitGroup`,
  `listGroups`, and the group parts of `receive`;
- how group messages are sent (sender key and fan-out, group send
  endorsements and tokens), including the group-story pieces that
  `stories.md` §2.4 defers.

Group v1 is legacy. It appears only where v1 records are migrated or where
a v1 code path shares a rule; it is otherwise out of scope.

Not repeated here:

- zkgroup cryptography: master key to secret/public params, group
  identifier, service-ID and profile-key ciphertexts, blob encryption,
  auth credentials and their presentations, expiring profile-key
  credentials and their presentations, server notary signatures, group
  send endorsements and tokens. These are named here by role only. See
  `spec/crypto/zkgroup-groups.md` and `spec/crypto/zkgroup-credentials.md`
  (written in parallel).
- the 1:1 send loop, sealed/unsealed choice, 401 fallbacks (`send.md`);
  sender-key fan-out, `PUT /v1/messages/multi_recipient`, 409/410
  handling, rotation (`stories.md` §3); Content padding and DataMessage
  fields (`content.md`, `content-features.md`); transcripts in general
  (`sync.md` §4); attachment/avatar CDN mechanics (`attachments.md`);
  the profile and profile-key-credential fetch (`profile.md`, crypto
  credentials spec); transport, TLS and user agent (`transport.md`).
- full CLI surface tables (help text, argument tables): the files in
  `spec/surface/cli/`. §14 lists where this file corrects them.

## Sources

| Ref | What | URL @ version |
|-----|------|---------------|
| G1 | signal-cli | https://github.com/AsamK/signal-cli @ v0.14.7 (`b01b6b370dc063599a1a2b9fde0f5ff4e2d78fe8`): `lib/src/main/java/org/asamk/signal/manager/helper/{GroupHelper,GroupV2Helper,SendHelper,IncomingMessageHandler,ProfileHelper,RecipientHelper,UnidentifiedAccessHelper,AccountHelper}.java`; `.../manager/storage/groups/{GroupStore,GroupInfo,GroupInfoV2,GroupMemberInfoV2}.java`; `.../manager/groups/{GroupUtils,GroupLinkPassword}.java`; `.../manager/api/{Group,GroupId,GroupIdV2,GroupInviteLinkUrl,GroupLinkState,GroupPermission,GroupNotFoundException,NotAGroupMemberException,LastGroupAdminException,GroupSendingNotAllowedException,InactiveGroupLinkException,PendingAdminApprovalException,GroupIdFormatException,MessageEnvelope}.java`; `.../manager/internal/{ManagerImpl,SignalDependencies}.java`; `.../manager/config/{StagingConfig,LiveConfig,ServiceConfig}.java`; `.../manager/syncStorage/GroupV2RecordProcessor.java`; `src/main/java/org/asamk/signal/commands/{UpdateGroup,JoinGroup,QuitGroup,ListGroups}Command.java`; `src/main/java/org/asamk/signal/util/CommandUtil.java`; `src/main/java/org/asamk/signal/{ReceiveMessageHandler.java,json/JsonGroupInfo.java}` |
| G2 | signal-service-java (Turasa fork) | https://github.com/Turasa/libsignal-service-java @ v2.15.3_unofficial_151 (`66956cb4a0394d50cee3590209baf58312084a44`): `lib/libsignal-service/src/main/protowire/{Groups,DecryptedGroups,SignalService}.proto`; `.../api/groupsv2/{GroupsV2Api,GroupsV2ApiHelper.kt,GroupsV2Operations,GroupsV2AuthorizationString,DecryptedGroupUtil,DecryptedGroupExtensions.kt,DecryptChangeVerificationMode.kt,GroupHistoryPage.kt,DecryptedGroupResponse.kt,ReceivedGroupSendEndorsements.kt,GroupSendEndorsements.kt,GroupLinkNotActiveException,CredentialResponse,TemporalCredential}`; `.../internal/push/{PushServiceSocket,ContentRange,GroupPatchResponse.kt}`, `.../internal/push/exceptions/{NotInGroup,GroupNotFound,GroupExists,GroupPatchNotAccepted,GroupTerminated,Forbidden}Exception.java`; `.../api/push/exceptions/{ConflictException,ContactManifestMismatchException}.java`; `.../api/SignalServiceMessageSender.java`, `.../api/message/MessageApi.kt`, `.../api/crypto/SealedSenderAccess.kt`; `.../api/messages/{SignalServiceGroupV2.java,SignalServiceDataMessage.kt,SignalServiceContent.kt,EnvelopeContentValidator.kt}`; `core/util-jvm/.../{Base64.kt,Hex.java,ExpiringProfileCredentialUtil.java}`; `core/network/.../NonSuccessfulResponseCodeException.kt`; tests `.../internal/push/ContentRange_parse{,_withInvalidStrings}_Test.kt` |
| G3 | storage-service (groups server) | https://github.com/signalapp/storage-service @ v20260713.0.2 (`0da239ba7ddda05306ed7b0c4b81fd627055d888`): `src/main/java/org/signal/storageservice/controllers/GroupsController.java`, `.../auth/GroupUserAuthenticator.java`, `.../groups/GroupValidator.java`, `.../configuration/GroupConfiguration.java`, `src/main/proto/Groups.proto`. Server behaviour only; the deployed server may differ. |
| G4 | Signal-Server | https://github.com/signalapp/Signal-Server @ v20261022.0.0 (`8edf3488f7d97398db7ffa5184c6b769a272ff59`): `service/src/main/java/org/whispersystems/textsecuregcm/controllers/CertificateController.java`, `.../entities/GroupCredentials.java`, `.../auth/RedemptionRange.java`. Server behaviour only. |
| G5 | Reference jars | `/work/third_party/signal-cli-0.14.7/lib/`: `signal-service-java-2.15.3_unofficial_151.jar` (bundled Wire-formatted `Groups.proto`/`DecryptedGroups.proto`, identical in content to G2), offline `jshell` runs (JDK 25.0.5) that encoded hand-built Wire messages and called `GroupInviteLinkUrl.fromUri/forGroup`, `GroupId.fromBase64`, `GroupSecretParams.deriveFromMasterKey` and okhttp `Credentials.basic` on dummy inputs. Results are in `testdata/vectors/groups.json`. |

No Signal server was contacted. The reference binary was not run against
any account (group commands need a registered account, so there is no
black-box observation beyond what `spec/surface/cli/` already records).

## 1. Identifiers

| Item | Definition | Text form in signal-cli |
|------|-----------|--------------------------|
| Master key | 32 random bytes; the only secret a member needs. Carried in every group message (`GroupContextV2.masterKey`) and in invite links. | never printed |
| Group secret params | derived from the master key (crypto spec). New groups get freshly generated secret params, and the master key is taken from them. | — |
| Group public params | derived from the secret params; sent as `Group.publicKey` on creation and as the auth username (§3.3). | lowercase hex in the auth header |
| Group ID (v2) | the 32-byte *group identifier* of the public params. Used as the local key, in output, in the sealed-sender USMC `groupId` of data messages, in typing messages and send logs. | standard base64 with padding (44 chars) |
| Group ID (v1) | 16 bytes (legacy) | standard base64 with padding (24 chars) |
| Distribution ID | random version-4 UUID created when the local record is first created; persisted; unique per group (§5) | — |
| Invite link password | 16 random bytes, or empty when no link was ever set | URL-safe base64 without padding in URLs |

Vectors: `groups.json` `group_id_derivation`.

**Parsing a group ID** (CLI `-g`, JSON-RPC `groupId`): Java's basic base64
decoder, i.e. RFC 4648 standard alphabet, padding optional, no `-`/`_`, no
whitespace. 16 decoded bytes give a v1 ID, 32 bytes a v2 ID, any other
length is rejected with `Invalid group id of size <n>`. The full message
is `Failed to decode groupId (must be base64) "<input>": <cause>`, which
commands prefix with `Invalid group id: `. Vectors: `group_id_parse`.

A v1 ID maps to a v2 ID through a "migration master key" (HKDF of the v1
ID with info `GV2 Migration`, 32 bytes, crypto spec). signal-cli uses it
only to find a migrated v1 record when a v2 message arrives (§6.2).

## 2. Wire formats

All group messages are **proto3** and come from G2 `Groups.proto` /
`DecryptedGroups.proto` (identical to the copies bundled in the G5 jar).
`GroupContextV2` is proto2 (`SignalService.proto`). The Wire encoder emits
set fields in ascending field-number order and omits proto3 defaults (0,
false, empty bytes/string, enum 0, empty repeated). A sub-message that is
set but empty is emitted as tag plus length 0. Vectors:
`groups.json` `proto_encodings`.

### 2.1 Enums

**Member.Role:** UNKNOWN 0, DEFAULT 1, ADMINISTRATOR 2.

**AccessControl.AccessRequired:** UNKNOWN 0, ANY 1, MEMBER 2,
ADMINISTRATOR 3, UNSATISFIABLE 4.

**EnabledState** (decrypted forms only): UNKNOWN 0, ENABLED 1, DISABLED 2.

### 2.2 Encrypted field contents

The server stores only ciphertexts. In the tables below:

| Shorthand | Contents |
|-----------|----------|
| SID-CT | a service-ID ciphertext (ACI or PNI, encrypted with the group secret params) |
| PK-CT | a profile-key ciphertext (bound to the member's ACI) |
| PKC-PRES | a profile-key credential presentation (carries an ACI ciphertext and a PK-CT) |
| BLOB(x) | blob encryption, with the group secret params, of the serialized `GroupAttributeBlob` whose oneof is set to x |
| RAWBLOB | blob encryption of raw UTF-8 bytes (no `GroupAttributeBlob` wrapper); empty bytes for an empty or absent value |

### 2.3 Stored group state

**Member**

| # | Name | Type | Contents |
|---|------|------|----------|
| 1 | userId | bytes | SID-CT of the ACI |
| 2 | role | Role | |
| 3 | profileKey | bytes | PK-CT |
| 4 | presentation | bytes | PKC-PRES. Clients send this instead of 1 and 3; the server converts it. |
| 5 | joinedAtVersion | uint32 | revision at which the member was added |
| 6 | labelEmoji | bytes | RAWBLOB |
| 7 | labelString | bytes | RAWBLOB |

**MemberPendingProfileKey** ("pending member", an invitee)

| # | Name | Type | Contents |
|---|------|------|----------|
| 1 | member | Member | only `userId` (SID-CT of an ACI or PNI) and `role` are used |
| 2 | addedByUserId | bytes | SID-CT of the inviter's ACI |
| 3 | timestamp | uint64 | ms since epoch, set by the server |

**MemberPendingAdminApproval** ("requesting member")

| # | Name | Type | Contents |
|---|------|------|----------|
| 1 | userId | bytes | SID-CT |
| 2 | profileKey | bytes | PK-CT |
| 3 | presentation | bytes | PKC-PRES (client form) |
| 4 | timestamp | uint64 | ms, server-set |

**MemberBanned**

| # | Name | Type | Contents |
|---|------|------|----------|
| 1 | userId | bytes | SID-CT |
| 2 | timestamp | uint64 | ms, server-set |

**AccessControl**

| # | Name | Type | Meaning |
|---|------|------|---------|
| 1 | attributes | AccessRequired | who may change title, description, avatar, timer |
| 2 | members | AccessRequired | who may add members |
| 3 | addFromInviteLink | AccessRequired | ANY = link joins directly; ADMINISTRATOR = link creates a join request; UNSATISFIABLE (or UNKNOWN) = link disabled |
| 4 | memberLabel | AccessRequired | who may set member labels (signal-cli never changes it) |

**Group**

| # | Name | Type | Contents |
|---|------|------|----------|
| 1 | publicKey | bytes | serialized group public params |
| 2 | title | bytes | BLOB(title) |
| 11 | description | bytes | BLOB(descriptionText) |
| 3 | avatarUrl | string | CDN 0 key of the encrypted avatar, or empty |
| 4 | disappearingMessagesTimer | bytes | BLOB(disappearingMessagesDuration) |
| 5 | accessControl | AccessControl | |
| 6 | version | uint32 | the revision |
| 7 | members | repeated Member | |
| 8 | membersPendingProfileKey | repeated MemberPendingProfileKey | |
| 9 | membersPendingAdminApproval | repeated MemberPendingAdminApproval | |
| 10 | inviteLinkPassword | bytes | 16 bytes or empty |
| 12 | announcements_only | bool | |
| 13 | members_banned | repeated MemberBanned | |
| 14 | terminated | bool | group ended; no further changes or sends |

**GroupAttributeBlob** (the plaintext inside BLOB; one oneof `content`)

| # | Name | Type |
|---|------|------|
| 1 | title | string |
| 2 | avatar | bytes (the avatar image) |
| 3 | disappearingMessagesDuration | uint32 (seconds) |
| 4 | descriptionText | string |

A oneof member set to its default is still emitted (`title=""` encodes as
`0a 00`, duration 0 as `18 00`).

**GroupInviteLink**: oneof `contents` with only `contentsV1` = 1, a
message with `groupMasterKey` 1 (bytes) and `inviteLinkPassword` 2
(bytes).

**GroupJoinInfo** (response of the join-info endpoint)

| # | Name | Type | Contents |
|---|------|------|----------|
| 1 | publicKey | bytes | |
| 2 | title | bytes | BLOB(title) |
| 8 | description | bytes | BLOB(descriptionText) |
| 3 | avatar | string | CDN key |
| 4 | memberCount | uint32 | |
| 5 | addFromInviteLink | AccessRequired | |
| 6 | version | uint32 | current revision |
| 7 | pendingAdminApproval | bool | the caller already has a pending join request |
| 9 | pendingAdminApprovalFull | bool | sent by the server (G3); not in the client schema, ignored |

### 2.4 GroupChange and its actions

**GroupChange** (signed change as stored and distributed)

| # | Name | Type | Contents |
|---|------|------|----------|
| 1 | actions | bytes | serialized `GroupChange.Actions` |
| 2 | serverSignature | bytes | server notary signature over field 1's bytes |
| 3 | changeEpoch | uint32 | highest feature epoch used by the actions (table below) |

**GroupChange.Actions**

| # | Name | Card. | Action message | Epoch |
|---|------|-------|----------------|-------|
| 1 | sourceUserId | bytes | SID-CT of the editor, filled in by the server (§3.5.3) | |
| 25 | group_id | bytes | the group identifier; **set only by the server**; a client that sets it gets 400 | |
| 2 | version | uint32 | the revision this change produces | |
| 3 | addMembers | repeated | AddMemberAction {1 added: Member, 2 joinFromInviteLink: bool} | 0 |
| 4 | deleteMembers | repeated | DeleteMemberAction {1 deletedUserId: SID-CT} | 0 |
| 5 | modifyMemberRoles | repeated | ModifyMemberRoleAction {1 userId: SID-CT, 2 role} | 0 |
| 6 | modifyMemberProfileKeys | repeated | ModifyMemberProfileKeyAction {1 presentation, 2 user_id: SID-CT, 3 profile_key: PK-CT}; clients send 1, the server stores 2 and 3 | 0 |
| 7 | addMembersPendingProfileKey | repeated | AddMemberPendingProfileKeyAction {1 added: MemberPendingProfileKey} | 0 |
| 8 | deleteMembersPendingProfileKey | repeated | DeleteMemberPendingProfileKeyAction {1 deletedUserId: SID-CT, byte-identical to the stored invitee's} | 0 |
| 9 | promoteMembersPendingProfileKey | repeated | PromoteMemberPendingProfileKeyAction {1 presentation, 2 user_id, 3 profile_key} | 0 |
| 10 | modifyTitle | single | ModifyTitleAction {1 title: BLOB(title)} | 0 |
| 11 | modifyAvatar | single | ModifyAvatarAction {1 avatar: string CDN key} | 0 |
| 12 | modifyDisappearingMessageTimer | single | ModifyDisappearingMessageTimerAction {1 timer: BLOB(duration)} | 0 |
| 13 | modifyAttributesAccess | single | {1 attributesAccess} | 0 |
| 14 | modifyMemberAccess | single | {1 membersAccess} | 0 |
| 15 | modifyAddFromInviteLinkAccess | single | {1 addFromInviteLinkAccess} | 1 |
| 16 | addMembersPendingAdminApproval | repeated | {1 added: MemberPendingAdminApproval} | 1 |
| 17 | deleteMembersPendingAdminApproval | repeated | {1 deletedUserId: SID-CT} | 1 |
| 18 | promoteMembersPendingAdminApproval | repeated | {1 userId: SID-CT, 2 role} | 1 |
| 19 | modifyInviteLinkPassword | single | {1 inviteLinkPassword: bytes} | 1 |
| 20 | modifyDescription | single | {1 description: BLOB(descriptionText)} | 2 |
| 21 | modify_announcements_only | single | {1 announcements_only: bool} | 3 |
| 22 | add_members_banned | repeated | {1 added: MemberBanned} | 4 |
| 23 | delete_members_banned | repeated | {1 deletedUserId: SID-CT} | 4 |
| 24 | promote_members_pending_pni_aci_profile_key | repeated | {1 presentation, 2 user_id: SID-CT of ACI, 3 pni: SID-CT of PNI, 4 profile_key: PK-CT} | 5 |
| 26 | modifyMemberLabels | repeated | {1 userId: SID-CT, 2 labelEmoji: RAWBLOB, 3 labelString: RAWBLOB} | 6 |
| 27 | modifyMemberLabelAccess | single | {1 memberLabelAccess} | 6 |
| 28 | terminate_group | single | TerminateGroupAction {} (empty message) | 7 |

"single" = an optional sub-message (absent means "no change"). The epoch
column is the server's rule for `changeEpoch` (G3): the maximum over the
actions present; 0 if none of the marked ones is present. signal-cli
decrypts epochs up to **7**; a change with a higher epoch is treated as
undecryptable (§4.2).

**GroupChanges** (logs response)

| # | Name | Type |
|---|------|------|
| 1 | groupChanges | repeated GroupChangeState {1 groupChange: GroupChange, 2 groupState: Group} |
| 2 | group_send_endorsements_response | bytes (serialized endorsement response; crypto spec) |

**GroupResponse**: 1 group (Group), 2 group_send_endorsements_response
(bytes).

**GroupChangeResponse**: 1 group_change (GroupChange, the signed change),
2 group_send_endorsements_response (bytes).

**AvatarUploadAttributes** (all strings): 1 key, 2 credential, 3 acl,
4 algorithm, 5 date, 6 policy, 7 signature.

**ExternalGroupCredential**: 1 token (string). signal-cli never requests
it.

### 2.5 GroupContextV2 in messages

`DataMessage.groupV2` is field 15 (`content.md` §4); `StoryMessage.group`
is field 2 (`stories.md` §1.1). Both use:

| # | Name | Type | Notes |
|---|------|------|-------|
| 1 | masterKey | bytes | 32 bytes, required |
| 2 | revision | uint32 | required; 0 is emitted explicitly (proto2 optional) |
| 3 | groupChange | bytes | serialized signed `GroupChange` (§2.4), only on group-update messages |

Sending: signal-cli includes `groupChange` only if the signed change is at
most **2048 bytes**; a larger change is silently dropped from the message
(receivers then fetch from the server).

Receiving: a context without `masterKey` or `revision`, or whose master
key is not 32 bytes, makes the whole envelope invalid (content validation,
`content.md`). An empty `groupChange` counts as absent.

### 2.6 Decrypted forms (what signal-cli stores)

signal-cli stores the decrypted group as a serialized `DecryptedGroup` in
the `group_data` column (§5). Field numbers deliberately mirror `Group`:

**DecryptedGroup**

| # | Name | Type |
|---|------|------|
| 2 | title | string (trimmed) |
| 3 | avatar | string (CDN key) |
| 4 | disappearingMessagesTimer | DecryptedTimer {1 duration: uint32} |
| 5 | accessControl | AccessControl (copied verbatim) |
| 6 | revision | uint32 |
| 7 | members | repeated DecryptedMember |
| 8 | pendingMembers | repeated DecryptedPendingMember |
| 9 | requestingMembers | repeated DecryptedRequestingMember |
| 10 | inviteLinkPassword | bytes |
| 11 | description | string (trimmed) |
| 12 | isAnnouncementGroup | EnabledState (ENABLED or DISABLED after a fetch) |
| 13 | bannedMembers | repeated DecryptedBannedMember |
| 14 | terminated | bool |
| 64 | isPlaceholderGroup | bool (never set by signal-cli) |

**DecryptedMember**: 1 aciBytes (16-byte ACI), 2 role, 3 profileKey (32
bytes), 5 joinedAtRevision (uint32), 6 pniBytes (only from a PNI
promotion), 7 labelEmoji (string), 8 labelString (string).

**DecryptedPendingMember**: 1 serviceIdBytes (service-ID binary: 16 bytes
for an ACI, 17 bytes with prefix 0x01 for a PNI), 2 role, 3 addedByAci, 4
timestamp, 5 serviceIdCipherText (the SID-CT exactly as stored by the
server; needed to revoke the invite).

**DecryptedRequestingMember**: 1 aciBytes, 2 profileKey, 4 timestamp.

**DecryptedBannedMember**: 1 serviceIdBytes, 2 timestamp.

**DecryptedGroupChange** mirrors `Actions` (same numbers 1..28, 25
reserved): 1 editorServiceIdBytes, 2 revision, 3 newMembers
(DecryptedMember), 4 deleteMembers (ACI bytes), 5 modifyMemberRoles {1
aciBytes, 2 role}, 6 modifiedProfileKeys (DecryptedMember), 7
newPendingMembers, 8 deletePendingMembers {1 serviceIdBytes, 2
serviceIdCipherText}, 9 promotePendingMembers, 10 newTitle {1 value}, 11
newAvatar {1 value}, 12 newTimer, 13 newAttributeAccess, 14
newMemberAccess, 15 newInviteLinkAccess, 16 newRequestingMembers, 17
deleteRequestingMembers (bytes), 18 promoteRequestingMembers {1 aciBytes,
2 role}, 19 newInviteLinkPassword, 20 newDescription {1 value}, 21
newIsAnnouncementGroup, 22 newBannedMembers, 23 deleteBannedMembers, 24
promotePendingPniAciMembers, 26 modifyMemberLabels {1 aciBytes, 2
labelEmoji, 3 labelString}, 27 newMemberLabelAccess, 28 terminateGroup
(bool). It is never stored; it exists only while applying a change.

**DecryptedGroupJoinInfo**: 2 title, 3 avatar, 4 memberCount, 5
addFromInviteLink, 6 revision, 7 pendingAdminApproval, 8 description, 9
isAnnouncementGroup (never filled).

A Go implementation may store group state in any format; the
`DecryptedGroup` encoding matters only for reading an existing signal-cli
database (vector in `groups.json`).

## 3. The groups service

### 3.1 Host and transport

| Item | Value |
|------|-------|
| Base URL (staging) | `https://storage-staging.signal.org` (production `https://storage.signal.org`) (G1 `StagingConfig`/`LiveConfig`) |
| Transport | **OkHttp REST** (HTTP/1.1 or HTTP/2 over TLS), not the chat WebSocket. TLS trust, proxy, timeouts and connection pool as for the other OkHttp clients (`transport.md` §5: truststore with the Signal RSA root; 30 s connect/read). |
| Headers | `Authorization` (§3.3); `User-Agent` from signal-cli's interceptor (`transport.md` §4.2); no `X-Signal-Agent`. Bodies are `Content-Type: application/x-protobuf`. Responses are raw protobuf. |
| Paths | as in §3.4; note the trailing slash on `/v2/groups/` |

The **auth credentials** themselves come from the chat server over the
authenticated chat WebSocket (§3.2). The avatar CDN is CDN 0
(`https://cdn-staging.signal.org`, `attachments.md`).

### 3.2 Auth credentials (`GET /v1/certificate/auth/group`)

Request on the **authenticated chat WebSocket** (G2
`GroupsV2ApiHelper`):

`GET /v1/certificate/auth/group?redemptionStartSeconds=<T>&redemptionEndSeconds=<T+604800>`

where T = start of the current **UTC day** in epoch seconds (floor of
now-in-ms / 86 400 000, times 86 400). No `v101` parameter is sent.

Response 200, JSON (G4 `GroupCredentials`):

| Key | Type | Notes |
|-----|------|-------|
| `credentials` | array of `{credential: base64 string, redemptionTime: integer seconds}` | one per day from T to T+7 days inclusive (8 entries). Each `credential` is a serialized auth-credential-with-PNI response (crypto spec). |
| `callLinkAuthCredentials` | same shape | parsed into call-link credentials; unused by group code |
| `pni` | UUID string or null | ignored (unknown keys are ignored) |

Server rules (G4 `RedemptionRange`): start and end must be day-aligned,
start ≥ yesterday, end ≤ today + 8 days, end − start ≤ 7 days; otherwise
400. A non-200 answer surfaces as an I/O error of the calling operation.

**Caching in signal-cli** (G1 `GroupV2Helper`):

- In memory only, keyed by redemption time; never written to disk. A
  fresh process fetches on its first group operation.
- Before every group request the client looks up today's T. If the map
  is missing or has no entry for T, it fetches a new 7-day batch
  (replacing the map).
- It then receives the credential for T (with the account's ACI and PNI)
  and builds a fresh auth presentation for the group (randomized each
  time). If receiving fails verification, it clears the map, fetches once
  more, and tries again; a second failure is an I/O error wrapping the
  verification failure.
- The cache is cleared when the account's ACI/PNI change (G1
  `AccountHelper`).

### 3.3 Authorization header

`Authorization: Basic <base64(username ":" password)>` (standard base64
with padding, ISO-8859-1 bytes), where:

- username = lowercase hex, no separators, of the serialized **group
  public params**;
- password = lowercase hex of the serialized **auth credential
  presentation** for this group (crypto spec).

A new presentation is made per request. The server decodes both hex
strings, verifies the presentation against the public params and derives
the caller's ACI (and PNI) ciphertexts from it (G3
`GroupUserAuthenticator`); a failure is 401. Vector:
`groups.json` `http.authorization_header`.

### 3.4 Endpoints used by signal-cli

| Purpose | Method and path | Request body | 2xx body |
|---------|-----------------|--------------|----------|
| Create | `PUT /v2/groups/` | Group | GroupResponse (ignored by signal-cli) |
| Current state | `GET /v2/groups/` | — | GroupResponse |
| Change | `PATCH /v2/groups/` or `PATCH /v2/groups/?inviteLinkPassword=<pw>` | GroupChange.Actions | GroupChangeResponse |
| History | `GET /v2/groups/logs/<fromRevision>?maxSupportedChangeEpoch=7&includeFirstState=false&includeLastState=false` with header `Cached-Send-Endorsements` | — | GroupChanges (200 or 206) |
| Join info | `GET /v2/groups/join/<pw>` | — | GroupJoinInfo |
| Avatar upload form | `GET /v2/groups/avatar/form` | — | AvatarUploadAttributes |
| Avatar upload | `POST https://cdn-staging.signal.org/` (multipart form, §3.5.6) | — | — |
| Avatar download | `GET https://cdn-staging.signal.org/<avatarUrl>` | — | encrypted blob |

`<pw>` is the invite-link password, URL-safe base64 **without padding**;
for the join-info path an absent password gives an empty segment
(`/v2/groups/join/`). Vector: `http.invite_link_password_query`.

Present in G2 but **never called** by signal-cli: `GET
/v2/groups/joined_at_version` (returns a `Member` with only
`joinedAtVersion`), `GET /v2/groups/token` (ExternalGroupCredential).

### 3.5 Endpoint details

#### 3.5.1 Response handling common to all group endpoints (G2 `makeStorageRequest`)

- A 2xx other than 204 is success; the body is decoded as the expected
  protobuf.
- Otherwise the endpoint-specific rules below run first. Each of them
  starts with a check that the error response carries an
  `X-Signal-Timestamp` header; if it does not, the error becomes "500
  Missing timestamp header" regardless of the real status (avatar form
  and token endpoints skip this check).
- Then the generic mapping: 204 → "no content"; 401 or 403 →
  authorization failed; 404 → not found; 409 → conflict (the variant
  carrying the body, which is what OkHttp always produces); 429 → rate
  limit; 499 → deprecated version; anything else → generic non-success.

Error message texts (they appear in CLI output as `<msg>`): a bare status
gives `StatusCode: <n>`; a status with text gives `[<n>] <text>`. The
generic mapping yields `[401] Authorization failed!` (or 403), `[404] Not
found`, `[409] Conflict`, `[429] Rate limit exceeded: 429`, and for other
codes `[<n>] Response: <OkHttp response description>`.

#### 3.5.2 GET current state

Endpoint-specific mapping: 403 → "not in group" (`StatusCode: 403`), 404 →
"group not found" (`StatusCode: 404`).

Server (G3): 404 if the group does not exist. 200 for full members and
for invited (pending profile key) members; requesting and other callers
get 403. The endorsement response is included only for **full members**
of a **non-terminated** group.

signal-cli treats 403 as "not a member" (§6.1) and any other failure
(I/O, other status, decryption or verification failure) as "could not
fetch" (logged warning `Failed to retrieve Group V2 info, ignoring: <msg>`,
result: no state).

#### 3.5.3 PATCH change

Query `inviteLinkPassword` is sent only by the join flow (§9). The body is
the **unsigned** `GroupChange.Actions`, with `version` = current revision
+ 1. signal-cli sets `sourceUserId` to the raw (unencrypted) service-ID
binary of the account on some changes and leaves it empty on others; the
server always replaces it with the authenticated caller's SID-CT, so its
value does not matter.

Endpoint-specific mapping: 400 → "patch not accepted" (`StatusCode: 400`,
or `[400] <message>` when the body is JSON `{"code":…, "message":"…"}`);
423 → "group terminated" (`StatusCode: 423`). 409 falls to the generic
conflict.

Server rules (G3 `modifyGroup`), relevant to clients:

- 400 if the group does not exist (`No such group exists`), if
  `group_id` is set, or if any validation fails (e.g. title too long,
  group size above the configured maximum, duplicate members, timer blob
  over 42 bytes, invalid avatar key, invite password not 16 bytes).
- **409 if `version` ≠ stored revision + 1**, or if a concurrent write
  won. The 409 has no body.
- 423 if the group is terminated (no change at all is allowed, not even
  leaving).
- 403 if the caller is not permitted (access control).
- The server validates and rewrites member additions (presentations
  become userId/profileKey ciphertexts, timestamps are added), sets
  `sourceUserId` and `group_id`, computes `changeEpoch`, signs the
  serialized actions, and returns `GroupChangeResponse`. The endorsement
  response is included only if the caller is still a full member of a
  non-terminated group after the change.

#### 3.5.4 GET history (logs)

Header `Cached-Send-Endorsements: <seconds>` = the stored endorsement
expiration of this group in ms (0 if none), divided by 1000 (§5). The
same value is sent on every page. The server requires the header (400
without it).

Endpoint-specific mapping: as for GET current state (403, 404).

Server (G3 `getGroupLogs`):

- 404 if no such group; 403 if the caller is not a full member, or if
  `fromRevision` is below the caller's `joinedAtVersion`.
- If `fromRevision` > current revision: 200 with an empty `GroupChanges`.
- Page size 64 (the client sends no `limit`). If more than 64 revisions
  remain: **206** with `Content-Range: versions <from>-<from+63>/<latest>`
  and no endorsement response.
- Last page: 200; it carries an endorsement response if the group is not
  terminated and either the cached expiration is less than 6 hours away
  or the page contains a membership change.

Client paging: a 206 whose `Content-Range` matches exactly `versions
<a>-<b>/<c>` (decimal integers) means "more pages; next fromRevision =
b + 1". A 206 with a missing or non-matching header is an error
("Unable to parse content range header on 206"). A 200 means "no more
pages". Vectors: `groups.json` `content_range` (copied from G2 tests).

Each entry's `groupState` is decrypted if present and `groupChange` is
decrypted **without** signature verification ("already
trusted"); an entry whose epoch is above 7 yields no change. signal-cli
ignores the endorsement response of history pages.

#### 3.5.5 GET join info

Endpoint-specific mapping: 403 → "forbidden" carrying the
`X-Signal-Forbidden-Reason` header value; 423 → "group terminated".
signal-cli maps 403 to "group link not active" with reason **BANNED** if
the header value equals `banned` (case-insensitive), else **UNKNOWN**.

Server (G3): 404 if the group does not exist. Unless the caller already
has a pending join request: 403 if the password does not match, or if
`addFromInviteLink` is UNSATISFIABLE or UNKNOWN. 403 with
`X-Signal-Forbidden-Reason: banned` if the caller is banned. 423 if
terminated.

#### 3.5.6 Avatar upload

1. `GET /v2/groups/avatar/form` with the group's auth (works before the
   group exists). Server: 403 if the caller may not modify attributes of
   an existing group, 423 if terminated. The form's `key` is
   `groups/<URL-safe base64 of the group ID, no padding>/<URL-safe base64
   of 16 random bytes, no padding>`, `acl` `private`, `algorithm`
   `AWS4-HMAC-SHA256`, policy limited to 3 MiB.
2. Ciphertext = BLOB(avatar) of the raw image bytes (the whole file, no
   resizing or type check).
3. `POST` to the CDN 0 base URL plus `/` (the form's path is empty), as
   `multipart/form-data` with parts in this order: `acl`, `key`,
   `policy`, `Content-Type` = `application/octet-stream`,
   `x-amz-algorithm`, `x-amz-credential`, `x-amz-date`,
   `x-amz-signature`, then `file` (filename `file`, the ciphertext). Any
   2xx is success; other codes are I/O errors.
4. The form's `key` becomes `Group.avatarUrl` (creation) or
   `ModifyAvatarAction.avatar` (update).

#### 3.5.7 Avatar download

`GET <CDN 0 base>/<avatarUrl>` into a temporary file, at most 10 MiB
(`attachments.md`). The content is decrypted as a blob and the
`GroupAttributeBlob.avatar` bytes are written to the avatar store
(`avatars/group-<id>`). Any failure logs `Failed to download avatar for
group <base64 id>, ignoring: <msg>` and is otherwise ignored.

#### 3.5.8 PUT create

Endpoint-specific mapping: 409 → "group exists" (`StatusCode: 409`).

Server (G3 `createGroup`): 400 unless `version` = 0, `publicKey` and
`title` are non-empty, the timer blob is at most 42 bytes, `attributes`
and `members` access are known values, the avatar key (if any) belongs to
this group, the creator is a member with role ADMINISTRATOR, there are no
duplicate users, no requesting or banned members, and size/length limits
hold. 403 if `publicKey` differs from the auth username's params. 409 if
the group already exists. The response includes an endorsement response
(unless the group is created terminated).

## 4. Decrypting and applying group state

### 4.1 Decrypting a Group (G2 `GroupsV2Operations.decryptGroup`)

- **Members:** if `presentation` is non-empty, the ACI and profile key
  come from the presentation; otherwise from `userId` and `profileKey`.
  The service ID must be an ACI; a PNI, or any member that fails to
  decrypt, makes the whole group invalid (the fetch fails). Labels are
  RAWBLOB-decrypted; a failure gives an empty label.
- **Pending members:** `userId` decrypts to an ACI or PNI. An
  undecryptable one becomes the **unknown ACI** (all-zero UUID) rather
  than an error. The ciphertext is kept. `addedByUserId` must decrypt to
  an ACI (else the group is invalid). A role other than DEFAULT or
  ADMINISTRATOR becomes DEFAULT.
- **Requesting members:** like members (ACI required). From a
  presentation, the timestamp is not copied.
- **Banned:** service ID (ACI or PNI) and timestamp.
- **Blobs** (title, description, timer, avatar content): empty input or
  input shorter than **29 bytes** gives an empty `GroupAttributeBlob`;
  decryption or parse failure also gives an empty one (logged). Title and
  description are trimmed of leading/trailing whitespace; an absent title
  or description is the empty string; an absent timer is 0.
- `isAnnouncementGroup` = ENABLED if `announcements_only`, else DISABLED.
  Access control, avatar key, revision, invite password and `terminated`
  are copied.

### 4.2 Decrypting a change (G2 `decryptChange`)

There are three modes:

- **Verified** (a change received peer-to-peer in a message): parse
  `GroupChange`; if `changeEpoch` > 7 the change is ignored (no result).
  Verify `serverSignature` over the `actions` bytes with the server's
  public params; then require `Actions.group_id` to equal the expected
  group identifier. Any failure means "no change".
- **Already trusted** (history pages): no verification; epoch > 7
  ignored.
- **Own unsigned actions** (§8.2): decrypt the client-built actions with
  the editor fixed to the own ACI.

Per action:

- editor = `sourceUserId` decrypted (any service ID), unless given.
- new members: from presentation or ciphertexts; `joinedAtRevision` =
  the change's revision.
- deleted members, role changes, approvals: ACI required.
- profile-key changes and promotions of invitees: from the presentation
  if `user_id` or `profile_key` is empty, otherwise from those two; ACI
  required. Promoted invitees get role DEFAULT and `joinedAtRevision`
  = −1, i.e. the uint32 value 4294967295 once encoded; this value ends up
  in the stored state of such a member. Profile-key entries carry role
  UNKNOWN and the same −1.
- new invitees and invite removals: service ID or unknown ACI, keeping
  the ciphertext.
- PNI promotions: ACI from `user_id`, PNI from `pni` (must be a PNI, else
  invalid), profile key; role DEFAULT; `joinedAtRevision` = change
  revision; **the editor is replaced by that ACI**.
- title, description, timer: blob rules of §4.1. Avatar key, access
  levels, password, announcements flag (ENABLED/DISABLED), bans
  (timestamps from the action), labels, terminate flag: copied/decrypted.
- **PNI editor fix-up:** if the editor is a PNI: with exactly one added
  member, the editor becomes that member's ACI if the add is flagged
  `joinFromInviteLink`, else the editor is cleared; otherwise, if the
  change has no invite removals and no PNI promotions, the editor is
  cleared.

### 4.3 Applying a change to a DecryptedGroup (G2 `DecryptedGroupUtil.apply`)

Precondition: change revision = state revision + 1, otherwise the change
cannot be applied. The result takes the change's revision. Effects, in
this order:

| Step | Effect | Failure that rejects the whole change |
|------|--------|---------------------------------------|
| add members | insert or replace by ACI (replacement keeps the original position, new ones go to the end); then drop pending and requesting entries whose service ID/ACI is now a full member | — |
| delete members | remove by ACI; unknown ACIs are skipped | — |
| role changes | set role | ACI not a member; role not DEFAULT/ADMINISTRATOR |
| profile-key changes | set the member's profile key | ACI not a member |
| add invitees | append unless an invitee with the same ciphertext exists | the service ID is already a full member |
| remove invitees | remove by ciphertext; unknown skipped | — |
| promote invitees | remove the invitee matching the ACI, append the member | no invitee with that ACI |
| title, description, avatar, timer | replace if present | — |
| announcements | replace unless UNKNOWN | — |
| access levels (attributes, members, link, label) | replace each unless UNKNOWN (creating the access-control object if absent) | — |
| add requesting | append | — |
| delete requesting | remove by ACI; unknown skipped | — |
| approve requesting | remove the request, append a member with its profile key and the given role (no `joinedAtRevision`); unknown skipped | role not DEFAULT/ADMINISTRATOR |
| invite password | replace if non-empty | — |
| add bans | append unless already banned | — |
| delete bans | remove by service ID; unknown skipped | — |
| PNI promotions | remove the invitee whose service ID is the PNI, append the member | no invitee with that PNI |
| labels | set emoji and text on the member | ACI not a member |
| terminate | set `terminated` | — |

### 4.4 Profile keys learned from groups

- **From a single change** (peer-to-peer apply, §6.2): if the editor
  appears in the change's new members, promoted invitees, profile-key
  changes or new requesting members, that profile key is the editor's
  **authoritative** key and **overwrites** the stored one.
- **From history** (§6.3): the same rule per change, but the keys are
  stored **only for recipients that have no profile key yet**. A later
  change in the history wins over an earlier one.
- **From the member list** of any newly stored state: the profile keys of
  members and requesting members are stored only where none is known.
- An invalid profile key is skipped.

## 5. The stored group record (G1 `GroupStore`, `GroupInfoV2`)

SQLite table `group_v2`:

| Column | Contents |
|--------|----------|
| `_id` | internal row ID |
| `storage_id` | storage-service record ID; **re-randomized on every save** of the record |
| `storage_record` | last storage-service record (opaque) |
| `group_id` | 32-byte group ID, unique |
| `master_key` | 32 bytes |
| `group_data` | serialized DecryptedGroup, or NULL when the state was never fetched or was lost (§6.2) |
| `distribution_id` | 16-byte UUID, unique; created randomly (v4) when the in-memory record is first created and saved with it; never changes |
| `endorsement_expiration_time` | ms, default 0 |
| `blocked` | bool |
| `profile_sharing` | bool ("whitelisted") |
| `permission_denied` | bool: last fetch returned 403 |

Table `group_v2_member`: (`group_id` → `group_v2._id`, `recipient_id`,
`endorsement` = serialized group send endorsement), unique per
(group, recipient), cascade-deleted with the group.

Rules:

- **Endorsements** are replaced as a set: on update, the expiration is
  written, all member rows of the group are deleted and the new ones
  inserted. The update is a **no-op if the group row does not exist
  yet**; consequence: the endorsements returned with a group's very first
  fetch or creation are dropped (they are fetched again before the first
  send, §12.2).
- Endorsements are stored for every full member returned by the server,
  **including the account itself**, keyed by recipient.
- `permission_denied` is set when a lazy fill (§6.1) gets 403, and cleared
  whenever non-null state is stored.
- A stored row whose master key or `group_data` cannot be decoded is
  skipped when listing.
- Deleting a group removes the row (and its endorsement rows) and the
  avatar file.
- Saving a group record never touches the endorsement table.
- When a v2 message arrives for a group whose v2 ID belongs to a stored
  **v1** record (by its `group_id_v2` column), the v1 record is deleted
  and a new v2 record (new distribution ID, no state) is created, keeping
  the blocked flag ("local migration").
- Records also arrive from storage-service sync (master key, blocked,
  whitelisted) without state; their state is filled lazily.

`GroupInfoV2` derived values used across the CLI:

| Value | Rule |
|-------|------|
| title / description | from state; null without state |
| members, admins | full members (by ACI); admins have role ADMINISTRATOR |
| pending / requesting / banned | from state; empty without state |
| is member / is pending | self's recipient among full members / pending members |
| message expiration | `disappearingMessagesTimer.duration`, 0 without state |
| announcement group | `isAnnouncementGroup` = ENABLED |
| permission add member / edit details | ONLY_ADMINS if `members` / `attributes` is ADMINISTRATOR; EVERY_MEMBER otherwise, including no access control |
| permission send message | ONLY_ADMINS if announcement group, else EVERY_MEMBER |
| invite link | null if no state, if the password is empty, or if `addFromInviteLink` is set and is neither ANY nor ADMINISTRATOR; otherwise `https://signal.group/#` + URL-safe unpadded base64 of GroupInviteLink{contentsV1{master key, password}} (vector `invite_link`) |

## 6. Keeping group state current

### 6.1 Lazy fill and forced refresh (G1 `GroupHelper`)

Loading a group record (for output, sending, permission checks) runs a
**fill** step for v2 records:

- Without force: if `permission_denied` is set or state exists, nothing
  happens. Otherwise it is a refresh.
- **Refresh:** GET current state (§3.5.2).
  - 403: set `permission_denied`, save, stop.
  - other failure: stop (state unchanged).
  - success: store the endorsements of the response (§5); store profile
    keys from history (§6.3; a 403 there is ignored); store profile keys
    from the members (§4.4); download the avatar if the key is non-empty
    (§3.5.7; always, `--ignore-avatars` does not apply here); set the
    state and save.

Callers and their force flag:

| Caller | Force |
|--------|-------|
| any single-group lookup (`getGroup`), all-groups listing (`listGroups` without `-g`) | no |
| `listGroups -g …` (listing specific groups) | **yes**, each listed v2 group |
| before every update, join-flow follow-up, quit (`getGroupForUpdating`) | yes |
| conflict retry (§8.2) | yes |
| endorsement refresh (§12.2) | yes |

### 6.2 Incoming GroupContextV2 (G1 `getOrMigrateGroup`)

Runs for the group context of every DataMessage, StoryMessage and sent
transcript's DataMessage, before other handling (and once more inside
DataMessage handling, which is then a no-op), and from the join flow (§9).
Inputs: master key, revision R, optional signed change C, and the
`--ignore-avatars` flag.

1. Derive the group ID; load the v2 record, or migrate a v1 record
   (§5), or create a new in-memory record (new distribution ID).
2. If the record has state with revision ≥ R: done (nothing saved).
3. Otherwise compute new state:
   - **P2P path:** if C is present, state exists and its revision + 1 =
     R: decrypt C in verified mode (§4.2). If that yields a change: store
     the editor's authoritative profile key (§4.4) and apply the change
     (§4.3). Failure at any point yields no state from this path.
   - **Fetch path** (if the P2P path produced nothing): GET current state.
     On success: profile keys from history (§6.3), then store the
     response's endorsements. A 403 from the GET (or from the history) is
     swallowed. Other failures yield no state.
4. If new state was obtained: store profile keys from its members (§4.4)
   and download the avatar if the key is non-empty and avatars are not
   ignored.
5. **Set the state to the result and save — even when the result is
   empty.** Consequence: a failed fetch (network error, 403 after being
   removed, verification failure) **erases the stored state** of an
   existing group (`group_data` NULL; `permission_denied` is not set). The
   next lookup then refetches lazily. A first-time record is saved
   without state.
6. Queue a storage-service sync.

A message with a revision gap (R > stored + 1) always takes the fetch
path. There is no use of the history endpoint to fill gaps.

### 6.3 Profile keys from history

Start revision = max(revision at which the account was added, local
revision before the update), where "added at" is the `joinedAtRevision`
of the member whose ACI equals the account's ACI or whose PNI bytes equal
the account's PNI, or the new state's revision if none matches. Local
revision is 0 without state. Then page through §3.5.4 from that revision
until a 200, collecting authoritative keys per §4.4 and finally storing
those for recipients without a key. Any non-403 failure stops paging
(warning `Failed to retrieve Group V2 history, ignoring: <msg>`); 403
propagates as "not a member" to the caller.

### 6.4 Group checks on incoming messages

Before handling, for a message with a group context (DataMessage or
StoryMessage group):

- **Blocked:** the sender is a blocked contact, or the group is blocked →
  dropped (`Ignoring a message from blocked user/group`).
- **Not allowed** (computed before and after the state update of §6.2;
  dropped only if both say "not allowed"):
  - an admin delete from a non-admin or in an unknown group;
  - otherwise, unknown group (no record) → allowed;
  - sender not a full member → not allowed, except an invited member
    sending a group-update DataMessage (signed change present, no
    renderable content);
  - announcement group and sender not admin → not allowed if the message
    is not a DataMessage or has body, attachments, quote, previews,
    mentions or sticker.

"Group-update" in the exception above means: v2 context with a non-empty
`groupChange` and no renderable content (G2 `isGroupV2Update`). Note that
the record lookup in these checks runs the lazy fill (§6.1).

The group's timer for v2 groups comes only from group state; the
DataMessage `expireTimer` of group messages is not stored.

## 7. Creating a group (`updateGroup` without `-g`)

Inputs: name (may be absent → empty string), member set (self removed if
present), optional avatar file (any path or data URI that signal-cli's
attachment reader accepts; read fully into memory).

1. **Own credential.** Get the account's own expiring profile-key
   credential (stored if still valid, else fetched with the profile,
   crypto/profile specs). Without one: warning `Cannot create a V2 group
   as self does not have a versioned profile` and the creation fails.
2. **Members' credentials.** For all members without a valid stored
   credential, fetch profile + credential (up to 10 in parallel; failures
   ignored). A member's credential requires that its profile key is
   known; otherwise none is obtained.
3. **Build the Group:** fresh random group secret params; `version` 0;
   `publicKey`; `title` = BLOB(title = name); timer = BLOB(duration 0);
   access control {attributes MEMBER, members MEMBER}
   (`addFromInviteLink` left UNKNOWN, i.e. no link); members = [self as
   ADMINISTRATOR with presentation] then each member **with** a
   credential as DEFAULT with presentation; every member **without** a
   credential becomes an invitee {member {userId = SID-CT of its service
   ID (ACI, or PNI for a PNI-only contact), role DEFAULT}} with no
   `addedByUserId` (the server fills it). Member order follows a hash
   set and is unspecified.
4. If an avatar is given: upload it (§3.5.6) and set `avatarUrl`.
5. `PUT /v2/groups/` (§3.5.8), then `GET /v2/groups/` for the state. Any
   failure in steps 4–5 (I/O, HTTP error, decryption) logs `Failed to
   create V2 group: <msg>` and the creation fails.
6. Store a new record: random distribution ID, the fetched state,
   profile sharing on, avatar written to the avatar store (the original
   bytes). (Endorsements from step 5 are lost, §5.)
7. Send the group update message (§8.3) with revision 0 and **no**
   `groupChange` to all members and invitees except self.
8. Queue a storage-service sync.

A failed creation surfaces as the I/O error `Failed to create group`.

After creation, `updateGroup` immediately runs the update path (§8) with
the remaining options (name, members and avatar are cleared first). This
always performs a forced refresh of the new group even when there is
nothing to change.

## 8. Modifying a group (`updateGroup -g`)

### 8.1 Preconditions and order of changes

The group must exist (`Group not found: <id>`) and the account must be a
full or invited member (`User is not a member in group: <title> (<id>)`,
title `null` without state). For v2 groups a forced refresh follows.

From the CLI, every member-list option is a (possibly empty) set, never
absent. Each step below that has something to do is **a separate PATCH
and a separate group-update message**, in this order:

| # | Step (only if non-empty after filtering) | Actions sent |
|---|-------------------------------------------|--------------|
| 1 | account is an invitee: accept the invitation | invited by ACI: `promoteMembersPendingProfileKey` [{presentation}]; invited by PNI: `promote_members_pending_pni_aci_profile_key` [{presentation}]. Needs the own credential, else I/O error `Cannot join a V2 group as self does not have a versioned profile`. If the own service ID cannot be found among invitees: I/O error `Cannot find service ID for self to accept invite`. |
| 2 | `-m` entries that are requesting members: approve | `promoteMembersPendingAdminApproval` [{userId: SID-CT(ACI), role DEFAULT}] |
| 3 | `-m` entries that are neither full nor requesting members: add | fetch credentials as in §7.2; with credential: `addMembers` [{added: {role DEFAULT, presentation}}]; without: `addMembersPendingProfileKey` [{added: {member {userId: SID-CT, role DEFAULT}, addedByUserId: SID-CT(own ACI)}}]; any of them currently banned is also unbanned in the same change (`delete_members_banned`) |
| 4 | full members in (`-r` ∪ `--ban`) minus `-m` minus self: remove | `deleteMembers` [{SID-CT(ACI)}] |
| 5 | `-r` entries that are invitees: revoke | `deleteMembersPendingProfileKey` [{the stored ciphertext}] |
| 6 | `-r` entries that are requesting members: refuse | `deleteMembersPendingAdminApproval` [{SID-CT}], ACIs only |
| 7 | `--admin` entries that are full members and not admins | `modifyMemberRoles` [{SID-CT, ADMINISTRATOR}] |
| 8 | `--remove-admin` entries that are admins | `modifyMemberRoles` [{SID-CT, DEFAULT}] |
| 9 | `--ban` entries not already banned | `add_members_banned` [{added {userId SID-CT}}]; if banned count + new > 1001, the oldest bans (by timestamp) are lifted in the same change (`delete_members_banned`) |
| 10 | `--unban` entries that are banned | `delete_members_banned` [{SID-CT}] |
| 11 | `--reset-link` | `modifyInviteLinkPassword` {new random 16 bytes} |
| 12 | `--link` | `modifyAddFromInviteLinkAccess` {enabled → ANY, enabled-with-approval → ADMINISTRATOR, disabled → UNSATISFIABLE}; when not disabled and the group has no password, plus `modifyInviteLinkPassword` {new random 16 bytes} in the same change |
| 13 | `--set-permission-add-member` | `modifyMemberAccess` {every-member → MEMBER, only-admins → ADMINISTRATOR} |
| 14 | `--set-permission-edit-details` | `modifyAttributesAccess` {same mapping} |
| 15 | `-e` | `modifyDisappearingMessageTimer` {BLOB(duration = seconds)} |
| 16 | `--set-permission-send-messages` | `modify_announcements_only` {only-admins → true, every-member → false} |
| 17 | `--member-label-emoji` and/or `--member-label` | `modifyMemberLabels` [{SID-CT(own ACI), emoji RAWBLOB, text RAWBLOB}]; a value not given keeps the own current label; an absent text is sent as empty. Fails with an uncaught error if the account is not a full member. |
| 18 | `-n`, `-d`, `-a` (any of them) | one change with `modifyTitle`, `modifyDescription`, `modifyAvatar` as given (avatar uploaded first, §3.5.6); after success the avatar file is also written to the local avatar store |

Steps 7, 11–16 are sent whenever the option is given, even if the value
equals the current one. Unknown recipients are resolved first (phone
numbers via CDSI); an unregistered one fails the whole command before any
change (`The user <identifier> is not registered.`).

### 8.2 Committing one change

1. `version` = current revision + 1.
2. Decrypt the unsigned actions locally (editor = own ACI, §4.2) and
   apply them to the current state (§4.3). A failure here is an I/O
   error wrapping it; nothing is sent.
3. PATCH (§3.5.3).
4. On success, the **locally computed** state becomes the group state
   (not a server copy; server-filled timestamps are therefore missing
   locally until the next fetch).
5. Send the update message (§8.3) and store the endorsements of the
   response, if any.

**Conflict:** a 409 at any step aborts the remaining steps, forces a
refresh, and re-runs the whole step list once against the fresh state
(steps already applied are filtered out by state where the table filters
by membership; unconditional steps such as `--reset-link` run again). A
second 409 propagates as an I/O error. Other errors propagate
immediately; changes already committed stay committed.

### 8.3 The group-update message

For each committed change (and for creation, join and leave):

- Recipients: full members and invitees **before** the change, united with
  those **after** it, minus self. Removed members thus still receive the
  message.
- DataMessage: `groupV2` {masterKey, revision = new revision, groupChange
  = the signed change from the response (if ≤ 2048 bytes)};
  `expireTimer` = the group's timer (after the change; omitted if 0);
  `expireTimerVersion` default 1; no body, no profile key. Timestamp =
  current time.
- Sent through the group send routine (§12) with content hint IMPLICIT
  and urgent = true, `isRecipientUpdate` false.
- The result of the command is the result of the **last** message sent;
  earlier messages' timestamps and results are not reported.

## 9. Joining via an invite link (`joinGroup`)

### 9.1 Parsing the link (G1 `GroupInviteLinkUrl.fromUri`)

1. Parse as a URI. Unparseable, scheme other than `https`/`sgnl`
   (case-insensitive), or host other than `signal.group`
   (case-insensitive) → not a link (`Link is not a signal group
   invitation link`). User info, port and query are ignored.
2. Path must be empty or `/`, else invalid (`No path was expected in
   uri`).
3. The fragment must be non-empty, else invalid (`No reference was in
   the uri`).
4. Decode the fragment with the lenient Signal base64 decoder (URL-safe
   if it contains `-` or `_`, else standard; padding added if missing).
   Decoder I/O errors → invalid with message `java.io.IOException: <…>`.
   **A fragment that is too short (e.g. one character) raises an
   unchecked error that escapes the command** (generic uncaught-exception
   handling, `spec/surface/cli/index.md`: stack trace, exit 2).
5. Decode GroupInviteLink; no `contentsV1` → unknown version (`Url
   contains no known group link content`). A master key that is not 32
   bytes → invalid with message
   `org.signal.libsignal.zkgroup.InvalidInputException: Length of array
   supplied was <n> expected 32`. A missing password is an empty
   password.

CLI wrappers: invalid → `Group link is invalid: <message>`; unknown
version → `Group link was created with an incompatible version:
<message>`; both exit 1. Vectors: `groups.json` `invite_link_parse`.

### 9.2 Flow (G1 `GroupHelper.joinGroup`, `GroupV2Helper.joinGroup`)

1. GET join info (§3.5.5) with the password. "Link not active" → error
   `Group link inactive (reason: <UNKNOWN|BANNED>)` → CLI `Group link is
   not valid: Group link inactive (reason: …)`, exit 1. Other failures
   (404 → `StatusCode: 404`, 423, I/O) → CLI `Failed to send message:
   <msg> (<ExceptionClass>)`, exit 3.
2. If `pendingAdminApproval` → error `You have already requested to join
   the group.` → CLI `Pending admin approval: You have already requested
   to join the group.`, exit 1.
3. Own credential required (else I/O error `Cannot join a V2 group as
   self does not have a versioned profile`).
4. Actions: `addFromInviteLink` = ADMINISTRATOR →
   `addMembersPendingAdminApproval` [{added: {presentation}}]; otherwise
   `addMembers` [{added: {role DEFAULT, presentation}}]
   (`joinFromInviteLink` not set by the client). `version` = join-info
   revision + 1. PATCH with `?inviteLinkPassword=<pw>`. No local apply.
5. Run §6.2 with revision = join-info revision + 1 and the signed change.
   There is no local state yet, so this is a full fetch. A requester gets
   403 there, so the record is saved without state.
6. **Only requested** (no state after step 5): result = timestamp **0**,
   no results. Nothing is sent.
7. **Joined:** send the update message (§8.3) using the current state and
   the PATCH response (recipients: members and invitees, minus self;
   revision = stored revision); store endorsements; queue storage sync.

Output decides "joined" vs "requested" by looking the group up again
(lazy fill; a requester's 403 sets `permission_denied`) and checking
full membership: plain text `Joined group "<id>"` or `Requested to join
group "<id>"`, then send-failure lines, then the timestamp (which is
`0` for a request). JSON adds `"onlyRequested": true` for a request
(`spec/surface/cli/joinGroup.md`).

## 10. Leaving (`quitGroup`)

1. Group lookup as in §8.1 (not found → CLI `Failed to send to group:
   Group not found: <id>`, exit 1; not a member → logged at info level
   only, nothing printed, processing continues with `--delete`). Forced
   refresh.
2. `--admin` recipients are resolved (unregistered → `The user <id> is
   not registered.`, exit 1) and filtered to full members that are not
   admins yet.
3. If the account is the **only admin**, the group has more than one full
   member, and the filtered `--admin` set is empty: error `User is last
   admin in group: <title> (<id>)` → CLI `You need to specify a new admin
   with --admin: User is last admin in group: …`, exit 1.
4. Actions:
   - account is an **invitee** (by ACI, else by PNI): revoke the own
     invitation: `deleteMembersPendingProfileKey` [{own stored
     ciphertext}];
   - otherwise: `deleteMembers` [{SID-CT(own ACI)}] plus
     `modifyMemberRoles` [{SID-CT, ADMINISTRATOR}] for each filtered
     `--admin` member.
   Committed as in §8.2 (409 → refresh and one retry).
5. The new state (self no longer a member) is saved; endorsements from
   the response are stored if present (none, since the caller is no longer
   a member).
6. Update message with the signed change to the members and invitees of
   the **new** state (minus self), revision = new revision.
7. Output: send results and timestamp (`_send-results.md`).
8. `--delete`: if the account is still a full member, the I/O error `The
   local group information cannot be removed, as the user is still a
   member of the group` (CLI `Failed to send message: … (IOException)`,
   exit 3); otherwise the record, its endorsements and its avatar are
   deleted, and a storage sync is queued.

## 11. Own profile key rotation

When the account's profile key changes, for every v2 group where the
account is a full member and profile sharing is on: forced refresh; if
the own member entry's profile key differs from the account's and an own
credential is available, commit `modifyMemberProfileKeys`
[{presentation}] (§8.2, with one conflict retry) and send the update
message. Errors per group are logged (`Failed to update group profile
key: <msg>`) and ignored.

## 12. Sending to a group

### 12.1 Entry points and recipients

- **Data messages** (`send -g`, reactions, deletes, polls, pins, edits,
  etc.): the group must exist and the account must be a **full** member
  (`Group not found: <id>`; `User is not a member in group: <title>
  (<id>)`). Profile sharing is switched on for the group if it was off.
  Context: `groupV2` {masterKey, revision = stored revision or 0 without
  state}, no `groupChange`; `expireTimer` = group timer. Recipients: full
  members minus self (plus self with `--notify-self`). Content hint
  RESENDABLE. In an announcement group, a non-admin may not send body,
  attachments, quote, previews, mentions or sticker: `User is not allowed
  to send message to group: <title> (<id>)`.
- **Group updates** (§8.3): recipients as stated there; IMPLICIT.
- **Typing** (`sendTyping -g`): full members minus self; typing message
  `groupId` = 32-byte group ID; a non-admin in an announcement group gets
  the "not allowed" error for any typing message.
- **Stories** (§13).

### 12.2 Common routine (G1 `sendGroupMessageInternal`)

1. Recipients flagged unregistered in the local store are skipped and get
   an *unregistered* result (appended at the end).
2. For each remaining recipient: address and unidentified access
   (`send.md` §3).
3. **Endorsements** (v2 groups only): read the stored endorsements and
   expiration. If there are none, or the expiration is less than **2
   hours** away, run a forced refresh (§6.1), which stores the GET
   response's endorsements, and read again. If still none or still
   expiring within 2 hours, the group is treated as having no
   endorsements.
4. **Sender-key targets** (only when the group has a distribution ID,
   i.e. v2, and endorsements exist): a recipient qualifies if it has an
   unidentified access, a valid ACI, a stored endorsement, and a stored
   identity whose trust level is trusted. If fewer than **2** qualify,
   there are no sender-key targets.
5. **Sender-key send** to the targets (§12.3). It returns per-target
   results, or "fall back" for the errors listed in `stories.md` §3.6
   (in which case all targets move to the fan-out). Targets with a failed
   result also move to the fan-out; successes are kept.
6. **Fan-out** (§12.4) to all remaining recipients, if there are any, or
   if there are none at all and the account is multi-device (so that a
   transcript is still produced for a group consisting only of the
   account). Not done for stories.
7. Result list = sender-key successes, fan-out results, skipped
   unregistered.

### 12.3 Sender-key path for group data messages

As `stories.md` §3, with these differences:

- Age rotation (14 days) applies (`stories.md` §3.7).
- SKDM fan-out to targets that need it: `?story=false`; access per
  target: access key first, with the target's **group send token** as
  fallback (`send.md` §3.3 fallbacks; the token replaces the access key
  when a sealed attempt is rejected at the prekey fetch). The token is
  the target's own endorsement turned into a full token with the stored
  expiration (crypto spec).
- USMC group ID = the **32-byte group ID** (not the master key, unlike
  group stories).
- Content hint as given (RESENDABLE for data messages, IMPLICIT for
  updates and typing).
- `PUT /v1/messages/multi_recipient?ts=<T>&online=false&urgent=<u>` (no
  `story`), header **`group-send-token`** = standard base64 with padding
  of the full token made from the **combination of the targets'
  endorsements** (exactly the target set of this attempt) and the stored
  expiration. The server checks the token against the recipient set;
  `uuids404` may list unregistered recipients (`stories.md` §3.4).
- Typing over sender key: online = true, content hint IMPLICIT, no
  transcript.
- After the multi-recipient call, if the account is multi-device and not
  itself a recipient, a sent transcript is sent with the targets'
  results (`sync.md` §4; no destination, `isRecipientUpdate` false).
- Results are written to the send log (for retries), as in
  `content-features.md`.

### 12.4 Fan-out path

- 1:1 sends per recipient (`send.md`), with the DataMessage's group
  context, the group ID in sealed USMCs, the content hint as given.
- Access per recipient: with an access key → sealed with the access key,
  group token as fallback; without an access key but with an endorsement
  → sealed with header **`Group-Send-Token`** (standard base64 with
  padding of the recipient's full token), using the sender certificate of
  the first fan-out recipient that has an access key; if no fan-out
  recipient has an access key there is no certificate and these
  recipients go unsealed; without both → unsealed. A 401 on a sealed
  `PUT` repeats it on the authenticated socket (`send.md` §3.3).
- `isRecipientUpdate` = true if the sender-key part already succeeded for
  someone. The transcript (if multi-device, or if any result needs sync)
  covers only the fan-out results; with both paths used, **two**
  transcripts are sent (the second with `isRecipientUpdate` true).
- An untrusted-identity or no-session error that aborts the whole
  fan-out yields an empty result list for it.

## 13. Group stories (fills `stories.md` §2.4)

`sendStory -g`: the group must exist, the account must be a full member,
and the group must be v2 (`Stories are only supported for V2 groups`).
The routine of §12.2 runs with **no fan-out**:

- Endorsements are needed for target selection exactly as in §12.2 step
  3–4 (refresh when missing or within 2 hours of expiry). Without them,
  or with fewer than 2 qualifying members, nothing is sent and the result
  contains only skipped-unregistered entries.
- The SKDM fan-out uses access key first, token fallback, `?story=false`,
  and the **master key** as USMC group ID (`stories.md` §2.4.6).
- The multi-recipient request uses story authorization (no
  `group-send-token`, `&story=true`).
- Failed targets get no result entry; there is no retry for them.

## 14. Output

### 14.1 `listGroups`

As in `spec/surface/cli/listGroups.md`, with these corrections:

- **The command can contact the groups service.** Without `-g`, every v2
  group with no stored state and no `permission_denied` flag is fetched
  (§6.1). With `-g`, **every listed v2 group is force-refreshed** (GET
  state plus history pages) on every run. Fetch failures are silent;
  such a group is listed from whatever is stored (possibly with null
  name).
- `isMember`/`Active` is full membership; an invited or requesting
  account shows `false`.
- `Link`/`groupInviteLink` follows §5's invite-link rule.
- Permission values follow §5.

### 14.2 Received group messages

`_received-message-output.md` ("Group info:" block, JSON `groupInfo`)
with these precisions:

- `Type: UPDATE` (JSON `"UPDATE"`) whenever the v2 context carries a
  **non-empty `groupChange`**, even if the message also has a body;
  otherwise `DELIVER`. For v1 contexts, UPDATE means type UPDATE.
- `Revision` is the revision in the message, not the stored one.
- The name comes from the local record **after** the state update of
  §6.2. Every incoming v2 context creates a record, so the group is never
  "unknown" for v2 messages; without state the title is null: plain text
  `Name: null`, JSON `"groupName": null` (the key is present; JSON
  `groupInfo` fields are not omit-null).
- The group ID shown is derived from the master key.

### 14.3 `updateGroup`

- Result printed = creation result (if any) plus the **last** update
  message's results, with the last update's timestamp
  (`spec/surface/cli/updateGroup.md`). Intermediate update messages
  (§8.1 runs one per step) are not reported.
- Messages: group not found → `Group not found: <id>`; not a member →
  `User is not a member in group: <title> (<id>)`; both exit 1.
  Creation failure → `Failed to send message: Failed to create group
  (IOException)`, exit 2. A 409 twice →
  `Failed to send message: [409] Conflict (ContactManifestMismatchException)`,
  exit 2. Server rejections of a change →
  `Failed to send message: StatusCode: <n> (<ExceptionClass>)` or the
  generic forms of §3.5.1, exit 2, e.g. `StatusCode: 400
  (GroupPatchNotAcceptedException)`, `[403] Authorization failed!
  (AuthorizationFailedException)` (permission denied by access control),
  `StatusCode: 423 (GroupTerminatedException)`.

### 14.4 Corrections to other specs

- `spec/surface/cli/listGroups.md` says "It makes no network call." Wrong
  (§14.1).
- `spec/surface/cli/updateGroup.md` §Behavior says every other change is
  applied "in one update". Wrong: one PATCH and one update message per
  step (§8.1), and only the last is reported.
- `spec/surface/cli/updateGroup.md`: `--ban` of a current member also
  removes the member (§8.1 step 4); `-m` of a banned user unbans it.
- `spec/surface/cli/joinGroup.md` §Constraints: a fragment that fails to
  decode gives `Group link is invalid: java.io.IOException: …` only for
  decoder I/O errors; a too-short fragment escapes as an uncaught
  exception (§9.1). For a pending join request the printed timestamp is
  `0` and results are empty (§9.2).
- `spec/surface/cli/quitGroup.md`: the `--delete` failure for a still
  member (§10 step 8) exits 3 with `Failed to send message: The local
  group information cannot be removed, as the user is still a member of
  the group (IOException)`.
- `spec/surface/cli/_received-message-output.md`: UPDATE rule and
  null-name behaviour (§14.2); the "group missing from the store" case
  cannot occur for v2 contexts.
- `spec/service/stories.md` §2.4/G4: target selection is now specified
  in §12.2 and §13.

## 15. Checklist for a compatible client

1. Store per group: master key, group ID, a random distribution ID created
   once, decrypted state, blocked, profile-sharing, permission-denied,
   endorsements (+ expiration).
2. Groups REST on `storage-staging.signal.org` with Basic auth (hex public
   params : hex auth presentation); auth credentials from `GET
   /v1/certificate/auth/group` on the chat WebSocket, cached per UTC day.
3. On incoming contexts: if revision = stored + 1 and a signed change is
   present, verify + decrypt + apply; else GET the state. Learn profile
   keys (§4.4, §6.3).
4. Changes: build unsigned actions with `version` = revision + 1, apply
   locally, PATCH, keep the local result, send an update message with the
   signed change; on 409 refresh and retry once.
5. Sends: refresh endorsements when missing or < 2 h from expiry;
   sender key for ≥ 2 qualifying members with a combined `group-send-token`;
   fan-out for the rest with per-recipient tokens as access-key
   alternatives.

## Gaps

- **G1.** No live observation. Everything comes from source; server rules
  come from storage-service and Signal-Server tags that need not match the
  deployed staging servers. In particular the presence of
  `X-Signal-Timestamp` on storage-service error responses (§3.5.1) was
  not verified; if absent, every group error would read `[500] Missing
  timestamp header`.
- **G2.** zkgroup byte sizes and algorithms (ciphertexts, presentations,
  endorsement responses, token serialization, signature verification)
  are deferred to the parallel crypto specs; this file assumes their
  names. The vectors here contain no ciphertexts (they are randomized).
- **G3.** `--notify-self` on a group send puts the account itself among
  the recipients; since the account has its own endorsement, it may
  become a sender-key target and suppress the transcript. Not traced
  further.
- **G4.** Member labels (§8.1 step 17) were not exercised against a
  server; whether the server accepts an all-empty label change (the only
  way to clear a label from the CLI, by passing empty strings) is
  unknown.
- **G5.** Server-side 403 for a change forbidden by access control maps
  to the generic `[403] Authorization failed!` text; this is inferred
  from the client's generic mapping, not observed.
- **G6.** The lost-endorsements quirk (§5) and the state-erasing quirk
  (§6.2 step 5) are read from source; their user-visible effect (extra
  fetches; temporarily `Name: null`) was not observed black-box.
- **G7.** JSON-RPC variants of these commands share the command code;
  their error-code mapping is in `spec/surface/jsonrpc/` and was not
  re-checked here.
