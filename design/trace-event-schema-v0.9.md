# Trace Event Schema v0.9 — Semantic Events for Execution Narrative

This document defines the **semantic event types** and **payload conventions** (field-level) for Aetheris v0.9. The goal is to support an **execution narrative** in the Trace UI (timeline, step view, reasoning, tool invocation, state diff), not just a raw event list. Trace UI is derived from this schema; new events are emitted first, then the narrative pipeline and UI consume them.

**Relationship:** Extends [execution-trace.md](execution-trace.md). Existing execution events (job_created, plan_generated, node_started, node_finished, command_emitted, command_committed, tool_called, tool_returned, job_completed, job_failed, job_cancelled) remain unchanged and continue to drive Replay. New semantic events are **additive** and used by the Trace narrative pipeline and UI only.

---

## 1. Design decisions

- **Step identity:** We keep **node** as the execution unit (node_id = step identifier). We do **not** introduce a separate `step_started` / `step_finished` event type; instead we **extend** `node_started` and `node_finished` payloads with scheduling/cognitive fields (attempt, worker_id, duration_ms, state). The horizontal timeline is derived from node_started/node_finished + optional recovery events.
- **State checkpoint:** We add a single event type `state_checkpointed` emitted **after** a node finishes, with `node_id`, `state_before` (payload.Results before the step), `state_after` (payload.Results after = same as node_finished.payload_results). One event per step keeps the stream simple; Runner writes it immediately after NodeFinished.
- **Cognitive:** New event types `agent_thought_recorded`, `decision_made`, `tool_selected` with explicit payloads. Emitted by adapters (LLM/Tool) when content is available; otherwise UI shows placeholder.
- **Tool:** We add `tool_result_summarized` (after tool_returned) with summary, error, and optional idempotency/rollback. Tool adapter emits it when possible.
- **Recovery:** We add `recovery_started`, `recovery_completed`, `step_compensated` for timeline visibility. Runner or Worker writes them when recovery/compensation runs.

---

## 2. New event types (jobstore)

Add to [internal/runtime/jobstore/event.go](internal/runtime/jobstore/event.go):

| EventType              | Meaning                          | Written by        |
|------------------------|----------------------------------|-------------------|
| StateCheckpointed      | state_before/state_after for step | Runner / NodeSink |
| AgentThoughtRecorded   | Agent reasoning text             | LLM Adapter       |
| DecisionMade           | Agent decision (e.g. tool choice) | LLM Adapter       |
| ToolSelected           | Tool chosen before call          | LLM Adapter       |
| ToolResultSummarized   | Tool result summary + error      | Tool Adapter      |
| RecoveryStarted        | Recovery from failure started    | Runner / Worker   |
| RecoveryCompleted      | Recovery finished                | Runner / Worker   |
| StepCompensated        | Step compensation executed       | Runner / Worker   |

**Backward compatibility:** JobStore stores events by Type string + Payload. New types are just new Type values; ListEvents returns them in order. Old jobs have no such events; narrative pipeline and UI treat missing events as “degraded” (no reasoning, no state diff, timeline from node_* only).

---

## 3. Extended payloads (existing events)

### 3.1 node_started (extended)

**Written by:** NodeEventSink (Runner calls AppendNodeStarted).

| Field          | Type   | Required | Description |
|----------------|--------|----------|-------------|
| node_id        | string | yes      | DAG node ID (step id). |
| trace_span_id  | string | no       | Same as node_id for tree. |
| parent_span_id | string | no       | "plan". |
| step_index     | int    | no       | 1-based event order. |
| **attempt**    | int    | no       | 1-based attempt for this node (retry). Default 1. |
| **worker_id**  | string | no       | Worker that is executing (if available). |

### 3.2 node_finished (extended)

**Written by:** NodeEventSink (Runner calls AppendNodeFinished).

| Field             | Type   | Required | Description |
|-------------------|--------|----------|-------------|
| node_id           | string | yes      | DAG node ID. |
| payload_results   | object | no       | Cumulative payload.Results JSON. |
| trace_span_id     | string | no       | Same as node_id. |
| parent_span_id    | string | no       | "plan". |
| step_index        | int    | no       | 1-based. |
| **duration_ms**   | int64  | no       | Elapsed ms since node_started. |
| **state**         | string | no       | "ok" \| "failed" \| "retryable". Omit = ok. |
| **attempt**       | int    | no       | Same as node_started for this run. |

---

## 4. New event payloads (field-level)

### 4.1 state_checkpointed

**Written by:** Runner (via NodeEventSink.AppendStateCheckpointed) immediately after NodeFinished for the same node.

| Field       | Type   | Required | Description |
|-------------|--------|----------|-------------|
| node_id     | string | yes      | Step (node) this checkpoint belongs to. |
| step_index  | int    | no       | 1-based. |
| state_before| object | no       | payload.Results JSON **before** this step. |
| state_after | object | yes      | payload.Results JSON **after** this step (= node_finished.payload_results). |

**Replay:** Does **not** participate in Replay. Replay continues to use plan_generated, node_finished, command_committed only. state_checkpointed is for Trace UI state diff only.

