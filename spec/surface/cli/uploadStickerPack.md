# signal-cli uploadStickerPack

## Sources

- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/commands/UploadStickerPackCommand.java (tag v0.14.7)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/lib/src/main/java/org/asamk/signal/manager/util/StickerUtils.java (input format)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/lib/src/main/java/org/asamk/signal/manager/storage/stickerPacks/JsonStickerPack.java (manifest keys)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/lib/src/main/java/org/asamk/signal/manager/internal/ManagerImpl.java (upload flow)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/lib/src/main/java/org/asamk/signal/manager/api/StickerPackUrl.java (URL format)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/lib/src/main/java/org/asamk/signal/manager/storage/stickers/StickerPack.java
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/man/signal-cli.1.adoc section "uploadStickerPack"
- Binary observation (signal-cli 0.14.7, empty scratch `--config <CONFIG>`), commands run:
  - `signal-cli --config <CONFIG> uploadStickerPack --help`
  - `signal-cli --config <CONFIG> uploadStickerPack` (exit 1, `too few arguments`)
  - `signal-cli --config <CONFIG> uploadStickerPack a b` (exit 1, `unrecognized arguments: 'b'`)
  - `signal-cli --config <CONFIG> -a +15555550123 uploadStickerPack /nonexistent/manifest.json` (exit 1, `User +15555550123 is not registered.`)

## Summary

Help line: `Upload a new sticker pack, consisting of a manifest file and the stickers images.`

Uploads a sticker pack (manifest + images) to the Signal CDN under a freshly generated
random pack key, records the pack locally and prints the shareable `https://signal.art/addstickers/...` URL.

## Invocation category & account requirement

Local single-account command; requires a registered local account (common account errors,
see index.md). Contacts the server.

## Output modes

plain-text (default) and json.

## Arguments

| names | arity | value type / metavar | default | required | help text (verbatim) | notes |
|---|---|---|---|---|---|---|
| `path` (positional) | exactly 1 | file path, PATH | — | yes | `The path of the manifest.json or a zip file containing the sticker pack you wish to upload.` | |

## Constraints & validation

The path's *file name* decides the mode:
- name ends with `.zip` -> zip mode; `manifest.json` and all image files are entries in the
  zip, referenced by entry name (top level, no root path).
- name is exactly `manifest.json` -> directory mode; image files are resolved relative to the
  directory containing the manifest.
- anything else -> `Invalid sticker pack: Could not find manifest.json` (exit 1). The file is not opened in this case.

### manifest.json format

A JSON object with keys (unknown keys are rejected, see Errors):

| key | type | required | meaning |
|---|---|---|---|
| `title` | string | no (null passed through) | pack title |
| `author` | string | no | pack author |
| `cover` | object (sticker entry) | no | cover image; man page: by default the first sticker is used as cover |
| `stickers` | array of sticker entries | yes, non-empty | the stickers |

Sticker entry (also used for `cover`):

| key | type | required | meaning |
|---|---|---|---|
| `file` | string | yes | file name (directory mode) or zip entry name |
| `contentType` | string | no | MIME type; if absent or empty, it is detected from the file contents/name |
| `emoji` | string | no | associated emoji; defaults to empty string |
| `id` | integer | no | accepted (used when the manifest is read back for listing), ignored for upload |

Validation order and messages (all `Invalid sticker pack: <reason>`, exit 1):
1. `stickers` missing/null -> `Must set a 'stickers' field.`
2. `stickers` empty -> `Must include stickers.`
3. a sticker without `file` -> `Must set a 'file' field on each sticker.`
4. sticker file cannot be opened (directory mode) -> `Could not find find <file>` (note the doubled word "find", verbatim)
5. `cover` present but without `file` -> `Must set a 'file' field on the cover.`
6. cover file cannot be opened (directory mode) -> `Could not find find <file>`

Image requirements per man page (not validated locally by signal-cli; the server/clients
enforce them): static PNG or WebP, animated APNG, max 300 KiB per sticker, 512x512 px.

## Behavior

1. Reads and validates the manifest as above (stickers are read in manifest order).
2. Generates a random sticker pack key, uploads the encrypted manifest and stickers; the
   server assigns the pack id (hex).
3. Stores the pack locally as known but not installed, sends a sticker-pack-operation sync
   message (install) to own linked devices.
4. Prints the pack URL: `https://signal.art/addstickers/#pack_id=<packIdHex>&pack_key=<packKeyHex>`
   (lowercase hex, no separators).

## Plain-text output

One line on stdout: the URL, e.g.
`https://signal.art/addstickers/#pack_id=0123abcd...&pack_key=89ef...`

## JSON output

One line on stdout: object with a single key:
- `url` (string): the same URL.

Example shape: `{"url":"https://signal.art/addstickers/#pack_id=...&pack_key=..."}`

## Errors

| condition | stream | exact message | exit |
|---|---|---|---|
| missing path | stderr | `usage: signal-cli uploadStickerPack [-h] path` then `signal-cli: error: too few arguments` | 1 |
| extra positional | stderr | usage + `signal-cli: error: unrecognized arguments: '<arg>'` | 1 |
| no/unregistered local account | stderr | common account errors (index.md) | 1 |
| invalid pack (see list above) | stderr | `Invalid sticker pack: <reason>` | 1 |
| manifest.json unreadable (e.g. does not exist), malformed JSON, unknown manifest key, zip unreadable, upload/network failure | stderr | `Upload error (maybe image size too large):<io message>` (no space after the colon, verbatim) | 3 |
| zip mode: `manifest.json` entry or a referenced image entry missing from the zip | stderr | Java stack trace (internal null-pointer error) | 2 |

The last row is source-derived (unhandled internal error -> generic exit 2 with stack trace);
a reimplementation should match the exit code; the stack-trace text itself is not meaningful.

## --help transcript

Command: `signal-cli --config <CONFIG> uploadStickerPack --help` — exit code 0, stderr empty, stdout:

```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli uploadStickerPack [-h] path

positional arguments:
  path                   The path  of  the  manifest.json  or  a  zip  file
                         containing the sticker pack you wish to upload.

named arguments:
  -h, --help             show this help message and exit
```

## Doc vs binary discrepancies

- The man page's manifest example contains `//` comments; real manifest files must be plain
  JSON (comments are a parse error -> exit 3).
