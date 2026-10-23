# signal-cli-go plan

Clean-room, Go-standard-library-only (plus one vendored module, see Decisions), drop-in replacement for signal-cli
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
| 2  | Crypto primitives with Layer A vectors      | done (2026-10-23) |
| 3  | Signal protocol core (PQXDH, ratchet, SPQR, sender keys, sealed sender) | done (2026-10-23); interop captures for sealed sender, sender keys and post-reply messages deferred until milestone 4 |
| 4  | Service layer, first Layer C 1:1 exchange on staging | **in progress** |
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
- `internal/crypto/edwards25519` (+ `/field`): GF(2^255-19), the
  edwards25519 group, scalars mod l
- `internal/crypto/ristretto255`: RFC 9496
- `internal/crypto/curve25519`: Signal key encoding, X25519, XEd25519,
  the PQXDH base-key check
- `internal/crypto/kyber`: round-3 Kyber1024 (PQXDH prekeys)
- `internal/proto/signalpb`: hand-written `.proto` files (from `spec/`) and
  the generated Go types; `just proto-gen` regenerates, `just proto-check`
  (and CI) fails on drift. The old hand-written codec lives on in its
  `_test.go` files as a differential oracle.
- `vendor/`: google.golang.org/protobuf v1.36.12, the one permitted module
- `internal/testvec`: loads `testdata/vectors/*.json` in tests
- `tools/rfcvectors`: extracts RFC 8032, 7748 and 9496 vectors from the
  RFC texts
- `spec/`: clean-room specs (spec role only, see `CLEANROOM.md`)
- `testdata/vectors`, `testdata/golden`: see `testdata/README.md`

## Decisions

- **Brief amendment: protobuf** (operator, 2026-10-23). This amends
  PROMPT.md section 3. `google.golang.org/protobuf` is permitted as the
  **single** third-party module, vendored (`vendor/`, committed), pinned,
  and built with `-mod=vendor`. ALLOWED_HOSTS gains `proxy.golang.org`
  and `sum.golang.org` for fetching it, and `go.sum` is checked against
  the checksum database. Signal's messages are written as `.proto` files
  from `spec/` (never copied from upstream) and generated with protoc
  (v36.2, installed by `tools/install-protoc`, SHA-256 pinned) and
  protoc-gen-go (from the vendored module). Generated code is committed,
  and CI checks that regenerating is a no-op. The hand-written
  `internal/proto/wire` codec is replaced by the generated types and
  survives only as a differential test. The no-dependencies repo rule
  becomes "only this module".
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
- **KEMs** (from `spec/crypto/kem.md`): PQXDH prekeys use pre-standard
  round-3 Kyber1024 (type `0x08`, expanded secret keys), implemented
  in-repo. CDSI/SVR2 Noise uses FIPS 203 ML-KEM-1024 (`crypto/mlkem`).
  SPQR uses ML-KEM-768 through libcrux's incremental API (see open
  questions).
- **XEd25519** follows signal.org plus the libsignal deviations in
  `spec/crypto/xeddsa.md` (sign bit in signature bit 511, nonce over the
  clamped key bytes, lenient u and S). VXEdDSA isn't used.
- **Strict Ed25519 point decoding** (RFC 8032): `edwards25519.SetBytes`
  rejects y >= p. XEd25519 never decodes R, so this doesn't affect
  interop.
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
- The image sets `GOPROXY=off` and `GOSUMDB=off`, and the justfile sets
  `GOFLAGS=-mod=vendor`. To change the vendored module (deliberately), run
  `GOFLAGS=-mod=mod GOPROXY=https://proxy.golang.org GOSUMDB=sum.golang.org go get …`
  and then `go mod vendor`, and update the pin in `internal/repocheck`.
  The egress proxy reads ALLOWED_HOSTS at startup only, so host changes
  need a proxy restart.
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

- **SPQR (new, milestone 3)**: libsignal v0.99.1 makes the Sparse
  Post-Quantum Ratchet mandatory for every session (minimum version 1).
  It uses ML-KEM-768 through libcrux's *incremental* API, with chunked
  keys and ciphertexts and libcrux-specific state encodings (including a
  workaround for libcrux issue 1275). `crypto/mlkem` has no such API.
  The in-repo Kyber core already has a FIPS 203 variant, checked
  against `crypto/mlkem` 768, which is a likely base. SPQR needs its
  own spec before milestone 3 can interoperate. It is a large, new item.
- Kyber vs ML-KEM: **resolved** (see Decisions).
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

### 2026-10-23, session 2
- The image was rebuilt: preflight OK, gcc present, `just ci` (with
  `-race`) passes. The git identity is in `.git/config` (matches history).
  Fixed the stale sandbox notes.
