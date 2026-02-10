# CoRag 1.0 release certification

This checklist is the **1.0 release gate**: execute in order and check off each item. **Any failure → do not release 1.0.**

---

## 0. Environment setup

**Minimum topology**: 1 API + 2 Workers + 1 storage.

- **Tests 3 and 8** require **Postgres** for job storage (`jobstore.type=postgres`); otherwise job state is not shared across API/process crashes and the tests cannot pass.
- Vector/metadata storage may be in-memory for this certification only.

**Start**:

```bash
# Terminal 1
go run ./cmd/api

# Terminal 2
go run ./cmd/worker

# Terminal 3
go run ./cmd/worker
```

**Verify**:

```bash
curl -s http://localhost:8080/api/health
```

- **Pass**: 200, response indicates OK.

---

## Part 1: Runtime correctness (v0.9 gate)

### Test 1: Job not lost

1. Create agent:
   ```bash
   curl -s -X POST http://localhost:8080/api/agents -H "Content-Type: application/json" -d '{}'
   ```
   Record the returned `id` as `agent_id`.

2. Send message:
   ```bash
   curl -s -X POST http://localhost:8080/api/agents/{agent_id}/message \
     -H "Content-Type: application/json" \
     -d '{"message":"Write a 200-word intro to AI and call a tool"}'
   ```
   Get **202 Accepted**, record `job_id`.

3. Poll status:
   ```bash
   curl -s http://localhost:8080/api/jobs/{job_id}
   ```

**Pass**: Status goes `pending` → `running` → `completed`. If it stays `pending`, Scheduler/Worker are not running → **fail**.

---

### Test 2: Worker crash recovery (critical for 1.0)

1. **While a job is running**, kill one worker:
   ```bash
   ps aux | grep worker
   kill -9 <pid>
   ```

2. Watch the same job:
   ```bash
   curl -s http://localhost:8080/api/jobs/{job_id}
   ```

**Required for 1.0**:

- Same `job_id` **eventually** becomes `completed` (may stay `running` until another Worker reclaims).
- **No re-plan** (only one `plan_generated` in event stream).
- **No restart from scratch**, **no lost steps** (resume from replay/checkpoint; completed nodes not re-run).

If it goes `RUNNING` → `FAILED`, or restarts from the beginning → **do not release 1.0**.

*Note: There is no `stalled` state today; if "lease expired, no heartbeat" is added later, you might see RUNNING → STALLED → RUNNING → COMPLETED.*

---

### Test 3: API crash

1. While a job is running, stop the API (e.g. Ctrl+C).
2. After ~10 seconds, restart API: `go run ./cmd/api`.

**Required for 1.0**: The same job continues and completes (Worker continues Claim/execute from Postgres).

If the job is lost or status regresses, job state was only in API memory → **not a runtime**. This test requires `jobstore.type=postgres`.

---

### Test 4: Multi-Worker concurrency consistency

Create 10 jobs at once:

```bash
for i in $(seq 1 10); do
  curl -s -X POST http://localhost:8080/api/agents/{agent_id}/message \
    -H "Content-Type: application/json" \
    -d '{"message":"Query weather and summarize"}' &
done
wait
```

**Check**: In logs and events **each job is executed exactly once**. If the same job is run by two Workers or the same tool is called twice → Claim/Lease is broken → **do not release 1.0**.

---

### Test 5: Replay consistency

After jobs complete, restart all Workers and verify:

1. `kill -9` all workers, start workers again.
2. Call **read-only** Replay/Trace (**do not trigger execution**):
   ```bash
   curl -s http://localhost:8080/api/jobs/{job_id}/replay
   # or
   curl -s http://localhost:8080/api/jobs/{job_id}/trace
   curl -s http://localhost:8080/api/jobs/{job_id}/events
   ```

**Required for 1.0**:

- **Same execution path** (consistent with event stream).
- **No LLM calls**, **no tool calls** (these endpoints only read events, do not run Runner).

If "replay" actually re-runs the flow, it is not runtime replay → **fail**.

---

### Test 6: Cancel job

While a job is running:

```bash
curl -s -X POST http://localhost:8080/api/jobs/{job_id}/stop
```

**Pass**:

- Status goes `running` → `cancelled`.
- LLM stops promptly; Worker stops executing that job.

If the job keeps running → not acceptable for production → **fail**.

---

## Part 2: 1.0 platform (v1.0 gate)

### Test 7: Trace explainability

For any completed job, the API must show:

- Each node (node_id, type).
- For tool nodes: tool input/output (from `tool_called` / `tool_returned` or `node_finished` payload).
- Timing (from `node_started` / `node_finished` timestamps or duration in trace).

**1.0 definition**: User can understand **why** the agent made a decision. If only logs like `INFO executing node...` are visible, that is not Trace UI / explainability → **fail**.

Example:

```bash
curl -s http://localhost:8080/api/jobs/{job_id}/trace
curl -s http://localhost:8080/api/jobs/{job_id}/nodes/{node_id}
```

---

### Test 8: Full recovery (hardest)

1. Create one **long job** (multiple tool/LLM steps).
2. **At the same time** kill:
   ```bash
   kill -9 all workers
   kill -9 api
   ```
3. Wait ~10 seconds.
4. **Restart everything**: API first, then 2 Workers.

**Pass**: The same job continues and **completes** → true Agent Runtime 1.0.

If it fails → that is the gap to 1.0. This test requires Postgres job storage.

---

## Part 3: Operations

1.0 must support troubleshooting:

| Capability | API |
|------------|-----|
| List all jobs for an agent | `GET /api/agents/{id}/jobs` |
| Job details | `GET /api/jobs/{id}` |
| Job execution steps | `GET /api/jobs/{id}/trace`, `GET /api/jobs/{id}/events`, `GET /api/jobs/{id}/nodes/{node_id}` |
| Force cancel | `POST /api/jobs/{id}/stop` |
| Replay / read-only | `GET /api/jobs/{id}/replay` or trace+events |

"Re-execute" means sending a new message to create a new job, not running the same job again.

---

## Release criteria (final)

Only when **all four** below hold can you claim:

> **CoRag v1.0 — Agent Runtime Platform**

1. **No job loss on any process crash**
2. **Recovery does not re-call tools**
3. **Full replay** (read-only trace/replay, no re-run)
4. **User can observe why execution happened** (Trace: nodes, tool I/O, timing)

If any is missing → at most v0.9.

---

## Automation

Steps that can be automated (once API + Workers are up) can be run with:

```bash
./scripts/release-cert-1.0.sh
```

The script covers: Step 0 (health), Test 1 (job not lost), Test 6 (cancel), Test 7 (Trace), optional Test 4 (multi job), Test 5 (Replay). Tests 2, 3, and 8 require **manual** kill/restart and verification as above.
