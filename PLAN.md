# signal-cli-go plan

Clean-room, Go-standard-library-only, drop-in replacement for signal-cli
**v0.14.7** (JVM release). The full brief is in `PROMPT.md`; this file is the
living plan and progress log. Read it, plus `COMPAT.md`, at the start of
every session, and update both before ending one.

## Session start checklist

1. `sandbox/preflight.sh` must print `PREFLIGHT OK`. If not, stop and tell
   the operator. Never fall back to running on the host.
2. Read this file and `COMPAT.md`.
3. `just` lists recipes. `just test` runs offline tests; `just ci` is what
   CI runs.
4. If `third_party/` is missing, run `just install-oracle` (verified download).

## Milestones

| #  | Milestone                                   | Status |
|----|---------------------------------------------|--------|
| 0  | Bootstrap: module, justfile, CI, docs, oracle installer | done (2026-10-23) |
| 1  | Surface inventory: CLI + JSON-RPC specs, COMPAT.md rows, account-free goldens | done (2026-10-23) |
| 2  | Crypto primitives with Layer A vectors      | **in progress** |
| 3  | Signal protocol core (PQXDH, ratchet, sender keys, sealed sender) | not started |
| 4  | Service layer, first Layer C 1:1 exchange on staging | not started |
| 5  | All message types, attachments, receipts, sync, linked devices | not started |
| 6  | Groups v2 and zkgroup                        | not started |
| 7  | Profiles, contacts, usernames, storage service, CDSI, SVR | not started |
| 8  | Daemon parity (HTTP/SSE, socket, TCP); Layer B fully green | not started |
| 9  | CLI parity                                   | not started |
| 9b | Revisit deferred items (voice calls, D-Bus client mode) | not started |
| 10 | Stretch goal: import an existing signal-cli data dir | not started |

The operator reviewed this file and the first COMPAT.md draft on 2026-10-23.

## Architecture

The layout follows PROMPT.md section 5. Packages appear as their milestone
starts. Existing pieces:

- `cmd/signal-cli-go/`: entrypoint (stub)
- `internal/repocheck/`: repo-rule tests (no `require` in go.mod; no
  secrets in `testdata/`)
- `tools/install-oracle/`: downloads signal-cli 0.14.7 into `third_party/`,
  checks SHA-256 and the OpenPGP signature, and extracts it safely
- `spec/`: clean-room specs (spec role only, see `CLEANROOM.md`)
- `testdata/vectors`, `testdata/golden`: see `testdata/README.md`

## Decisions

- **Module path** `github.com/aren55555/signal-cli-go`, go 1.27.3.
- **Oracle trust**: the tarball SHA-256 is pinned from the GitHub release
  API digest. The signature is checked against key
  `FA10826A74907F9EC6BBB7FC2BA2CD21B5B09570`, fetched from
  `github.com/AsamK.gpg`. signal-cli's README publishes no fingerprint,
  so the GitHub account is the only trust anchor. The JDK is baked into
  the sandbox image, which the oracle runs on.
- **Progress log** lives at the bottom of this file, not in a separate file.
- **Voice calls: deferred** (operator, 2026-10-23). Calls are excluded for
  now and revisited after milestone 9. They are deferred, not "won't do":
  the specs stay in `spec/surface/` and the COMPAT.md rows are marked
  `deferred`. Until then signal-cli-go answers the call commands and
  methods (`startCall`, `acceptCall`, `hangupCall`, `rejectCall`,
  `listCalls`, `subscribeCallEvents`, `unsubscribeCallEvents`) with a
  documented error rather than implementing them. Deferred rows do not
  count toward "100%" until they are revisited.
- **D-Bus client mode: deferred** (operator, 2026-10-23). The CLI's
  `--dbus`, `--dbus-system` and `--bus-name` (which drive a running daemon
  over D-Bus) are handled like voice calls: excluded for now, revisited in
  milestone 9b, interim documented error, COMPAT.md rows marked `deferred`.
  `daemon --dbus` stays excluded by PROMPT.md.
- **Proxy support**: all egress goes through `HTTPS_PROXY`. Our HTTP and
  WebSocket clients must support HTTP CONNECT proxies from day one, or
  Layer C cannot run.

## Sandbox notes (non-obvious, learned the hard way)

