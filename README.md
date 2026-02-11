# Aetheris

**Aetheris is an execution runtime for intelligent agents.**

It provides a durable, replayable, and observable environment where AI agents can plan, execute, pause, resume, and recover long-running tasks. Execution is event-sourced and recoverable, so agents can resume after crashes and be replayed for debugging.

Instead of treating LLM calls as stateless requests, Aetheris treats an agent as a **stateful process** — similar to how an operating system manages programs.

---

## What Aetheris Actually Is

**Aetheris is an Agent Hosting Runtime — Temporal for Agents.** You don’t use it to *write* agents; you use it to *run* them. Durable, recoverable, and auditable.

Aetheris is closer to **Temporal / workflow engine / distributed runtime** than to a traditional AI framework.

It is *not*:

* a chatbot framework
* a prompt wrapper
* a RAG library
* a tool for *authoring* agent logic (use LangGraph, AutoGen, CrewAI, etc. for that)

It *is*:

* an **agent execution runtime** — you host long-running, recoverable agent jobs on it
* a long-running task orchestrator
* a recoverable planning & execution engine
* a durable memory of agent actions

Aetheris turns agent behavior into a deterministic execution history.

### Where Aetheris sits in the stack

| Layer | Role | Examples |
|-------|------|----------|
| **Agent authoring** | Define plans, tools, prompts | LangGraph, AutoGen, CrewAI |
| **Agent runtime** | Run agents: durability, lease, replay, signal, forensics | **Aetheris** |
| **Capabilities** | RAG, search, APIs | Vector DBs, RAG pipelines |
| **Compute** | LLM, embedding, inference | OpenAI, local models |

You build agents with your favorite framework; you **host** them on Aetheris for production-grade execution.

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

**Important**: These guarantees require developers to follow the [Step Contract](design/step-contract.md) — steps must be deterministic and side effects must go through Tools. See the contract for how to write correct steps.

---

## When to Use Aetheris

Aetheris is designed for **three core scenarios**:

### 1. Human-in-the-Loop Operations

审批流、客服工单、运营决策 — agent 等待人工（可能 3 天）并从中断点恢复

**Why Aetheris**:
- **StatusParked**: 长时间等待不占 Scheduler 资源
- **Continuation**: 恢复时绑定完整 state（思维连续性）
- **Signal**: 外部触发恢复（at-least-once delivery）

**Examples**: 法务审批合同、财务审批付款、客服升级工单、HR 招聘审批

---

### 2. Long-Running API Orchestration

SaaS 操作代理、数据 pipeline、批量处理 — agent 调用多个外部 API（可能 1 小时）

**Why Aetheris**:
- **At-most-once**: Tool 调用不重复（Ledger + Effect Store）
- **Crash recovery**: Worker 崩溃后从 Checkpoint 继续
- **Step timeout**: 超时自动重试或失败

**Examples**: Salesforce 批量同步、Stripe 订单处理、数据清洗 pipeline、API 编排

---

### 3. Auditable Decision Agents

金融交易、医疗处方、政府系统 — 必须记录"谁、何时、为什么做了什么"

**Why Aetheris**:
- **Evidence Graph**: 记录 RAG doc IDs、tool invocations、LLM model/version
- **Execution Proof Chain**: 不可篡改的决策历史
- **Replay deterministic**: 可证明"决策可重现"

**Examples**: 自动放款审批、处方推荐、补贴发放、合规决策

---

**Don't use Aetheris** for:
- Stateless chatbots (单次请求/响应，无需持久化)
- Prototype/demo agents (崩溃可接受，无审计需求)
- Pure in-memory tasks (<1 min, no side effects)

If your agent is becoming a "critical system" (customers depend on it, data loss is unacceptable, failures cost money), you need Aetheris.

---

## Quick Start

**Build your first production agent in 15 minutes**: [Getting Started with Agents](docs/getting-started-agents.md)

See a real business scenario (refund approval agent with human-in-the-loop) running on Aetheris, including:
- Tool definition (at-most-once side effects)
- Wait node (StatusParked for long waits)
- Signal (human approval)
- Crash recovery (Worker crash → resume without duplicate)
- Trace & Replay (audit & debug)

