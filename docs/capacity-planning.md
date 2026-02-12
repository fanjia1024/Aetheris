# Aetheris 2.0 Capacity Planning Guide

## Single Worker Capacity

### Baseline Configuration
- **CPU**: 2 cores
- **Memory**: 4GB
- **PostgreSQL**: Dedicated or shared
- **Concurrent jobs**: 2-4 (configurable)

### Expected Throughput
- **Simple jobs** (single LLM call): ~10-20/minute
- **Complex jobs** (multi-tool + reasoning): ~2-5/minute
- **Long-running jobs** (HITL): 1-2 concurrent

## PostgreSQL Performance

### Event Store Bottleneck
- **Write QPS**: ~1000 events/sec (single table)
- **Read QPS**: ~5000 events/sec (with indexes)
- **Recommended**: Use connection pooling (pgbouncer)

### Scaling Strategies
1. **Read replicas**: For trace queries
2. **Partitioning**: By job_id or date range
3. **Sharding**: Multiple PostgreSQL instances (see sharded_store.go)

## Recommended Configurations

### Small Deployment (< 100 jobs/day)
- 1-2 Workers
- 1 PostgreSQL instance (shared or dedicated)
- Memory store for dev/testing

### Medium Deployment (100-1000 jobs/day)
- 4-8 Workers
- 1 PostgreSQL instance (dedicated, 4 cores)
- Enable snapshot/compaction
- Enable rate limiting

### Large Deployment (> 1000 jobs/day)
- 10+ Workers (with HPA)
- Sharded PostgreSQL (2-4 shards)
- Read replicas for trace queries
- Full observability stack (Prometheus + Jaeger)
- Enable all 2.0 features

## Monitoring Thresholds

- **Worker busy > 80%**: Scale horizontally
- **Queue backlog > 100**: Scale workers
- **PostgreSQL CPU > 70%**: Consider sharding
- **Event table size > 10GB**: Enable compaction
