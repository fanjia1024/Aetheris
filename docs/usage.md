# 使用说明

## 启动服务

### API 服务

```bash
# 使用合并配置（api + model），推荐
go run ./cmd/api

# 默认监听 :8080，可通过 configs/api.yaml 的 api.port 修改
```

启动时会加载 `configs/api.yaml` 与 `configs/model.yaml`，若配置了 `model.defaults.llm` 与 `model.defaults.embedding`，将自动注册完整的 query_pipeline 与 ingest_pipeline（检索 + 生成、解析 + 切片 + 向量化 + 索引）。

### Worker（离线任务）

```bash
go run ./cmd/worker
```

使用 `configs/worker.yaml`，负责从任务队列消费并调用 eino 的 ingest_pipeline 等（当前队列为占位实现）。

### CLI

```bash
go run ./cmd/cli
```

用于调试或管理，具体子命令见实现。

## 环境变量与配置

- **API Key**：在 `configs/model.yaml` 中可写为 `api_key: "${OPENAI_API_KEY}"`，运行时从环境变量替换。
- **Planner 类型（v1 Agent）**：`PLANNER_TYPE=rule` 时，新创建的 v1 Agent 使用**规则规划器**（无 LLM，返回固定 TaskGraph），便于稳定调试 Executor；不设置或设为其他值时使用 **LLM 规划器**。启动日志会提示当前使用的规划器。
- **敏感项**：不要将真实 API Key 提交到仓库，使用环境变量或密钥管理。
- **存储**：API 默认使用 memory 存储（重启后数据丢失）；生产可配置 MySQL/Milvus 等（需实现对应 Store）。
- **链路追踪**：在 `configs/api.yaml` 的 `monitoring.tracing` 下可开启 OpenTelemetry；未配置 `export_endpoint` 时使用环境变量 `OTEL_EXPORTER_OTLP_ENDPOINT`。详见 [链路追踪（tracing.md）](tracing.md)。

## 典型流程

### 1. 上传文档

```bash
curl -X POST http://localhost:8080/api/documents/upload \
  -F "file=@/path/to/your.pdf"
```

成功后会执行 ingest_pipeline：加载 → 解析 → 切片 → 向量化 → 写入默认向量索引与元数据。

### 2. 查看文档列表

```bash
curl http://localhost:8080/api/documents/
```

### 3. 使用 v1 Agent（推荐）

```bash
# 创建 Agent
curl -X POST http://localhost:8080/api/agents \
  -H "Content-Type: application/json" \
  -d '{"name": "my-agent"}'
# 返回 {"id": "agent-xxx", "name": "my-agent"}

# 向 Agent 发送消息（会触发规划与执行）
curl -X POST http://localhost:8080/api/agents/<agent-id>/message \
  -H "Content-Type: application/json" \
  -d '{"message": "你的问题"}'
# 返回 202 Accepted，并带 job_id，例如：{"status":"accepted","agent_id":"...","job_id":"job-xxx"}

# 轮询任务状态（根据 job_id 查询单条任务）
curl http://localhost:8080/api/agents/<agent-id>/jobs/<job_id>
# 返回任务详情：id、agent_id、goal、status（pending|running|completed|failed）、cursor、retry_count、created_at、updated_at

# 列出该 Agent 的所有任务（可选查询参数：status、limit）
curl "http://localhost:8080/api/agents/<agent-id>/jobs?limit=20&status=completed"

# 查看 Agent 状态
curl http://localhost:8080/api/agents/<agent-id>/state

# 列出所有 Agent
curl http://localhost:8080/api/agents
```

**v0.8 执行路径**：消息写入 Session → **双写**创建 Job：若配置了 JobEventStore，先向事件流 JobStore `Append(JobCreated)`，再调用状态型 JobStore.Create → Scheduler 从状态型 JobStore 拉取 Pending Job → Runner.RunForJob（Steppable + 节点级 Checkpoint）→ PlanGoal 产出 TaskGraph → 编译为 eino DAG → 逐节点执行 → 完成/失败时更新 Job 状态。RAG 可通过 workflow 节点被规划器选用。

**任务存储（事件流）**：事件流接口（ListEvents、Append、Claim、Heartbeat、Watch）为崩溃恢复、多 Worker 与审计回放预留；当前 API 进程内使用内存实现。

