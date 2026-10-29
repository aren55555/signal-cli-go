# signal-cli 0.14.7 command-line surface: index and global behavior

## Sources

- Upstream sources at tag v0.14.7 (read by the spec role only):
  - https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/Main.java
  - https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/App.java
  - https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/GlobalConfig.java
  - https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/ConfigLoader.java
  - https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/OutputType.java
  - https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/ServiceEnvironmentCli.java
  - https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/TrustNewIdentityCli.java
  - https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/DbusConfig.java
  - https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/BaseConfig.java
  - https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/Shutdown.java
  - https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/commands/ (Commands.java, Command.java, CliCommand.java, CommandHandler.java, LocalCommand.java, MultiLocalCommand.java, RegistrationCommand.java, ProvisioningCommand.java, JsonRpc*Command.java, every *Command.java for the help strings and category)
  - https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/commands/exceptions/ (all seven files)
  - https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/output/ (JsonWriter*.java, PlainTextWriter*.java, OutputWriter.java)
  - https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/util/CommandUtil.java, util/IOUtils.java
  - https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/logging/LogConfigurator.java, logging/Scrubber.java
  - https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/dbus/DbusCommandHandler.java
  - https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/man/signal-cli.1.adoc (sections Synopsis, Description, Options, Exit codes, Files, Configuration file)
  - File list: https://api.github.com/repos/AsamK/signal-cli/git/trees/v0.14.7?recursive=1
- Bundled argument-parser library observed in the release: `lib/argparse4j-0.9.0.jar`
  (help/error layout below is its behavior, observed black-box).
- Binary observation: `/work/third_party/signal-cli-0.14.7/bin/signal-cli` (release
  0.14.7, Java 25), always run as
  `JAVA_OPTS="$JAVA_OPTS -Djava.io.tmpdir=<jvmtmp>" signal-cli --config <CONFIG> ...`
  with stdin from `/dev/null`, stdout and stderr captured to files (so no TTY).
  Every invocation and its full output is in [observations.md](observations.md).
  Representative commands: `--help`, `-h`, `--version`, `version`,
  `-o json version`, `bogus`, `--bogus`, `-o xml version`, `--dbus --dbus-system version`,
  `--d version`, `-- version`, `-a`, `listAccounts`, `-o json listAccounts`,
  `send -m hi +15555550123`, `-a +15555550123 listContacts`, `-a 12345 listContacts`,
  `-a +15555550123 link`, `-a +15555550123 listAccounts`, `-o json link`,
  `-o plain-text jsonRpc`, `--dbus version`, `-d /nonexistent/x version`, and
  `SIGNAL_CLI_CONFIG=<file> ... version` with various config files.

Where the man page and the binary disagree, both are recorded and **the binary is
authoritative**.

## 1. Invocation shape

```
signal-cli [GLOBAL OPTIONS] SUBCOMMAND [SUBCOMMAND OPTIONS AND POSITIONALS]
```

- Global options must come **before** the subcommand; subcommand options after it.
  A global option placed after the subcommand is parsed by the subcommand parser and
  either means something else there (e.g. `-a` is `--attachment` for `send`) or is
  rejected as `unrecognized arguments`.
- Exactly one subcommand is required. With none, a parse error `too few arguments`
  results (exit 1).
- Long options may be abbreviated to any unambiguous prefix (`--vers` works as
  `--version`, `--verb` as `--verbose`). An ambiguous prefix is an error, e.g.
  `--d` -> `ambiguous option: --d could match --data-dir, --dbus, --dbus-system, --disable-send-log`.
  This applies inside subcommand parsers as well.
- Values may be attached: `--output=json`, `-ojson`, `-oplain-text` all work.
- Repeating a single-valued option: the last occurrence wins
  (`-a +15555550123 --account +15555550124` uses `+15555550124`).
- `--` ends option parsing; everything after it is treated as positionals of the
  current parser. At top level this means `signal-cli -- version` fails with
  `unrecognized arguments: 'version'` (the subcommand cannot be supplied after `--`).
- An empty-string argument where a subcommand is expected is an invalid choice
  (see 5.2).

## 2. Global options

There are **14** global options (counting `-h/--help` and `--version`). Defaults shown
are the built-in ones; every default except `-a` can be overridden by a configuration
file (section 7) and the help text then shows the configured default.

