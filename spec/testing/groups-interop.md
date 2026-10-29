# Test plan: live group interop with a third account

Status: plan (2026-10-29). Implementation role document; it cites our
specs and adds no upstream facts.

## 1. Goal

Close the Layer C gaps that need a third account (COMPAT.md: `send` and
`sendStory` Layer C, "Sealed sender", "Stories", "Groups v2", "Session
recovery after reinstall"):

- sender-key group messages (`spec/service/groups.md` §12.2-12.3),
- group stories (groups.md §13, `spec/service/stories.md` §3),
- sealed sender v2 through `PUT /v1/messages/multi_recipient` with a
  combined group-send token (stories.md §3.4),
- sender-key redistribution on join, leave, removal and device add, and
- retry receipts for group messages (`spec/service/send-log.md` §4,
  stories.md §3.7).

Every scenario runs in both directions (signal-cli-go sends and the
reference receives, and the reverse). Every scenario proves which send
path ran, and it fails if the message fell back to 1:1 delivery.

Everything runs against staging only, with `--service-environment staging`.

## 2. Accounts and devices

A third account **C** is registered with the reference signal-cli 0.14.7
(operator: number, captcha, SMS code). Each account then gets one
reference device and one signal-cli-go device. Each message therefore
reaches every receiving account on both implementations, and both copies
are rendered and compared.

| Account | Reference device | signal-cli-go device |
|---------|------------------|----------------------|
| A | A.1 primary, `.data/capture-a` (exists) | A.2 linked, `.data/cli-a` (exists) |
| B | B.2 linked (new; linked by our B.1 with `addDevice`), `.data/interop/ref-b` | B.1 primary, `.data/cli-b` (exists) |
| C | C.1 primary (new registration), `.data/capture-c` | C.2 linked (new; linked by C.1 with `addDevice`), `.data/interop/cli-c` |

Senders per direction:

| Direction | Sender | Receiving accounts (both devices each) | Sync transcript to |
|-----------|--------|----------------------------------------|--------------------|
| ours → ref | B.1 (signal-cli-go primary) | A (A.1, A.2), C (C.1, C.2) | B.2 (reference) |
| ref → ours | C.1 (reference primary) | A (A.1, A.2), B (B.1, B.2) | C.2 (signal-cli-go) |
| ours → ref, linked sender (S1 only) | A.2 (signal-cli-go linked) | B, C | A.1 (reference) |

Byte-identical comparison works across the two devices of one receiving
account. A sync transcript reaches only the sender's other device, which
is a single implementation, so transcripts are checked field by field
(destination-less group transcript, recipient list, `isRecipientUpdate`)
rather than byte-compared (§5.3).

### 2.1 Setup (phase 0, once)

1. Register C.1 with the reference: `register --captcha <token>`, then
   `verify <code>`, then `updateProfile --given-name RefC`. Wait 60 s.
2. Link C.2 (ours) to C.1: our `link`, then the reference's `addDevice --uri`.
3. Link B.2 (reference) to B.1: the reference's `link`, then our `addDevice --uri`.
4. Mutual profile sharing and unidentified access. Every account sends a
   1:1 text to every other account. Every device receives. Every account
   accepts the message requests (`sendMessageRequestResponse --type accept`).
   Then every pair has a profile key and an access key, which sender-key
   targeting needs (groups.md §12.2 step 4).
5. Trust: `listIdentities` on every device must show the other accounts
   as trusted (sender-key targets need a trusted identity).
6. Groups, all created by the account named:
   - **G-main** (B.1 creates it; members A and C): S1-S3, S6.
   - **G-ref** (C.1 creates it; members A and B): S1-S3, S6 in the ref →
     ours direction, so that the reference sender is an admin of its own
     group.
   - **G-churn** (one per direction, created by that direction's
     sender; members A, then C/B join later; invite link enabled with
     `--link enabled`): S4.
7. Preconditions check: one verbose dry-run send in G-main from B.1 must
   log `Group send path: multi-recipient endpoint with group-send token,
   ..., 2 recipient(s)`. If it falls back, setup is incomplete and the run
   stops.

## 3. Send-path assertion

### 3.1 signal-cli-go as the sender

`signal-cli-go -v` writes one DEBUG line per decision to stderr, under the
`SendHelper` logger (they appear with `-v` only):

| Line (prefix `Group send path: `) | Meaning |
|-----------------------------------|---------|
| `sender key distribution to N recipient(s), distribution D, new key B` | SKDMs sent 1:1 before the multi-recipient call; `new key true` means our sender key was created for this send |
| `multi-recipient endpoint with group-send token, sender key, sealed sender v2, N recipient(s), M device(s), attempt K` | the group-message path (groups.md §12.3) |
| `multi-recipient endpoint with story, sender key, sealed sender v2, ...` | the group-story path (groups.md §13) |
| `multi-recipient endpoint accepted (U unregistered)` | 200 |
| `multi-recipient endpoint answered 409 (device mismatch), retrying` / `answered 410 ...` | device mismatch, handled (stories.md §3.5) |
| `multi-recipient endpoint answered S` | any other status, including 401/413/429/5xx |
| `1:1 fallback for N of M recipient(s): REASON` | **fallback**: no endorsements, fewer than 2 qualifying members, or a failed sender-key send |
| `story not sent to N of M recipient(s): REASON` | a group story that went nowhere (stories have no fallback) |

**Pass rule** for every scenario except the labelled controls: the send's
stderr contains the expected `multi-recipient endpoint with ...` line and
an `accepted` line, and contains no `1:1 fallback` and no `story not sent`
line. A missing or unexpected line fails the scenario.

### 3.2 The reference as the sender

The reference cannot be instrumented, so the path is read on the
receivers. Our receiving devices log one line per decrypted envelope
under the `ReceiveHelper` logger:

    Envelope path: multi-recipient (sealed sender v2, sender key message, group id true) from <sender>.<device>, timestamp <ms>
    Envelope path: 1:1 (sealed sender v1, whisper message, group id true) from <sender>.<device>, timestamp <ms>

Sealed sender v2 is produced only by the multi-recipient endpoint, so
sealed sender v2 carrying a sender-key message identifies that path; any
other combination was a 1:1 delivery.

**Pass rule**: on every signal-cli-go receiver (A.2, B.1 or C.2), the
envelope carrying the scenario's message timestamp has a
`multi-recipient` line from the reference sender's device. A `1:1` line
for that timestamp fails the scenario. SKDM deliveries are 1:1 by design.
They carry a different timestamp and are counted separately, since their
count is the redistribution assertion (§4.4).

Token vs combined access key: receivers cannot tell them apart. The
reference uses libsignal 0.99.1, which never sends `Unidentified-Access-Key`
on this request (stories.md §3.4). A non-story multi-recipient send that
the server accepts therefore carried a group-send token. The plan relies
on that spec fact and says so in its results.

### 3.3 Both senders

Both implementations' senders get the receiver check of §3.2 as well, so
a signal-cli-go send has two independent proofs (its own log and the
receivers' envelope paths).

## 4. Scenarios

Each scenario lists its steps for one direction. It runs once as ours →
ref (sender B.1, group created by B) and once as ref → ours (sender C.1,
group created by C), and S1 runs once more from the linked A.2. "Receive
all" means `receive -t 10` on every device of the receiving accounts and
on the sender's other device, one at a time, with our devices at `-v`.

### S1 Group message

1. Sender: `send -g G -m <text> -a <png>` with a quote of the previous
   group message on the second run (attachments and quotes exercise
   padding and size).
2. Receive all.
3. Assert: path (§3); rendering byte-identical within each receiving
   account, in plain text and JSON (§5); transcript fields at the
   sender's other device.

### S2 Group story

1. Sender: `sendStory -g G -a <png>`.
2. Receive all.
3. Assert: `multi-recipient endpoint with story`; receivers show
   `sealed sender v2, sender key message`, and the story context carries
   the master key as group id (stories.md §2.4.6); identical rendering;
   transcript at the sender's other device lists the group's
   distribution.

### S3 Sealed sender v2 with endorsements

1. Endorsement refresh: on our sender, set the stored endorsement
   expiration to now + 1 h (store edit by the harness, signal-cli-go data
   only). Send to G. Assert a forced group fetch, then the multi-recipient
   send with the token (groups.md §12.2 step 3).
2. Token coverage: send again. Assert `2 recipient(s)` and acceptance:
   the server checks the combined token against exactly the target set,
   so a wrong combination answers 401 (stories.md §3.4).
3. Reference direction: C.1 sends to G-ref, and receivers assert
   multi-recipient envelopes (§3.2).
4. Control (expected fallback, labelled): the harness removes one
   member's stored endorsement from our sender's store, which leaves one
   qualifying member. The send must log `1:1 fallback for 2 of 2
   recipient(s): only 1 member(s) qualify`. This proves the assertion can
   fail.

### S4 Redistribution: join, leave, removal, device add

Group G-churn starts with the sender and A only.

- **Control** (labelled): sender sends; expect `1:1 fallback ... only 1
  member(s) qualify` (one other member).
- **Join**: the third account (C for ours → ref, B for ref → ours) joins
  by invite link (`joinGroup --uri`). The sender sends.
  - Assert: `sender key distribution to 1 recipient(s), ..., new key false`
    (only the joiner lacks our key; for a reference sender: exactly one
    SKDM envelope on the joiner's devices, none on A's).
  - Assert: the multi-recipient send to 2 recipients is accepted, and
    the rendering is identical on both devices of A and of the joiner.
- **Leave**: the joiner runs `quitGroup`. Control send (fallback, one
  other member); the joiner's devices must receive nothing for it. The
  joiner rejoins by link. The sender sends.
  - Assert: no SKDM to the rejoiner. Leaving resets nothing, and there
    is no rotation on leave (stories.md §3.7), so the rejoiner decrypts
    with the key it kept.
  - Assert: the multi-recipient send to 2 recipients is accepted.
  - A different outcome on either implementation is a finding, not
    noise.
- **Removal**: the sender removes the joiner (`updateGroup -r`). Control
  send; nothing may reach the removed account. The sender re-adds it
  (`updateGroup -m`), then sends. Same assertions as for leave.
- **Device add**: the receiving account links a new device: a scratch
  signal-cli-go device (`.data/interop/dev-add-<n>`), linked by that
  account's primary. The sender sends.
  - Assert, signal-cli-go sender: `answered 409 (device mismatch),
    retrying`, then `sender key distribution to 1 recipient(s)` (the new
    device only), then `attempt 2` accepted.
  - Assert, reference sender: the new device gets one SKDM and then a
    multi-recipient envelope for the message.
  - Assert: the rendering on the new device equals that account's other
    devices.
  - Then remove the scratch device (`removeDevice`).

### S5 Retry receipts

Two faults, one per direction, made only by editing signal-cli-go's own
store (harness helpers in `tools/interop`, never the reference's
database):

- **ref → ours**: delete, on our receiver B.1, the sender-key record it
  holds for C.1's distribution in G-ref. C.1 sends.
  - Assert, B.1: `Envelope path: multi-recipient` for the message, a
    decryption failure, and a retry receipt sent (DecryptionErrorMessage
    without a ratchet key, `pipe.md` §7.2).
  - Assert: C.1 resends 1:1 with a fresh SKDM attached (stories.md §3.7,
    last paragraph). B.1 renders the resent message identically to B.2,
    which got the original.
- **ours → ref**: on our sender B.1, in a fresh group G-retry (new
  distribution), pre-mark C's devices as already holding our sender key
  before the first send, so the SKDM to C is skipped and C.1 cannot
  decrypt. B.1 sends.
  - Assert: the multi-recipient path; C.1 reports the failure and sends a
    retry receipt.
  - Assert, B.1 on its next receive: it handles the receipt, resends 1:1
    with a fresh SKDM, and clears the sharing row (send-log.md §4).
  - Assert: C.1 and C.2 render the resent message identically.

The resends are 1:1 by design. The pass rule here is that the original
send took the multi-recipient path and the resend is a sealed 1:1
message carrying an SKDM. A fallback on the original send still fails the
scenario.

### S6 Typing over sender key

Optional, cheap. `sendTyping -g G` uses the sender-key path with
online=true (groups.md §12.3). Assert the path and that both devices of
each receiver render the typing message identically.

## 5. Rendering comparison

1. A message can be received only once per device, so S1 and S2 each
   run twice: received with `-o plain-text` the first time and `-o json`
   the second. Other scenarios compare plain text only.
2. Within an account, the reference device's output and ours must be
   byte-identical after these normalizations, and only these:
   - the per-device server delivery timestamp (already known to differ
     between devices);
   - the receiving device id where printed;
   - the data-directory prefix of stored attachment paths.
3. Transcripts: field-level checks (§2).

Any other difference fails the scenario. The diff is kept in the capture
(§6).

## 6. Captures (opt-in replay)

Each scenario run writes, under gitignored `.data/interop/<run id>/<scenario>-<direction>/`:

| Path | Content |
|------|---------|
| `manifest.json` | scenario, direction, device roles, message timestamps, expected path, pass/fail per assertion. Numbers appear only as `<A>`/`<B>`/`<C>` |
| `pre/<device>/` | data-directory snapshot of every receiving device before it received |
| `envelopes/<device>/` | raw envelopes as received by our devices (`SIGNAL_CLI_GO_CAPTURE_DIR`, an opt-in debug variable to add: writes each envelope and its server timestamps before processing) |
| `out/<device>.txt`, `out/<device>.json` | receive output of every device |
| `logs/<device>.log` | stderr at `-v` of our devices (send and receive) |
| `diff/` | rendering diffs, if any |

Replay: `INTEROP_CAPTURES=<dir> go test ./internal/interop -run TestReplay`
(opt-in, never in CI). It restores each `pre/` snapshot of our devices
into a temporary directory and decrypts the captured envelopes offline.
It then checks that the rendering still equals the recorded reference
output (normalized as in §5), so a later code change that breaks
interop shows up without touching staging. Captures hold live key
material: they stay in `.data/`, are never committed, and are deleted
when the accounts are retired.

## 7. Pacing and rate limits

- One process at a time; no parallel staging traffic.
- At least 5 s between sends and 60 s between scenarios. Registration
  and linking get 60 s pauses after each step.
- Budget: at most 40 sends and 6 link/unlink operations per hour.
  Phase 0 plus one direction of S1-S6 fits in about two hours, so the
  directions run in separate sessions.
- On **429** (rate limited) or **413** from any request on either
  implementation:
  - Our client logs `multi-recipient endpoint answered 429` (or the
    command fails with a `[429]`/`[413]` error); the reference prints its
    rate-limit error.
  - Stop the scenario and wait for `Retry-After` when reported, else
    60 s, then 120 s, 240 s and 480 s.
  - After 4 attempts, mark the scenario `rate-limited` (not failed) and
    end the run.
- On a proof-required/captcha challenge (`submitRateLimitChallenge`
  territory, `spec/service/rate-limit-challenge.md`): stop the run and
  ask the operator. Don't retry.
- A scenario never repeats a send to "get it green". Each repeat counts
  against the budget and is recorded in the manifest.

## 8. Work before the first run

1. Done (this change): the `Group send path:` and `Envelope path:` debug
   lines (the latter with the envelope's client timestamp) and their unit
   tests.
2. `SIGNAL_CLI_GO_CAPTURE_DIR` envelope capture in our receiver (§6).
3. `tools/interop`: the driver (phases, pacing, backoff, masking,
   captures, comparison) and the store-edit helpers for S3 and S5. It is
   not a shipped command, so it goes under `tools/`, not `cmd/`.
4. `internal/interop` replay test (§6).
5. COMPAT.md rows updated from the results.

## 9. Needs from the operator

- A third staging phone number for C: provided 2026-10-29 in
  `/tmp/staging-number-c` (memory-only, read at runtime, never written to
  the repo); later launches also get `SIGNAL_STAGING_NUMBER_C`. Masked as
  `<C>` like A and B.
- A fresh staging captcha token, sent when phase 0 starts (tokens
  expire within minutes).
- The SMS verification code for C during `verify`.

## 10. Results of the first run (2026-10-29)

Run directory `.data/interop/run-20261029` (gitignored); `results.txt`
there lists every assertion. Offline replay of the captures
(`TestInteropReplay`): 97 device captures reproduce the reference
device's rendering (or our live one where no reference device rendered the
message); skips state their reason (a group fetch the replay cannot do, a
deliberately injected fault, a known state difference).

| Scenario | ours → ref | ref → ours |
|----------|------------|------------|
| S1 group message (plain, JSON; plus linked sender A.2) | sender-key path, identical rendering | sender-key path, identical rendering |
| S2 group story (plain, JSON) | story path, identical rendering | story path, identical rendering |
| S3 endorsements (refresh, token coverage, control) | pass (control falls back as designed) | covered by S1: the reference's endorsement handling (see findings) |
| S4 join / leave / removal | pass; no SKDM after rejoin or re-add | pass; same behaviour |
| S4 device add | pass: 409, SKDM to the new device, attempt 2 accepted | pass: the new device gets the SKDM and the message |
| S5 retry receipts | pass: receipts from both of C's devices, one 1:1 resend with a fresh SKDM, identical rendering | receipt delivered; the reference never resends (finding 5) |
| S6 typing | not run (optional, and `sendTyping` prints no timestamp to match) | not run |

Procedure changes made during the run:

- Phase 0: both clients invited members by PNI (they lacked profile-key
  credentials), and the reference cannot accept such an invitation; C was
  re-added by ACI.
- A sender refreshes its endorsements before the redistribution sends by
  committing a change (description), since both implementations keep
  endorsements from the last fetch or commit (finding 2).
- S5 ref → ours sends a priming message first: an earlier session renewal
  makes the reference hand out its key again, which would hide the fault.
- The SKDM count on the receiving account's other device is 1, not 0: an
  SKDM is a 1:1 message to the account, so all its devices get it (both
  implementations).

Bugs found in signal-cli-go and fixed (each with a unit test):

1. Group-update messages skipped invitees known only by their PNI; now
   sent to them (excluding our own PNI).
2. Accepting an invitation sent to our PNI failed locally ("invalid group
   content"); the change is now applied from the server's signed copy.
3. No retry receipt was sent for a sender-key decryption failure (it has
   no ratchet key by design).
4. Retry receipts went to the failing device only and failed with 409 on
   multi-device senders; they are now a normal 1:1 send to all devices.
5. Retry receipts lacked the sealed-sender group ID that the reference
   includes.
6. A sealed sender-key failure was rendered without the sealed-sender line
   and with the wrong exception; it now matches the reference
   (`NoSessionException: missing sender key state for distribution ID …`,
   `ProtocolNoSessionException`).

Findings about the reference (signal-cli 0.14.7):

1. It cannot accept a group invitation sent to its PNI (`Cannot find
   service ID for self to accept invite`), from a primary device.
2. Endorsements: after a membership change it learns from a message (an
   accepted invitation, a join), the sender keeps its old endorsements
   until they are within 2 hours of expiry, so new members do not qualify
   and sends fall back to 1:1 (groups.md §12.2 step 3). signal-cli-go does
   the same.
3. No sender-key rotation or sharing reset on leave or removal: a
   rejoining member decrypts with the key it kept. signal-cli-go does the
   same.
4. A missing number-to-ACI link on one device shows the ACI instead of the
   number in `Envelope from:`; seen on both implementations, a matter of
   when each device learned the link (CDSI), not of rendering.
5. It never resends a logged group message in answer to a retry receipt:
   the membership check always fails (send-log.md §4.2.1). It still resets
   its sharing state, so its next send hands out the key again.
   signal-cli-go resends as intended; whether to match 0.14.7 is an open
   operator decision.
