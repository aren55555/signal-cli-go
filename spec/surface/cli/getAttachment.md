# signal-cli getAttachment

## Sources

- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/commands/GetAttachmentCommand.java (tag v0.14.7)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/json/JsonAttachmentData.java
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/lib/src/main/java/org/asamk/signal/manager/storage/AttachmentStore.java (id -> file mapping)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/man/signal-cli.1.adoc section "getAttachment"
- Binary observation (signal-cli 0.14.7, empty scratch `--config <CONFIG>`), commands run:
  - `signal-cli --config <CONFIG> getAttachment --help`
  - `signal-cli --config <CONFIG> getAttachment` (exit 1)
  - `signal-cli --config <CONFIG> getAttachment --id x` (exit 1)
  - `signal-cli --config <CONFIG> getAttachment --id x --recipient +15555550199 -g abc` (exit 1)
  - `signal-cli --config <CONFIG> -a +15555550123 getAttachment --id x -g abc` (exit 1, `User +15555550123 is not registered.`)

## Summary

Help line: `Retrieve an already downloaded attachment base64 encoded.`

Prints the contents of an attachment file previously downloaded by `receive`/daemon,
base64 encoded.

## Invocation category & account requirement

Local single-account command; requires a registered local account (common account errors,
see index.md). Purely local.

## Output modes

plain-text (default) and json.

## Arguments

| names | arity | value type / metavar | default | required | help text (verbatim) | notes |
|---|---|---|---|---|---|---|
| `--id` | 1 | string, ID | — | yes | `The ID of the attachment file.` | the attachment `id` as reported in received-message output (stored file name) |
| `--recipient` | 1 | string, RECIPIENT | — | one of group required | `Sender of the attachment` | mutually exclusive with `-g`; value is NOT used |
| `-g`, `--group-id` | 1 | string, GROUP_ID | — | one of group required | `Group in which the attachment was received` | mutually exclusive with `--recipient`; value is NOT used |

Required mutual-exclusion group: exactly one of { `--recipient`, `-g/--group-id` } must be given.

## Constraints & validation

- Parse-level checks only (see Errors). The `--recipient` / `--group-id` value is neither
  validated nor used; any string is accepted.
- The id is sanitized before use: every character outside `A-Z a-z 0-9 _ . -` is replaced by
  `_`. The result is looked up as a file name in the account's attachments directory
  (`<data dir>/attachments/`, where `<data dir>` is the `--config`/`--data-dir` directory; shared by all accounts). Ids therefore cannot escape that directory.

## Behavior

Reads the whole file and base64-encodes it (standard alphabet, `=` padding, single line).

## Plain-text output

One line: the base64 string.

## JSON output

One line: `{"data":"<base64>"}` — object with single key `data` (string).

## Errors

| condition | stream | exact message | exit |
|---|---|---|---|
| `--id` missing | stderr | usage (two lines, see below) + `signal-cli: error: argument --id is required` | 1 |
| neither `--recipient` nor `-g` | stderr | usage + `signal-cli: error:  one  of  the  arguments  --recipient  -g/--group-id  is` / `required` (observed, justified wrap) | 1 |
| both `--recipient` and `-g` | stderr | usage + `signal-cli: error: argument  -g/--group-id:  not  allowed  with argument --` / `recipient` (observed) | 1 |
| no/unregistered local account | stderr | common account errors (index.md) | 1 |
| no such attachment file | stderr | `Could not find attachment with ID: <id as given>` | 1 |
| read error | stderr | `An error occurred reading attachment: <id as given>` | 2 |

Usage block printed for parse errors (exact):

```
usage: signal-cli getAttachment [-h] --id ID (--recipient RECIPIENT |
                  -g GROUP_ID)
```

## --help transcript

Command: `signal-cli --config <CONFIG> getAttachment --help` — exit code 0, stderr empty, stdout:

```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli getAttachment [-h] --id ID (--recipient RECIPIENT |
                  -g GROUP_ID)

named arguments:
  -h, --help             show this help message and exit
  --id ID                The ID of the attachment file.
  --recipient RECIPIENT  Sender of the attachment
  -g GROUP_ID, --group-id GROUP_ID
                         Group in which the attachment was received
```

## Doc vs binary discrepancies

- Man page implies recipient/group identify the attachment; binary only requires one of them
  syntactically and ignores the value.
