# Method: updateProfile

## Sources

- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/commands/UpdateProfileCommand.java (tag v0.14.7)
- https://github.com/AsamK/signal-cli/blob/v0.14.7/lib/src/main/java/org/asamk/signal/manager/api/UpdateProfile.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/lib/src/main/java/org/asamk/signal/manager/helper/ProfileHelper.java, lib/.../manager/util/Utils.java (avatar source handling)
- `index.md` conventions
- Not observed on the binary. Source reading only.

## Summary

Updates the account's own profile (name, about, emoji, MobileCoin address, avatar) and uploads it.

## CLI mapping

`signal-cli -a ACCOUNT updateProfile [--given-name|--name NAME] [--family-name NAME] [--about TEXT]
[--about-emoji EMOJI] [--mobile-coin-address|--mobilecoin-address B64] [--avatar PATH | --remove-avatar]`.

## Account requirement

Class **single**.

## Params

Namespace-style; all optional; absent/null = keep the previous value. Only the primary long
option names map to JSON (the CLI aliases `--name` and `--mobilecoin-address` are **not** JSON
keys).

| JSON key(s) | type | CLI flag | notes |
|---|---|---|---|
| `given-name` / `givenName` | string | `--given-name`, `--name` | |
| `family-name` / `familyName` | string | `--family-name` | |
| `about` | string | `--about` | |
| `about-emoji` / `aboutEmoji` | string | `--about-emoji` | |
| `mobile-coin-address` / `mobileCoinAddress` | string (standard base64) | `--mobile-coin-address` | invalid base64 -> -32603 `<decoder message> (IllegalArgumentException)` |
| `avatar` | string | `--avatar` | either an RFC 2397 data URI (`data:<mime>[;filename=<name>];base64,<data>`, same syntax as `send` attachments) or a path of an image file readable by the daemon process; anything that is not a valid data URI is treated as a path |
| `remove-avatar` / `removeAvatar` | boolean | `--remove-avatar` | if true the avatar is deleted and `avatar` is ignored (CLI mutual exclusion otherwise not enforced) |

## Result

`{}`. After the upload a sync "fetch profile" message is sent to linked devices. The avatar is
also stored in the local avatar store (or deleted from it when `removeAvatar`).

## Errors

| code | message | trigger |
|---|---|---|
| -32602 | account selection errors | see `index.md` |
| -3 | `Update profile error: <io message>` | upload failure, unreadable avatar file |
| -32603 | `... (IllegalArgumentException)` / `... (ClassCastException)` | bad base64 / bad boolean type |
