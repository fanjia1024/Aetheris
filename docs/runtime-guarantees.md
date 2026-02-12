# Aetheris Runtime Guarantees — 你可以依赖什么

This document explains what Aetheris guarantees in normal and failure scenarios. For technical details, see [design/execution-guarantees.md](../design/execution-guarantees.md) and [design/scheduler-correctness.md](../design/scheduler-correctness.md).

---

## Normal Scenario Guarantees

| Scenario | Guarantee | Condition |
|----------|-----------|-----------|
| **Worker executes step** | At-most-once | Configured InvocationLedger + Effect Store |
| **Tool calls external API** | Pass idempotency key → downstream dedup | Tool uses `StepIdempotencyKeyForExternal(ctx, jobID, stepID)` |
| **LLM generates result** | Stored in Effect Store; Replay injects (NOT re-called) | Effect Store configured |
| **Signal sent** | At-least-once (write wait_completed → Job scheduled) | WakeupQueue for multi-worker |
| **Checkpoint saved** | After each step; crash recovery from latest Checkpoint | CheckpointStore configured |

### DAG parallel execution (2.0)

When **max parallel steps** is configured (> 0), steps in the same topological level may run in parallel on a single worker. See [design/dag-parallel-execution.md](../design/dag-parallel-execution.md).

| Aspect | Behavior |
|--------|----------|
| **Max concurrency** | Configurable per Runner (`SetMaxParallelSteps(n)`); 0 = sequential (default). |
| **Failure** | If any step in a level fails, the level is failed; the job is marked failed and no results from that level are committed. Other in-flight steps in the level are effectively canceled (context). |
| **Replay / determinism** | Results are merged by node ID in sorted order; NodeStarted/NodeFinished are written in deterministic order. Replay and checkpoint semantics remain the same. |
| **Wait nodes** | Levels that contain a Wait node are run sequentially (one step at a time) so Wait semantics are unchanged. |

---

## Failure Scenarios

### 1. Worker Crash (Medium Frequency)

**Scenario**: Worker terminates mid-execution (process killed, machine power loss, network disconnect)

**What happens**:

1. Worker holds Job lease with `attempt_id` and heartbeat (30s TTL)
2. **Crash** → heartbeat stops → lease expires after 30s
3. **Scheduler Reclaim**: Scans expired leases, sets Job back to Pending
4. **Another Worker claims**: Loads from latest Checkpoint
5. **Replay**: Already-completed steps injected from event stream (no re-execution)
6. **Tool side effect**: If Tool executed but crash before `command_committed`:
   - Effect Store has record → catch-up: append event without re-executing Tool
   - Ledger/InvocationStore recovery flow ensures Tool NOT re-executed

**Guarantees**:
- ✅ Job not lost (Reclaim ensures eventual progress)
- ✅ Step not duplicated (Ledger + Effect Store at-most-once)
- ✅ Maximum loss: Progress of last step (redo from Checkpoint, but Replay injects recorded results)

**Example**:
```
Worker A: query_order (success) → llm_decide (success) → send_refund (executing...) → CRASH
Worker B: Reclaim → Replay: query_order, llm_decide injected → send_refund: check Ledger → injected (NOT re-executed)
Result: Refund sent once
```

**Configuration requirements**:
- JobStore: Postgres (or shared store)
- Event Store: Postgres (lease management)
- InvocationLedger: Enabled
- Effect Store: Enabled

---

### 2. Step Timeout (High Frequency)

**Scenario**: Step execution exceeds configured timeout (e.g., 5 minutes)

**What happens**:

1. Runner wraps step execution in `context.WithTimeout(stepTimeout)`
2. **Timeout** → context canceled → step returns `context.DeadlineExceeded`
3. **Classification**: `StepResultRetryableFailure`
4. **Job status**: Set to Failed (or Requeue based on retry policy)
5. **Tool partial execution**: If Tool started but not committed:
   - Next replay: Activity Log Barrier (event stream has tool_invocation_started, no finished)
   - **Block re-execution**: Recover from Ledger or fail permanently

