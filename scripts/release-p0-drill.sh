#!/usr/bin/env bash
set -euo pipefail

API_URL="${CORAG_API_URL:-http://localhost:8080}"
AUTH_HEADER="${CORAG_AUTH_HEADER:-}"
AUTO_LOGIN="${CORAG_AUTO_LOGIN:-1}"
LOGIN_USERNAME="${CORAG_LOGIN_USERNAME:-admin}"
LOGIN_PASSWORD="${CORAG_LOGIN_PASSWORD:-admin}"
DRILL_POLL_MAX="${DRILL_POLL_MAX:-90}"
DRILL_POLL_INTERVAL="${DRILL_POLL_INTERVAL:-2}"
DRILL_DB_OUTAGE_SECONDS="${DRILL_DB_OUTAGE_SECONDS:-5}"
RUN_DB_DRILL="${RUN_DB_DRILL:-0}"
ARTIFACT_DIR="${DRILL_ARTIFACT_DIR:-artifacts/release}"
COMPOSE_FILE="${DRILL_COMPOSE_FILE:-deployments/compose/docker-compose.yml}"

CURL_BASE=(--connect-timeout 5 -sS -m 30 -L)
if [[ -n "$AUTH_HEADER" ]]; then
  CURL_BASE+=(-H "$AUTH_HEADER")
fi

RESPONSE_BODY=""
RESPONSE_CODE=""

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "error: required command not found: $1" >&2
    exit 1
  fi
}

compose_cmd() {
  if command -v docker >/dev/null 2>&1 && docker compose version >/dev/null 2>&1; then
    docker compose -f "$COMPOSE_FILE" "$@"
    return
  fi
  if command -v docker-compose >/dev/null 2>&1; then
    docker-compose -f "$COMPOSE_FILE" "$@"
    return
  fi
  echo "error: neither 'docker compose' nor 'docker-compose' is available" >&2
  exit 1
}

json_get() {
  local key="$1"
  local input="$2"
  if command -v jq >/dev/null 2>&1; then
    echo "$input" | jq -r ".$key // empty" 2>/dev/null || true
    return
  fi
  echo "$input" | sed -n "s/.*\"$key\"[[:space:]]*:[[:space:]]*\"\([^\"]*\)\".*/\1/p" | head -1
}

ensure_auth_header() {
  if [[ -n "$AUTH_HEADER" || "$AUTO_LOGIN" != "1" ]]; then
    return
  fi
  local body_file
  body_file="$(mktemp)"
  local code
  code="$(curl --connect-timeout 5 -sS -m 10 -L -X POST "$API_URL/api/login" \
    -H "Content-Type: application/json" \
    -d "{\"username\":\"$LOGIN_USERNAME\",\"password\":\"$LOGIN_PASSWORD\"}" \
    -o "$body_file" -w "%{http_code}" || true)"
  if [[ "$code" != "200" ]]; then
    rm -f "$body_file"
    return
  fi
  local token
  token="$(json_get "token" "$(cat "$body_file")")"
  rm -f "$body_file"
  if [[ -z "$token" ]]; then
    return
  fi
  AUTH_HEADER="Authorization: Bearer $token"
  CURL_BASE+=(-H "$AUTH_HEADER")
  echo "[drill] auto login success: user=$LOGIN_USERNAME"
}

http_request() {
  local method="$1"
  local url="$2"
  local data="${3:-}"
  local body_file
  body_file="$(mktemp)"

  if [[ -n "$data" ]]; then
    RESPONSE_CODE="$(curl "${CURL_BASE[@]}" -X "$method" "$url" -H "Content-Type: application/json" -d "$data" -o "$body_file" -w "%{http_code}")"
  else
    RESPONSE_CODE="$(curl "${CURL_BASE[@]}" -X "$method" "$url" -o "$body_file" -w "%{http_code}")"
  fi
  RESPONSE_BODY="$(cat "$body_file")"
  rm -f "$body_file"
}

