# Clean-room log

signal-cli is GPLv3; libsignal and Signal-Server are AGPLv3. This project
aims to be an independent implementation that copies none of their code.

## Process

- **Spec role** (subagents only): may read upstream sources, published
  docs, and the running signal-cli binary. Writes **prose** specifications
  to `spec/` and nothing else: no code, no pseudocode that mirrors
  upstream structure, no copied comments. Every spec cites its sources
  (URL plus version or commit). Spec agents may also extract published
  test-vector **data** into `testdata/vectors/`, with citations.
- **Implementation role** (main session): writes all Go code, working only
  from `spec/`, public standards (RFCs, FIPS, signal.org/docs), and
  black-box observation of the signal-cli binary. It does not open
  upstream source files. If a spec is missing something, it requests a
  narrower spec instead.
- Both roles may run the reference binary and record its inputs and
  outputs (black-box observation).

## Honest limits

The implementing model's training data very likely includes signal-cli,
libsignal, and Signal-Server. This process reduces the risk of copying,
but it **cannot certify independence**. The implementation role writes
code in its own structure and idiom and does not transliterate recalled
upstream code.

**Licensing flag:** if this project is released under any license other
than GPL or AGPL, the owner should get a legal review first.

## Log

Each row is one spec artifact: which subagent produced it, from which
sources.

| Date | Spec file | Produced by | Sources (URL @ version/commit) | Notes |
|------|-----------|-------------|--------------------------------|-------|
| 2026-10-23 | `spec/surface/jsonrpc/` (transports, errors, notifications, observations, `methods/*.md` ×65) | spec subagent "JSON-RPC surface inventory" (Claude Opus 5.5, general-purpose), plus four forks of itself for the method files | signal-cli @ v0.14.7: `src/main/java/org/asamk/signal/{commands,jsonrpc,http,json,util,output}/…`, `DbusConfig.java`; tests `JsonCallEventTest.java`, `SubscribeCallEventsTest.java`; `lib/src/main/java/org/asamk/signal/manager/{api/*,Manager,MultiAccountManager,RegistrationManager,ProvisioningManager,SignalAccountFiles,internal/*Impl,helper/{Group,Account,Profile}Helper,helper/CallManager,util/Utils}.java`; `man/signal-cli-jsonrpc.5.adoc`, `man/signal-cli.1.adoc`, `docs/CALL_TUNNEL.md`; release asset `signal-cli-0.14.7-json-schemas.tar.gz` (sha256 5ca03152…fc1c, verified); argparse4j 0.9.0 (MIT) getter behavior via `javap -c`; black-box runs of the 0.14.7 binary (commands in `observations.md`) | Method files come from reading the source; only the transport, error and zero-account behavior was checked against the binary. **Deviations, reported by the agent:** (1) briefly bound a daemon to `0.0.0.0:18081` to observe Host-header checking, then killed it (the rule was 127.0.0.1 only); (2) used a one-line perl script to reach the UNIX socket (no socat/nc in the image); (3) a fork wrote a scratch Java program (a HashMap key-order probe, not upstream code) to `~/.cache/jvmtmp/hm/`, which the main session reviewed and deleted. Main-session review: no code or pseudocode found; Java class names appear only inside verbatim error texts and source citations. An example phone number that could belong to a real person was replaced with a placeholder. |
| 2026-10-23 | `spec/surface/cli/` (`index.md`, 61 command files, `_send-results.md`, `_received-message-output.md`, `observations.md`) | spec subagent "CLI surface inventory" (Claude Opus 5.5, general-purpose), with forks G1–G4 for the per-command files | signal-cli @ v0.14.7 via raw.githubusercontent.com (file list from the git trees API): `man/signal-cli.1.adoc`, `man/signal-cli-jsonrpc.5.adoc` (partial); `src/main/java/org/asamk/signal/` top-level config/startup files, `commands/*` (all commands plus base types and `exceptions/*`), `output/*`, `util/*`, `logging/*`, `dbus/DbusCommandHandler`, `jsonrpc/{SignalJsonRpcDispatcherHandler,JsonRpcReader}`, `json/*` (output shapes); `lib/.../manager/{Manager, api/*, util/{PhoneNumberFormatter,Utils,StickerUtils}, internal/{ManagerImpl,PathConfig}, helper/{CallManager,AttachmentHelper,SyncHelper}, storage/*}`; black-box: 375 account-free invocations of the 0.14.7 binary (`observations.md`) | Success outputs and exit codes 4/5/6 come from source only. Main-session review: no code or pseudocode; all phone numbers are in the unassigned +1-555 area code. **Incident, reported by the agent:** one early run (`-a <fictional number> register`, no `--service-environment`, so it defaulted to **production**) passed local checks and tried to reach Signal. The egress proxy refused it (403; production hosts are not allowlisted), so Signal was never contacted. Left out of goldens. |
| 2026-10-23 | `testdata/golden/cli/scenarios.json` fixture (two-account `accounts.json`) | main session, copied from the CLI spec agent's scratch data dir | black-box test data the spec agent wrote by hand to probe account selection; no upstream file content | Test data, not code. The two numbers are fictional (+1 555 555-01xx). |