| # | Names (all aliases) | Arg | Default | Meaning |
|---|---|---|---|---|
| 1 | `-h`, `--help` | none | - | Print help (see 4) to stdout, exit 0. |
| 2 | `--version` | none | - | Print `signal-cli 0.14.7` + newline to stdout, exit 0. Help text: `Show package version.` |
| 3 | `-v`, `--verbose` | none, countable (`-vvv` = 3) | `0` | Raise log level. Help: `Raise log level and include lib signal logs. Specify multiple times for even more logs.` See 8. |
| 4 | `--log-file` | `LOG_FILE` (path) | none | Help: `Write log output to the given file. If --verbose is also given, the detailed logs will only be written to the log file.` |
| 5 | `--scrub-log` | none | `false` | Help: `Scrub possibly sensitive information from the log, like phone numbers and UUIDs.` |
| 6 | `-d`, `--data-dir`, `-c`, `--config` | `DATA_DIR` (path) | `$XDG_DATA_HOME/signal-cli`, or `$HOME/.local/share/signal-cli` if `XDG_DATA_HOME` is unset | Data directory. Account files live under `<DATA_DIR>/data/` (e.g. `<DATA_DIR>/data/accounts.json`). Help: `Set the path where to store data (Default: $XDG_DATA_HOME/signal-cli , $HOME/.local/share/signal-cli).` |
| 7 | `-a`, `--account`, `-u`, `--username` | `ACCOUNT` (string) | none | The local account: an E.164 number (`+` and country code) or, for local commands, an ACI UUID. Help: `Specify your phone number, that will be your identifier.` |
| 8 | `--dbus` | none | `false` | Run the command as a client of a running signal-cli D-Bus daemon on the session bus. Help: `Make request via user dbus.` Mutually exclusive with `--dbus-system`. |
| 9 | `--dbus-system` | none | `false` | Same on the system bus. Help: `Make request via system dbus.` |
| 10 | `--bus-name` | `GLOBAL-BUS-NAME` | `org.asamk.Signal` (or `org.asamk.SignalCli` when the file `/.flatpak-info` exists) | D-Bus name to connect to. Help: `Specify the D-Bus bus name to connect to.` |
| 11 | `-o`, `--output` | one of `plain-text`, `json` | unset: each command's first supported mode (see 3) | Help: `Choose to output in plain text or JSON` |
| 12 | `--service-environment` | one of `live`, `staging` | `live` | Help: `Choose the server environment to use.` |
| 13 | `--trust-new-identities` | one of `always`, `on-first-use`, `never` | `on-first-use` | Help: `Choose when to trust new identities.` `on-first-use`: trust the first seen key, changed keys need manual verification; `always`: trust any new key; `never`: every unknown key must be verified manually. |
| 14 | `--disable-send-log` | none | `false` | Help: `Disable message send log (for resending messages that recipient couldn't decrypt)` |

Choice values on the command line are matched exactly (case-sensitive), e.g.
`-o JSON` is rejected. (Config files are more lenient, see 7.)

Order of the options in the top-level usage line (verbatim, see 4) is:
`-h`, `--version`, `-v`, `--log-file`, `--scrub-log`, `-d`, `-a`, `--bus-name`, `-o`,
`--service-environment`, `--trust-new-identities`, `--disable-send-log`,
`--dbus | --dbus-system`. The order in the "named arguments" help list is:
`-h`, `--version`, `-v`, `--log-file`, `--scrub-log`, `-d...`, `-a...`, `--dbus`,
`--dbus-system`, `--bus-name`, `-o`, `--service-environment`,
`--trust-new-identities`, `--disable-send-log`.

### Account selection rules (`-a`)

Applied after parsing, in this order (messages to stderr, exit codes per section 6):

1. `link` (provisioning): if `-a` is given -> `You cannot specify a account (phone number) when linking` (exit 1).
2. No `-a` and the command can run in multi-account mode (`listAccounts`, `daemon`,
   `jsonRpc`, `version`): run over all local accounts.
3. No `-a` otherwise: read the local account list. Zero accounts ->
   `No local users found, you first need to register or link an account` (exit 1);
   more than one -> `Multiple users found, you need to specify an account (phone number) with -a` (exit 1);
   exactly one -> it is used. (The account list is not filtered by
   `--service-environment`; observed with two LIVE accounts and `--service-environment staging`.)
4. Registration commands (`register`, `verify`, `deleteLocalAccountData`): the account
   must be a valid phone number, else
   `Invalid account (phone number), make sure you include the country code.` (exit 1).
