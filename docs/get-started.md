# Get Started — 核心功能完整测试指南

本文档帮助你在本地快速跑通 Aetheris 的主要能力，并按需完成「完整运行时」测试（崩溃恢复、多 Worker、Trace、取消等）。

---

## 1. 前置条件

- **Go**：1.25.7+（与 [go.mod](../go.mod) 一致）
- **模型配置**：[configs/model.yaml](../configs/model.yaml) 中至少配置一种 LLM + Embedding：
  - 当前默认使用 Qwen（需在 `model.yaml` 中填写有效 `api_key`，或设置 `DASHSCOPE_API_KEY`）
  - 或设置 `OPENAI_API_KEY`，并将 `defaults.llm` / `defaults.embedding` 改为 `openai.gpt_35_turbo`、`openai.text_embedding_ada_002` 等
- **可选：Postgres**：仅当要跑「完整运行时测试」（崩溃恢复、API 重启后 Job 不丢、多 Worker）时需要。默认 [configs/api.yaml](../configs/api.yaml) 中 `jobstore.type` 为 `postgres`；若使用内存模式则无需 Postgres（见下节）。

---

## 2. 选择运行模式

| 模式           | 用途                                          | jobstore   | 需要 Postgres | 需要 Worker 进程              |
| -------------- | --------------------------------------------- | ---------- | ------------- | ----------------------------- |
| **快速体验**   | 本地体验 API、文档、Agent 发消息、轮询、Trace | `memory`   | 否            | 否（API 内建 Scheduler 执行） |
| **完整运行时** | 崩溃恢复、多 Worker、取消、Replay、发布验收   | `postgres` | 是            | 是（至少 1 个，推荐 2 个）    |

### 快速体验

- 在 [configs/api.yaml](../configs/api.yaml) 中临时将 `jobstore.type` 改为 `memory`。
- 只启动 API 即可（无需单独启动 Worker）；内存模式下 API 内会启 Scheduler，Job 由 API 进程执行。

### 完整运行时

- 保持 `jobstore.type: postgres`。
- 先启动 Postgres 并执行 schema。一键启动 Postgres（Docker）：

```bash
docker run -d --name aetheris-pg -p 5432:5432 \
  -e POSTGRES_USER=aetheris -e POSTGRES_PASSWORD=aetheris -e POSTGRES_DB=aetheris \
  -v $(pwd)/internal/runtime/jobstore/schema.sql:/docker-entrypoint-initdb.d/01-schema.sql:ro \
  postgres:15-alpine
```

或使用项目 Compose：`docker compose -f deployments/compose/docker-compose.yml up -d postgres`。  
Schema 文件：[internal/runtime/jobstore/schema.sql](../internal/runtime/jobstore/schema.sql)。

- 再启动 1 个 API + 2 个 Worker（见 [docs/release-certification-1.0.md](release-certification-1.0.md) 环境说明）。

---

## 3. 启动服务

### 快速体验（仅 API）

```bash
# 确保 configs/api.yaml 中 jobstore.type 为 memory
go run ./cmd/api
```

或使用 Makefile（会同时启动 API + 单 Worker；内存模式下可只起 API 后手动停掉 Worker）：

```bash
make run
# 健康检查: curl http://localhost:8080/api/health
```

### 完整运行时（API + 2 Workers）

```bash
# 终端 1
go run ./cmd/api

# 终端 2
go run ./cmd/worker

# 终端 3（可选，推荐用于并发与崩溃恢复测试）
go run ./cmd/worker
```

若 `jobstore.type=postgres` 且未起 Postgres，API 会连库失败，需先起 Postgres 或改用 `memory`。

---

## 4. 第一步：健康检查

```bash
curl -s http://localhost:8080/api/health
```

预期：HTTP 200，响应表示服务正常。

---

## 5. 文档与知识库（RAG）

### 上传文档

```bash
curl -X POST http://localhost:8080/api/documents/upload \
  -F "file=@./AGENTS.md"
```

预期：200，响应中含 `doc_id` 或文档 id；ingest 流程（解析 → 分片 → 向量化 → 写入）在默认内存存储下可完成。

### 列出文档

```bash
curl -s http://localhost:8080/api/documents/
```

预期：列表中包含刚上传的文档。

### 文档详情与删除（可选）

```bash
# 详情（将 :id 替换为实际 doc id）
curl -s http://localhost:8080/api/documents/:id

# 删除
curl -X DELETE http://localhost:8080/api/documents/:id
```

### 知识集合

```bash
# 列表
curl -s http://localhost:8080/api/knowledge/collections

# 创建（body 按 API 要求，如 {"name":"my-collection"}）
curl -X POST http://localhost:8080/api/knowledge/collections \
  -H "Content-Type: application/json" \
  -d '{"name":"my-collection"}'

# 删除（将 :id 替换为集合 id）
curl -X DELETE http://localhost:8080/api/knowledge/collections/:id
```

