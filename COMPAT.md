# Compatibility matrix

Target: signal-cli **0.14.7**. A row is done only when every column is ✅
and links to the tests that prove it. Partial, skipped, or flaky coverage
is stated in the Notes column.

Legend: ✅ done · 🟡 partial (see notes) · ⬜ not started · n/a not applicable · deferred: out of scope for now, revisited later (not "won't do")

**Status: draft.** JSON-RPC rows are filled in from `spec/surface/jsonrpc/`; transport, error and zero-account behavior was observed on the binary. CLI rows are filled in from `spec/surface/cli/`.

Layer B goldens: all account-free behavior is recorded (CLI 355, JSON-RPC 162 scenarios, see `testdata/README.md`). A Layer B cell turns ✅ only when the scenario replays green against signal-cli-go, not when it is recorded.

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
| [`acceptCall`](spec/surface/jsonrpc/methods/acceptCall.md) | 🟡 | deferred | n/a | deferred | deferred | spec from source + zero-account daemon; not yet checked with a live account; **deferred**: voice calls, revisit after milestone 9 (PLAN.md) |
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
| [`hangupCall`](spec/surface/jsonrpc/methods/hangupCall.md) | 🟡 | deferred | n/a | deferred | deferred | spec from source + zero-account daemon; not yet checked with a live account; **deferred**: voice calls, revisit after milestone 9 (PLAN.md) |
| [`joinGroup`](spec/surface/jsonrpc/methods/joinGroup.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | spec from source + zero-account daemon; not yet checked with a live account |
| [`listAccounts`](spec/surface/jsonrpc/methods/listAccounts.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | spec from source + zero-account daemon; not yet checked with a live account |
| [`listCalls`](spec/surface/jsonrpc/methods/listCalls.md) | 🟡 | deferred | n/a | deferred | deferred | spec from source + zero-account daemon; not yet checked with a live account; **deferred**: voice calls, revisit after milestone 9 (PLAN.md) |
| [`listContacts`](spec/surface/jsonrpc/methods/listContacts.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | spec from source + zero-account daemon; not yet checked with a live account |
| [`listDevices`](spec/surface/jsonrpc/methods/listDevices.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | spec from source + zero-account daemon; not yet checked with a live account |
| [`listGroups`](spec/surface/jsonrpc/methods/listGroups.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | spec from source + zero-account daemon; not yet checked with a live account |
| [`listIdentities`](spec/surface/jsonrpc/methods/listIdentities.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | spec from source + zero-account daemon; not yet checked with a live account |
| [`listStickerPacks`](spec/surface/jsonrpc/methods/listStickerPacks.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | spec from source + zero-account daemon; not yet checked with a live account |
| [`quitGroup`](spec/surface/jsonrpc/methods/quitGroup.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | spec from source + zero-account daemon; not yet checked with a live account |
| [`receive`](spec/surface/jsonrpc/methods/receive.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | spec from source + zero-account daemon; not yet checked with a live account |
| [`register`](spec/surface/jsonrpc/methods/register.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | spec from source + zero-account daemon; not yet checked with a live account |
| [`rejectCall`](spec/surface/jsonrpc/methods/rejectCall.md) | 🟡 | deferred | n/a | deferred | deferred | spec from source + zero-account daemon; not yet checked with a live account; **deferred**: voice calls, revisit after milestone 9 (PLAN.md) |
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
| [`startCall`](spec/surface/jsonrpc/methods/startCall.md) | 🟡 | deferred | n/a | deferred | deferred | spec from source + zero-account daemon; not yet checked with a live account; **deferred**: voice calls, revisit after milestone 9 (PLAN.md) |
| [`startChangeNumber`](spec/surface/jsonrpc/methods/startChangeNumber.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | spec from source + zero-account daemon; not yet checked with a live account |
| [`startLink`](spec/surface/jsonrpc/methods/startLink.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | spec from source + zero-account daemon; not yet checked with a live account |
| [`submitRateLimitChallenge`](spec/surface/jsonrpc/methods/submitRateLimitChallenge.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | spec from source + zero-account daemon; not yet checked with a live account |
| [`subscribeCallEvents`](spec/surface/jsonrpc/methods/subscribeCallEvents.md) | 🟡 | deferred | n/a | deferred | deferred | spec from source + zero-account daemon; not yet checked with a live account; not available over HTTP (-32601); **deferred**: voice calls, revisit after milestone 9 (PLAN.md) |
| [`subscribeReceive`](spec/surface/jsonrpc/methods/subscribeReceive.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | spec from source + zero-account daemon; not yet checked with a live account; not available over HTTP (-32601) |
| [`trust`](spec/surface/jsonrpc/methods/trust.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | spec from source + zero-account daemon; not yet checked with a live account |
| [`unblock`](spec/surface/jsonrpc/methods/unblock.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | spec from source + zero-account daemon; not yet checked with a live account |
| [`unregister`](spec/surface/jsonrpc/methods/unregister.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | spec from source + zero-account daemon; not yet checked with a live account |
| [`unsubscribeCallEvents`](spec/surface/jsonrpc/methods/unsubscribeCallEvents.md) | 🟡 | deferred | n/a | deferred | deferred | spec from source + zero-account daemon; not yet checked with a live account; not available over HTTP (-32601); **deferred**: voice calls, revisit after milestone 9 (PLAN.md) |
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

Spec: [`spec/surface/cli/index.md`](spec/surface/cli/index.md). Raw observations: [`observations.md`](spec/surface/cli/observations.md).

| Feature | Spec'd | Impl | Layer A | Layer B | Layer C | Notes |
|---------|--------|------|---------|---------|---------|-------|
| `-h`, `--help` | ✅ | ⬜ | n/a | ⬜ | n/a | |
| `--version` | ✅ | ⬜ | n/a | ⬜ | n/a | |
| `-v`, `--verbose` | ✅ | ⬜ | n/a | ⬜ | n/a | |
| `--log-file` | ✅ | ⬜ | n/a | ⬜ | n/a | |
| `--scrub-log` | ✅ | ⬜ | n/a | ⬜ | n/a | |
| `-d`, `--data-dir`, `-c`, `--config` | ✅ | ⬜ | n/a | ⬜ | n/a | |
| `-a`, `--account`, `-u`, `--username` | ✅ | ⬜ | n/a | ⬜ | n/a | |
| `--dbus` | ✅ | deferred | n/a | deferred | n/a | **deferred**: D-Bus client mode, revisit after milestone 9 (PLAN.md) |
| `--dbus-system` | ✅ | deferred | n/a | deferred | n/a | **deferred**, as `--dbus` |
| `--bus-name` | ✅ | deferred | n/a | deferred | n/a | **deferred**, as `--dbus` |
| `-o`, `--output` | ✅ | ⬜ | n/a | ⬜ | n/a | |
| `--service-environment` | ✅ | ⬜ | n/a | ⬜ | n/a | |
| `--trust-new-identities` | ✅ | ⬜ | n/a | ⬜ | n/a | |
| `--disable-send-log` | ✅ | ⬜ | n/a | ⬜ | n/a | |
| Help layout (width 75 or `$COLUMNS`, fully justified; leading logging-parser block) | ✅ | ⬜ | n/a | ⬜ | n/a | goldens recorded (`help`, `*-help`, `help-columns-*`); replay pending implementation |
| Parse errors (usage + `signal-cli: error:`, `Did you mean:`, prefix abbreviation, exit 1) | ✅ | ⬜ | n/a | ⬜ | n/a | goldens recorded (`no-args`, `unknown-*`, `bad-output-value`); replay pending implementation |
| Exit codes 0–6 | 🟡 | ⬜ | n/a | ⬜ | ⬜ | 4/5/6 known from source only; 0/1 goldens recorded, replay pending |
| Config file (`SIGNAL_CLI_CONFIG`, per-user file, unknown keys rejected) | ✅ | ⬜ | n/a | ⬜ | n/a | binary differs from man page; binary is authoritative |
| Logging to stderr (`-v` levels, `--log-file`, `--scrub-log`) | 🟡 | ⬜ | n/a | ⬜ | n/a | subcommand `-v` also enables global verbose |

### Subcommands

61 subcommands. `startLink`/`finishLink` are JSON-RPC only. Success output needs a registered account and is so far known only from source.

| Command | Spec'd | Impl | Layer A | Layer B | Layer C | Notes |
|---------|--------|------|---------|---------|---------|-------|
| [`acceptCall`](spec/surface/cli/acceptCall.md) | 🟡 | deferred | n/a | deferred | deferred | from source + account-free runs; **deferred**: voice calls, revisit after milestone 9 (PLAN.md) |
| [`addDevice`](spec/surface/cli/addDevice.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | from source + account-free runs |
| [`addStickerPack`](spec/surface/cli/addStickerPack.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | from source + account-free runs |
| [`block`](spec/surface/cli/block.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | from source + account-free runs |
| [`daemon`](spec/surface/cli/daemon.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | from source + account-free runs |
| [`deleteLocalAccountData`](spec/surface/cli/deleteLocalAccountData.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | from source + account-free runs; registration command |
| [`finishChangeNumber`](spec/surface/cli/finishChangeNumber.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | from source + account-free runs |
| [`getAttachment`](spec/surface/cli/getAttachment.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | from source + account-free runs |
| [`getAvatar`](spec/surface/cli/getAvatar.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | from source + account-free runs |
| [`getSticker`](spec/surface/cli/getSticker.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | from source + account-free runs |
| [`getUserStatus`](spec/surface/cli/getUserStatus.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | from source + account-free runs |
| [`hangupCall`](spec/surface/cli/hangupCall.md) | 🟡 | deferred | n/a | deferred | deferred | from source + account-free runs; **deferred**: voice calls, revisit after milestone 9 (PLAN.md) |
| [`joinGroup`](spec/surface/cli/joinGroup.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | from source + account-free runs |
| [`jsonRpc`](spec/surface/cli/jsonRpc.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | from source + account-free runs; json only |
| [`link`](spec/surface/cli/link.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | from source + account-free runs; provisioning (-a forbidden); plain-text only |
| [`listAccounts`](spec/surface/cli/listAccounts.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | from source + account-free runs |
| [`listCalls`](spec/surface/cli/listCalls.md) | 🟡 | deferred | n/a | deferred | deferred | from source + account-free runs; **deferred**: voice calls, revisit after milestone 9 (PLAN.md) |
| [`listContacts`](spec/surface/cli/listContacts.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | from source + account-free runs |
| [`listDevices`](spec/surface/cli/listDevices.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | from source + account-free runs |
| [`listGroups`](spec/surface/cli/listGroups.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | from source + account-free runs |
| [`listIdentities`](spec/surface/cli/listIdentities.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | from source + account-free runs |
| [`listStickerPacks`](spec/surface/cli/listStickerPacks.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | from source + account-free runs |
| [`quitGroup`](spec/surface/cli/quitGroup.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | from source + account-free runs |
| [`receive`](spec/surface/cli/receive.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | from source + account-free runs |
| [`register`](spec/surface/cli/register.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | from source + account-free runs; registration command |
| [`rejectCall`](spec/surface/cli/rejectCall.md) | 🟡 | deferred | n/a | deferred | deferred | from source + account-free runs; **deferred**: voice calls, revisit after milestone 9 (PLAN.md) |
| [`remoteDelete`](spec/surface/cli/remoteDelete.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | from source + account-free runs |
| [`removeContact`](spec/surface/cli/removeContact.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | from source + account-free runs |
| [`removeDevice`](spec/surface/cli/removeDevice.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | from source + account-free runs |
| [`removePin`](spec/surface/cli/removePin.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | from source + account-free runs |
| [`send`](spec/surface/cli/send.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | from source + account-free runs |
| [`sendAdminDelete`](spec/surface/cli/sendAdminDelete.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | from source + account-free runs |
| [`sendContacts`](spec/surface/cli/sendContacts.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | from source + account-free runs |
| [`sendMessageRequestResponse`](spec/surface/cli/sendMessageRequestResponse.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | from source + account-free runs |
| [`sendPaymentNotification`](spec/surface/cli/sendPaymentNotification.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | from source + account-free runs |
| [`sendPinMessage`](spec/surface/cli/sendPinMessage.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | from source + account-free runs |
| [`sendPollCreate`](spec/surface/cli/sendPollCreate.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | from source + account-free runs |
| [`sendPollTerminate`](spec/surface/cli/sendPollTerminate.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | from source + account-free runs |
| [`sendPollVote`](spec/surface/cli/sendPollVote.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | from source + account-free runs |
| [`sendReaction`](spec/surface/cli/sendReaction.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | from source + account-free runs |
| [`sendReceipt`](spec/surface/cli/sendReceipt.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | from source + account-free runs |
| [`sendStory`](spec/surface/cli/sendStory.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | from source + account-free runs |
| [`sendSyncRequest`](spec/surface/cli/sendSyncRequest.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | from source + account-free runs |
| [`sendTyping`](spec/surface/cli/sendTyping.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | from source + account-free runs |
| [`sendUnpinMessage`](spec/surface/cli/sendUnpinMessage.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | from source + account-free runs |
| [`setPin`](spec/surface/cli/setPin.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | from source + account-free runs |
| [`startCall`](spec/surface/cli/startCall.md) | 🟡 | deferred | n/a | deferred | deferred | from source + account-free runs; **deferred**: voice calls, revisit after milestone 9 (PLAN.md) |
| [`startChangeNumber`](spec/surface/cli/startChangeNumber.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | from source + account-free runs |
| [`submitRateLimitChallenge`](spec/surface/cli/submitRateLimitChallenge.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | from source + account-free runs |
| [`trust`](spec/surface/cli/trust.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | from source + account-free runs |
| [`unblock`](spec/surface/cli/unblock.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | from source + account-free runs |
| [`unregister`](spec/surface/cli/unregister.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | from source + account-free runs |
| [`updateAccount`](spec/surface/cli/updateAccount.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | from source + account-free runs |
| [`updateConfiguration`](spec/surface/cli/updateConfiguration.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | from source + account-free runs |
| [`updateContact`](spec/surface/cli/updateContact.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | from source + account-free runs |
| [`updateDevice`](spec/surface/cli/updateDevice.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | from source + account-free runs |
| [`updateGroup`](spec/surface/cli/updateGroup.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | from source + account-free runs |
| [`updateProfile`](spec/surface/cli/updateProfile.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | from source + account-free runs |
| [`uploadStickerPack`](spec/surface/cli/uploadStickerPack.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | from source + account-free runs |
| [`verify`](spec/surface/cli/verify.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | from source + account-free runs; registration command |
| [`version`](spec/surface/cli/version.md) | 🟡 | ⬜ | n/a | ⬜ | ⬜ | from source + account-free runs |

## Protocol and interop (priority 3)

| Feature | Spec'd | Impl | Layer A | Layer B | Layer C | Notes |
|---------|--------|------|---------|---------|---------|-------|
| Registration and verification (staging) | ⬜ | ⬜ | ⬜ | ⬜ | ⬜ | |
| Linking as a secondary device (provisioning) | ⬜ | ⬜ | ⬜ | ⬜ | ⬜ | |
| Prekeys: EC, signed, Kyber last-resort | ✅ [`pqxdh.md`](spec/protocol/pqxdh.md), [`kem.md`](spec/crypto/kem.md) | 🟡 KEM framing and bundle checks in [`internal/protocol`](internal/protocol); generation and upload are milestone 4 | 🟡 Kyber KATs, framing tests | n/a | ⬜ | |
| PQXDH session setup | ✅ [`pqxdh.md`](spec/protocol/pqxdh.md) | ✅ key schedule (96-byte split), asymmetric init, PreKeySignalMessage framing, responder flow (base-key match, prekey lookup, archive), SPQR seeded from bytes 64–95, in [`internal/protocol`](internal/protocol) | 🟡 key schedule checked against its definition; no fixed upstream session vectors exist | n/a | 🟡 | Reference → signal-cli-go proven offline: capture-1 (4 PreKeySignalMessages from the reference on staging, to the PNI) all decrypt with PQXDH, SPQR and the address binding (opt-in `TestCaptureReplay`, not in CI). Reverse direction needs milestone 4. Self-session unlimited jump not yet set |
| Double Ratchet | ✅ [`double-ratchet.md`](spec/protocol/double-ratchet.md) | ✅ libsignal session state, limits, MAC over raw bytes, Sesame-style record with 40 archived states, SPQR as the message-key salt ([`internal/protocol/spqr`](internal/protocol/spqr)), in [`internal/protocol`](internal/protocol) | 🟡 `ratchet_kdf.json` (chain step, AES-CBC); behaviour tests; no fixed upstream session vectors exist | n/a | 🟡 | Capture-1 exercises the first chain only (all prekey messages; the recipient never replied). SPQR: all 11 states, erasure code, chain, ML-KEM-768 split encapsulation (100 KATs); no fixed upstream SPQR vectors exist. Reverse direction needs milestone 4 |
| Sealed sender | ✅ [`sealed-sender.md`](spec/protocol/sealed-sender.md) | ✅ certificates (known IDs, trust roots per environment, revocation, expiry at the server timestamp), v1 seal/unseal, v2 seal, SentMessage parsing and fan-out, v2 unseal, in [`internal/protocol`](internal/protocol) | 🟡 certificate vectors (3 known, 3 sender, 5 roots), all 16 SentMessage vectors including received forms, AES-CTR vector, GCM-SIV RFC 8452; no fixed upstream v1/v2 ciphertexts exist (round trips, mutation-checked) | n/a | ⬜ | Dispatching unsealed content to the session and group ciphers and the self-send check come with the receive path |
| Sender keys | ✅ [`sender-keys.md`](spec/protocol/sender-keys.md) | ✅ [`internal/protocol`](internal/protocol) | 🟡 derivation by definition, behaviour tests; no fixed upstream sender-key vectors exist | n/a | ⬜ | |
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
| edwards25519 field and group | n/a (RFC 8032) | ✅ [`internal/crypto/edwards25519`](internal/crypto/edwards25519) | ✅ RFC 8032 §7.1 (all 5 Ed25519 vectors), RFC 7748 §5.2, §6.1 and the 1,000-iteration chain, RFC 9496 A.4 sqrt-ratio; cross-checked against `crypto/ed25519`, `crypto/ecdh` and `math/big` | RFC 7748 1,000,000-iteration chain not run (minutes); `FuzzSetBytes` for strict decoding. Constant-time by construction (masks, fixed windows); Go gives no codegen guarantee, same as the stdlib |
| Ristretto255 | n/a (RFC 9496) | ✅ [`internal/crypto/ristretto255`](internal/crypto/ristretto255) | ✅ RFC 9496 A.1 (16 generator multiples via add, ScalarMult, ScalarBaseMult), A.2 (all 29 invalid encodings), A.3 (all 11 derivation vectors); section 4.1 constants checked against their identities | `FuzzSetCanonicalBytes` (round trip) |
| XEdDSA / VXEdDSA | ✅ [`spec/crypto/xeddsa.md`](spec/crypto/xeddsa.md) (libsignal deviations) | ✅ XEd25519 in [`internal/crypto/curve25519`](internal/crypto/curve25519); VXEdDSA n/a | ✅ all 68 libsignal verify vectors (4 real signatures, both sign bits, plus 64 bit-flip negatives), X25519 agreement and key derivation, 23 u-range vectors; signing checked by round trip and against `crypto/ed25519` after conversion | No upstream vector fixes Z, so signing output is pinned only through the nonce-hash test. Torsion half of the PQXDH base-key check is tested with constructed points (no upstream vectors). VXEdDSA is unused in libsignal v0.99.1. `FuzzVerify` |
| Kyber1024 (pre-standard) and/or ML-KEM-1024 | ✅ [`spec/crypto/kem.md`](spec/crypto/kem.md) | 🟡 round-3 Kyber1024 in [`internal/crypto/kyber`](internal/crypto/kyber) ✅; ML-KEM-1024 for the CDSI/SVR2 Noise handshake comes from `crypto/mlkem` (no wrapper yet); ML-KEM-768 for SPQR ⬜ | ✅ Kyber1024: 10 committed NIST round-3 KATs, all 100 run once via the opt-in `TestKyber1024FullKAT` (pinned SHA-256); the shared core's FIPS 203 variant cross-checked against `crypto/mlkem` 768 and 1024; NTT checked against the draft's matrix definition and schoolbook multiplication | PQXDH uses only `0x08` Kyber1024 (the libsignal type-byte framing comes in milestone 3). SPQR needs ML-KEM-768 via libcrux's incremental API: **needs its own spec**. Implicit rejection follows the Kyber spec; libcrux differs, with no interop impact (see kem.md). `FuzzEncapsulate` |
| AES-256-GCM-SIV (sealed sender v2) | n/a (RFC 8452) | ✅ [`internal/crypto/gcmsiv`](internal/crypto/gcmsiv) | ✅ RFC 8452 appendix C: all 50 vectors (C.1, C.2, C.3 counter wrap): derived keys, POLYVAL, seal, open, tamper | Not in the standard library. Constant-time POLYVAL by masks |
| Protobuf (generated types, `google.golang.org/protobuf`, vendored) | ✅ `.proto` files from `spec/` | ✅ [`internal/proto/signalpb`](internal/proto/signalpb) | ✅ libsignal record and certificate vectors parse, and re-encode byte-exact (unknown fields kept); differential test against the old hand-written codec; `FuzzDecodersAgree`; CI regeneration check | Brief amendment 2026-10-23. Old codec kept only as a test oracle. protobuf-go normalizes non-minimal tags in unknown fields, so MACs are checked over raw bytes |
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
