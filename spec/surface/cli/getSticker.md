# signal-cli getSticker

## Sources

- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/commands/GetStickerCommand.java (tag v0.14.7)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/util/Hex.java
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/json/JsonAttachmentData.java
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/lib/src/main/java/org/asamk/signal/manager/api/StickerPackId.java
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/lib/src/main/java/org/asamk/signal/manager/internal/ManagerImpl.java (sticker retrieval)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/man/signal-cli.1.adoc section "getSticker"
- Binary observation (signal-cli 0.14.7, empty scratch `--config <CONFIG>`), commands run:
  - `signal-cli --config <CONFIG> getSticker --help`
  - `signal-cli --config <CONFIG> getSticker` (exit 1)
  - `signal-cli --config <CONFIG> getSticker --pack-id ab --sticker-id x` (exit 1)
  - `signal-cli --config <CONFIG> getSticker --pack-id ab --sticker-id 99999999999` (exit 1)
  - `signal-cli --config <CONFIG> -a +15555550123 getSticker --pack-id ab --sticker-id 1` (exit 1, `User +15555550123 is not registered.`)

## Summary

Help line: `Retrieve the sticker of a sticker pack base64 encoded.`

Prints the raw image bytes of one sticker of a locally known sticker pack, base64 encoded.

## Invocation category & account requirement

Local single-account command; requires a registered local account (common account errors,
see index.md). Normally local; may contact the CDN (see Behavior).

## Output modes

plain-text (default) and json.

## Arguments

| names | arity | value type / metavar | default | required | help text (verbatim) | notes |
|---|---|---|---|---|---|---|
| `--pack-id` | 1 | string (hex), PACK_ID | — | yes | `The ID of the sticker pack.` | hex encoded pack id |
| `--sticker-id` | 1 | 32-bit signed integer, STICKER_ID | — | yes | `The ID of the sticker.` | non-integer / out-of-range values are parse errors |

## Constraints & validation

- `--sticker-id` must parse as a 32-bit integer; otherwise
  `signal-cli: error: argument --sticker-id: could not convert '<v>' to integer (32 bits)` for
  both non-numeric and out-of-range values (the suffix is always present; it wraps onto a second line at width 75); the error text is wrapped/justified (see transcripts in Errors).
- `--pack-id` hex decoding is lenient: characters are taken pairwise; non-hex characters are
  not rejected (they decode to garbage bytes, so the pack simply will not be found). An
  odd-length pack id causes an internal error (stack trace on stderr, exit 2) — source-derived.

## Behavior

Looks up the sticker file in the local sticker cache. If not cached but the pack is known
locally (pack key available), an attempt is made to download the pack from the CDN (failure
only logs a warning `Failed to download sticker pack`); the lookup result is still the one from
before the download attempt, so the command fails with "Could not find sticker" in that run
(a later invocation may succeed). On success, the whole file is base64-encoded (standard
alphabet, with `=` padding, no line breaks).

## Plain-text output

One line: the base64 string.

## JSON output

One line: `{"data":"<base64>"}` — object with single key `data` (string).

## Errors

| condition | stream | exact message | exit |
|---|---|---|---|
| required option missing | stderr | `usage: signal-cli getSticker [-h] --pack-id PACK_ID --sticker-id STICKER_ID` + `signal-cli: error: argument --sticker-id is required` (or `--pack-id`; argparse reports the first missing in its own order — observed: `--sticker-id` when both missing) | 1 |
| non-integer sticker id | stderr | usage + `signal-cli: error: argument --sticker-id: could  not convert 'x' to integer` + newline + `(32 bits)` (observed; wrapped at 75 columns) | 1 |
| out-of-range sticker id | stderr | usage + `signal-cli: error: argument --sticker-id:  could  not convert '99999999999'` / `to integer (32 bits)` (two lines, observed) | 1 |
| no/unregistered local account | stderr | common account errors (index.md) | 1 |
| sticker not cached locally | stderr | `Could not find sticker with ID: <stickerId> in pack StickerPackId{<packIdHex>}` | 1 |
| read error | stderr | `An error occurred reading sticker with ID: <stickerId> in pack StickerPackId{<packIdHex>}` | 2 |
| odd-length `--pack-id` | stderr | stack trace | 2 |

Note the literal `StickerPackId{...}` wrapper (lowercase hex inside braces) in the two messages.

## --help transcript

Command: `signal-cli --config <CONFIG> getSticker --help` — exit code 0, stderr empty, stdout:

```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli getSticker [-h] --pack-id PACK_ID --sticker-id STICKER_ID

named arguments:
  -h, --help             show this help message and exit
  --pack-id PACK_ID      The ID of the sticker pack.
  --sticker-id STICKER_ID
                         The ID of the sticker.
```

## Doc vs binary discrepancies

- Man page calls `--sticker-id` "the index of a sticker in the sticker pack"; it is the
  sticker id as stored in the manifest.
