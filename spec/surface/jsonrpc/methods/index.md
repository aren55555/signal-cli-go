# signal-cli 0.14.7 JSON-RPC methods: index and parameter conventions

## Sources

- Upstream signal-cli at tag v0.14.7: https://github.com/AsamK/signal-cli/tree/v0.14.7
  - `src/main/java/org/asamk/signal/commands/` (command registry, every command, the JSON-RPC parameter namespace)
  - `src/main/java/org/asamk/signal/jsonrpc/` (request dispatch, account selection, error mapping)
  - `src/main/java/org/asamk/signal/util/Util.java`, `util/CommandUtil.java`
  - `lib/src/main/java/org/asamk/signal/manager/api/RecipientIdentifier.java`, `GroupId.java`, `GroupIdFormatException.java`
- Man page: https://github.com/AsamK/signal-cli/blob/v0.14.7/man/signal-cli-jsonrpc.5.adoc
- Bundled dependency behavior (argparse4j 0.9.0, MIT) observed by disassembling
  `lib/argparse4j-0.9.0.jar` with `javap -c` (getter semantics listed below).
- Binary observation: zero-account daemon, see `../observations.md`.

## Parameter conventions (apply to every method unless its file says otherwise)

Most methods are "namespace" methods: the JSON `params` object is turned into a
string-keyed map, and the command reads values out of it by the name of its CLI
long option. The rules below are therefore shared by every such method.

1. **params must be a JSON object** (or omitted, or `null`). `null` is treated
   as omitted. A JSON array for `params` on a namespace method fails with
   code -32600 (Invalid Request) and a Jackson "Cannot deserialize value of type
   `java.util.LinkedHashMap<java.lang.String,java.lang.Object>` from Array value"
   style message (exact text in `../observations.md`). Exceptions:
   `unsubscribeReceive`/`unsubscribeCallEvents` accept arrays; `subscribeReceive`, `subscribeCallEvents` and `startLink` ignore `params` entirely (any container accepted); and the typed-param methods `receive`, `register`, `verify`, `finishLink` bind `params` strictly to fixed camelCase keys (unknown keys -> -32600), see their files. Note `version` and `listAccounts` ARE namespace methods, so an array `params` fails for them (observed).
2. **Key lookup**: for a CLI option `--foo-bar`, the method first looks for JSON
   key `"foo-bar"`; if absent or `null`, it looks for the camelCase form
   `"fooBar"`. CamelCase conversion: split on `-`, keep the first part
   unchanged, then upper-case the first letter and lower-case the rest of each
   further non-empty part. Only the option's primary long name is consulted;
   short flags and secondary aliases of the CLI option (e.g. `--group` for
   `--group-id`, `-m`) are **not** accepted as JSON keys. Positional CLI
   arguments use their metavariable name (e.g. `recipient`).
3. **List parameters** (CLI options with `nargs("*")` or `"+"`): the value is
   looked up as in rule 2. If found and it is a JSON array, it is used. If found
   and it is **not** an array (e.g. a single string), it is wrapped into a
   one-element list. If not found, the plural key (`dest + "s"`, then its
   camelCase form) is looked up, e.g. `recipient` -> `recipients`, `group-id` ->
   `group-ids` / `groupIds`, `attachment` -> `attachments`. Under the plural key
   the value **must** be an array; a non-array there causes a
   ClassCastException, surfaced as -32603. Singular key wins when both exist.
4. **Strings** (`getString`): any JSON scalar or container is accepted and
   converted with Java `toString()` (number `5` -> `"5"`, `true` -> `"true"`,
   arrays become `"[a, b]"`, objects `"{k=v}"`). So string params never fail on
   type.
5. **Booleans** (`getBoolean`, flags with `storeTrue`): the value must be a JSON
   boolean. Any other JSON type causes ClassCastException -> -32603 (message
   is the Java ClassCastException text plus ` (ClassCastException)`). A missing
   flag is `false`.