**Scheduler 行为**：调度器在 API 进程内运行，从 Job 队列拉取任务并执行。Scheduler 参数（MaxConcurrency、RetryMax、Backoff）由应用代码默认配置（如并发 2、重试 2 次、Backoff 1s），见 `internal/app/api/app.go`。

### 4. 发起查询（已废弃，建议用 Agent 发消息）

```bash
curl -X POST http://localhost:8080/api/query \
  -H "Content-Type: application/json" \
  -d '{"query": "你的问题", "top_k": 10}'
```

将走 query_pipeline：问题向量化 → 检索 → LLM 生成回答。**已标记 Deprecated**，推荐使用 `POST /api/agents/{id}/message`。

### 5. 批量查询

```bash
curl -X POST http://localhost:8080/api/query/batch \
  -H "Content-Type: application/json" \
  -d '{"queries": [{"query": "问题1"}, {"query": "问题2"}]}'
```

## API 端点汇总

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | /api/health | 健康检查 |
| **v1 Agent** | | |
| POST | /api/agents | 创建 Agent |
| GET | /api/agents | 列出所有 Agent |
| POST | /api/agents/:id/message | 向 Agent 发送消息（创建 Job，返回 202 + job_id） |
| GET | /api/agents/:id/state | Agent 状态（status、current_task、last_checkpoint） |
| GET | /api/agents/:id/jobs | 列出该 Agent 的 Job 列表（可选 ?status=、?limit=） |
| GET | /api/agents/:id/jobs/:job_id | 单条 Job 详情（轮询任务状态） |
| POST | /api/agents/:id/resume | 恢复执行 |
| POST | /api/agents/:id/stop | 停止执行 |
| **文档与知识库** | | |
| POST | /api/documents/upload | 上传文档 |
| GET | /api/documents/ | 文档列表 |
| GET | /api/documents/:id | 文档详情 |
| DELETE | /api/documents/:id | 删除文档 |
| GET | /api/knowledge/collections | 集合列表 |
| POST | /api/knowledge/collections | 创建集合 |
| DELETE | /api/knowledge/collections/:id | 删除集合 |
| **查询（Deprecated）** | | |
| POST | /api/query | 单条查询（推荐用 Agent message） |
| POST | /api/query/batch | 批量查询 |
| **Legacy Agent** | | |
| POST | /api/agent/run | 按 session 执行 Agent（query + session_id） |
| **系统** | | |
| GET | /api/system/status | 系统状态（workflows、agents） |
| GET | /api/system/metrics | 系统指标 |

以上文档类、知识库类、Agent 类、查询类路由可能挂有鉴权中间件，见 `internal/api/http/router.go`。

## 常见问题

- **Job 与事件流**：创建任务时返回的 `job_id` 同时写入事件流（JobCreated）与状态型 Job，便于未来从事件重放恢复或多 Worker 消费；当前执行仍由状态型 JobStore + Scheduler 驱动。
- **v1 Agent 与 /api/query 区别**：v1 Agent 以「Agent + Session + 规划 → TaskGraph → eino DAG」为唯一执行路径，RAG 作为可选工具；`/api/query` 仍直连 query_pipeline，已标记废弃，建议新用法走 Agent 发消息。
- **PLANNER_TYPE=rule**：用于调试时关闭 LLM 规划，规则规划器返回固定单节点 llm TaskGraph，便于验证 Executor 与 DAG 链路。
- **无 OPENAI_API_KEY**：未设置或未在配置中填写时，API 仍可启动，但不会注册带真实 LLM/Embedding 的 query 与 ingest 工作流，查询/上传会走占位或返回错误。使用 RulePlanner 时规划不依赖 LLM，但执行 llm 节点仍需要 LLM 配置。
- **memory 存储**：默认元数据与向量均为内存实现，进程重启后数据清空；需要持久化请配置并实现对应存储类型。
- **配置未生效**：确认 API 使用 `LoadAPIConfigWithModel`（cmd/api 已使用），并检查 `configs/model.yaml` 中 `defaults.llm`、`defaults.embedding` 与对应 provider/model 键是否存在。
- **链路追踪**：需在 `configs/api.yaml` 中设置 `monitoring.tracing.enable: true` 并配置 `export_endpoint`（或设置 `OTEL_EXPORTER_OTLP_ENDPOINT`），否则不会上报 trace；本地可用 Jaeger 等 OTLP 后端查看，见 [tracing.md](tracing.md)。

架构与模块职责以 [design/](design/) 为准；部署步骤以各 [deployments/](../deployments/) 下 README 为准。
