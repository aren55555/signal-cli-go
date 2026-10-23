#!/usr/bin/env bash
# Fails if regenerating internal/proto/signalpb would change anything:
# regenerates into a temporary directory with the pinned protoc and the
# vendored protoc-gen-go, then compares with the committed .pb.go files.
# Usage: tools/proto-check/check.sh PATH_TO_PROTOC
set -euo pipefail
protoc=${1:?usage: check.sh PATH_TO_PROTOC}
test -x "$protoc" || { echo "protoc missing at $protoc: run 'just install-protoc' or 'go run ./tools/install-protoc'"; exit 1; }
pkg=internal/proto/signalpb
tmp=$(mktemp -d "${GOTMPDIR:-${TMPDIR:-/tmp}}/protocheck.XXXXXX")
trap 'rm -rf "$tmp"' EXIT
go build -mod=vendor -o "$tmp/protoc-gen-go" google.golang.org/protobuf/cmd/protoc-gen-go
"$protoc" --plugin=protoc-gen-go="$tmp/protoc-gen-go" -I "$pkg" --go_out="$tmp" --go_opt=paths=source_relative "$pkg"/*.proto
status=0
for f in "$pkg"/*.pb.go; do
  test -e "$tmp/$(basename "$f")" || { echo "stale: $f has no .proto"; status=1; }
done
for f in "$tmp"/*.pb.go; do
  diff -u "$pkg/$(basename "$f")" "$f" || status=1
done
if [ "$status" -ne 0 ]; then echo "generated code is out of date: run 'just proto-gen'"; fi
exit "$status"