---

## 6. v1 Agent 端到端（核心）

### 创建 Agent

```bash
curl -s -X POST http://localhost:8080/api/agents \
  -H "Content-Type: application/json" \
  -d '{"name":"my-agent"}'
```

记录返回的 `id` 为 `agent_id`。

### 发送消息（创建 Job）

```bash
curl -s -X POST http://localhost:8080/api/agents/<agent_id>/message \
  -H "Content-Type: application/json" \
  -d '{"message":"你的问题，例如：1+1 等于几？"}'
```

预期：202 Accepted，响应中含 `job_id`。

### 轮询 Job 状态

```bash
# 按 Agent 维度查询（推荐）
curl -s http://localhost:8080/api/agents/<agent_id>/jobs/<job_id>

# 或按 Job id 直接查询
curl -s http://localhost:8080/api/jobs/<job_id>
```

重复请求直到 `status` 为 `completed` 或 `failed`。正常流程为 `pending` → `running` → `completed`。

### 列出该 Agent 的 Jobs

```bash
curl -s "http://localhost:8080/api/agents/<agent_id>/jobs?limit=20&status=completed"
```

### Agent 状态

```bash
curl -s http://localhost:8080/api/agents/<agent_id>/state
```

---

## 7. 执行可观测性（Trace / Replay）

将 `<job_id>` 替换为实际 Job id。

### 事件流

```bash
curl -s http://localhost:8080/api/jobs/<job_id>/events
```

预期：包含 `job_created`、`plan_generated`、`node_started`、`node_finished`、`tool_called`/`tool_returned`、`job_completed` 等事件。

### Trace（结构化）

```bash
curl -s http://localhost:8080/api/jobs/<job_id>/trace
```

预期：含 timeline、节点列表、耗时等。

### Trace 页面（浏览器）

在浏览器打开：

```
http://localhost:8080/api/jobs/<job_id>/trace/page
```

可读的 HTML 时间线。

### 只读 Replay

```bash
curl -s http://localhost:8080/api/jobs/<job_id>/replay
```

预期：仅基于事件回放/展示，不触发重新执行、不调用 LLM/工具。

---

## 8. RAG 检索智能体场景

在「文档与知识库」上传文档后，通过 v1 Agent 发送与文档相关的问题，由 Planner 生成含 `knowledge.search`（知识库检索）的 TaskGraph，再经 LLM 节点汇总回答，即 RAG 检索智能体流程。

**建议步骤**：

