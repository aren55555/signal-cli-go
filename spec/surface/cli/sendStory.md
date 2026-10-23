# signal-cli sendStory

## Sources

- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/commands/SendStoryCommand.java (tag v0.14.7)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/util/CommandUtil.java
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/util/SendMessageResultUtils.java
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/lib/src/main/java/org/asamk/signal/manager/Manager.java
- man page https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/man/signal-cli.1.adoc section `=== sendStory`
- Binary commands run (`--config <CONFIG>` empty scratch dir):
  - `signal-cli --config <CONFIG> sendStory --help`
  - `signal-cli --config <CONFIG> sendStory`
  - `signal-cli --config <CONFIG> sendStory -a f.jpg -g a b`
  - `signal-cli --config <CONFIG> sendStory -a f.jpg -g`
- Shared result format: [_send-results.md](_send-results.md).

## Summary

Help line: `Post a story to your Story.`

Uploads one image or video file and posts it as a story, either to "My Story"
(default) or to a group's story.

## Invocation category & account requirement

Local single-account command; requires a registered account (see index.md).
Contacts the Signal server.

## Output modes

`plain-text` (default) and `json`.

## Arguments

| Names | Arity | Value | Default | Required | Help text (verbatim) | Notes |
|---|---|---|---|---|---|---|
| `-a`, `--attachment` | 1 | ATTACHMENT | none | yes | Specify the file path to the image or video to post as a story. | single value |
| `--no-replies` | 0 | flag | false | no | Disable replies on this story. | replies allowed by default |
| `-g`, `--group-id` | 1 | GROUP_ID | none | no | Specify a group to post the story to. Without this, posts to My Story. | single value; NO `--group` alias here |
| `-h`, `--help` | 0 | | | | show this help message and exit | |

No positional arguments.

## Constraints & validation

After account loading:
1. Empty attachment string -> `An attachment is required for sending a story.` (exit 1).
2. Group id decoded -> `Invalid group id: Failed to decode groupId (must be base64) "<id>": <cause>` (exit 1).

## Behavior

Posts the story with "allows replies" = not `--no-replies`, to the group if given.

## Plain-text output

See [_send-results.md](_send-results.md).

## JSON output

See [_send-results.md](_send-results.md).

## Errors

| Condition | Stream | Exact message | Exit |
|---|---|---|---|
| missing `-a` (parse) | stderr | usage + `signal-cli: error: argument -a/--attachment is required` (observed) | 1 |
| `-g` without value (parse) | stderr | usage + `signal-cli: error: argument -g/--group-id: expected one argument` (observed) | 1 |
| extra positional (parse) | stderr | usage + `signal-cli: error: unrecognized arguments: 'b'` (observed) | 1 |
| no account / not registered | stderr | see index.md | 1 |
| empty attachment | stderr | `An attachment is required for sending a story.` | 1 |
| invalid group id | stderr | `Invalid group id: ...` | 1 |
| group not found / not a member | stderr | `Group not found: <id>` / `User is not a member in group: <name> (<id>)` | 1 |
| attachment invalid or I/O error | stderr | `Failed to send story: <msg> (<ExceptionName>)` | 2 |
| all sends failed | stderr | per [_send-results.md](_send-results.md) | 4/5/1 |

## --help transcript

Command: `signal-cli --config <CONFIG> sendStory --help` -- exit code: 0, stderr empty. Stdout:

```text
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli sendStory [-h] -a ATTACHMENT [--no-replies] [-g GROUP_ID]

named arguments:
  -h, --help             show this help message and exit
  -a ATTACHMENT, --attachment ATTACHMENT
                         Specify the file path  to  the  image  or video to
                         post as a story.
  --no-replies           Disable replies on this story.
  -g GROUP_ID, --group-id GROUP_ID
                         Specify a group  to  post  the  story  to. Without
                         this, posts to My Story.
```

## Doc vs binary discrepancies

- Man page says "Post a file attachment story to your Story, visible to all
  contacts."; binary help line is `Post a story to your Story.`
