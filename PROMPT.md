# Task: clean-room reimplementation of signal-cli in pure Go

You are starting in an empty directory. Your job is to build `signal-cli-go`, a
drop-in replacement for **signal-cli v0.14.7** (the JVM release, and libsignal
at the version that release bundles), written in Go with no non-Go code.
Compatibility must be proven by automated tests, not asserted.

This is a long, multi-session project. Plan for that from the first commit:
keep a written plan, a progress log and a compatibility matrix in the repo,
so any later session can pick up the work cold.

## 0. Sandbox: never touch the host

This is the most important rule in this document and it overrides every other
instruction. All work happens inside an isolated sandbox. Nothing you do may
install, modify, or execute anything on the host machine.

- **Where work runs**: a Linux container or VM defined in this repo under
  `sandbox/` (a Containerfile with its base image pinned by digest, plus a
  README on how to build and launch it). The agent session itself runs inside
  the sandbox, so every command you execute, including builds, tests, the
  reference signal-cli, the JDK, and the daemon, runs there.
- **Preflight check, every session, before anything else**: confirm you are
  inside the sandbox. The image bakes in a marker file
  (`/etc/signal-cli-go-sandbox`), and you also check that the OS is Linux and
  that host paths like `/Users` aren't visible. If any check fails, stop
  immediately and tell the operator. Never fall back to running on the host.
- **Bootstrapping**: if `sandbox/` doesn't exist yet, the only thing you may
  do from the host is write the sandbox definition files into this repo. Do
  not build or run them. Then stop and ask the operator to build the sandbox
  and restart the session inside it.
- **Nothing gets installed on the host**: no Go toolchain, JDK, signal-cli,
  Homebrew packages, global modules, or changes to shell config, launch
  agents, or system settings. Toolchains are baked into the sandbox image and
  pinned by version and checksum.
- **Minimal host exposure**: the repo checkout is the only host path mounted
  into the sandbox. Never mount `$HOME`, `~/.ssh`, `~/.config`, the host
  keychain, or the Docker/container runtime socket. No privileged containers
  and no added capabilities.
- **Writes stay contained**: everything you write goes into the repo or into
  paths internal to the sandbox. Go caches, `third_party/`, the JDK, and the
  reference signal-cli's data directories live inside the sandbox or in
  gitignored repo directories, never in the host's home directory.
- **Downloads are untrusted**: verify every downloaded artifact (the
  signal-cli release, the JDK) against a SHA-256 checksum pinned in the repo,
  and against published signatures where they exist, before using it. Run
  these artifacts only inside the sandbox.
- **Network egress is allowlisted**: list the hosts the work needs (Signal
  staging endpoints, the signal-cli and JDK download hosts) in
  `sandbox/ALLOWED_HOSTS`, and enforce the list in the sandbox if the tooling
  supports it. Adding a host requires the operator's approval.
- **Secrets**: the operator passes staging test numbers and captcha tokens
  into the sandbox at runtime, as environment variables. They are never
  written to the host or committed.
- **Git**: commit inside the sandbox. Never push from it, since it has no
  credentials. The operator pushes from the host.
- If a task seems to need host access (a GUI, a device, the host keychain,
  a host-installed tool), stop and ask. There are no "just this once"
  exceptions.

## 1. Definition of "drop-in"

A user must be able to replace the `signal-cli` binary with `signal-cli-go` and
see no difference across these surfaces, in priority order:

1. **The JSON-RPC daemon** (`signal-cli daemon --http`, `--socket`, `--tcp`,
   `--dbus` excluded): every JSON-RPC method, parameter, result shape, error
   code and notification shape. Over HTTP that means `POST /api/v1/rpc`, the
   SSE stream at `GET /api/v1/events`, and `GET /api/v1/check`. This surface
   is the top priority because existing integrations depend on it.
2. **The CLI**: every subcommand, flag, exit code, and stdout/stderr output in
   both `--output=plain-text` and `--output=json`.
3. **On the wire, to Signal's servers and to other clients**: a `signal-cli-go`
   account must interoperate with official Signal apps and with real
   signal-cli: 1:1 messages, groups v2, attachments, reactions, quotes,
   mentions, edits, deletes, read and typing receipts, stories, sealed sender,
   linked devices, and sync messages.
4. **Stretch goal: the on-disk data directory.** Opening an existing
   signal-cli data directory (account JSON files plus its SQLite
   `account.db`) and migrating it. This needs a SQLite reader, which the
   standard library doesn't have (see section 3), so do it last, as an import
   tool. Until then, signal-cli-go can use its own storage format.

