# Rate-limit challenges: 428 "proof required" and `submitRateLimitChallenge` (signal-cli 0.14.7, staging)

Spec role: prose and tables only; no code. This file covers how a
challenge token reaches the user (the 428 response to a message send and
what signal-cli reports for it), and the request that
`submitRateLimitChallenge --challenge TOKEN --captcha CAPTCHA` sends,
with every outcome mapped to signal-cli's messages.

Not repeated here:

- the send loop, sealed/unsealed choice and the other send statuses:
  `send.md` §1–§6;
- the per-recipient result lines and JSON of send-type commands:
  `spec/surface/cli/_send-results.md`;
- the command surfaces: `spec/surface/cli/submitRateLimitChallenge.md`,
  `spec/surface/jsonrpc/methods/submitRateLimitChallenge.md`;
- transports and credentials: `api.md` §2, §4, `transport.md`.

## Sources

| Ref | What | URL @ version |
|-----|------|---------------|
| L1 | signal-cli | https://github.com/AsamK/signal-cli @ v0.14.7 (b01b6b370dc063599a1a2b9fde0f5ff4e2d78fe8): `lib/src/main/java/org/asamk/signal/manager/internal/{ManagerImpl,SignalDependencies}.java` (`submitRateLimitRecaptchaChallenge`, which API object is used), `.../manager/util/Utils.java`, `.../manager/api/{ProofRequiredException,SendMessageResult,CaptchaRejectedException}.java`; `src/main/java/org/asamk/signal/commands/SubmitRateLimitChallengeCommand.java`, `src/main/java/org/asamk/signal/util/SendMessageResultUtils.java`, `src/main/java/org/asamk/signal/json/JsonSendMessageResult.java`; `man/signal-cli.1.adoc` section "submitRateLimitChallenge" |
| L2 | signal-service-java (Turasa fork) | https://github.com/Turasa/libsignal-service-java @ v2.15.3_unofficial_151 (66956cb4a0394d50cee3590209baf58312084a44): `lib/network/src/main/java/org/signal/network/api/RateLimitChallengeApi.kt`, `lib/libsignal-service/src/main/java/org/whispersystems/signalservice/api/NetworkResultUtil.kt`, `.../api/push/exceptions/{ProofRequiredException,RateLimitException,AuthorizationFailedException}.java`, `.../internal/push/{ProofRequiredResponse,PushServiceSocket}.java`, `.../internal/push/exceptions/CaptchaRejectedException.kt`, `.../api/SignalServiceMessageSender.java` (send-error to result mapping); `core/network/src/main/java/org/signal/network/{NetworkResult.kt,websocket/WebSocketRequestExt.kt}` |
| L3 | Signal-Server | https://github.com/signalapp/Signal-Server @ v20261022.0.0 (8edf3488f7d97398db7ffa5184c6b769a272ff59): `service/src/main/java/org/whispersystems/textsecuregcm/controllers/{ChallengeController,MessageController}.java`, `.../entities/{AnswerChallengeRequest,AnswerCaptchaChallengeRequest,AnswerPushChallengeRequest,RateLimitChallenge}.java`, `.../limits/RateLimitChallengeManager.java`, `.../captcha/CaptchaChecker.java` (server behaviour notes only) |
| L4 | Reference distribution, offline | `jshell` over `/work/third_party/signal-cli-0.14.7/lib/*.jar`, no network: (a) the bundled serializer applied to the captcha-answer payload class (JSON in §2.2); (b) the bundled WebSocket converter plus "basic" status mapping applied to synthetic responses (exception classes and messages in §2.4); (c) a byte search of all bundled jars for code that constructs the library's captcha-rejected exception (only its own class file references it) |

## 1. Where the challenge token comes from: 428 on a send

### 1.1 The response

A message send (`PUT /v1/messages/{destination}`, `send.md` §2) can be
answered with **428 Precondition Required** when the server's spam logic
asks for proof of humanity. In the public server code (L3
`MessageController`) the 428 is produced by a pluggable spam checker; the
open-source default never produces it, so it is seen only where the
deployment (production, possibly staging) runs a real one.