**Guarantees**:
- ✅ Timeout does NOT cause "step half-done, state inconsistent"
- ✅ Tool already called but uncommitted → NOT re-executed (Activity Log Barrier)

**Configuration**:
```yaml
# configs/worker.yaml
executor:
  step_timeout: "5m"  # Per-step timeout
```

**Example**:
```
Step: call_slow_api (timeout 5m)
3 min: API responding...
5 min: Timeout → context canceled
→ Step classified as retryable_failure
→ Job Failed (or Requeue if retry_max > 0)
→ Tool invocation recorded as "started" but not "finished"
→ Replay: Block re-execution (wait for recovery or fail)
```

---

### 3. Signal Lost (Low Risk)

**Scenario**: POST `/api/jobs/:id/signal` request fails (network down, API crash)

**What happens**:

1. **If `wait_completed` NOT written**: Job remains StatusWaiting/StatusParked
2. **Retry signal**: Idempotent (see [design/runtime-contract.md](../design/runtime-contract.md) § External Event Guarantee)
   - If last event already `wait_completed` with same `correlation_key` → return 200 (already delivered)
3. **WakeupQueue**: If configured, signal → NotifyReady → Worker immediately claims (no poll delay)

**Guarantees**:
- ✅ Signal at-least-once (once wait_completed written, Job WILL be scheduled)
- ✅ Duplicate signal idempotent (no double-unblock)

**Recovery**:
```bash
# Check if signal delivered
curl -s http://localhost:8080/api/jobs/job-xxx/replay | jq '.events[] | select(.type=="wait_completed")'

# If empty → signal not delivered, re-send:
curl -X POST http://localhost:8080/api/jobs/job-xxx/signal \
  -d '{"correlation_key": "approval-xxx", "payload": {"approved": true}}'

# If returns 200 → delivered (even if duplicate)
```

---

### 4. Two Workers Execute Same Step (Very Low Risk)

**Scenario**: Lease just expired, two Workers simultaneously claim same Job

**What happens**:

1. **Event Store Append**: Validates `attempt_id` (see [design/runtime-contract.md](../design/runtime-contract.md) § Execution Epoch)
   - Only Worker with current lease's `attempt_id` can write events
   - Other Worker gets `ErrStaleAttempt` → aborts
2. **Tool Ledger Acquire**: Same `idempotency_key` → only one Worker can Commit
   - Worker A: Acquire → AllowExecute → execute tool → Commit
   - Worker B: Acquire → WaitOtherWorker or ReturnRecordedResult

**Guarantees**:
- ✅ Tool executed exactly once (Ledger arbitration)
- ✅ Event stream not polluted by stale Worker (attempt_id validation)

**Example**:
```
Time 0s: Worker A claims job-123 (attempt_id=attempt-1, lease expires at 30s)
Time 30s: Lease expires
Time 30.1s: Worker B claims job-123 (attempt_id=attempt-2)
Time 30.2s: Worker A tries to append event → ErrStaleAttempt (attempt-1 != attempt-2)
Time 30.3s: Worker B executes tool → Ledger Acquire → AllowExecute → success
Result: Tool executed once by Worker B
```

---

### 5. LLM Model Update (Medium Impact)

**Scenario**: First execution uses gpt-4o-2024-08-06; replay time uses gpt-4o-2024-11-20

**What happens**:

1. **First execution**: LLM called → response recorded to Effect Store (with model metadata)
2. **Replay**: Effect Store has record → inject response → **LLM NOT called**
3. **Trace**: Shows `llm_model: gpt-4o-2024-08-06` (original)
4. **Warning log**: Model version changed (if version tracking configured)

**Guarantees**:
- ✅ Replay result matches first execution (not affected by model update)
- ✅ Audit can trace "which model was used during execution"

