# signal-cli-go

signal-cli-go is a reimplementation of [signal-cli](https://github.com/AsamK/signal-cli)
in Go, using only the standard library plus the vendored
`google.golang.org/protobuf`. (A separate importer command also uses the pure-Go
`modernc.org/sqlite`; see below.) It aims to be a **drop-in replacement for signal-cli
0.14.7**: the same command line, output, exit codes, data directory and JSON-RPC
daemon (HTTP with SSE events, UNIX socket, TCP, and `jsonRpc` on stdio). Behaviour
is checked against the real signal-cli 0.14.7 with recorded transcripts and live
differential runs.

## Status

Work in progress. [`COMPAT.md`](COMPAT.md) has the feature-by-feature matrix, and
[`PLAN.md`](PLAN.md) lists the milestones and the progress log. In short:

- Messaging (1:1 and groups v2), sync with linked devices, profiles, contacts,
  usernames, storage service, contact discovery (CDSI) and PINs (SVR2) work
  against Signal's staging servers.
- The JSON-RPC daemon replays every recorded signal-cli transcript identically.
- Existing signal-cli 0.14.7 accounts can be imported with
  `signal-cli-go-import` (below).
- Not yet: the production (`live`) service environment, voice calls, D-Bus
  (out of scope), and some CLI details (see COMPAT.md).

Only `--service-environment staging` is supported for now.

## Building

    CGO_ENABLED=0 go build -o signal-cli-go ./cmd/signal-cli-go
    ./signal-cli-go --service-environment staging --help

Builds use only vendored modules and no C (`CGO_ENABLED=0`, checked by
`internal/repocheck`).

## Importing a signal-cli data directory

    CGO_ENABLED=0 go build -o signal-cli-go-import ./cmd/signal-cli-go-import
    ./signal-cli-go-import -from ~/.local/share/signal-cli -to NEW_DATA_DIR [-a NUMBER]

Stop signal-cli first. The importer reads signal-cli 0.14.7's account files and
`account.db` (account file version 10, database schema 30; run signal-cli 0.14.7
once to migrate older ones) and writes signal-cli-go accounts into a separate data
directory, which you then pass to signal-cli-go with `-d`. The source directory is
never modified. This is the only binary that links a SQLite driver.

`just ci` runs the checks CI runs (formatting, vet, tests with the race
detector, protobuf drift).

## License

GPL-3.0-or-later, the same license as signal-cli. See [`LICENSE`](LICENSE).

## How it was built

The code was written with a clean-room process: separate spec authors read the
upstream projects and wrote prose specifications (in [`spec/`](spec/)), and the
implementation was written from those specs, public standards and black-box
observation of the signal-cli binary. [`CLEANROOM.md`](CLEANROOM.md) describes the
process and its limits.
