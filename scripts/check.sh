#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
test -z "$(gofmt -l cmd internal tools)" || { echo '请先运行 gofmt'; exit 1; }
test "$(go env GOVERSION)" = "go1.26.5" || { echo '需要 Go 1.26.5'; exit 1; }
go mod verify
go vet ./...
mkdir -p artifacts
go test -race -count=1 -coverprofile=artifacts/coverage.out ./...
go run ./tools/coverage artifacts/coverage.out
go build ./...
