# Tool & LLM Versioning — 世界变化与版本追踪

企业生产环境中，Tool API 会更新、LLM 模型会升级、Planner 逻辑会演进。Aetheris 必须记录执行时使用的版本，以便审计、排查、跨版本恢复。

---

## 问题

### Tool Schema Drift（工具漂移）

**场景**：
```bash
# 今天
/api/price?sku=123 → $10

# 一周后（API 更新）
/api/price?sku=123 → $12
```

**风险**：
- Replay 恢复旧结果（`$10`），但新 Agent 调用返回 `$12`
- 审计无法解释"为什么历史执行返回 X，现在返回 Y"
- Tool 实现版本变化（v1.0 → v1.2），行为不同

---

### LLM Model Drift（模型漂移）

**场景**：
```bash
# 2024-08-06
gpt-4o-2024-08-06: "Approve payment"

# 2024-11-20
gpt-4o-2024-11-20: "Reject payment" (same prompt)
```

**风险**：
- 相同 prompt 在不同模型版本下输出不同
- Replay 恢复旧输出，但新执行行为变化
- Provider 切换（OpenAI → Anthropic），model name 变化但事件流无记录

---

### Execution Version Mismatch（执行代码版本不匹配）

**场景**：
- Planner 更新：旧 Job 的 TaskGraph schema 变化
- Step 逻辑更新：新代码期望 Step X，旧事件流有 Step B
- Replay 时 unmarshal 失败或行为不一致

---

## 解决方案

### 1. Tool Schema Versioning

#### 1.1 Schema Hash 计算

Tool 的 **request schema** 和 **response schema** 必须可计算 hash（用于检测 schema 漂移）。

**实现**（可选，未来扩展）：
```go
// ToolVersionRegistry 接口（可选注入到 ToolNodeAdapter）
type ToolVersionRegistry interface {
    GetToolVersion(toolName string) (version, requestSchemaHash, responseSchemaHash string, err error)
}

// 计算 schema hash：JSON schema 序列化后 SHA256
func SchemaHash(schema interface{}) string {
    b, _ := json.Marshal(schema)
    h := sha256.Sum256(b)
    return "sha256:" + hex.EncodeToString(h[:16])
}
```

#### 1.2 Event Payload 增强

`tool_invocation_started` payload 增加：
```json
{
  "invocation_id": "...",
  "tool_name": "send_invoice",
  "idempotency_key": "...",
  "tool_version": "v1.2.0",               // Tool 实现版本
  "request_schema_hash": "sha256:abc..."  // 输入 schema hash
}
```

`tool_invocation_finished` payload 增加：
```json
{
  "invocation_id": "...",
  "outcome": "success",
  "result": {...},
  "response_schema_hash": "sha256:def..."  // 输出 schema hash
}
```

**用途**：
- 审计：知道执行时用的哪个 Tool 版本
- Schema Drift 检测：对比 request_schema_hash 与当前 schema，若不匹配则 warning
- Replay 行为解释：若 Replay 结果与新执行不同，可追溯到"Tool schema 已变化"

#### 1.3 实现位置

- **Payload 定义**：[internal/agent/runtime/executor/invocation.go](../internal/agent/runtime/executor/invocation.go) `ToolInvocationStartedPayload` / `ToolInvocationFinishedPayload` 增加字段（已实现）
- **写入**：[internal/agent/runtime/executor/node_adapter.go](../internal/agent/runtime/executor/node_adapter.go) `ToolNodeAdapter.runNodeExecute` 在 `AppendToolInvocationStarted` 时填充 `ToolVersion` / `RequestSchemaHash`（当 ToolVersionRegistry 可用时）
- **向后兼容**：字段可选（`omitempty`），未配置时为空；旧事件流无这些字段仍可正常 Replay

---

### 2. LLM Effect Versioning

#### 2.1 Model Info 记录

`command_committed` (LLM 步) payload 顶层增加：
```json
{
  "result": "...",
  "llm_model": "gpt-4o-2024-11-20",
  "llm_provider": "openai",
  "llm_temperature": 0.7,
  "prompt_hash": "sha256:abc...",   // 可选：用于判断"相同输入产生不同输出"（model drift 检测）
  "token_count": 1234               // 可选
}
```

**用途**：
- 审计：知道执行时用的哪个模型、哪个 provider
- Model Drift 检测：若 prompt_hash 相同但输出不同，可能是模型更新
- Replay 行为解释：若 Replay 恢复的输出与新执行不同，可追溯到"模型已升级"

#### 2.2 实现位置

- **LLMNodeAdapter**：[internal/agent/runtime/executor/node_adapter.go](../internal/agent/runtime/executor/node_adapter.go) 在 `AppendCommandCommitted` 时增加 `llm_model`、`llm_provider`、`llm_temperature`、`prompt_hash`
- **LLM 接口扩展**：[internal/model/llm](../internal/model/llm) 可选增加 `GetModelInfo() (name, provider, version string)` 方法（未来）
- **Effect Store**：Effect Store Metadata 已记录 model/temperature（见 [effect-system.md](effect-system.md)），需暴露到 `command_committed` payload 顶层供 Trace/API 直接访问

#### 2.3 Trace 展示

[internal/api/http/trace_narrative.go](../internal/api/http/trace_narrative.go) `StepNarrative` 增加：
```go
type StepNarrative struct {
    ...
    LLMInvocation *LLMInvocationSummary `json:"llm_invocation,omitempty"`
}

type LLMInvocationSummary struct {
    Model       string  `json:"model"`
    Provider    string  `json:"provider"`
    Temperature float64 `json:"temperature"`
    PromptHash  string  `json:"prompt_hash,omitempty"`
    TokenCount  int     `json:"token_count,omitempty"`
}
```

