# signal-cli 0.14.7 never resends a group message in answer to a retry receipt

Report for https://github.com/AsamK/signal-cli (draft, 2026-10-30). Observed
with signal-cli 0.14.7 against Signal's staging servers on 2026-10-29, and
reproduced on 2026-10-30 with signal-cli 0.14.7 on every participating
device.

## What happens

When a group member fails to decrypt a group message sent by signal-cli,
the member's client sends a retry receipt (a `DecryptionErrorMessage`
naming the message's timestamp). signal-cli finds the message in its
message send log, but instead of resending it, it logs

    WARN  SendHelper - The target user is no longer in the group <group id>! Skipping message send.

and sends nothing, even though the requester is a full member of the
group. `listGroups -d` on the same account lists the requester as a
member, and signal-cli keeps delivering new group messages to it (over
sender key).

signal-cli still deletes its "shared with" record for the requester's
device, so its *next* group message carries a fresh sender key
distribution message and that one decrypts. The message that failed is
never delivered again.

## Why (as far as we can tell)

The check before the resend asks whether the group's member collection
contains the requester's `RecipientId`. That collection holds member-info
objects (one per full member), not `RecipientId`s, and the two types never
compare equal. So the check is false for every requester and every group,
and every group resend takes the "no longer in the group" branch. Other
membership checks in the codebase compare the members' `RecipientId`s and
are not affected. This is only an analysis of the behaviour; we have not
tested a patch.

## How to reproduce

You need three accounts on staging (`--service-environment staging`):
**A** runs signal-cli 0.14.7 and sends; **B** and **C** are group members.
All three run signal-cli 0.14.7.

1. Create a group on A with B and C as full members (all three must have
   exchanged profile keys, e.g. by messaging each other first, so that A
   uses sender key: A needs at least two qualifying members).
2. Have A send a group message and let B and C receive it, so that B
   holds A's sender key and A has recorded that B has it.
3. Stop B. Remove the sender key B received from A from B's
   `account.db`, for example:

       sqlite3 ~/.local/share/signal-cli/data/<B's path>.d/account.db \
         "DELETE FROM sender_key WHERE address = '<A's ACI>';"

4. Have A send another group message:
   `signal-cli -a <A> send -g <group id> -m "test"`.
5. Receive on B: the message fails with
   `missing sender key state for distribution ID …` and B sends a retry
   receipt to A.
6. Receive on A (`signal-cli -a <A> receive`, `-v` shows more). A logs the
   WARN above and sends nothing.
7. Receive on B again: the message from step 4 never arrives.

We saw the same result with the requester's membership coming from an
accepted invitation and from being in the group since creation.

### Observed run (2026-10-30, signal-cli 0.14.7 on every device involved)

C sent, B received on a linked signal-cli device whose stored sender keys
from C had been removed (the account's other device stayed idle), A was the
third member. Output, with identifiers shortened:

- B, receiving the test message:

      Sent by unidentified/sealed sender
      Exception: org.signal.libsignal.protocol.NoSessionException: missing sender key state for distribution ID e771…a5cd (ProtocolNoSessionException)

- A decrypted the same message normally.
- C, receiving B's retry receipt:

      WARN  SendHelper - The target user is no longer in the group 1Do2…x+c=! Skipping message send.

- B, receiving again: the test message never arrived. C's next group
  message did decrypt on B (it carried a fresh sender key distribution
  message), but the test message stayed lost.

An earlier run (2026-10-29) with a different client as B gave the same
WARN on C.

## Expected behaviour

For a requester that is a full member of the group, signal-cli should
resend the logged message to the requesting device, with the same
timestamp and a fresh sender key distribution message attached. That is
what the send-log code path is evidently meant to do, and the official
Signal clients answer retry receipts by resending. The "no longer in the
group" branch should only apply to requesters who really are no longer
members.

## Environment

- signal-cli 0.14.7 (release build, JDK 25), libsignal 0.99.1 as bundled.
- Signal staging servers, 2026-10-29.
