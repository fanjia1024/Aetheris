# LangGraph Adapter (Migration Guide)

This document explains how to run a **LangGraph-based agent** on Aetheris and keep runtime guarantees (durability, replay determinism, at-most-once side effects).

## What is supported today

You can migrate LangGraph in two practical patterns:

1. **Black-box bridge (recommended first)**  
   Run a LangGraph flow inside one Aetheris node, then let Aetheris handle job lifecycle, wait/signal, replay, and audit.
2. **Node-by-node bridge (advanced)**  
   Map important LangGraph nodes to Aetheris steps for finer-grained trace and checkpointing.

In 2.0, Aetheris provides a built-in `langgraph` node adapter entry in executor (`LangGraphNodeAdapter`) with `invoke/stream/state` client interface.

## Runtime integration points (code pointers)

- Job creation and planning:
  - `internal/api/http/handler.go`
  - `internal/app/api/agent_v1.go`
- Node execution:
  - `internal/agent/runtime/executor/node_adapter.go`
  - `internal/agent/runtime/executor/langgraph_adapter.go`
  - `internal/agent/runtime/executor/runner.go`
- Side-effect and replay guarantees:
  - `design/effect-system.md`
  - `design/step-contract.md`

## Step-by-step migration

### 1. Keep LangGraph logic, move execution envelope to Aetheris

- Use a `TaskNode{Type: "langgraph"}` and register `LangGraphNodeAdapter`.
- Feed job goal/input from Aetheris payload.
- Write LangGraph output back to `payload.Results`.

### 2. Route external side effects through Aetheris Tool path

- Any payment/email/webhook/API mutation must run via Aetheris Tool adapter, not direct ad-hoc calls in the bridge.
- Pass `StepIdempotencyKeyForExternal(...)` to downstream APIs for de-duplication.

### 3. Model human review as Aetheris Wait

- Replace ad-hoc “human approval pause” in LangGraph with Aetheris wait semantics:
  - emit `job_waiting` with `correlation_key`
  - resume via `POST /api/jobs/:id/signal`

### 4. Validate replay and signal behavior

- Run:
  - `GET /api/jobs/:id/events`
  - `GET /api/jobs/:id/replay`
  - `aetheris verify <job_id>`
- Ensure replay injects recorded results and does not re-execute side effects.
- For wait/resume paths, ensure `wait_completed` resumes job without re-invoking already completed langgraph steps.

## Minimal runnable skeleton

```go
type MyLangGraphClient struct{}

func (c *MyLangGraphClient) Invoke(ctx context.Context, input map[string]any) (map[string]any, error) {
    // call langgraph invoke API
    return map[string]any{"result": "ok"}, nil
}

func (c *MyLangGraphClient) Stream(ctx context.Context, input map[string]any, onChunk func(map[string]any) error) error {
    return nil
}

func (c *MyLangGraphClient) State(ctx context.Context, threadID string) (map[string]any, error) {
    return map[string]any{"thread_id": threadID}, nil
}

compiler := executor.NewCompiler(map[string]executor.NodeAdapter{
    planner.NodeLangGraph: &executor.LangGraphNodeAdapter{
        Client: &MyLangGraphClient{},
    },
    planner.NodeTool: &executor.ToolNodeAdapter{ /* ... */ },
})
_ = compiler
```

TaskGraph example:

```go
&planner.TaskGraph{
    Nodes: []planner.TaskNode{
        {ID: "lg_invoke", Type: planner.NodeLangGraph},
        {ID: "wait_approval", Type: planner.NodeWait, Config: map[string]any{
            "wait_kind": "signal",
            "correlation_key": "approval-123",
        }},
    },
    Edges: []planner.TaskEdge{
        {From: "lg_invoke", To: "wait_approval"},
    },
}
```

For side-effect steps, delegate to Tool adapters instead of performing raw external writes in this bridge.

## Error handling and recovery rules

- `LangGraphError{Code: retryable}` → mapped to `StepResultRetryableFailure`
- `LangGraphError{Code: permanent}` → mapped to `StepResultPermanentFailure`
- `LangGraphError{Code: wait, correlation_key=...}` → mapped to signal wait (`job_waiting` + `ErrJobWaiting`)

This keeps retry/failure/wait semantics consistent with native nodes.

## Recommended migration path

1. Start with **black-box bridge** to go live quickly.
2. Split high-risk/high-value LangGraph nodes into dedicated Aetheris steps.
3. Add Wait + Signal for human checkpoints.
4. Add forensics export/verify checks in CI.

## Integration test

Reference test:

- `internal/agent/runtime/executor/langgraph_adapter_test.go`

It covers:

- Error mapping (`retryable` / `permanent` / `wait`)
- Replay + signal resume path (after `wait_completed`, langgraph invoke is not re-executed)

## Related docs

- [Custom Agent Adapter](custom-agent.md)
- [Getting Started with Agents](../getting-started-agents.md)
- [E2E Business Scenario: Refund](../e2e-business-scenario-refund.md)