**实现**：`BuildNarrative` 从 `command_committed` (LLM 步) 或 Effect Store 填充；GET trace 暴露供 UI 展示。

---

### 3. Cross-Version Replay（跨版本恢复）

#### 3.1 Execution Version Binding

Job 创建时记录执行代码版本：
```json
{
  "job_id": "...",
  "execution_version": "v1.2.0",          // 执行代码版本（如 git tag）
  "planner_version": "planner-v2",        // Planner 版本（可选）
  "tool_registry_version": "tools-v3"     // Tool 集合版本（可选）
}
```

**实现**：
- **Job struct**：[internal/runtime/jobstore/store.go](../internal/runtime/jobstore/store.go) `Job` 增加 `ExecutionVersion`、`PlannerVersion` 字段
- **RunForJob 校验**：[internal/agent/runtime/executor/runner.go](../internal/agent/runtime/executor/runner.go) 开始时检查：若 `j.ExecutionVersion != ""` 且 `!= current_version`，warning（或可选 strict mode 拒绝）

#### 3.2 PlanGenerated Versioning

`plan_generated` payload 增加版本字段：
```json
{
  "task_graph": {...},
  "planner_version": "v2.1.0",
  "schema_version": "taskgraph-v1"
}
```

**用途**：
- Replay 时知道 Plan 是由哪个 Planner 版本生成
- TaskGraph schema 变化时，可区分"旧 schema"与"新 schema"

#### 3.3 Version Mismatch 策略

| 策略 | 行为 | 适用场景 |
|------|------|----------|
| **warning**（1.0 默认） | 记录 warning 日志，继续执行/Replay | 开发环境、版本兼容性强 |
| **fail** | 拒绝执行/Replay，返回错误 | 生产环境 strict mode |
| **auto-migrate**（未来） | 按 version 路由到旧代码或执行 migration | 长期演进，支持多版本共存 |

**实现**：
- Runner 开始时检查 `j.ExecutionVersion`，若不匹配且配置为 strict mode，返回 `ErrVersionMismatch`
- 可选：按 version 路由到不同 Planner/Compiler（2.0 特性）

---

## 实现优先级

| Order | Item | Rationale |
|-------|------|-----------|
| 1 | Tool Payload 字段增加（已完成） | 向后兼容，低风险；为审计奠定基础 |
| 2 | Versioning Design Doc（本文档） | 定义语义与契约 |
| 3 | LLM command_committed 增强 | 补全 LLM model/provider 记录 |
| 4 | Trace 暴露 LLM/Tool 版本 | UI 可展示"执行时用的哪个模型/工具" |
| 5 | Execution Version Binding | Job 绑定 execution_version |
| 6 | Version Mismatch 策略 | warning / fail / auto-migrate |

---

## Version Mismatch 示例

### 示例 1：Tool Schema 变化

```bash
# 第一次执行（2024-08-01）
tool_invocation_started: {
  tool_name: "send_email",
  tool_version: "v1.0.0",
  request_schema_hash: "sha256:abc123"
}
→ 成功发送邮件

# Replay（2024-11-01）
当前 Tool version: "v1.2.0"
当前 request_schema_hash: "sha256:def456"  # schema 已变化（增加新字段）

警告：Tool schema mismatch (v1.0.0 → v1.2.0, hash: abc123 → def456)
行为：Replay 注入旧结果（不重新调用），但 warning 记录"schema 已变化"
```

---

### 示例 2：LLM Model 升级

```bash
# 第一次执行（2024-08-06）
command_committed (LLM): {
  result: "Approve payment",
  llm_model: "gpt-4o-2024-08-06",
  llm_provider: "openai"
}

# Replay（2024-11-20）
当前 Model: "gpt-4o-2024-11-20"

警告：LLM model changed (gpt-4o-2024-08-06 → gpt-4o-2024-11-20)
行为：Replay 注入旧输出（"Approve payment"），但 warning 记录"模型已升级"
```

---

### 示例 3：Execution Version 不匹配

```bash
# Job 创建时（v1.0.0）
Job: {
  execution_version: "v1.0.0",
  planner_version: "planner-v1"
}

# Replay 时（v1.2.0）
当前 Runtime Version: "v1.2.0"

警告（或失败）：Execution version mismatch (v1.0.0 → v1.2.0)
行为（strict mode）：拒绝 Replay，返回 ErrVersionMismatch
行为（warning mode）：继续 Replay，但记录 warning
```

---

## 审计需求

企业审计要求回答：

1. **"该决策使用了哪个 LLM 模型？"** → `llm_model`、`llm_provider`
2. **"Tool 调用时用的哪个版本？"** → `tool_version`、`request_schema_hash`
3. **"为什么历史执行返回 X，现在返回 Y？"** → 对比 `tool_version`、`llm_model`、schema hash
4. **"旧 Job 能否在新代码上恢复？"** → `execution_version` mismatch 检测与策略

补全版本化后，Aetheris 可满足金融/医疗/政府系统的审计级要求。

---

## 参考

- [execution-guarantees.md](execution-guarantees.md) — 运行时保证
- [effect-system.md](effect-system.md) — Effect Store 与 LLM Effect Capture
- [execution-forensics.md](execution-forensics.md) — Evidence Graph 与审计
- [runtime-contract.md](runtime-contract.md) — Cross-Version Replay 契约
- [Temporal: Workflow Versioning](https://docs.temporal.io/workflows#versioning) — 参考实现
