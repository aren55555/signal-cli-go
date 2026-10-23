# Compatibility matrix

Target: signal-cli **0.14.7**. A row is done only when every column is ✅
and links to the tests that prove it. Partial, skipped, or flaky coverage
is stated in the Notes column.

Legend: ✅ done · 🟡 partial (see notes) · ⬜ not started · n/a not applicable

**Status: draft.** JSON-RPC rows are filled in from `spec/surface/jsonrpc/`; transport, error and zero-account behavior was observed on the binary. CLI rows are pending the CLI inventory.

## JSON-RPC daemon (priority 1)

### Transports and endpoints

Spec: [`transports.md`](spec/surface/jsonrpc/transports.md), [`errors.md`](spec/surface/jsonrpc/errors.md), [`notifications.md`](spec/surface/jsonrpc/notifications.md). Raw observations: [`observations.md`](spec/surface/jsonrpc/observations.md).

| Feature | Spec'd | Impl | Layer A | Layer B | Layer C | Notes |
|---------|--------|------|---------|---------|---------|-------|
| `daemon --http`: `POST /api/v1/rpc` | ✅ | ⬜ | ⬜ | ⬜ | ⬜ | |
| `daemon --http`: SSE `GET /api/v1/events` | ✅ | ⬜ | ⬜ | ⬜ | ⬜ | |
| `daemon --http`: `GET /api/v1/check` | ✅ | ⬜ | ⬜ | ⬜ | ⬜ | |
| `daemon --socket` | ✅ | ⬜ | ⬜ | ⬜ | ⬜ | |
| `daemon --tcp` | ✅ | ⬜ | ⬜ | ⬜ | ⬜ | |
| `daemon --dbus` | n/a | n/a | n/a | n/a | n/a | excluded by PROMPT.md |
| Malformed JSON / invalid request errors | ✅ | ⬜ | ⬜ | ⬜ | n/a | |
| Unknown method error | ✅ | ⬜ | ⬜ | ⬜ | n/a | |

### Methods

64 methods (spec: [`spec/surface/jsonrpc/methods/index.md`](spec/surface/jsonrpc/methods/index.md)). Layer A is n/a for methods: they are covered by Layer B and C.

| Method | Spec'd | Impl | Layer A | Layer B | Layer C | Notes |
|--------|--------|------|---------|---------|---------|-------|


## CLI (priority 2)

### Global flags and output modes

_Rows pending the milestone 1 inventory._

### Subcommands

_Rows pending the milestone 1 inventory._

## Protocol and interop (priority 3)

| Feature | Spec'd | Impl | Layer A | Layer B | Layer C | Notes |
|---------|--------|------|---------|---------|---------|-------|
| Registration and verification (staging) | ⬜ | ⬜ | ⬜ | ⬜ | ⬜ | |
| Linking as a secondary device (provisioning) | ⬜ | ⬜ | ⬜ | ⬜ | ⬜ | |
| Prekeys: EC, signed, Kyber last-resort | ⬜ | ⬜ | ⬜ | n/a | ⬜ | Kyber vs ML-KEM: open question |
| PQXDH session setup | ⬜ | ⬜ | ⬜ | n/a | ⬜ | |
| Double Ratchet | ⬜ | ⬜ | ⬜ | n/a | ⬜ | |
| Sealed sender | ⬜ | ⬜ | ⬜ | n/a | ⬜ | |
| Sender keys | ⬜ | ⬜ | ⬜ | n/a | ⬜ | |
| 1:1 text | ⬜ | ⬜ | ⬜ | ⬜ | ⬜ | |
| Attachments (send and receive) | ⬜ | ⬜ | ⬜ | ⬜ | ⬜ | |
| Reactions | ⬜ | ⬜ | ⬜ | ⬜ | ⬜ | |
| Quotes | ⬜ | ⬜ | ⬜ | ⬜ | ⬜ | |
| Mentions | ⬜ | ⬜ | ⬜ | ⬜ | ⬜ | |
| Edits | ⬜ | ⬜ | ⬜ | ⬜ | ⬜ | |
| Remote deletes | ⬜ | ⬜ | ⬜ | ⬜ | ⬜ | |
| Read and delivery receipts | ⬜ | ⬜ | ⬜ | ⬜ | ⬜ | |
| Typing indicators | ⬜ | ⬜ | ⬜ | ⬜ | ⬜ | |
| Stories | ⬜ | ⬜ | ⬜ | ⬜ | ⬜ | |
| Groups v2: create, update, join, leave | ⬜ | ⬜ | ⬜ | ⬜ | ⬜ | |
| zkgroup credentials and proofs | ⬜ | ⬜ | ⬜ | n/a | ⬜ | |
| Sync messages to linked devices | ⬜ | ⬜ | ⬜ | ⬜ | ⬜ | |
| Safety number change and trust | ⬜ | ⬜ | ⬜ | ⬜ | ⬜ | |
| Session recovery after reinstall | ⬜ | ⬜ | n/a | n/a | ⬜ | |
| Profiles (encryption, fetch, set) | ⬜ | ⬜ | ⬜ | ⬜ | ⬜ | |
| Usernames | ⬜ | ⬜ | ⬜ | ⬜ | ⬜ | |
| Storage service | ⬜ | ⬜ | ⬜ | ⬜ | ⬜ | |
| CDSI contact discovery | ⬜ | ⬜ | ⬜ | n/a | ⬜ | enclave attestation |
| SVR2 (PIN, registration lock) | ⬜ | ⬜ | ⬜ | n/a | ⬜ | enclave attestation |
| WebSocket client (RFC 6455, via CONNECT proxy) | ⬜ | ⬜ | ⬜ | n/a | ⬜ | |

## Crypto primitives (Layer A)

| Primitive | Spec'd | Impl | Layer A | Notes |
|-----------|--------|------|---------|-------|
| edwards25519 field and group | n/a (RFC 8032) | ⬜ | ⬜ | |
| Ristretto255 | n/a (RFC 9496) | ⬜ | ⬜ | |
| XEdDSA / VXEdDSA | n/a (signal.org/docs) | ⬜ | ⬜ | |
| Kyber1024 (pre-standard) and/or ML-KEM-1024 | ⬜ | ⬜ | ⬜ | |
| Protobuf wire codec | n/a | ⬜ | ⬜ | fuzz test required |
| Attachment encryption | ⬜ | ⬜ | ⬜ | |
| Profile encryption | ⬜ | ⬜ | ⬜ | |
| Group master key derivation | ⬜ | ⬜ | ⬜ | |

## Storage

| Feature | Spec'd | Impl | Tests | Notes |
|---------|--------|------|-------|-------|
| Crash-safe file store (temp, fsync, rename) | ⬜ | ⬜ | ⬜ | |
| Import of a signal-cli data dir (stretch goal) | ⬜ | ⬜ | ⬜ | needs an in-repo SQLite reader |

## Repository rules

| Rule | Test | Status |
|------|------|--------|
| go.mod has no `require` lines, and there is no go.sum | `internal/repocheck` `TestGoModHasNoRequires`; CI step | ✅ |
| No phone numbers, captchas, or keys in `testdata/` | `internal/repocheck` `TestTestdataHasNoSecrets` | ✅ |
| Oracle download verified (SHA-256 and signature) | `tools/install-oracle` (runtime check) + extraction tests | ✅ |
