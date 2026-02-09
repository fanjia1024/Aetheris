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

---

## Why This Matters

Current AI stacks focus on model intelligence.

Aetheris focuses on **execution reliability**.

LLMs made agents possible.
Reliable runtimes will make agents usable in production.

Aetheris is an attempt to provide the missing layer:

> Kubernetes manages containers.
> Aetheris manages agents.

---

## License

Aetheris is licensed under the Apache License 2.0. See [LICENSE](LICENSE) for details.

## Contributing

Contributions are welcome! Please see [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines on how to get started.

## Code of Conduct

Please note that this project is governed by a [Code of Conduct](CODE_OF_CONDUCT.md).
By participating, you are expected to uphold this code.