6. **Integers / longs** (`getInt` / `getLong`): JSON numbers are materialised as
   Java Integer when they fit in 32 bits, Long when they fit in 64 bits,
   BigInteger beyond that, Double when fractional. A `getLong` parameter
   (timestamps, etc.) given a small value such as `5` (which becomes Integer)
   fails with ClassCastException -> -32603; a `getInt` parameter given a value
   above 2^31-1 fails the same way; strings fail the same way. Real
   millisecond timestamps (> 2^31) are Longs and work. Implementations aiming
   for exact parity must reproduce this; a more lenient implementation would
   differ only on these error paths. The method files note which parameters
   are int vs long.
7. **Enum-typed CLI options** (e.g. `--type`, `--trust-level` style choices): in
   JSON-RPC the raw value is read; see each method for how it is parsed (often
   as a string, case-insensitive or not).
8. **Unknown keys are ignored** silently.
9. **`account`**: in multi-account mode the `account` key selects the account
   and is **removed** from params before the command sees it (see
   `../transports.md` "Account selection"). In single-account mode it is not
   removed: namespace methods ignore it as an unknown key, but the strict typed-param method `receive` rejects it with -32600 (and `finishLink`, multi-account-only, rejects a leftover `account` in multi-account mode since multi-only methods never consume it).
10. **Recipients** (`recipient` strings and other "single recipient"
    parameters): accepted forms, in priority order: a UUID string (ACI);
    `PNI:<uuid>`; `u:<username>`; otherwise a phone number which is normalised
    to E.164 relative to the account's own number. Invalid numbers give code
    -1 with message `Invalid phone number '<input>': <reason>`; `PNI:` with a
    bad UUID gives reason `Invalid PNI`.
11. **Group ids**: standard base64 (with padding, not URL-safe) of a 16-byte
    (v1) or 32-byte (v2) id. Undecodable input gives code -1 with message
    `Invalid group id: Failed to decode groupId (must be base64) "<input>": <decoder message>`.
12. **Result**: a method's result is the single JSON value it produces. If a
    method produces nothing, the result is the empty object `{}`.
13. **Error data**: if a method produced a result value and then failed, the
    error object's `data` is `{"response": <that value>}`; otherwise `data` is
    `null`. (Used by send-type methods whose partial results are returned with
    an error.)

## Account requirement classes

- **single**: requires a concrete account. In single-account daemon mode the
  daemon's account is used. In multi-account mode: the `account` param if
  present (must name a loaded account, else -32602 `Specified account does not
  exist`), otherwise the only loaded account if exactly one is loaded, otherwise
  -32602 `Method requires valid account parameter`.
- **multi**: operates on the account set; only available in multi-account mode
  (in single-account mode it is -32601 `Method not implemented`).
- **single+multi**: if `account` is given (multi-account mode) it runs against
  that account, otherwise against all accounts / the global context.
- **registration**: multi-account mode only; `account` key is mandatory
  (-32602 `Method requires account parameter` when absent); if the account
  cannot be opened -32602 `Failed to load the given account`. In
  single-account mode -32601 `Method not implemented`.

## Methods

See the table below; one file per method in this directory.

Account classes: S = single, M = multi only, SM = single+multi, R = registration. "HTTP" = available over `POST /api/v1/rpc`.

