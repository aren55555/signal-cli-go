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
| 1  | Surface inventory: CLI + JSON-RPC specs, COMPAT.md rows, account-free goldens | **in progress** |
| 2  | Crypto primitives with Layer A vectors      | not started |
| 3  | Signal protocol core (PQXDH, ratchet, sender keys, sealed sender) | not started |
| 4  | Service layer, first Layer C 1:1 exchange on staging | not started |
| 5  | All message types, attachments, receipts, sync, linked devices | not started |
| 6  | Groups v2 and zkgroup                        | not started |
| 7  | Profiles, contacts, usernames, storage service, CDSI, SVR | not started |
| 8  | Daemon parity (HTTP/SSE, socket, TCP); Layer B fully green | not started |
| 9  | CLI parity                                   | not started |
| 10 | Stretch goal: import an existing signal-cli data dir | not started |

Milestone 2 does not start until the operator has reviewed this file and
the first draft of `COMPAT.md`.

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
- **Proxy support**: all egress goes through `HTTPS_PROXY`. Our HTTP and
  WebSocket clients must support HTTP CONNECT proxies from day one, or
  Layer C cannot run.

## Sandbox notes (non-obvious, learned the hard way)

- `/tmp` is a `noexec` tmpfs, so `go test` fails unless `GOTMPDIR` points
  somewhere else. The justfile sets it to `~/.cache/gotmp`, inside the
  `agent-home` volume. A plain `go test` outside `just` needs
  `GOTMPDIR` exported.
- `go test -race` needs cgo and gcc. Commit `4cf161f` adds `gcc` and
  `libc6-dev` to the image. **Until the operator rebuilds**, `just ci`
  and `just test-race` fail locally; GitHub CI is unaffected.
- The repo has no git identity configured, and the README says the
  operator sets it from the host. Until then, commits use
  `git -c user.name=… -c user.email=…` with the identity already in
  history.
- In this Claude Code build, `grep` in the agent shell is a wrapper
  function that fails with `unknown option '-G'`. Use `command grep`.

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
  once the oracle is observed (milestone 1). Each addition needs operator
  approval.
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
