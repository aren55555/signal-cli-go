# Method `trust`

## Sources

- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/commands/TrustCommand.java
- https://github.com/AsamK/signal-cli/blob/v0.14.7/lib/src/main/java/org/asamk/signal/manager/api/IdentityVerificationCode.java
- Binary observation: none. Conventions: [index.md](index.md#parameter-conventions).

## Summary

Marks a recipient's identity key(s) as trusted, either all known keys
(unverified trust) or a specific key verified via safety number / fingerprint.

CLI equivalent: `signal-cli -a ACCOUNT trust RECIPIENT (-a | -v SAFETY_NUMBER)`.

Account requirement: **single**.

## Params

| JSON key(s) | Type | Req. | Default | CLI flag | Notes |
|---|---|---|---|---|---|
| `recipient` | string | yes | — | positional (required) | Single recipient. Missing -> -32603. |
| `trust-all-known-keys` / `trustAllKnownKeys` | boolean | no | false | `-a/--trust-all-known-keys` | If true, `verified-safety-number` is ignored. |
| `verified-safety-number` / `verifiedSafetyNumber` | string | required unless trust-all | — | `-v/--verified-safety-number/--verified-fingerprint` | All spaces are removed, then: length 66 -> hex fingerprint (case-insensitive); length 60 -> numeric safety number; otherwise -> standard base64 of the scannable safety number. Alias `verified-fingerprint` is **not** accepted as a JSON key. |

## Result

`{}`.

## Errors

| Code | Message | Trigger |
|---|---|---|
| -1 | `Invalid phone number '<input>': <reason>` | bad recipient |
| -1 | `Failed to set the trust for this number, make sure the number is correct.` | trust-all: no known identity |
| -1 | `You need to specify the fingerprint/safety number you have verified with -v SAFETY_NUMBER` | neither option given |
| -1 | `Safety number has invalid format, either specify the old hex fingerprint or the new safety number` | parse failure |
| -1 | `Failed to set the trust for this number, make sure the number and the fingerprint/safety number are correct.` | code does not match the key |
| -1 | `The user <identifier> is not registered.` | recipient unregistered |
| -32603 | `<msg> (<ExceptionClass>)` | missing recipient; non-boolean flag |
