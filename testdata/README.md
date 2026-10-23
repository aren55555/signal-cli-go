# testdata

- `vectors/` — Layer A test vectors. Each file cites its source (URL plus
  version or commit). Signal-specific vectors are extracted as data by a
  spec subagent; see CLEANROOM.md.
- `golden/cli`, `golden/rpc` — Layer B transcripts recorded from the reference signal-cli
  0.14.7. Scenarios live in `scenarios*.json`, recordings in `recorded/`,
  not-yet-matching scenario names in `PENDING`. Replayed offline by `go test`;
  re-record with `just record-golden`.

## Placeholders, never real identifiers

`internal/repocheck` fails the build if anything here looks like a real
phone number, captcha token, or key. Scrub recordings to these values:

| Kind          | Placeholder                                   |
|---------------|-----------------------------------------------|
| phone number  | `+12025550100` … `+12025550199` (NANP fiction range) |
| (also allowed) | `+1 555 555-01xx` (unassigned area code), e.g. `+15555550123` |
| UUID / ACI    | `00000000-0000-4000-8000-0000000000NN`        |
| captcha       | `<CAPTCHA>`                                   |
| timestamps    | see normalization rules in `internal/difftest` |

## Full Kyber1024 KATs (optional)

`vectors/kyber1024_kat.json` holds the first 10 of the 100 official
round-3 KATs. To run all 100, download NIST's `Kyber-Round3.zip` (URL and
SHA-256 in that file's `source` block), extract
`KAT/kyber1024/PQCkemKAT_3168.rsp`, and run
`KYBER1024_KAT_RSP=/path/to/PQCkemKAT_3168.rsp go test ./internal/crypto/kyber -run FullKAT`.
The test checks the file's pinned SHA-256 before using it.
