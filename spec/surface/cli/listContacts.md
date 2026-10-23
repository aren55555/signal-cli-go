# signal-cli listContacts

## Sources
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/commands/ListContactsCommand.java
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/json/JsonContact.java
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/lib/src/main/java/org/asamk/signal/manager/api/Contact.java, Profile.java, Recipient.java, RecipientAddress.java
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/man/signal-cli.1.adoc, section `=== listContacts`
- Binary observations:
  - `signal-cli --config <CONFIG> listContacts --help`
  - `signal-cli --config <CONFIG> listContacts`
  - `signal-cli --config <CONFIG> listContacts --blocked`
  - `signal-cli --config <CONFIG> -a +15555550123 listContacts --blocked maybe`
  - `signal-cli --config <CONFIG> -a +15555550123 listContacts` and `-a 12345 listContacts` (both recorded by the parent agent)

## Summary
Help text: "Show a list of known contacts with names and profiles."

The command lists recipients from the local store. According to the man page, giving explicit recipients refreshes their profiles.

## Invocation category & account requirement
This is a single-account local command and needs a registered account.

## Output modes
`plain-text` (the default) and `json`. JSON output always includes the detailed fields.

## Arguments

| names | arity | value type / metavar | default | required | help text (verbatim) | notes |
|---|---|---|---|---|---|---|
| `-h`, `--help` | 0 | – | – | no | `show this help message and exit` | |
| `recipient` (positional) | 0..n | recipients | – | no | `Specify one ore more phone numbers to show.` | The typo "ore" is verbatim. A recipient may be a number, a UUID, `PNI:`, or `u:`. |
| `-a`, `--all-recipients` | 0 | flag | false | no | `Include all known recipients, not only contacts.` | |
| `--blocked` | 1 | boolean / `{true,false}` | not set (all) | no | `Specify if only blocked or unblocked contacts should be shown (default: all contacts)` | See the parsing quirk below. |
| `--name` | 1 | string / `NAME` | – | no | `Find contacts with the given contact or profile name.` | |
| `--detailed` | 0 | flag | false | no | `List the contacts with more details. If output=json, then this is always set` | |
| `--internal` | 0 | flag | false | no | `Include internal information that's normally not user visible` | Affects JSON output too. |

`--blocked` parsing quirk (observed): the help shows the choices `{true,false}`, but any value is accepted. The value `true`, in any letter case, means true. **Every other string, for example `maybe`, means false**, and no parse error is raised. `--blocked` with no value is a parse error: `argument --blocked: expected one argument`.

## Constraints & validation
An invalid recipient gives `Invalid phone number '<s>': <reason>` (exit 1).

## Behavior
- By default only contacts are listed. With `-a`, all known recipients are listed.
- `--blocked true` lists only blocked recipients. `--blocked false` lists only unblocked ones.
- Positional recipients restrict the list to those recipients.
- `--name` filters by contact name or profile name.

## Plain-text output
One line per recipient:

`Number: <number or ""> ACI: <aci or ""> Name: <contact name> Profile name: <profile display name> Username: <username or ""> Color: <color or ""> Blocked: <bool> Message expiration: <EXP>`

- The contact name is the given and family name joined with a single space, or just whichever one is set. It is an empty string if neither is set.
- The profile display name is built the same way from the profile.
- EXP is `disabled` when the value is 0, otherwise `<n>s`.

With `--detailed`, a second line follows, indented by 2 spaces. Note the irregular punctuation, which is verbatim: a comma after the family name, and no colon after `Nick family name`.

`  PNI: <pni or ""> Given name: <...> Family name: <...>, Nick name: <...> Nick given name: <...> Nick family name <...> Note: <...> Archived: <bool> Hidden: <bool> Profile sharing: <bool> About: <...> About Emoji: <...> Unregistered: <bool>`

With `--internal`, another line follows, also indented by 2 spaces:

`  Capabilities: [<cap>, ...] Unidentified access mode: <DISABLED|ENABLED|UNRESTRICTED or ""> Shares number: <true|false|""> Discoverable by number: <true|false|"">`

- The capabilities list uses Java list rendering, for example `[storage, storageServiceEncryptionV2Capability]` or `[]`.
- An unknown unidentified-access mode prints as empty.

Edge case found in the source: if `--detailed` or `--internal` is used for a recipient that has no contact or profile record, which is possible with `-a`, the command fails with an internal error (stack trace, exit 2).

## JSON output
A JSON array with one object per recipient. The keys appear in this order and are all always present unless noted:

| key | type |
|---|---|
| number | string or null |
| uuid | string or null (ACI) |
| username | string or null |
| name | string (contact name; `""` when unset) |
| givenName | string or null |
| familyName | string or null |
| nickName | string or null |
| nickGivenName | string or null |
| nickFamilyName | string or null |
| note | string or null |
| color | string or null |
| isArchived | boolean |
| isBlocked | boolean |
| isHidden | boolean |
| messageExpirationTime | integer |
| profileSharing | boolean |
| unregistered | boolean |
| profile | `{lastUpdateTimestamp: integer, givenName: string or null, familyName: string or null, about: string or null, aboutEmoji: string or null, hasAvatar: boolean, mobileCoinAddress: base64 string or null}`, or null when no profile is known |
| internal | `{capabilities: string[], unidentifiedAccessMode: string or null, sharesPhoneNumber: boolean or null, discoverableByPhonenumber: boolean or null}`. Present **only** with `--internal`; otherwise the key is omitted. |

With no recipients, the output is `[]`.

## Errors

| condition | stream | exact message | exit code |
|---|---|---|---|
| `--blocked` without a value | stderr | usage + `signal-cli: error: argument --blocked: expected one argument` | 1 |
| No local account | stderr | `No local users found, you first need to register or link an account` | 1 |
| Invalid `-a` | stderr | `Invalid account (phone number), make sure you include the country code.` | 1 |
| Not registered | stderr | `User +15555550123 is not registered.` | 1 |
| Invalid recipient | stderr | `Invalid phone number '<s>': <reason>` | 1 |

## --help transcript
Command: `signal-cli --config <CONFIG> listContacts --help`

exit code: 0

stdout:
```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli listContacts [-h] [-a] [--blocked {true,false}]
                  [--name NAME] [--detailed] [--internal]
                  [recipient [recipient ...]]

positional arguments:
  recipient              Specify one ore more phone numbers to show.

named arguments:
  -h, --help             show this help message and exit
  -a, --all-recipients   Include all known recipients, not only contacts.
  --blocked {true,false}
                         Specify if  only  blocked  or  unblocked  contacts
                         should be shown (default: all contacts)
  --name NAME            Find contacts with  the  given  contact or profile
                         name.
  --detailed             List  the   contacts   with   more   details.   If
                         output=json, then this is always set
  --internal             Include internal information  that's  normally not
                         user visible
```

stderr:
```
```


## Observed invocations
### `signal-cli --config <CONFIG> listContacts`

exit code: 1

stdout:
```
```

stderr:
```
No local users found, you first need to register or link an account
```

### `signal-cli --config <CONFIG> listContacts --blocked`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli listContacts [-h] [-a] [--blocked {true,false}]
                  [--name NAME] [--detailed] [--internal]
                  [recipient [recipient ...]]
signal-cli: error: argument --blocked: expected one argument
```

### `signal-cli --config <CONFIG> -a +15555550123 listContacts --blocked maybe`

exit code: 1

stdout:
```
```

stderr:
```
User +15555550123 is not registered.
```


## Doc vs binary discrepancies
- The man page describes `--blocked` without a value set. The binary takes one argument and shows the choices `{true,false}`, but it accepts any string, and any string other than "true" is treated as false.
- The man page says positional recipients cause a profile refresh. This could not be observed without an account.