- `/tmp` is a `noexec` tmpfs, so `go test` fails unless `GOTMPDIR` points
  somewhere else. The justfile sets it to `~/.cache/gotmp`, inside the
  `agent-home` volume. A plain `go test` outside `just` needs
  `GOTMPDIR` exported.
- The reference signal-cli unpacks libsignal JNI into the JVM temp dir,
  which defaults to `/tmp`. From noexec `/tmp` it fails with "Missing
  required native library dependency: libsignal-client", and the output
  looks plausible but is wrong. `just oracle`, `internal/difftest`, and
  (after a relaunch) `compose.yaml` set `-Djava.io.tmpdir=~/.cache/jvmtmp`.
  Recording refuses any output that contains that error.
- `go test -race` needs cgo and gcc. Commit `4cf161f` adds `gcc` and
  `libc6-dev` to the image. The operator rebuilt it on 2026-10-23, and
  `just ci` (with `-race`) passes locally.
- The git identity (Aren Patel <git@arenpatel.com>, the identity in
  history) is set in `.git/config`, so plain `git commit` works. No
  `-c` overrides are needed.
- In this Claude Code build, `grep` in the agent shell is a wrapper
  function that fails with `unknown option '-G'`. Use `command grep`.

## Milestone 2 plan: crypto primitives

Order matters: each step is a dependency of the next. Every primitive gets
standard vectors (Layer A) before it is used, and secret-dependent code is
constant-time. Any place where that is uncertain is noted in the code and
here.

1. **Spec requests (spec subagents, run in parallel with step 2)**:
   - which KEM each key and message type in the libsignal version bundled
     with signal-cli 0.14.7 uses (pre-standard Kyber1024 or FIPS 203
     ML-KEM-1024), with key-type prefix bytes;
   - published test vectors as data in `testdata/vectors/` (XEdDSA, KEM
     keys, HKDF info strings used by Signal constructions), with citations.
2. **edwards25519**: field arithmetic mod 2^255-19 (fixed limbs,
   constant-time select and swap), group operations, scalars mod the group
   order, encoding. Vectors: RFC 8032 (checked against `crypto/ed25519`
   too), RFC 7748 (checked against `crypto/ecdh`).
3. **Ristretto255** (RFC 9496 vectors). zkgroup in milestone 6 builds on
   it.
4. **XEdDSA and VXEdDSA**, from Signal's published specification at
   signal.org/docs (a public standard, so the implementation role reads it
   directly). Vectors from step 1.
5. **KEM**: `crypto/mlkem` if the answer is ML-KEM. Otherwise an in-repo
   pre-standard Kyber1024 with its round-3 known-answer tests.
6. **Protobuf wire codec**: hand-written varint, length-delimited and
   fixed types, unknown-field preservation, plus a `testing.F` fuzz test.

The primitive rows in COMPAT.md turn green as each step lands with its
vectors.

## Facts from the milestone 1 inventory that shape the implementation

- Help and parse-error text wrap at `$COLUMNS`, else the terminal width,
  else 75, with fully justified lines. Goldens leave `COLUMNS` unset
  (width 75, what scripts see); `*-columns-80` and `*-columns-120`
  scenarios cover other widths.
- With `LANG` unset, non-ASCII characters in help print as `?`. Goldens pin
  `C.UTF-8`.
- Some JSON objects (send results, joinGroup and quitGroup) come out in a
  different key order on each JVM run, so difftest needs semantic JSON
  comparison for those. Byte comparison is fine elsewhere.
- Long options accept any unambiguous prefix; for a repeated option the
  last value wins; when several required options are missing, only the
  last-declared one is reported.
- Where the man page and the binary disagree (config-file merging, the
  `receive` timeout of 3 s rather than 5 s), the binary wins.
- JSON-RPC: requests run concurrently on stream transports, so responses
  can come back out of order. HTTP errors are 200 with a JSON-RPC error,
  except 415, 405, 404, 421 and 400 as listed in `transports.md`.
- **Safety**: any oracle command that gets past local validation
  contacts Signal, and production is the default environment. Live
  oracle runs must pass `--service-environment staging`, and the egress
  allowlist must keep excluding production hosts. (It blocked one
  accidental production `register` attempt on 2026-10-23.)

