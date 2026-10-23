# Method: register

## Sources

- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/commands/RegisterCommand.java (tag v0.14.7)
- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/jsonrpc/SignalJsonRpcCommandHandler.java (registration-class dispatch, account handling, error mapping)
- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/util/CommandUtil.java (captcha / rate-limit message text)
- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/util/DateUtils.java (timestamp format inside messages)
- https://github.com/AsamK/signal-cli/blob/v0.14.7/lib/src/main/java/org/asamk/signal/manager/internal/MultiAccountManagerImpl.java and lib/.../SignalAccountFiles.java (what opening a registration account does)
- https://github.com/AsamK/signal-cli/blob/v0.14.7/lib/src/main/java/org/asamk/signal/manager/api/ (CaptchaRequiredException, RateLimitException, VerificationMethodNotAvailableException, NonNormalizedPhoneNumberException message texts)
- https://github.com/AsamK/signal-cli/blob/v0.14.7/man/signal-cli-jsonrpc.5.adoc
- https://github.com/AsamK/signal-cli/blob/v0.14.7/lib/src/main/java/org/asamk/signal/manager/internal/RegistrationManagerImpl.java
- Not observed on the binary (would contact Signal servers). Everything below is from source reading.

## Summary

Requests an SMS or voice verification code for a phone number, creating the local account
if necessary. Second step is `verify`.

## CLI mapping

`signal-cli -a ACCOUNT register [-v|--voice] [--captcha CAPTCHA] [--reregister]`.
The man page states register/verify are not JSON-RPC commands; in fact they are available in
**multi-account mode only** (the man page's multi-account paragraph mentions `register`).

## Account requirement

Class **registration** (see `index.md`):

- Single-account mode: -32601 `Method not implemented`.
- Multi-account mode: `params` must be an object that contains the key `account` (presence is
  enough; a JSON `null` value is taken as the literal text `null`). If `params` is omitted, `null`,
  an array, or lacks `account`: -32602 `Method requires account parameter`.
- The `account` value (text form of the JSON value) is the phone number to register. It is
  removed from params before the remaining params are read.
- Opening the account: if no local account exists for that number, a new local account is
  created on disk immediately (fresh identity keys, profile key, database), **before** params are
  validated and before any network traffic; if one exists it is loaded. If the account is currently
  open by a running manager, the daemon closes that manager and retries once. If it still cannot be
  opened (locked, I/O error, number mismatch): -32602 `Failed to load the given account`.
- The number is not syntax-checked at this point; malformed numbers are rejected later by the
  register step itself (see errors, `Failed to register: ...`).

## Params

Typed (strict) parameter object: unlike namespace methods, keys are bound by exact name only,
there are no dash-form aliases, and **unknown keys are rejected** (-32600 with a Jackson
"Unrecognized field" message that names the field and lists the 3 known properties).

| JSON key | type | required | default | CLI flag | notes |
|---|---|---|---|---|---|
| `voice` | boolean | no | false | `-v`, `--voice` | request a voice call instead of SMS. Jackson default scalar coercion applies (e.g. the strings `"true"`/`"false"` are accepted); uncoercible values -> -32600 |
| `captcha` | string | no | none | `--captcha` | captcha token (`signalcaptcha://...` value). Non-string scalars are coerced to their text; objects/arrays -> -32600 |
| `reregister` | boolean | no | false | `--reregister` | register even if the local account is already registered |

Keys such as `"re-register"` or `"voiceVerification"` are unknown keys -> -32600.

## Result

Empty object `{}` on success. Success covers three outcomes, indistinguishable to the client:

1. A verification session was created/reused and an SMS or voice code was requested; the
   local account is marked not-registered and the client must call `verify` next.
2. Without `reregister`: the local account is not registered but already has an ACI, and the
   server accepted its stored credentials again ("reactivation"); no code is sent and no `verify`
   is needed.
3. The account is a primary device holding a stored recovery password and the server accepted a
   re-registration with it; no `verify` needed.

In cases 2 and 3 the account is immediately added to the daemon's account set (visible to
`listAccounts`, other methods and receive subscriptions).

Without `reregister`, an account that is already registered is rejected (see errors) and no
code is requested.

Side effect worth reproducing: if the account named by `account` is currently loaded and running
in the daemon, opening it for registration finds it locked; the daemon then **closes that running
account** and retries. So calling `register` on an active account stops it (it disappears from
`listAccounts`) even if the register step then fails with `Account is already registered`.

## Errors

| code | message | trigger |
|---|---|---|
| -32601 | `Method not implemented` | single-account mode |
| -32602 | `Method requires account parameter` | no `account` key (multi-account mode) |
| -32602 | `Failed to load the given account` | local account cannot be opened |
| -32600 | Jackson deserialization message (e.g. `Unrecognized field "x" (class ...), not marked as ignorable (3 known properties: ...)` followed by `\n at [Source: UNKNOWN; byte offset: #UNKNOWN]` and a reference chain) | unknown key or wrong JSON type in params |
| -5 | `Rate limit reached`, optionally followed by a newline and `Next attempt may be tried at <ms> (<ISO-8601 UTC>)` | server rate limit. Timestamp format: decimal epoch milliseconds, space, then in parentheses `yyyy-MM-dd'T'HH:mm:ss.SSS` + `Z` in UTC, e.g. `1761200000000 (2025-10-23T06:13:20.000Z)` |
| -1 | Without a captcha given: three lines `Captcha required for verification, use --captcha CAPTCHA` / `To get the token, go to https://signalcaptchas.org/registration/generate.html` / `After solving the captcha, right-click on the "Open Signal" link and copy the link.` (joined by `\n`). With a captcha given: `Invalid captcha given.` Either form optionally followed by `\nNext Captcha may be provided at <timestamp as above>` | server requires a captcha |
| -1 | `Failed to register: <reason>` | number not in normalised E.164 form (reason from the number normaliser) |
| -1 | `Failed to register: Invalid verification method` and, when `voice` was true, the suffix `: Before requesting voice verification you need to request SMS verification and wait a minute.` | server refuses the chosen verification transport |
| -3 | `Failed to register: Account is already registered (IOException)` | account already registered and `reregister` not true |
| -3 | `Failed to register: Account is registered in another environment: <ENV> (IOException)` | local account registered against a different service environment (e.g. `LIVE` vs `STAGING`) |
| -3 | `Failed to register: <io message> (<exception simple name>)` | other network / I/O failure |
| -32603 | `<message> (<exception simple name>)` | any other unexpected failure |

`data` is always `null` (the method writes no partial result).
