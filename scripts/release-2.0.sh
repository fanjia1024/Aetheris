#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

# In CI/release pipelines, run P0 gates by default unless explicitly disabled.
if [[ "${CI:-}" == "true" ]]; then
  RUN_P0_PERF="${RUN_P0_PERF:-1}"
  RUN_P0_DRILLS="${RUN_P0_DRILLS:-1}"
  RUN_DB_DRILL="${RUN_DB_DRILL:-1}"
  RUN_TENANT_REGRESSION="${RUN_TENANT_REGRESSION:-1}"
else
  RUN_P0_PERF="${RUN_P0_PERF:-0}"
  RUN_P0_DRILLS="${RUN_P0_DRILLS:-0}"
  RUN_DB_DRILL="${RUN_DB_DRILL:-0}"
  RUN_TENANT_REGRESSION="${RUN_TENANT_REGRESSION:-1}"
fi

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

if [[ "$RUN_P0_PERF" == "1" ]]; then
  echo "[release-2.0] P0 performance gate"
  ./scripts/release-p0-perf.sh
fi

if [[ "$RUN_P0_DRILLS" == "1" ]]; then
  echo "[release-2.0] P0 failure drill gate"
  RUN_DB_DRILL="$RUN_DB_DRILL" ./scripts/release-p0-drill.sh
fi

if [[ "$RUN_TENANT_REGRESSION" == "1" ]]; then
  echo "[release-2.0] tenant regression gate"
  ./scripts/release-tenant-regression.sh
fi

echo "[release-2.0] completed successfully"
echo "[release-2.0] see docs/release-checklist-2.0.md for manual sign-off items"