5. Local commands (all others): the account must be a valid phone number **or** a
   valid ACI UUID, else the same `Invalid account ...` message (exit 1). Then the
   account is loaded; if it does not exist locally or is not registered:
   `User <ACCOUNT> is not registered.` (exit 1), where `<ACCOUNT>` is the string as
   given (number or UUID).
6. A command that only works in multi-account mode (only `listAccounts`) given `-a` ->
   `Command only works in multi-account mode` (exit 1).

Note: `version` given `-a` takes the local-command path, so with an unregistered
account `-a +15555550123 version` fails with `User +15555550123 is not registered.`

Account loading failures other than "not registered": an I/O failure while checking
the account -> `Error while checking account <ACCOUNT>: <detail>` (exit 3); other
check failures -> same prefix (exit 2); any other load failure ->
`Error loading state file for user <ACCOUNT>: <detail> (<ErrorKind>)` (exit 2). For
registration commands a load failure ->
`Error loading or creating state file: <detail> (<ErrorKind>)` (exit 2). Failure to
read the account list -> `Failed to read local accounts list` (exit 3; observed with
`-d /nonexistent/x version`, because the data directory cannot be created) or
`Failed to load local accounts file` (exit 3).

Before any of the account logic, if the native crypto library is unavailable the
binary fails with `Missing required native library dependency: libsignal-client`
(exit 1). (A Go reimplementation has no such dependency; listed for completeness.)

### D-Bus client mode (`--dbus` / `--dbus-system`)

D-Bus is out of scope for the reimplementation, but the flags must parse. Behavior
of the reference: the command is executed against a running D-Bus daemon instead of
local data. Without a reachable bus the command fails with
`Dbus client failed: <detail>` (exit 2); observed details:
`Cannot Resolve Session Bus Address: DISPLAY variable not set` (`--dbus`) and
`Cannot Resolve Session Bus Address: MachineId file can not be found` (`--dbus-system`).
If the bus is reachable but no daemon owns the name:
`signal-cli DBus daemon not running on session bus: <detail>` (or `system bus`), exit 1.
Commands not supported over D-Bus: `Command is not yet implemented via dbus` (exit 1).
The `link`/`-a` and single-account rules of the previous section also apply.

## 3. Output modes

- Two modes: `plain-text` and `json`, selected with `-o/--output` (or config key
  `output`). When unset, the command's first supported mode is used: **plain-text**
  for every command except `jsonRpc` (json).
- Requesting an unsupported mode is checked right after parsing, **before** any account
  handling: `Command doesn't support output type <mode>` on stderr, exit 1, where
  `<mode>` is `plain-text` or `json`.
- Support matrix:
  - plain-text and json: every subcommand except the two below.
  - plain-text only: `link`.
  - json only: `jsonRpc`.
- All command output goes to **stdout**; diagnostics, errors and logs go to **stderr**.
  Plain-text output is written line by line with the platform line separator (`\n` on
  Linux); nested blocks are indented by 2 spaces per level.
- JSON mode writes each JSON value as compact JSON on a single line followed by a
  newline (no pretty printing). Commands that produce one result print one line; streaming
  commands (`receive`, `daemon`, `jsonRpc`) print one line per object. Commands that
  produce no result print nothing in json mode. Example: `-o json version` prints
  `{"version":"0.14.7"}`; `-o json listAccounts` with no accounts prints `[]`.
- JSON objects emit `null` for absent values unless the specific field is documented
  as omitted-when-null in the per-command file. Key order follows the per-command
  specs, but at least the send-result object (`timestamp`/`results`) was observed with
  **unstable key order between runs** in the reference; golden tests must compare JSON
  semantically, not byte-wise.
- Commands that print nothing in plain-text on success also print nothing (not `{}`)
  in json mode.
- Account resolution (section 2) happens before any command-specific validation, so
  messages such as `No recipients given` are only reachable with a registered account.
- Plain-text timestamps are rendered as `<epoch-ms> (<yyyy-MM-ddTHH:mm:ss.SSSZ in UTC>)`,
  e.g. `1700000000000 (2023-11-14T22:13:20.000Z)`.
- Output text is encoded in the console charset if a console is attached, otherwise
  the JVM default charset (UTF-8 in practice).
- Exact plain-text and JSON shapes per command are in the per-command files; the
  received-message rendering shared by `receive`/`daemon`/`jsonRpc` is in
  [_received-message-output.md](_received-message-output.md); the send-result
  rendering shared by the send family is in [_send-results.md](_send-results.md).

## 4. Help output

