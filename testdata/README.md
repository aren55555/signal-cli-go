# testdata

- `vectors/` — Layer A test vectors. Each file cites its source (URL plus
  version or commit). Signal-specific vectors are extracted as data by a
  spec subagent; see CLEANROOM.md.
- `golden/` — Layer B transcripts recorded from the reference signal-cli
  0.14.7. Replayed offline by `go test`.

## Placeholders, never real identifiers

`internal/repocheck` fails the build if anything here looks like a real
phone number, captcha token, or key. Scrub recordings to these values:

| Kind          | Placeholder                                   |
|---------------|-----------------------------------------------|
| phone number  | `+12025550100` … `+12025550199` (NANP fiction range) |
| UUID / ACI    | `00000000-0000-4000-8000-0000000000NN`        |
| captcha       | `<CAPTCHA>`                                   |
| timestamps    | see normalization rules in `internal/difftest` |
