# Observability

本页说明 **Job 执行可观测**（Execution Trace UI、时间线、节点延迟与重试原因）。HTTP 请求级 OpenTelemetry 见 [tracing.md](tracing.md)。

## Execution Trace UI

当 `jobstore.type` 为 `postgres` 或 API 使用事件存储时，可为单个 Job 打开 **Trace 回放页**，查看执行时间线、每步延迟、重试原因与节点输入输出。

### 如何打开

- **URL**：`GET /api/jobs/:job_id/trace/page`（需替换 `:job_id` 为实际 Job ID）。
- 示例：Job ID 为 `job-xxx` 时，在浏览器打开 `http://localhost:8080/api/jobs/job-xxx/trace/page`（若 API 启用了认证，需带 Cookie 或 Token）。
- **JSON 接口**：`GET /api/jobs/:job_id/trace` 返回时间线与执行树 JSON，供自定义前端或脚本使用。

### 页面内容

- **时间线条**：按时间顺序展示 plan、node、tool、recovery 等片段；每段可带 `duration_ms`、`status`（ok / failed / retryable）。
- **步骤列表**：左侧步骤列表，点击可在右侧查看该步的 **Step 详情**（node_id、result_type、reason、duration_ms、attempts）、**推理/决策**（若有）、**Tool 输入输出**、**状态变更 diff**（state_checkpointed）。
- **执行树**：可折叠的树形结构（job → plan → node → tool），与 [design/execution-trace.md](../design/execution-trace.md) 对应。

### 字段含义

| 字段 | 说明 |
|------|------|
| duration_ms | 该步执行耗时（毫秒） |
| result_type | 世界语义：pure / success / side_effect_committed / retryable_failure / permanent_failure / compensatable_failure |
| reason | 失败或重试原因（从 node_finished 等事件解析） |
| attempts | 该步尝试次数 |

Trace 数据由事件流推导（`ListEvents(job_id)` → `BuildExecutionTree` + `BuildNarrative`），与 [design/event-replay-recovery.md](../design/event-replay-recovery.md)、[design/execution-state-machine.md](../design/execution-state-machine.md) 一致。

### 未启用 Trace 时

若 API 未配置 `JobEventStore`（或 `jobstore.type` 为空），访问 trace 接口会返回 503 "Trace 未启用"。使用 Postgres 事件存储并正确注入 `SetJobEventStore` 后即可使用。

## 参考

- [tracing.md](tracing.md) — HTTP 请求 OTLP 追踪
- [design/execution-trace.md](../design/execution-trace.md) — 执行 trace 事件与树结构
- [usage.md](usage.md) — API 与 Job 流程概览
