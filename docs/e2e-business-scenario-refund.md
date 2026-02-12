# End-to-End Business Scenario: Refund Approval Agent

This document describes a **full flow** from agent design to deployment on Aetheris: a refund approval agent with human-in-the-loop. It summarizes the scenario, design, and run steps; for complete code and step-by-step instructions see [Getting Started with Agents](getting-started-agents.md).

---

## 1. Scenario

**Business flow**:

1. User requests a refund (e.g. for order-123).
2. Agent queries order status via a **tool** (read-only).
3. LLM (or rules) decides whether human approval is needed.
4. If yes → agent **waits** for human approval (StatusParked; can wait hours or days).
5. Human approves via **signal** (API or UI).
6. Agent resumes and executes refund via a **tool** (at-most-once side effect).
7. Job completes with full **audit trail** (who, when, why).

**Why Aetheris**:

- **At-most-once**: Refund tool must not run twice; Aetheris Ledger + Effect Store guarantee this even after crash or retry.
- **Human-in-the-loop**: Wait node and Signal let the agent park until approval; no thread blocking.
- **Replay**: Execution is event-sourced; you can replay and debug without re-executing tools.
- **Audit**: Trace and evidence answer “who approved, at which step, and based on which LLM output?”

---

## 2. Agent Design

- **Goal**: e.g. “Refund order-123”.
- **TaskGraph**:  
  - Node 1: **Tool** `query_order` (order_id → status, amount).  
  - Node 2: **LLM** or rule: “need approval?”.  
  - Node 3: **Wait** (correlation_key for approval signal).  
  - Node 4: **Tool** `send_refund` (order_id, amount; idempotency key from runtime).
- **Tools**:  
  - Implement `ToolExec`; use `StepIdempotencyKeyForExternal(ctx, jobID, stepID)` and pass it to the payment API so the refund is at-most-once.  
  - See [design/step-contract.md](../design/step-contract.md): all side effects go through tools.
- **Wait/Signal**:  
  - Wait node emits `job_waiting` with a `correlation_key`.  
  - Call `POST /api/jobs/:id/signal` with the same `correlation_key` and optional payload to unblock.

---

## 3. Config and Deployment

- **Config**: Use `configs/api.yaml` (and `configs/worker.yaml` if separate). For production, set `jobstore.type: postgres` and DSN so that crash recovery and multi-worker work correctly.
- **Run**:  
  - Start Postgres (if using postgres jobstore), then run API and Worker (e.g. `make run` or `go run ./cmd/api` and `go run ./cmd/worker`).  
  - See [get-started.md](get-started.md) and [deployment.md](deployment.md).

---

## 4. Run and Verify

1. **Create agent**: `aetheris agent create refund-agent` (or `POST /api/agents`).
2. **Send message**: `aetheris chat <agent_id>` and enter e.g. “Refund order-123” (or `POST /api/agents/:id/message`).
3. **Wait for parked**: Job status becomes `waiting` when the wait node is reached.
4. **Send signal**: `POST /api/jobs/:id/signal` with body `{"correlation_key": "<approval_key>", "payload": {}}` (use the key from your planner).
5. **Check completion**: Job status becomes `completed`; trace shows plan → query_order → LLM → wait → send_refund.
6. **Trace and replay**: Open Trace UI (`GET /api/jobs/:id/trace/page`) or `aetheris trace <job_id>`; use `aetheris replay <job_id>` to inspect the event stream. Verify that the refund tool ran once and that replay does not re-call it.

---

## 5. References

- [Getting Started with Agents](getting-started-agents.md) — Full code (tools, planner, wait node, signal).
- [Custom Agent Adapter](adapters/custom-agent.md) — Migrate an existing agent to Aetheris (imperative → TaskGraph).
- [Step Contract](../design/step-contract.md) — Rules for deterministic replay and at-most-once tools.
- [Runtime guarantees](runtime-guarantees.md) — Crash recovery, retries, and signal semantics.
