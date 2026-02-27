# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

**Aetheris** (also known as CoRag) is an execution runtime for intelligent agents — "Temporal for Agents." It provides a durable, replayable, and observable environment where AI agents can plan, execute, pause, resume, and recover long-running tasks.

- **Go Module:** `rag-platform`
- **CLI:** `aetheris`
- **Default API Port:** 8080

## Common Commands

```bash
# Build all binaries (api, worker, cli) to bin/
make build

# Build and start API + Worker in background
make run

# Run only API or Worker
make run-api
make run-worker

# Stop services
make stop

# Run tests (with race detector)
make test

# Run integration tests (runtime + http)
make test-integration

# Format and lint
make fmt         # gofmt -w
make fmt-check   # check formatting
make vet         # go vet
make tidy        # go mod tidy

# Docker
make docker-build  # Build runtime container
make docker-run    # Start local 2.0 stack (postgres + api + workers)
make docker-stop   # Stop local stack

# Health check
curl http://localhost:8080/api/health
```

## Architecture

Aetheris treats agents as **virtual processes** — workers schedule and host processes; processes can pause, wait for signals, receive messages, and resume across different workers.

### Core Components

| Component | Path | Purpose |
|-----------|------|---------|
| **API Server** | `cmd/api/` | HTTP server (Hertz), creates/interacts with agents |
| **Worker** | `cmd/worker/` | Background execution worker, schedules and executes jobs |
| **CLI** | `cmd/cli/` | Command-line tool (`aetheris init`, `chat`, `jobs`, `trace`, `replay`, etc.) |
| **Agent Runtime** | `internal/agent/runtime/` | Core execution engine |
| **Job Store** | `internal/agent/runtime/job/` | Event-sourced durable execution history (PostgreSQL) |
| **Scheduler** | `internal/agent/runtime/job/scheduler.go` | Leases and retries tasks with lease fencing |
| **Runner** | `internal/agent/runtime/runner/` | Step-level execution with checkpointing |
| **Planner** | `internal/agent/planner/` | Produces TaskGraph from goals |
| **Executor** | `internal/agent/runtime/executor/` | Executes DAG nodes using eino framework |
| **Effects** | `internal/agent/effects/` | At-most-once tool execution guarantee via Ledger |

### Execution Flow

```
User → Agent API → Job → Scheduler → Runner → Planner → TaskGraph → Tool/Workflow Nodes
```

### Key Design Documents

- `design/runtime-core-diagrams.md` — Runtime flow and StepOutcome semantics
- `design/1.0-runtime-semantics.md` — Three mechanisms and Execution Proof Chain
- `design/scheduler-correctness.md` — Lease fencing, step timeout guarantees
- `design/step-contract.md` — Contract for writing correct steps (deterministic, side effects through Tools)
- `design/execution-guarantees.md` — Formal guarantees table

### Storage

- **PostgreSQL** — Job events, job state, checkpoints (primary)
- **Redis** — Optional for RAG/indexer

### Three Core Use Cases

1. **Human-in-the-Loop Operations** — Approval flows, StatusParked for long waits
2. **Long-Running API Orchestration** — At-most-once tool execution, crash recovery
3. **Auditable Decision Agents** — Evidence graph, execution proof chain, replay

### Key Technologies

- **Agent Framework:** cloudwego/eino
- **Web Framework:** cloudwego/hertz
- **Database:** jackc/pgx/v5 (PostgreSQL)
- **Cache:** redis/go-redis/v9
- **Auth:** hertz-contrib/jwt
- **Observability:** OpenTelemetry, Prometheus, slog
