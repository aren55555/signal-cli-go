# signal-cli removeContact

## Sources
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/commands/RemoveContactCommand.java
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/util/CommandUtil.java
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/man/signal-cli.1.adoc, section `=== removeContact`
- Binary observations:
  - `signal-cli --config <CONFIG> removeContact --help`
  - `signal-cli --config <CONFIG> removeContact`
  - `signal-cli --config <CONFIG> removeContact --hide --forget +1`
  - `signal-cli --config <CONFIG> removeContact +1 +2`
  - `signal-cli --config <CONFIG> -a +15555550123 removeContact +15555550124`

## Summary
Help text: "Remove the details of a given contact"

The command works only on local data and makes no network call.

## Invocation category & account requirement
This is a single-account local command and needs a registered account.

## Output modes
`plain-text` (the default) and `json`. **Nothing is printed on success.**

## Arguments

| names | arity | value type / metavar | default | required | help text (verbatim) | notes |
|---|---|---|---|---|---|---|
| `-h`, `--help` | 0 | – | – | no | `show this help message and exit` | |
| `recipient` (positional) | 1 | recipient | – | **yes** | `Contact number` | Exactly one. Extra positionals give `unrecognized arguments`. |
| `--hide` | 0 | flag | false | no | `Hide the contact in the contact list, but keep the data.` | Mutually exclusive with `--forget`. |
| `--forget` | 0 | flag | false | no | `Delete all data associated with this contact, including identity keys and sessions.` | Mutually exclusive with `--hide`. |

Mutually exclusive group: `--hide | --forget`.

## Constraints & validation
- Both flags given: `argument --forget: not allowed with argument --hide` (exit 1, observed).
- An extra positional gives `unrecognized arguments: '+2'` (exit 1, observed).
- An invalid recipient gives `Invalid phone number '<s>': <reason>` (exit 1).

## Behavior
- `--hide`: the recipient is marked hidden, and its data is kept.
- `--forget`: all data for the recipient is deleted, including identity keys and sessions.
- Neither flag: only the contact details (name and similar) are removed.

## Plain-text output
None.

## JSON output
None.

## Errors

| condition | stream | exact message | exit code |
|---|---|---|---|
| Missing recipient | stderr | usage + `signal-cli: error: too few arguments` | 1 |
| `--hide` with `--forget` | stderr | usage + `signal-cli: error: argument --forget: not allowed with argument --hide` | 1 |
| Extra positional | stderr | usage + `signal-cli: error: unrecognized arguments: '+2'` | 1 |
| No account / not registered (observed) | stderr | `No local users found, you first need to register or link an account` / `User +15555550123 is not registered.` | 1 |
| Invalid recipient | stderr | `Invalid phone number '<s>': <reason>` | 1 |

## --help transcript
Command: `signal-cli --config <CONFIG> removeContact --help`

exit code: 0

stdout:
```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli removeContact [-h] [--hide | --forget] recipient

positional arguments:
  recipient              Contact number

named arguments:
  -h, --help             show this help message and exit
  --hide                 Hide the contact  in  the  contact  list, but keep
                         the data.
  --forget               Delete all  data  associated  with  this  contact,
                         including identity keys and sessions.
```

stderr:
```
```


## Observed invocations
### `signal-cli --config <CONFIG> removeContact`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli removeContact [-h] [--hide | --forget] recipient
signal-cli: error: too few arguments
```

### `signal-cli --config <CONFIG> removeContact --hide --forget +1`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli removeContact [-h] [--hide | --forget] recipient
signal-cli: error: argument --forget: not allowed with argument --hide
```

### `signal-cli --config <CONFIG> removeContact +1 +2`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli removeContact [-h] [--hide | --forget] recipient
signal-cli: error: unrecognized arguments: '+2'
```

### `signal-cli --config <CONFIG> -a +15555550123 removeContact +15555550124`

exit code: 1

stdout:
```
```

stderr:
```
User +15555550123 is not registered.
```


## Doc vs binary discrepancies
None. The man page says "Remove the info of a given contact"; the binary help says "details".
