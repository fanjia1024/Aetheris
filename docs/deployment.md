# Deployment

This document summarizes the three deployment options and prerequisites. See each directory’s README for step-by-step instructions.

## Prerequisites

- **Go**: 1.25.7+ (aligned with [go.mod](../go.mod) and CI).
- **Postgres** (for jobstore): If using `jobstore.type=postgres`, prepare the database and apply the schema. Schema: [internal/runtime/jobstore/schema.sql](../internal/runtime/jobstore/schema.sql); Compose can mount it for init.

## Compose (recommended for single-node)

**Single node: API + 2 Workers + Postgres**, suitable for v1.0 deployability and local integration.

- **Details**: [deployments/compose/README.md](../deployments/compose/README.md)
- **Start** (from repo root):
  ```bash
  docker compose -f deployments/compose/docker-compose.yml up -d --build
  ```
  or:
  ```bash
  ./scripts/local-2.0-stack.sh start
  ```
- **Services**: postgres (5432), api (8080), worker1, worker2.
- **Check**: Health check, create agent, send message; after restarting API/Worker, jobs remain in Postgres and continue.
- **Stop**:
  ```bash
  ./scripts/local-2.0-stack.sh stop
  ```
- **Schema upgrade**: If the DB already exists and is missing `cancel_requested_at`:
  ```sql
  ALTER TABLE jobs ADD COLUMN IF NOT EXISTS cancel_requested_at TIMESTAMPTZ;
  ```

## Docker

Placeholder; single-image Dockerfile and build scripts can be added here later.

- **Details**: [deployments/docker/README.md](../deployments/docker/README.md)

## Kubernetes

Placeholder; Deployment, Service, etc. manifests can be added here later.

- **Details**: [deployments/k8s/README.md](../deployments/k8s/README.md)

## Multi-Environment Deployment

Use the same runtime contract across `dev`, `staging`, and `prod`, with different scale and safety gates.

| Environment | Suggested topology | Main purpose |
|-------------|--------------------|--------------|
| `dev` | Compose (single node) | Feature development, local debugging |
| `staging` | Compose or K8s with Postgres | Integration validation, release rehearsal |
| `prod` | K8s + managed Postgres + monitoring | Production traffic and SLOs |

### Recommended promotion flow

1. `dev`: run `./scripts/release-2.0.sh` and local stack smoke checks.
2. `staging`: deploy candidate image/tag, run end-to-end scenarios (agent run, replay, export/verify).
3. `prod`: rollout with canary/rolling strategy and monitor error rate, stuck jobs, and queue backlog.

### Operational gates before promotion

- CI green (`.github/workflows/ci.yml`)
- Postgres integration tests green
- Runtime forensics checks pass (`export` + `verify`, consistency API)
- Rollback plan verified (previous image/tag ready)

---

For config (api.yaml, worker.yaml, model.yaml) and env vars see [config.md](config.md); for API and CLI usage see [usage.md](usage.md) and [cli.md](cli.md).
