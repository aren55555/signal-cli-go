# Method: verify

## Sources

- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/commands/VerifyCommand.java (tag v0.14.7)
- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/jsonrpc/SignalJsonRpcCommandHandler.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/lib/src/main/java/org/asamk/signal/manager/RegistrationManager.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/lib/src/main/java/org/asamk/signal/manager/api/ (PinLockedException, IncorrectPinException, PinLockMissingException)
- https://github.com/AsamK/signal-cli/blob/v0.14.7/lib/src/main/java/org/asamk/signal/manager/internal/RegistrationManagerImpl.java
- Not observed on the binary (would contact Signal servers). Source reading only.

## Summary

Completes registration with the verification code received by SMS/voice, optionally providing the
registration-lock PIN. On success the account becomes a fully registered account in the daemon.

## CLI mapping

`signal-cli -a ACCOUNT verify VERIFICATION_CODE [-p|--pin PIN]`.

## Account requirement

Class **registration**, identical to `register.md` (multi-account mode only; mandatory `account`
key, removed before parameter binding; account opened or created locally, and a running manager for
the same account is closed first if it holds the lock; same -32601/-32602
errors).

## Params

Typed (strict) object; exact key names only; unknown keys -> -32600.

| JSON key | type | required | default | CLI | notes |
|---|---|---|---|---|---|
| `verificationCode` | string | effectively yes | none | positional `VERIFICATION_CODE` | the received code. Note: the CLI positional name `verification-code` is **not** accepted (unknown key -> -32600). Numbers are coerced to text (e.g. `123456` -> `"123456"`); missing -> the verify step fails (error from the server/IO layer, typically -3 or -32603) |
| `pin` | string | no | none | `-p`, `--pin` | registration lock PIN |

## Result

`{}` on success. The account is then registered and is added to the daemon's account set (it
appears in `listAccounts`, and existing multi-account receive subscriptions are extended to it).

## Errors

| code | message | trigger |
|---|---|---|
| -32601 | `Method not implemented` | single-account mode |
| -32602 | `Method requires account parameter` / `Failed to load the given account` | as for `register` |
| -32600 | Jackson deserialization message | unknown key / uncoercible type |
| -1 | `Verification failed! This number is locked with a pin. Hours remaining until reset: <N>` + `\n` + `Use '--pin PIN_CODE' to specify the registration lock PIN` | account has registration lock and no PIN (or PIN needed); N = remaining time in whole hours (integer division) |
| -1 | `Verification failed! Invalid pin, tries remaining: <N>` | wrong PIN |
| -1 | `Account is pin locked, but pin data has been deleted on the server.` | locked but no SVR data |
| -3 | `Verify error: Account is already registered` | the local account is already registered |
| -3 | `Verify error: No registration verification session active` | no prior successful `register` that created a session for this number |
| -3 | `Verify error: <io message>` | other I/O failure, including a wrong verification code reported by the server |
| -32603 | `<message> (<exception simple name>)` | anything else |
