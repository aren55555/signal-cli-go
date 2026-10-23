# Method `addStickerPack`

## Sources

- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/commands/AddStickerPackCommand.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/lib/src/main/java/org/asamk/signal/manager/api/StickerPackUrl.java
- Binary observation: none. Conventions: [index.md](index.md#parameter-conventions).

## Summary

Installs one or more sticker packs given their `signal.art` URLs (downloads
the manifest and marks installed; syncs to linked devices).

CLI equivalent: `signal-cli -a ACCOUNT addStickerPack --uri URI [URI ...]`.

Account requirement: **single**.

## Params

| JSON key(s) | Type | Req. | Default | CLI flag | Notes |
|---|---|---|---|---|---|
| `uri` / `uris` | list of string (single string allowed) | yes | — | `--uri` (one or more, required) | Each must parse as a URI whose fragment contains `pack_id=<hex>` and `pack_key=<hex>` (query-string syntax inside the fragment). Host/scheme are not checked. Missing entirely -> -32603 (iteration over null). |

URIs are processed in order; processing stops at the first failure (earlier
packs stay installed).

## Result

`{}`.

## Errors

| Code | Message | Trigger |
|---|---|---|
| -1 | `Sticker pack uri has invalid format: <URISyntaxException message>` | unparseable URI |
| -1 | `Invalid sticker pack link` | no fragment, missing/empty `pack_id` or `pack_key`, or non-hex values |
| -3 | `Install sticker pack failed` | I/O error while downloading/installing |
| -32603 | `<msg> (<ExceptionClass>)` | missing `uri` |
