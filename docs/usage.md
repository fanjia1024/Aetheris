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
- **敏感项**：不要将真实 API Key 提交到仓库，使用环境变量或密钥管理。
- **存储**：API 默认使用 memory 存储（重启后数据丢失）；生产可配置 MySQL/Milvus 等（需实现对应 Store）。

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

### 3. 发起查询

```bash
curl -X POST http://localhost:8080/api/query \
  -H "Content-Type: application/json" \
  -d '{"query": "你的问题", "top_k": 10}'
```

将走 query_pipeline：问题向量化 → 检索 → LLM 生成回答。

### 4. 批量查询

```bash
curl -X POST http://localhost:8080/api/query/batch \
  -H "Content-Type: application/json" \
  -d '{"queries": [{"query": "问题1"}, {"query": "问题2"}]}'
```

## API 端点汇总

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | /api/health | 健康检查 |
| POST | /api/documents/upload | 上传文档 |
| GET | /api/documents/ | 文档列表 |
| GET | /api/documents/:id | 文档详情 |
| DELETE | /api/documents/:id | 删除文档 |
| GET | /api/knowledge/collections | 集合列表 |
| POST | /api/knowledge/collections | 创建集合 |
| DELETE | /api/knowledge/collections/:id | 删除集合 |
| POST | /api/query | 单条查询 |
| POST | /api/query/batch | 批量查询 |
| GET | /api/system/status | 系统状态（workflows、agents） |
| GET | /api/system/metrics | 系统指标 |

以上文档类、知识库类、查询类路由可能挂有鉴权中间件，见 `internal/api/http/router.go`。

## 常见问题

- **无 OPENAI_API_KEY**：未设置或未在配置中填写时，API 仍可启动，但不会注册带真实 LLM/Embedding 的 query 与 ingest 工作流，查询/上传会走占位或返回错误。
- **memory 存储**：默认元数据与向量均为内存实现，进程重启后数据清空；需要持久化请配置并实现对应存储类型。
- **配置未生效**：确认 API 使用 `LoadAPIConfigWithModel`（cmd/api 已使用），并检查 `configs/model.yaml` 中 `defaults.llm`、`defaults.embedding` 与对应 provider/model 键是否存在。

架构与模块职责以 [design/](design/) 为准；部署步骤以各 [deployments/](../deployments/) 下 README 为准。
