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
- **Services**: postgres (5432), api (8080), worker1, worker2.
- **Check**: Health check, create agent, send message; after restarting API/Worker, jobs remain in Postgres and continue.
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

---

For config (api.yaml, worker.yaml, model.yaml) and env vars see [config.md](config.md); for API and CLI usage see [usage.md](usage.md) and [cli.md](cli.md).