- `-h`/`--help` prints to **stdout** and exits **0**, with empty stderr.
- Quirk: every help invocation is preceded by the help of a small early "logging"
  parser, so the output always starts with this exact 7-line block (then directly,
  without a blank line, the real help):

```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
```

- `signal-cli -h` then prints the top-level help (verbatim in observations.md, and
  reproduced at the end of this file). `signal-cli CMD -h` prints the subcommand help
  (`usage: signal-cli CMD ...`, an optional `positional arguments:` section, then a
  `named arguments:` section). Per-command verbatim help is in each command file.
- Help is triggered as soon as `-h` is reached left to right: arguments after it are not
  validated (`version -h --bogus` and `-h -o xml` both print help, exit 0), but errors
  in arguments before it win (`-o xml -h` fails with exit 1). `-h bogus` prints the
  top-level help.
- `--version` likewise short-circuits: `--version bogus` prints the version, exit 0.
- Quirk: the `version` subcommand has no help string, so it appears in the
  `{...}` choice list but has **no row** in the "subcommands:" description list.
- Subcommand help never shows `(default: ...)` annotations, even for options that have
  defaults (e.g. `receive -t` defaults to 3.0); only the top-level help does.
- Help (like all output) is encoded in the console/default charset. With `LANG` unset
  the U+2019 apostrophe in the `jsonRpc`/`daemon`/`receive` help (`Don’t download ...`)
  prints as `Don?t`; with `LANG=C.UTF-8` the real character is printed. The recorded
  transcripts were taken with `LANG` unset; golden tests must pin the locale.
- Help layout (argparse4j 0.9.0 style): option column indented 2 spaces, descriptions
  start at column 26 (25 spaces of indent); an option string longer than 21 characters
  puts its description on the next line. Defaults are appended as ` (default: X)` for
  options that have a non-null default and a help text. The top-level help ends with
  the description `Commandline interface for Signal.` near the top and this epilog:
  `The global arguments are shown with 'signal-cli -h' and need to come before the subcommand, while the subcommand-specific arguments (shown with 'signal-cli SUBCOMMAND -h') need to be given after the subcommand.`
- Text wrapping width: taken from the `COLUMNS` environment variable if set, else the
  controlling terminal width, else **75** columns (the case for all recorded
  transcripts: no TTY, no `COLUMNS`). Wrapped help/description text is
  **fully justified**: extra spaces are inserted between words so each wrapped line
  (except a paragraph's last line) fills the width (e.g.
  `Raise log  level  and  include  lib  signal  logs.`). Usage lines wrap with
  continuation lines indented to align after `usage: signal-cli ` (18 spaces for the
  top-level parser; subcommand usage continuations are also indented 18 spaces).
  Unbreakable tokens may exceed the width. With `COLUMNS=120` the same texts are not
  justified/wrapped. Byte-exact compatibility therefore requires reproducing this
  wrapping/justification at width 75.

## 5. Argument-parse errors

### 5.1 Format

All parse errors go to **stderr**, nothing to stdout, exit code **1**:

```
<usage of the parser that failed, possibly several lines>
signal-cli: error: <message>
```

optionally followed by a blank line, `Did you mean:` and one line per suggestion
(each prefixed by a TAB). The usage block is the usage of the parser where the error
occurred: the top-level usage for global errors, `usage: signal-cli CMD ...` for
subcommand errors (the `signal-cli: error:` prefix stays `signal-cli` even for
subcommand errors). The whole `signal-cli: error: <message>` text is wrapped and
justified at the format width (section 4), e.g.

```
signal-cli:  error:  argument   -a/--account/-u/--username:   expected  one
argument
```

No help, no "try -h" hint. Note the top-level usage block always ends with the line
`                  ...` after the subcommand choice list.

### 5.2 Messages (verbatim before wrapping)

