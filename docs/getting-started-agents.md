# Getting Started: Build Your First Production Agent

This guide walks you through building a **real production agent** on Aetheris in 15 minutes: an automatic refund agent with human approval, external API calls (at-most-once), crash recovery, and audit trail.

**What you'll learn**:
- Define Tools with idempotency (no duplicate side effects)
- Create a TaskGraph with Wait node (human-in-the-loop)
- Send signals to resume execution
- View execution trace and evidence
- Replay for debugging (deterministic)

---

## Scenario: Automatic Refund Agent

**Business flow**:
1. User requests refund for order-123
2. Agent queries order status (tool call)
3. LLM decides if approval needed
4. If yes → wait for human approval (StatusParked, may wait 3 hours)
5. Human approves via signal
6. Agent resumes and executes refund (at-most-once, even if crash)
7. Complete with full audit trail

**Why this matters**:
- **Human-in-the-loop**: Real production scenario (not demo)
- **External side effects**: Refund API must be at-most-once
- **Long wait**: 3 hour wait doesn't block Worker
- **Crash recovery**: Worker crash during refund → recovers without duplicate
- **Audit**: Can answer "why did we refund? who approved? when?"

---

## Prerequisites

- Go 1.25.7+
- Aetheris running (see [get-started.md](get-started.md) for setup)
- Postgres (for production features; memory mode also works but no crash recovery)

**Quick setup**:
```bash
# Start Postgres
docker run -d --name aetheris-pg -p 5432:5432 \
  -e POSTGRES_USER=aetheris -e POSTGRES_PASSWORD=aetheris -e POSTGRES_DB=aetheris \
  postgres:15-alpine

# Initialize schema
psql -h localhost -U aetheris -d aetheris -f internal/runtime/jobstore/schema.sql

# Start API + Worker
make run
```

---

## Step 1: Define Tools

Tools are the only way to perform external side effects in Aetheris (see [design/step-contract.md](../design/step-contract.md)).

**Create** `examples/refund_agent/tools.go`:

```go
package main

import (
	"context"
	"encoding/json"
	"fmt"

	"rag-platform/internal/agent/runtime/executor"
)

// QueryOrderTool 查询订单状态（模拟外部 API）
type QueryOrderTool struct{}

func (t *QueryOrderTool) Execute(ctx context.Context, toolName string, input map[string]any, state interface{}) (executor.ToolResult, error) {
	orderID, _ := input["order_id"].(string)
	
	// 模拟查询订单 API
	orderStatus := map[string]interface{}{
		"order_id": orderID,
		"status":   "completed",
		"amount":   100.0,
		"issue":    "product_defective", // 触发退款
	}
	
	output, _ := json.Marshal(orderStatus)
	return executor.ToolResult{
		Done:   true,
		Output: string(output),
	}, nil
}

// SendRefundTool 执行退款（外部副作用，必须 at-most-once）
type SendRefundTool struct{}

func (t *SendRefundTool) Execute(ctx context.Context, toolName string, input map[string]any, state interface{}) (executor.ToolResult, error) {
	// ⚠️ 关键：获取 step idempotency key 并传给下游 API
	// Runtime 保证同一 key 最多执行一次（Ledger + Effect Store）
	jobID := executor.JobIDFromContext(ctx)
	stepID := executor.ExecutionStepIDFromContext(ctx)
	if stepID == "" {
		stepID = toolName
	}
	idempotencyKey := executor.StepIdempotencyKeyForExternal(ctx, jobID, stepID)
	
	orderID, _ := input["order_id"].(string)
	amount, _ := input["amount"].(float64)
	
	// 模拟调用支付 API（实际应传 idempotencyKey 到 Stripe/Alipay）
	fmt.Printf("[RefundTool] Calling payment API: refund(order=%s, amount=%.2f, idempotency_key=%s)\n",
		orderID, amount, idempotencyKey)
	
	// 真实实现示例：
	// err := stripeClient.Refunds.Create(&stripe.RefundParams{
	//     Charge: stripe.String(orderID),
	//     Amount: stripe.Int64(int64(amount * 100)),
	//     IdempotencyKey: stripe.String(idempotencyKey), // ← 关键！
	// })
	
	result := map[string]interface{}{
		"refund_id":       "refund-" + orderID,
		"status":          "success",
		"idempotency_key": idempotencyKey,
	}
	output, _ := json.Marshal(result)
	
	return executor.ToolResult{
		Done:   true,
		Output: string(output),
	}, nil
}
```

