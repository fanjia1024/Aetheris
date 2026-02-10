# Aetheris Runtime — Core sequence and StepOutcome

High-level execution flow and Step outcome world semantics. For the detailed execution-proof sequence (Adapter, Ledger Acquire/Commit, single tool path) see [execution-proof-sequence.md](execution-proof-sequence.md). For Ledger state machine and Confirmation Replay see [1.0-runtime-semantics.md](1.0-runtime-semantics.md).

---

## 1. Runner ↔ Ledger ↔ JobStore ↔ Worker (core execution flow)

InvocationLedger decides whether each step may execute the tool; at-most-once and replay semantics follow from this.

**Permission mapping (code):** NEW → `AllowExecute`, RECORDED → `ReturnRecordedResult`, INFLIGHT → `WaitOtherWorker` (see [1.0-runtime-semantics.md](1.0-runtime-semantics.md) Ledger state machine).

```mermaid
sequenceDiagram
    participant User
    participant AgentAPI
    participant JobStore
    participant Scheduler
    participant Runner
    participant InvocationLedger
    participant Worker
    participant Tool

    User->>AgentAPI: Submit Job
    AgentAPI->>JobStore: Create Job + initial event
    Scheduler->>Runner: Lease next Step
    Runner->>InvocationLedger: Request Tool Execution Permission
    alt Permission NEW
        InvocationLedger->>Runner: Grant permit
        Runner->>Worker: Execute Tool
        Worker->>Tool: Call Tool
        Tool-->>Worker: Tool Result
        Worker->>InvocationLedger: Commit Tool Result
        InvocationLedger->>JobStore: Append Tool Event
    else Permission RECORDED
        InvocationLedger->>Runner: Return cached result
    else Permission INFLIGHT
        InvocationLedger->>Runner: Wait
    end
    Runner->>Scheduler: Step Complete
    Scheduler->>AgentAPI: Job Progress Update
    AgentAPI->>User: Job Result / Replay Verification
```

**Notes:** Replay mode — Runner only reads from Ledger and never triggers tool calls. Worker crash or multiple workers — Ledger ensures the step runs at most once or waits.

---

## 2. StepOutcome state machine (world semantics)

Each step is classified into exactly one outcome; the diagram below describes the **conceptual lifecycle** of outcomes.

**In code:** Each step is classified into exactly one of these outcomes (e.g. tool success → SideEffectCommitted, non-tool success → Pure). See [internal/agent/runtime/executor/runner.go](internal/agent/runtime/executor/runner.go) (`ClassifyError` and step result handling).

```mermaid
stateDiagram-v2
    [*] --> Pure
    Pure --> SideEffectCommitted : Tool executed successfully
    Pure --> Retryable : Tool failed, retry allowed
    Pure --> PermanentFailure : Tool failed, cannot retry
    SideEffectCommitted --> Compensated : Rollback applied
    Retryable --> Pure : Retry execution
    PermanentFailure --> [*]
    Compensated --> [*]
```

| Outcome | Meaning |
|--------|---------|
| **Pure** | No side effects; safe to replay. |
| **SideEffectCommitted** | World changed; must not re-execute. |
| **Retryable** | Failure, world unchanged; retry allowed. |
| **PermanentFailure** | Failure; job cannot continue. |
| **Compensated** | Rollback applied; terminal. |

---

- **Detailed sequence (proof view):** [execution-proof-sequence.md](execution-proof-sequence.md)  
- **Semantics and Ledger state machine:** [1.0-runtime-semantics.md](1.0-runtime-semantics.md)
