# signal-cli link

## Sources

- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/commands/LinkCommand.java (tag v0.14.7)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/App.java (provisioning-command dispatch, `-a` rejection, output type check)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/man/signal-cli.1.adoc sections "link", "Options" (`-a`)
- Binary observation (signal-cli 0.14.7):
  - `signal-cli --config <CONFIG> link --help`
  - `signal-cli --config <CONFIG> link extra`
  - `signal-cli --config <CONFIG> link -n`
  - `signal-cli --config <CONFIG> -a +15555550123 link -n foo`
  - `signal-cli --config <CONFIG> -a +15555550123 link`
  - `signal-cli --config <CONFIG> -o json link`
  - (a successful `link` was NOT run: it contacts the Signal servers)

## Summary

Help line (verbatim): `Link to an existing device, instead of registering a new number.`

Creates a new secondary-device identity and prints a device-link URI (`sgnl://linkdevice?uuid=...&pub_key=...`). The user scans it (as a QR code) with the primary Signal app, or passes it to another signal-cli's `addDevice --uri`. The command then waits until the primary device completes the linking, stores the new account locally, and prints the associated phone number.

## Invocation category & account requirement

Provisioning command. Needs no existing account. `-a/--account` MUST NOT be given (error). Works with zero, one or many local accounts. Contacts the Signal server (provisioning websocket).

## Output modes

`plain-text` only (also the default). `-o json` → `Command doesn't support output type json`, exit 1 (observed).

## Arguments

| Names | Arity | Value type / metavar | Default | Required | Help text (verbatim) | Notes |
|---|---|---|---|---|---|---|
| `-n`, `--name` | 1 | string, `NAME` | `cli` (applied when option absent; not shown in help) | no | `Specify a name to describe this new device.` | Device name shown in the primary's linked-device list. |

## Constraints & validation

- `-a` present → error before any network activity (observed).
- `-n` without value → `signal-cli: error: argument -n/--name: expected one argument` (observed).

## Behavior

1. Obtain the device-link URI from the server.
2. If the JVM reports an attached console (interactive terminal), print a QR code of the URI first (see below).
3. Print the URI on its own line, flushed immediately (so callers can read it while the command keeps waiting).
4. Block until the primary device finishes linking (or the request times out).
5. Print `Associated with: <NUMBER>` (the account's phone number) and exit 0.

## Plain-text output

All on stdout:
- Optional QR block (only when a console is attached): a line consisting of the ANSI sequence ESC`[37;40m` (white foreground on black background), then one line per pair of QR module rows, then a line consisting of ESC`[39;49m` (reset colors). The QR code encodes the URI, uses the smallest possible module size (one character column per module) and the encoder's default quiet zone (believed 4 modules). Each character represents two vertically stacked modules (upper/lower; the lower is treated as light past the last row): both dark → space ` `; only upper dark → `▄` (U+2584); only lower dark → `▀` (U+2580); both light → `█` (U+2588). (Dark modules render as the black background, light ones as the white full/half block.)
- Line: the device link URI, e.g. `sgnl://linkdevice?uuid=<id>&pub_key=<urlencoded base64 key>`.
- After successful linking, line: `Associated with: <E164 number>`.

## JSON output

Not supported.

## Errors

| Condition | Stream | Exact message | Exit |
|---|---|---|---|
| `-a/--account` given | stderr | `You cannot specify a account (phone number) when linking` | 1 |
| `-o json` | stderr | `Command doesn't support output type json` | 1 |
| Linking not completed in time | stderr | `Link request timed out, please try again.` | 1 |
| Network / I/O error | stderr | `Link request error: <detail>` | 3 |
| The linked account already exists locally | stderr | two lines: `The user <NUMBER> already exists` / `Delete "<PATH-OF-ACCOUNT-DATA>" before trying again.` | 1 |
| Argument parse error | stderr | usage `usage: signal-cli link [-h] [-n NAME]` + `signal-cli: error: <reason>` | 1 |

Observed: `-a +15555550123 link` and `-a +15555550123 link -n foo` → `You cannot specify a account (phone number) when linking`, exit 1.

## --help transcript

Command: `signal-cli --config <CONFIG> link --help` — exit code 0, stderr empty, stdout:

```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli link [-h] [-n NAME]

named arguments:
  -h, --help             show this help message and exit
  -n NAME, --name NAME   Specify a name to describe this new device.
```

The first 7 lines (the short usage of only `-h`, `-v`, `--log-file`, `--scrub-log`) are printed by the early logging-options pre-parse and precede every `--help` output; the command-specific help follows immediately with no blank line in between.

## Doc vs binary discrepancies

- Man page states the default name `cli`; help text does not show it. Binary uses `cli`.
- Whether the QR block appears when stdout is redirected depends on the JVM's console detection (Java 22+ may report a console even when not attached to a terminal); not verified (would require contacting the server). Implementations should print the QR only when stdout is a terminal; flagged as undetermined.