| Situation | Message |
|---|---|
| No subcommand | `too few arguments` |
| Unknown subcommand `X` | `invalid choice: 'X' (choose from 'acceptCall', 'addDevice', ..., 'version')` (all 61 names, each single-quoted, comma+space separated, sorted as in section 9) followed by `Did you mean:` suggestions (e.g. `bogus` -> `trust`; empty string -> all 61 names) |
| Unknown option / extra positional `X` | `unrecognized arguments: 'X'` (multiple are listed space-separated, each quoted), optionally followed by `Did you mean:` (e.g. `--bogus` -> `--dbus`; `send --bogus` -> `--group`) |
| Option missing its value | `argument -o/--output: expected one argument` (the argument is named by all its option strings joined with `/`) |
| Bad choice | `argument -o/--output: could not convert 'xml' (choose from {plain-text,json})` |
| Bad integer (64-bit option) | `argument -t/--target-timestamp: could not convert 'abc' to integer (64 bits)` (the ` (64 bits)` / ` (32 bits)` suffix is always present, for non-numeric, fractional and out-of-range values alike; at width 75 it often wraps to its own line) |
| Bad value for a choices-restricted (non-enum) option | `argument --type: invalid choice: 'foo' (choose from {read,viewed})` (e.g. `sendReceipt --type`), as opposed to the `could not convert ... (choose from {...})` form used by enum-typed options such as `-o` or `sendMessageRequestResponse --type` |
| Boolean-valued options shown as `{true,false}` | Any string is accepted at parse time (`yes`, `maybe`, `TRUE` pass); per source the value is true only if it equals `true` ignoring case. Only a missing value is an error (`expected one argument`). |
| Mutually exclusive options both given | `argument --dbus-system: not allowed with argument --dbus` |
| Ambiguous abbreviation | `ambiguous option: --d could match --data-dir, --dbus, --dbus-system, --disable-send-log` |
| Missing required option | `argument --call-id is required`; with aliases: `argument -t/--target-timestamp is required`, `argument -g/--group-id/--group is required` (only one missing option is reported even if several are missing: observed `getSticker` with neither `--pack-id` nor `--sticker-id` reports `argument --sticker-id is required`, i.e. the later-declared one; see per-command files) |
| Required mutually exclusive group, none given | `one of the arguments -c/--contact -p/--profile -g/--group-id is required` (option names joined by `/`, alternatives separated by a space) |
| Missing required positional | `too few arguments` (e.g. `removeContact`, `setPin`) |

## 6. Exit codes

| Code | Meaning (man page) | What triggers it (binary) |
|---|---|---|
| 0 | success | Command completed; also `-h/--help` and `--version`. |
| 1 | "Error is probably caused and fixable by the user" | Every argument-parse error; config-file load/parse errors; unsupported output mode; account selection errors (no local users, multiple users, invalid account number, user not registered, `-a` with `link`, `-a` with a multi-account-only command); invalid input values detected by commands (invalid group id, invalid phone number, missing recipients, etc.); D-Bus daemon not running. |
| 2 | "Some unexpected error" | Unexpected internal failures (`Error loading state file ...`), D-Bus client failures, and any uncaught exception (the JVM prints a stack trace to stderr). |
| 3 | "Server or IO error" | Network / server / file I/O errors (e.g. `Failed to read local accounts list`; a failed server connection, e.g. `Failed to register: ...`). |
| 4 | "Sending failed due to untrusted key" | Send-family commands when a recipient identity is untrusted. |
| 5 | "Server rate limiting error" | Server rate limit hit (message starts `Rate limit reached`). |
| 6 | "CAPTCHA was rejected" | Captcha/proof required or rejected during registration / sending. |

The error message for codes 1-6 is a single `System.err` print of the message
(possibly multi-line) followed by newline, nothing on stdout. With `-v` (verbose >= 1)
and an underlying cause, the cause's stack trace is printed to stderr after the message.

## 7. Configuration file (global defaults)

Source: ConfigLoader/GlobalConfig + man page "Configuration file"; verified by binary.

- JSON object files, read before argument parsing. Files merged in this order (later
  keys override earlier; nested objects merged key-wise):
  1. `/etc/signal-cli/config.json`
  2. **either** the file named by `SIGNAL_CLI_CONFIG` (if set and non-empty), **or**,
     only when that variable is unset/empty, `$XDG_CONFIG_HOME/signal-cli/config.json`
     (if `XDG_CONFIG_HOME` set and non-empty) else `$HOME/.config/signal-cli/config.json`.
  Missing files are skipped silently.
- Discrepancy: the man page lists SIGNAL_CLI_CONFIG and the per-user file as two
  merged layers. **Binary:** setting `SIGNAL_CLI_CONFIG` disables the per-user file
  entirely (observed: XDG file `{"output":"json"}` + `SIGNAL_CLI_CONFIG` file
  `{"output":"PLAIN_TEXT"}` -> plain-text; XDG file alone -> json).
- Recognised keys (exactly 12, camelCase): `verbose` (int), `logFile` (string),
  `scrubLog` (bool), `dataDir` (string), `busName` (string), `dbus` (bool),
  `dbusSystem` (bool), `output`, `serviceEnvironment`, `trustNewIdentities`,
  `disableSendLog` (bool), `account` (string).
