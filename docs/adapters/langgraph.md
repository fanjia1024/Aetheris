# LangGraph Adapter (Migration Path)

This document outlines how to run a **LangGraph-based agent** on Aetheris for durability, crash recovery, and audit. A full adapter implementation is planned; below is the migration path and a minimal integration pattern.

---

## Why run LangGraph on Aetheris?

- **Durability**: LangGraph graphs are typically in-memory; Aetheris provides event-sourced execution and checkpointing so long-running or human-in-the-loop flows survive restarts.
- **At-most-once side effects**: Tools called from LangGraph can be wrapped so that Aetheris Ledger guarantees no duplicate execution on replay or retry.
- **Audit and replay**: Every step and tool call is recorded; you can trace and replay jobs for debugging and compliance.

---

## Migration path (high level)

1. **Treat the LangGraph run as one or more “nodes” in an Aetheris TaskGraph**  
   - Option A: A single “LangGraph” node that receives the goal, runs your LangGraph (e.g. in process or via a subprocess), and returns the result. Aetheris handles job lifecycle, wait/signal, and checkpointing; the LangGraph sub-run is the node’s implementation.  
   - Option B: Map LangGraph nodes to Aetheris nodes (e.g. each LangGraph node → one Aetheris step); more work but finer-grained checkpointing and trace.

2. **Side effects**  
   - Any tool or API call that must be at-most-once should be exposed as an Aetheris Tool and called from the Aetheris Runner (or from a LangGraph node that delegates to an Aetheris Tool). Do not rely on LangGraph alone for idempotency under crash/retry.

3. **Human-in-the-loop**  
   - If your LangGraph has a “human review” step, model it as an Aetheris Wait node: park the job with a `correlation_key`, then resume via `POST /api/jobs/:id/signal` when the human approves.

---

## Minimal example (conceptual)

- **Planner**: Returns a TaskGraph with one node, e.g. `langgraph_node`, of type “workflow” or custom.
- **Node runner**: For that node, your code invokes the LangGraph graph (e.g. `graph.invoke({"goal": goal})`). Pass the goal from the Aetheris job; write the result back into the Aetheris payload so the next step (if any) or job completion can use it.
- **Tools**: If the graph uses tools that perform external side effects, register them with Aetheris and call them through the Aetheris Tool layer (or ensure they use the same idempotency keys as Aetheris) so that at-most-once is preserved across retries and replays.

---

## Status and next steps

- **Coming soon**: Official LangGraph adapter package or example that wires a LangGraph graph into an Aetheris TaskGraph and documents the exact contract (inputs/outputs, tool delegation, wait/signal).
- **Today**: Use the [Custom Agent Adapter](custom-agent.md) to wrap your LangGraph agent as a single “black box” node and gain job lifecycle, wait/signal, and audit; then refine toward Option B if you need per-node checkpointing.

For full custom agent migration (including imperative agents), see [Custom Agent Adapter](custom-agent.md). For an end-to-end business scenario (refund approval with wait and signal), see [E2E Business Scenario: Refund](../e2e-business-scenario-refund.md).
