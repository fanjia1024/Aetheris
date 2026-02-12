# Aetheris 1.0 → 2.0 Improvement Checklist

This checklist supports **1.0 hardening** and **2.0 planning**. Each row has **Priority**, **Risk**, **Effort**, and **Module(s)** so you can align roadmap and backlog. It does not replace [design/aetheris-2.0-overview.md](../design/aetheris-2.0-overview.md) or [docs/next_plan.md](next_plan.md); it references them and provides a single actionable table.

**Effort legend**: S ≈ &lt;1 week, M ≈ 1–2 weeks, L ≈ 2+ weeks.

---

## Checklist table

| Area | Item | Priority | Risk | Effort | Module(s) |
|------|------|----------|------|--------|-----------|
| Core runtime & reliability | Step Contract static check or test harness — automatically verify Steps comply with contract (no direct HTTP, no `time.Now()`, no `rand`, side effects via Tools only) | P1 | Low – additive tooling | M | New tooling; [design/step-contract.md](../design/step-contract.md), [internal/agent/runtime/executor/](../internal/agent/runtime/executor/) |
| Core runtime & reliability | Default wrapper or helper so developers are less likely to break the Step Contract (e.g. time/random injection, tool-only side-effect helpers) | P1 | Low – additive | S | [internal/agent/runtime/](../internal/agent/runtime/), [design/step-contract.md](../design/step-contract.md) |
| Core runtime & reliability | Stress and multi-worker tests — more edge cases for crash/retry, concurrent workers, large job volume beyond the four fatal tests | P1 | Medium – touches Runner/Scheduler | M | [internal/agent/runtime/executor/ledger_1_0_test.go](../internal/agent/runtime/executor/ledger_1_0_test.go), [internal/agent/job/](../internal/agent/job/), [docs/release-checklist-v1.0.md](release-checklist-v1.0.md) (load test item) |
| Developer experience | Official templates and CLI for agent onboarding (e.g. `aetheris init`, scaffold TaskGraph + tools + config) | P1 | Low – additive | M | New: templates/ or docs; [cmd/aetheris/](../cmd/), [docs/getting-started-agents.md](getting-started-agents.md) |
| Developer experience | Trace UI improvements: interactive DAG view, step-level folding, event filters, state diff view | P2 | Low – UI only | M | [internal/api/http/trace_tree.go](../internal/api/http/trace_tree.go), Trace UI (see [design/execution-trace.md](../design/execution-trace.md)) |
| Developer experience | End-to-end business scenario doc — full flow from agent authoring to deployment on Aetheris (e.g. refund-approval or similar) | P1 | Low – docs only | S | New doc under [docs/](.); [docs/getting-started-agents.md](getting-started-agents.md), [docs/adapters/custom-agent.md](adapters/custom-agent.md) |
| Developer experience | Adapter ease: ship LangGraph adapter and/or improve custom adapter docs so onboarding is simpler | P1 | Low – additive | M | [docs/adapters/](adapters/), README “Adapters” section; new or extended adapter code |
| 2.0 feature expansion | RAG / knowledge pipeline integration — tighter integration and standardized tool pipelines (not just optional plug-in) | P2 | Medium – pipeline contracts | L | [design/aetheris-2.0-overview.md](../design/aetheris-2.0-overview.md) Phase 2; [internal/pipeline/](../internal/pipeline/), [internal/tool/](../internal/tool/) |
| 2.0 feature expansion | Multi-tenant / security isolation — isolation and access control for multi-agent or multi-team use | P2 | High – cross-cutting | L | [design/aetheris-2.0-overview.md](../design/aetheris-2.0-overview.md) Phase 4; Security & Governance, RBAC; new or [internal/api/](../internal/api/) |
| 2.0 feature expansion | Scalability: Worker scaling, JobStore performance, event stream compression and archival strategy | P2 | Medium – storage and scheduler | L | [internal/app/worker/](../internal/app/worker/), [internal/runtime/jobstore/](../internal/runtime/jobstore/), [design/aetheris-2.0-overview.md](../design/aetheris-2.0-overview.md) Performance and Stability |
| 2.0 feature expansion | Notifications and external triggers — webhook, queue, or event-driven workflow support (beyond current Signal) | P2 | Medium – API and Runner | M | [design/runtime-contract.md](../design/runtime-contract.md) (wait/signal); [internal/api/http/](../internal/api/http/), [internal/agent/job/](../internal/agent/job/) |
| Open-source & community | More official adapters (e.g. AutoGen, CrewAI) in addition to Custom and LangGraph | P2 | Low – additive | M | [docs/adapters/](adapters/); new adapter implementations |
| Open-source & community | Integration test coverage for agent run, tool invocation, and replay (beyond unit tests) | P1 | Low – additive tests | M | New or [internal/agent/runtime/executor/](../internal/agent/runtime/executor/), [docs/test-e2e.md](test-e2e.md) |
| Open-source & community | CI expansion — run integration tests (and optionally load tests) in CI | P1 | Low – CI config | S | CI config (e.g. `.github/workflows/` or equivalent); [docs/release-checklist-v1.0.md](release-checklist-v1.0.md) |

---

## How to use this checklist

Use this table during **backlog grooming** or **2.0 planning**: sort by Priority (P0 → P1 → P2) and Effort when capacity is limited. Prefer low-risk, additive items (e.g. Step Contract tooling, docs, adapters) for quick wins; schedule high-risk or cross-cutting items (e.g. multi-tenant) when you have design and testing bandwidth. Link each chosen item to the **Module(s)** column so implementation stays traceable to the codebase and design docs.

---

## References

- **2.0 modules and phases**: [design/aetheris-2.0-overview.md](../design/aetheris-2.0-overview.md)
- **Step Contract and runtime semantics**: [design/step-contract.md](../design/step-contract.md), [design/runtime-contract.md](../design/runtime-contract.md), [design/1.0-runtime-semantics.md](../design/1.0-runtime-semantics.md)
- **Eight-week 2.0 coding plan**: [docs/next_plan.md](next_plan.md)
- **1.0 release baseline**: [docs/release-checklist-v1.0.md](release-checklist-v1.0.md), [docs/release-certification-1.0.md](release-certification-1.0.md)
- **Critical tests**: [internal/agent/runtime/executor/ledger_1_0_test.go](../internal/agent/runtime/executor/ledger_1_0_test.go), [internal/agent/runtime/executor/runner_test.go](../internal/agent/runtime/executor/runner_test.go); e2e: [docs/test-e2e.md](test-e2e.md)
- **Adapters**: [docs/adapters/custom-agent.md](adapters/custom-agent.md)
