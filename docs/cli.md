# CLI

The CLI is used for debugging and admin: start services, create agents, send messages, inspect jobs and event streams, replay, and cancel. The binary is typically named **corag**.

## Install and run

From the repo root:

```bash
go build -o corag ./cmd/cli
```

Put `corag` in your PATH to run it directly. Or run without building:

```bash
go run ./cmd/cli <command> [args]
```

## API base URL

The CLI uses the **CORAG_API_URL** environment variable for the API base URL; default is `http://localhost:8080`. Set it for remote or custom deployment.

## Subcommands

| Command | Description |
|---------|-------------|
| version | Print version (e.g. rag-platform cli 0.1.0) |
| health | Health check (prints ok) |
| config | Show config summary (e.g. api.port, api.host) |
| server start | Start API (runs go run ./cmd/api) |
| worker start | Start Worker (runs go run ./cmd/worker) |
| agent create [name] | Create agent, print agent_id; default name "default" if omitted |
| chat [agent_id] | Interactive chat: send messages, get job_id, poll status; uses CORAG_AGENT_ID if agent_id not passed |
| jobs \<agent_id\> | List jobs for this agent |
| trace \<job_id\> | Print job execution timeline (trace JSON) and Trace page URL |
| workers | List active workers (Postgres mode) |
| replay \<job_id\> | Print job event stream (for replay) and Trace page URL |
| cancel \<job_id\> | Request cancel of a running job |

## Mapping to REST API

| CLI command | REST API |
|-------------|----------|
| agent create [name] | POST /api/agents (body includes name) |
| chat | POST /api/agents/:id/message; poll GET /api/agents/:id/jobs/:job_id |
| jobs \<agent_id\> | GET /api/agents/:id/jobs |
| trace \<job_id\> | GET /api/jobs/:id/trace |
| replay \<job_id\> | GET /api/jobs/:id/events |
| cancel \<job_id\> | POST /api/jobs/:id/stop |

For more endpoints and flows see [usage.md](usage.md) "API endpoint summary" and "Typical flows".
