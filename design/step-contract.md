# Step Semantic Contract — 如何编写正确的 Step

Aetheris 提供 at-most-once、deterministic replay、crash recovery 等运行时保证，**前提是开发者遵守 Step 编写契约**。违反契约会导致：副作用重复执行、replay 不确定、状态污染。

本文档定义 **Step 的执行语义边界**：哪些行为允许、哪些禁止、如何正确实现外部副作用。

---

## 核心原则

### 运行时保证的前提

Aetheris 保证（见 [execution-guarantees.md](execution-guarantees.md)）：

* **Step 至多执行一次**（at-most-once）— 配置 InvocationLedger + Effect Store 时
* **Replay 不改变行为**（deterministic）— Replay 时注入已记录结果，不重新执行
* **崩溃后不重复副作用**— 两步提交 + Effect Store catch-up

**这些保证仅在开发者遵守以下契约时成立**：

> **Step 必须是确定性的纯函数 + 外部副作用通过 Tool/Runtime 注入**

违反契约 = 运行时保证失效。

---

## 禁止行为（Forbidden）

以下行为会破坏确定性与 at-most-once 保证：

### ❌ 1. 直接调用外部系统

**错误示例**：

```go
func SendInvoice(ctx context.Context, customer string) error {
    // ❌ 直接 HTTP 调用 → replay 时会重发
    resp, err := http.Post("https://api.stripe.com/v1/invoices", ...)
    return err
}
```

**为什么错误**：
- Replay 时会再次执行 `http.Post`，导致客户收到两次账单
- 崩溃重试时 Step 可能执行两次（at-most-once 失效）

**正确做法**：通过 Tool（见 § 强制行为）。

---

### ❌ 2. 读取 wall-clock time

**错误示例**：

```go
func ScheduleTask(ctx context.Context) error {
    // ❌ 直接读系统时间 → replay 时结果不同
    now := time.Now()
    deadline := now.Add(24 * time.Hour)
    saveDeadline(deadline) // 第一次执行与 replay 时 deadline 不同
    return nil
}
```

**为什么错误**：
- Replay 时 `time.Now()` 返回不同值，导致行为不一致
- 违反确定性：同一事件流 replay 两次应产生相同结果

**正确做法**：时间通过 Runtime 注入（见 § 强制行为）。

---

### ❌ 3. 使用随机数

**错误示例**：

```go
func AssignWorker(ctx context.Context, tasks []Task) error {
    // ❌ 随机分配 → replay 时分配结果不同
    workerID := rand.Intn(len(workers))
    assignTask(tasks[0], workers[workerID])
    return nil
}
```

**为什么错误**：
- Replay 时 `rand.Intn` 返回不同值
- 第一次执行与 replay 行为不一致

**正确做法**：随机数通过 Runtime seed 注入（未来支持）或由 Planner 决策（确定性写入 PlanGenerated）。

---

### ❌ 4. 读取非确定性外部状态

**错误示例**：

```go
func CheckInventory(ctx context.Context, sku string) error {
    // ❌ 直接查数据库 → replay 时库存可能已变化
    stock := db.Query("SELECT stock FROM inventory WHERE sku = ?", sku)
    if stock < 10 {
        triggerReorder()
    }
    return nil
}
```

**为什么错误**：
- 第一次执行库存=5，replay 时库存=100，行为不同
- 外部世界状态变化导致 replay 不确定

**正确做法**：外部查询通过 Tool，Runtime 记录结果（见 § 强制行为）。

---

### ❌ 5. Step 内修改全局状态

**错误示例**：

```go
var globalCounter int // ❌ 全局可变状态

func ProcessItem(ctx context.Context, item Item) error {
    globalCounter++ // ❌ replay 时 counter 会再次增加
    log.Printf("Processed %d items", globalCounter)
    return nil
}
```

**为什么错误**：
- Replay 时全局状态已改变，行为不一致
- 多 Worker 并发执行时全局状态不可靠

**正确做法**：状态存储在 Agent Memory（通过 payload），由 Runtime 管理。

---

## 强制行为（Required）

以下是编写正确 Step 的**唯一**方式：

### ✅ 1. 外部副作用必须通过 Tool

**正确示例**：

```go
// Tool 实现（在 Tool 注册表中）
type EmailTool struct{}

func (t *EmailTool) Execute(ctx context.Context, toolName string, input map[string]any, state interface{}) (executor.ToolResult, error) {
    // Step idempotency key 由 Runtime 注入（见 design/effect-system.md）
    key := executor.StepIdempotencyKeyForExternal(ctx, jobID, stepID)
    
    // 将 key 传给下游 API（确保 at-most-once）
    err := emailAPI.Send(key, input["to"].(string), input["subject"].(string))
    
    return executor.ToolResult{
        Done:   true,
        Output: "email sent",
        Err:    errToString(err),
    }, nil
}

// Step 定义（在 TaskGraph 中）
func SendEmailStep(ctx context.Context, p *executor.AgentDAGPayload) error {
    // ✅ 通过 Tool 调用，Runtime 保证 at-most-once
    // Tool 执行前 Runtime 写入 tool_invocation_started（idempotency barrier）
    // Replay 时 Runtime 注入已记录结果，不再次执行
    return nil // Tool 由 eino DAG 自动调用
}
```