Body (L3 `RateLimitChallenge`), JSON object:

| Member | Type | Meaning |
|---|---|---|
| `token` | string | opaque challenge token; the user passes it as `--challenge` |
| `options` | array of strings | the challenge kinds the client may answer: `"captcha"` and/or `"pushChallenge"` |

Header: `Retry-After` (integer seconds), optional. On the WebSocket the
header name arrives lowercased (`retry-after`).

### 1.2 How signal-cli turns it into a send result

- Over the WebSocket and the REST fallback alike, 428 becomes a
  "proof required" failure for that recipient (`send.md` §5 table, §6).
  Like 401/404/409/410/413/429/508, it is a non-technical outcome: it is
  **not** retried over REST and the send loop does not retry it.
- `options` is parsed into a set. `"captcha"` → CAPTCHA,
  `"pushChallenge"` → PUSH_CHALLENGE. Any other string is dropped with a
  log warning.
- Retry-after: the header value in seconds, or **−1** when the header is
  missing or not numeric. signal-cli multiplies by 1000 to get
  milliseconds, so a missing header becomes −1000 ms.
- signal-cli never answers a push challenge (it has no push token) and
  never requests one (`POST /v1/challenge/push` is not used).

### 1.3 What the user sees

The per-recipient line and the JSON result are specified in
`spec/surface/cli/_send-results.md` ("proof required" row, the four extra
CAPTCHA lines, JSON `type` `RATE_LIMIT_FAILURE`, `token`,
`retryAfterSeconds`). Facts that file needs from the wire:

- `<token>` is the `token` member verbatim.
- `<options>` are the parsed option names (`CAPTCHA`, `PUSH_CHALLENGE`)
  joined with `, `, in hash-set order.
- `<seconds>` is the retry-after in milliseconds divided by 1000,
  rounded **up**. When the server sent no `Retry-After`, this is
  **−1**: the plain-text line reads `… or wait "-1" seconds.`, and the
  JSON result carries `"retryAfterSeconds":-1`. For a proof-required
  failure, `retryAfterSeconds` is therefore **always present**. This
  refines the "only when a retry-after is known" wording in
  `_send-results.md`.
- A command whose results are all rate-limit/proof-required failures
  ends with `Failed to send message due to rate limiting`, exit 5
  (`_send-results.md`, "Exit status after output").

### 1.4 Malformed 428 bodies

If the body lacks `options`, or is not JSON, parsing fails inside the
library. The failure surfaces as an unexpected error for that send, not
as a proof-required result. Not observed; a Go client may instead treat
a missing `options` as an empty set.

## 2. `submitRateLimitChallenge --challenge TOKEN --captcha CAPTCHA`

### 2.1 Preparation (local)

- Normal account check first (`api.md` §4.3); the command needs a
  registered account. It works on primary and linked devices alike.
- The captcha has every occurrence of the literal `signalcaptcha://`
  removed. A missing captcha (possible only through JSON-RPC) becomes
  the empty string.
- The challenge is used verbatim. A missing challenge (JSON-RPC only)
  fails locally with an unexpected null-argument error (JSON-RPC −32603)
  before any request.

### 2.2 The request: `PUT /v1/challenge`

- **Transport:** the **authenticated WebSocket** (`transport.md` §3),
  credentials `<ACI>.<deviceId>:<password>`. Request header
  `content-type:application/json`. There is no REST fallback.
- **Body:** compact JSON, exactly these three members in this order
  (L4 a):

      {"type":"captcha","token":"<TOKEN>","captcha":"<CAPTCHA without signalcaptcha://>"}

| Member | Value |
|---|---|
| `type` | always `"captcha"`. The server dispatches on it; `"rateLimitPushChallenge"` with a `challenge` member is the push variant, which signal-cli never sends. |
| `token` | `--challenge` value |
| `captcha` | `--captcha` value with the prefix removed. Its expected shape is `<provider>.<sitekey>.<action>.<token>`, for example `signal-hcaptcha.<sitekey>.challenge.<…>`. |

