# DAG Parallel Execution (2.0)

This document describes the execution model for running multiple steps of a TaskGraph in parallel when the DAG allows it. It complements [step-contract.md](step-contract.md), [scheduler-correctness.md](scheduler-correctness.md), and the existing sequential execution in `internal/agent/runtime/executor/runner.go` and [steppable.go](../internal/agent/runtime/executor/steppable.go).

---

## Goals

- **Within a level**: All steps that have no dependency on each other (same topological “level”) may run in parallel on a single worker.
- **Determinism**: Merge of `payload.Results` and checkpoint/cursor updates must be deterministic so replay and recovery produce the same state.
- **Single-worker ownership**: One job is still owned by one worker; parallel steps run on that worker. Lease and fencing are unchanged (see [scheduler-correctness.md](scheduler-correctness.md)).

---

## Execution model

### Topological levels

- The TaskGraph is compiled to a topological order via `TopoOrder` in [steppable.go](../internal/agent/runtime/executor/steppable.go). Nodes with in-degree 0 are level 0; after removing them, the next set with in-degree 0 is level 1; and so on.
- **Level** = set of node IDs that can run concurrently because they share no dependency among themselves (all their predecessors are in earlier levels).
- Helper: given `order []string`, `edges []TaskEdge`, and `completedSet map[string]struct{}`, **next runnable level** = all node IDs in the next level whose predecessors are all in `completedSet`. For the first level, predecessors are empty so the first level is runnable.

### Ready steps per level

- **NextRunnable(order, g, completedSet)** (to be added in steppable or a small helper):
  - Build a map: for each node, its predecessors from `g.Edges`.
  - Scan in topological order; the “current level” is the first consecutive set of nodes that are not in `completedSet` and whose predecessors are all in `completedSet`.
  - Return these node IDs (deterministic: same order as in `order` for the same level).
- Steps that are **Wait** nodes (e.g. `planner.NodeWait`) are not run in parallel with others in the same level when we treat Wait as “blocking” the level; the current design runs one step at a time through the loop, so the first refactor can be “run all runnable steps in the current level in parallel, except Wait”. If a level contains a Wait node, we can either run the non-wait steps in parallel and then run the Wait step alone, or keep Wait handling as today (one step at a time until Wait is hit).

### Parallel execution within a level

- For the current level’s runnable steps (excluding Wait if desired for simplicity):
  - Launch one goroutine per step (or use a worker pool with max concurrency).
  - Each step receives a **copy** of the current `payload` (or read-only view) so steps do not share mutable state. Each step produces its own result (e.g. `payload.Results[nodeID] = ...`).
  - After all steps in the level complete: merge results into a single `payload.Results` (by node ID), then update `completedSet`, write NodeStarted/NodeFinished for each step (order can be by node ID for determinism), and advance the cursor/checkpoint once for the whole level.
- **Failure**: If any step in the level fails (retryable or permanent), the level is considered failed: do not apply results from other steps in the level (or apply only those that succeeded and then fail the job—design choice). Recommended: on first failure, cancel other in-flight steps (context cancellation) and treat the job as failed/retryable per existing semantics.

---

## Payload and checkpoint merge

- **Merge strategy**: For each node ID in the level, `payload.Results[nodeID] = resultFromStep(nodeID)`. Merge order: by node ID (sorted) so that the final `payload.Results` is deterministic.
- **Checkpoint**: After merging, create one node-level checkpoint that reflects the state after the entire level (or one checkpoint per step, depending on product requirement; single checkpoint per level reduces write volume and keeps semantics “cursor after level N”).
- **Cursor**: Update cursor once per level (or once per step if we keep per-step checkpoint for finer resume). See runLoop in runner.go: today cursor is updated after each step; for parallel level we update after the level.

---

## Lease and fencing

- **Single worker**: The job is still claimed by one worker. All steps in a level run on that worker. No cross-worker parallelism for the same job.
- **Writes**: Event Append (NodeStarted/NodeFinished, command_committed, etc.) and Ledger Commit and Cursor update are still done by that worker under the same job attempt_id. Lease fencing (see [scheduler-correctness.md](scheduler-correctness.md)) applies unchanged: Append and Ledger Commit validate attempt_id; cursor update must be done only by the lease holder.
- **Replay**: Replay and recovery logic consume events in order; as long as we write NodeFinished (and command_committed) in a deterministic order (e.g. by node ID) for the level, replay reconstructs the same completedSet and payload.

---

## Limits and configuration

- **Max concurrency per level**: Cap the number of steps run in parallel in one level (e.g. 4 or 8) to avoid resource exhaustion. Config: e.g. `Runner.MaxParallelSteps int` (0 = sequential, current behavior).
- **Cancellation on first failure**: When one step in the level fails, cancel the others via `context.CancelFunc` and do not commit results from the failed level (or commit only successful steps and then mark job failed—must be consistent with at-most-once and replay).
- **Step timeout**: Each step in the level still runs with `context.WithTimeout(runCtx, r.stepTimeout)` so a single slow step does not block the level indefinitely.

---

## Refactor outline (runLoop / Advance)

1. **Compute levels once** from the TaskGraph (or on demand): e.g. `Levels(order, g.Edges) [][]string` so that `Levels[i]` is the i-th level of node IDs.
2. **In runLoop**: Instead of iterating `for i := startIndex; i < len(steps)`, maintain “current level index” and “completedSet”. For each level:
   - Get runnable node IDs for this level (all in level if predecessors completed).
   - If the level contains a Wait node, fall back to sequential execution for that level (or run non-wait in parallel, then run Wait alone).
   - Run the level’s steps in parallel (with timeout per step); collect results; on first error, cancel others and return.
   - Merge results (sorted by node ID), write events (NodeStarted for each, then NodeFinished for each in same order), update completedSet and cursor.
3. **Advance**: Similarly, when advancing one step (or one level), compute next runnable and execute; keep Advance compatible with “one step at a time” if used by event-driven loop, or extend it to “one level at a time”.

---

## References

- [internal/agent/runtime/executor/compiler.go](../internal/agent/runtime/executor/compiler.go) — DAG build from TaskGraph
- [internal/agent/runtime/executor/steppable.go](../internal/agent/runtime/executor/steppable.go) — `TopoOrder`, `CompileSteppable`
- [internal/agent/runtime/executor/runner.go](../internal/agent/runtime/executor/runner.go) — runLoop, Advance, checkpoint and event writes
- [scheduler-correctness.md](scheduler-correctness.md) — lease, fencing, step timeout
- [step-contract.md](step-contract.md) — step semantics and determinism