wait_health() {
  local i
  for ((i = 0; i < 60; i++)); do
    if curl -fsS "$API_URL/api/health" >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done
  return 1
}

create_agent() {
  local name="$1"
  local i
  for ((i = 0; i < 5; i++)); do
    http_request POST "$API_URL/api/agents" "{\"name\":\"$name\"}"
    if [[ "$RESPONSE_CODE" == "200" ]]; then
      local id
      id="$(json_get "id" "$RESPONSE_BODY")"
      if [[ -n "$id" ]]; then
        echo "$id"
        return 0
      fi
    fi
    sleep 1
  done
  return 1
}

create_job() {
  local agent_id="$1"
  local message="$2"
  local i
  for ((i = 0; i < 3; i++)); do
    http_request POST "$API_URL/api/agents/$agent_id/message" "{\"message\":\"$message\"}"
    if [[ "$RESPONSE_CODE" == "202" ]]; then
      local job_id
      job_id="$(json_get "job_id" "$RESPONSE_BODY")"
      if [[ -n "$job_id" ]]; then
        echo "$job_id"
        return 0
      fi
    fi
    # 404 交给上层做 Agent 重建；其余短暂错误可重试
    if [[ "$RESPONSE_CODE" == "404" ]]; then
      return 1
    fi
    sleep 1
  done
  return 1
}

create_job_resilient() {
  local agent_id="$1"
  local message="$2"
  local ts="$3"
  local job_id
  if job_id="$(create_job "$agent_id" "$message")"; then
    echo "$agent_id|$job_id"
    return 0
  fi

  # API 重启后内存态 Agent 可能丢失（404）；重建 Agent 后重试一次，避免误报 Drill 失败。
  if [[ "$RESPONSE_CODE" != "404" ]]; then
    return 1
  fi
  local recreated_agent_id
  if ! recreated_agent_id="$(create_agent "drill-agent-$ts-recovered")"; then
    return 1
  fi
  if ! job_id="$(create_job "$recreated_agent_id" "$message")"; then
    return 1
  fi
  echo "$recreated_agent_id|$job_id"
}

wait_terminal() {
  local job_id="$1"
  local i
  local status=""
  for ((i = 0; i < DRILL_POLL_MAX; i++)); do
    http_request GET "$API_URL/api/jobs/$job_id"
    if [[ "$RESPONSE_CODE" != "200" ]]; then
      echo "http_${RESPONSE_CODE}"
      return
    fi
    status="$(json_get "status" "$RESPONSE_BODY")"
    case "$status" in
      completed|failed|cancelled)
        echo "$status"
        return
        ;;
    esac
    sleep "$DRILL_POLL_INTERVAL"
  done
  echo "timeout"
}

record_result() {
  local file="$1"
  local name="$2"
  local status="$3"
  local detail="$4"
  echo "- $name: $status ($detail)" >> "$file"
}

