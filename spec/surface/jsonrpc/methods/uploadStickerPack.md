# Method `uploadStickerPack`

## Sources

- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/commands/UploadStickerPackCommand.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/lib/src/main/java/org/asamk/signal/manager/api/StickerPackUrl.java
- Man page https://github.com/AsamK/signal-cli/blob/v0.14.7/man/signal-cli.1.adoc (manifest format is described there under uploadStickerPack)
- Binary observation: none. Conventions: [index.md](index.md#parameter-conventions).

## Summary

Uploads a new sticker pack from a local `manifest.json` (with images alongside)
or a zip file containing them, and returns the pack's share URL.

CLI equivalent: `signal-cli -a ACCOUNT uploadStickerPack PATH`.

Account requirement: **single**.

## Params

| JSON key(s) | Type | Req. | Default | CLI flag | Notes |
|---|---|---|---|---|---|
| `path` | string | yes | — | positional `path` | Path on the daemon host to `manifest.json` or a `.zip`. Missing -> -32603 (NullPointerException). |

## Result

`{ "url": "https://signal.art/addstickers/#pack_id=<hex>&pack_key=<hex>" }`

## Errors

| Code | Message | Trigger |
|---|---|---|
| -1 | `Invalid sticker pack: <reason>` | manifest invalid / files missing |
| -3 | `Upload error (maybe image size too large):<msg>` | I/O error (note: no space after the colon) |
| -32603 | `<msg> (<ExceptionClass>)` | missing `path` |
