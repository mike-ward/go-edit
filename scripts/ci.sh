#!/usr/bin/env bash
# Local CI: race, shuffle, vet, lint. Mirrors what a CI workflow
# should run. Fails on first error.
set -euo pipefail

cd "$(dirname "$0")/.."

echo "==> go vet"
go vet ./...

echo "==> go test -race -shuffle=on"
go test -count=1 -race -shuffle=on ./edit/...

echo "==> golangci-lint"
if command -v golangci-lint >/dev/null 2>&1; then
  golangci-lint run
else
  echo "golangci-lint not installed; skipping"
fi

echo "==> build examples"
go build ./examples/basic
go build ./examples/npad

echo "OK"