1. 先完成 [5. 文档与知识库](#5-文档与知识库rag) 中的上传与列表。
2. 创建 Agent（同 [6. v1 Agent 端到端](#6-v1-agent-端到端核心)）。
3. 发送与文档内容相关的问题，例如：「总结这份文档的要点」「文档里对 Agent 的规范有哪些」。
4. 轮询 Job 至 `completed` 后，打开 `GET /api/jobs/<job_id>/trace` 或 `/trace/page`，确认执行图中出现 **knowledge.search** 节点（或 events 中含 `tool_called` / `tool_name` 为 knowledge.search）；回答内容应与已上传文档相关。

与直接调用「已弃用」的 `POST /api/query` 相比：RAG 智能体走 Agent 规划与 DAG 执行，可多步（先检索再总结）、可观测（Trace 中可见检索与生成节点），适合复杂问答与验收测试。一键脚本见 [scripts/test-e2e-rag-agent.sh](../scripts/test-e2e-rag-agent.sh)，文档见 [test-e2e.md](test-e2e.md) 第 7 节。

**多步/多工具场景**：若问题需要「先检索再总结」，Planner 会生成多节点 TaskGraph（如 n1: knowledge.search → n2: llm）。在 Trace 页面或 `GET /api/jobs/:id/trace` 的 `execution_tree` / `nodes` 中可验证节点顺序与类型（tool vs llm）。

---

## 9. 控制：取消 Job、Resume

### 取消运行中的 Job

```bash
curl -s -X POST http://localhost:8080/api/jobs/<job_id>/stop
```

预期：该 Job 状态变为 `cancelled`，执行停止。

### Resume（若支持）

```bash
curl -s -X POST http://localhost:8080/api/agents/<agent_id>/resume \
  -H "Content-Type: application/json" \
  -d '{}'
```

按当前 API 行为验证即可。

---

## 10. 可选：单次 / 批量 Query（已弃用）

推荐使用 Agent 发消息代替；以下仅用于快速验证 RAG 管线。

### 单次查询

```bash
curl -X POST http://localhost:8080/api/query \
  -H "Content-Type: application/json" \
  -d '{"query":"你的问题", "top_k": 10}'
```

### 批量查询

```bash
curl -X POST http://localhost:8080/api/query/batch \
  -H "Content-Type: application/json" \
  -d '{"queries":[{"query":"问题1"},{"query":"问题2"}]}'
```

---

## 11. 一键脚本

### 快速 E2E（无需 Postgres）

[scripts/test-e2e.sh](../scripts/test-e2e.sh)：健康检查 → 上传文件 → 列文档 → 单次 `/api/query`。适用于「快速体验」模式，且 API 已启动。

```bash
./scripts/test-e2e.sh ./AGENTS.md "Summarize the main content."
# 或指定 PDF
./scripts/test-e2e.sh /path/to/your.pdf "Your question"
```

### 完整运行时验收（需 Postgres + 2 Workers）

[scripts/release-cert-1.0.sh](../scripts/release-cert-1.0.sh)：自动化健康、创建 Agent、发消息、轮询、取消、Trace、Replay 等。Tests 2、3、8（Worker/API 崩溃、全量恢复）需手动 kill/重启，步骤见 [release-certification-1.0.md](release-certification-1.0.md)。

```bash
# 先启动 API + 2 个 Worker，再执行
./scripts/release-cert-1.0.sh
```

环境变量：`AETHERIS_API_URL`（默认 `http://localhost:8080`）、`RUN_TEST4=1` 可跑多 Job 并发测试。

---

## 12. CLI 速查

CLI 用于调试与管理：创建 Agent、发消息、查 Job、Trace、Replay、取消。详见 [CLI (cli.md)](cli.md)。

| CLI 命令                               | 说明                        | 对应 REST API                          |
| -------------------------------------- | --------------------------- | -------------------------------------- |
| `go run ./cmd/cli health`              | 健康检查                    | GET /api/health                        |
| `go run ./cmd/cli agent create [name]` | 创建 Agent                  | POST /api/agents                       |
| `go run ./cmd/cli chat <agent_id>`     | 交互式发消息、轮询          | POST .../message；GET .../jobs/:job_id |
| `go run ./cmd/cli jobs <agent_id>`     | 列出该 Agent 的 Jobs        | GET /api/agents/:id/jobs               |
| `go run ./cmd/cli trace <job_id>`      | 输出 Trace JSON 与页面 URL  | GET /api/jobs/:id/trace                |
| `go run ./cmd/cli replay <job_id>`     | 输出事件流与 Trace 页面 URL | GET /api/jobs/:id/events               |
| `go run ./cmd/cli cancel <job_id>`     | 取消运行中的 Job            | POST /api/jobs/:id/stop                |

可通过环境变量 `AETHERIS_API_URL` 指定 API 地址（默认 `http://localhost:8080`）。

---

## 13. 故障排查

- **Job 一直 pending**：若 `jobstore.type=postgres`，必须至少启动一个 Worker，否则没有进程从 Postgres Claim 执行；检查 Worker 是否在运行、DSN 是否正确。
- **API 启动报错连不上 Postgres**：先起 Postgres 并执行 schema，或将 [configs/api.yaml](../configs/api.yaml) 中 `jobstore.type` 改为 `memory` 做快速体验。
- **无 OPENAI_API_KEY**：可改用 Qwen（在 [configs/model.yaml](../configs/model.yaml) 中配置 `defaults.llm: "qwen.qwen3_max"` 等并填写对应 api_key）。
- **上传/查询失败**：确认 `model.defaults.llm` 与 `model.defaults.embedding` 已在 model.yaml 中配置，且 API 使用 `LoadAPIConfigWithModel` 加载（cmd/api 已使用）。

更多配置与接口说明见 [usage.md](usage.md)、[config.md](config.md)；部署见 [deployment.md](deployment.md)。

---

## 附录：核心功能与测试方式速查表

| 核心功能                | 验证方式                               | 快速体验 | 完整运行时 |
| ----------------------- | -------------------------------------- | -------- | ---------- |
| 健康检查                | GET /api/health                        | 是       | 是         |
| 文档上传/列表           | upload + GET /documents/               | 是       | 是         |
| 知识集合 CRUD           | GET/POST/DELETE /knowledge/collections | 是       | 是         |
| 创建 Agent              | POST /api/agents                       | 是       | 是         |
| 发消息得 Job            | POST .../message → 202 + job_id        | 是       | 是         |
| Job 状态/列表           | GET .../jobs/:id, GET .../jobs         | 是       | 是         |
| 事件流 / Trace / Replay | GET .../events, /trace, /replay        | 是       | 是         |
| 取消 Job                | POST /api/jobs/:id/stop                | 是       | 是         |
| Worker 崩溃恢复         | kill Worker，同一 Job 仍完成           | 否       | 是         |
| API 崩溃后 Job 不丢     | 停 API 再启，Job 继续                  | 否       | 是         |
| 多 Worker 并发一致性    | 多 Job 各执行一次                      | 否       | 是         |
| 全量恢复（全进程重启）  | 全 kill 再启，Job 继续完成             | 否       | 是         |
