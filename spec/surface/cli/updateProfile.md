# signal-cli updateProfile

## Sources
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/commands/UpdateProfileCommand.java
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/man/signal-cli.1.adoc, section `=== updateProfile`
- Binary observations:
  - `signal-cli --config <CONFIG> updateProfile --help`
  - `signal-cli --config <CONFIG> updateProfile`
  - `signal-cli --config <CONFIG> updateProfile --avatar a --remove-avatar`
  - `signal-cli --config <CONFIG> -a +15555550123 updateProfile --given-name x`

## Summary
Help text: "Set a name, about and avatar image for the user profile"

The command updates the account's encrypted profile on the server. Fields that are not given keep their current values.

## Invocation category & account requirement
This is a single-account local command and needs a registered account.

## Output modes
`plain-text` (the default) and `json`. **Neither mode prints anything on success.**

## Arguments

| names | arity | value type / metavar | default | required | help text (verbatim) | notes |
|---|---|---|---|---|---|---|
| `-h`, `--help` | 0 | – | – | no | `show this help message and exit` | |
| `--given-name`, `--name` | 1 | string / `GIVEN_NAME` | – | no | `New profile (given) name` | |
| `--family-name` | 1 | string / `FAMILY_NAME` | – | no | `New profile family name (optional)` | |
| `--about` | 1 | string / `ABOUT` | – | no | `New profile about text` | |
| `--about-emoji` | 1 | string / `ABOUT_EMOJI` | – | no | `New profile about emoji` | |
| `--mobile-coin-address`, `--mobilecoin-address` | 1 | base64 string / `MOBILE_COIN_ADDRESS` | – | no | `New MobileCoin address (Base64 encoded public address)` | |
| `--avatar` | 1 | file path / `AVATAR` | – | no | `Path to new profile avatar` | Mutually exclusive with `--remove-avatar`. |
| `--remove-avatar` | 0 | flag | false | no | (no help text; the help shows the bare flag) | Mutually exclusive with `--avatar`. |

Mutually exclusive group: `--avatar | --remove-avatar`.

## Constraints & validation
- If both avatar flags are given, parsing fails with `argument --remove-avatar: not allowed with argument --avatar` (exit 1, observed).
- Invalid base64 in `--mobile-coin-address` is not caught by the command. It produces an uncaught error: a Java stack trace on stderr with an `IllegalArgumentException`, and exit 2. This comes from source and was not observed.
- Running with no options is accepted: the command does not check that at least one field was given. The server-side effect was not observed.

## Behavior
- Updates the given fields. With `--avatar`, it uploads the file as the new avatar. With `--remove-avatar`, it deletes the avatar.

## Plain-text output
None.

## JSON output
None. stdout is empty.

## Errors

| condition | stream | exact message | exit code |
|---|---|---|---|
| Avatar mutex violated | stderr | usage + `signal-cli: error: argument --remove-avatar: not allowed with argument --avatar` (re-wrapped; see Observed) | 1 |
| No account / not registered (observed) | stderr | `No local users found, you first need to register or link an account` / `User +15555550123 is not registered.` | 1 |
| Network or I/O error, or unreadable avatar | stderr | `Update profile error: <msg>` | 3 |
| Bad base64 MobileCoin address | stderr | Java stack trace | 2 |

## --help transcript
Command: `signal-cli --config <CONFIG> updateProfile --help`

exit code: 0

stdout:
```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli updateProfile [-h] [--given-name GIVEN_NAME]
                  [--family-name FAMILY_NAME] [--about ABOUT]
                  [--about-emoji ABOUT_EMOJI]
                  [--mobile-coin-address MOBILE_COIN_ADDRESS]
                  [--avatar AVATAR | --remove-avatar]

named arguments:
  -h, --help             show this help message and exit
  --given-name GIVEN_NAME, --name GIVEN_NAME
                         New profile (given) name
  --family-name FAMILY_NAME
                         New profile family name (optional)
  --about ABOUT          New profile about text
  --about-emoji ABOUT_EMOJI
                         New profile about emoji
  --mobile-coin-address MOBILE_COIN_ADDRESS, --mobilecoin-address MOBILE_COIN_ADDRESS
                         New  MobileCoin  address  (Base64  encoded  public
                         address)
  --avatar AVATAR        Path to new profile avatar
  --remove-avatar
```

stderr:
```
```


## Observed invocations
### `signal-cli --config <CONFIG> updateProfile`

exit code: 1

stdout:
```
```

stderr:
```
No local users found, you first need to register or link an account
```

### `signal-cli --config <CONFIG> updateProfile --avatar a --remove-avatar`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli updateProfile [-h] [--given-name GIVEN_NAME]
                  [--family-name FAMILY_NAME] [--about ABOUT]
                  [--about-emoji ABOUT_EMOJI]
                  [--mobile-coin-address MOBILE_COIN_ADDRESS]
                  [--avatar AVATAR | --remove-avatar]
signal-cli: error: argument --remove-avatar:  not  allowed with argument --
avatar
```

### `signal-cli --config <CONFIG> -a +15555550123 updateProfile --given-name x`

exit code: 1

stdout:
```
```

stderr:
```
User +15555550123 is not registered.
```


## Doc vs binary discrepancies
- The man page lists `--remove-avatar` with the text "Remove the avatar". The binary has no help text for it.
- The man page calls `--about` the "profile status text", which matches the binary's behavior.
