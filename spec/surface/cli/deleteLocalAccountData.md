# signal-cli deleteLocalAccountData

## Sources

- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/commands/DeleteLocalAccountDataCommand.java (tag v0.14.7)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/App.java
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/man/signal-cli.1.adoc section "deleteLocalAccountData"
- Binary observation (signal-cli 0.14.7):
  - `signal-cli --config <CONFIG> deleteLocalAccountData --help`
  - `signal-cli --config <CONFIG> deleteLocalAccountData` (no accounts)
  - `signal-cli --config <CONFIG> deleteLocalAccountData extra`
  - `signal-cli --config <CONFIG> -a 12345 deleteLocalAccountData`

## Summary

Help line (verbatim): `Delete all local data for this account. Data should only be deleted if the account is unregistered. CAUTION: This cannot be undone.`

Removes the account's local data (its entry in the accounts list and its data files) from the data directory. Refuses while the account is still registered, unless `--ignore-registered` is given.

## Invocation category & account requirement

Registration-class command (same account handling as `register`/`verify`): account must be a phone number with country code (ACI UUID rejected); if `-a` omitted, the single local account is used. The account need NOT be registered. Whether the account is "registered" is decided from local state (not verified against the server by this command as far as the CLI layer shows).

## Output modes

`plain-text` and `json`; default `plain-text`. Nothing printed on success.

## Arguments

| Names | Arity | Value type / metavar | Default | Required | Help text (verbatim) | Notes |
|---|---|---|---|---|---|---|
| `--ignore-registered` | 0 (flag) | — | false | no | `Delete the account data even though the account is still registered on the Signal servers.` | |

## Constraints & validation

If the account is registered and `--ignore-registered` is not set → error (below), nothing deleted.

## Behavior

Deletes local data for the account, exit 0, no output.

## Plain-text output

None.

## JSON output

None.

## Errors

| Condition | Stream | Exact message | Exit |
|---|---|---|---|
| `-o` output type not supported by the command (checked before any account handling; cannot occur for commands supporting both modes) | stderr | `Command doesn't support output type <plain-text\|json>` | 1 |
| No `-a` and zero local accounts | stderr | `No local users found, you first need to register or link an account` | 1 |
| No `-a` and more than one local account | stderr | `Multiple users found, you need to specify an account (phone number) with -a` | 1 |
| Account is not a valid phone number with country code (an ACI UUID is NOT accepted here) | stderr | `Invalid account (phone number), make sure you include the country code.` | 1 |
| Local state for the number could not be loaded or created | stderr | `Error loading or creating state file: <detail> (<ErrorKind>)` | 2 |
| Argument parse error | stderr | sub-command usage line(s) followed by `signal-cli: error: <reason>` | 1 |
| Account still registered, no `--ignore-registered` | stderr | `Not deleting account, it is still registered. Use --ignore-registered to delete it anyway.` | 1 |
| I/O failure while deleting | stderr | `Deletion error: <detail>` | 3 |

Observed: no accounts → `No local users found, you first need to register or link an account`, exit 1. `-a 12345` → `Invalid account (phone number), make sure you include the country code.`, exit 1. `deleteLocalAccountData extra` → usage line + `signal-cli: error: unrecognized arguments: 'extra'`, exit 1.

## --help transcript

Command: `signal-cli --config <CONFIG> deleteLocalAccountData --help` — exit code 0, stderr empty, stdout:

```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli deleteLocalAccountData [-h] [--ignore-registered]

named arguments:
  -h, --help             show this help message and exit
  --ignore-registered    Delete the account  data  even  though the account
                         is still registered on the Signal servers.
```

The first 7 lines (the short usage of only `-h`, `-v`, `--log-file`, `--scrub-log`) are printed by the early logging-options pre-parse and precede every `--help` output; the command-specific help follows immediately with no blank line in between.

## Doc vs binary discrepancies

None found.