- Enum values are parsed leniently: trimmed, lower-cased, and for `output` and
  `trustNewIdentities` all non-alphanumerics removed, so `PLAIN_TEXT`, `plain-text`,
  `PlainText` are all plain-text and `on_first_use` is on-first-use;
  `serviceEnvironment` only trims and lower-cases (`live`, `staging`).
- `account` is accepted but has **no effect** in 0.14.7 (observed: config
  `{"account":"+15555550123"}` + `version` -> normal output, no account used), despite
  the man page listing it.
- Config values become the parser defaults (and are shown as `(default: ...)` in help,
  e.g. `Choose to output in plain text or JSON (default: json)`); command-line options
  override them.
- Errors (stderr, exit 1, before anything else runs):
  - unreadable/malformed JSON: `Failed to load config from <path>: <parser detail>`,
    e.g. `Failed to load config from /tmp/x/bad.json: Unexpected character ('b' (code 98)): was expecting double-quote to start field name` followed by a second line
    ` at [Source: REDACTED (`StreamReadFeature.INCLUDE_SOURCE_IN_LOCATION` disabled); line: 1, column: 2]`.
  - invalid value or unknown key: `Failed to parse configuration file(s): <detail>`,
    where detail is the JSON library's message, e.g. for `{"output":"xml"}`
    `Cannot construct instance of `org.asamk.signal.OutputType`, problem: Invalid output type: xml` + location line; for an unknown key
    `Unrecognized field "unknownKey" (class org.asamk.signal.GlobalConfig), not marked as ignorable (12 known properties: ...)`.
    These details are library-specific; a reimplementation should match the prefix
    and exit code; full byte-equality of the detail is not realistic.

## 8. Logging (stderr)

Byte-exact details (line formats, name abbreviation, logger names, `--scrub-log` rules)
are in [logging.md](logging.md); it takes precedence over this summary.

- Quirk: logging options are read by an early pass over the **entire** argument list
  that ignores unknown arguments. Consequently a subcommand's own `-v` (e.g.
  `register -v` meaning `--voice`, `trust -v SAFETY_NUMBER`, `finishChangeNumber -v CODE`,
  `startChangeNumber -v`) **also** raises global verbosity, producing DEBUG lines on
  stderr. The long forms (`--voice`, `--verified-safety-number`, ...) do not. Likewise
  `--log-file`/`--scrub-log` would be honored anywhere on the line. Separately,
  `-h` anywhere also triggers the logging-parser help block (section 4).
