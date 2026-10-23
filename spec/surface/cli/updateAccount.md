# signal-cli updateAccount

## Sources

- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/commands/UpdateAccountCommand.java (tag v0.14.7)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/output/PlainTextWriterImpl.java, .../output/JsonWriterImpl.java, .../util/Util.java (output formatting / JSON serializer settings)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/man/signal-cli.1.adoc section "updateAccount"
- Binary observation (signal-cli 0.14.7):
  - `signal-cli --config <CONFIG> updateAccount --help`
  - `signal-cli --config <CONFIG> updateAccount -u x --delete-username`
  - `signal-cli --config <CONFIG> updateAccount --number-sharing` (missing value)
  - `signal-cli --config <CONFIG> updateAccount --number-sharing yes` (no accounts)
  - `signal-cli --config <CONFIG> -a +15555550123 updateAccount --number-sharing TRUE`
  - `signal-cli --config <CONFIG> -a +15555550123 updateAccount`

## Summary

Help line (verbatim): `Update the account attributes on the signal server.`

Re-uploads the account attributes to the server (man page: "Can fix problems with receiving messages"), optionally changing device name and privacy attributes; optionally sets or deletes the account username. Also reactivates an account deactivated with `unregister` (per the `unregister` man section).

## Invocation category & account requirement

Local (single-account) command. Requires a registered account: the account given with `-a/--account` (phone number in E.164 form with leading `+`, or an ACI UUID), or, if `-a` is omitted, the single account present in the data directory. The account must exist locally and be registered, otherwise the command fails before doing anything (no network traffic). Also available as a JSON-RPC method of the same name in daemon/jsonRpc mode (not covered here). With `--dbus`/`--dbus-system` the command is forwarded over D-Bus instead (not covered here).

## Output modes

`plain-text` and `json`; default `plain-text`. Output is produced only when `-u/--username` is given.

## Arguments

| Names | Arity | Value type / metavar | Default | Required | Help text (verbatim) | Notes |
|---|---|---|---|---|---|---|
| `-n`, `--device-name` | 1 | string, `DEVICE_NAME` | unset | no | `Specify a name to describe this device.` | Man: sets a new device name for the primary or linked device. |
| `--unrestricted-unidentified-sender` | 1 | boolean, shown as `{true,false}` | unset (unchanged) | no | `Enable if anyone should be able to send you unidentified sender messages.` | see boolean note |
| `--discoverable-by-number` | 1 | boolean `{true,false}` | unset | no | `Enable/disable if the account should be discoverable by phone number` | see boolean note |
| `--number-sharing` | 1 | boolean `{true,false}` | unset | no | `Indicates if Signal should share its phone number when sending a message.` | see boolean note |
| `-u`, `--username` | 1 | string, `USERNAME` | unset | no | `Specify a username that can then be used to contact this account.` | Mutually exclusive with `--delete-username`. Man: either just the nickname (e.g. `test`) or with discriminator (e.g. `test.000`). |
| `--delete-username` | 0 (flag) | — | false | no | `Delete the username associated with this account.` | Mutually exclusive with `-u`. |

Boolean note: although the help shows `{true,false}`, the parser does NOT restrict the value: any single string is accepted (observed: `yes` and `TRUE` both pass parsing). The value is treated as true only when it equals `true` ignoring case; any other string means false (inferred from the value conversion used; not observable without an account). A missing value is a parse error: `signal-cli: error: argument --number-sharing: expected one argument`. An omitted option means "leave unchanged".

## Constraints & validation

- Mutually exclusive group: `-u/--username` | `--delete-username`. Violation (observed): stderr usage block then `signal-cli: error: argument --delete-username: not  allowed with argument -` / `u/--username` (argparse text is wrapped/justified at 75 columns; logical message is `argument --delete-username: not allowed with argument -u/--username`), exit 1.

## Behavior