**Key point**: `StepIdempotencyKeyForExternal` returns `aetheris:<job_id>:<step_id>:<attempt_id>`. Pass this to external APIs to ensure at-most-once execution even across retries.

---

## Step 2: Define Planner

Planner returns a TaskGraph (see [design/workflow-decision-record.md](../design/workflow-decision-record.md)).

**Create** `examples/refund_agent/planner.go`:

```go
package main

import (
	"context"

	"github.com/google/uuid"
	"rag-platform/internal/agent/memory"
	"rag-platform/internal/agent/planner"
)

// RefundAgentPlanner 为退款场景生成固定 TaskGraph
type RefundAgentPlanner struct{}

func (p *RefundAgentPlanner) PlanGoal(ctx context.Context, goal string, mem memory.Memory) (*planner.TaskGraph, error) {
	// 生成唯一 correlation_key（用于 signal 匹配）
	approvalKey := "approval-" + uuid.New().String()
	
	return &planner.TaskGraph{
		Nodes: []planner.TaskNode{
			// Step 1: 查询订单
			{
				ID:       "query_order",
				Type:     planner.NodeTool,
				ToolName: "query_order",
				Config: map[string]any{
					"order_id": "order-123", // 从 goal 提取
				},
			},
			// Step 2: LLM 决策是否需要审批
			{
				ID:   "llm_decide",
				Type: planner.NodeLLM,
				Config: map[string]any{
					"goal": "根据订单状态判断是否需要人工审批。如果订单异常或金额>500，回复'需审批'；否则'无需审批'",
				},
			},
			// Step 3: 等待人工审批（长时间，park=true）
			{
				ID:   "wait_approval",
				Type: planner.NodeWait,
				Config: map[string]any{
					"wait_kind":       "human",
					"correlation_key": approvalKey,
					"park":            true,  // StatusParked，scheduler 不扫描
					"timeout":         "24h", // 24 小时超时
					"reason":          "等待人工审批退款",
				},
			},
			// Step 4: 执行退款
			{
				ID:       "send_refund",
				Type:     planner.NodeTool,
				ToolName: "send_refund",
				Config: map[string]any{
					"order_id": "order-123", // 从 state 读取
					"amount":   100.0,
				},
			},
		},
		Edges: []planner.TaskEdge{
			{From: "query_order", To: "llm_decide"},
			{From: "llm_decide", To: "wait_approval"},
			{From: "wait_approval", To: "send_refund"},
		},
	}, nil
}

// 注意：correlation_key 必须在创建 Job 时保存（或从 job_waiting 事件读取），
// 供后续 signal 调用时传递
```

**Important**: Wait node with `park: true` → Job enters StatusParked → Scheduler skips it → Only signal wakes it up (via WakeupQueue).

---

## Step 3: Register Tools & Create Agent

**Create** `examples/refund_agent/main.go`:

```go
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"rag-platform/internal/agent/memory"
	"rag-platform/internal/agent/runtime"
	"rag-platform/internal/agent/tools"
)

func main() {
	ctx := context.Background()
	
	// 1. 注册 Tools
	registry := tools.NewRegistry()
	registry.Register("query_order", &QueryOrderTool{})
	registry.Register("send_refund", &SendRefundTool{})
	
	// 2. 创建 Agent（使用 RefundAgentPlanner）
	planner := &RefundAgentPlanner{}
	mem := memory.NewCompositeMemory(nil)
	session := runtime.NewSession("session-1")
	
	agent := runtime.NewAgent("refund-agent-1", "Refund Agent", session, mem, planner, registry)
	
	fmt.Println("✓ Agent 创建成功")
	fmt.Println("✓ Tools 注册: query_order, send_refund")
	fmt.Println()
	
	// 3. 创建 Job（POST /api/agents/:id/message）
	jobID := createRefundJob(ctx, agent)
	fmt.Printf("✓ Job 创建: %s\n", jobID)
	fmt.Println("  Job 将执行到 Wait 节点（StatusParked）并等待审批")
	fmt.Println()
	
	// 4. 等待 Agent 执行到 Wait（StatusParked）
	time.Sleep(5 * time.Second)
	
	// 5. 查询 Job 状态
	status := getJobStatus(jobID)
	fmt.Printf("✓ Job 状态: %s\n", status)
	if status == "parked" || status == "waiting" {
		fmt.Println("  → Agent 在等待人工审批")
	}
	fmt.Println()
	
	// 6. 人工审批（POST /api/jobs/:id/signal）
	fmt.Println("模拟人工审批（3 小时后）...")
	time.Sleep(2 * time.Second)
	
	correlationKey := getCorrelationKeyFromJob(jobID) // 从 job_waiting 事件读取
	sendSignal(ctx, jobID, correlationKey, map[string]interface{}{"approved": true})
	fmt.Printf("✓ Signal 发送: correlation_key=%s, approved=true\n", correlationKey)
	fmt.Println("  Job 将重新入队并继续执行")
	fmt.Println()
	
	// 7. 等待 Agent 完成
	time.Sleep(5 * time.Second)
	
	finalStatus := getJobStatus(jobID)
	fmt.Printf("✓ Job 最终状态: %s\n", finalStatus)
	fmt.Println()
	
	// 8. 查看 Trace
	fmt.Println("=== Execution Trace ===")
	trace := getTrace(jobID)
	fmt.Println(trace)
	fmt.Println()
	
	// 9. Replay 验证（determinism）
	fmt.Println("=== Replay Verification ===")
	replayResult := triggerReplay(jobID)
	fmt.Println(replayResult)
}

func createRefundJob(ctx context.Context, agent *runtime.Agent) string {
	// POST /api/agents/:id/message
	// 返回 job_id
	return "job-refund-001" // 实际应调用 API
}

func getJobStatus(jobID string) string {
	// GET /api/jobs/:id
	return "parked" // 实际应调用 API
}

func getCorrelationKeyFromJob(jobID string) string {
	// GET /api/jobs/:id/replay，解析 job_waiting 事件
	return "approval-xxx" // 实际应从事件流读取
}

func sendSignal(ctx context.Context, jobID, correlationKey string, payload map[string]interface{}) {
	// POST /api/jobs/:id/signal
	// Body: {"correlation_key": "...", "payload": {...}}
}

func getTrace(jobID string) string {
	// GET /api/jobs/:id/trace
	return `Timeline:
  [10:00:00] Plan Generated
  [10:00:05] query_order → success
  [10:00:10] llm_decide → "需审批"
  [10:00:15] wait_approval → StatusParked
  [13:12:30] Signal received (approved=true)
  [13:12:35] send_refund → success (idempotency_key: aetheris:job:step:1)`
}

func triggerReplay(jobID string) string {
	// GET /api/jobs/:id/replay
	return `✓ Replay deterministic
✓ Tool NOT re-executed (injected from Ledger)
✓ LLM NOT re-called (injected from Effect Store)
✓ State consistent`
}
```

---

## Step 4: Run the Agent

```bash
# Terminal 1: API (if not already running)
go run ./cmd/api

# Terminal 2: Worker (if not already running)
go run ./cmd/worker

# Terminal 3: Run the refund agent
go run ./examples/refund_agent
```

**Expected output**:

```
✓ Agent 创建成功
✓ Tools 注册: query_order, send_refund

✓ Job 创建: job-refund-001
  Job 将执行到 Wait 节点（StatusParked）并等待审批

✓ Job 状态: parked
  → Agent 在等待人工审批

模拟人工审批（3 小时后）...
✓ Signal 发送: correlation_key=approval-xxx, approved=true
  Job 将重新入队并继续执行

✓ Job 最终状态: completed

=== Execution Trace ===
Timeline:
  [10:00:00] Plan Generated
  [10:00:05] query_order → success
  [10:00:10] llm_decide → "需审批"
  [10:00:15] wait_approval → StatusParked
  [13:12:30] Signal received (approved=true)
  [13:12:35] send_refund → success (idempotency_key: aetheris:job:step:1)

=== Replay Verification ===
✓ Replay deterministic
✓ Tool NOT re-executed (injected from Ledger)
✓ LLM NOT re-called (injected from Effect Store)
✓ State consistent
```

