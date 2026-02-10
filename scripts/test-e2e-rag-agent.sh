#!/usr/bin/env bash
# RAG 检索智能体 E2E：健康检查 → 上传文档 → 创建 Agent → 发送与文档相关的问题 → 轮询 Job → 校验 Trace 含 knowledge.search
# 用法: ./scripts/test-e2e-rag-agent.sh [文件路径] [问题]
# 示例: ./scripts/test-e2e-rag-agent.sh ./AGENTS.md "总结这份文档的要点"
# 默认: 文件路径 ./AGENTS.md，问题 "总结这份文档的要点"
# 前提: API 已启动（memory 模式仅 API 即可；postgres 模式需至少 1 个 Worker）

set -e

BASE_URL="${BASE_URL:-http://localhost:8080}"
FILE_PATH="${1:-./AGENTS.md}"
QUERY="${2:-总结这份文档的要点}"
POLL_MAX="${RAG_POLL_MAX:-90}"
POLL_INTERVAL="${RAG_POLL_INTERVAL:-2}"
CURL_OPTS=(--connect-timeout 5 -s -m 30 -L)

# 从 stdin 解析 JSON 字符串字段
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

echo "=============================================="
echo "  RAG 检索智能体 E2E"
echo "=============================================="
echo "  BASE_URL=$BASE_URL"
echo "  FILE_PATH=$FILE_PATH"
echo "  QUERY=$QUERY"
echo "=============================================="
echo ""

if [[ ! -f "$FILE_PATH" ]]; then
  echo "[FAIL] 文件不存在: $FILE_PATH"
  exit 1
fi

echo "=== 1. Health check ==="
health=$(curl -s -o /dev/null -w "%{http_code}" "$BASE_URL/api/health")
if [[ "$health" != "200" ]]; then
  echo "[FAIL] 健康检查失败 (HTTP $health)，请确认 API 已启动: $BASE_URL"
  exit 1
fi
echo "[OK] Health $health"
echo ""

echo "=== 2. Upload document ==="
upload_resp=$(curl "${CURL_OPTS[@]}" -X POST "$BASE_URL/api/documents/upload" -F "file=@$FILE_PATH")
if ! echo "$upload_resp" | grep -q '"doc_id"\|"document_id"\|"id"'; then
  echo "[FAIL] 上传文档未返回 doc_id，响应: ${upload_resp:0:300}"
  exit 1
fi
echo "[OK] 文档已上传"
echo ""

echo "=== 3. Create agent ==="
agent_resp=$(curl "${CURL_OPTS[@]}" -X POST "$BASE_URL/api/agents" -H "Content-Type: application/json" -d '{"name":"rag-e2e-agent"}')
agent_id=$(echo "$agent_resp" | _json "id")
if [[ -z "$agent_id" ]]; then
  echo "[FAIL] 创建 Agent 失败，响应: ${agent_resp:0:200}"
  exit 1
fi
echo "[OK] agent_id=$agent_id"
echo ""

echo "=== 4. Send message (RAG 问题) ==="
msg_resp=$(curl "${CURL_OPTS[@]}" -X POST "$BASE_URL/api/agents/$agent_id/message" \
  -H "Content-Type: application/json" \
  -d "{\"message\":\"$QUERY\"}")
job_id=$(echo "$msg_resp" | _json "job_id")
if [[ -z "$job_id" ]]; then
  echo "[FAIL] 发消息未返回 job_id，响应: ${msg_resp:0:200}"
  exit 1
fi
echo "[OK] job_id=$job_id"
echo ""

echo "=== 5. Poll job status (max ${POLL_MAX} x ${POLL_INTERVAL}s) ==="
status=""
poll=0
while [[ $poll -lt $POLL_MAX ]]; do
  job_resp=$(curl "${CURL_OPTS[@]}" "$BASE_URL/api/jobs/$job_id")
  status=$(echo "$job_resp" | _json "status")
  # 兼容 status 为数字的 API（如 3=completed）
  if [[ "$status" == "completed" || "$status" == "3" ]]; then
    echo "[OK] Job completed"
    break
  fi
  if [[ "$status" == "failed" || "$status" == "4" ]]; then
    echo "[FAIL] Job 以 failed 结束（可查看 trace: $BASE_URL/api/jobs/$job_id/trace）"
    exit 1
  fi
  sleep "$POLL_INTERVAL"
  poll=$((poll + 1))
done

if [[ "$status" != "completed" && "$status" != "3" ]]; then
  echo "[FAIL] 轮询超时，Job 未在预期内完成，当前 status=$status"
  exit 1
fi
echo ""

echo "=== 6. Verify Trace 含 knowledge.search ==="
trace_resp=$(curl "${CURL_OPTS[@]}" "$BASE_URL/api/jobs/$job_id/trace" 2>/dev/null || true)
events_resp=$(curl "${CURL_OPTS[@]}" "$BASE_URL/api/jobs/$job_id/events" 2>/dev/null || true)
if echo "$trace_resp$events_resp" | grep -q 'knowledge\.search'; then
  echo "[PASS] Trace/Events 中发现 knowledge.search，RAG 检索智能体 E2E 通过"
  exit 0
fi
if echo "$trace_resp$events_resp" | grep -q 'tool_called\|tool_name.*search'; then
  echo "[PASS] Trace/Events 中发现工具调用，RAG 检索智能体 E2E 通过"
  exit 0
fi
echo "[FAIL] Trace/Events 中未发现 knowledge.search 或 tool_called，可能 Planner 未选择检索工具"
echo "  可打开 $BASE_URL/api/jobs/$job_id/trace/page 查看执行图"
exit 1