- Milestone 2 done. Each step's vector tests were written and seen
  failing against a stub before the implementation:
  - edwards25519 field, group and scalars: RFC 8032 §7.1, RFC 7748 §5.2,
    §6.1 and 1,000 iterations, RFC 9496 A.4; cross-checked against
    `crypto/ed25519`, `crypto/ecdh` and `math/big`.
  - ristretto255: all of RFC 9496 appendix A.
  - XEd25519 and Signal key encoding: all libsignal vectors.
  - Kyber1024: 10 committed KATs; all 100 checked once through the opt-in
    `TestKyber1024FullKAT`. The ML-KEM variant matches `crypto/mlkem`.
  - Protobuf wire codec: checked against `encoding/binary`, plus a fuzz
    target.
  - Fuzz targets: `FuzzSetBytes`, `FuzzSetCanonicalBytes`, `FuzzVerify`,
    `FuzzEncapsulate`, `FuzzFields`. Each ran 15–30 s locally with no
    findings; CI runs only their seed corpora.
- Spec subagent "XEdDSA vectors, Kyber vs ML-KEM" produced
  `spec/crypto/{kem,xeddsa}.md` and two vector files (CLEANROOM.md row).
- Notes:
  - The draft-cfrg-schwabe-kyber-04 NTT example has an erratum (the
    fourth coefficient).
  - protobuf.dev isn't allowlisted, so the encoding-guide example bytes
    were written from memory; `encoding/binary` is the independent check.
- Still open in COMPAT.md's crypto table: attachment encryption, profile
  encryption and group master key derivation. They need Signal-specific
  specs and land with the milestones that use them (5, 7, 6).
- **Next (milestone 3)**: spec requests for PQXDH (key schedule, prekey
  bundle and message protobufs), the Double Ratchet as libsignal runs it,
  **SPQR** (see open questions), sender keys, sealed sender v1/v2, and
  the libsignal vectors for each.

### 2026-10-23, session 2 (continued): milestone 3 started
- Spec subagents launched: SPQR (`spec/protocol/spqr/`), and PQXDH,
  Double Ratchet, sender keys, sealed sender and capture feasibility
  (`spec/protocol/`).
- `internal/protocol`: PQXDH and the Double Ratchet core from the
  signal.org specs, behavioural tests first. The libsignal info strings,
  framing and vectors are pending the spec.
- Brief amendment (protobuf, see Decisions):
  - Done: ALLOWED_HOSTS updated; `tools/install-protoc` (protoc v36.2,
    SHA-256 pinned and verified).
  - **Blocked on the operator:** restart the egress proxy so it re-reads
    ALLOWED_HOSTS (it loads the list at startup only). Until then,
    proxy.golang.org is unreachable and `go mod vendor` can't run.
  - Remaining after the restart: vendoring, `-mod=vendor`, the repo-rule
    and CI changes, `.proto` files from `spec/`, the generation check, and
    moving `internal/proto/wire` into a differential test.
- Layer C captures need two staging numbers plus captchas from the
  operator (not yet provided).
- Proxy restarted by the operator (2026-10-23). Protobuf amendment
  completed:
  - google.golang.org/protobuf v1.36.12 vendored (go.sum only has its
    own two lines). protoc-gen-go comes from a `tool` directive.
  - `.proto` files for the wire messages, sealed sender, storage records
    and the msg-cache envelope, written from `spec/`.
  - Vector tests pass against the libsignal vectors: prekey and session
    records, decryption-error content, and sealed-sender and server
    certificates. Re-encoding is byte-exact and keeps an unknown field.
  - Differential test plus `FuzzDecodersAgree` against the old codec:
    3.4M execs, one finding. protobuf-go normalizes non-minimal tag
    varints in unknown fields, so **MACs and signatures must always be
    checked over raw received bytes**.
  - CI: single-module rule; a `go mod vendor` re-run against the proxy
    must be a no-op; protoc regeneration must be a no-op.
  - Housekeeping: commit 1dae000 also picked up a duplicate host-added
    ALLOWED_HOSTS block (the host was editing at the same moment). The
    host removed it, and the removal is committed with this change.
- Milestone 3: SPQR spec and protocol specs committed; libsignal session
  state implemented. Next: SignalMessage / PreKeySignalMessage framing on
  the generated types, then SPQR.
- Milestone 3 progress (later the same day):
  - `internal/protocol`: SignalMessage / PreKeySignalMessage framing on
    the generated types (MAC over raw bytes), and SessionRecord with
    Encrypt, Decrypt (Sesame-style, 40 archived states) and the responder
    DecryptPreKey flow of pqxdh.md §7. Record tests mutation-checked.
  - `internal/crypto/kyber`: ML-KEM-768 with SPQR's split encapsulation;
    all 100 libcrux KATs pass through both paths.
  - **Remaining for milestone 3:**
    - SPQR, in this order: varint, then the GF(2^16) erasure code
      (`chunking.md`), then the chain and authenticator (`protocol.md`
      §3–4), then the 11-state SCKA (§5–7), then the wire format
      (`wire.md`), then wiring it into Encrypt/Decrypt (SPQR key as the
      HKDF salt, `pq_ratchet` field 5, V1 minimum);
    - sender keys and sealed sender v1/v2;
    - staging captures (needs operator numbers) and the offline replay
      test.
  - Until SPQR is wired in, our sessions interoperate only with
    themselves: libsignal v0.99.1 requires SPQR V1.
