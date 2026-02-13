#!/usr/bin/env bash
set -euo pipefail

API_URL="${CORAG_API_URL:-http://localhost:8080}"
AUTH_HEADER="${CORAG_AUTH_HEADER:-}"
AUTO_LOGIN="${CORAG_AUTO_LOGIN:-1}"
LOGIN_USERNAME="${CORAG_LOGIN_USERNAME:-admin}"
LOGIN_PASSWORD="${CORAG_LOGIN_PASSWORD:-admin}"
PERF_SAMPLES="${PERF_SAMPLES:-20}"
PERF_POLL_MAX="${PERF_POLL_MAX:-90}"
PERF_POLL_INTERVAL="${PERF_POLL_INTERVAL:-2}"
PERF_MIN_JOBS_PER_MIN="${PERF_MIN_JOBS_PER_MIN:-10}"
PERF_MIN_COMPLETION_RATIO="${PERF_MIN_COMPLETION_RATIO:-0.95}"
PERF_MAX_POST_MESSAGE_P95_MS="${PERF_MAX_POST_MESSAGE_P95_MS:-500}"
PERF_MAX_GET_JOB_P95_MS="${PERF_MAX_GET_JOB_P95_MS:-200}"
PERF_MAX_GET_EVENTS_P95_MS="${PERF_MAX_GET_EVENTS_P95_MS:-500}"
ARTIFACT_DIR="${PERF_ARTIFACT_DIR:-artifacts/release}"

CURL_BASE=(--connect-timeout 5 -sS -m 30 -L)
if [[ -n "$AUTH_HEADER" ]]; then
  CURL_BASE+=(-H "$AUTH_HEADER")
fi

RESPONSE_BODY=""
RESPONSE_CODE=""
RESPONSE_TIME_MS=""

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "error: required command not found: $1" >&2
    exit 1
  fi
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
  echo "[perf] auto login success: user=$LOGIN_USERNAME"
}

http_request() {
  local method="$1"
  local url="$2"
  local data="${3:-}"

  local body_file
  body_file="$(mktemp)"
  local meta

  if [[ -n "$data" ]]; then
    meta=$(curl "${CURL_BASE[@]}" -X "$method" "$url" -H "Content-Type: application/json" -d "$data" -o "$body_file" -w "%{http_code} %{time_total}")
  else
    meta=$(curl "${CURL_BASE[@]}" -X "$method" "$url" -o "$body_file" -w "%{http_code} %{time_total}")
  fi

  RESPONSE_BODY="$(cat "$body_file")"
  rm -f "$body_file"

  RESPONSE_CODE="${meta%% *}"
  local time_total
  time_total="${meta##* }"
  RESPONSE_TIME_MS="$(awk -v s="$time_total" 'BEGIN { printf "%.0f", s * 1000 }')"
}

percentile() {
  local file="$1"
  local p="$2"
  local n
  n="$(wc -l < "$file" | tr -d ' ')"
  if [[ "$n" -eq 0 ]]; then
    echo "0"
    return
  fi
  local idx
  idx="$(awk -v n="$n" -v p="$p" 'BEGIN { i = int((n*p + 99)/100); if (i < 1) i = 1; if (i > n) i = n; print i }')"
  sort -n "$file" | awk -v i="$idx" 'NR == i { print; exit }'
}

wait_terminal() {
  local job_id="$1"
  local status=""
  local i

  for ((i = 0; i < PERF_POLL_MAX; i++)); do
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
    sleep "$PERF_POLL_INTERVAL"
  done

  echo "timeout"
}

