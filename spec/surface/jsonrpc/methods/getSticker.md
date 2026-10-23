# Method `getSticker`

## Sources

- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/commands/GetStickerCommand.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/json/JsonAttachmentData.java, util/Hex.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/lib/src/main/java/org/asamk/signal/manager/api/StickerPackId.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/lib/src/main/java/org/asamk/signal/manager/internal/ManagerImpl.java (sticker retrieval)
- Binary observation: none. Conventions: [index.md](index.md#parameter-conventions).

## Summary

Returns one sticker image, base64 encoded. If the sticker is not stored
locally but the pack is known, a download of the pack is attempted, but the
current call still fails with "Could not find sticker" (the data is available
to a later call).

CLI equivalent: `signal-cli -a ACCOUNT getSticker --pack-id HEX --sticker-id N`.

Account requirement: **single**.

## Params

| JSON key(s) | Type | Req. | Default | CLI flag | Notes |
|---|---|---|---|---|---|
| `pack-id` / `packId` | string | yes | — | `--pack-id` | Hex pack id. Parsed leniently: pairs of characters, invalid hex digits become garbage values (no error); odd length -> -32603 (StringIndexOutOfBoundsException); missing -> -32603. |
| `sticker-id` / `stickerId` | integer (32-bit) | yes | — | `--sticker-id` | Missing -> -32603 (null unboxing). |

## Result

`{ "data": "<standard base64 of the sticker file bytes>" }`

## Errors

| Code | Message | Trigger |
|---|---|---|
| -1 | `Could not find sticker with ID: <stickerId> in pack StickerPackId{<hex>}` | sticker not stored locally |
| -32603 | `An error occurred reading sticker with ID: <stickerId> in pack StickerPackId{<hex>} (UnexpectedErrorException)` | other I/O error |
| -32603 | `<msg> (<ExceptionClass>)` | missing/invalid params as above |
