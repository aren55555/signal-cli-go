# signal-cli-go task runner. Run recipes inside the sandbox only.
# `just` with no arguments lists recipes and does nothing else.

# /tmp is noexec in the sandbox; build test binaries in the agent volume.
export GOTMPDIR := env("GOTMPDIR", env("HOME", "/home/agent") / ".cache/gotmp")

oracle_dir := "third_party/signal-cli-0.14.7"

[private]
default:
    @just --list

# Run the sandbox preflight check
preflight:
    sandbox/preflight.sh

# Build everything
build: _gotmp
    go build ./...

# go vet
vet: _gotmp
    go vet ./...

# Fail if gofmt would change anything
fmt-check:
    @test -z "$(gofmt -l .)" || { gofmt -l .; exit 1; }

# Offline tests (Layer A + replayed Layer B)
test: _gotmp
    go test ./...

# Tests with the race detector (needs gcc in the image)
test-race: _gotmp
    go test -race ./...

# Fail if go.mod has any require lines or a go.sum exists
check-no-deps:
    @! command grep -nE '^\s*require\b' go.mod sandbox/proxy/go.mod
    @test ! -e go.sum
    @echo "no module dependencies"

# Everything CI runs
ci: fmt-check check-no-deps vet test-race

# Download and verify the reference signal-cli 0.14.7 into third_party/
install-oracle: _gotmp
    go run ./tools/install-oracle

# Run the reference signal-cli (test oracle) with arguments.
# /tmp is noexec, so the JVM must unpack libsignal elsewhere
oracle *args:
    @mkdir -p ~/.cache/jvmtmp
    JAVA_OPTS="${JAVA_OPTS:-} -Djava.io.tmpdir=$HOME/.cache/jvmtmp" {{oracle_dir}}/bin/signal-cli {{args}}

_gotmp:
    @mkdir -p "$GOTMPDIR"

# Re-record golden transcripts from the reference signal-cli (sandbox only)
record-golden: _gotmp
    go test ./internal/difftest -run Golden -count=1 -record -v -timeout 30m
