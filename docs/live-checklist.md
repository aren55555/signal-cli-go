# Live production checklist (milestone 12, P7–P10)

The live production runs, in order. Every command goes through the live driver,
which allows only the numbers in `/tmp/live-numbers`, sets
`--service-environment live`, paces sends and masks numbers:

    go build -o ~/.cache/live-driver ./tools/live
    SIGNAL_CLI_GO_LIVE=1 ~/.cache/live-driver ours|ref .data/live/DIR TAG -- ARGS...

`@N1`, `@N2` and `@N3` stand for lines 1–3 of `/tmp/live-numbers`:

| | Number | Client |
|---|--------|--------|
| N1 | registered with the JVM signal-cli 0.14.7, then imported (P8) | `ref`, then `ours` |
| N2 | registered with signal-cli-go (P7) | `ours` |
| N3 | the official Signal app on the operator's phone | the operator |

All three numbers are **temporary leases**. Never commit numbers, codes,
captchas, tokens or transcripts. Results go in COMPAT.md, section Production.

## P7: live proof with signal-cli-go (N2) and the official app (N3)

- [x] Register N2 (production captcha and SMS code from the operator); set a profile name (2026-10-31)
- [x] CDSI negative lookup: numbers not on Signal return `isRegistered: false` (2026-10-31)
- [x] The operator registers N3 in the official app
- [x] CDSI positive lookup of N3 (`getUserStatus @N3` returns its ACI)
- [x] 1:1 text both ways, N2 ↔ N3
- [x] Throwaway group of N2 and N3 (test accounts only): messages both ways, with @mentions and quotes
- [x] Images both ways (1:1 and in the group)
- [ ] Reactions and typing indicators both ways
- [x] Receiving through `daemon --http` (`curl -N http://127.0.0.1:PORT/api/v1/events`)

## P8: import of a production account (N1)

- [ ] Register N1 with the JVM signal-cli (`ref`); 1:1 and group messages with N3
- [ ] Stop it, import with `signal-cli-go-import`, run signal-cli-go on N1
- [ ] Sessions decrypt both ways and groups are intact; N3's app shows no
      re-registration, re-link or safety-number change

## P9: daemon side by side

- [ ] The JVM and signal-cli-go daemons on test accounts, the listed methods and
      SSE events diffed with Layer B normalization, scrubbed

## P10: release the leased numbers (before the leases end)

So the next owner of each number doesn't inherit a Signal account:

- [ ] Delete N2's account from the server:
      `~/.cache/live-driver ours .data/live/n2 p10-n2 -- -a @N2 unregister --delete-account`
- [ ] Delete N1's account the same way, from its imported signal-cli-go data
      directory (or with `ref` if the import never happened)
- [ ] From a remaining account, `getUserStatus @N1 @N2` returns `isRegistered: false`
      for both. Run this before N3 is deleted, or skip it if N3 is already gone
- [ ] **Remind the operator** to delete N3's account in the official app
      (Settings → Account → Delete account)
- [ ] Remove `.data/live/` (keys of deleted accounts are of no further use)
- [ ] Record the cleanup in PLAN.md's progress log