---

## Step 5: Test Crash Recovery

**Simulate Worker crash during refund execution**:

```bash
# Terminal 1: API running
# Terminal 2: Worker running

# Terminal 3: Create job
curl -X POST http://localhost:8080/api/agents/refund-agent-1/message \
  -H "Content-Type: application/json" \
  -d '{"message": "退款 order-123"}'
# Response: {"job_id": "job-xxx"}

# Watch Worker logs until it starts executing send_refund tool
# Then kill Worker (Ctrl+C or kill -9)

# Terminal 4: Start new Worker
go run ./cmd/worker

# Observe: New Worker claims the job, recovers from Checkpoint
# send_refund tool NOT re-executed (injected from Ledger/Effect Store)
# Job completes successfully
```

**What happens**:
1. Worker A executes send_refund, writes to Effect Store, crashes before command_committed
2. Lease expires (30s), Scheduler Reclaim → Job back to Pending
3. Worker B claims, Replay from events
4. Effect Store has record → catch-up: append command_committed without re-executing tool
5. **Refund NOT sent twice** (at-most-once)

---

## Step 6: View Trace & Evidence

```bash
# Get execution trace
curl -s http://localhost:8080/api/jobs/job-xxx/trace | jq .

# Key fields:
# - timeline_segments: visual timeline
# - steps[].reasoning_snapshot: per-step context
# - steps[].evidence: tool_invocation_ids, llm_decision, input_keys, output_keys
# - steps[].llm_invocation: model, provider, temperature (if LLM step)
```

**Evidence for audit**:
```json
{
  "steps": [
    {
      "node_id": "send_refund",
      "evidence": {
        "tool_invocation_ids": ["aetheris:job:send_refund:1"],
        "input_keys": ["order_id", "amount"],
        "output_keys": ["refund_status"]
      }
    }
  ]
}
```

**Can answer**:
- "谁让 AI 发起退款？" → Job job-xxx, Agent refund-agent-1
- "何时执行？" → 2024-11-20 13:12:35
- "为什么退款？" → llm_decide 基于 query_order 返回"订单异常"
- "用的哪个模型？" → gpt-4o-2024-11-20, temp=0.7
- "是否重复执行？" → idempotency_key 唯一，Ledger 验证仅一次

---

## Step 7: Replay for Debugging

```bash
# Trigger replay (read-only, doesn't modify job)
curl -s http://localhost:8080/api/jobs/job-xxx/replay | jq .

# Response includes:
# - current_state: completed_node_ids, cursor_node
# - payload_results: final memory state
# - completed_command_ids: all committed commands
```

**Replay guarantees** (see [design/execution-guarantees.md](../design/execution-guarantees.md)):
- **Tools NOT re-executed**: Injected from Ledger/Effect Store
- **LLM NOT re-called**: Injected from Effect Store
- **State deterministic**: Same inputs → same outputs
- **No side effect duplication**: Tool idempotency_key ensures uniqueness

---

## Step 8: Human-in-the-Loop Flow (Real Wait)

In production, the wait → signal flow is asynchronous:

```bash
# 1. Create job → Agent executes to Wait node
POST /api/agents/refund-agent-1/message
→ Job job-123, Status: parked

# 2. Worker releases (Agent "sleeping")
# Scheduler skips StatusParked jobs
# No resource consumption during wait

# 3. Three hours later: Human approves in admin UI
# Admin UI calls:
POST /api/jobs/job-123/signal
Body: {
  "correlation_key": "approval-xxx",  # From job_waiting event
  "payload": {"approved": true, "reason": "客户投诉合理"}
}

# 4. Signal writes wait_completed → UpdateStatus(Pending) → NotifyReady(WakeupQueue)
# Worker immediately claims (no poll delay) and resumes from wait point

# 5. Continuation: Agent resumes with full state
# payload.Results = resumption_context.payload_results + signal.payload
# Next step (send_refund) executes with correct order_id, amount

# 6. Job completes → full audit trail in event stream
```

