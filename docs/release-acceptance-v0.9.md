# v0.9 Release acceptance (runtime correctness)

With Postgres + Worker deployed, run the following 9 checks in order; all must pass.

## Prerequisites

- Deployed: 1 API + at least 1 Worker + Postgres (or use `deployments/compose`)
- `AETHERIS_API_URL` points to the API (default http://localhost:8080)
- CLI: `go run ./cmd/cli` or installed `aetheris`

---

## 1. Worker crash recovery

**Goal**: After killing a Worker, jobs are not lost; execution continues from checkpoint/replay without re-running completed nodes.

1. Start 1 API + 1 Worker + PG; create an Agent; send one message that triggers multiple steps (tool + LLM), note `job_id`.
2. Mid-execution (e.g. after first NodeFinished), `kill -9` the Worker process.
3. Start a new Worker (or another on the same machine).
4. **Verify**: The same `job_id` eventually reaches Completed; event stream has no duplicate PlanGenerated; tool call count matches (no re-execution of completed nodes).

```bash
# Example (adjust agent_id and message as needed)
aetheris agent create test
aetheris chat <agent_id>   # Send a message, note job_id; in another terminal kill -9 worker
# After restarting worker, poll job status until completed
aetheris trace <job_id>   # Inspect event sequence
```

---

## 2. API restart does not affect jobs

1. With a job running, restart the API (do not restart Worker).
2. **Verify**: Job is not lost, status does not regress, Worker continues to completion.

```bash
# After sending a message to get job_id, restart the API process; observe job still completed by worker
```

---

## 3. Multiple Workers, no duplicate execution

1. Start 3 Workers, create about 10 jobs in a row.
2. **Verify**: Each job completes exactly once; no duplicate tool calls or duplicate plan in logs/events.

```bash
# After starting 3 workers, use a script or loop to POST 10 messages; check each job's trace and completion count
```

---

## 4. Planner determinism

1. For the same job: export "execution graph / node sequence" before and after recovery (e.g. from trace or event stream).
2. **Verify**: DAG before recovery matches DAG after (same node set and order).

```bash
aetheris trace <job_id>   # Save a copy before recovery
# Trigger recovery (e.g. kill worker, restart)
aetheris trace <job_id>   # Get another copy; compare plan_generated and node sequence
```

---

## 5. Session persistence

1. After a multi-turn conversation, restart the Worker, then send one message that depends on prior context.
2. **Verify**: LLM response still references earlier turns.

```bash
aetheris chat <agent_id>  # Multi-turn
# Restart worker
# Send a message that depends on context; check reply continues the conversation
```

---

## 6. Tool idempotency

1. In a recovery scenario, for a completed tool node: confirm in logs/events that the tool was called only once; after recovery only later nodes run.
2. **Verify**: After recovery only subsequent nodes run; completed tools are not called again.

(Combined with 1 and 4: after recovery, trace has tool_called/tool_returned at most once per node.)

---

## 7. Event log replay

1. Use `GET /api/jobs/:id/trace` or `GET /api/jobs/:id/events` to get the event sequence.
2. **Verify**: Execution path can be fully replayed; result matches a single real recovery run.

```bash
aetheris replay <job_id>
# or
curl -s "$AETHERIS_API_URL/api/jobs/<job_id>/events"
```

---

## 8. Backpressure

1. Create 100+ jobs in a row (or above the configured concurrency limit).
2. **Verify**: No OOM, goroutines under control, LLM not overloaded; Worker respects `worker.concurrency`.

```bash
# Loop to create 100 jobs; watch memory and goroutine count; confirm worker max_concurrency is applied
```

---

## 9. Cancellation

1. While a job is running, call `POST /api/jobs/:id/stop`.
2. **Verify**: LLM stops, tool is interrupted, job moves to CANCELLED.

```bash
# Start a long task, note job_id; then:
aetheris cancel <job_id>
# or
curl -X POST "$AETHERIS_API_URL/api/jobs/<job_id>/stop"
# Check job status is cancelled, event stream contains job_cancelled
aetheris trace <job_id>
```

---

## Pass criteria

Only when all 9 items above pass may you tag **v0.9**.

Script skeleton: `scripts/release-acceptance-v0.9.sh` (can be filled with executable commands).