| Method | Class | HTTP | CLI equivalent | Summary |
|---|---|---|---|---|
| [acceptCall](acceptCall.md) | S | yes | `acceptCall` | Accept an incoming voice call by `callId`; returns call-info object (state `CONNECTING`). |
| [addDevice](addDevice.md) | S | yes | `addDevice` | Link a new device via its `sgnl://linkdevice` `uri`; result `{}`. |
| [addStickerPack](addStickerPack.md) | S | yes | `addStickerPack` | Install sticker packs from signal.art URIs (`uri`); result `{}`. |
| [block](block.md) | S | yes | `block` | Block recipients and/or groups; result `{}`. |
| [deleteLocalAccountData](deleteLocalAccountData.md) | R | yes | `deleteLocalAccountData` | Delete an account's local data (`ignoreRegistered` to force); result `{}`. |
| [finishChangeNumber](finishChangeNumber.md) | S | yes | `finishChangeNumber` | Complete a phone-number change with verification code (and PIN); result `{}`. |
| [finishLink](finishLink.md) | M | yes | `link` (second half) | Wait for the primary to finish provisioning (`deviceLinkUri`, `deviceName`); returns `{number}`. |
| [getAttachment](getAttachment.md) | S | yes | `getAttachment` | Return a stored attachment by `id` as `{data: base64}`. |
| [getAvatar](getAvatar.md) | S | yes | `getAvatar` | Return a contact/profile/group avatar as `{data: base64}`. |
| [getSticker](getSticker.md) | S | yes | `getSticker` | Return a sticker image (`packId`, `stickerId`) as `{data: base64}`. |
| [getUserStatus](getUserStatus.md) | S | yes | `getUserStatus` | Registration status for numbers/usernames; array of `{recipient, number, username, uuid, isRegistered}`. |
| [hangupCall](hangupCall.md) | S | yes | `hangupCall` | Hang up an active call by `callId`; result `{}`. |
| [joinGroup](joinGroup.md) | S | yes | `joinGroup` | Join a group via invite link `uri`; returns `{timestamp, results, groupId[, onlyRequested]}`. |
| [listAccounts](listAccounts.md) | M | yes | `listAccounts` | List local accounts as `[{number}]`. |
| [listCalls](listCalls.md) | S | yes | `listCalls` | List active calls. |
| [listContacts](listContacts.md) | S | yes | `listContacts` | List contacts/recipients with profile data; filters `recipient`, `allRecipients`, `blocked`, `name`, `internal`. |
| [listDevices](listDevices.md) | S | yes | `listDevices` | List linked devices `[{id, name, createdTimestamp, lastSeenTimestamp}]`. |
| [listGroups](listGroups.md) | S | yes | `listGroups` | List groups with full details; optional `groupId` filter. |
| [listIdentities](listIdentities.md) | S | yes | `listIdentities` | List identity keys and safety numbers; optional `number` filter. |
| [listStickerPacks](listStickerPacks.md) | S | yes | `listStickerPacks` | List known sticker packs. |
| [quitGroup](quitGroup.md) | S | yes | `quitGroup` | Leave a group (optionally promote `admin`s, `delete` local data). |
| [receive](receive.md) | S | yes | `receive` | Poll for messages (`timeout`, `maxMessages`); only when nothing else is receiving. |
| [register](register.md) | R | yes | `register` | Request SMS/voice verification code (`voice`, `captcha`, `reregister`); result `{}`. |
| [rejectCall](rejectCall.md) | S | yes | `rejectCall` | Reject an incoming call by `callId`; result `{}`. |
| [remoteDelete](remoteDelete.md) | S | yes | `remoteDelete` | Delete one of your sent messages for everyone (`targetTimestamp`). |
| [removeContact](removeContact.md) | S | yes | `removeContact` | Remove contact details, or `hide` / `forget` a recipient; result `{}`. |
| [removeDevice](removeDevice.md) | S | yes | `removeDevice` | Unlink a device by `deviceId`; result `{}`. |
| [removePin](removePin.md) | S | yes | `removePin` | Remove the registration-lock PIN; result `{}`. |
| [send](send.md) | S | yes | `send` | Send/edit a message (text, attachments, mentions, styles, quote, sticker, preview, story reply) or end session; returns `{timestamp, results}`. |
| [sendAdminDelete](sendAdminDelete.md) | S | yes | `sendAdminDelete` | Group admin deletes another member's message/story. |
| [sendContacts](sendContacts.md) | S | yes | `sendContacts` | Send contacts sync to linked devices; result `{}`. |
| [sendMessageRequestResponse](sendMessageRequestResponse.md) | S | yes | `sendMessageRequestResponse` | Accept or delete a message request (`type`). |
| [sendPaymentNotification](sendPaymentNotification.md) | S | yes | `sendPaymentNotification` | Send a MobileCoin payment notification (`receipt`, `note`). |
| [sendPinMessage](sendPinMessage.md) | S | yes | `sendPinMessage` | Pin a message (`pinDuration` seconds, -1 forever). |
| [sendPollCreate](sendPollCreate.md) | S | yes | `sendPollCreate` | Create a poll (`question`, 2-10 `option`s, `noMulti`). |
| [sendPollTerminate](sendPollTerminate.md) | S | yes | `sendPollTerminate` | Close a poll you created. |
| [sendPollVote](sendPollVote.md) | S | yes | `sendPollVote` | Vote on a poll. |
| [sendReaction](sendReaction.md) | S | yes | `sendReaction` | Add/remove an emoji reaction to a message or story. |
| [sendReceipt](sendReceipt.md) | S | yes | `sendReceipt` | Send read (default) or viewed receipts for timestamps. |
| [sendStory](sendStory.md) | S | yes | `sendStory` | Post an image/video story to My Story or a group. |
| [sendSyncRequest](sendSyncRequest.md) | S | yes | `sendSyncRequest` | Request all sync data from the primary device; result `{}`. |
| [sendTyping](sendTyping.md) | S | yes | `sendTyping` | Send typing START/STOP indicator. |
| [sendUnpinMessage](sendUnpinMessage.md) | S | yes | `sendUnpinMessage` | Unpin a message. |
| [setPin](setPin.md) | S | yes | `setPin` | Set the registration-lock PIN; result `{}`. |
| [startCall](startCall.md) | S | yes | `startCall` | Start an outgoing voice call; returns call-info object. |
| [startChangeNumber](startChangeNumber.md) | S | yes | `startChangeNumber` | Begin changing the account's number (`number`, `voice`, `captcha`). |
| [startLink](startLink.md) | M | yes | `link` (first half) | Start provisioning a new linked account; returns `{deviceLinkUri}`. |
| [submitRateLimitChallenge](submitRateLimitChallenge.md) | S | yes | `submitRateLimitChallenge` | Submit a rate-limit captcha (`challenge`, `captcha`); result `{}`. |
| [subscribeCallEvents](subscribeCallEvents.md) | SM | **no** | none (JSON-RPC only) | Subscribe this connection to `callEvent` notifications; returns subscription id. |
| [subscribeReceive](subscribeReceive.md) | SM | **no** | none (JSON-RPC only) | Subscribe this connection to `receive` notifications; returns subscription id. |
| [trust](trust.md) | S | yes | `trust` | Trust identity keys (all known, or verified safety number). |
| [unblock](unblock.md) | S | yes | `unblock` | Unblock recipients and/or groups; result `{}`. |
| [unregister](unregister.md) | S | yes | `unregister` | Unregister the account (optionally `deleteAccount`); result `{}`. |
| [unsubscribeCallEvents](unsubscribeCallEvents.md) | SM | **no** | none (JSON-RPC only) | Cancel a call-event subscription. |
| [unsubscribeReceive](unsubscribeReceive.md) | SM | **no** | none (JSON-RPC only) | Cancel a receive subscription. |
| [updateAccount](updateAccount.md) | S | yes | `updateAccount` | Update device name, privacy settings, username; returns `{}` or `{username, usernameLink}`. |
| [updateConfiguration](updateConfiguration.md) | S | yes | `updateConfiguration` | Update read-receipt/typing/link-preview/unidentified-delivery settings; result `{}`. |
| [updateContact](updateContact.md) | S | yes | `updateContact` | Set a contact's names, note, expiration; result `{}`. |
| [updateDevice](updateDevice.md) | S | yes | `updateDevice` | Rename a linked device; result `{}`. |
| [updateGroup](updateGroup.md) | S | yes | `updateGroup` | Create or update a group; returns `{timestamp?, results?, groupId?}`. |
| [updateProfile](updateProfile.md) | S | yes | `updateProfile` | Update own profile (names, about, avatar, mobilecoin address); result `{}`. |
| [uploadStickerPack](uploadStickerPack.md) | S | yes | `uploadStickerPack` | Upload a sticker pack from `path`; returns `{url}`. |
| [verify](verify.md) | R | yes | `verify` | Finish registration with `verificationCode` (and `pin`); result `{}`. |
| [version](version.md) | SM | yes | `version` | Returns `{"version":"0.14.7"}`. |

Total: 64 methods (60 CLI-backed commands + 4 JSON-RPC-only subscription methods).
Not methods (return -32601): `daemon`, `jsonRpc`, `link`.
