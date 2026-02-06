# CoRag

Go + eino 驱动的 RAG/Agent 平台：统一流程编排、Agent 调度与模型调用，支持复杂 RAG Pipeline 与 DAG Workflow。

## 架构概览

- **API 层**：HTTP/REST，提供文档上传、查询、知识库管理、系统状态等接口。
- **编排核心（eino）**：Workflow/DAG 执行、Agent 调度、Context 传递；所有 Pipeline 仅由 eino 调度。
- **领域 Pipeline（Go 原生）**：Ingest（loader → parser → splitter → embedding → indexer）、Query（retriever → generator）、以及可扩展的专项 Pipeline。
- **模型抽象**：LLM、Embedding、Vision 多厂商抽象；支持运行时切换。
- **存储**：元数据、向量、对象、缓存抽象；当前默认提供 memory 实现。

详见 [design/](design/) 下的架构与仓库结构说明。

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

## 主要功能

- **文档上传**：`POST /api/documents/upload` 触发 ingest_pipeline（解析 → 切片 → 向量化 → 写入向量与元数据）。
- **查询**：`POST /api/query` 使用 query_pipeline（query 向量化 → 检索 → 生成回答）。
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
- 示例代码：[examples/](examples/)

## License

见项目根目录 LICENSE 文件（如有）。
