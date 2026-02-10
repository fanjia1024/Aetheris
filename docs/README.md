# Documentation

This directory is the entry point for architecture, usage, and API documentation.

## Quick start

Install **Go 1.25.7+**, clone the repo, then run:

```bash
go run ./cmd/api
```

Health check: `curl http://localhost:8080/api/health`. For full startup, environment variables, and typical flows see [usage.md](usage.md); for upload → retrieve E2E steps see [test-e2e.md](test-e2e.md).

## Project names

- **Aetheris** — Product/project name
- **rag-platform** — go.mod module name
- **CoRag** — Short name used in deployment and CLI (e.g. `corag` command, `CORAG_API_URL`)

All refer to the same project.

## Version and changes

Recommended **Go 1.25.7+**, aligned with go.mod and CI.

- [CHANGELOG.md](../CHANGELOG.md) — Version history and notable changes (v0.8 persistent runtime, event JobStore, Job/Scheduler/Checkpoint/Steppable, v1 Agent API, TaskGraph execution layer, RulePlanner, planner selection, etc.)

## Recommended reading order

- **Getting started**: This README → [usage.md](usage.md) → [design/core.md](../design/core.md), [design/struct.md](../design/struct.md)
- **Advanced**: [design/services.md](../design/services.md), [design/jobstore_postgres.md](../design/jobstore_postgres.md), [design/execution-trace.md](../design/execution-trace.md), [design/poison-job.md](../design/poison-job.md)
- **Operations**: [tracing.md](tracing.md), [config.md](config.md), [deployment.md](deployment.md)

## Design docs

- [design/core.md](../design/core.md) — Overall architecture, layers, Agent Runtime and task execution, Pipeline and eino orchestration
- [design/struct.md](../design/struct.md) — Repo structure and module roles (internal/agent, internal/runtime/jobstore)
- [design/services.md](../design/services.md) — Multi-service architecture (api / agent / index)
- [design/jobstore_postgres.md](../design/jobstore_postgres.md) — JobStore event model and Postgres design

## Usage and API

- [Usage (usage.md)](usage.md) — Startup, environment variables, typical flows, API endpoint summary, FAQ
- [Configuration (config.md)](config.md) — api.yaml, model.yaml, worker.yaml field reference and env vars
- [CLI (cli.md)](cli.md) — corag subcommands, install and run, REST API mapping
- [E2E testing (test-e2e.md)](test-e2e.md) — Upload → parse → split → index → retrieve (PDF / AGENTS.md)
- [Tracing (tracing.md)](tracing.md) — OpenTelemetry config, OTEL_EXPORTER_OTLP_ENDPOINT, local Jaeger

## Examples and deployment

- [Examples guide (examples.md)](examples.md) — basic_agent, simple_chat_agent, streaming, tool, workflow purpose and run instructions
- [examples/](../examples/) — Example code
- [Deployment (deployment.md)](deployment.md) — Compose / Docker / K8s overview and prerequisites
- [deployments/](../deployments/) — Docker, Compose, K8s directories

## Release and acceptance

- [release-acceptance-v0.9.md](release-acceptance-v0.9.md) — v0.9 runtime correctness (Worker crash recovery, API restart, multi-Worker, Replay)
- [release-certification-1.0.md](release-certification-1.0.md) — 1.0 release gate checklist
- [release-checklist-v1.0.md](release-checklist-v1.0.md) — Post-release checklist (core features, distributed, CLI/API, logging and docs)