---

### 4.2 agent_thought_recorded

**Written by:** LLM Adapter when reasoning content is available (e.g. from LLM response).

| Field       | Type   | Required | Description |
|-------------|--------|----------|-------------|
| node_id     | string | no       | Node this thought belongs to. |
| step_index  | int    | no       | 1-based. |
| content     | string | yes      | Reasoning text. |
| role        | string | no       | "reasoning" (default) or custom. |

---

### 4.3 decision_made

**Written by:** LLM Adapter when a decision (e.g. “call tool X”) is made.

| Field       | Type   | Required | Description |
|-------------|--------|----------|-------------|
| node_id     | string | no       | Node. |
| step_index  | int    | no       | 1-based. |
| content     | string | yes      | Decision description. |
| kind        | string | no       | e.g. "tool_call", "answer", "retry". |

---

### 4.4 tool_selected

**Written by:** LLM Adapter when the agent selects a tool (before ToolCalled).

| Field      | Type   | Required | Description |
|------------|--------|----------|-------------|
| node_id    | string | no       | Node. |
| tool_name  | string | yes      | Tool chosen. |
| step_index | int    | no       | 1-based. |
| reason     | string | no       | Short reason. |

---

### 4.5 tool_result_summarized

**Written by:** Tool Adapter after tool execution (after ToolReturned or instead of it when summarizing).

| Field              | Type   | Required | Description |
|--------------------|--------|----------|-------------|
| node_id            | string | yes      | Node that invoked the tool. |
| tool_name          | string | yes      | Tool name. |
| step_index         | int    | no       | 1-based. |
| summary            | string | no       | Human-readable result summary. |
| error              | string | no       | Error message if failed. |
| idempotent         | bool   | no       | Whether the call was idempotent. |
| rollback_available | bool   | no       | Whether rollback is available (future). |

**Replay:** Does **not** participate in Replay. tool_returned (and command_committed) remain the source of truth for recovery.

---

### 4.6 recovery_started

**Written by:** Runner or Worker when starting recovery (e.g. after failure, before retry).

| Field       | Type   | Required | Description |
|-------------|--------|----------|-------------|
| node_id     | string | no       | Node being recovered. |
| step_index  | int    | no       | 1-based. |
| reason      | string | no       | e.g. "failure", "timeout". |

---

### 4.7 recovery_completed

**Written by:** Runner or Worker when recovery finished (success or gave up).

| Field    | Type   | Required | Description |
|----------|--------|----------|-------------|
| node_id  | string | no       | Node. |
| success  | bool   | no       | Whether recovery succeeded. |

---

### 4.8 step_compensated

**Written by:** Runner or Worker when compensation for a step is executed.

| Field   | Type   | Required | Description |
|---------|--------|----------|-------------|
| node_id | string | no       | Step that was compensated. |
| summary | string | no       | What was done. |

---

## 5. Who writes what (summary)

| Event / extension          | Writer           |
|----------------------------|------------------|
| node_started (attempt, worker_id) | NodeEventSink (Runner passes attempt, worker_id when available) |
| node_finished (duration_ms, state, attempt) | NodeEventSink (Runner passes these) |
| state_checkpointed        | NodeEventSink (Runner after NodeFinished) |
| agent_thought_recorded     | LLM Adapter      |
| decision_made              | LLM Adapter      |
| tool_selected              | LLM Adapter      |
| tool_result_summarized     | Tool Adapter     |
| recovery_started           | Runner / Worker  |
| recovery_completed         | Runner / Worker  |
| step_compensated           | Runner / Worker  |

---

## 6. Replay impact

- **ReplayContext** (BuildFromEvents) continues to use only:
  - `plan_generated` → TaskGraphState
  - `node_finished` → CompletedNodeIDs, CursorNode, PayloadResults, PayloadResultsByNode
  - `command_committed` → CompletedCommandIDs, CommandResults
- All new semantic events are **ignored** by Replay. They are for Trace narrative and UI only.
- Backward compatibility: Jobs with only old events still replay and still show a degraded Trace (no state diff, no reasoning, timeline from node_* only).

---

## 7. Trace narrative pipeline (consumption)

The narrative pipeline (Phase 3) will:

- Build **timeline segments** from: plan_generated, node_started, node_finished, recovery_*, step_compensated; use extended node_* fields (attempt, duration_ms, state, worker_id) for the horizontal bar.
- Build **step view** from node_started + node_finished (name = node_id, state, attempts, worker, duration).
- Build **reasoning snapshot** from agent_thought_recorded, decision_made, tool_selected (grouped by node_id / step_index).
- Attach **tool invocation** summary/error from tool_result_summarized to the corresponding tool node.
- Build **state diff** from state_checkpointed (state_before vs state_after, key-level diff).

---

## 8. References

- [execution-trace.md](execution-trace.md) — existing event types and tree
- [event-replay-recovery.md](event-replay-recovery.md) — Replay semantics
- [internal/runtime/jobstore/event.go](internal/runtime/jobstore/event.go) — EventType and JobEvent
- [internal/app/api/node_sink.go](internal/app/api/node_sink.go) — current Append implementations
