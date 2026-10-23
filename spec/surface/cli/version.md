# signal-cli version

## Sources

- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/commands/VersionCommand.java (tag v0.14.7)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/BaseConfig.java (project name/version source)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/App.java (dispatch: with `-a` → single-account path)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/man/signal-cli.1.adoc section "version"
- Binary observation (signal-cli 0.14.7):
  - `signal-cli --config <CONFIG> version --help`
  - `signal-cli --config <CONFIG> version`
  - `signal-cli --config <CONFIG> -o plain-text version`, `... -o json version`
  - `signal-cli --config <CONFIG> version extra`, `... version --bogus`
  - `signal-cli --config <CONFIG> -a +15555550123 version`, `... -a +15555550123 -o json version`
  - `signal-cli --config <CONFIG> --version` (global flag, for comparison)

## Summary

Help line: NONE — this subcommand has no help description, so it appears in the `{...}` choice list of the top-level usage/help but is NOT listed with a description in the "subcommands:" section of `signal-cli --help`. Man page: `Show version information.`

Prints the program name and version.

## Invocation category & account requirement

Hybrid: without `-a` it runs in multi-account mode and needs no account at all (works with an empty data directory; any unregistered accounts present would produce the same `WARN  MultiAccountManager - Ignoring ...` stderr lines as `listAccounts`, inferred from the shared loading path). WITH `-a` it runs as a single-account command and therefore requires that account to be valid and registered (observed: `-a +15555550123 version` → `User +15555550123 is not registered.`, exit 1). No network traffic.

## Output modes

`plain-text` and `json`; default `plain-text`.

## Arguments

None besides `-h/--help`. Any extra argument → `signal-cli: error: unrecognized arguments: '<x>'`, with usage `usage: signal-cli version [-h]` (observed), exit 1.

## Constraints & validation

None.

## Behavior

Prints version, exit 0.

## Plain-text output

One line: `signal-cli 0.14.7` (program name, space, version). Identical to the output of the global `--version` flag (observed). If the build has no embedded name/version, the fallbacks are `signal-cli` and `unknown`.

## JSON output

One line: `{"version":"0.14.7"}` — a single key `version` (string). The program name is not included (observed).

## Errors

| Condition | Stream | Exact message | Exit |
|---|---|---|---|
| `-a` given and account invalid | stderr | `Invalid account (phone number), make sure you include the country code.` | 1 |
| `-a` given and account not registered / unknown | stderr | `User <ACCOUNT> is not registered.` | 1 |
| `-a` given, account state load failure | stderr | `Error while checking account <ACCOUNT>: <detail>` (3 if I/O else 2) / `Error loading state file for user <ACCOUNT>: <detail> (<ErrorKind>)` (2) | 2/3 |
| Argument parse error | stderr | `usage: signal-cli version [-h]` + `signal-cli: error: <reason>` | 1 |

## --help transcript

Command: `signal-cli --config <CONFIG> version --help` — exit code 0, stderr empty, stdout:

```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli version [-h]

named arguments:
  -h, --help             show this help message and exit
```

The first 7 lines (the short usage of only `-h`, `-v`, `--log-file`, `--scrub-log`) are printed by the early logging-options pre-parse and precede every `--help` output; the command-specific help follows immediately with no blank line in between.

## Doc vs binary discrepancies

- Man documents the subcommand ("Show version information."), but the binary's help has no description for it, so it is absent from the described subcommand list in `--help`.
- Global `--version` (handled by the argument parser, exits 0 immediately, never touches accounts) vs. `version` subcommand (honours `-a` and `-o`).
