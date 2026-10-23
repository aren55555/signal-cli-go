# Method `listContacts`

## Sources

- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/commands/ListContactsCommand.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/json/JsonContact.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/lib/src/main/java/org/asamk/signal/manager/api/Contact.java, Profile.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/lib/src/main/java/org/asamk/signal/manager/internal/ManagerImpl.java (recipient lookup)
- JSON schema asset `signal-cli-0.14.7-json-schemas.tar.gz` (no contact-list schema; `contact.schema.json` there is the shared-contact shape, not this one).
- Binary observation: none. Conventions: [index.md](index.md#parameter-conventions).

## Summary

Lists known contacts (or all known recipients) with names and cached profile
data. Profiles of the listed recipients may be refreshed from the server first.

CLI equivalent: `signal-cli -a ACCOUNT -o json listContacts [...]`.

Account requirement: **single**.

## Params

| JSON key(s) | Type | Req. | Default | CLI flag | Notes |
|---|---|---|---|---|---|
| `recipient` / `recipients` | list of string | no | none (no filter) | positional `recipient...` | Restrict to these recipients (index.md rule 10). If recipients are given but none resolve to a known/registered recipient, result is `[]`. |
| `all-recipients` / `allRecipients` | boolean | no | false | `-a/--all-recipients` | Include all known recipients, not only contacts. |
| `blocked` | boolean | no | null (both) | `--blocked` | `true` = only blocked, `false` = only unblocked, absent = all. Must be a JSON boolean. |
| `name` | string | no | none | `--name` | Filter by contact or profile name. |
| `detailed` | boolean | no | false | `--detailed` | Read, but has no effect on JSON output. |
| `internal` | boolean | no | false | `--internal` | Adds the `internal` object to each entry. |

Behavior when no recipients are given: profiles of all matching registered
recipients are fetched (failures only logged). When recipients are given,
their profiles are refreshed.

## Result

JSON array of contact objects. Keys (all present unless noted):

| Key | Type | Notes |
|---|---|---|
| `number` | string \| null | E.164 |
| `uuid` | string \| null | ACI |
| `username` | string \| null | |
| `name` | string | contact display name: given + " " + family, or whichever is non-empty, or `""` |
| `givenName` | string \| null | |
| `familyName` | string \| null | |
| `nickName` | string \| null | |
| `nickGivenName` | string \| null | |
| `nickFamilyName` | string \| null | |
| `note` | string \| null | |
| `color` | string \| null | |
| `isArchived` | boolean | |
| `isBlocked` | boolean | |
| `isHidden` | boolean | |
| `messageExpirationTime` | integer | seconds |
| `profileSharing` | boolean | |
| `unregistered` | boolean | true if an unregistered timestamp is recorded |
| `profile` | object \| null | null when no profile is cached |
| `internal` | object | **omitted** unless `internal` param is true |

`profile` object: `lastUpdateTimestamp` (int64), `givenName` (string|null),
`familyName` (string|null), `about` (string|null), `aboutEmoji`
(string|null), `hasAvatar` (boolean), `mobileCoinAddress` (base64 string|null).

`internal` object: `capabilities` (array of strings; known values `storage`,
`storageServiceEncryptionV2Capability`), `unidentifiedAccessMode` (string
`DISABLED`/`ENABLED`/`UNRESTRICTED`, or null when unknown),
`sharesPhoneNumber` (boolean|null), `discoverableByPhonenumber` (boolean|null).

Quirk: for a recipient that has no contact record (possible with
`allRecipients`), reading `profileSharing`/`unregistered` dereferences the
missing contact and the whole call fails with -32603 (NullPointerException).
Likewise `internal: true` with a recipient lacking a profile fails with -32603.
A compatible implementation may reproduce or document this.

## Errors

| Code | Message | Trigger |
|---|---|---|
| -1 | `Invalid phone number '<input>': <reason>` | bad recipient |
| -32603 | `<msg> (ClassCastException)` / `(NullPointerException)` | wrong JSON types; quirks above |
