# Aetheris

**Aetheris is an execution runtime for intelligent agents.**

It provides a durable, replayable, and observable environment where AI agents can plan, execute, pause, resume, and recover long-running tasks. Execution is event-sourced and recoverable, so agents can resume after crashes and be replayed for debugging.

Instead of treating LLM calls as stateless requests, Aetheris treats an agent as a **stateful process** — similar to how an operating system manages programs.

---

## What Aetheris Actually Is

Aetheris is closer to **Temporal / workflow engine / distributed runtime** than to a traditional AI framework.

It is *not*:

* a chatbot framework
* a prompt wrapper
* a RAG library

It *is*:

* an agent execution runtime
* a long-running task orchestrator
* a recoverable planning & execution engine
* a durable memory of agent actions

Aetheris turns agent behavior into a deterministic execution history.

---

## The Problem

Modern agent frameworks assume that:

* requests are short
* failures are rare
* memory is ephemeral
* execution is synchronous

Real agent workloads break all of these assumptions.

Agents need to:

* run for minutes or hours
* call tools and external systems
* survive crashes and restarts
* be inspectable and debuggable
* resume from the middle of a task

Without a runtime, an agent is just a fragile script.

---

## The Core Idea

Aetheris introduces a different execution model:

> An agent interaction is a **Job**.
> A Job produces an **event stream**.
> The system can replay the stream to reconstruct execution.

Every action becomes an event:

* planning
* tool calls
* intermediate reasoning steps
* retries
* failures
* recovery

Because execution is event-sourced, Aetheris can:

* resume after crash
* run across multiple workers
* audit every decision
* deterministically replay an agent run

---

## Architecture Overview

User → Agent API → Job → Scheduler → Runner → Planner → TaskGraph → Tool/Workflow Nodes

Key components:

* **Agent API** — creates and interacts with agents
* **JobStore (event sourcing)** — durable execution history
* **Scheduler** — leases and retries tasks
* **Runner** — step-level execution with checkpointing
* **Planner** — produces a TaskGraph
* **Execution Engine (eino)** — executes DAG nodes
* **Workers** — distributed execution

RAG is just one capability that an agent may choose to use.

Detailed documentation (configuration, CLI, deployment) is in [docs/](docs/).

---

## Makefile — Build and run

The project provides a Makefile for one-command build and startup of all services.

| Command | Description |
|---------|-------------|
| `make` / `make help` | Show help |
| `make build` | Build api, worker, and cli into `bin/` |
| `make run` | **Build and start API + Worker in background** (one-command startup) |
| `make stop` | Stop API and Worker started by `make run` |
| `make clean` | Remove `bin/` |
| `make test` | Run tests |
| `make vet` | go vet |
| `make fmt` | gofmt -w |
| `make tidy` | go mod tidy |

**One-command run**: From the repo root, run `make run` to build and then start the API (default :8080) and Worker in the background; PIDs and logs are under `bin/`. Use `make stop` to stop. If using Postgres as jobstore, start Postgres first (see [docs/deployment.md](docs/deployment.md)).

---

## Why This Matters

Current AI stacks focus on model intelligence.

Aetheris focuses on **execution reliability**.

LLMs made agents possible.
Reliable runtimes will make agents usable in production.

Aetheris is an attempt to provide the missing layer:

> Kubernetes manages containers.
> Aetheris manages agents.

That claim holds only when the runtime can **prove** that agent steps do not repeat external side effects. Aetheris 1.0 provides:

* **At-most-once tool execution** — Every tool invocation is a persistent fact (Tool Invocation Ledger). On replay, the runner looks up the ledger and restores results instead of calling the tool again.
* **World-consistent replay** — Replay is not “run the step again”; it is “verify the external world still matches the event stream, then restore memory and skip execution” (Confirmation Replay). If verification fails, the job fails rather than silently re-executing.

So: **external side effects are executed at most once**. 1.0 proof: the four fatal tests (worker crash before tool, crash after tool before commit, two workers same step, replay restore output) pass — no step repeats external side effects under crash, restart, or duplicate worker. See [design/1.0-runtime-semantics.md](design/1.0-runtime-semantics.md) for the three mechanisms and the Execution Proof Chain; [design/execution-proof-sequence.md](design/execution-proof-sequence.md) for the Runner–Ledger–JobStore sequence diagram.

---

## License

Aetheris is licensed under the Apache License 2.0. See [LICENSE](LICENSE) for details.

## Contributing

Contributions are welcome! Please see [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines on how to get started.

## Code of Conduct

Please note that this project is governed by a [Code of Conduct](CODE_OF_CONDUCT.md).
By participating, you are expected to uphold this code.
