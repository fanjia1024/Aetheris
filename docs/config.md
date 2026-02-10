# Configuration

This document describes the config files under `configs/` for deployment and troubleshooting:

- [configs/api.yaml](../configs/api.yaml) — API service
- [configs/model.yaml](../configs/model.yaml) — Models (LLM / Embedding / Vision)
- [configs/worker.yaml](../configs/worker.yaml) — Worker service

## api.yaml

### api

| Field | Description |
|-------|-------------|
| port | HTTP listen port, default 8080 |
| host | Listen address, default "0.0.0.0" |
| timeout | Request timeout |
| cors.enable / allow_origins | CORS toggle and allowed origins |
| middleware.auth | Enable auth |
| middleware.rate_limit / rate_limit_rps | Rate limit toggle and RPS |
| middleware.jwt_key / jwt_timeout / jwt_max_refresh | JWT (when auth is true); prefer `${JWT_SECRET}` env for jwt_key |
| grpc.enable / port | gRPC toggle and port, default 9090 |

### jobstore

Task event storage (event stream + lease).

| Field | Description |
|-------|-------------|
| type | `memory` or `postgres` |
| dsn | Connection string; use env `JOBSTORE_DSN` to override for Postgres |
| lease_duration | Lease duration; Heartbeat interval should be &lt; lease_duration/2 |

**Important**: When `jobstore.type=postgres`, **only Worker processes execute via event Claim**; the API **does not start** an in-process Scheduler (single execution ownership). With memory, the API starts the Scheduler and runs jobs.

### agent.job_scheduler

Only when `jobstore.type=memory`; with `postgres` the API does not start the Scheduler.

| Field | Description |
|-------|-------------|
| enabled | Enable scheduler |
| max_concurrency | Max concurrent jobs |
| retry_max | Max retries after failure (excluding first attempt) |
| backoff | Wait before retry |
| queues | Optional. Priority-ordered queue list, e.g. `["realtime","default","background"]`. Scheduler claims from the first non-empty queue. Empty or unset → single queue (no class). Job.QueueClass / Job.Priority set at create time (e.g. by API) control which queue a job belongs to; Postgres store requires schema migration for queue columns to filter by queue. |

### agent.adk (Eino ADK 主 Runner)

当 **agent.adk.enabled** 未配置或为 true 时，对话入口 **POST /api/agent/run**、**POST /api/agent/resume**、**POST /api/agent/stream** 使用 Eino ADK Runner 执行（ChatModelAgent + 检索/生成/文档等工具）。设为 **false** 时改用原 Plan→Execute Agent。

| Field | Description |
|-------|-------------|
| enabled | Optional. When `false`, disable ADK and use legacy agent for /api/agent/run. Unset or true → use ADK. |
| checkpoint_store | `memory` (default) for in-process checkpoint; reserved for future postgres/redis. |

**Resume**：请求体 `{"checkpoint_id":"..."}`，用于从 ADK 中断点恢复。**Stream**：与 run 相同请求体，响应为 SSE（`text/event-stream`）。详见 [docs/adk.md](adk.md).

### storage (API)