**为什么正确**：
- Runtime 在 Tool 执行前写入 `tool_invocation_started`（Activity Log Barrier，见 [effect-system.md](effect-system.md)）
- Replay 时 Runtime 从 Ledger/Effect Store 注入已记录结果，**不调用** Tool
- `StepIdempotencyKeyForExternal` 包含 `(job_id, step_id, attempt_id)`，下游 API 据此去重

---

### ✅ 2. 时间必须通过 Runtime 注入

**正确示例**：

```go
// 方案 A：时间由 Planner 决策时确定（写入 PlanGenerated）
// TaskNode config 中包含 deadline
config := map[string]any{
    "deadline": "2024-12-01T00:00:00Z", // Planner 写入，replay 时从事件流读取
}

// 方案 B：时间通过 Tool 获取（Runtime 记录结果）
func GetCurrentTimeTool(ctx context.Context, ...) (executor.ToolResult, error) {
    now := time.Now() // Tool 内可读系统时间，但 Runtime 会记录结果
    return executor.ToolResult{
        Done:   true,
        Output: now.Format(time.RFC3339),
    }, nil
}
// Step 调用 GetCurrentTimeTool；Replay 时 Runtime 注入已记录时间
```

**为什么正确**：
- 时间值写入事件流（PlanGenerated 或 tool_invocation_finished）
- Replay 时从事件读取，不重新调用 `time.Now()`

**Recommended: use runtime-injected clock and RNG**

The runtime package provides helpers so steps can use a replay-safe clock and RNG without reading the event stream explicitly:

- **`runtime.Clock(ctx)`** — Returns the current time. When the Runner is in replay mode, it injects a deterministic clock (derived from jobID + stepID) so that `Clock(ctx)` returns the same value on replay. Use this instead of `time.Now()` inside steps.
- **`runtime.RandIntn(ctx, n)`** — Returns a random int in `[0, n)`. When the Runner is in replay mode, it injects a deterministic RNG seeded from jobID + stepID. Use this instead of `rand.Intn(n)` inside steps.

Context keys and injection are in `internal/agent/runtime/contract.go`. The Runner injects the clock (and in replay, the RNG) before calling `step.Run`. Steps that use only these helpers and Tools satisfy the contract for deterministic replay.

**StepValidator (2.0)** — Optional pluggable validation can check that a step does not violate the contract (no direct `time.Now`, `rand.*`, or `net/http` in step code). The interface is defined in `internal/agent/runtime/executor/validator.go`:

- **`StepValidator.ValidateStep(ctx, req)`** — `req` carries jobID, stepID, nodeID, nodeType and an optional `RunInSandbox(ctx) error` closure. Implementations may run the step in a sandbox (e.g. with detectors for forbidden calls) or use static analysis; recommend test-time use first to avoid slowing production.
- The Runner can register one or more validators via `Runner.SetStepValidators(...)`; if any validator returns an error before a step runs, the step is treated as a contract violation (e.g. permanent failure). When no validators are set, behavior is unchanged.

See § StepValidator (2.0) below for details.

---

### ✅ 3. Memory 读取必须纯函数式

**正确示例**：

```go
func ProcessUserData(ctx context.Context, p *executor.AgentDAGPayload) (*executor.AgentDAGPayload, error) {
    // ✅ 从 payload（Agent Memory）读取状态
    userName := p.Results["user_name"].(string)
    
    // ✅ 纯计算
    greeting := fmt.Sprintf("Hello, %s", userName)
    
    // ✅ 写回 payload（新状态）
    if p.Results == nil {
        p.Results = make(map[string]any)
    }
    p.Results["greeting"] = greeting
    
    return p, nil
}
```

**为什么正确**：
- 输入来自 `payload`（已在事件流中记录）
- 输出写入 `payload.Results`，由 Runtime 记录为 `state_after`
- Replay 时直接注入 `state_after`，不重新执行

---

### ✅ 4. LLM 调用必须通过 LLMNodeAdapter

**正确示例**：

```go
// Planner 返回 TaskNode，type = "llm"
TaskNode{
    ID:       "summarize",
    NodeType: planner.NodeLLM,
    Config:   map[string]any{"goal": "Summarize the document"},
}

// Runtime 自动使用 LLMNodeAdapter：
// 1. 调用 LLM.Generate
// 2. 写入 Effect Store（prompt + response）
// 3. 写入 command_committed
// Replay 时从 Effect Store 注入，不重新调用 LLM
```

