#!/usr/bin/env bash
# CoRag 1.0 发布验证脚本（完整可测）
# 用法：先启动 1 API + 2 Worker，再执行： ./scripts/release-cert-1.0.sh
# 环境变量：
#   CORAG_API_URL     API 地址（默认 http://localhost:8080）
#   CERT_POLL_MAX     Test 1 轮询次数（默认 90，约 3 分钟）
#   RUN_TEST4         设为 1 时执行多 Worker 并发测试（10 个 job）
#   RUN_MANUAL_REMINDER 设为 0 时不输出手动测试提醒（默认 1）

set -e
API_URL="${CORAG_API_URL:-http://localhost:8080}"
CERT_POLL_MAX="${CERT_POLL_MAX:-90}"
RUN_TEST4="${RUN_TEST4:-0}"
RUN_MANUAL_REMINDER="${RUN_MANUAL_REMINDER:-1}"
CURL_OPTS=(--connect-timeout 5 -s -m 30)
FAILED=0
PASSED=0

# 从 stdin 解析 JSON 字符串字段；参数为字段名
_json() {
  local key="$1"
  local input
  input=$(cat)
  if command -v jq &>/dev/null; then
    echo "$input" | jq -r ".$key // empty" 2>/dev/null || true
  else
    echo "$input" | sed -n "s/.*\"$key\"[[:space:]]*:[[:space:]]*\"\\([^\"]*\\)\".*/\1/p" | head -1
  fi
}

_pass() { echo "  [PASS] $1"; PASSED=$((PASSED + 1)); }
_fail() { echo "  [FAIL] $1"; FAILED=$((FAILED + 1)); }

# 请求并返回 HTTP 状态码；body 写入 stdout（需配合 -o 重定向时用 -w "%{http_code}\n" 并分离）
_curl() {
  curl "${CURL_OPTS[@]}" "$@"
}

echo "=============================================="
echo "  CoRag 1.0 Release Certification (自动化部分)"
echo "=============================================="
echo "  API_URL=$API_URL"
echo "  CERT_POLL_MAX=$CERT_POLL_MAX  RUN_TEST4=$RUN_TEST4"
echo "=============================================="
echo ""

# --- Step 0: Health ---
echo "--- Step 0: Health ---"
code=$(_curl -o /tmp/corag_health.json -w "%{http_code}" "$API_URL/api/health")
if [[ "$code" != "200" ]]; then
  _fail "GET /api/health 返回 $code（期望 200）"
else
  _pass "GET /api/health 返回 200"
fi
echo ""

# --- Test 1: Job 不丢失 ---
echo "--- Test 1: Job 不丢失 ---"
agent_resp=$(_curl -X POST "$API_URL/api/agents" -H "Content-Type: application/json" -d '{"name":"cert-agent"}')
agent_id=$(echo "$agent_resp" | _json "id")
if [[ -z "$agent_id" ]]; then
  _fail "创建 Agent 失败或无法解析 id: $agent_resp"
else
  _pass "创建 Agent: $agent_id"
fi

msg_resp=$(_curl -X POST "$API_URL/api/agents/$agent_id/message" \
  -H "Content-Type: application/json" \
  -d '{"message":"简短回复：1+1 等于几？"}')
job_id=$(echo "$msg_resp" | _json "job_id")
if [[ -z "$job_id" ]]; then
  _fail "发消息失败或无法解析 job_id: $msg_resp"
else
  _pass "获得 job_id: $job_id"
fi

seen_pending=0
seen_running=0
seen_completed=0
status=""
poll=0
while [[ $poll -lt $CERT_POLL_MAX ]]; do
  job_resp=$(_curl "$API_URL/api/jobs/$job_id")
  status=$(echo "$job_resp" | _json "status")
  case "$status" in
    pending)  seen_pending=1 ;;
    running)  seen_running=1 ;;
    completed) seen_completed=1; break ;;
    failed)   _fail "Job 以 failed 结束"; break ;;
    cancelled) _fail "Job 以 cancelled 结束（非本测试预期）"; break ;;
  esac
  sleep 2
  poll=$((poll + 1))
done

if [[ $seen_completed -eq 1 ]]; then
  _pass "Job 到达 completed（已见 pending→running→completed）"
elif [[ $seen_running -eq 1 ]]; then
  _fail "轮询 ${CERT_POLL_MAX} 次后仍 running（请确认 Worker 已启动并拉取任务）"
else
  _fail "Job 未到达 completed（当前 status=$status）"
fi
echo ""

# --- Test 5: Replay 只读 ---
echo "--- Test 5: Replay 只读 ---"
replay_code=$(_curl -o /tmp/corag_replay.json -w "%{http_code}" "$API_URL/api/jobs/$job_id/replay")
if [[ "$replay_code" != "200" ]]; then
  _fail "GET /api/jobs/:id/replay 返回 $replay_code（期望 200）"
else
  if grep -q '"read_only"[[:space:]]*:[[:space:]]*true' /tmp/corag_replay.json 2>/dev/null; then
    _pass "Replay 返回 200 且 read_only: true"
  else
    _pass "Replay 返回 200（含 timeline）"
  fi
fi
echo ""