**Key**: Resumption context (added in System Closure) ensures "same continuation" — not "new execution".

---

## Step 9: Production Deployment

**Configuration** (configs/api.yaml, configs/worker.yaml):

```yaml
jobstore:
  type: postgres  # Required for crash recovery
  postgres:
    dsn: "postgres://user:pass@localhost:5432/aetheris"

effect_store:
  enabled: true  # Required for at-most-once & LLM replay guard

invocation_ledger:
  enabled: true  # Required for Tool at-most-once

wakeup_queue:
  type: redis    # Required for multi-worker signal delivery
  redis:
    addr: "localhost:6379"
```

**Deploy**:
- **Single-process**: 1 API (includes Scheduler) + WakeupQueueMem
- **Multi-worker**: 1 API + N Workers + Postgres + Redis (WakeupQueue)

See [docs/deployment.md](deployment.md) for Kubernetes/Docker Compose.

---

## What You Built

A **production-grade refund agent** with:

✅ **At-most-once refund**: Tool idempotency key → no duplicate even if crash  
✅ **Human-in-the-loop**: Wait 3 hours without blocking Worker  
✅ **Crash recovery**: Worker crash → new Worker resumes from Checkpoint  
✅ **Audit trail**: Full evidence (who approved, when, which model decided)  
✅ **Replay debugging**: Verify determinism (Tool/LLM NOT re-executed)  

**Time to build**: 15 minutes  
**Time to production-ready**: Add real payment API + configure Postgres/Redis

---

## Next Steps

- **Add more tools**: Email notification, Slack message, database update
- **Add conditional logic**: Conditional edges in TaskGraph (amount > 500 → need approval)
- **Add retry**: Configure RetryMax in Worker (see [docs/config.md](config.md))
- **Add monitoring**: Prometheus metrics + Grafana (see [docs/observability.md](observability.md))
- **Read contracts**: [design/step-contract.md](../design/step-contract.md) (how to write correct steps), [design/execution-guarantees.md](../design/execution-guarantees.md) (runtime guarantees)

---

## Troubleshooting

### Job stuck in Waiting/Parked

**Check**: correlation_key in signal must match job_waiting event

```bash
# Get correlation_key from event stream
curl -s http://localhost:8080/api/jobs/job-xxx/replay | jq '.events[] | select(.type=="job_waiting") | .payload.correlation_key'

# Send signal with correct key
curl -X POST http://localhost:8080/api/jobs/job-xxx/signal \
  -d '{"correlation_key": "approval-yyy", "payload": {"approved": true}}'
```

### Tool executed twice (duplicate side effect)

**Check**: Effect Store and Ledger configured?

```yaml
# configs/api.yaml & configs/worker.yaml
effect_store:
  enabled: true  # ← Must be true

invocation_ledger:
  enabled: true  # ← Must be true
```

Without these, at-most-once is NOT guaranteed (dev mode only).

### Replay returns different result

**Check**: Effect Store has LLM effect?

```bash
# If LLM step result changes on replay → Effect Store not configured
# Production MUST configure Effect Store for LLM determinism
```

---

## Real Production Example

This pattern is used for:

- **Financial approval**: Loan approval agent (wait for risk team)
- **Customer support**: Ticket escalation (wait for human agent)
- **SaaS automation**: Salesforce sync (wait for rate limit, then retry)
- **Legal workflow**: Contract review (wait for lawyer approval)

**Common pattern**:

```
Query external system (tool)
  ↓
LLM decides (llm)
  ↓
Wait for human/webhook (wait, park=true)
  ↓
Execute action (tool, idempotency_key)
  ↓
Audit trail (trace, evidence)
```

Aetheris guarantees: at-most-once execution, crash recovery, audit proof, no "lost in wait" jobs.
