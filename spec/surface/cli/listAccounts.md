# signal-cli listAccounts

## Sources

- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/commands/ListAccountsCommand.java (tag v0.14.7)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/App.java (multi-account dispatch, "Command only works in multi-account mode")
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/man/signal-cli.1.adoc section "listAccounts"
- Binary observation (signal-cli 0.14.7):
  - `signal-cli --config <CONFIG> listAccounts --help`
  - `signal-cli --config <CONFIG> listAccounts` (empty data dir)
  - `signal-cli --config <CONFIG> -o json listAccounts` (empty data dir)
  - same two commands with a data dir containing one unregistered account entry (+15555550123, left over by a failed `register`)
  - `signal-cli --config <CONFIG> listAccounts extra`
  - `signal-cli --config <CONFIG> -a +15555550123 listAccounts`

## Summary

Help line (verbatim): `Show a list of registered accounts.`

Lists the phone numbers of all registered accounts stored in the data directory.

## Invocation category & account requirement

Multi-account command: runs only WITHOUT `-a`. Needs no account; works with an empty or non-existent data directory (prints nothing / `[]`). Giving `-a` is an error (`Command only works in multi-account mode`). No network traffic for listing (loading accounts is local).

## Output modes

`plain-text` and `json`; default `plain-text`.

## Arguments

None besides `-h/--help`.

## Constraints & validation

- `-a` given → error (observed, exit 1).
- Any extra argument → `signal-cli: error: unrecognized arguments: '<x>'` (observed).

## Behavior

Loads all local accounts. Accounts that are not registered (or fail to load) are skipped; for each skipped account a warning is logged to stderr, observed form (default verbosity): `WARN  MultiAccountManager - Ignoring +15555550123: User is not registered. (NotRegisteredException)`. Exit 0 even if the list is empty. Ordering of multiple accounts: not determined (comes from the account manager library; not observable without registered accounts).

## Plain-text output

One line per registered account: `Number: <E164 number>`. Nothing at all if there are none (empty stdout).

## JSON output

A single line: a JSON array of objects, one per registered account, each `{"number":"<E164 string>"}`. Empty list → `[]` (observed). Note: the `number` value could in principle be null for an account without a number; not observed.

## Errors

| Condition | Stream | Exact message | Exit |
|---|---|---|---|
| `-a/--account` given | stderr | `Command only works in multi-account mode` | 1 |
| Accounts file could not be read | stderr | `Failed to load local accounts file` or `Failed to read local accounts list` | 3 |
| Argument parse error | stderr | usage `usage: signal-cli listAccounts [-h]` + `signal-cli: error: <reason>` | 1 |

Observed transcripts: empty data dir → plain-text stdout empty, stderr empty, exit 0; `-o json` → stdout `[]`, exit 0. With one unregistered account: stdout empty / `[]`, stderr the WARN line above, exit 0. `-a +15555550123 listAccounts` → `Command only works in multi-account mode`, exit 1.

## --help transcript

Command: `signal-cli --config <CONFIG> listAccounts --help` — exit code 0, stderr empty, stdout:

```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli listAccounts [-h]

named arguments:
  -h, --help             show this help message and exit
```

The first 7 lines (the short usage of only `-h`, `-v`, `--log-file`, `--scrub-log`) are printed by the early logging-options pre-parse and precede every `--help` output; the command-specific help follows immediately with no blank line in between.

## Doc vs binary discrepancies

None found.