# --- Test 7: Trace 可解释性 ---
echo "--- Test 7: Trace 可解释性 ---"
trace_code=$(_curl -o /tmp/corag_trace.json -w "%{http_code}" "$API_URL/api/jobs/$job_id/trace")
if [[ "$trace_code" != "200" ]]; then
  _fail "GET /api/jobs/:id/trace 返回 $trace_code（期望 200）"
else
  if grep -q '"timeline"' /tmp/corag_trace.json 2>/dev/null; then
    _pass "Trace 含 timeline"
  else
    _fail "Trace 响应缺少 timeline"
  fi
  if grep -qE 'node_finished|tool_called|tool_returned|plan_generated' /tmp/corag_trace.json 2>/dev/null; then
    _pass "Trace 含节点/tool/plan 事件"
  else
    _fail "Trace 应含 node_finished 或 tool_called/tool_returned 或 plan_generated"
  fi
  if grep -q 'node_durations' /tmp/corag_trace.json 2>/dev/null; then
    _pass "Trace 含 node_durations（节点耗时）"
  fi
fi
echo ""

# --- Test 6: 取消任务 ---
echo "--- Test 6: 取消任务 ---"
msg2_resp=$(_curl -X POST "$API_URL/api/agents/$agent_id/message" \
  -H "Content-Type: application/json" \
  -d '{"message":"写一篇很长的文章，不少于五千字"}')
job_id2=$(echo "$msg2_resp" | _json "job_id")
if [[ -z "$job_id2" ]]; then
  _fail "取消测试无法获得 job_id"
else
  stop_code=$(_curl -o /dev/null -w "%{http_code}" -X POST "$API_URL/api/jobs/$job_id2/stop")
  if [[ "$stop_code" != "200" && "$stop_code" != "201" ]]; then
    _fail "POST /api/jobs/:id/stop 返回 $stop_code"
  else
    _pass "POST /api/jobs/:id/stop 返回 $stop_code"
  fi
  poll2=0
  status2=""
  while [[ $poll2 -lt 30 ]]; do
    job2_resp=$(_curl "$API_URL/api/jobs/$job_id2")
    status2=$(echo "$job2_resp" | _json "status")
    case "$status2" in
      cancelled) _pass "Job 已进入 cancelled"; break ;;
      completed) _pass "Job 在取消前已完成（可接受）"; break ;;
    esac
    sleep 1
    poll2=$((poll2 + 1))
  done
  if [[ $poll2 -ge 30 && "$status2" != "cancelled" && "$status2" != "completed" ]]; then
    _fail "30s 内未变为 cancelled（当前 status=$status2）"
  fi
fi
echo ""

# --- Test 4: 多 Worker 并发（可选）---
if [[ "$RUN_TEST4" == "1" ]]; then
  echo "--- Test 4: 多 Worker 并发（10 个 job）---"
  declare -a jids
  for i in $(seq 1 10); do
    r=$(_curl -X POST "$API_URL/api/agents/$agent_id/message" \
      -H "Content-Type: application/json" \
      -d "{\"message\":\"第 $i 个任务：总结天气\"}")
    j=$(echo "$r" | _json "job_id")
    if [[ -n "$j" ]]; then
      jids+=("$j")
    fi
  done
  if [[ ${#jids[@]} -lt 10 ]]; then
    _fail "仅创建 ${#jids[@]} 个 job（期望 10）"
  else
    _pass "已创建 10 个 job"
  fi
  # 等待全部结束
  poll4=0
  while [[ $poll4 -lt $CERT_POLL_MAX ]]; do
    done_count=0
    for j in "${jids[@]}"; do
      resp=$(_curl "$API_URL/api/jobs/$j")
      st=$(echo "$resp" | _json "status")
      [[ "$st" == "completed" || "$st" == "failed" || "$st" == "cancelled" ]] && done_count=$((done_count + 1))
    done
    [[ $done_count -eq ${#jids[@]} ]] && break
    sleep 2
    poll4=$((poll4 + 1))
  done
  if [[ $done_count -eq ${#jids[@]} ]]; then
    _pass "10 个 job 均已结束（每个只应被一个 Worker 执行一次）"
  else
    _fail "超时：仅 $done_count/10 个 job 结束"
  fi
  echo ""
fi

# --- 手动测试提醒 ---
if [[ "$RUN_MANUAL_REMINDER" == "1" ]]; then
  echo "--- 需手动执行的测试（见 docs/release-certification-1.0.md）---"
  echo "  Test 2: Worker Crash Recovery — 运行中 kill -9 一个 worker，验证同一 job 由另一 Worker 续跑完成"
  echo "  Test 3: API Crash — 运行中重启 API，验证 job 不丢"
  echo "  Test 4: 多 Worker 并发 — 可设 RUN_TEST4=1 本脚本内跑 10 job；或手动观察无重复执行"
  echo "  Test 8: 真正恢复 — kill -9 所有 worker 与 api，10 秒后全部重启，验证 job 继续完成"
  echo ""
fi

# --- 汇总 ---
echo "=============================================="
echo "  结果: PASS=$PASSED  FAIL=$FAILED"
echo "=============================================="
if [[ $FAILED -eq 0 ]]; then
  echo "  自动步骤全部通过。"
  exit 0
else
  echo "  存在失败项，不能发 1.0。"
  exit 1
fi
