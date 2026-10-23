# Shared: send-result output and exit status (send-family commands)

## Sources

- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/util/SendMessageResultUtils.java (tag v0.14.7)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/json/JsonSendMessageResult.java
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/json/JsonRecipientAddress.java
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/util/Util.java (JSON serializer settings)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/output/PlainTextWriterImpl.java, .../output/JsonWriterImpl.java
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/Main.java (exit-code mapping)
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/lib/src/main/java/org/asamk/signal/manager/api/SendMessageResults.java
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/lib/src/main/java/org/asamk/signal/manager/api/SendMessageResult.java
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/lib/src/main/java/org/asamk/signal/manager/api/RecipientAddress.java
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/lib/src/main/java/org/asamk/signal/manager/api/RecipientIdentifier.java
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/lib/src/main/java/org/asamk/signal/manager/api/ProofRequiredException.java, .../RateLimitException.java
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/lib/src/main/java/org/asamk/signal/manager/api/GroupNotFoundException.java, NotAGroupMemberException.java, GroupSendingNotAllowedException.java, GroupIdFormatException.java, GroupId.java
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/lib/src/main/java/org/asamk/signal/manager/util/PhoneNumberFormatter.java
- https://raw.githubusercontent.com/AsamK/signal-cli/v0.14.7/src/main/java/org/asamk/signal/util/CommandUtil.java
- man page `man/signal-cli.1.adoc` @ v0.14.7, section "Exit codes".
- Binary observation: none of the result output can be observed without a registered account (no account is available in this sandbox). Everything in this file comes from source reading and is UNVERIFIED against the binary.

## Which commands use this

send, sendAdminDelete, sendPollCreate, sendPollVote, sendPollTerminate,
sendPaymentNotification, sendPinMessage, sendUnpinMessage, sendReaction,
sendReceipt, sendStory, sendTyping, remoteDelete.

NOT used by: `sendMessageRequestResponse` (prints nothing) and `send -e/--end-session`
(prints nothing).

A send operation yields one timestamp (milliseconds since the Unix epoch, the
timestamp of the sent message) and a list of per-recipient results. Results are
grouped internally by the "recipient key" the user named (a phone number, ACI,
PNI, username, group, or note-to-self); one key may produce several results
(e.g. a group expands into one result per member; note-to-self/self produce
results for own devices).

## Plain-text output (stdout)

1. If at least one result is a failure that has a message (see list below),
   print the line `Failed to send (some) messages:` followed by one line per
   such failure, each formatted as `<recipient key>: <failure message>`.
2. Then print the timestamp as a bare decimal integer on its own line.

Lines end with the platform line separator (`\n` on Linux). Both parts go to
stdout. Order of failure lines across recipient keys is unspecified (hash
order); within one key, the order of the underlying results.

`<recipient key>` is the identifier as the user specified it after
normalisation:
- phone number: E.164 normalised form (e.g. `+15555550123`);
- ACI: the lowercase UUID string;
- PNI: `PNI:<uuid>`;
- username: `u:<username>`;
- group: the group id in standard base64 (with padding);
- note-to-self: the literal `Note-To-Self`.

Failure messages (checked in this priority order; the first that applies is used).
`<id>` is the result's address "legacy identifier": the phone number if known,
otherwise the ACI, otherwise the PNI, otherwise the username.

| Failure kind | Message |
|---|---|
| proof required (server demands a challenge) | `CAPTCHA proof required for sending to "<id>", available options "<options>" with challenge token "<token>", or wait "<seconds>" seconds.` followed by a newline; if the options include CAPTCHA, followed by the four lines shown below |
| network failure | `Network failure for "<id>"` |
| rate limit with known retry-after | `Rate limit failure for "<id>", retry after <seconds> seconds` |
| rate limit without retry-after | `Rate limit failure for "<id>"` |
| unregistered recipient | `Unregistered user "<id>"` |
| untrusted identity | `Untrusted Identity for "<id>"` |
| invalid pre-key failure | no line is printed (the result is still a failure) |

The four extra lines for a CAPTCHA proof-required failure:

```text
To get the captcha token, go to https://signalcaptchas.org/challenge/generate.html
After solving the captcha, right-click on the "Open Signal" link and copy the link.
Use the following command to submit the captcha token:
signal-cli submitRateLimitChallenge --challenge CHALLENGE_TOKEN --captcha CAPTCHA_TOKEN
```