- Level: default WARN (signal-cli's own loggers INFO); `-v` -> INFO (own: DEBUG);
  `-vv` -> own loggers TRACE; `-vvv` -> everything TRACE.
- Console log line format with verbose 0 (or whenever `--log-file` is given):
  `<LEVEL padded to 5> <short logger name> - <message>`, e.g.
  `WARN  MultiAccountManager - Ignoring +15555550111: User is not registered. (NotRegisteredException)`.
- Console format with verbose >= 1 and no log file:
  `<ISO-8601 UTC timestamp with ms, e.g. 2026-10-23T05:58:05.928Z> [<thread>] <LEVEL padded to 5> <logger name, abbreviated to 36 chars> - <message>`.
  E.g. `-v --version` prints
  `... [main] DEBUG org.asamk.signal.util.IOUtils - XDG_RUNTIME_DIR not set, falling back to temp dir`.
- `--log-file F`: detailed format goes to the file; console keeps only WARN+ (and INFO+
  from signal-cli's own loggers), in the simple format.
- `--scrub-log` censors log text (not command output): phone numbers keep the leading
  `+` and the last 2 digits, middle replaced by `*`; UUIDs become
  `********-****-****-****-**********` + last 2 hex chars; e-mail addresses keep first
  char then `...@...`; group ids keep the first and last 2 chars; IPv4 addresses become
  `...ipv4...`; domain names with a common TLD become `***.<tld>` (except
  `signal.org`/`whispersystems.org`).
- Logs are not part of the compatibility contract beyond: they never go to stdout.

## 9. Subcommands

61 subcommands, listed in the parser's (case-sensitive lexicographic) order. "Cat."
is the invocation category: **L** local (needs exactly one resolvable registered
account), **M** multi-account capable (runs over all accounts when `-a` omitted),
**R** registration (account must be a phone number; works on unregistered accounts),
**P** provisioning (`-a` forbidden). Output: **pj** = plain-text+json, **p** =
plain-text only, **j** = json only.

| Command | Cat. | Out | Summary (help text) | Spec |
|---|---|---|---|---|
| acceptCall | L | pj | Accept an incoming voice call. | [acceptCall.md](acceptCall.md) |
| addDevice | L | pj | Link another device to this device. Only works, if this is the primary device. | [addDevice.md](addDevice.md) |
| addStickerPack | L | pj | Install a sticker pack for this account. | [addStickerPack.md](addStickerPack.md) |
| block | L | pj | Block the given contacts or groups (no messages will be received) | [block.md](block.md) |
| daemon | M (or L with `-a`) | pj | Run in daemon mode and provide a JSON-RPC or an experimental dbus interface. | [daemon.md](daemon.md) |
| deleteLocalAccountData | R | pj | Delete all local data for this account. Data should only be deleted if the account is unregistered. CAUTION: This cannot be undone. | [deleteLocalAccountData.md](deleteLocalAccountData.md) |
| finishChangeNumber | L | pj | Verify the new number using the code received via SMS or voice. | [finishChangeNumber.md](finishChangeNumber.md) |
| getAttachment | L | pj | Retrieve an already downloaded attachment base64 encoded. | [getAttachment.md](getAttachment.md) |
| getAvatar | L | pj | Retrieve the avatar of a contact, contact's profile or group base64 encoded. | [getAvatar.md](getAvatar.md) |
| getSticker | L | pj | Retrieve the sticker of a sticker pack base64 encoded. | [getSticker.md](getSticker.md) |
| getUserStatus | L | pj | Check if the specified phone number/s have been registered | [getUserStatus.md](getUserStatus.md) |
| hangupCall | L | pj | Hang up an active voice call. | [hangupCall.md](hangupCall.md) |
| joinGroup | L | pj | Join a group via an invitation link. | [joinGroup.md](joinGroup.md) |
| jsonRpc | M (or L with `-a`) | j | Take commands from standard input as line-delimited JSON RPC while receiving messages. | [jsonRpc.md](jsonRpc.md) |
| link | P | p | Link to an existing device, instead of registering a new number. | [link.md](link.md) |
| listAccounts | M only | pj | Show a list of registered accounts. | [listAccounts.md](listAccounts.md) |
| listCalls | L | pj | List active voice calls. | [listCalls.md](listCalls.md) |
| listContacts | L | pj | Show a list of known contacts with names and profiles. | [listContacts.md](listContacts.md) |
| listDevices | L | pj | Show a list of linked devices. | [listDevices.md](listDevices.md) |
| listGroups | L | pj | List group information including names, ids, active status, blocked status and members | [listGroups.md](listGroups.md) |
| listIdentities | L | pj | List all known identity keys and their trust status, fingerprint and safety number. | [listIdentities.md](listIdentities.md) |
| listStickerPacks | L | pj | Show a list of known sticker packs. | [listStickerPacks.md](listStickerPacks.md) |
| quitGroup | L | pj | Send a quit group message to all group members and remove self from member list. | [quitGroup.md](quitGroup.md) |
| receive | L | pj | Query the server for new messages. | [receive.md](receive.md) |
| register | R | pj | Register a phone number with SMS or voice verification. | [register.md](register.md) |
| rejectCall | L | pj | Reject an incoming voice call. | [rejectCall.md](rejectCall.md) |
| remoteDelete | L | pj | Remotely delete a previously sent message. | [remoteDelete.md](remoteDelete.md) |
| removeContact | L | pj | Remove the details of a given contact | [removeContact.md](removeContact.md) |
| removeDevice | L | pj | Remove a linked device. | [removeDevice.md](removeDevice.md) |
| removePin | L | pj | Remove the registration lock pin. | [removePin.md](removePin.md) |
| send | L | pj | Send a message to another user or group. | [send.md](send.md) |
| sendAdminDelete | L | pj | Send admin delete message for a previously received or sent message. | [sendAdminDelete.md](sendAdminDelete.md) |
| sendContacts | L | pj | Send a synchronization message with the local contacts list to all linked devices. | [sendContacts.md](sendContacts.md) |
| sendMessageRequestResponse | L | pj | Send response to a message request to linked devices. | [sendMessageRequestResponse.md](sendMessageRequestResponse.md) |
| sendPaymentNotification | L | pj | Send a payment notification. | [sendPaymentNotification.md](sendPaymentNotification.md) |
| sendPinMessage | L | pj | Send pin message for a previously received or sent message. | [sendPinMessage.md](sendPinMessage.md) |
| sendPollCreate | L | pj | Create a poll and send it to another user or group. | [sendPollCreate.md](sendPollCreate.md) |
| sendPollTerminate | L | pj | Terminate a poll and send it to another user or group. | [sendPollTerminate.md](sendPollTerminate.md) |
| sendPollVote | L | pj | Vote on a poll and send it to another user or group. | [sendPollVote.md](sendPollVote.md) |
| sendReaction | L | pj | Send reaction to a previously received or sent message. | [sendReaction.md](sendReaction.md) |
| sendReceipt | L | pj | Send a read or viewed receipt to a previously received message. | [sendReceipt.md](sendReceipt.md) |
| sendStory | L | pj | Post a story to your Story. | [sendStory.md](sendStory.md) |
| sendSyncRequest | L | pj | Send a synchronization request message to primary device (for group, contacts, ...). | [sendSyncRequest.md](sendSyncRequest.md) |
| sendTyping | L | pj | Send typing message to trigger a typing indicator for the recipient. Indicator will be shown for 15seconds unless a typing STOP message is sent first. | [sendTyping.md](sendTyping.md) |
| sendUnpinMessage | L | pj | Send unpin message for a previously received or sent message. | [sendUnpinMessage.md](sendUnpinMessage.md) |
| setPin | L | pj | Set a registration lock pin, to prevent others from registering this number. | [setPin.md](setPin.md) |
| startCall | L | pj | Start an outgoing voice call. | [startCall.md](startCall.md) |
| startChangeNumber | L | pj | Change account to a new phone number with SMS or voice verification. | [startChangeNumber.md](startChangeNumber.md) |
| submitRateLimitChallenge | L | pj | Submit a captcha challenge to lift the rate limit. This command should only be necessary when sending fails with a proof required error. | [submitRateLimitChallenge.md](submitRateLimitChallenge.md) |
| trust | L | pj | Set the trust level of a given number. | [trust.md](trust.md) |
| unblock | L | pj | Unblock the given contacts or groups (messages will be received again) | [unblock.md](unblock.md) |
| unregister | L | pj | Unregister the current device from the signal server. | [unregister.md](unregister.md) |
| updateAccount | L | pj | Update the account attributes on the signal server. | [updateAccount.md](updateAccount.md) |
| updateConfiguration | L | pj | Update signal configs and sync them to linked devices. | [updateConfiguration.md](updateConfiguration.md) |
| updateContact | L | pj | Update the details of a given contact | [updateContact.md](updateContact.md) |
| updateDevice | L | pj | Update a linked device. | [updateDevice.md](updateDevice.md) |
| updateGroup | L | pj | Create or update a group. | [updateGroup.md](updateGroup.md) |
| updateProfile | L | pj | Set a name, about and avatar image for the user profile | [updateProfile.md](updateProfile.md) |
| uploadStickerPack | L | pj | Upload a new sticker pack, consisting of a manifest file and the stickers images. | [uploadStickerPack.md](uploadStickerPack.md) |
| verify | R | pj | Verify the number using the code received via SMS or voice. | [verify.md](verify.md) |
| version | M (or L with `-a`) | pj | (no help text; man page: "Show version information.") | [version.md](version.md) |

Not exposed on the CLI: `startLink` and `finishLink` exist only as JSON-RPC methods
(they have no subparser); `signal-cli startLink` is an invalid choice.

Shared reference files: [_received-message-output.md](_received-message-output.md),
[_send-results.md](_send-results.md). Raw transcripts: [observations.md](observations.md).

## 10. Recipient syntax (man page, Description)

Wherever a RECIPIENT is expected, it may be: a phone number (E.164 with `+`), an ACI
(bare UUID), a PNI (UUID prefixed `PNI:`), or a username prefixed `u:` (e.g.
`u:test.000`). Invalid numbers produce
`Invalid phone number '<input>': <detail>` (exit 1); invalid group ids
`Invalid group id: <detail>` (exit 1); empty recipient sets `No recipients given` (exit 1).

## 11. Verbatim top-level help (`signal-cli --help`, stdout, exit 0)

See the first help transcript in [observations.md](observations.md) (section
"Global"); it is reproduced byte-for-byte there, including the leading 7-line logging
parser block, justified wrapping at width 75, and the trailing epilog.
