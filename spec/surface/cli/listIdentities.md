# signal-cli listIdentities

## Sources
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/commands/ListIdentitiesCommand.java
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/util/Hex.java, Util.java (safety-number grouping), DateUtils.java
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/lib/src/main/java/org/asamk/signal/manager/api/TrustLevel.java, Identity.java
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/man/signal-cli.1.adoc, section `=== listIdentities`
- Binary observations:
  - `signal-cli --config <CONFIG> listIdentities --help`
  - `signal-cli --config <CONFIG> listIdentities`
  - `signal-cli --config <CONFIG> listIdentities -n`
  - `signal-cli --config <CONFIG> -a +15555550123 listIdentities`

## Summary
Help text: "List all known identity keys and their trust status, fingerprint and safety number."

The command reads the local identity store. It makes no network call.

## Invocation category & account requirement
This is a single-account local command and needs a registered account.

## Output modes
`plain-text` (the default) and `json`.

## Arguments

| names | arity | value type / metavar | default | required | help text (verbatim) | notes |
|---|---|---|---|---|---|---|
| `-h`, `--help` | 0 | – | – | no | `show this help message and exit` | |
| `-n`, `--number` | 1 | recipient / `NUMBER` | – | no | `Only show identity keys for the given phone number.` | Any recipient form is accepted (number, UUID, `PNI:`, `u:`). |

## Constraints & validation
An invalid `-n` gives `Invalid phone number '<s>': <reason>` (exit 1).

## Behavior
Lists all identities, or only those of the given recipient.

## Plain-text output
One line per identity:

`<identifier>: <TRUSTLEVEL> Added: TS(added) Fingerprint: <FP> Safety Number: <SN>`

- `<identifier>` is the phone number if known, otherwise the UUID.
- TRUSTLEVEL is one of `UNTRUSTED`, `TRUSTED_UNVERIFIED`, `TRUSTED_VERIFIED`.
- TS is the timestamp form `<millis> (<yyyy-MM-ddTHH:mm:ss.SSSZ>)`, in UTC.
- FP is the identity public key bytes as lowercase two-digit hex values separated by single spaces, for example `05 1a 2b ...`.
- SN is the numeric safety number split into 12 groups of equal length, separated by single spaces. For the standard 60 digits that is 12 groups of 5. If there is no safety number, SN prints as `null`.

## JSON output
A JSON array of objects, one per identity, with the keys in this order:

| key | type |
|---|---|
| number | string or null |
| uuid | string or null |
| fingerprint | string (the same spaced hex as plain text) |
| safetyNumber | string (the same grouped digits) or null |
| scannableSafetyNumber | base64 string or null |
| trustLevel | `"UNTRUSTED"`, `"TRUSTED_UNVERIFIED"`, or `"TRUSTED_VERIFIED"` |
| addedTimestamp | integer (epoch ms) |

## Errors

| condition | stream | exact message | exit code |
|---|---|---|---|
| `-n` without a value | stderr | usage + `signal-cli: error: argument -n/--number: expected one argument` | 1 |
| No account / not registered (observed) | stderr | `No local users found, you first need to register or link an account` / `User +15555550123 is not registered.` | 1 |
| Invalid recipient | stderr | `Invalid phone number '<s>': <reason>` | 1 |

## --help transcript
Command: `signal-cli --config <CONFIG> listIdentities --help`

exit code: 0

stdout:
```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli listIdentities [-h] [-n NUMBER]

named arguments:
  -h, --help             show this help message and exit
  -n NUMBER, --number NUMBER
                         Only  show  identity  keys  for  the  given  phone
                         number.
```

stderr:
```
```


## Observed invocations
### `signal-cli --config <CONFIG> listIdentities`

exit code: 1

stdout:
```
```

stderr:
```
No local users found, you first need to register or link an account
```

### `signal-cli --config <CONFIG> listIdentities -n`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli listIdentities [-h] [-n NUMBER]
signal-cli: error: argument -n/--number: expected one argument
```

### `signal-cli --config <CONFIG> -a +15555550123 listIdentities`

exit code: 1

stdout:
```
```

stderr:
```
User +15555550123 is not registered.
```


## Doc vs binary discrepancies
The man page names the argument RECIPIENT and says "for the given recipient". The binary help says "phone number" and uses the metavar `NUMBER`. In both cases any recipient form is accepted.
