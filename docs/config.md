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

### jobstore

Must match the API jobstore (type and dsn). When sharing Postgres with the API, Workers run jobs via Claim; the API does not execute.

### storage

Metadata, vector, object, cache currently support `memory`; mysql, milvus, s3, redis require future implementations. metadata can have dsn, pool_size.

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