Notes on the proof-required message:
- `<options>` is the set of option names joined with `, ` (names are `CAPTCHA`
  and `PUSH_CHALLENGE`; order unspecified).
- `<seconds>` = retry-after milliseconds divided by 1000, rounded up.
- When CAPTCHA is not among the options, the message still ends with a newline
  character, so the printed output contains an empty line after it.
- Retry-after seconds for plain rate limits are also rounded up from milliseconds.

## JSON output (stdout, `-o json`)

Exactly one line containing one JSON object, followed by a newline:

- `timestamp`: integer (int64), the message timestamp in ms.
- `results`: array of result objects, one per underlying result (flattened over
  all recipient keys; order unspecified across keys).

Key order of the top-level object (`timestamp` vs `results`) is NOT stable in the
reference implementation (it is built from an unordered immutable map whose
iteration order is randomised per JVM start). Consumers must not depend on it;
the Go implementation may emit `timestamp` first.

Each result object, keys in this order:

| Key | Type | Presence | Meaning |
|---|---|---|---|
| `recipientAddress` | object | always | `{"uuid": string-or-null, "number": string-or-null, "username": string-or-null}` (keys in that order; null values are emitted as `null`, not omitted). `uuid` is the ACI if known. |
| `groupId` | string | only when the recipient key was a group | base64 group id |
| `type` | string | always | one of `SUCCESS`, `NETWORK_FAILURE`, `UNREGISTERED_FAILURE`, `IDENTITY_FAILURE`, `RATE_LIMIT_FAILURE`, `INVALID_PRE_KEY_FAILURE` |
| `token` | string | only for proof-required failures | challenge token |
| `retryAfterSeconds` | integer | only when a retry-after is known (rate limit or proof required) | ceil(ms/1000) |

`type` is chosen by priority: success, then rate-limit (including
proof-required), network, unregistered, invalid-pre-key, otherwise
identity failure.

Compact JSON (no spaces, no pretty printing).

## Exit status after output

The output above is always written first (stdout). Then:

- If the result list is empty, or at least one result is SUCCESS: exit 0.
- Otherwise (no successes, at least one result):
  - all results are untrusted-identity failures: stderr
    `Failed to send message due to untrusted identities`, exit **4**;
  - all results are rate-limit/proof-required failures: stderr
    `Failed to send message due to rate limiting`, exit **5**;
  - anything else: stderr `Failed to send message`, exit **1**.

The stderr message is a single line (printed by the generic error handler).
With `-v`, the cause's stack trace may follow (only rate-limit has a cause).

## Common recipient-parsing errors (all exit 1, stderr)

These are raised by the shared recipient-resolution logic before anything is sent:

| Condition | Message |
|---|---|
| no recipient of any kind given | `No recipients given` |
| group id not valid base64 or not 16/32 bytes after decoding | `Invalid group id: Failed to decode groupId (must be base64) "<id>": <cause message>` |
| recipient string `PNI:` followed by a non-UUID | `Invalid phone number '<s>': Invalid PNI` |
| recipient string containing `@` | `Invalid phone number '<s>': Possible attempt to use email address.` |
| recipient string without any digit or `+` | `Invalid phone number '<s>': No valid characters found.` |

Recipient strings are interpreted as: a UUID -> ACI; `PNI:<uuid>` -> PNI;
`u:<name>` -> username; anything else -> phone number, stripped of all
characters other than digits and `+`, then normalised to E.164 using the
account's own number to infer the country when no `+` prefix is present (with a
lenient fallback that prefixes `+` and, for short numbers, the leading digits of
the own number).

Common group errors raised during sending (exit 1, message only):
- `Group not found: <base64 id>`
- `User is not a member in group: <group name> (<base64 id>)`
- `User is not allowed to send message to group: <group name> (<base64 id>)`

Common recipient error: `The user <identifier> is not registered.` (exit 1)

Common I/O errors (exit 2, "unexpected"): `Failed to send message: <message> (<exception simple name>)`,
and when the message contains `No prekeys available`, the suffix
`, maybe one of the devices of the recipient wasn't online for a while.` is appended.
The exception simple name is a Java class name (e.g. `IOException`); a Go
reimplementation should reproduce the observable string where feasible.
