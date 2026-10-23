# signal-cli sendMessageRequestResponse

## Sources

- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/commands/SendMessageRequestResponseCommand.java (tag v0.14.7)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/commands/MessageRequestResponseType.java
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/util/CommandUtil.java
- man page https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/man/signal-cli.1.adoc section `=== sendMessageRequestResponse`
- Binary commands run (`--config <CONFIG>` empty scratch dir):
  - `signal-cli --config <CONFIG> sendMessageRequestResponse --help`
  - `signal-cli --config <CONFIG> sendMessageRequestResponse`
  - `signal-cli --config <CONFIG> sendMessageRequestResponse --type foo`
  - `signal-cli --config <CONFIG> sendMessageRequestResponse --type ACCEPT +15555550199`
  - `signal-cli --config <CONFIG> sendMessageRequestResponse --type accept +15555550199`

## Summary

Help line: `Send response to a message request to linked devices.`

Records an accept/delete response to a message request from the given
contacts/groups and syncs it to the account's linked devices.

## Invocation category & account requirement

Local single-account command; requires a registered account (see index.md).
Contacts the Signal server (sync message).

## Output modes

`plain-text` (default) and `json`. Neither mode prints anything.

## Arguments

| Names | Arity | Value | Default | Required | Help text (verbatim) | Notes |
|---|---|---|---|---|---|---|
| `-g`, `--group-id`, `--group` | 0..n | GROUP_ID | none | no | Specify the recipient group ID. | |
| `recipient` (positional) | 0..n | string | none | no | Specify the recipients' phone number. | |
| `-u`, `--username` | 0..n | USERNAME | none | no | Specify the recipient username or username link. | |
| `--type` | 1 | `{accept,delete}` | none | yes | Type of message request response | case-sensitive lowercase; uses the "could not convert" error form, not "invalid choice" |
| `-h`, `--help` | 0 | | | | show this help message and exit | |

No `--note-to-self` option.

## Constraints & validation

After account loading: recipients resolved (see
[_send-results.md](_send-results.md#common-recipient-parsing-errors-all-exit-1-stderr));
none -> `No recipients given` (exit 1).

## Behavior

Sends the response (accept or delete) for each recipient. Per-recipient send
results are ignored entirely: no output, and exit 0 even if the sync failed.

## Plain-text output

None (empty stdout) on success.

## JSON output

None (empty stdout) on success.

## Errors

| Condition | Stream | Exact message | Exit |
|---|---|---|---|
| missing `--type` (parse) | stderr | usage + `signal-cli: error: argument --type is required` (observed) | 1 |
| bad `--type` value (parse) | stderr | usage + `signal-cli: error: argument --type:  could  not  convert 'foo' (choose from` / `{accept,delete})` (observed; same form for `ACCEPT`: `signal-cli: error: argument  --type:  could  not  convert  'ACCEPT' (choose` / `from {accept,delete})`) | 1 |
| no account | stderr | `No local users found, you first need to register or link an account` (observed) | 1 |
| not registered | stderr | `User <account> is not registered.` | 1 |
| no recipients / invalid recipient / invalid group | stderr | see _send-results.md | 1 |

## --help transcript

Command: `signal-cli --config <CONFIG> sendMessageRequestResponse --help` -- exit code: 0, stderr empty. Stdout:

```text
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli sendMessageRequestResponse [-h]
                  [-g [GROUP_ID [GROUP_ID ...]]]
                  [-u [USERNAME [USERNAME ...]]] --type {accept,delete}
                  [recipient [recipient ...]]

positional arguments:
  recipient              Specify the recipients' phone number.

named arguments:
  -h, --help             show this help message and exit
  -g [GROUP_ID [GROUP_ID ...]], --group-id [GROUP_ID [GROUP_ID ...]], --group [GROUP_ID [GROUP_ID ...]]
                         Specify the recipient group ID.
  -u [USERNAME [USERNAME ...]], --username [USERNAME [USERNAME ...]]
                         Specify the recipient username or username link.
  --type {accept,delete}
                         Type of message request response
```

## Doc vs binary discrepancies

None found.
