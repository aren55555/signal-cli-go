# Method `listStickerPacks`

## Sources

- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/commands/ListStickerPacksCommand.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/lib/src/main/java/org/asamk/signal/manager/api/StickerPack.java, StickerPackUrl.java
- Binary observation: none. Conventions: [index.md](index.md#parameter-conventions).

## Summary

Lists sticker packs known to the account (installed or merely seen).

CLI equivalent: `signal-cli -a ACCOUNT -o json listStickerPacks`.

Account requirement: **single**.

## Params

None. Any params object is ignored.

## Result

JSON array of pack objects, all keys present:

| Key | Type | Notes |
|---|---|---|
| `packId` | string | lowercase hex, no separators |
| `url` | string | `https://signal.art/addstickers/#pack_id=<hex>&pack_key=<hex>` |
| `installed` | boolean | |
| `title` | string \| null | |
| `author` | string \| null | |
| `cover` | sticker object \| null | |
| `stickers` | array of sticker objects | |

Sticker object: `{ "id": integer, "emoji": string|null, "contentType": string|null }`.
Title/author/stickers are only populated if the manifest has been downloaded.

## Errors

Only generic errors (account selection; -32603 on unexpected failures).