**为什么正确**：
- LLM 调用由 Runtime 管理，写入 Effect Store（见 [effect-system.md](effect-system.md) § LLM Effect Capture）
- Replay 时 **绝不调用 LLM**，仅注入已记录 response

---

## 实现模式（Patterns）

### Pattern 1: 纯计算 Step

```go
func CalculateDiscount(ctx context.Context, p *executor.AgentDAGPayload) (*executor.AgentDAGPayload, error) {
    price := p.Results["price"].(float64)
    discount := price * 0.1 // 纯计算，replay 时结果相同
    p.Results["discount"] = discount
    return p, nil
}
```

**特点**：无外部依赖，输入 → 输出确定性映射；Replay 可安全重放（result_type = Pure）。

---

### Pattern 2: Tool 调用 Step（副作用）

```go
// TaskGraph 定义
TaskNode{
    ID:       "send_email",
    NodeType: planner.NodeTool,
    ToolName: "email",
    Config:   map[string]any{"to": "user@example.com", "subject": "Invoice"},
}

// Tool 实现
func (t *EmailTool) Execute(ctx context.Context, toolName string, input map[string]any, state interface{}) (executor.ToolResult, error) {
    key := executor.StepIdempotencyKeyForExternal(ctx, jobID, stepID)
    err := emailAPI.Send(key, input["to"].(string), input["subject"].(string))
    return executor.ToolResult{Done: true, Output: "sent", Err: errToString(err)}, nil
}
```

**特点**：外部副作用由 Tool 封装；Runtime 保证 at-most-once（Ledger + Effect Store）；Replay 注入已记录结果。

---

### Pattern 3: Wait Step（阻塞与恢复）

```go
// TaskGraph 定义
TaskNode{
    ID:       "wait_approval",
    NodeType: planner.NodeWait,
    Config: map[string]any{
        "wait_kind":       "human",
        "correlation_key": "approval-123",
        "timeout":         "72h",
        "park":            true, // 长时间等待，scheduler 不扫描
    },
}

// Runtime 行为：
// 1. 写入 job_waiting 事件
// 2. Job 状态 → StatusParked（scheduler 跳过）
// 3. 收到 signal（POST /api/jobs/:id/signal，correlation_key=approval-123）
// 4. 写入 wait_completed，Job 状态 → Pending，Worker 认领继续
```

**特点**：Blocking 语义；外部 signal 触发 resume；Replay 时从 `wait_completed` 注入 payload。

---

## 违反契约的后果

### 后果 1：副作用重复执行

**场景**：Step 直接调用 `http.Post` 发送邮件；Worker 崩溃后 reclaim，新 Worker replay 并再次执行该 Step。

**结果**：客户收到两封邮件；Stripe 扣款两次。

**原因**：Runtime 无法识别"该 Step 已执行"（因为未通过 Tool + Ledger），replay 时再次执行。

---

### 后果 2：Replay 行为不一致

**场景**：Step 读取 `time.Now()` 或 `rand.Int()`；第一次执行与 replay 时返回值不同。

**结果**：
- 第一次执行：分配 worker A
- Replay 时：分配 worker B（随机数不同）
- 审计无法解释"为什么两次结果不同"

**原因**：非确定性输入未由 Runtime 记录。

---

### 后果 3：状态污染

**场景**：Step 修改全局变量或数据库（不通过 Tool）；replay 时全局状态已改变。

**结果**：
- 第一次执行：`globalCounter = 1`
- Replay 时：`globalCounter = 2`（已被第一次执行改变）
- 行为不一致

**原因**：全局状态不在 Runtime 管理范围内。

---

## 测试与验证

### 单测：Replay 确定性

```go
func TestStepDeterminism(t *testing.T) {
    // 构造 replay context（含已记录事件）
    replayCtx := &executor.ReplayContext{
        CompletedCommandIDs: map[string]bool{"step1": true},
        CommandResults:      map[string][]byte{"step1": resultBytes},
    }
    
    // 第一次执行
    result1, _ := runner.RunForJob(ctx, job)
    
    // Replay（注入已记录结果）
    result2, _ := runner.RunForJob(replayCtx, job)
    
    // 断言：结果必须相同
    require.Equal(t, result1, result2, "Replay must be deterministic")
}
```

---

### 集成测试：Tool at-most-once