- SPQR done and wired in (same day): `internal/protocol/spqr` (erasure
  code, varints, V1 messages, chain, authenticator, 11-state SCKA, top
  level) and the triple ratchet in `internal/protocol`. Our sessions now
  follow libsignal v0.99.1 end to end, from what the specs and vectors can
  show. The interop proof still needs the staging captures.
  - Known gap: self-session (own other device) unlimited jump isn't set
    yet; it needs the addresses at session creation.
  - **Remaining for milestone 3:** sender keys; sealed sender v1/v2;
    loading signal-cli's stored records (StoredSession plus SPQR state
    per `spqr/state.md`) for the capture replay; the captures themselves
    (operator numbers) and the offline replay test.
- Sender keys, sealed sender, and the stored-session loader done (same day):
  - `internal/protocol`:
    - sender keys;
    - sealed sender (certificates and trust roots, v1, v2 seal,
      SentMessage parse and fan-out, v2 unseal);
    - loaders for SessionRecord (with lazy SPQR import), prekey,
      signed-prekey and Kyber-prekey records, sender-key records, and
      msg-cache envelope files;
    - HasUsableSenderChain.
  - `internal/crypto/gcmsiv`: AES-GCM-SIV with all RFC 8452 vectors.
  - `internal/protocol/spqr`: Import/Export of libsignal's SPQR state. The
    real issue-1275 states import and finish the upstream exchange.
  - Where green-first runs couldn't be compiled against a stub, mutation
    checks showed the tests catch the targeted behaviours.
- **Waiting on the operator:** staging numbers for the captures.
  Unconfirmed allowlist need: libsignal's native library names
  `grpc.chat.staging.signal.org` and `svrb.staging.signal.org`, which
  aren't allowlisted; whether signal-cli 0.14.7 connects to them is
  unknown.
- Capture plan (from `spec/protocol/capture.md`):
  - Register two fresh reference instances on staging. The sender sends
    a set of messages (text, a group message for sender keys, sealed and
    unsealed) to a recipient that hasn't replied yet.
  - Stop the recipient and snapshot its data dir, then run one `receive`
    with `--trust-new-identities never` so the envelopes stay in
    `msg-cache/`.
  - Export the needed keys with a JVM tool over the bundled sqlite-jdbc
    driver, then scrub and store the result under `testdata/`. The Go
    replay test decrypts everything offline with the loaders above.
- **Capture-1 (staging, 2026-10-23): first interop evidence.**
  - Setup: both staging accounts were registered with the reference
    signal-cli (operator captchas and SMS codes); B enabled unrestricted
    unidentified sender; A sent 4 texts to B by phone number; B was
    snapshotted, then ran one `receive --trust-new-identities never`.
    Everything is in `.data/capture-1/` (gitignored: live staging keys
    and numbers).
  - Result: `TestCaptureReplay` (opt-in, `SIGNAL_CAPTURE_DIR`) decrypts
    all 4 envelopes offline: PQXDH with Kyber1024, the triple ratchet
    with SPQR, the MAC and the address binding, against the reference
    client.
  - Findings:
    - Addressed by number, the messages go to B's **PNI**: PNI identity
      and prekeys (`account_id_type` 1).
    - Staging fills only the **binary** service-ID envelope fields
      (`ServiceIDFromBinary` added).
    - A sent **unsealed** (type 3) despite B's unrestricted UD setting,
      so sealed sender is still unproven by capture.
    - The reference JVM with `LANG` unset turned non-ASCII arguments into
      U+FFFD, so future oracle runs must set `LANG=C.UTF-8`.
  - Not covered yet: sealed sender, SignalMessages after a reply, sender
    keys (they need at least 2 sender-key-capable group recipients, so a
    third account), and the reverse direction (milestone 4).
- Self-session flag done: same identity key and same service ID mean no
  forward-jump limit in the ratchet and SPQR (max_jump 2^32-1).
- **Milestone 3 done** (operator decision, 2026-10-23): finish offline.
  - Sealed sender, sender keys and post-reply interop are redone by
    capture once our own client talks to staging (milestone 4), not with
    a third staging number.
  - Proven by capture-1: reference → us for session-opening messages.
- **Milestone 4 started.** Spec subagents launched:
  - "service API and transport": libsignal-net vs Java WebSocket, TLS
    roots, auth, registration, prekeys, send;
  - "message pipe, content, linking": WebSocket messages, envelope
    acks, Content/DataMessage and padding, provisioning.
