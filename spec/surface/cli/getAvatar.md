# signal-cli getAvatar

## Sources

- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/commands/GetAvatarCommand.java (tag v0.14.7)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/util/CommandUtil.java
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/json/JsonAttachmentData.java
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/lib/src/main/java/org/asamk/signal/manager/internal/ManagerImpl.java (avatar retrieval)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/lib/src/main/java/org/asamk/signal/manager/storage/AvatarStore.java
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/man/signal-cli.1.adoc section "getAvatar"
- Binary observation (signal-cli 0.14.7, empty scratch `--config <CONFIG>`), commands run:
  - `signal-cli --config <CONFIG> getAvatar --help`
  - `signal-cli --config <CONFIG> getAvatar` (exit 1)
  - `signal-cli --config <CONFIG> getAvatar -c +15555550199 -p +15555550199` (exit 1)
  - `signal-cli --config <CONFIG> -a +15555550123 getAvatar -g abc` (exit 1, `User +15555550123 is not registered.`)

## Summary

Help line: `Retrieve the avatar of a contact, contact's profile or group base64 encoded.`

## Invocation category & account requirement

Local single-account command; requires a registered local account (common account errors,
see index.md). `--profile` may contact the server (profile refresh).

## Output modes

plain-text (default) and json.

## Arguments

| names | arity | value type / metavar | default | required | help text (verbatim) | notes |
|---|---|---|---|---|---|---|
| `-c`, `--contact` | 1 | string, CONTACT (recipient) | — | one of group required | `Get a contact avatar` | locally stored contact avatar (e.g. from contact sync) |
| `-p`, `--profile` | 1 | string, PROFILE (recipient) | — | one of group required | `Get a profile avatar` | profile avatar |
| `-g`, `--group-id` | 1 | string (base64), GROUP_ID | — | one of group required | `Get a group avatar` | |

Required mutual-exclusion group: exactly one of { `-c`, `-p`, `-g` }.

## Constraints & validation

- Recipient values: any recipient form; invalid phone number ->
  `Invalid phone number '<input>': <reason>` (exit 1); unresolvable user ->
  `The user <identifier> is not registered.` (exit 1).
- Group id must be valid base64 group id, else `Invalid group id: <reason>` (exit 1).

## Behavior

- `-c`: returns the locally stored contact avatar file of the recipient.
- `-p`: refreshes the recipient's profile if needed (may download the profile avatar), then
  returns the locally stored profile avatar file.
- `-g`: returns the locally stored group avatar file.
In all cases the file bytes are base64 encoded (standard alphabet, padded, one line).

## Plain-text output

One line: the base64 string.

## JSON output

One line: `{"data":"<base64>"}`.

## Errors

| condition | stream | exact message | exit |
|---|---|---|---|
| none of -c/-p/-g | stderr | `usage: signal-cli getAvatar [-h] (-c CONTACT \| -p PROFILE \| -g GROUP_ID)` + `signal-cli: error: one  of  the  arguments  -c/--contact -p/--profile -g/--` / `group-id is required` (observed) | 1 |
| more than one of -c/-p/-g | stderr | usage + e.g. `signal-cli: error: argument -p/--profile:  not  allowed with argument -c/--` / `contact` (observed for `-c X -p Y`) | 1 |
| no/unregistered local account | stderr | common account errors (index.md) | 1 |
| invalid recipient / group id | stderr | `Invalid phone number '<input>': <reason>` / `Invalid group id: <reason>` | 1 |
| recipient not registered | stderr | `The user <identifier> is not registered.` | 1 |
| no avatar stored | stderr | `Could not find avatar` | 1 |
| read / IO error | stderr | `An error occurred reading avatar` | 2 |

## --help transcript

Command: `signal-cli --config <CONFIG> getAvatar --help` — exit code 0, stderr empty, stdout:

```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli getAvatar [-h] (-c CONTACT | -p PROFILE | -g GROUP_ID)

named arguments:
  -h, --help             show this help message and exit
  -c CONTACT, --contact CONTACT
                         Get a contact avatar
  -p PROFILE, --profile PROFILE
                         Get a profile avatar
  -g GROUP_ID, --group-id GROUP_ID
                         Get a group avatar
```

## Doc vs binary discrepancies

- Man page lists `--contact`/`--profile` without the short `-c`/`-p` aliases; binary has them.
- Man page: "-g ... Alternatively, specify the group ID" — binary requires exactly one of the three.