When present, the API uses it for ingest_pipeline and query_pipeline. Same structure as worker storage: **storage.vector** (type, collection, addr, db) and **storage.ingest** (batch_size, concurrency). See [worker.yaml — storage](#storage) for field descriptions. If api.yaml does not define storage, merged config may fall back to zero values (type `""` → treated as memory; collection `""` → `"default"`).

### service

Service discovery: agent_service, index_service addr and timeout.

### log

level, format, file (optional log file path).

### monitoring

- **prometheus**: enable, port (e.g. 9092).
- **tracing**: OpenTelemetry. When `enable` is true, spans are exported; when `export_endpoint` is empty, env **OTEL_EXPORTER_OTLP_ENDPOINT** is used (endpoint only, e.g. `localhost:4317`). `insecure: true` means no TLS. See [tracing.md](tracing.md).

---

## model.yaml

### Relation to pipelines

When `model.defaults.llm` and `model.defaults.embedding` are set, the API registers **query_pipeline** (retrieve + generate) and **ingest_pipeline** (parse + split + embed + index) at startup. If unset or keys missing, pipelines may not register or use placeholders.

### Structure

- **model.llm.providers**: Each provider (e.g. openai, qwen, claude) has `api_key`, `base_url`, `models`. Each model has name, context_window, temperature, etc.
- **model.embedding.providers**: Same shape; models include dimension, input_limit, etc.
- **model.vision.providers**: Optional; models include max_tokens, temperature, etc.
- **model.defaults**: `llm`, `embedding`, `vision` are default keys in "provider.model" form, e.g. `qwen.qwen3_max`, `openai.text-embedding-ada-002`.

### Secrets

**Do not commit real API keys.** Use environment variable placeholders, e.g.:

```yaml
api_key: "${OPENAI_API_KEY}"
```

Use `DASHSCOPE_API_KEY` for Qwen/DashScope, `ANTHROPIC_API_KEY` for Claude, `COHERE_API_KEY` for Cohere. Viper substitutes these at runtime.

---

## worker.yaml

### worker

| Field | Description |
|-------|-------------|
| concurrency | Concurrency |
| queue_size | Queue size |
| retry_count | Retry count |
| retry_delay | Retry delay |
| timeout | Task timeout |
| poll_interval | Interval for Claiming jobs from the event store |
| capabilities | Optional. List of worker capabilities (e.g. `["llm", "tool", "rag"]`). When set, the Worker only claims jobs whose **required_capabilities** are satisfied by this list (empty job requirements = any worker). Enables multi-agent / multi-model dispatch: e.g. LLM-only workers vs. tool+rag workers. Omit or leave empty to accept any job. |

### jobstore

Must match the API jobstore (type and dsn). When sharing Postgres with the API, Workers run jobs via Claim; the API does not execute.

### storage

- **metadata**: type, dsn, pool_size. Currently only `memory` is fully supported; MySQL etc. require future implementations.
- **vector**: Vector store used by ingest (index) and query (retrieve). Implemented via [internal/einoext](../internal/einoext) factory (memory uses [internal/storage/vector](../internal/storage/vector); redis uses eino-ext components).
  - **type**: `memory` (default) or `redis`. With `memory`, a process-local in-memory store is used. With `redis`, Indexer and Retriever are created from eino-ext Redis components; **Redis Stack** is required (vector search via FT.SEARCH), and the index must be created separately (see eino-ext docs).
  - **addr**: For `redis`, Redis server address (e.g. `localhost:6379`). Ignored for `memory`.
  - **db**: For `redis`, Redis logical DB number as string (e.g. `"0"`). Ignored for `memory`.
  - **collection**: Default index/collection name. Ingest writes to this name; query retrieves from it. Empty means `"default"`. For `redis`, this is used as the index name / key prefix. API and Worker should use the same value when sharing a vector store.
  - **password**: Optional. For `redis`, Redis AUTH password. Omit or leave empty if not used.
- **ingest**: Optional tuning for the ingest pipeline (API and Worker).
  - **batch_size**: Vectors per batch when writing to the vector store (default 100).
  - **concurrency**: Concurrency for embedding and indexing (default 4).

Document metadata written by the indexer includes `vector_store` (the configured type) and `collection` (the index name used).

### splitter

chunk_size, chunk_overlap, max_chunks for ingest splitting.

### Model config

Worker loads config via **LoadWorkerConfigWithModel**, which merges `configs/model.yaml`, so LLM/Embedding/Vision are shared with the API.

### log / monitoring

Same as API for log; monitoring.prometheus port can be set per Worker; use env **CORAG_WORKER_METRICS_PORT** when running multiple workers (e.g. 9094).

---

## Environment variables summary

| Variable | Purpose |
|----------|---------|
| OPENAI_API_KEY | OpenAI API key (model.yaml placeholder) |
| ANTHROPIC_API_KEY | Claude API key |
| DASHSCOPE_API_KEY | Alibaba DashScope / Qwen |
| COHERE_API_KEY | Cohere Embedding |
| JWT_SECRET | API auth JWT secret (when middleware.auth is true) |
| JOBSTORE_DSN | Postgres DSN; overrides jobstore.dsn in api.yaml / worker.yaml |
| OTEL_EXPORTER_OTLP_ENDPOINT | Tracing OTLP endpoint (when export_endpoint is unset) |
| PLANNER_TYPE | Set to `rule` for v1 Agent rule planner (no LLM), for debugging |
| CORAG_API_URL | CLI API base URL, default http://localhost:8080 |
| CORAG_AGENT_ID | Used by CLI `chat` when agent_id is not passed |
| CORAG_WORKER_METRICS_PORT | Worker Prometheus port (when running multiple instances) |

For more on startup and typical flows see the "Environment variables and configuration" section in [usage.md](usage.md).
