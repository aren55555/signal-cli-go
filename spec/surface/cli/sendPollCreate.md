# signal-cli sendPollCreate

## Sources

- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/commands/SendPollCreateCommand.java (tag v0.14.7)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/util/CommandUtil.java
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/util/SendMessageResultUtils.java
- man page https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/man/signal-cli.1.adoc section `=== sendPollCreate`
- Binary commands run (`--config <CONFIG>` empty scratch dir):
  - `signal-cli --config <CONFIG> sendPollCreate --help`
  - `signal-cli --config <CONFIG> sendPollCreate`
  - `signal-cli --config <CONFIG> sendPollCreate -q Q`
  - `signal-cli --config <CONFIG> sendPollCreate -o a b`
  - `signal-cli --config <CONFIG> sendPollCreate -q Q -o`
- Shared result format: [_send-results.md](_send-results.md).

## Summary

Help line: `Create a poll and send it to another user or group.`

Sends a poll-create message with a question and 2-10 options.

## Invocation category & account requirement

Local single-account command; requires a registered account (see index.md).
Contacts the Signal server.

## Output modes

`plain-text` (default) and `json`.

## Arguments

| Names | Arity | Value | Default | Required | Help text (verbatim) | Notes |
|---|---|---|---|---|---|---|
| `recipient` (positional) | 0..n | string | none | no | Specify the recipients' phone number. | recipient syntax |
| `-g`, `--group-id`, `--group` | 0..n | GROUP_ID | none | no | Specify the recipient group ID. | |
| `-u`, `--username` | 0..n | USERNAME | none | no | Specify the recipient username or username link. | |
| `--note-to-self` | 0 | flag | false | no | Send the message to self | (no trailing period) |
| `--notify-self` | 0 | flag | false | no | If self is part of recipients/groups send a normal message, not a sync message. | |
| `-q`, `--question` | 1 | QUESTION | none | yes | Specify the poll question. | |
| `--no-multi` | 0 | flag | false | no | Allow only one option to be selected by each recipient. | default: multiple selection allowed |
| `-o`, `--option` | 1..n | OPTION | none | yes | The options for the poll | |
| `-h`, `--help` | 0 | | | | show this help message and exit | |

Note: subcommand `-o` is `--option`, distinct from the global `-o/--output`
(which must precede the subcommand).

## Constraints & validation

After account loading, in order (all exit 1):
1. Recipients (see [_send-results.md](_send-results.md)); none -> `No recipients given`.
2. Fewer than 2 options -> `Poll needs at least two options`.
3. More than 10 options -> `Poll cannot have more than 10 options`.
4. For each option in order: empty -> `Poll options must not be empty`;
   longer than 100 characters (UTF-16 code units) ->
   `Poll option "<option>" exceeds the maximum length of 100 characters`.
The question itself is not length-checked.

## Behavior

Sends the poll; "allow multiple" is true unless `--no-multi`.

## Plain-text output

See [_send-results.md](_send-results.md). The printed timestamp identifies the
poll (used later as `--poll-timestamp`).

## JSON output

See [_send-results.md](_send-results.md).

## Errors

| Condition | Stream | Exact message | Exit |
|---|---|---|---|
| missing `-o` (parse) | stderr | usage + `signal-cli: error: argument -o/--option is required` (observed) | 1 |
| missing `-q` (parse) | stderr | usage + `signal-cli: error: argument -q/--question is required` (observed) | 1 |
| `-o` without values (parse) | stderr | usage + `signal-cli: error: argument -o/--option: expected 1 argument(s)` (observed) | 1 |
| no account / not registered | stderr | see index.md | 1 |
| no recipients / invalid recipient | stderr | see _send-results.md | 1 |
| option count/length violations | stderr | messages listed above | 1 |
| group errors | stderr | `Group not found: <id>` etc. | 1 |
| unregistered recipient | stderr | `The user <identifier> is not registered.` | 1 |
| I/O error | stderr | `Failed to send message: <msg> (<ExceptionName>)` (+ prekeys suffix when applicable) | 2 |
| all sends failed | stderr | per [_send-results.md](_send-results.md) | 4/5/1 |

## --help transcript

Command: `signal-cli --config <CONFIG> sendPollCreate --help` -- exit code: 0, stderr empty. Stdout:

```text
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli sendPollCreate [-h] [-g [GROUP_ID [GROUP_ID ...]]]
                  [-u [USERNAME [USERNAME ...]]] [--note-to-self]
                  [--notify-self] -q QUESTION [--no-multi]
                  -o OPTION [OPTION ...] [recipient [recipient ...]]

positional arguments:
  recipient              Specify the recipients' phone number.

named arguments:
  -h, --help             show this help message and exit
  -g [GROUP_ID [GROUP_ID ...]], --group-id [GROUP_ID [GROUP_ID ...]], --group [GROUP_ID [GROUP_ID ...]]
                         Specify the recipient group ID.
  -u [USERNAME [USERNAME ...]], --username [USERNAME [USERNAME ...]]
                         Specify the recipient username or username link.
  --note-to-self         Send the message to self
  --notify-self          If  self  is  part  of  recipients/groups  send  a
                         normal message, not a sync message.
  -q QUESTION, --question QUESTION
                         Specify the poll question.
  --no-multi             Allow only  one  option  to  be  selected  by each
                         recipient.
  -o OPTION [OPTION ...], --option OPTION [OPTION ...]
                         The options for the poll
```

## Doc vs binary discrepancies

None of substance. Man page states option limits (2..10 options, 1..100
characters each) consistent with the source.
