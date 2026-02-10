# Usage

Requires **Go 1.25.7+** (aligned with go.mod and security fixes). For a first-time setup and full core-feature walkthrough (快速体验 vs 完整运行时), see [Get Started](get-started.md).

## Starting services

### API

```bash
# Recommended: merged config (api + model)
go run ./cmd/api

# Default listen :8080; override via api.port in configs/api.yaml
```

Startup loads `configs/api.yaml` and `configs/model.yaml`. If `model.defaults.llm` and `model.defaults.embedding` are set, the full query_pipeline and ingest_pipeline (retrieve + generate, parse + split + embed + index) are registered automatically.

### Worker (offline tasks)

```bash
go run ./cmd/worker
```

Uses `configs/worker.yaml`; consumes from the task queue and runs eino ingest_pipeline (queue is currently a placeholder implementation).

### CLI

```bash
go run ./cmd/cli
```

For debugging and admin; subcommands and usage are in [CLI (cli.md)](cli.md).

## Environment variables and configuration

- **API Key**: In `configs/model.yaml` use `api_key: "${OPENAI_API_KEY}"`; it is substituted at runtime.
- **Planner (v1 Agent)**: When `PLANNER_TYPE=rule`, new v1 agents use the **rule planner** (no LLM, fixed TaskGraph) for stable Executor debugging; otherwise the **LLM planner** is used. Startup logs indicate which planner is active.
- **Secrets**: Do not commit real API keys; use environment variables or a secrets manager.
- **Storage**: API defaults to memory storage (data lost on restart); for production configure MySQL/Milvus etc. (implement the corresponding Store).
- **Tracing**: Enable OpenTelemetry under `monitoring.tracing` in `configs/api.yaml`; when `export_endpoint` is unset, the **OTEL_EXPORTER_OTLP_ENDPOINT** env var is used. See [Tracing (tracing.md)](tracing.md).
- For per-file config reference see [Configuration (config.md)](config.md).

## Typical flows

### 1. Upload a document

```bash
curl -X POST http://localhost:8080/api/documents/upload \
  -F "file=@/path/to/your.pdf"
```

On success, ingest_pipeline runs: load → parse → split → embed → write to default vector index and metadata.

### 2. List documents

```bash
curl http://localhost:8080/api/documents/
```

### 3. Use v1 Agent (recommended)

```bash
# Create Agent
curl -X POST http://localhost:8080/api/agents \
  -H "Content-Type: application/json" \
  -d '{"name": "my-agent"}'
# Returns {"id": "agent-xxx", "name": "my-agent"}

# Send message (triggers planning and execution)
curl -X POST http://localhost:8080/api/agents/<agent-id>/message \
  -H "Content-Type: application/json" \
  -d '{"message": "Your question"}'
# Returns 202 Accepted with job_id, e.g. {"status":"accepted","agent_id":"...","job_id":"job-xxx"}

# Poll job status
curl http://localhost:8080/api/agents/<agent-id>/jobs/<job_id>
# Returns job details: id, agent_id, goal, status (pending|running|completed|failed), cursor, retry_count, created_at, updated_at

# List jobs for this Agent (optional query: status, limit)
curl "http://localhost:8080/api/agents/<agent-id>/jobs?limit=20&status=completed"

# Agent state
curl http://localhost:8080/api/agents/<agent-id>/state

# List all agents
curl http://localhost:8080/api/agents
```

**v0.8 execution path**: Message is written to Session → **dual-write** creates Job (if JobEventStore is configured: append JobCreated to event stream, then state JobStore.Create) → Scheduler pulls Pending jobs from state JobStore → Runner.RunForJob (Steppable + node-level Checkpoint) → PlanGoal produces TaskGraph → compile to eino DAG → execute node by node → update Job status on completion/failure. RAG can be used via workflow nodes chosen by the planner.

**Job storage (event stream)**: The event stream interface (ListEvents, Append, Claim, Heartbeat, Watch) supports crash recovery, multiple workers, and audit replay; the API currently uses an in-process memory implementation.

**Scheduler**: Runs in the API process, pulls jobs and executes them. Scheduler params (MaxConcurrency, RetryMax, Backoff) are set in app code (e.g. concurrency 2, retry 2, backoff 1s); see `internal/app/api/app.go`.

**Control / Data plane (Postgres)**: When `jobstore.type=postgres`, the API is control-plane only (create job, query, cancel); it **does not start the Scheduler or run any jobs**. All execution is done by Workers via Postgres Claim. API restart or scale does not affect already-claimed jobs. See [design/services.md](../design/services.md) §7.

### 4. Query (deprecated; prefer Agent message)

```bash
curl -X POST http://localhost:8080/api/query \
  -H "Content-Type: application/json" \
  -d '{"query": "Your question", "top_k": 10}'
```

Uses query_pipeline: embed query → retrieve → LLM generate answer. **Deprecated**; use `POST /api/agents/{id}/message` instead.