**Example**:
```
First exec (2024-08-06): LLM(model=gpt-4o-2024-08-06) → "Approve"
Replay (2024-11-20): Inject "Approve" from Effect Store (model=gpt-4o-2024-08-06)
→ Replay output = "Approve" (even if new model would return "Reject")
→ Deterministic
```

---

### 6. Tool Schema Change (Medium Impact)

**Scenario**: Tool API updated (e.g., `/api/price?sku=123` returns `$10` → `$12`)

**What happens**:

1. **First execution**: Tool called → result `$10` recorded
2. **Replay**: Inject `$10` from Ledger/Effect Store → Tool NOT re-called
3. **Versioning**: tool_invocation_started includes `tool_version`, `request_schema_hash` (see [design/versioning.md](../design/versioning.md))
4. **Audit**: Can explain "why historical execution returned X, now returns Y" (tool version changed)

**Guarantees**:
- ✅ Replay uses recorded result (not affected by tool schema change)
- ✅ Version tracking for audit (tool_version, schema_hash)

---

### 7. Database Transaction Rollback (Edge Case)

**Scenario**: Tool writes to database, transaction commits, but process crashes before writing command_committed

**What happens**:

1. **Tool executed**: Database transaction committed (external side effect done)
2. **Crash**: Before writing `command_committed` to event stream
3. **Effect Store** (if enabled): Tool result already written (two-phase commit)
4. **Replay**: Effect Store has record → catch-up: append command_committed without re-executing Tool
5. **Without Effect Store**: Activity Log Barrier (tool_invocation_started, no finished) → Block re-execution → Recover from Ledger or fail

**Guarantees**:
- ✅ With Effect Store: Catch-up writes event, Tool NOT re-executed (two-phase commit)
- ✅ Without Effect Store: Block re-execution (Activity Log Barrier), wait for manual recovery

**Prevention**: Always configure Effect Store for production.

---

### 8. Network Partition (Split Brain)

**Scenario**: Worker A loses network to JobStore, Worker B claims same Job

**What happens**:

1. Worker A: Executing, but cannot heartbeat → lease expires
2. Worker B: Claims Job (new `attempt_id`)
3. Worker A: Network recovers, tries to append event → `ErrStaleAttempt` (attempt_id mismatch)
4. Worker A: Aborts execution
5. Worker B: Continues from Checkpoint

**Guarantees**:
- ✅ Only one Worker can progress (attempt_id validation)
- ✅ Tool executed once (Ledger arbitration)

**Configuration**: Ensure JobStore/Event Store accessible to all Workers (shared Postgres, not local file).

---

## Configuration Requirements

### Development Mode (Minimal)

```yaml
jobstore:
  type: memory  # In-memory, no persistence

effect_store:
  enabled: false  # Optional

invocation_ledger:
  enabled: false  # Optional
```

**Guarantees**: Basic execution, no crash recovery, no at-most-once (tools may duplicate on retry)

**Use for**: Local testing, prototyping

---

### Production Mode (Recommended)

```yaml
jobstore:
  type: postgres
  postgres:
    dsn: "postgres://aetheris:aetheris@localhost:5432/aetheris"

effect_store:
  enabled: true      # ← Required for at-most-once & LLM replay guard
  type: postgres

invocation_ledger:
  enabled: true      # ← Required for Tool at-most-once
  type: postgres

wakeup_queue:
  type: redis        # ← Required for multi-worker signal delivery
  redis:
    addr: "redis:6379"
```

**Guarantees**: All guarantees in [design/execution-guarantees.md](../design/execution-guarantees.md) hold

**Use for**: Production deployment, multi-worker, crash recovery, audit

---

## Guarantee Summary Table

