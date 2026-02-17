# Aetheris Production Deployment Guide

## Prerequisites

- Go 1.21+
- PostgreSQL 15+
- Docker & Docker Compose (optional)
- Prometheus & Grafana (monitoring)

---

## Quick Start (Docker Compose)

```bash
# 1. Clone repository
git clone https://github.com/yourusername/aetheris
cd aetheris

# 2. Start services
docker-compose -f deployments/docker-compose/docker-compose.yml up -d

# 3. Initialize database
docker exec aetheris-pg psql -U aetheris -d aetheris -f /schema.sql

# 4. Verify
curl http://localhost:8080/api/health
# {"status": "ok"}
```

---

## Production Deployment (Kubernetes + Helm)

### Install

```bash
# Add Helm repo (when available)
helm repo add aetheris https://charts.aetheris.dev

# Install
helm install aetheris aetheris/aetheris \
  --set postgresql.enabled=true \
  --set replicaCount.worker=3 \
  --set prometheus.enabled=true

# Verify
kubectl get pods -l app=aetheris
```

---

## Configuration

### Minimal Production Config

```yaml
# configs/api.yaml (production)
api:
  port: 8080
  
jobstore:
  type: postgres
  dsn: "postgres://aetheris:password@postgres:5432/aetheris?sslmode=require"
  lease_duration: "30s"

# 2.0: Rate limiting (required for production)
rate_limits:
  tools:
    _default:
      qps: 100
      max_concurrent: 10

# 2.0: Basic tenant isolation
auth:
  enable: true
  mode: jwt
  multi_tenant: true

# Monitoring
monitoring:
  tracing:
    enable: false  # Enable when needed
```

---

## Operations

### Health Check

```bash
curl http://api:8080/api/health
```

### Metrics

```bash
curl http://api:9092/metrics
```

### Logs

```bash
# API logs
kubectl logs -f deployment/aetheris-api

# Worker logs
kubectl logs -f deployment/aetheris-worker
```

---

## Monitoring

### Prometheus Targets

- API metrics: `http://aetheris-api:9092/metrics`
- Worker metrics: `http://aetheris-worker:9092/metrics`

### Key Metrics

- `aetheris_job_duration_seconds` - Job 执行耗时
- `aetheris_job_total` - Job 总数
- `aetheris_worker_busy` - Worker 繁忙度
- `aetheris_rate_limit_wait_seconds` - 限流等待时间

### Alerts

Import alerts from `deployments/prometheus/alerts.yml`

---

## Troubleshooting

### Worker not claiming jobs

**Check**:
1. JobStore connection
2. Lease duration configuration
3. Job status (not terminal)

### High latency

**Check**:
1. PostgreSQL performance
2. Rate limiting configuration
3. Network latency

### Memory growth

**Enable**:
- Event compaction
- Storage GC
- Job quota

---

## Backup & Recovery

### Database Backup

```bash
pg_dump -h postgres -U aetheris aetheris > backup.sql
```

### Restore

```bash
psql -h postgres -U aetheris aetheris < backup.sql
```

---

## Scaling

### Horizontal Scaling

```bash
# Scale workers
kubectl scale deployment aetheris-worker --replicas=10

# Or with HPA
kubectl autoscale deployment aetheris-worker \
  --min=2 --max=20 --cpu-percent=70
```

### Database Scaling

- Read replicas for trace queries
- Connection pooling (pgbouncer)
- Partitioning (by date or job_id)

---

**Status**: Production deployment guide complete
