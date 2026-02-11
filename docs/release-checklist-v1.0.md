# Aetheris v1.0 post-release checklist

Use after release to quickly confirm core features, event tracing, distributed execution, CLI/API, logging, and docs.

---

## 1. Core features

- [ ] **Job creation** — Jobs can be created via Agent API or CLI. Check: `POST /api/agents/:id/message` or `aetheris chat <agent_id>`; confirm `job_id` is returned.
- [ ] **Job execution** — Job runs from start to completion. Check: Run example DAG or TaskGraph, observe job status until completed.
- [ ] **Event stream** — Every action (planning, tool call, step, retry, failure) is recorded. Check: `GET /api/jobs/:id/events` or `aetheris trace <job_id>`.
- [ ] **Job replay** — Job can be replayed with same result. Check: `GET /api/jobs/:id/replay` or `aetheris replay <job_id>`.
- [ ] **Idempotency** — Repeating replay or query does not cause duplicate writes or side effects. Check: Re-trigger replay/query for same job, confirm no duplicates.
- [ ] **Job resume after interrupt** — After simulating an interrupt, job can resume. Check: Kill Runner or Worker while running; after restart, resume via `POST /api/agents/:id/resume` or Scheduler.

## 2. Distributed execution and Workers

- [ ] **Multiple Workers** — Tasks can be assigned to different Workers. Check: Run 2+ Workers (e.g. multiple `go run ./cmd/worker`), run jobs for same Agent, confirm different Workers handle them.
- [ ] **Scheduler retry** — Failed tasks are retried. Check: Force failure (e.g. disconnect, bad tool), observe Scheduler retry and backoff.
- [ ] **Runner checkpoint** — Each step has a checkpoint and can resume. Check: Stop Runner mid-run, restart Worker, confirm execution resumes from checkpoint.

## 3. CLI and Agent API

- [ ] **CLI commands** — Common commands work. Check: `aetheris agent create [name]`, `aetheris chat [agent_id]`, `aetheris jobs <agent_id>`, `aetheris trace <job_id>`, `aetheris replay <job_id>`, `aetheris cancel <job_id>`, `aetheris workers`, `aetheris debug <job_id>`.
- [ ] **Agent API** — REST endpoints work. Check: curl, Postman, or script for `POST /api/agents`, `POST /api/agents/:id/message`, `GET /api/agents`, etc.
- [ ] **Job cancel** — Job cancellation is supported. Check: `POST /api/jobs/:id/stop` or `aetheris cancel <job_id>`; job reaches cancelled/stopped.
- [ ] **Event query** — Event stream can be queried by job. Check: `GET /api/jobs/:id/events` or `aetheris trace <job_id>`, `aetheris replay <job_id>`.

## 4. Logging and monitoring

- [ ] **Trace UI** — Task execution state, DAG, TaskGraph are visible. Check: Open URL from `GET /api/jobs/:id/trace/page`, or inspect `GET /api/jobs/:id/trace` JSON.
- [ ] **Logs** — Include job execution, errors, retries. Check: API/Worker logs for key steps and errors.
- [ ] **Audit** — Every decision can be traced. Check: Sample historical job events/trace; planning, tool calls, node completion are visible.

## 5. Docs and version

- [ ] **README** — Matches v1.0 behavior. Check: Architecture, run instructions, CLI description match implementation.
- [ ] **CHANGELOG** — Includes v1.0. Check: CHANGELOG.md and GitHub Release notes.
- [ ] **AGENTS.md** — Up-to-date agent examples and project scope. Check: Commands, layout, tech stack in docs.
- [ ] **GitHub Release** — v1.0 tag is correct. Check: GitHub Releases page for v1.0 tag and notes.

## 6. Optional advanced checks

- [ ] **RAG** — Agent can use RAG when wired. Check: Run example or pipeline; retrieval and answers work.
- [ ] **DAG edge cases** — Correct behavior with cycles and complex branches. Check: Build special DAG tests; execution order and checkpoint match.
- [ ] **Load test** — Many concurrent jobs run stably. Check: Run load script; observe success rate and latency.

---

When all items are checked, use together with [release-certification-1.0.md](release-certification-1.0.md) as v1.0 release acceptance evidence.
