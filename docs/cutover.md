# Moving an account from signal-cli 0.14.7 to signal-cli-go

This is the procedure for replacing the JVM signal-cli 0.14.7 with signal-cli-go on
an existing, registered account. The example is a daemon started as
`signal-cli --config DIR -a NUMBER daemon --http=127.0.0.1:8080 --receive-mode=on-start`.

Read the whole page first, especially [Going back](#going-back-to-the-jvm-signal-cli):
once signal-cli-go has run the account, going back is not free.

## 1. Stop signal-cli and back up everything

1. Stop the JVM signal-cli completely, including the daemon, any cron jobs and any
   wrapper that restarts it. Nothing may write to the data directory while it is
   copied. The importer refuses an account that signal-cli holds open, but a
   backup copied from a running signal-cli can be inconsistent.

2. Back up the **whole** data directory (the `--config`/`-d` directory, default
   `~/.local/share/signal-cli`), keeping permissions:

       tar -C "$(dirname DIR)" -cpf signal-cli-backup-$(date +%Y%m%d).tar "$(basename DIR)"

   The backup holds the account's private keys and its message history: store it
   like a password. It must include `data/accounts.json`, the account file
   `data/<path>`, the directory `data/<path>.d/` (`account.db`, and
   `account.db-wal` if present), and `attachments/`, `avatars/` and `stickers/`.

3. Write down what the account looks like now. These are the reference for
   step 4:

       signal-cli --config DIR -a NUMBER listDevices
       signal-cli --config DIR -a NUMBER listGroups -d
       signal-cli --config DIR -a NUMBER listIdentities

## 2. Import into a new directory

    signal-cli-go-import -from DIR -to NEWDIR -a NUMBER

- The source is only read: the database is copied aside before it is opened.
- Never import into `DIR` itself (the importer refuses).
- The importer prints what it did not carry over. For a current 0.14.7 account
  that is normally nothing, or v1 groups and the username link colour.
- It refuses account files older than version 10 and databases older than schema 30
  (run any account command once with signal-cli 0.14.7 first). It also refuses an
  account that signal-cli holds open.

## 3. Start signal-cli-go

Use the same command line, with the new directory:

    signal-cli-go --config NEWDIR -a NUMBER daemon --http=127.0.0.1:8080 --receive-mode=on-start

Production is the default, as with signal-cli. Don't start the JVM signal-cli on
`DIR` again (see below).

## 4. Check the imported account

With the daemon stopped, or through JSON-RPC against the running daemon:

| Check | Command | Expect |
|-------|---------|--------|
| Same device | `signal-cli-go --config NEWDIR -a NUMBER listDevices` | the same device IDs, names and creation dates as in step 1.3 |
| Identities | `… listIdentities` | the same contacts with the same safety-number fingerprints and trust levels. A changed fingerprint here would mean a broken import: stop and restore |
| Groups | `… listGroups -d` | the same groups, members and admins |
| Sending | `… send --note-to-self -m "signal-cli-go check"` | a timestamp, and the message on your other devices (note to self) |
| Receiving | `… receive -t 30`, or with the daemon running: `curl -N http://127.0.0.1:8080/api/v1/events` | incoming messages decrypt, with no `Exception:` lines for contacts you were already talking to |
| Daemon alive | `curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:8080/api/v1/check` | `200` |

Then send a message to one real conversation and one group, and check the answer
arrives. On the official apps of your contacts nothing should be visible: no "safety
number changed", no "re-registered", no new device. (The production import proof is
tracked in COMPAT.md, Production.)

## Going back to the JVM signal-cli

There is no tool that converts signal-cli-go's data back into signal-cli's format.
The only way back is the backup from step 1.2. **Restoring it is not like undoing
the import.** While signal-cli-go ran, the account's state moved on, on the server
and at your contacts, and the backup does not know that.

What happens, as measured on staging on 2026-10-31. A signal-cli linked device was
backed up, imported and used with signal-cli-go for a few messages, then restored
from the backup and used with signal-cli again, with signal-cli and signal-cli-go
contacts on both sides:

| Effect | Why | Observed | Recovers? |
|--------|-----|----------|-----------|
| Messages that arrived while signal-cli-go ran are gone for signal-cli | the server delivers each envelope once; signal-cli-go acknowledged them | a 1:1 message received by signal-cli-go never reached the restored signal-cli | no: the history only exists in NEWDIR |
| The first 1:1 messages signal-cli sends are rejected by contacts | signal-cli resumes its sending chain where the backup left it and reuses message keys signal-cli-go already used | contacts: `InvalidMessageException: invalid Whisper message: decryption failed` | yes: contacts send retry receipts; signal-cli resends those 1:1 messages and they arrive |
| Its group messages fail for every member | its sender-key distribution messages go out on those same stale sessions and fail (`DuplicateMessageException: message with old counter …`), so the members never get its sender key | members: `NoSessionException: missing sender key state for distribution ID …` | the message is lost: signal-cli 0.14.7 never resends group messages ([upstream report](upstream-reports/signal-cli-group-resend-never-happens.md)). Later group messages work once the sessions are renewed |
| Receiving keeps working for a while | a contact's messages decrypt as long as the contact's sending chain has not turned over | a contact's next message decrypted on the restored signal-cli | until the contact's ratchet turns; then retry receipts and renewal as above |
| After one round of retry receipts and session renewal, things work again | the contacts' clients renew the sessions | new 1:1 and group messages from the restored signal-cli arrived everywhere without errors | yes |
| No safety-number change | the identity keys never change in either client | no "safety number changed" anywhere | n/a |

Not exercised in that test, but expected from how prekeys work (spec/service/keys.md):

- **Prekeys.** signal-cli-go rotates the signed prekey and the last-resort Kyber key
  every 2 days, and uploads new one-time prekeys when the server runs low. If it ran
  long enough to do that, the server hands contacts prekeys whose private keys only
  NEWDIR has. A contact who starts a new session with the restored signal-cli then
  sends a message signal-cli cannot decrypt. This lasts until signal-cli rotates and
  uploads its own keys (its signed prekey looks old to it, so it does so at its next
  account check), and until the contact's client retries.
- **Settings and profile.** Changes signal-cli-go made on the server (profile, group
  changes, storage-service records, username) stay. signal-cli picks them up from the
  server.
- **The send log.** signal-cli cannot answer retry receipts for messages that
  signal-cli-go sent.

So, if you must go back:

1. Stop signal-cli-go for good and keep NEWDIR as a record. **Never run both on the
   same account**: they are the same device with diverging keys.
2. Restore the backup into `DIR` and start signal-cli.
3. Run `receive` a few times over the next minutes, so that retry receipts and
   session renewals are processed. Expect the first messages it sends, especially
   group messages, to be lost.
4. Message the important groups and contacts again once a receive has run.

The longer signal-cli-go ran, the more of this applies. Going back after a few
minutes costs a few messages. After days, also expect prekey failures for new
sessions.
