# LangGraph Adapter (Migration Guide)

This document explains how to run a **LangGraph-based agent** on Aetheris and keep Aetheris runtime guarantees (durability, replay determinism, at-most-once side effects).

## What is supported today

You can migrate LangGraph in two practical patterns:

1. **Black-box bridge (recommended first)**  
   Run a LangGraph flow inside one Aetheris node, then let Aetheris handle job lifecycle, wait/signal, replay, and audit.
2. **Node-by-node bridge (advanced)**  
   Map important LangGraph nodes to Aetheris steps for finer-grained trace and checkpointing.

No dedicated `pkg/adapters/langgraph` package is required to start. You can implement the bridge in your app layer and runtime node adapters.

## Runtime integration points (code pointers)

- Job creation and planning:
  - `internal/api/http/handler.go`
  - `internal/app/api/agent_v1.go`
- Node execution:
  - `internal/agent/runtime/executor/node_adapter.go`
  - `internal/agent/runtime/executor/runner.go`
- Side-effect and replay guarantees:
  - `design/effect-system.md`
  - `design/step-contract.md`

## Step-by-step migration

### 1. Keep LangGraph planning, move execution envelope to Aetheris

- Wrap your LangGraph invocation into a node runner.
- Feed job goal/input from Aetheris payload.
- Write LangGraph output back to `payload.Results`.

### 2. Route external side effects through Aetheris Tool path

- Any payment/email/webhook/API mutation must run via Aetheris Tool adapter, not direct ad-hoc calls in the bridge.
- Pass `StepIdempotencyKeyForExternal(...)` to downstream APIs for de-duplication.

### 3. Model human review as Aetheris Wait

- Replace ad-hoc “human approval pause” in LangGraph with Aetheris wait semantics:
  - emit `job_waiting` with `correlation_key`
  - resume via `POST /api/jobs/:id/signal`

### 4. Validate replay behavior

- Run:
  - `GET /api/jobs/:id/events`
  - `GET /api/jobs/:id/replay`
  - `aetheris verify <job_id>`
- Ensure replay injects recorded results and does not re-execute side effects.

## Minimal bridge skeleton

```go
// inside a custom node runner
func runLangGraphBridge(ctx context.Context, p *executor.AgentDAGPayload) (*executor.AgentDAGPayload, error) {
    goal := p.Goal
    // invoke your LangGraph app (in-process or RPC)
    // result := langGraphApp.Invoke(goal)
    result := "langgraph-result"

    if p.Results == nil {
        p.Results = make(map[string]any)
    }
    p.Results["langgraph_result"] = result
    return p, nil
}
```

For side-effect steps, delegate to Tool adapters instead of performing raw external writes in this bridge.

## Error handling and recovery rules

- Treat bridge errors as normal step errors so Retry/Failure policy remains consistent with Aetheris.
- For transient external failures, prefer Tool-layer retries.
- For permanent failures, fail the step and keep full trace/evidence.

## Recommended migration path

1. Start with **black-box bridge** to go live quickly.
2. Split high-risk/high-value LangGraph nodes into dedicated Aetheris steps.
3. Add Wait + Signal for human checkpoints.
4. Add forensics export/verify checks in CI.

## Related docs

- [Custom Agent Adapter](custom-agent.md)
- [Getting Started with Agents](../getting-started-agents.md)
- [E2E Business Scenario: Refund](../e2e-business-scenario-refund.md)
