# CoRag

Go + eino 驱动的 **Agent Runtime** 与 RAG 平台：以 Agent 为第一公民，统一流程编排、规划与执行，支持 TaskGraph → eino DAG、工具调用与 RAG Pipeline 作为能力之一。

## 架构概览

- **API 层**：HTTP/REST，提供 **v1 Agent API**（创建/发消息/状态/恢复/停止）、文档上传、查询、知识库管理、系统状态等接口。
- **Agent 中心**：用户请求经 Agent Manager → Session；发消息时创建 **Job**（双写 **事件流 JobStore** + 状态型 Job）→ **Scheduler**（并发/重试）拉取 Job → **Runner.RunForJob**（Steppable + 节点级 Checkpoint）→ Planner 产出 TaskGraph → 执行适配层逐节点执行；RAG/Pipeline 作为 workflow 或工具节点可被规划器选用。
- **任务存储（JobStore）**：`internal/runtime/jobstore` 提供**事件流**语义：版本化 Append（乐观并发）、Claim/Heartbeat（租约）、Watch（订阅）。当前为内存实现；为崩溃恢复、多 Worker、审计/回放与未来分布式打基础。
- **编排核心（eino）**：仅作为 Agent 的**执行内核**被调用（DAG 调度、Context 传递）；不再直接面对「用户查询」请求。
- **领域 Pipeline**：Ingest、Query 等由 eino 调度，可作为 TaskGraph 中的 workflow 节点被 Agent 调用。
- **模型与存储**：LLM、Embedding、Vision 多厂商抽象；元数据、向量、对象、缓存抽象，当前默认提供 memory 实现。

详见 [design/](design/) 与 [CHANGELOG.md](CHANGELOG.md)。

## 前置依赖

- Go 1.20+
- 可选：`OPENAI_API_KEY`（或其它厂商 API Key）用于 LLM/Embedding；未配置时部分能力为占位实现。

## 快速开始

```bash
# 克隆与依赖
git clone <repo>
cd CoRag
go mod download

# 配置：复制并编辑 configs，设置模型 API Key 等
# 见 configs/api.yaml、configs/model.yaml

# 启动 API 服务（会合并 configs/model.yaml，便于使用 LLM/Embedding）
go run ./cmd/api
# 默认 :8080
```

环境变量示例：`export OPENAI_API_KEY=sk-...`；配置中可使用 `${OPENAI_API_KEY}`。

**最小示例（无需 HTTP/配置文件）**：`go run ./examples/simple_chat_agent` 即可用 CoRag 跑一次对话（依赖 `OPENAI_API_KEY`）。

## 主要功能

- **v1 Agent（推荐）**：`POST /api/agents` 创建 Agent，`POST /api/agents/:id/message` 发送消息并创建 Job（返回 202 + `job_id`），由 Scheduler 拉取并执行（Steppable + 节点级 Checkpoint，支持恢复）；支持状态查询、恢复、停止。规划器可通过环境变量 `PLANNER_TYPE=rule` 切换为无 LLM 的规则规划器便于调试。
- **文档上传**：`POST /api/documents/upload` 触发 ingest_pipeline（解析 → 切片 → 向量化 → 写入向量与元数据）。
- **查询**：`POST /api/query` 使用 query_pipeline（已标记 Deprecated，推荐通过 Agent 发消息交互）。
- **知识库**：集合的列表/创建/删除（见 `/api/knowledge/collections`）。
- **系统**：`/api/health`、`/api/system/status`、`/api/system/metrics`。

## 配置说明

| 文件 | 说明 |
|------|------|
| configs/api.yaml | API 端口、CORS、中间件、日志、监控 |
| configs/model.yaml | LLM/Embedding/Vision 的 providers 与 defaults（如 `defaults.llm: openai.gpt_35_turbo`） |
| configs/worker.yaml | Worker 并发、存储、切片等（离线任务） |

API 启动时通过 `LoadAPIConfigWithModel` 合并 api + model 配置，因此无需单独指定即可使用 model 段。

## 部署

- Docker：[deployments/docker/](deployments/docker/)
- Compose：[deployments/compose/](deployments/compose/)
- K8s：[deployments/k8s/](deployments/k8s/)

## 开发与设计

- 目录结构：`cmd/` 入口，`internal/` 核心（app、runtime/eino、pipeline、model、storage），`pkg/` 公共库，`design/` 设计文档。
- 设计文档：[design/core.md](design/core.md)、[design/struct.md](design/struct.md)、[design/services.md](design/services.md)
- 使用说明与 API 汇总：[docs/](docs/)
- 示例代码：[examples/](examples/)（含 `simple_chat_agent`：基于 `pkg/agent` 的可编程 Agent，无需启动服务）

### 可编程 Agent（pkg/agent）

通过 `rag-platform/pkg/agent` 可在代码中直接创建 Agent、注册工具并执行，无需启动 HTTP 或 Worker：

- **注册工具**：`agent.Tool(name, description, runFunc)` 或 `agent.RegisterTool(tools.Tool)`。注册后的工具会进入同一 Registry，**Planner 通过 Schema 可见**，**Runner 按名调用执行**；在服务端 Job 路径下执行时与事件流一致。
- **执行**：`agent.Run(ctx, prompt)` 或 `agent.RunWithSession(ctx, sessionID, prompt)`，返回最终回答、步数、耗时。

## License

见项目根目录 LICENSE 文件（如有）。
