# signal-cli-go task runner. Run recipes inside the sandbox only.
# `just` with no arguments lists recipes and does nothing else.

# /tmp is noexec in the sandbox; build test binaries in the agent volume.
export GOTMPDIR := env("GOTMPDIR", env("HOME", "/home/agent") / ".cache/gotmp")

# Build only from vendor/ (the permitted modules, see PLAN.md Decisions).
export GOFLAGS := "-mod=vendor"

# No C in any build (operator, 2026-10-29). Only test-race turns cgo on, for
# the race runtime; TestProductionBinariesHaveNoC checks the shipped builds.
export CGO_ENABLED := "0"

oracle_dir := "third_party/signal-cli-0.14.7"
protoc := "third_party/protoc-36.2/bin/protoc"

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

# Fail if gofmt would change anything (vendor/ excluded). Fails loudly if
# git lists no Go files (an empty or torn index), instead of letting gofmt
# wait on stdin.
fmt-check:
    @files=$(git ls-files '*.go' | command grep -v '^vendor/'); test -n "$files" || { echo "fmt-check: git lists no Go files (empty index? run git reset)"; exit 1; }; out=$(gofmt -l $files); test -z "$out" || { echo "$out"; exit 1; }

# Offline tests (Layer A + replayed Layer B)
test: _gotmp
    go test ./...

# Tests with the race detector (the race runtime needs cgo and gcc; tests only)
test-race: _gotmp
    CGO_ENABLED=1 go test -race ./...

# Fail unless only the permitted modules (pinned, vendored) are dependencies,
# the main binary contains protobuf only, and no shipped binary contains C
check-deps: _gotmp
    go test ./internal/repocheck -run 'TestOnlyAllowedModule|TestRequiresParser|TestProductionBinariesHaveNoC|TestNoCgoPackages' -count=1

# Everything CI runs
ci: fmt-check check-deps vet test-race proto-check

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

# Download and verify protoc into third_party/ (for proto-gen)
install-protoc: _gotmp
    go run ./tools/install-protoc

# Regenerate internal/proto/signalpb from its .proto files
proto-gen: _gotmp
    @test -x {{protoc}} || { echo "protoc missing: run 'just install-protoc'"; exit 1; }
    go build -o third_party/bin/protoc-gen-go google.golang.org/protobuf/cmd/protoc-gen-go
    {{protoc}} --plugin=protoc-gen-go=third_party/bin/protoc-gen-go -I internal/proto/signalpb --go_out=internal/proto/signalpb --go_opt=paths=source_relative internal/proto/signalpb/*.proto

# Fail if regenerating the protobuf code would change anything
proto-check: _gotmp
    bash tools/proto-check/check.sh {{protoc}}
