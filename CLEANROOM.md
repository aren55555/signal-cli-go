# Clean-room log

signal-cli is GPLv3; libsignal and Signal-Server are AGPLv3. This project
aims to be an independent implementation that copies none of their code.

## Process

- **Spec role** (subagents only): may read upstream sources, published
  docs, and the running signal-cli binary. Writes **prose** specifications
  to `spec/` and nothing else: no code, no pseudocode that mirrors
  upstream structure, no copied comments. Every spec cites its sources
  (URL plus version or commit). Spec agents may also extract published
  test-vector **data** into `testdata/vectors/`, with citations.
- **Implementation role** (main session): writes all Go code, working only
  from `spec/`, public standards (RFCs, FIPS, signal.org/docs), and
  black-box observation of the signal-cli binary. It does not open
  upstream source files. If a spec is missing something, it requests a
  narrower spec instead.
- Both roles may run the reference binary and record its inputs and
  outputs (black-box observation).

## Honest limits

The implementing model's training data very likely includes signal-cli,
libsignal, and Signal-Server. This process reduces the risk of copying,
but it **cannot certify independence**. The implementation role writes
code in its own structure and idiom and does not transliterate recalled
upstream code.

**Licensing flag:** if this project is released under any license other
than GPL or AGPL, the owner should get a legal review first.

## Log

Each row is one spec artifact: which subagent produced it, from which
sources.

| Date | Spec file | Produced by | Sources (URL @ version/commit) | Notes |
|------|-----------|-------------|--------------------------------|-------|
