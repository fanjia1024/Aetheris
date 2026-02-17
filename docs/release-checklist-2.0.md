# Release Checklist 2.0

This checklist is for shipping Aetheris 2.0 changes (runtime + forensics + CLI/DevOps).

## 1. Automated checks

Run from repository root:

```bash
./scripts/release-2.0.sh
```

CI equivalent:

- `.github/workflows/release-gates.yml` runs the same gate path on `main/master` pushes and `v*` tags.
- Gate artifacts are uploaded from `artifacts/release/`.

Expected:
- `gofmt` clean
- `go vet` clean
- `go test ./...` pass
- `go build ./...` pass

## 2. Runtime smoke checks

### 2.1 Local 2.0 stack

```bash
./scripts/local-2.0-stack.sh start
./scripts/local-2.0-stack.sh health
./scripts/local-2.0-stack.sh status
```

### 2.2 Core API flow

1. Create agent
2. Post a message
3. Confirm job transitions to completed/failed deterministically

### 2.3 Forensics flow

1. Export evidence package
2. Verify evidence package offline
3. Call consistency API endpoint

## 3. CLI checks

Run and verify output shape:

```bash
aetheris monitor
aetheris migrate m1-sql
aetheris replay <job_id>
aetheris export <job_id> --output evidence.zip
aetheris verify evidence.zip
```

## 4. Deployment checks

- Compose: `deployments/compose/docker-compose.yml` starts healthy
- CI workflow green (`.github/workflows/ci.yml`)
- Postgres integration job green

## 5. Manual sign-off

- [ ] Runtime guarantees docs are up-to-date
- [ ] Migration docs match actual CLI behavior
- [ ] Roadmap progress table updated
- [ ] Release notes updated (if publishing)

## 6. P0 docs and readiness gates

- [ ] Upgrade + rollback runbook completed (`docs/upgrade-1.x-to-2.0.md`)
- [ ] API contract includes stable/experimental boundary (`docs/api-contract.md`)
- [ ] Performance baseline report attached (`docs/performance-baseline-2.0.md`)
- [ ] Failure drills executed and recorded (`docs/runbook-failure-drills.md`)
- [ ] Security baseline checks completed (`docs/security.md`)

### 6.1 Execute P0 performance gate

```bash
./scripts/release-p0-perf.sh
```

Artifact:
- `artifacts/release/perf-baseline-2.0-*.md`

### 6.2 Execute P0 failure drills

```bash
./scripts/release-p0-drill.sh
```

Optional DB outage drill:

```bash
RUN_DB_DRILL=1 ./scripts/release-p0-drill.sh
```

Artifact:
- `artifacts/release/failure-drill-2.0-*.md`

### 6.3 Run all gates in one command

```bash
RUN_P0_PERF=1 RUN_P0_DRILLS=1 ./scripts/release-2.0.sh
```

### 6.4 Execute tenant regression gate

```bash
./scripts/release-tenant-regression.sh
```

Artifact:
- `artifacts/release/tenant-regression-2.0-*.md`

## 7. Failure triage (when release gate fails)

1. Check workflow artifact bundle (`artifacts/release/*`) first:
   - `perf-baseline-2.0-*.md`
   - `failure-drill-2.0-*.md`
   - `tenant-regression-2.0-*.md`
2. If perf gate failed:
   - verify API health latency and completion ratio in report
   - confirm local stack resources and DB health
3. If drill gate failed:
   - identify failed drill item (A/B/C/D/E) from report
   - replay the same drill locally with `scripts/release-p0-drill.sh`
4. If DB outage drill failed:
   - check compose recovery order and Postgres health transitions
5. If tenant regression gate failed:
   - run `./scripts/release-tenant-regression.sh` locally and identify failing suite (`internal/api/http` or `pkg/auth`)
   - inspect role assignment, permission check, and tenant-boundary test assertions
6. Re-run full gate only after root cause is fixed and documented.
