# Worker 能力调度（Worker Capabilities）

## 目标

支持 Worker 注册能力、Scheduler 按能力派发 Job，用于多 Agent / 多模型场景：例如部分 Worker 仅提供 LLM，部分提供 LLM+Tool+RAG，Job 可指定所需能力，仅被满足条件的 Worker 认领。

## 模型

### Job.RequiredCapabilities

- 可选字段 `required_capabilities []string`（如 `["llm", "tool"]`）。
- **空或未设置**：表示任意 Worker 均可执行。
- **非空**：仅当 Worker 的 capabilities 列表**包含** Job 所需的每一项时，该 Worker 才能认领该 Job（子集匹配：Job 所需 ⊆ Worker 能力）。

### Worker 能力

- 在 **worker 配置**中配置 `worker.capabilities`（如 `["llm", "tool", "rag"]`）。
- Worker 进程启动时读取该配置，认领时仅选择 `RequiredCapabilities` 被自己能力覆盖的 Job。

## 认领语义

### 进程内 Scheduler（API，jobstore=memory）

- `SchedulerConfig` 支持可选 `Capabilities []string`。
- 若配置了 `Capabilities`，调度器调用 `ClaimNextPendingForWorker(ctx, queueClass, capabilities)`，否则沿用 `ClaimNextPendingFromQueue` / `ClaimNextPending`。

### 独立 Worker 进程（jobstore=postgres）

- Worker 使用 **AgentJobRunner**，配置 `worker.capabilities`。
- **当 capabilities 非空**：
  1. 先通过 **metadata JobStore** 的 `ClaimNextPendingForWorker(ctx, "", capabilities)` 认领一条能力匹配的 Job。
  2. 再在 **事件存储** 上对该 jobID 调用 `ClaimJob(ctx, workerID, jobID)` 占租约。
  3. 若 `ClaimJob` 失败（如已被其他 Worker 占或已终止），则对 metadata 侧 `Requeue` 该 Job，继续轮询。
- **当 capabilities 为空**：保持原有行为，仅通过事件存储 `Claim(ctx, workerID)` 认领任意一条可执行 Job。

## 存储

### 元数据 JobStore（internal/agent/job）

- **Mem**：`Job.RequiredCapabilities` 在内存中过滤；`ClaimNextPendingForWorker` 在现有队列/优先级逻辑上增加能力过滤。
- **Postgres**：`jobs` 表增加 `required_capabilities TEXT`（逗号分隔，如 `llm,tool`）；`ClaimNextPendingForWorker` 的 SQL 条件为：`required_capabilities` 为空/NULL 或其中每个能力均在传入的 `workerCapabilities` 数组中。

### 事件存储（internal/runtime/jobstore）

- 新增 **ClaimJob(ctx, workerID, jobID)**：对指定 jobID 占租约；若该 job 已终止或已被其他 Worker 占用则返回错误。用于「先由 metadata 按能力选出 Job，再在事件侧占租约」的流程。

## 配置示例

**configs/worker.yaml**：

```yaml
worker:
  concurrency: 4
  poll_interval: "2s"
  # 仅认领需要 llm 与 tool 的 Job（或未指定 required_capabilities 的 Job）
  capabilities: ["llm", "tool"]
```

创建 Job 时（如 API 或 SDK）可设置 `RequiredCapabilities`；不设置则任何 Worker 都可认领。

## 与多 Agent / 多模型的关系

- 不同 Worker 可配置不同 `capabilities`（如专用 LLM 池、Tool+RAG 池），实现按能力分流。
- Job 创建时可按来源或参数设置 `required_capabilities`，便于与路由策略、多模型选型结合。

## 参考

- 元数据 Job 与认领：`internal/agent/job/job.go`、`job_store.go`、`pg_store.go`
- 事件存储认领：`internal/runtime/jobstore/store.go`、`pgstore.go`、`memory_store.go`
- Worker 启动与 AgentJobRunner：`internal/app/worker/app.go`、`agent_job.go`
- 配置：`pkg/config/config.go`（WorkerConfig.Capabilities）、`docs/config.md`
