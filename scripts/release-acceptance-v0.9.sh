#!/usr/bin/env bash
# v0.9 发布验收脚本骨架（一步步命令级别）
# 使用方式：按顺序执行各段，或取消注释并设置 API_URL / AGENT_ID / JOB_ID 后分段运行。

set -e
API_URL="${CORAG_API_URL:-http://localhost:8080}"
# AGENT_ID=   # 创建 agent 后填入
# JOB_ID=     # 发消息后填入

echo "=== 1. Worker Crash Recovery ==="
echo "请手动：1) 启动 1 API + 1 Worker + PG  2) 创建 Agent 并发多步消息，记下 job_id"
echo "        3) kill -9 worker  4) 再起 Worker  5) 验证 job 完成且无重复 plan/tool"
# corag agent create test
# corag chat $AGENT_ID  # 发消息，记 job_id；另一终端 kill -9 worker；重启 worker
# corag trace $JOB_ID

echo ""
echo "=== 2. API 重启不影响任务 ==="
echo "请手动：运行中任务存在时重启 API，验证 job 不丢、Worker 继续执行完成"
# 重启 API 后轮询 job 状态

echo ""
echo "=== 3. 多 Worker 不重复 ==="
echo "请手动：启动 3 Worker，创建 10 个 job，验证每个只完成一次、无重复 tool/plan"
# for i in $(seq 10); do curl -s -X POST "$API_URL/api/agents/$AGENT_ID/message" -H "Content-Type: application/json" -d "{\"message\":\"msg$i\"}"; done

echo ""
echo "=== 4. Planner Determinism ==="
echo "请手动：恢复前后各取 trace，对比 DAG 一致"
# corag trace $JOB_ID > trace_before.txt
# corag trace $JOB_ID > trace_after.txt  # 恢复后再取

echo ""
echo "=== 5. Session Persistence ==="
echo "请手动：多轮对话后重启 Worker，再发依赖上文的消息，验证 LLM 引用上文"
# corag chat $AGENT_ID  # 多轮；重启 worker；再发一条

echo ""
echo "=== 6. Tool Idempotency ==="
echo "与 1/4 结合：恢复后 trace 中每 tool 节点仅调用一次"

echo ""
echo "=== 7. Event Log Replay ==="
echo "GET events 与 trace 可完整重放"
# curl -s "$API_URL/api/jobs/$JOB_ID/events" | jq .
# corag replay $JOB_ID

echo ""
echo "=== 8. Backpressure ==="
echo "请手动：连续创建 100+ job，验证无 OOM、Worker 并发受 limit 限制"
# for i in $(seq 100); do curl -s -X POST "$API_URL/api/agents/$AGENT_ID/message" -H "Content-Type: application/json" -d "{\"message\":\"bp$i\"}"; done

echo ""
echo "=== 9. Cancellation ==="
echo "发长任务后立即 cancel，验证 job 进入 CANCELLED"
# JOB_ID=$(curl -s -X POST "$API_URL/api/agents/$AGENT_ID/message" -H "Content-Type: application/json" -d '{"message":"long task"}' | jq -r .job_id)
# corag cancel $JOB_ID
# curl -s "$API_URL/api/jobs/$JOB_ID" | jq .status  # 应为 cancelled

echo ""
echo "全部通过后可打 v0.9 标签。详见 docs/release-acceptance-v0.9.md"