1. Always sends updated account attributes (device name / unidentified-sender / discoverability / number-sharing; unspecified ones unchanged), even if no option is given.
2. If `-u` given: reserves/sets the username, then prints the resulting username (with discriminator) and username link.
3. If `--delete-username`: deletes the username. No output.

## Plain-text output

Only with `-u`: one line `Your new username: <USERNAME> (<LINK>)` where `<USERNAME>` is the full username with discriminator (e.g. `test.123`) and `<LINK>` is the username link URL, or `-` if there is no link.

## JSON output

Only with `-u`: a single JSON object on one line followed by newline: `{"username":"<string>","usernameLink":"<url string>"}`. Each key is omitted when its value is null. Keys in that order.

## Errors

| Condition | Stream | Exact message | Exit |
|---|---|---|---|
| `-o` output type not supported by the command (checked before any account handling; cannot occur for commands supporting both modes) | stderr | `Command doesn't support output type <plain-text\|json>` | 1 |
| No `-a` and zero local accounts in the data directory | stderr | `No local users found, you first need to register or link an account` | 1 |
| No `-a` and more than one local account | stderr | `Multiple users found, you need to specify an account (phone number) with -a` | 1 |
| `-a` value is neither a valid phone number (with country code) nor an ACI UUID | stderr | `Invalid account (phone number), make sure you include the country code.` | 1 |
| Account unknown locally or not registered | stderr | `User <ACCOUNT> is not registered.` (ACCOUNT echoed exactly as given / resolved) | 1 |
| Account check failed with an I/O problem | stderr | `Error while checking account <ACCOUNT>: <detail>` | 3 |
| Account check failed otherwise | stderr | `Error while checking account <ACCOUNT>: <detail>` | 2 |
| Account state file could not be loaded | stderr | `Error loading state file for user <ACCOUNT>: <detail> (<ErrorKind>)` | 2 |
| Argument parse error (unknown flag, missing required, bad value) | stderr | sub-command usage line(s) followed by `signal-cli: error: <reason>` (see index.md) | 1 |
| Updating attributes failed (I/O) | stderr | `UpdateAccount error: <detail>` | 3 |
| Setting username failed (I/O) | stderr | `Failed to set username: <detail>` | 3 |
| Username invalid | stderr | `Invalid username: <detail>` | 1 |
| Deleting username failed (I/O) | stderr | `Failed to delete username: <detail>` | 3 |

Observed: `updateAccount --number-sharing yes` with empty data dir → `No local users found, you first need to register or link an account`, exit 1 (i.e. the value passed parsing). `-a +15555550123 updateAccount` → `User +15555550123 is not registered.`, exit 1.

## --help transcript

Command: `signal-cli --config <CONFIG> updateAccount --help` — exit code 0, stderr empty, stdout:

```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli updateAccount [-h] [-n DEVICE_NAME]
                  [--unrestricted-unidentified-sender {true,false}]
                  [--discoverable-by-number {true,false}]
                  [--number-sharing {true,false}] [-u USERNAME |
                  --delete-username]

named arguments:
  -h, --help             show this help message and exit
  -n DEVICE_NAME, --device-name DEVICE_NAME
                         Specify a name to describe this device.
  --unrestricted-unidentified-sender {true,false}
                         Enable if  anyone  should  be  able  to  send  you
                         unidentified sender messages.
  --discoverable-by-number {true,false}
                         Enable/disable   if   the    account   should   be
                         discoverable by phone number
  --number-sharing {true,false}
                         Indicates if Signal should  share its phone number
                         when sending a message.
  -u USERNAME, --username USERNAME
                         Specify a  username  that  can  then  be  used  to
                         contact this account.
  --delete-username      Delete the username associated with this account.
```

The first 7 lines (the short usage of only `-h`, `-v`, `--log-file`, `--scrub-log`) are printed by the early logging-options pre-parse and precede every `--help` output; the command-specific help follows immediately with no blank line in between.

## Doc vs binary discrepancies

- Help/usage suggests the boolean options accept only `true`/`false`; the binary accepts any string (binary authoritative).