| What Could Go Wrong | What Happens | Guarantee | Required Config |
|---------------------|--------------|-----------|-----------------|
| Worker crash | Reclaim → another Worker resumes | Job not lost | Postgres JobStore |
| Worker crash during Tool | Effect Store catch-up | Tool NOT re-executed | Effect Store |
| Step timeout | Classified as retryable_failure | Timeout safe | step_timeout configured |
| Signal lost | Retry signal (idempotent) | At-least-once delivery | (always) |
| Two Workers same step | attempt_id validation | Only one succeeds | Event Store + Ledger |
| LLM model update | Replay injects old output | Deterministic | Effect Store |
| Tool schema change | Replay injects old result | Audit traceable | tool_version tracking |
| Database rollback | Catch-up or barrier | Tool NOT re-executed | Effect Store |
| Network partition | attempt_id mismatch | Split-brain safe | Shared JobStore |

---

## Testing Failure Scenarios

### Test 1: Worker Crash During Tool

```bash
# Terminal 1: Start API
go run ./cmd/api

# Terminal 2: Start Worker
go run ./cmd/worker

# Terminal 3: Create job with long-running tool
curl -X POST http://localhost:8080/api/agents/test-agent/message \
  -d '{"message": "test crash during tool"}'

# Watch Worker logs → when tool starts executing, kill Worker (Ctrl+C)

# Terminal 4: Start new Worker
go run ./cmd/worker

# Observe:
# - New Worker claims job
# - Replay: Tool result injected (NOT re-executed)
# - Job completes
# - Check Ledger: only one invocation record
```

**Expected**: Tool executed once, no duplicate side effect.

---

### Test 2: Signal Lost (Retry)

```bash
# Create job with Wait node
POST /api/agents/agent-1/message

# Job enters StatusParked

# Send signal (simulate network failure by killing API mid-request)
POST /api/jobs/job-xxx/signal
→ 500 Internal Server Error (API crashed)

# Restart API
go run ./cmd/api

# Retry signal (idempotent)
POST /api/jobs/job-xxx/signal
→ 200 OK

# Check events
GET /api/jobs/job-xxx/replay
→ Only ONE wait_completed (not duplicate)
```

**Expected**: Signal idempotent, Job resumes once.

---

### Test 3: Two Workers Same Job

```bash
# Configure short lease TTL (for testing)
# worker.yaml: lease_ttl: "5s"

# Start 2 Workers
# Terminal 1: go run ./cmd/worker
# Terminal 2: go run ./cmd/worker

# Create job
POST /api/agents/agent-1/message

# Observe logs:
# - Worker A claims (attempt_id=attempt-1)
# - Lease expires (5s)
# - Worker B claims (attempt_id=attempt-2)
# - Worker A tries to append → ErrStaleAttempt → aborts
# - Worker B continues

# Check Tool invocations
GET /api/jobs/job-xxx/trace
→ Only ONE tool_invocation_finished (not duplicate)
```

**Expected**: Tool executed once, only one Worker succeeded.

---

## When Guarantees Do NOT Hold

### Scenario 1: Effect Store Not Configured

**Without Effect Store**:
- ❌ at-most-once NOT guaranteed for Tools (may duplicate on crash before commit)
- ❌ LLM replay guard weakened (Adapter layer still checks, but no two-phase commit)

**Use for**: Development only; **DO NOT use in production**

---

### Scenario 2: Ledger Not Configured

**Without InvocationLedger**:
- ❌ Tool at-most-once NOT guaranteed across Workers
- ❌ Two Workers may execute same Tool (if both claim simultaneously)

**Use for**: Single-worker dev; **DO NOT use in multi-worker production**

---

### Scenario 3: WakeupQueue Not Configured (Multi-Worker)

**Without WakeupQueue**:
- ⚠️ Signal delivery has delay (poll interval, default 2s)
- ⚠️ Under high load (1k+ parked jobs), poll inefficient

**Use for**: Single-worker or low-load; multi-worker production should configure Redis/Postgres WakeupQueue

---

### Scenario 4: Violating Step Contract

