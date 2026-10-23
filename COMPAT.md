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
| [`acceptCall`](spec/surface/jsonrpc/methods/acceptCall.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | spec from source + zero-account daemon; not yet checked with a live account; **calls: scope question, see PLAN.md** |
| [`addDevice`](spec/surface/jsonrpc/methods/addDevice.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | spec from source + zero-account daemon; not yet checked with a live account |
| [`addStickerPack`](spec/surface/jsonrpc/methods/addStickerPack.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | spec from source + zero-account daemon; not yet checked with a live account |
| [`block`](spec/surface/jsonrpc/methods/block.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | spec from source + zero-account daemon; not yet checked with a live account |
| [`deleteLocalAccountData`](spec/surface/jsonrpc/methods/deleteLocalAccountData.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | spec from source + zero-account daemon; not yet checked with a live account |
| [`finishChangeNumber`](spec/surface/jsonrpc/methods/finishChangeNumber.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | spec from source + zero-account daemon; not yet checked with a live account |
| [`finishLink`](spec/surface/jsonrpc/methods/finishLink.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | spec from source + zero-account daemon; not yet checked with a live account |
| [`getAttachment`](spec/surface/jsonrpc/methods/getAttachment.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | spec from source + zero-account daemon; not yet checked with a live account |
| [`getAvatar`](spec/surface/jsonrpc/methods/getAvatar.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | spec from source + zero-account daemon; not yet checked with a live account |
| [`getSticker`](spec/surface/jsonrpc/methods/getSticker.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | spec from source + zero-account daemon; not yet checked with a live account |
| [`getUserStatus`](spec/surface/jsonrpc/methods/getUserStatus.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | spec from source + zero-account daemon; not yet checked with a live account |
| [`hangupCall`](spec/surface/jsonrpc/methods/hangupCall.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | spec from source + zero-account daemon; not yet checked with a live account; **calls: scope question, see PLAN.md** |
| [`joinGroup`](spec/surface/jsonrpc/methods/joinGroup.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | spec from source + zero-account daemon; not yet checked with a live account |
| [`listAccounts`](spec/surface/jsonrpc/methods/listAccounts.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | spec from source + zero-account daemon; not yet checked with a live account |
| [`listCalls`](spec/surface/jsonrpc/methods/listCalls.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | spec from source + zero-account daemon; not yet checked with a live account; **calls: scope question, see PLAN.md** |
| [`listContacts`](spec/surface/jsonrpc/methods/listContacts.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | spec from source + zero-account daemon; not yet checked with a live account |
| [`listDevices`](spec/surface/jsonrpc/methods/listDevices.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | spec from source + zero-account daemon; not yet checked with a live account |
| [`listGroups`](spec/surface/jsonrpc/methods/listGroups.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | spec from source + zero-account daemon; not yet checked with a live account |
| [`listIdentities`](spec/surface/jsonrpc/methods/listIdentities.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | spec from source + zero-account daemon; not yet checked with a live account |
| [`listStickerPacks`](spec/surface/jsonrpc/methods/listStickerPacks.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | spec from source + zero-account daemon; not yet checked with a live account |
| [`quitGroup`](spec/surface/jsonrpc/methods/quitGroup.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | spec from source + zero-account daemon; not yet checked with a live account |
| [`receive`](spec/surface/jsonrpc/methods/receive.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | spec from source + zero-account daemon; not yet checked with a live account |
| [`register`](spec/surface/jsonrpc/methods/register.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | spec from source + zero-account daemon; not yet checked with a live account |
| [`rejectCall`](spec/surface/jsonrpc/methods/rejectCall.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | spec from source + zero-account daemon; not yet checked with a live account; **calls: scope question, see PLAN.md** |
| [`remoteDelete`](spec/surface/jsonrpc/methods/remoteDelete.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | spec from source + zero-account daemon; not yet checked with a live account |
| [`removeContact`](spec/surface/jsonrpc/methods/removeContact.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | spec from source + zero-account daemon; not yet checked with a live account |
| [`removeDevice`](spec/surface/jsonrpc/methods/removeDevice.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | spec from source + zero-account daemon; not yet checked with a live account |
| [`removePin`](spec/surface/jsonrpc/methods/removePin.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | spec from source + zero-account daemon; not yet checked with a live account |
| [`send`](spec/surface/jsonrpc/methods/send.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | spec from source + zero-account daemon; not yet checked with a live account |
| [`sendAdminDelete`](spec/surface/jsonrpc/methods/sendAdminDelete.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | spec from source + zero-account daemon; not yet checked with a live account |
| [`sendContacts`](spec/surface/jsonrpc/methods/sendContacts.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | spec from source + zero-account daemon; not yet checked with a live account |
| [`sendMessageRequestResponse`](spec/surface/jsonrpc/methods/sendMessageRequestResponse.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | spec from source + zero-account daemon; not yet checked with a live account |
| [`sendPaymentNotification`](spec/surface/jsonrpc/methods/sendPaymentNotification.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | spec from source + zero-account daemon; not yet checked with a live account |
| [`sendPinMessage`](spec/surface/jsonrpc/methods/sendPinMessage.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | spec from source + zero-account daemon; not yet checked with a live account |
| [`sendPollCreate`](spec/surface/jsonrpc/methods/sendPollCreate.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | spec from source + zero-account daemon; not yet checked with a live account |
| [`sendPollTerminate`](spec/surface/jsonrpc/methods/sendPollTerminate.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | spec from source + zero-account daemon; not yet checked with a live account |
| [`sendPollVote`](spec/surface/jsonrpc/methods/sendPollVote.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | spec from source + zero-account daemon; not yet checked with a live account |
| [`sendReaction`](spec/surface/jsonrpc/methods/sendReaction.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | spec from source + zero-account daemon; not yet checked with a live account |
| [`sendReceipt`](spec/surface/jsonrpc/methods/sendReceipt.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | spec from source + zero-account daemon; not yet checked with a live account |
| [`sendStory`](spec/surface/jsonrpc/methods/sendStory.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | spec from source + zero-account daemon; not yet checked with a live account |
| [`sendSyncRequest`](spec/surface/jsonrpc/methods/sendSyncRequest.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | spec from source + zero-account daemon; not yet checked with a live account |
| [`sendTyping`](spec/surface/jsonrpc/methods/sendTyping.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | spec from source + zero-account daemon; not yet checked with a live account |
| [`sendUnpinMessage`](spec/surface/jsonrpc/methods/sendUnpinMessage.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | spec from source + zero-account daemon; not yet checked with a live account |
| [`setPin`](spec/surface/jsonrpc/methods/setPin.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | spec from source + zero-account daemon; not yet checked with a live account |
| [`startCall`](spec/surface/jsonrpc/methods/startCall.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | spec from source + zero-account daemon; not yet checked with a live account; **calls: scope question, see PLAN.md** |
| [`startChangeNumber`](spec/surface/jsonrpc/methods/startChangeNumber.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | spec from source + zero-account daemon; not yet checked with a live account |
| [`startLink`](spec/surface/jsonrpc/methods/startLink.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | spec from source + zero-account daemon; not yet checked with a live account |
| [`submitRateLimitChallenge`](spec/surface/jsonrpc/methods/submitRateLimitChallenge.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | spec from source + zero-account daemon; not yet checked with a live account |
| [`subscribeCallEvents`](spec/surface/jsonrpc/methods/subscribeCallEvents.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | spec from source + zero-account daemon; not yet checked with a live account; not available over HTTP (-32601); **calls: scope question, see PLAN.md** |
| [`subscribeReceive`](spec/surface/jsonrpc/methods/subscribeReceive.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | spec from source + zero-account daemon; not yet checked with a live account; not available over HTTP (-32601) |
| [`trust`](spec/surface/jsonrpc/methods/trust.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | spec from source + zero-account daemon; not yet checked with a live account |
| [`unblock`](spec/surface/jsonrpc/methods/unblock.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | spec from source + zero-account daemon; not yet checked with a live account |
| [`unregister`](spec/surface/jsonrpc/methods/unregister.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | spec from source + zero-account daemon; not yet checked with a live account |
| [`unsubscribeCallEvents`](spec/surface/jsonrpc/methods/unsubscribeCallEvents.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | spec from source + zero-account daemon; not yet checked with a live account; not available over HTTP (-32601); **calls: scope question, see PLAN.md** |
| [`unsubscribeReceive`](spec/surface/jsonrpc/methods/unsubscribeReceive.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | spec from source + zero-account daemon; not yet checked with a live account; not available over HTTP (-32601) |
| [`updateAccount`](spec/surface/jsonrpc/methods/updateAccount.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | spec from source + zero-account daemon; not yet checked with a live account |
| [`updateConfiguration`](spec/surface/jsonrpc/methods/updateConfiguration.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | spec from source + zero-account daemon; not yet checked with a live account |
| [`updateContact`](spec/surface/jsonrpc/methods/updateContact.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | spec from source + zero-account daemon; not yet checked with a live account |
| [`updateDevice`](spec/surface/jsonrpc/methods/updateDevice.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | spec from source + zero-account daemon; not yet checked with a live account |
| [`updateGroup`](spec/surface/jsonrpc/methods/updateGroup.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | spec from source + zero-account daemon; not yet checked with a live account |
| [`updateProfile`](spec/surface/jsonrpc/methods/updateProfile.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | spec from source + zero-account daemon; not yet checked with a live account |
| [`uploadStickerPack`](spec/surface/jsonrpc/methods/uploadStickerPack.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | spec from source + zero-account daemon; not yet checked with a live account |
| [`verify`](spec/surface/jsonrpc/methods/verify.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | spec from source + zero-account daemon; not yet checked with a live account |
| [`version`](spec/surface/jsonrpc/methods/version.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | spec from source + zero-account daemon; not yet checked with a live account |

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