main() {
  require_cmd curl
  require_cmd awk
  require_cmd sort
  ensure_auth_header

  mkdir -p "$ARTIFACT_DIR"
  local ts
  ts="$(date +%Y%m%d-%H%M%S)"
  local report
  report="$ARTIFACT_DIR/perf-baseline-2.0-$ts.md"

  local tmp_dir
  tmp_dir="$(mktemp -d)"
  trap 'if [[ -n "${tmp_dir:-}" ]]; then rm -rf "$tmp_dir"; fi' EXIT

  local post_file="$tmp_dir/post_ms.txt"
  local get_job_file="$tmp_dir/get_job_ms.txt"
  local get_events_file="$tmp_dir/get_events_ms.txt"
  : > "$post_file"
  : > "$get_job_file"
  : > "$get_events_file"

  echo "[perf] checking health..."
  http_request GET "$API_URL/api/health"
  if [[ "$RESPONSE_CODE" != "200" ]]; then
    echo "[perf] health check failed: HTTP $RESPONSE_CODE" >&2
    exit 1
  fi

  local agent_name="perf-agent-$ts"
  echo "[perf] creating agent: $agent_name"
  http_request POST "$API_URL/api/agents" "{\"name\":\"$agent_name\"}"
  if [[ "$RESPONSE_CODE" != "200" ]]; then
    echo "[perf] create agent failed: HTTP $RESPONSE_CODE" >&2
    echo "$RESPONSE_BODY" >&2
    exit 1
  fi

  local agent_id
  agent_id="$(json_get "id" "$RESPONSE_BODY")"
  if [[ -z "$agent_id" ]]; then
    echo "[perf] create agent failed: missing id" >&2
    echo "$RESPONSE_BODY" >&2
    exit 1
  fi

  local started_at
  started_at="$(date +%s)"

  local total=0
  local completed=0
  local failed=0
  local cancelled=0
  local timeout=0
  local request_fail=0

  local i
  for ((i = 1; i <= PERF_SAMPLES; i++)); do
    local msg="release-perf sample-$i: 请用一句话确认收到"
    local idem="perf-$ts-$i"

    http_request POST "$API_URL/api/agents/$agent_id/message" "{\"message\":\"$msg\"}"
    echo "$RESPONSE_TIME_MS" >> "$post_file"

    if [[ "$RESPONSE_CODE" != "202" ]]; then
      request_fail=$((request_fail + 1))
      total=$((total + 1))
      continue
    fi

    local job_id
    job_id="$(json_get "job_id" "$RESPONSE_BODY")"
    if [[ -z "$job_id" ]]; then
      request_fail=$((request_fail + 1))
      total=$((total + 1))
      continue
    fi

    local terminal
    terminal="$(wait_terminal "$job_id")"
    case "$terminal" in
      completed) completed=$((completed + 1)) ;;
      failed) failed=$((failed + 1)) ;;
      cancelled) cancelled=$((cancelled + 1)) ;;
      timeout) timeout=$((timeout + 1)) ;;
      *) request_fail=$((request_fail + 1)) ;;
    esac

    http_request GET "$API_URL/api/jobs/$job_id"
    if [[ "$RESPONSE_CODE" == "200" ]]; then
      echo "$RESPONSE_TIME_MS" >> "$get_job_file"
    fi

    http_request GET "$API_URL/api/jobs/$job_id/events"
    if [[ "$RESPONSE_CODE" == "200" ]]; then
      echo "$RESPONSE_TIME_MS" >> "$get_events_file"
    fi

    total=$((total + 1))
  done

  local ended_at
  ended_at="$(date +%s)"
  local elapsed
  elapsed=$((ended_at - started_at))
  if [[ "$elapsed" -le 0 ]]; then
    elapsed=1
  fi

  local post_p95
  local get_job_p95
  local get_events_p95
  post_p95="$(percentile "$post_file" 95)"
  get_job_p95="$(percentile "$get_job_file" 95)"
  get_events_p95="$(percentile "$get_events_file" 95)"

  local completion_ratio
  completion_ratio="$(awk -v c="$completed" -v t="$total" 'BEGIN { if (t == 0) print "0.00"; else printf "%.2f", c/t }')"
  local jobs_per_min
  jobs_per_min="$(awk -v c="$completed" -v s="$elapsed" 'BEGIN { printf "%.2f", c*60/s }')"

  local gate_fail=0
  if (( post_p95 > PERF_MAX_POST_MESSAGE_P95_MS )); then gate_fail=1; fi
  if (( get_job_p95 > PERF_MAX_GET_JOB_P95_MS )); then gate_fail=1; fi
  if (( get_events_p95 > PERF_MAX_GET_EVENTS_P95_MS )); then gate_fail=1; fi
  if ! awk -v a="$jobs_per_min" -v b="$PERF_MIN_JOBS_PER_MIN" 'BEGIN { exit !(a >= b) }'; then gate_fail=1; fi
  if ! awk -v a="$completion_ratio" -v b="$PERF_MIN_COMPLETION_RATIO" 'BEGIN { exit !(a >= b) }'; then gate_fail=1; fi

  cat > "$report" <<REPORT
# Performance Baseline Report (2.0)

- API URL: $API_URL
- Samples: $PERF_SAMPLES
- Elapsed seconds: $elapsed
- Generated at: $(date -u +"%Y-%m-%dT%H:%M:%SZ")

## Result Summary

- Completed: $completed
- Failed: $failed
- Cancelled: $cancelled
- Timeout: $timeout
- Request failure: $request_fail
- Completion ratio: $completion_ratio
- Throughput (completed jobs/min): $jobs_per_min

## Latency (ms)

- POST /api/agents/:id/message P95: $post_p95
- GET /api/jobs/:id P95: $get_job_p95
- GET /api/jobs/:id/events P95: $get_events_p95

## Gate Thresholds

- POST message P95 <= $PERF_MAX_POST_MESSAGE_P95_MS
- GET job P95 <= $PERF_MAX_GET_JOB_P95_MS
- GET events P95 <= $PERF_MAX_GET_EVENTS_P95_MS
- Throughput >= $PERF_MIN_JOBS_PER_MIN jobs/min
- Completion ratio >= $PERF_MIN_COMPLETION_RATIO

## Gate Verdict

- Gate passed: $(if [[ "$gate_fail" -eq 0 ]]; then echo "yes"; else echo "no"; fi)
REPORT

  echo "[perf] report: $report"
  echo "[perf] completed=$completed failed=$failed cancelled=$cancelled timeout=$timeout req_fail=$request_fail"
  echo "[perf] p95(post/get_job/get_events)=$post_p95/$get_job_p95/$get_events_p95 ms"
  echo "[perf] throughput=$jobs_per_min jobs/min completion_ratio=$completion_ratio"

  if [[ "$gate_fail" -ne 0 ]]; then
    echo "[perf] gate failed" >&2
    exit 1
  fi

  echo "[perf] gate passed"
}

main "$@"
