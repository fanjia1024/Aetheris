#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

ARTIFACT_DIR="${TENANT_ARTIFACT_DIR:-artifacts/release}"
GOCACHE="${GOCACHE:-/tmp/corag-gocache}"

HTTP_SUITE_REGEX='TestGetJob_TenantIsolation|TestGetJob_DefaultTenantFallback|TestGetJobEvents_TenantIsolation|TestGetJobReplay_TenantIsolation|TestGetJobTrace_TenantIsolation|TestJobStop_RBACAndTenantMatrix'
AUTH_SUITE_REGEX='TestRBAC_TenantIsolation'

mkdir -p "$ARTIFACT_DIR"
mkdir -p "$GOCACHE"
export GOCACHE
ts="$(date +%Y%m%d-%H%M%S)"
report="$ARTIFACT_DIR/tenant-regression-2.0-$ts.md"

tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

http_log="$tmp_dir/http.log"
auth_log="$tmp_dir/auth.log"

http_status="PASS"
auth_status="PASS"
overall="PASS"

echo "[tenant-regression] running HTTP tenant/RBAC matrix suite..."
if ! go test -v ./internal/api/http -run "$HTTP_SUITE_REGEX" >"$http_log" 2>&1; then
  http_status="FAIL"
  overall="FAIL"
fi

echo "[tenant-regression] running auth tenant isolation suite..."
if ! go test -v ./pkg/auth -run "$AUTH_SUITE_REGEX" >"$auth_log" 2>&1; then
  auth_status="FAIL"
  overall="FAIL"
fi

{
  echo "# Tenant Regression Gate 2.0"
  echo
  echo "- Timestamp: $ts"
  echo "- Overall: $overall"
  echo
  echo "## Suites"
  echo
  echo "- internal/api/http ($http_status)"
  echo "  - regex: \`$HTTP_SUITE_REGEX\`"
  echo "- pkg/auth ($auth_status)"
  echo "  - regex: \`$AUTH_SUITE_REGEX\`"
  echo
  echo "## internal/api/http output"
  echo
  echo '```text'
  cat "$http_log"
  echo '```'
  echo
  echo "## pkg/auth output"
  echo
  echo '```text'
  cat "$auth_log"
  echo '```'
} >"$report"

echo "[tenant-regression] report written: $report"

if [[ "$overall" != "PASS" ]]; then
  echo "[tenant-regression] gate failed" >&2
  exit 1
fi

echo "[tenant-regression] gate passed"
