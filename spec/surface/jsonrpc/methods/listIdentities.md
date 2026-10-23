# Method `listIdentities`

## Sources

- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/commands/ListIdentitiesCommand.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/util/Util.java (safety number formatting), util/Hex.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/lib/src/main/java/org/asamk/signal/manager/api/TrustLevel.java
- Binary observation: none. Conventions: [index.md](index.md#parameter-conventions).

## Summary

Lists known identity keys with trust level, fingerprint and safety numbers.

CLI equivalent: `signal-cli -a ACCOUNT -o json listIdentities [-n NUMBER]`.

Account requirement: **single**.

## Params

| JSON key(s) | Type | Req. | Default | CLI flag | Notes |
|---|---|---|---|---|---|
| `number` | string | no | all identities | `-n/--number` | Only identities for this recipient (any form from index.md rule 10, despite the name). |

## Result

JSON array of identity objects, all keys always present:

| Key | Type | Notes |
|---|---|---|
| `number` | string \| null | |
| `uuid` | string \| null | |
| `fingerprint` | string | hex of the identity key fingerprint bytes, lowercase pairs **separated by single spaces** (e.g. `05 ab cd ...`) |
| `safetyNumber` | string \| null | 60 digits split into 12 groups of 5 separated by single spaces |
| `scannableSafetyNumber` | string \| null | standard base64 of the scannable safety number bytes |
| `trustLevel` | string | `UNTRUSTED`, `TRUSTED_UNVERIFIED`, `TRUSTED_VERIFIED` |
| `addedTimestamp` | integer (int64) | ms epoch when the key was first seen |

Fingerprint formatting: each byte as two lowercase hex digits, bytes joined by
a single space, no trailing space; empty string for zero bytes.

## Errors

| Code | Message | Trigger |
|---|---|---|
| -1 | `Invalid phone number '<input>': <reason>` | bad `number` |