Out of scope: voice and video calls (signal-cli doesn't support them either).

## 2. Clean-room rules

signal-cli is GPLv3, and libsignal and Signal-Server are AGPLv3. The point of
this project is an independent implementation that copies none of their code.
Follow this two-role process strictly:

- **Spec role** (run as subagents): may read upstream sources, published
  docs, and the behavior of the running signal-cli binary. Its only output is
  prose specifications in `spec/`: behavior, state machines, message flows,
  and wire-level facts that interoperability requires (protobuf field numbers
  and types, JSON keys, HTTP paths, KDF info strings, constants). It must
  **never** write code, pseudocode that mirrors upstream structure, or copied
  comments into `spec/`. Every spec file cites its sources (URL plus a
  version or commit).
- **Implementation role** (you, the main session): writes all Go code,
  working **only** from `spec/`, public standards (RFCs, FIPS, Signal's
  published specifications at signal.org/docs), and black-box observation of
  the signal-cli binary. You do not open upstream source files yourself. If
  the spec is missing something, ask a spec subagent for a narrower spec;
  don't go and read the source.
- Keep a `CLEANROOM.md` log recording which source each spec came from and
  which subagent produced it.
- Be honest about the limits: your training data probably includes these
  codebases, so this process reduces copying but can't certify independence.
  Write code in your own structure and idiom. Don't recall upstream code from
  memory and transliterate it. If the result will be relicensed under
  anything other than GPL or AGPL, flag in `CLEANROOM.md` that the owner
  should get legal review.

Running the real signal-cli and recording its inputs and outputs is
black-box observation, and both roles may do it.

## 3. What "vanilla Go" means

- **Go standard library only**: no third-party modules, no cgo, no assembly
  beyond what the standard library ships. `go.mod` has no `require` lines.
- Anything the standard library doesn't provide, you implement in-repo under
  `internal/`, each piece with its own test vectors. Expect to write:
  - edwards25519 field and group arithmetic, plus Ristretto255 (the
    standard library's version is internal and can't be imported)
  - XEdDSA and VXEdDSA signatures over X25519 keys
  - the protobuf binary wire format: a small hand-written encoder/decoder.
    Don't build a code generator unless hand-writing gets clearly worse.
  - an RFC 6455 WebSocket client
  - Signal's zkgroup (zero-knowledge group credentials and proofs)
  - anything needed for SGX/enclave attestation (CDSI, SVR2/SVR3), plus Noise
    handshakes
  - Kyber/ML-KEM where needed. Check carefully whether a given message uses
    pre-standard Kyber1024 or FIPS 203 ML-KEM-1024. `crypto/mlkem` covers
    only the FIPS 203 version.
  - storage. SQLite isn't in the standard library, so design a simple
    file-based store with crash-safe writes (write to a temp file, fsync,
    rename) and a migration path.
- Use what the standard library does provide: `crypto/ecdh` (X25519),
  `crypto/ed25519`, `crypto/aes`, `crypto/cipher`, `crypto/hmac`,
  `crypto/sha256`, `crypto/sha512`, `crypto/hkdf`, `crypto/mlkem`,
  `crypto/tls`, `net/http`, `encoding/json`.
- All crypto must be constant-time where the secret-handling requires it.
  Note any place where you're unsure.

## 4. Proving compatibility: three layers of tests

**Layer A: primitive test vectors** (`go test ./...`, offline, fast)
- Use RFC and standard vectors wherever they exist (RFC 7748, RFC 8032,
  RFC 5869, RFC 9496 for Ristretto255, FIPS 203, NIST AES-GCM).
- For Signal-specific constructions (XEdDSA, X3DH/PQXDH, Double Ratchet,
  sealed sender, zkgroup, attachment encryption, profile encryption, group
  master key derivation), a spec subagent extracts the **published test
  vectors** (input/output data, not code) from upstream test suites into
  `testdata/vectors/*.json`, with citations. Data is fine to reuse; test code
  is not.
- Add property and fuzz tests (`testing.F`) for every parser that handles
  untrusted input: protobuf, WebSocket frames, envelopes, JSON-RPC.

**Layer B: differential black-box tests against the real signal-cli**
- Write a harness (`internal/difftest`) that runs the same scenario against
  both the reference signal-cli 0.14.7 binary and signal-cli-go, then compares:
  CLI stdout, stderr and exit codes; JSON-RPC responses; and SSE/notification
  payloads. Normalize only fields that genuinely vary (timestamps, random
  IDs) and document each normalization.
- Record reference runs as golden transcripts in `testdata/golden/`, so
  Layer B can replay offline once recorded. Live re-recording is a separate
  make/just target.
- Cover argument-parsing errors, `--help` output, unknown methods, malformed
  JSON-RPC, and the error-code mapping, not just the happy paths.
- The reference signal-cli is the JVM build. Install it into `third_party/`
  (gitignored) with a script that pins 0.14.7, plus a JDK it can run on.
  It's a test oracle only and never a runtime dependency.

**Layer C: end-to-end interop on Signal's staging environment**
- Use `--service-environment staging` and dedicated test numbers. Never run
  tests against production with someone's real account. The operator will
  supply numbers and solve captchas. Pause and ask for these; never invent
  or reuse credentials.
- Scenarios run signal-cli-go ⇄ real signal-cli in both directions: register or
  link, exchange keys, 1:1 text, every message type listed in section 1,
  group create/update/join/leave, attachments in both directions, safety
  number change and trust, a linked device receiving sync messages, and
  session recovery after one side reinstalls.
- Rate-limit yourself and back off on 413/429 responses. A misbehaving
  client can get a number rate-limited or banned.

Keep a **compatibility matrix**, `COMPAT.md`. Each row is a CLI command,
JSON-RPC method or protocol feature, and the columns are spec'd /
implemented / Layer A / Layer B / Layer C. A feature counts as done only
when its row is green and links to the tests that prove it. "100%" means
every row is green.

## 5. Suggested architecture (adjust as you learn)

```
cmd/signal-cli-go/      CLI entrypoint and flag parsing (mirrors signal-cli's surface)
internal/crypto/...     curve25519/edwards25519, ristretto, xeddsa, kem, kdf helpers
internal/protocol/      X3DH/PQXDH, Double Ratchet, sessions, prekeys, sealed sender, sender keys
internal/zkgroup/       group credentials, profile key credentials, proofs
internal/proto/         hand-written protobuf wire codec + message types
internal/service/       Signal server HTTP + WebSocket API client, auth, CDN, attachments
internal/enclave/       attestation, Noise, CDSI, SVR
internal/store/         account, identity, session, prekey, group, recipient stores
internal/manager/       high-level operations (the equivalent of signal-cli's manager layer)
internal/jsonrpc/       daemon: HTTP, SSE, socket, TCP transports
internal/output/        plain-text and JSON output formatting
internal/difftest/      differential test harness
spec/                   clean-room specifications (spec role only)
testdata/               vectors and golden transcripts
```

## 6. Milestones (each one ends with tests green and a commit)

0. **Bootstrap** (inside the sandbox from section 0): `go mod init`, a justfile (with no arguments, `just` must
   only list recipes), CI that runs `go vet`, `go test -race`, and a check
   that `go.mod` has no requires. Add PLAN.md, COMPAT.md, CLEANROOM.md, and
   the reference signal-cli installer script.
1. **Surface inventory**: spec subagents enumerate every CLI subcommand and
   flag, and every JSON-RPC method with its params, result and errors, by
   running `signal-cli --help` and each `<cmd> --help`, reading the public
   man page and docs, and exercising the daemon. This produces the full
   COMPAT.md row list. Then record golden transcripts for everything that
   needs no account.
2. **Crypto primitives** with Layer A vectors.
3. **The Signal protocol core**: sessions, PQXDH, Double Ratchet, sender
   keys, sealed sender, all Layer A. Interop is checked offline by
   decrypting messages captured from the reference client.
4. **Service layer**: registration, verification, linking (provisioning),
   prekey upload, WebSocket message pipe, send and receive. This is the first
   Layer C test: a 1:1 text exchange with real signal-cli on staging.
5. **Messages**: all content types, attachments, receipts, sync messages,
   linked devices.
6. **Groups v2** and zkgroup.
7. **Profiles, contacts, usernames, storage service**, then CDSI and SVR
   (PIN and registration lock).
8. **Daemon parity**: every JSON-RPC method, plus SSE, socket and TCP
   transports. Layer B is fully green.
9. **CLI parity**: every remaining command and output mode.
10. **Stretch goal**: importing an existing signal-cli data directory.

When a feature becomes infeasible under these constraints (for example,
attestation turns out to need something the standard library can't support),
stop. Write the problem up in PLAN.md with the options, and ask the operator.
Don't quietly add a dependency or skip the feature.

## 7. Working rules

- Work milestone by milestone. At the start of each session, read PLAN.md
  and COMPAT.md, and update them before ending.
- Write the failing compatibility test before the feature.
- Never mark a COMPAT.md row green without a passing test that proves it.
  If a test is skipped, flaky, or only partially covers the behavior, say so
  in the row.
- Never commit secrets: phone numbers, keys, captcha tokens, or recorded
  transcripts that contain real identifiers. Scrub golden files and add a
  test that scans `testdata/` for phone-number and key patterns.
- Small, focused commits with clear messages.
- Prefer `go run` for helper scripts and `bun` if you need JavaScript. Don't
  use Python or Node.

**Start now**: run the section 0 preflight check. If you aren't inside the
sandbox, write the sandbox definition if it's missing, then stop and ask the
operator to launch it. Once you're inside, do milestone 0, then begin milestone 1 by launching spec
subagents to inventory the CLI and JSON-RPC surface. Show me PLAN.md and the
first draft of COMPAT.md before starting milestone 2.