main() {
  require_cmd curl
  require_cmd docker
  ensure_auth_header

  mkdir -p "$ARTIFACT_DIR"
  local ts
  ts="$(date +%Y%m%d-%H%M%S)"
  local report
  report="$ARTIFACT_DIR/failure-drill-2.0-$ts.md"

  local result_file
  result_file="$(mktemp)"
  trap 'if [[ -n "${result_file:-}" ]]; then rm -f "$result_file"; fi' EXIT

  local passed=0
  local failed=0
  local skipped=0

  echo "[drill] checking health..."
  if ! wait_health; then
    echo "[drill] API health check failed" >&2
    exit 1
  fi

  local agent_id
  if ! agent_id="$(create_agent "drill-agent-$ts")"; then
    echo "[drill] create agent failed" >&2
    exit 1
  fi

  echo "[drill] Drill A: worker restart during processing"
  local job_a
  local pair_a
  if ! pair_a="$(create_job_resilient "$agent_id" "drill-a: please respond with one short sentence" "$ts")"; then
    record_result "$result_file" "Drill A (worker crash recovery)" "FAIL" "create job failed code=$RESPONSE_CODE"
    failed=$((failed + 1))
  else
    IFS='|' read -r agent_id job_a <<< "$pair_a"
    compose_cmd restart worker1 >/dev/null
    local terminal_a
    terminal_a="$(wait_terminal "$job_a")"
    if [[ "$terminal_a" == "timeout" || "$terminal_a" == http_* ]]; then
      record_result "$result_file" "Drill A (worker crash recovery)" "FAIL" "terminal=$terminal_a"
      failed=$((failed + 1))
    else
      record_result "$result_file" "Drill A (worker crash recovery)" "PASS" "terminal=$terminal_a"
      passed=$((passed + 1))
    fi
  fi

  echo "[drill] Drill B: API restart"
  local job_b
  local pair_b
  if ! pair_b="$(create_job_resilient "$agent_id" "drill-b: please respond with one short sentence" "$ts")"; then
    record_result "$result_file" "Drill B (api restart)" "FAIL" "create job failed code=$RESPONSE_CODE"
    failed=$((failed + 1))
  else
    IFS='|' read -r agent_id job_b <<< "$pair_b"
    compose_cmd restart api >/dev/null
    if ! wait_health; then
      record_result "$result_file" "Drill B (api restart)" "FAIL" "api did not recover"
      failed=$((failed + 1))
    else
      local terminal_b
      terminal_b="$(wait_terminal "$job_b")"
      if [[ "$terminal_b" == "timeout" || "$terminal_b" == http_* ]]; then
        record_result "$result_file" "Drill B (api restart)" "FAIL" "terminal=$terminal_b"
        failed=$((failed + 1))
      else
        record_result "$result_file" "Drill B (api restart)" "PASS" "terminal=$terminal_b"
        passed=$((passed + 1))
      fi
    fi
  fi

  echo "[drill] Drill C: Postgres short outage"
  if [[ "$RUN_DB_DRILL" != "1" ]]; then
    record_result "$result_file" "Drill C (postgres outage)" "SKIP" "set RUN_DB_DRILL=1 to enable"
    skipped=$((skipped + 1))
  else
    compose_cmd stop postgres >/dev/null
    sleep "$DRILL_DB_OUTAGE_SECONDS"
    compose_cmd start postgres >/dev/null
    if ! wait_health; then
      record_result "$result_file" "Drill C (postgres outage)" "FAIL" "api did not recover"
      failed=$((failed + 1))
    else
      local job_c
      local pair_c
      if ! pair_c="$(create_job_resilient "$agent_id" "drill-c: please respond with one short sentence" "$ts")"; then
        record_result "$result_file" "Drill C (postgres outage)" "FAIL" "create job failed code=$RESPONSE_CODE"
        failed=$((failed + 1))
      else
        IFS='|' read -r agent_id job_c <<< "$pair_c"
        local terminal_c
        terminal_c="$(wait_terminal "$job_c")"
        if [[ "$terminal_c" == "timeout" || "$terminal_c" == http_* ]]; then
          record_result "$result_file" "Drill C (postgres outage)" "FAIL" "terminal=$terminal_c"
          failed=$((failed + 1))
        else
          record_result "$result_file" "Drill C (postgres outage)" "PASS" "terminal=$terminal_c"
          passed=$((passed + 1))
        fi
      fi
    fi
  fi

  echo "[drill] Drill D: replay and trace availability"
  local job_d
  local agent_d
  if ! agent_d="$(create_agent "drill-agent-$ts-d")"; then
    record_result "$result_file" "Drill D (replay/trace)" "FAIL" "create agent failed code=$RESPONSE_CODE"
    failed=$((failed + 1))
  else
  local pair_d
  if ! pair_d="$(create_job_resilient "$agent_d" "drill-d: please respond with one short sentence" "$ts")"; then
    record_result "$result_file" "Drill D (replay/trace)" "FAIL" "create job failed code=$RESPONSE_CODE"
    failed=$((failed + 1))
  else
    IFS='|' read -r agent_id job_d <<< "$pair_d"
    local terminal_d
    terminal_d="$(wait_terminal "$job_d")"
    http_request GET "$API_URL/api/jobs/$job_d/replay"
    local replay_ok=0
    if [[ "$RESPONSE_CODE" == "200" ]] && grep -q '"timeline"' <<<"$RESPONSE_BODY"; then
      replay_ok=1
    fi
    http_request GET "$API_URL/api/jobs/$job_d/trace"
    local trace_ok=0
    if [[ "$RESPONSE_CODE" == "200" ]] && grep -q '"timeline"' <<<"$RESPONSE_BODY"; then
      trace_ok=1
    fi
    if [[ "$terminal_d" != "timeout" && "$terminal_d" != http_* && "$replay_ok" -eq 1 && "$trace_ok" -eq 1 ]]; then
      record_result "$result_file" "Drill D (replay/trace)" "PASS" "terminal=$terminal_d"
      passed=$((passed + 1))
    else
      record_result "$result_file" "Drill D (replay/trace)" "FAIL" "terminal=$terminal_d replay_ok=$replay_ok trace_ok=$trace_ok"
      failed=$((failed + 1))
    fi
  fi
  fi

  echo "[drill] Drill E: forensics export + verify endpoint"
  local job_e
  local agent_e
  if ! agent_e="$(create_agent "drill-agent-$ts-e")"; then
    record_result "$result_file" "Drill E (forensics)" "FAIL" "create agent failed code=$RESPONSE_CODE"
    failed=$((failed + 1))
  else
  local pair_e
  if ! pair_e="$(create_job_resilient "$agent_e" "drill-e: please respond with one short sentence" "$ts")"; then
    record_result "$result_file" "Drill E (forensics)" "FAIL" "create job failed code=$RESPONSE_CODE"
    failed=$((failed + 1))
  else
    IFS='|' read -r agent_id job_e <<< "$pair_e"
    local terminal_e
    terminal_e="$(wait_terminal "$job_e")"
    http_request POST "$API_URL/api/jobs/$job_e/export"
    local export_ok=0
    if [[ "$RESPONSE_CODE" == "200" ]]; then
      export_ok=1
    fi
    http_request GET "$API_URL/api/jobs/$job_e/verify"
    local verify_ok=0
    if [[ "$RESPONSE_CODE" == "200" ]] && grep -q '"execution_hash"' <<<"$RESPONSE_BODY"; then
      verify_ok=1
    fi

    if [[ "$terminal_e" != "timeout" && "$terminal_e" != http_* && "$export_ok" -eq 1 && "$verify_ok" -eq 1 ]]; then
      record_result "$result_file" "Drill E (forensics)" "PASS" "terminal=$terminal_e"
      passed=$((passed + 1))
    else
      record_result "$result_file" "Drill E (forensics)" "FAIL" "terminal=$terminal_e export_ok=$export_ok verify_ok=$verify_ok"
      failed=$((failed + 1))
    fi
  fi
  fi

  {
    echo "# Failure Drill Report (2.0)"
    echo
    echo "- API URL: $API_URL"
    echo "- Generated at: $(date -u +"%Y-%m-%dT%H:%M:%SZ")"
    echo "- Passed: $passed"
    echo "- Failed: $failed"
    echo "- Skipped: $skipped"
    echo
    echo "## Drill Results"
    cat "$result_file"
    echo
    echo "## Verdict"
    if [[ "$failed" -eq 0 ]]; then
      echo "- Gate passed: yes"
    else
      echo "- Gate passed: no"
    fi
  } > "$report"

  echo "[drill] report: $report"
  echo "[drill] passed=$passed failed=$failed skipped=$skipped"

  if [[ "$failed" -ne 0 ]]; then
    exit 1
  fi
}

main "$@"