### 5. Batch query

```bash
curl -X POST http://localhost:8080/api/query/batch \
  -H "Content-Type: application/json" \
  -d '{"queries": [{"query": "Question 1"}, {"query": "Question 2"}]}'
```

## API endpoint summary

| Method | Path | Description |
|--------|------|-------------|
| GET | /api/health | Health check |
| **v1 Agent** | | |
| POST | /api/agents | Create agent |
| GET | /api/agents | List all agents |
| POST | /api/agents/:id/message | Send message (creates job, 202 + job_id); optional `Idempotency-Key` header |
| GET | /api/agents/:id/state | Agent state (status, current_task, last_checkpoint) |
| GET | /api/agents/:id/jobs | List jobs for this agent (?status=, ?limit=) |
| GET | /api/agents/:id/jobs/:job_id | Single job (poll status) |
| **Execution trace** | | |
| GET | /api/jobs/:id/events | Raw event stream (id, type, payload, created_at) |
| GET | /api/jobs/:id/trace | Timeline and execution_tree, node timings |
| GET | /api/jobs/:id/trace/page | Same as trace, HTML page |
| GET | /api/jobs/:id/replay | Read-only replay |
| POST | /api/agents/:id/resume | Resume execution |
| POST | /api/agents/:id/stop | Stop execution |
| **Documents and knowledge** | | |
| POST | /api/documents/upload | Upload document |
| GET | /api/documents/ | List documents |
| GET | /api/documents/:id | Document details |
| DELETE | /api/documents/:id | Delete document |
| GET | /api/knowledge/collections | List collections |
| POST | /api/knowledge/collections | Create collection |
| DELETE | /api/knowledge/collections/:id | Delete collection |
| **Query (deprecated)** | | |
| POST | /api/query | Single query (prefer Agent message) |
| POST | /api/query/batch | Batch query |
| **Legacy agent** | | |
| POST | /api/agent/run | Run agent by session (query + session_id) |
| **System** | | |
| GET | /api/system/status | System status (workflows, agents) |
| GET | /api/system/metrics | Metrics |

Document, knowledge, agent, and query routes may have auth middleware; see `internal/api/http/router.go`.

## Execution trace (explainable execution)

After sending a message you get a `job_id`. Use these endpoints to see what the job did:

- **GET /api/jobs/:id/events**: Full event stream (`job_created`, `plan_generated`, `node_started`, `node_finished`, `command_emitted`, `command_committed`, `tool_called`, `tool_returned`, `job_completed`, etc.) to reconstruct the User → Plan → nodes → tool calls chain.
- **GET /api/jobs/:id/trace**: Timeline, node timings, and **execution_tree** for an explainable view.
- **GET /api/jobs/:id/trace/page**: Same as trace, as an HTML page.

Event semantics and tree derivation are in [design/execution-trace.md](../design/execution-trace.md).

## FAQ

- **Job and event stream**: The returned `job_id` is written to both the event stream (JobCreated) and the state JobStore for future replay or multi-worker consumption; execution is still driven by the state JobStore + Scheduler.
- **Idempotency-Key**: `POST /api/agents/:id/message` supports header `Idempotency-Key`. Duplicate requests with the same key (e.g. retries) return the existing `job_id` (202) and do not create a new job or rewrite Session/Plan.
- **Poison jobs**: When a job keeps failing, after max_attempts (Scheduler retry_max, Worker max_attempts) it is marked Failed and no longer scheduled; see [design/poison-job.md](../design/poison-job.md).
- **v1 Agent vs /api/query**: v1 Agent uses Agent + Session + plan → TaskGraph → eino DAG as the only path; RAG is an optional tool. `/api/query` still hits query_pipeline directly and is deprecated; use Agent messages for new usage.
- **PLANNER_TYPE=rule**: Disables LLM planning for debugging; the rule planner returns a fixed single-node llm TaskGraph to verify Executor and DAG.
- **No OPENAI_API_KEY**: API still starts but will not register real LLM/Embedding query and ingest workflows; query/upload may use placeholders or fail. With RulePlanner, planning does not need LLM, but executing llm nodes still requires LLM config.
- **Memory storage**: Default metadata and vector are in-memory; data is lost on restart. Configure and implement a persistent store for production.
- **Config not applied**: Ensure the API uses `LoadAPIConfigWithModel` (cmd/api does) and that `configs/model.yaml` has `defaults.llm`, `defaults.embedding` and the matching provider/model keys.
- **Tracing**: Set `monitoring.tracing.enable: true` and `export_endpoint` in `configs/api.yaml` (or `OTEL_EXPORTER_OTLP_ENDPOINT`); otherwise no traces are sent. Use Jaeger or another OTLP backend; see [tracing.md](tracing.md).

Architecture and module roles are in [design/](design/); deployment steps are in each [deployments/](../deployments/) README.