## Open questions and risks

- **Kyber vs ML-KEM**: confirm which KEM each libsignal 0.14.7-bundled
  message and key type uses. Pre-standard Kyber1024 needs an in-repo
  implementation; `crypto/mlkem` covers only FIPS 203. (Spec task for
  milestone 2/3.)
- **Enclave attestation** (CDSI, SVR2/SVR3): needs SGX DCAP quote and
  collateral verification and Noise. That looks feasible with `crypto/x509`
  and `crypto/ecdsa`, but it is unverified. If it isn't feasible, stop
  and ask, per PROMPT.md section 6.
- **Staging hosts**: `sandbox/ALLOWED_HOSTS` may need more staging hosts
  once real staging traffic starts (milestone 4). Each addition needs
  operator approval.
- **Java-specific error text** (milestone 8): many JSON-RPC errors carry
  Jackson parser messages with Java class names (for example
  ``Cannot construct instance of `org.asamk.signal.jsonrpc.JsonRpcRequest` ``).
  Strict byte parity means reproducing those strings. The goldens keep
  them verbatim. Decide in milestone 8 whether to match them exactly or to
  relax the comparison to code plus `data` for those cases, documented as a
  normalization. Verbose `-v` log lines (upstream logger names) raise the
  same question for the CLI.
- **License**: undecided. See `CLEANROOM.md`.

## Progress log

### 2026-10-23, session 1
- Preflight OK inside the sandbox (amd64, Go 1.27.3, Temurin 25.0.5+8,
  just 1.59.0).
- Committed the host-written sandbox definition unchanged (`35a1efc`).
  Then fixed two gaps: noexec `/tmp` broke `go test`, and there was no
  C toolchain for `-race` (`4cf161f`). The image needs a rebuild.
- Milestone 0: `go mod init`, justfile, CI workflow (actions pinned by
  SHA), repo-rule tests, oracle installer with tests, and these documents.
  `just install-oracle` verified and installed signal-cli 0.14.7;
  `--version` runs.
- Milestone 1 started: spec subagents launched for the CLI and JSON-RPC
  surface inventory.
- `internal/difftest`: CLI golden harness. Scenarios in
  `testdata/golden/cli/scenarios.json`, record with `just record-golden`,
  replay in `go test`. Not-yet-matching scenarios are listed in `PENDING`
  (they run, and an unexpected match fails). Nine seed goldens recorded.
- Both spec subagents finished: `spec/surface/jsonrpc/` (64 methods)
  and `spec/surface/cli/` (61 subcommands, 14 global options). They are
  logged in CLEANROOM.md, and COMPAT.md has the full row list (draft).
- **Next**: turn the account-free cases in `spec/surface/cli/observations.md`
  and `spec/surface/jsonrpc/observations.md` into difftest scenarios and
  record them. That needs a JSON-RPC harness (HTTP, socket, TCP) and
  semantic JSON comparison. Then show the operator PLAN.md and COMPAT.md
  before milestone 2.
- Operator decision: voice calls deferred until after milestone 9
  (12 COMPAT.md rows marked `deferred`; milestone 9b added).
- Operator decision: D-Bus client mode deferred to milestone 9b
  (3 global-option rows in COMPAT.md marked `deferred`).
- Milestone 1 done. Account-free goldens:
  - CLI: 355 scenarios (`testdata/golden/cli/`). 329 are generated from
    `spec/surface/cli/observations.md` by `tools/obs2scenarios`; 26 are
    hand-written (setup cases: config files, a two-account data dir;
    wrap widths; seeds).
  - JSON-RPC: 162 scenarios (`testdata/golden/rpc/`). HTTP paths, verbs,
    Content-Type and Host handling, `/check`, SSE headers, every
    request-parsing body from the observations, TCP and socket sessions,
    and every method with zero accounts and no params. `startLink` and
    `finishLink` are excluded because they would contact Signal.
  - All 517 are in PENDING. Exit codes of all 329 generated CLI scenarios
    match the spec agent's observations. Re-recording a CLI subset was
    byte-identical; JSON-RPC re-records differ only in batch and stream
    order, which the comparison ignores (tested).
  - Recording takes about 5 minutes (CLI, 4 JVMs in parallel) plus 20 s
    (JSON-RPC).