```go
func TestToolAtMostOnce(t *testing.T) {
    callCount := 0
    mockTool := &MockTool{
        Execute: func(ctx context.Context, ...) (executor.ToolResult, error) {
            callCount++
            return executor.ToolResult{Done: true, Output: "ok"}, nil
        },
    }
    
    // 执行 → 崩溃 → reclaim → replay
    job := createJob()
    runner.RunForJob(ctx, job) // 第一次执行
    simulateCrash()
    runner.RunForJob(replayCtx, job) // Replay
    
    // 断言：Tool 仅执行一次
    require.Equal(t, 1, callCount, "Tool must execute at-most-once")
}
```

---

## StepValidator (2.0)

Optional pre-step or test-time check that a step does not violate the contract (no direct `time.Now`, `rand.*`, or `net/http` in step code).

### Interface

Defined in `internal/agent/runtime/executor/validator.go`:

```go
type StepValidator interface {
    ValidateStep(ctx context.Context, req StepValidationRequest) error
}

type StepValidationRequest struct {
    JobID, StepID, NodeID, NodeType string
    RunInSandbox func(ctx context.Context) error  // optional: run step in sandbox to detect forbidden calls
}
```

### Semantics

- **Purpose**: Catch contract violations before they cause non-deterministic replay or duplicate side effects.
- **Run in sandbox**: If the validator has access to the step closure, it can run it in a context where `time.Now`, `rand.Intn`, and `http.DefaultClient` are wrapped; if the step calls them, the validator returns an error. This is recommended for tests first; production may leave validators unset.
- **Static analysis**: A validator could instead accept source or AST and reject forbidden symbols; not required for 2.0.
- **Optional**: When no validators are registered, the Runner behaves as today (no validation).

### Default validator implementations

- **NoOpValidator** (`executor.NoOpValidator`): Always returns nil. Use when validation is disabled (e.g. production).
- **SandboxRunValidator** (`executor.NewSandboxRunValidator()`): When the Runner passes `RunInSandbox`, runs the step inside the validator; useful for test-time validators that run the step with a custom context (e.g. detector Clock/RNG). Detection of direct `time.Now`/`rand.*`/`net/http` from inside step code requires running the step in an environment where those are wrapped (e.g. in tests, replace them in the step’s package or use a test double). See `internal/agent/runtime/executor/validator.go` and `contract_test.go`.
- **ErrContractViolation**: Sentinal error (`executor.ErrContractViolation`) for contract violations.

### Usage

- Register validators on the Runner via `SetStepValidators(...)`. Before each step run, the Runner calls each validator; if any returns an error, the step is not executed and the job can be marked failed (contract violation).
- Use `runtime.Clock(ctx)` and `runtime.RandIntn(ctx, n)` and Tools so that steps pass validation and remain deterministic on replay.

---

## Async event handling (2.0)

Steps may emit **async events** (e.g. "request sent", "approval requested") that are recorded and replayed without re-executing the step.

### Contract

- **Recording**: When a step calls `EmitAsyncEvent(ctx, name, payload)`, the runtime appends the event to the trace (if a sink is configured). Events are tied to the current job and step (from context).
- **Replay**: On replay, the runner does not re-execute the step; it may replay previously recorded async events for trace consistency. Idempotency: handlers that process these events (e.g. webhooks) must use the same idempotency key as the step (job_id + step_id + attempt_id) so duplicate events do not cause duplicate side effects.
- **Relation to command_committed**: Async events are informational; at-most-once for side effects is still enforced via Tool + Ledger and `command_committed`. Async events do not replace the need for Tools for external calls.

### Helpers

- **Context key**: An async event sink can be attached to context (e.g. `WithAsyncEventSink(ctx, sink)`). Steps call `runtime.EmitAsyncEvent(ctx, name, payload)`; if no sink is set, it no-ops.
- Implementation: small helper in `pkg/effects` or `internal/agent/runtime`; Runner wires the sink when building the run context if trace/audit is enabled.

---

## 参考

- [execution-guarantees.md](execution-guarantees.md) — Runtime 保证与成立条件
- [effect-system.md](effect-system.md) — Effect 类型、Replay 协议、Tool/LLM 屏障
- [runtime-contract.md](runtime-contract.md) — 可重放边界、禁止行为、Epoch 校验
- [step-identity.md](step-identity.md) — Step Idempotency Key 计算与用途
- [Temporal: How to write Deterministic Workflow Code](https://docs.temporal.io/develop/go/determinism) — 参考实现

---

## TL;DR

**允许**：
- 纯计算（输入 → 输出）
- 从 `payload` 读取状态
- 通过 Tool 执行副作用
- 通过 LLMNodeAdapter 调用 LLM

**禁止**：
- 直接调用外部系统（HTTP、数据库、文件 IO）
- 读取 `time.Now()`、`rand.Int()`
- 修改全局状态
- 直接调用 LLM API

**核心**：
> Step 必须是确定性纯函数 + 外部副作用通过 Runtime 注入

遵守契约 → Runtime 保证成立；违反契约 → 副作用重复、replay 不一致、状态污染。
