# Contributing to signal-cli-go

Thanks for your interest. signal-cli-go is a clean-room reimplementation of
signal-cli, so a few rules here are stricter than in most projects. Please read
them before writing code.

## The clean-room rule

The implementation must not be derived from signal-cli's, libsignal's or
Signal-Server's source code. [`CLEANROOM.md`](CLEANROOM.md) describes the process
in full. For contributors this means:

- **When you write or change Go code, work only from** the prose specifications in
  [`spec/`](spec/), public standards (RFCs, FIPS, Signal's published specifications
  at signal.org/docs), and black-box observation of the signal-cli binary (running
  it and recording what it does).
- **Don't read upstream source to write or fix code,** and don't paste or
  paraphrase it from memory. If a spec is missing something, open an issue asking
  for a spec addition instead.
- **Spec work is separate.** Specs are written by someone who may read upstream
  source but writes only prose: tables, field layouts, behaviour descriptions.
  That means no code, no pseudocode that mirrors upstream structure and no copied
  comments. Every spec cites its sources (URL and tag or commit), and each one
  gets a row in the log in `CLEANROOM.md`. Don't write a spec and the code that
  implements it in the same change, unless you only worked from public standards
  and black-box observation.
- Say in your pull request which specs or observations your change is based on.

## Ground rules for changes

- **License:** contributions are accepted under GPL-3.0-or-later. Every Go file
  you write starts with `// SPDX-License-Identifier: GPL-3.0-or-later` (CI checks
  this).
- **Dependencies:** the main binary uses the Go standard library and the vendored
  `google.golang.org/protobuf` only. The importer may also use `modernc.org/sqlite`.
  Adding or upgrading a module is a project decision (see PLAN.md, Decisions), and
  the repo checks enforce the exact list. Write what's missing under `internal/`,
  with test vectors.
- **No C:** every build uses `CGO_ENABLED=0`, and CI checks that the shipped
  binaries are static and contain no C. Only the race-detector test run enables
  cgo.
- **Compatibility is proven, not asserted:** behaviour visible to users (CLI
  output, exit codes, JSON-RPC, data on the wire) must match signal-cli 0.14.7. Add
  or update a test: unit tests with vectors (Layer A), recorded golden transcripts
  (Layer B, see [`testdata/README.md`](testdata/README.md)), or a live staging run
  (Layer C). Update [`COMPAT.md`](COMPAT.md) when a row changes. A deliberate
  deviation from 0.14.7 needs a reason, recorded in COMPAT.md.
- **Live tests use staging only:** always pass `--service-environment staging`.
  Never commit phone numbers, keys, captcha tokens, verification codes or
  transcripts with real identifiers. Test data uses the fictional range
  +1 NPA 555-0100…0199. Live data belongs in the gitignored `.data/` directory.
  CI runs gitleaks over the whole history; see `.gitleaks.toml` for the reviewed
  exceptions.

## Workflow

1. `just ci` must pass. It runs formatting, the dependency and no-C checks, vet,
   tests with the race detector, the protobuf drift check and the secret scan.
2. Keep changes focused, and explain in the commit message what changed and why.
3. Generated code (`internal/proto/signalpb/*.pb.go`) comes from the `.proto` files
   via `just proto-gen`. Edit the `.proto`, never the generated file.
4. Don't reformat `vendor/`. It must stay identical to the checksum-verified
   modules.

## Reporting bugs

Open an issue with the command or JSON-RPC request, the output you got, and what
signal-cli 0.14.7 does in the same situation if you can check. For security
issues, follow [`SECURITY.md`](SECURITY.md) instead.
