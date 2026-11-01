# signal-cli-go

signal-cli-go is a reimplementation of [signal-cli](https://github.com/AsamK/signal-cli)
in Go. It aims to be a **drop-in replacement for signal-cli 0.14.7**: the same
command line, output, exit codes and JSON-RPC daemon (HTTP with server-sent events,
UNIX socket, TCP, and `jsonRpc` on stdio). Its behaviour is checked against the
real signal-cli 0.14.7, using recorded transcripts and live differential runs on
Signal's staging servers.

The main binary uses only the Go standard library and the vendored
`google.golang.org/protobuf`, with no C code. A separate import tool also links the
pure-Go `modernc.org/sqlite` to read signal-cli's database.

> **Status: production supported — see [COMPAT.md#production](COMPAT.md#production).**
> Like signal-cli, signal-cli-go uses Signal's production service by default
> (`--service-environment live`); `--service-environment staging` selects the staging
> servers. Every row of COMPAT.md's Production section was proven live on production
> with throwaway accounts and the official iOS app: registration, linking as a secondary
> device, contact discovery, 1:1 and group messaging with mentions, quotes, images,
> reactions and typing, a first message from a stranger, the `daemon --http` event stream, importing an account
> registered with signal-cli 0.14.7, and the JSON-RPC daemon side by side with signal-cli
> 0.14.7. Read [Known gaps](#known-gaps) (including the production gaps) and
> [`docs/cutover.md`](docs/cutover.md) before moving a real account.

## Status

[`COMPAT.md`](COMPAT.md) has the feature-by-feature matrix and
[`PLAN.md`](PLAN.md) the milestones and progress log. In short:

- 1:1 and group (v2) messaging with every content type, attachments, stories,
  sealed sender, sender keys and group-send endorsements interoperate with the
  reference signal-cli on staging, in both directions.
- Linked devices and sync, profiles, contacts, usernames, the storage service,
  contact discovery (CDSI) and PINs (SVR2) work too.
- The JSON-RPC daemon replays every recorded signal-cli transcript identically.
  Every CLI command and output mode matches the recorded transcripts.
- Existing signal-cli 0.14.7 accounts can be imported (see
  [Migrating from signal-cli](#migrating-from-signal-cli)).
- Production: supported — see [COMPAT.md#production](COMPAT.md#production). Built in are
  the service values of signal-cli 0.14.7's production configuration, tested against
  the spec, and every production row was proven live: messaging with an official Signal
  app (including a stranger's first message), import of a production account, and all
  21 JSON-RPC methods a typical bridge uses, plus the SSE event stream, compared side by
  side with signal-cli 0.14.7. The linux/arm64 build was run on an arm64 host.

## Install

There are no release binaries yet. Build from source with Go 1.27 or later:

    git clone https://github.com/aren55555/signal-cli-go
    cd signal-cli-go
    CGO_ENABLED=0 go build -o signal-cli-go ./cmd/signal-cli-go
    CGO_ENABLED=0 go build -o signal-cli-go-import ./cmd/signal-cli-go-import   # optional, for migrating

The builds use only the vendored modules (`-mod=vendor` is the default when a
`vendor/` directory is present) and produce static binaries.

Then use it like signal-cli, for example:

    ./signal-cli-go -a +15555550100 register --captcha 'signalcaptcha://…'
    ./signal-cli-go -a +15555550100 verify 123456
    ./signal-cli-go -a +15555550100 send -m "hello" +15555550101
    ./signal-cli-go -a +15555550100 receive

(add `--service-environment staging` to use Signal's staging servers instead)

or link it to an existing account as a secondary device with
`./signal-cli-go link -n my-device`. The data
directory defaults to `$XDG_DATA_HOME/signal-cli` (or
`~/.local/share/signal-cli`), as for signal-cli; `-d DIR` chooses another.
signal-cli-go shares signal-cli's account index (`data/accounts.json`) but keeps
each account's data in its own format, next to signal-cli's files. It doesn't
read signal-cli's account files or database; migrate those with the importer.

### Building for linux/arm64 (Raspberry Pi)

    just build-arm64        # dist/linux-arm64/signal-cli-go and signal-cli-go-import, with SHA-256 sums

or by hand, with exactly Go 1.27.3:

    GOTOOLCHAIN=go1.27.3 CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -o signal-cli-go ./cmd/signal-cli-go
    GOTOOLCHAIN=go1.27.3 CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -o signal-cli-go-import ./cmd/signal-cli-go-import

The output is a static binary with no dependencies.

The build is **byte-reproducible**: the same commit built from a clean checkout
with Go 1.27.3 gives identical files, whatever the checkout path, host or build
cache (checked on two paths and with a cold cache). Go stamps the commit and a
"modified" flag into the binary, so a checkout with uncommitted changes builds a
different file.

`just check-arm64` (part of `just ci`) runs `go vet` for arm64 and compiles every
test package for it. Running the tests needs an arm64 machine. On a Mac with an
arm64 Docker host (Colima's VM is arm64 natively), from the repository root:

    docker run --rm --platform linux/arm64 --network none -v "$PWD":/src:ro -w /src \
      --user 1000:1000 -e HOME=/tmp -e GOCACHE=/tmp/gocache \
      -e GOFLAGS=-mod=vendor -e CGO_ENABLED=0 -e GOTOOLCHAIN=local \
      golang:1.27.3 go test ./...

Run the tests as a normal user, as above. As root, one recorded case (`-d` under
`/nonexistent`) would behave differently, because root can create directories in
`/`. It is skipped with a message when the tests run as root.

and to smoke-test the release binaries themselves:

    docker run --rm --platform linux/arm64 -v "$PWD/dist/linux-arm64":/opt/sg:ro \
      alpine:3 /opt/sg/signal-cli-go --version

## Migrating from signal-cli

For a real account, follow [`docs/cutover.md`](docs/cutover.md) (backup, checks, and
what going back to the JVM signal-cli costs).

`signal-cli-go-import` copies accounts from a signal-cli 0.14.7 data directory into
a signal-cli-go data directory, keeping the identity keys, prekeys, sessions,
sender keys, contacts, groups, sticker packs and settings, so the account keeps
working without being linked or registered again:

    # stop signal-cli first (the importer refuses an account that is in use)
    ./signal-cli-go-import -from ~/.local/share/signal-cli -to ~/signal-cli-go-data [-a +15555550100]
    ./signal-cli-go -d ~/signal-cli-go-data -a +15555550100 receive

- The source directory is never modified. The database is copied aside before it is
  read.
- It needs account file version 10 and database schema 30. For older ones, run any
  account command once with signal-cli 0.14.7, which migrates them, then import.
- Not carried over: v1 groups (their storage IDs are kept), the username link
  colour, and cached envelopes in pre-0.14.7 formats. The importer lists what it
  skipped for each account.
- Don't run signal-cli and signal-cli-go on the same account afterwards. Both
  would act as the same device.

## The daemon

As with signal-cli, `daemon` serves JSON-RPC (see signal-cli's
`signal-cli-jsonrpc(5)`; the method set, parameters and results are the same):

    ./signal-cli-go --config ~/signal-cli-go-data -a +15555550100 daemon --http=127.0.0.1:8080 --receive-mode=on-start
    ./signal-cli-go daemon --socket /run/user/1000/signal-cli.socket   # all accounts
    ./signal-cli-go -a +15555550100 daemon --tcp 127.0.0.1:7583
    ./signal-cli-go -a +15555550100 jsonRpc                          # stdio

With `--http`, requests go to `POST /api/v1/rpc` and incoming messages stream from
`GET /api/v1/events` as server-sent events. With `--socket` and `--tcp` they arrive
as JSON-RPC notifications. D-Bus (`--dbus`, `--dbus-system`) is not supported.

## Known gaps

[`COMPAT.md`](COMPAT.md) lists every row that isn't fully green and why. The main
gaps:

- **Production gaps** (see [COMPAT.md#production](COMPAT.md#production)):
  - The rate-limit challenge flow (a send answered with "proof required" and a captcha,
    `submitRateLimitChallenge`) has never been seen live, on production or staging. It is
    tested offline only. Production rate limits are stricter than staging's.
  - Change number and registration-lock (PIN) recovery are tested offline only.
  - The live runs used throwaway accounts with a few contacts and groups and daemon runs
    of up to about an hour. Large accounts, high message rates and multi-day uptime were
    not tested live.
  - Side by side, signal-cli-go as a primary sent its linked device two more empty sync
    messages than signal-cli did. The event JSON was identical; the cause is not yet
    known.
  - The error text of `verify` after the registration session has expired
    (`Verify error: rest: status 404`) was not compared with signal-cli's.
- **Going back to the JVM signal-cli** after signal-cli-go has run an account
  needs care; see [`docs/cutover.md`](docs/cutover.md).
- **Voice calls** (`startCall`, `acceptCall` and friends, call events) are
  deferred. With no active call the commands behave like signal-cli's.
- **D-Bus** client and daemon modes are deferred.
- A few paths are tested offline only because staging can't exercise them
  safely: change number, the rate-limit challenge, and registration lock
  recovery.
- **Deliberate deviations from 0.14.7** are listed in COMPAT.md with the reason
  for each. For example, signal-cli-go resends group messages in answer to retry
  receipts, which signal-cli 0.14.7 never does.

## Development

`just ci` runs what CI runs: formatting, the dependency and no-C checks, vet,
tests with the race detector, the protobuf drift check and a secret scan of the
tree and of every commit in history (`internal/repocheck`). See
[`CONTRIBUTING.md`](CONTRIBUTING.md) before sending changes, and
[`SECURITY.md`](SECURITY.md) to report a vulnerability.

## How it was built

The code was written with a clean-room process. Separate spec authors read the
upstream projects and wrote prose specifications (in [`spec/`](spec/)). The
implementation was then written only from those specs, public standards and
black-box observation of the signal-cli binary. [`CLEANROOM.md`](CLEANROOM.md)
records every spec, its sources and the process's limits: it reduces the risk of
copying but cannot certify independence.

## License

GPL-3.0-or-later, the same license as signal-cli. See [`LICENSE`](LICENSE). The
vendored modules keep their own licenses (BSD-3-Clause and MIT; SQLite is public
domain), listed in [`CLEANROOM.md`](CLEANROOM.md).
