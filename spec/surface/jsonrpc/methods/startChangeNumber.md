# Method: startChangeNumber

## Sources

- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/commands/StartChangeNumberCommand.java (tag v0.14.7)
- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/util/CommandUtil.java, util/DateUtils.java (captcha / rate limit messages)
- https://github.com/AsamK/signal-cli/blob/v0.14.7/lib/src/main/java/org/asamk/signal/manager/internal/ManagerImpl.java, lib/.../manager/helper/AccountHelper.java
- `index.md` conventions
- Not observed on the binary. Source reading only.

## Summary

Starts moving the account to a new phone number: opens (or reuses) a verification session for the
new number and requests an SMS/voice code. Completed by `finishChangeNumber`. Primary only.

## CLI mapping

`signal-cli -a ACCOUNT startChangeNumber NUMBER [-v|--voice] [--captcha CAPTCHA]`.

## Account requirement

Class **single**.

## Params

| JSON key(s) | type | required | default | CLI | notes |
|---|---|---|---|---|---|
| `number` | string | yes | none | positional `NUMBER` | new number in E.164; not normalised by the daemon (non-normalised input is rejected) |
| `voice` | boolean | no | false | `-v`, `--voice` | non-boolean -> -32603 ClassCastException |
| `captcha` | string | no | none | `--captcha` | captcha token |

## Result

`{}`.

## Errors

| code | message | trigger |
|---|---|---|
| -32602 | account selection errors | see `index.md` |
| -1 | `This command doesn't work on linked devices.` | linked device |
| -5 | `Rate limit reached` [+ `\nNext attempt may be tried at <ms> (<yyyy-MM-ddTHH:mm:ss.SSSZ>)`] | rate limited |
| -1 | captcha-required text identical to `register` (three lines without captcha / `Invalid captcha given.` with one, optional `\nNext Captcha may be provided at ...`) | captcha required |
| -1 | `Failed to change number: <reason>` | number not normalised E.164 |
| -1 | `Failed to register: Invalid verification method` [+ `: Before requesting voice verification you need to request SMS verification and wait a minute.` when `voice`] | verification transport unavailable (note: the message says "register", verbatim) |
| -3 | `Failed to change number: <io message> (<exception simple name>)` | I/O failure |
| -32603 | `<message> (<simple name>)` | other (e.g. `number` missing) |
