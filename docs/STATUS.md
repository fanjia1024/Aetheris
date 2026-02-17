# Aetheris Status (Single Source of Truth)

> Last updated: 2026-02-17  
> Scope: repository status, release lane, and post-2.0 evolution policy.

## 1. Purpose

This file is the authoritative status entry for `main`.

- If other roadmap/release docs conflict with this file, use this file as final truth.
- Historical docs remain valuable for context, but not for current release decisions.

## 2. Status Model

Every capability must be labeled with one of these states:

- `prototype`: package/design exists, not integrated into main API/CLI flow.
- `integrated`: connected to runtime/API/CLI, but not yet release-gated.
- `production-ready`: integrated and covered by release gates + drills.

## 3. Current Snapshot

### 3.1 Runtime lane (2.x)

- Durable execution / event sourcing: `production-ready`
- Deterministic replay + at-most-once tool execution: `production-ready`
- Signal + human-in-the-loop (at-least-once delivery): `production-ready`
- Observability summary/stuck endpoints: `integrated` (UI/SRE workflows still strengthening)

### 3.2 Forensics and compliance lane (M1-M3)

- Evidence export/verify: `integrated` to `production-ready` (release gates exist)
- Forensics query/evidence graph APIs: `integrated` but currently experimental exposure policy
- RBAC/redaction/retention: `integrated` (operational hardening still required for broad multi-tenant rollout)

### 3.3 Enterprise lane (M4 / 3.0 candidates)

The following are currently treated as `prototype` unless explicitly promoted:

- `pkg/signature`
- `pkg/distributed`
- `pkg/ai_forensics`
- `pkg/monitoring`
- `pkg/compliance`

`prototype` means “technical reserve”, not “GA”.

## 4. Active Release Strategy

Current release lane: **Operational Runtime first (2.x)**.

Priority order:

1. Runtime correctness and recoverability
2. Multi-tenant safety and operational readiness
3. Release gates and runbooks
4. Selective 3.0 productization by vertical slices

## 5. 2.x Exit Gates (Must Pass)

A 2.x production release requires all of:

- CI green (`build`, `vet`, `test`, Postgres integration)
- Release checklist completed (`docs/release-checklist-2.0.md`)
- P0 performance gate report available
- P0 failure drill report available (including DB outage drill in release rehearsal)
- Upgrade/rollback runbook validated
- Security baseline checklist completed

If any gate is missing, release status stays `integrated`, not `production-ready`.

## 6. 3.0 Promotion Policy

A 3.0 capability may be promoted from `prototype` to `integrated` only when all are present:

- config: documented and wired in runtime config
- schema/storage: migration and backward compatibility notes
- API: routed and contract-documented
- CLI: command surface implemented
- tests: unit + integration + failure-path coverage
- ops: observability and runbook updates

No “docs-only complete” claims without these artifacts.

## 7. Doc Governance Rules

To avoid roadmap confusion:

- `docs/STATUS.md` is the single current-state source.
- Roadmap docs should focus on plan and history, not final state authority.
- “Complete / Ready” wording is allowed only when status is `production-ready`.

## 8. Next Focus (Recommended)

1. Harden 2.x gates in CI/release pipeline (make P0 gates non-optional for release jobs).
2. Close multi-tenant operational gaps (isolation tests, authz drills, runbook evidence).
3. Productize exactly one 3.0 slice first (recommended: signature flow end-to-end).

