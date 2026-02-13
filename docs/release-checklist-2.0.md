# Release Checklist 2.0

This checklist is for shipping Aetheris 2.0 changes (runtime + forensics + CLI/DevOps).

## 1. Automated checks

Run from repository root:

```bash
./scripts/release-2.0.sh
```

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

