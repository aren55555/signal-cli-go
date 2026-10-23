# Method: finishChangeNumber

## Sources

- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/commands/FinishChangeNumberCommand.java (tag v0.14.7)
- https://github.com/AsamK/signal-cli/blob/v0.14.7/lib/src/main/java/org/asamk/signal/manager/helper/AccountHelper.java (retry on mismatched devices)
- `index.md` conventions
- Not observed on the binary. Source reading only.

## Summary

Completes a number change with the verification code (and registration-lock PIN if the new
number is locked). Distributes new PNI identity material to linked devices; retried up to 5
times when the server reports mismatched devices. Primary only.

## CLI mapping

`signal-cli -a ACCOUNT finishChangeNumber NUMBER -v|--verification-code CODE [-p|--pin PIN]`.

## Account requirement

Class **single**.

## Params

| JSON key(s) | type | required | CLI | notes |
|---|---|---|---|---|
| `number` | string | yes | positional `NUMBER` | must equal the number used in `startChangeNumber` (its verification session is looked up by number) |
| `verification-code` / `verificationCode` | string | yes | `-v`, `--verification-code` | stringified |
| `pin` | string | no | `-p`, `--pin` | registration lock PIN of the new number |

## Result

`{}`. The account's number (and PNI) change; subsequent `listAccounts` shows the new number and
the multi-account `account` param must use the new number (or the ACI).

## Errors

| code | message | trigger |
|---|---|---|
| -32602 | account selection errors | see `index.md` |
| -1 | `This command doesn't work on linked devices.` | linked device |
| -1 | `Verification failed! This number is locked with a pin. Hours remaining until reset: <N>` + `\n` + `Use '--pin PIN_CODE' to specify the registration lock PIN` | locked, PIN needed |
| -1 | `Verification failed! Invalid pin, tries remaining: <N>` | wrong PIN |
| -1 | `Account is pin locked, but pin data has been deleted on the server.` | no SVR data |
| -3 | `Failed to change number: <io message> (<exception simple name>)` | I/O failure, wrong code, no session |
| -32603 | `<message> (<simple name>)` | other |
