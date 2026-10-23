# signal-cli updateContact

## Sources
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/commands/UpdateContactCommand.java
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/util/CommandUtil.java
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/man/signal-cli.1.adoc, section `=== updateContact`
- Binary observations:
  - `signal-cli --config <CONFIG> updateContact --help`
  - `signal-cli --config <CONFIG> updateContact`
  - `signal-cli --config <CONFIG> updateContact +1 -e x`
  - `signal-cli --config <CONFIG> -a +15555550123 updateContact +15555550124 -n x`

## Summary
Help text: "Update the details of a given contact"

The command sets local contact fields and/or the disappearing-message timer for one recipient. If the contact does not exist yet, it is created. The name and note changes are local only. The man page says they can be synchronized to linked devices with `sendContacts`.

## Invocation category & account requirement
This is a single-account local command and needs a registered account.

## Output modes
`plain-text` (the default) and `json`. **Nothing is printed on success.**

## Arguments

| names | arity | value type / metavar | default | required | help text (verbatim) | notes |
|---|---|---|---|---|---|---|
| `-h`, `--help` | 0 | – | – | no | `show this help message and exit` | |
| `recipient` (positional) | 1 | recipient | – | **yes** | `Contact number` | A number, UUID, `PNI:<uuid>`, or `u:<username>`. |
| `-n`, `--name` | 1 | string / `NAME` | – | no | `New contact name` | Legacy. See the behavior notes. |
| `--given-name` | 1 | string / `GIVEN_NAME` | – | no | `New system given name` | |
| `--family-name` | 1 | string / `FAMILY_NAME` | – | no | `New system family name` | |
| `--nick-given-name` | 1 | string / `NICK_GIVEN_NAME` | – | no | `New nick given name` | |
| `--nick-family-name` | 1 | string / `NICK_FAMILY_NAME` | – | no | `New nick family name` | |
| `--note` | 1 | string / `NOTE` | – | no | `New note` | |
| `-e`, `--expiration` | 1 | 32-bit int / `EXPIRATION` | – | no | `Set expiration time of messages (seconds)` | 0 disables expiration. |

There are no mutually exclusive groups.

## Constraints & validation
- A missing positional gives the parse error `too few arguments` (exit 1, observed).
- A non-integer `-e` gives a parse error (exit 1, observed).
- An invalid recipient gives `Invalid phone number '<s>': <reason>` (exit 1). This check runs after the account loads.

## Behavior
1. If `-e` is given, set the disappearing-message timer for the recipient first. This sends an expiration-timer update message.
2. Compute the name:
   - The given name comes from `--given-name`.
   - If `--given-name` is absent, the given name is taken from `-n/--name`. In that case, if `--family-name` is also absent, the family name is set to the empty string.
3. Store the given name, family name, nick given name, nick family name, and note. A value that was not provided is passed as "unset", which leaves that field unchanged. This step always runs, even if only `-e` was given.

## Plain-text output
None.

## JSON output
None. stdout is empty.

## Errors

| condition | stream | exact message | exit code |
|---|---|---|---|
| Missing recipient | stderr | usage + `signal-cli: error: too few arguments` | 1 |
| Non-integer `-e` | stderr | usage + `signal-cli: error: argument -e/--expiration: could not convert 'x' to integer (32 bits)` (re-wrapped; see Observed) | 1 |
| No account / not registered (observed) | stderr | `No local users found, you first need to register or link an account` / `User +15555550123 is not registered.` | 1 |
| Invalid recipient | stderr | `Invalid phone number '<s>': <reason>` | 1 |
| Recipient not registered on Signal | stderr | `The user <identifier> is not registered.` | 1 |
| Network or I/O error | stderr | `Update contact error: <msg>` | 3 |

## --help transcript
Command: `signal-cli --config <CONFIG> updateContact --help`

exit code: 0

stdout:
```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli updateContact [-h] [-n NAME] [--given-name GIVEN_NAME]
                  [--family-name FAMILY_NAME]
                  [--nick-given-name NICK_GIVEN_NAME]
                  [--nick-family-name NICK_FAMILY_NAME] [--note NOTE]
                  [-e EXPIRATION] recipient

positional arguments:
  recipient              Contact number

named arguments:
  -h, --help             show this help message and exit
  -n NAME, --name NAME   New contact name
  --given-name GIVEN_NAME
                         New system given name
  --family-name FAMILY_NAME
                         New system family name
  --nick-given-name NICK_GIVEN_NAME
                         New nick given name
  --nick-family-name NICK_FAMILY_NAME
                         New nick family name
  --note NOTE            New note
  -e EXPIRATION, --expiration EXPIRATION
                         Set expiration time of messages (seconds)
```

stderr:
```
```


## Observed invocations
### `signal-cli --config <CONFIG> updateContact`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli updateContact [-h] [-n NAME] [--given-name GIVEN_NAME]
                  [--family-name FAMILY_NAME]
                  [--nick-given-name NICK_GIVEN_NAME]
                  [--nick-family-name NICK_FAMILY_NAME] [--note NOTE]
                  [-e EXPIRATION] recipient
signal-cli: error: too few arguments
```

### `signal-cli --config <CONFIG> updateContact`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli updateContact [-h] [-n NAME] [--given-name GIVEN_NAME]
                  [--family-name FAMILY_NAME]
                  [--nick-given-name NICK_GIVEN_NAME]
                  [--nick-family-name NICK_FAMILY_NAME] [--note NOTE]
                  [-e EXPIRATION] recipient
signal-cli: error: too few arguments
```

### `signal-cli --config <CONFIG> updateContact +1 -e x`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli updateContact [-h] [-n NAME] [--given-name GIVEN_NAME]
                  [--family-name FAMILY_NAME]
                  [--nick-given-name NICK_GIVEN_NAME]
                  [--nick-family-name NICK_FAMILY_NAME] [--note NOTE]
                  [-e EXPIRATION] recipient
signal-cli: error:  argument  -e/--expiration:  could  not  convert  'x' to
integer (32 bits)
```

### `signal-cli --config <CONFIG> -a +15555550123 updateContact +15555550124 -n x`

exit code: 1

stdout:
```
```

stderr:
```
User +15555550123 is not registered.
```


## Doc vs binary discrepancies
- The man page lists `--given-name GIVEN_NAME, --name NAME` as aliases of one option. In the binary they are separate options: `-n/--name` and `--given-name`. `-n/--name` acts as a given name and clears the family name unless `--family-name` is also given. The binary is authoritative.
- The man page says "Use "u:" prefix to specify a username or username link". This is consistent with the binary.
