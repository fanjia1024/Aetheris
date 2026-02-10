# Step Result Contract and Failure Model (Phase A)

This document defines the **Step Result** type and **event stream changes** for deterministic failure semantics. Replay and recovery depend on every step completion being classified so that partial commits do not cause duplicate side effects.

## 1. Step Result type (Go)

```go
// StepResultType is the classification of a step completion (Production Semantics Phase A).
type StepResultType string

const (
	StepResultSuccess              StepResultType = "success"
	StepResultRetryableFailure     StepResultType = "retryable_failure"
	StepResultPermanentFailure     StepResultType = "permanent_failure"
	StepResultCompensatableFailure StepResultType = "compensatable_failure"
)

// StepResult carries the outcome of a step execution for the event stream and Runner.
type StepResult struct {
	Type   StepResultType // required
	Reason string         // optional; error message or classification reason
	// PayloadResults is set on success (and optionally on compensatable with partial state)
}
```

**Semantics:**

| Type | Meaning | Replay |
|------|--------|--------|
| success | Step completed; side effects committed. | CompletedNodeIDs += node_id; PayloadResults updated; command_committed entries honored. |
| retryable_failure | Transient (LLM format, network timeout). | Do **not** add to CompletedNodeIDs; on restart Runner may retry this step. |
| permanent_failure | Non-retryable (404, permission, business rejection). | Do **not** add to CompletedNodeIDs; job failed; do not re-execute this step. |
| compensatable_failure | Partial commit (e.g. release created, upload failed). | Do **not** add to CompletedNodeIDs until compensation runs (Phase B); then treat as terminal. |

## 2. Event stream: extend node_finished (Option 2)

We **extend** `node_finished` so it is emitted on **all** step completions (success and failure). No new event type.

**Payload fields (add to existing):**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| result_type | string | yes | One of: success, retryable_failure, permanent_failure, compensatable_failure. |
| reason | string | no | Human-readable reason (e.g. error message). |

Existing fields (node_id, payload_results, trace_span_id, parent_span_id, step_index, duration_ms, state, attempt) unchanged. For backward compatibility, if `result_type` is missing, treat as **success** so old jobs still replay correctly.

**Who writes:** Runner (via NodeEventSink.AppendNodeFinished). On success: result_type=success, payload_results as today. On failure: result_type=retryable_failure | permanent_failure | compensatable_failure, reason=err.Error(), payload_results may be empty or partial.

## 3. Runner behavior

1. Before step: AppendNodeStarted (unchanged).
2. Execute: `payload, runErr = step.Run(ctx, payload)`.
3. **Classify:** If runErr != nil, map to StepResult: use errors.As/Is for RetryableError, PermanentError, CompensatableError (see below); default = PermanentFailure with reason runErr.Error().
4. **Always** call AppendNodeFinished(nodeID, payloadResults, durationMs, state, attempt, **resultType, reason**). On success payloadResults = payload.Results; on failure payloadResults may be nil or partial.
5. **Branch:**
   - **Success:** AppendStateCheckpointed, checkpoint, continue.
   - **RetryableFailure:** Optionally retry (future: retry policy); for now fail job (no checkpoint advance).
   - **PermanentFailure / CompensatableFailure:** UpdateStatus failed; return (no checkpoint). CompensatableFailure may trigger compensation in Phase B.

## 4. Classifying errors (adapter / caller)

Adapters today return `(payload, error)`. We introduce **sentinel errors** or **wrappers** so Runner can classify:

```go
// RetryableError marks a step failure as retryable (transient).
var RetryableError = errors.New("retryable")

// PermanentError marks a step failure as non-retryable.
var PermanentError = errors.New("permanent")

// CompensatableError marks partial commit (compensation may run).
var CompensatableError = errors.New("compensatable")
```

Or a wrapper:

```go
type StepFailure struct { Type StepResultType; Err error }
func (e *StepFailure) Error() string { return e.Err.Error() }
```

Runner: `if runErr != nil { resultType, reason = classifyError(runErr) } else { resultType = success }`.

Default: if error is not RetryableError/CompensatableError, treat as **PermanentFailure**.

## 5. Replay rules (BuildFromEvents)

In [internal/agent/replay/replay.go](internal/agent/replay/replay.go):

- For each **node_finished** event, parse payload for `result_type` (and `reason`).
- **Only** when `result_type == "success"` (or `result_type` is missing, for backward compat): add node_id to CompletedNodeIDs, update CursorNode and PayloadResults.
- When result_type is retryable_failure, permanent_failure, or compensatable_failure: do **not** add to CompletedNodeIDs; do not update PayloadResults. The job has failed; ReplayContext will not consider this node complete, so a future run would not skip it — but for failed jobs we do not continue anyway. Important: we must not advance the "completed" set so that if we ever support "retry from last retryable" the step is retried.

Document in [design/event-replay-recovery.md](design/event-replay-recovery.md).

## 6. Trace UI

- Narrative and step view already consume `state` from node_finished. Extend to consume **result_type** and **reason**: show in step list (e.g. "Node n1 · permanent_failure") and in detail panel (Step view: Result type, Reason).
- Timeline segment status can map result_type to status: success → ok, retryable_failure → retryable, permanent_failure / compensatable_failure → failed.

## 7. Backward compatibility

- Old events: node_finished without result_type → Replay treats as success (existing behavior).
- New code: always emit result_type on node_finished; Replay only advances on success (or missing for old events).

## 8. References

- [production_semantics_roadmap](.cursor/plans/production_semantics_roadmap_18cdd7f4.plan.md) — Phase A scope
- [event-replay-recovery.md](event-replay-recovery.md) — Replay semantics
- [trace-event-schema-v0.9.md](trace-event-schema-v0.9.md) — node_finished payload