## Adapters

Already have agents? Migrate them to Aetheris:

- [Custom Agent Adapter](docs/adapters/custom-agent.md) — Wrap your existing agents (imperative → TaskGraph)
- [LangGraph Adapter](docs/adapters/langgraph.md) — Run LangGraph agents on Aetheris (coming soon)

---

## Architecture Overview

**Aetheris treats agents as virtual processes, not tasks.** Workers schedule and host processes; processes can pause, wait for signals, receive messages, and resume across different workers.

User → Agent API → Job → Scheduler → Runner → Planner → TaskGraph → Tool/Workflow Nodes

Key components:

* **Agent API** — creates and interacts with agents
* **JobStore (event sourcing)** — durable execution history
* **Scheduler** — leases and retries tasks
* **Runner** — step-level execution with checkpointing
* **Planner** — produces a TaskGraph
* **Execution Engine (eino)** — executes DAG nodes
* **Workers** — distributed execution

Scheduler correctness (lease fencing, step timeout) is implemented and documented in [design/scheduler-correctness.md](design/scheduler-correctness.md).

RAG is one capability that agents can use via pipelines or tools; it is **pluggable**, not the only built-in scenario. Aetheris is an **Agent Hosting Runtime** (Temporal for agents): retrieval, generation, and knowledge pipelines are integrated as optional components, not the core product.

**Names**: The product name is **Aetheris**. The Go module name (and import path) is **rag-platform**. The CLI command is **aetheris**. See [docs/README.md](docs/README.md) for naming details.

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

**One-command run**: From the repo root, run `make run` to build and then start the API (default :8080) and Worker in the background; PIDs and logs are under `bin/`. Use `make stop` to stop. If using Postgres as jobstore, start Postgres first (see [docs/deployment.md](docs/deployment.md)). For a full walkthrough of core features (快速体验 vs 完整运行时), see [docs/get-started.md](docs/get-started.md).

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

So: **external side effects are executed at most once**. 1.0 proof: the four fatal tests (worker crash before tool, crash after tool before commit, two workers same step, replay restore output) pass — no step repeats external side effects under crash, restart, or duplicate worker. High-level runtime flow and StepOutcome semantics: [design/runtime-core-diagrams.md](design/runtime-core-diagrams.md). See [design/1.0-runtime-semantics.md](design/1.0-runtime-semantics.md) for the three mechanisms and the Execution Proof Chain; [design/execution-proof-sequence.md](design/execution-proof-sequence.md) for the detailed Runner–Ledger–JobStore sequence diagram. For 2.0 feature modules and roadmap: [design/aetheris-2.0-overview.md](design/aetheris-2.0-overview.md).

---

## Auditability & Forensics

Aetheris is built not only to **trace** execution but to **audit** and **attribute** it. You can answer: *"Who had the AI send that email, at which step, and based on which LLM output or tool result?"*

* **Decision timeline** — The event stream is the source of truth; every step has node_started/node_finished, command_emitted/command_committed, and tool_invocation_* events.
* **Reasoning snapshot** — Per-step context (goal, state_before, state_after, and optionally llm_request/llm_response for LLM nodes) is written as `reasoning_snapshot` events for causal debugging.
* **Step causality** — The execution tree (plan → node → tool) and Trace API let you see which step’s input/output led to the next.
* **Tool provenance** — Every tool call’s input and output is recorded; you can trace side effects back to the exact step and command.

See [design/execution-forensics.md](design/execution-forensics.md) and [design/causal-debugging.md](design/causal-debugging.md).

**Runtime guarantees and failure behavior** are documented in [docs/runtime-guarantees.md](docs/runtime-guarantees.md). See what happens when workers crash, steps timeout, or signals are lost. Formal guarantees table: [design/execution-guarantees.md](design/execution-guarantees.md).

---

## License

Aetheris is licensed under the Apache License 2.0. See [LICENSE](LICENSE) for details.

## Contributing

Contributions are welcome! Please see [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines on how to get started.

## Code of Conduct

Please note that this project is governed by a [Code of Conduct](CODE_OF_CONDUCT.md).
By participating, you are expected to uphold this code.