**If developer breaks [design/step-contract.md](../design/step-contract.md)**:
- ❌ Step directly calls external API (not via Tool) → Replay will re-execute → duplicate side effect
- ❌ Step reads `time.Now()` or `rand.Int()` → Replay non-deterministic
- ❌ Step modifies global state → Replay behavior unpredictable

**Fix**: Follow Step Contract (external side effects must go through Tools).

---

## Disaster Recovery

### Scenario: Total Data Loss (Postgres Crash)

**If JobStore/Event Store lost**:
- ❌ All Jobs lost (no recovery)
- ❌ Execution history lost (no audit)

**Prevention**:
- **Database backups**: pg_dump or continuous archiving
- **Replicas**: Postgres streaming replication
- **Disaster recovery plan**: RTO/RPO requirements

Aetheris provides runtime guarantees **given persistent storage**; storage layer disaster recovery is infrastructure responsibility.

---

### Scenario: Ledger Corruption

**If InvocationLedger inconsistent with event stream**:
- ⚠️ Tool may be blocked (Ledger says "in progress", event stream says "finished")
- ⚠️ Manual intervention: Query Ledger + Event Store, reconcile

**Prevention**:
- Use transactional stores (Postgres Ledger + Event Store in same DB)
- Regular consistency checks (TODO: `aetheris check-consistency` CLI command)

---

## SLA & Performance Characteristics

### Latency

| Operation | Latency | Note |
|-----------|---------|------|
| **Job creation** | ~50ms | POST /api/agents/:id/message → job_id |
| **Worker claim** | ~20ms | Poll JobStore, return Pending job |
| **Step execution** | Depends on Tool/LLM | Aetheris overhead ~5ms per step |
| **Signal delivery** | <100ms | With WakeupQueue; ~2s without (poll interval) |
| **Checkpoint save** | ~30ms | Write to Postgres |
| **Replay** | ~50ms per 100 steps | Read event stream, inject results |

### Throughput

| Metric | Value | Note |
|--------|-------|------|
| **Jobs/sec** | ~100-500 | Single API, Postgres JobStore |
| **Concurrent Jobs** | ~10k+ | With StatusParked (long-wait jobs don't block) |
| **Workers** | Scales linearly | Add Workers → higher throughput |
| **Event stream size** | ~1KB per step | 100-step job ≈ 100KB events |

### Resource Usage

| Component | Memory | CPU | Disk |
|-----------|--------|-----|------|
| **API** | ~100MB | ~5% | Minimal (logs only) |
| **Worker** | ~50MB per job | ~20% per job | Minimal |
| **Postgres** | Depends on job count | ~10% | ~1MB per job (events + checkpoints) |

**Scaling**: Horizontal (add Workers) + Vertical (Postgres resources)

---

## Production Checklist

Before deploying Aetheris to production:

- [ ] **Effect Store enabled** (at-most-once guarantee)
- [ ] **InvocationLedger enabled** (Tool at-most-once)
- [ ] **JobStore = Postgres** (crash recovery)
- [ ] **WakeupQueue configured** (multi-worker signal delivery)
- [ ] **Postgres backups** (disaster recovery)
- [ ] **Monitoring** (Prometheus metrics, see [docs/observability.md](observability.md))
- [ ] **Step timeout configured** (prevent hung steps)
- [ ] **Retry policy configured** (max retries, backoff)
- [ ] **All Tools follow Step Contract** (no direct external calls, use idempotency key)
- [ ] **Test failure scenarios** (worker crash, step timeout, signal retry)

---

## References

- [design/execution-guarantees.md](../design/execution-guarantees.md) — Formal guarantees table
- [design/scheduler-correctness.md](../design/scheduler-correctness.md) — Lease, heartbeat, reclaim
- [design/step-contract.md](../design/step-contract.md) — How to write correct steps
- [design/effect-system.md](../design/effect-system.md) — Effect Store, replay, two-phase commit
- [design/runtime-contract.md](../design/runtime-contract.md) — Blocking, epoch, attempt_id validation
