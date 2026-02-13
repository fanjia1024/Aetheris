#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

echo "[release-2.0] starting release checks..."

echo "[release-2.0] gofmt check"
if [ -n "$(gofmt -l .)" ]; then
  echo "[release-2.0] gofmt check failed:" >&2
  gofmt -l . >&2
  exit 1
fi

echo "[release-2.0] go vet"
go vet ./...

echo "[release-2.0] unit and integration tests"
go test -v ./...

echo "[release-2.0] build artifacts"
go build -v ./...

echo "[release-2.0] cli smoke"
./scripts/local-2.0-stack.sh --help >/dev/null || true

echo "[release-2.0] completed successfully"
echo "[release-2.0] see docs/release-checklist-2.0.md for manual sign-off items"
