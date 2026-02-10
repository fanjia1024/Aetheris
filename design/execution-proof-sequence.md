# Execution Proof — Runner ↔ Ledger ↔ JobStore ↔ Worker

Runner does not own tool execution; **Ledger is the only arbiter**. The tool is called only when Ledger returns `AllowExecute`. All other paths only read from Store or replay and inject results.

---

## Sequence diagram

```mermaid
sequenceDiagram
  participant Worker
  participant Runner
  participant Adapter
  participant Ledger
  participant Store
  participant JobStore
  participant Tool

  Worker->>JobStore: ListEvents(jobID) / get cursor
  JobStore-->>Worker: events
  Worker->>Runner: RunForJob(job, replayCtx)
  Runner->>Runner: Build ReplayContext from events (if replay)
  Runner->>Runner: Inject CompletedToolInvocations, StateChangesByStep into ctx

  loop For each step
    Runner->>Adapter: step.Run(ctx, payload)  [tool node]
    Adapter->>Ledger: Acquire(jobID, stepID, tool, argsHash, idempotencyKey, replayResult)
    Ledger->>Store: GetByJobAndIdempotencyKey
    Store-->>Ledger: record or nil

    alt replayResult present or committed record in Store
      Ledger-->>Adapter: ReturnRecordedResult + record
      Adapter->>Adapter: runConfirmation (ResourceVerifier)
      alt Verification fails
        Adapter-->>Runner: PermanentFailure (job fails)
      else Verification OK
        Adapter->>Adapter: Inject record.Result into payload
        Adapter-->>Runner: payload (no tool call)
      end
    else Record exists but not committed
      Ledger-->>Adapter: WaitOtherWorker
      Adapter-->>Runner: RetryableFailure
    else No record
      Ledger->>Store: SetStarted(record)
      Ledger-->>Adapter: AllowExecute + record
      Adapter->>JobStore: Append tool_invocation_started
      Adapter->>Tool: Execute(toolName, input)
      Tool-->>Adapter: result
      Adapter->>Ledger: Commit(invocationID, idempotencyKey, result)
      Ledger->>Store: SetFinished(success, result, committed=true)
      Adapter->>JobStore: Append tool_invocation_finished, command_committed
      Adapter-->>Runner: payload
    end

    Runner->>JobStore: Append NodeFinished, StateCheckpointed
    Runner->>Runner: Update cursor / checkpoint
  end

  Runner-->>Worker: nil (success) or error
```

---

See [1.0-runtime-semantics.md](1.0-runtime-semantics.md) for the three mechanisms (Tool Invocation Ledger, StepOutcome world semantics, Confirmation Replay) and the Execution Proof Chain section.
