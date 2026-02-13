# Custom Node Registration (2.0)

This guide shows how to use built-in wait-like nodes and register custom node adapters.

## Built-in wait-like node types

Aetheris 2.0 provides three built-in wait-like node types in `TaskGraph`:

- `wait` (`planner.NodeWait`)
- `approval` (`planner.NodeApproval`)
- `condition` (`planner.NodeCondition`)

All three are handled by Runner as job-blocking nodes and are resumed by `POST /api/jobs/:id/signal` with matching `correlation_key`.

Default behavior:

- `approval` defaults to `wait_kind=signal`, `reason=approval_required`
- `condition` defaults to `wait_kind=condition`, `reason=wait_condition`
- `wait` keeps existing behavior from `config.wait_kind`

`config.correlation_key` is supported for deterministic keys; otherwise runtime generates `wait-<uuid>`.

## Register a custom node adapter

`Compiler` now supports registration + discovery of node adapters.

```go
compiler := agentexec.NewCompiler(map[string]agentexec.NodeAdapter{
    planner.NodeLLM:      llmAdapter,
    planner.NodeTool:     toolAdapter,
    planner.NodeWorkflow: workflowAdapter,
})

// Register custom node type
compiler.Register("my_custom_node", myAdapter)

// Discover registered node types
nodeTypes := compiler.RegisteredNodeTypes()
fmt.Println(nodeTypes)
```

Your adapter must implement:

- `ToDAGNode(task, agent)`
- `ToNodeRunner(task, agent)`

For API/Worker assembly, see `internal/app/api/agent_dag.go` where built-in adapters are registered.

## Example: approval node in TaskGraph

```go
&planner.TaskGraph{
    Nodes: []planner.TaskNode{
        {
            ID:   "approve_refund",
            Type: planner.NodeApproval,
            Config: map[string]any{
                "correlation_key": "approval-refund-123",
                "park":            true,
            },
        },
    },
}
```

When this node is reached:

1. Runner appends `job_waiting`.
2. Job status becomes `Waiting` (or `Parked` when `park=true`).
3. External signal resumes execution.
