# signal-cli listStickerPacks

## Sources

- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/commands/ListStickerPacksCommand.java (tag v0.14.7)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/util/Hex.java
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/lib/src/main/java/org/asamk/signal/manager/internal/ManagerImpl.java (pack listing)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/lib/src/main/java/org/asamk/signal/manager/api/StickerPack.java
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/lib/src/main/java/org/asamk/signal/manager/api/StickerPackUrl.java
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/lib/src/main/java/org/asamk/signal/manager/storage/stickerPacks/JsonStickerPack.java
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/man/signal-cli.1.adoc section "listStickerPacks"
- Binary observation (signal-cli 0.14.7, empty scratch `--config <CONFIG>`), commands run:
  - `signal-cli --config <CONFIG> listStickerPacks --help`
  - `signal-cli --config <CONFIG> listStickerPacks` (exit 1, `No local users found, you first need to register or link an account`)
  - `signal-cli --config <CONFIG> -o json listStickerPacks` (exit 1, same message)
  - `signal-cli --config <CONFIG> -a +15555550123 listStickerPacks` (exit 1, `User +15555550123 is not registered.`)

## Summary

Help line: `Show a list of known sticker packs.`

Lists all sticker packs known to the account (installed ones and ones merely seen/uploaded),
with metadata from the locally cached manifest when available.

## Invocation category & account requirement

Local single-account command; requires a registered local account (common account errors,
see index.md). Purely local, no network.

## Output modes

plain-text (default) and json.

## Arguments

No command-specific arguments besides `-h/--help`.

## Constraints & validation

None.

## Behavior

For each known pack, in local store order:
- If the pack's manifest is cached locally, title, author, cover and sticker list come from it.
  A sticker's `id` is the manifest `id`, or, if absent, the manifest `file` value parsed as an
  integer.
- If the manifest is not cached (or cannot be read — a warning
  `Failed to read local sticker pack manifest: <msg>` is logged), title and author are empty
  strings, cover is absent and the sticker list is empty.
- `installed` reflects whether the pack is installed for this account.
- The URL is `https://signal.art/addstickers/#pack_id=<packIdHex>&pack_key=<packKeyHex>`
  (lowercase hex).

## Plain-text output

One line per pack; no output at all when there are no packs:

`Pack <packIdHex>: “<title>” by “<author>” has <stickerCount> stickers. <url>`

Note the typographic quotes U+201C (“) and U+201D (”) around title and author. `<stickerCount>`
is the number of stickers in the cached manifest (0 when not cached). Installed state and
cover are not shown.

## JSON output

A single JSON array (one line), `[]` when empty. Each element:

| key | type | notes |
|---|---|---|
| `packId` | string | lowercase hex |
| `url` | string | full signal.art URL as above |
| `installed` | boolean | |
| `title` | string | `""` if manifest not cached; may be `null` if the cached manifest lacks a title |
| `author` | string | same as title |
| `cover` | object or `null` | sticker object, `null` when no cover |
| `stickers` | array of sticker objects | `[]` when manifest not cached |

Sticker object: `id` (integer), `emoji` (string or null), `contentType` (string or null).
Key order as listed. Null-valued keys are emitted (not omitted).

## Errors

| condition | stream | exact message | exit |
|---|---|---|---|
| extra argument | stderr | usage + `signal-cli: error: unrecognized arguments: '<arg>'` | 1 |
| no/unregistered local account | stderr | common account errors (index.md) | 1 |

## --help transcript

Command: `signal-cli --config <CONFIG> listStickerPacks --help` — exit code 0, stderr empty, stdout:

```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli listStickerPacks [-h]

named arguments:
  -h, --help             show this help message and exit
```

## Doc vs binary discrepancies

None found.