### 2.3 Server behaviour (L3, informative)

- The challenge `token` must be non-blank. The server does not otherwise
  check it against the 428 it issued; only the captcha is assessed.
- The captcha is split on `.` into four parts. Fewer parts, an unknown
  provider, an unknown action, an action other than `challenge`, or a
  site key not valid for that action → **400** (with a reason phrase such
  as `invalid captcha action`).
- Too many attempts → **429**. Push challenges not permitted → 429
  (push variant only).
- The captcha was assessed and rejected (score below threshold, expired,
  reused, solved from another IP) → **428** with no body.
- An assessment back-end failure → 503.
- Accepted → **200**, no body. The rate limit is lifted for the account.
- An unauthenticated caller → 401.

### 2.4 Outcomes in signal-cli 0.14.7

The response goes through the generic WebSocket converter and the
"basic" status mapping (`api.md` §4.2). Verified offline (L4 b):

| Server answer | Library exception and message | Printed (stderr) | Exit / JSON-RPC |
|---|---|---|---|
| 200 | — | nothing; JSON-RPC result `{}` | 0 |
| 428 (captcha rejected) | `NonSuccessfulResponseCodeException`, `[428] ` (with a trailing space) | `Submit challenge error: [428] ` (with a trailing space) | **3** / −3 |
| 400 (malformed captcha, wrong action) | same class, `[400] ` | `Submit challenge error: [400] ` | 3 / −3 |
| 401, 403 | `AuthorizationFailedException`, `[401] Authorization failed!` | `Submit challenge error: [401] Authorization failed!` | 3 / −3 |
| 413, 429 | `RateLimitException`, `[429] Rate Limited` | `Submit challenge error: [429] Rate Limited` | 3 / −3 |
| other non-2xx (e.g. 503) | `NonSuccessfulResponseCodeException`, `[<code>] ` | `Submit challenge error: [<code>] ` | 3 / −3 |
| socket or network failure | the I/O exception | `Submit challenge error: <message>` | 3 / −3 |

Unlike the change-number messages, this message carries no
`(<ExceptionSimpleName>)` suffix.

**Exit 6 is unreachable in 0.14.7.** The command maps a library
"captcha rejected" exception to
`Captcha rejected, it may be outdated, already used or solved from a different IP address.`
(exit 6, JSON-RPC −6). No code in the bundled libraries ever constructs
that exception (L4 c): the WebSocket path turns a 428 into the generic
status error shown above. The exit-6 rows in
`spec/surface/cli/submitRateLimitChallenge.md`,
`spec/surface/jsonrpc/methods/submitRateLimitChallenge.md` and
`spec/surface/jsonrpc/errors.md` describe intent, not observable
behaviour against a real server.

A byte-compatible Go client prints `Submit challenge error: [428] ` and
exits 3 on a rejected captcha. Printing the exit-6 message instead would
be a deliberate, documented divergence.

### 2.5 After success

Nothing is stored locally and nothing is retried automatically. The user
re-runs the failed send. The challenge token is single-purpose; a new
428 brings a new token.

## 3. Captcha pages

- Production challenge captchas: https://signalcaptchas.org/challenge/generate.html
  (quoted from the man page and the send-result hint text, L1).
- For staging, the man page says to use
  https://signalcaptchas.org/staging/registration/generate.html. The
  server requires the captcha's action part to be `challenge` (§2.3). If
  the staging registration page yields a captcha with a `registration`
  action, staging answers 400 → `Submit challenge error: [400] `. Not
  verified (no page was fetched and no live request was made).

## Gaps

1. Whether staging's spam checker issues 428 on sends at all, and with
   which `options` and `Retry-After`, was not observed.
2. The staging captcha page and action (§3) were not verified.
3. The exact behaviour on a malformed 428 body (§1.4) was inferred from
   the parser, not exercised.
