# signal-cli addStickerPack

## Sources

- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/commands/AddStickerPackCommand.java (tag v0.14.7)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/lib/src/main/java/org/asamk/signal/manager/api/StickerPackUrl.java (URI parsing)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/lib/src/main/java/org/asamk/signal/manager/internal/ManagerImpl.java (install flow)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/man/signal-cli.1.adoc section "addStickerPack"
- Binary observation (signal-cli 0.14.7, empty scratch `--config <CONFIG>`), commands run:
  - `signal-cli --config <CONFIG> addStickerPack --help`
  - `signal-cli --config <CONFIG> addStickerPack` (exit 1, `argument --uri is required`)
  - `signal-cli --config <CONFIG> addStickerPack --uri` (exit 1, `argument --uri: expected 1 argument(s)`)
  - `signal-cli --config <CONFIG> -a +15555550123 addStickerPack --uri https://signal.art/addstickers/#pack_id=00&pack_key=00` (exit 1, `User +15555550123 is not registered.`)

## Summary

Help line: `Install a sticker pack for this account.`

Downloads the sticker pack(s) referenced by signal.art URIs and marks them installed.

## Invocation category & account requirement

Local single-account command; requires a registered local account (common account errors,
see index.md). Contacts the server (CDN download + sync message).

## Output modes

plain-text (default) and json. No output on success in either mode.

## Arguments

| names | arity | value type / metavar | default | required | help text (verbatim) | notes |
|---|---|---|---|---|---|---|
| `--uri` | 1..n (`+`) | string, URI | — | yes | `Specify the uri of the sticker pack. (e.g. https://signal.art/addstickers/#pack_id=XXX&pack_key=XXX)` | Repeating the option replaces the earlier list (last occurrence wins, argparse default). |

## Constraints & validation

URIs are processed one at a time in the given order; processing stops at the first failure
(packs from earlier URIs stay installed).

For each URI:
1. Must be a syntactically valid URI, else
   `Sticker pack uri has invalid format: <URI syntax error detail>` (exit 1), where the detail
   is the platform URI-parser message (e.g. `Illegal character in fragment at index <n>: <uri>`).
2. The URI's fragment (part after `#`, raw/undecoded) must be non-empty and is parsed as an
   `&`-separated `name=value` query string (URL-decoded, with literal `+` preserved). Both
   `pack_id` and `pack_key` must be present and non-empty, and both must be valid
   (even-length) hex. Scheme, host and path are not checked. Any failure prints
   `Invalid sticker pack link` (exit 1); additionally an ERROR log line
   `Invalid sticker pack link` is emitted on stderr before it.

## Behavior

Downloads and decrypts the pack manifest (and stickers) from the CDN using the pack key,
stores them locally, marks the pack installed, and sends a sticker-pack-operation (install)
sync message to own linked devices.

## Plain-text output

None.

## JSON output

None.

## Errors

| condition | stream | exact message | exit |
|---|---|---|---|
| `--uri` missing | stderr | `usage: signal-cli addStickerPack [-h] --uri URI [URI ...]` + `signal-cli: error: argument --uri is required` | 1 |
| `--uri` without value | stderr | usage + `signal-cli: error: argument --uri: expected 1 argument(s)` | 1 |
| no/unregistered local account | stderr | common account errors (index.md) | 1 |
| URI syntax error | stderr | `Sticker pack uri has invalid format: <detail>` | 1 |
| invalid/incomplete sticker link | stderr | log line (ERROR level) `Invalid sticker pack link`, then `Invalid sticker pack link` | 1 |
| download/decrypt failure | stderr | log line (ERROR level) `Install sticker pack failed: <io message>`, then `Install sticker pack failed` | 3 |

Log line format at default verbosity is `<LEVEL padded to 5> <logger short name> - <message>`,
e.g. `ERROR AddStickerPackCommand - Invalid sticker pack link` (format inferred from other
observed WARN lines, e.g. `WARN  MultiAccountManager - ...`; the exact logger-name text is not
interoperability-relevant).

## --help transcript

Command: `signal-cli --config <CONFIG> addStickerPack --help` — exit code 0, stderr empty, stdout:

```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli addStickerPack [-h] --uri URI [URI ...]

named arguments:
  -h, --help             show this help message and exit
  --uri URI [URI ...]    Specify the uri of the  sticker pack. (e.g. https:
                         //signal.
                         art/addstickers/#pack_id=XXX&pack_key=XXX)
```

## Doc vs binary discrepancies

- Man page shows `--uri [URI]` (single); binary accepts one or more URIs after a single `--uri`.
