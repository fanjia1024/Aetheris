# Aetheris Benchmark Suite

## Prerequisites

- k6 or vegeta installed
- Aetheris running (API + Worker + PostgreSQL)

## Scenarios

### 1. Simple Job Throughput

Single LLM call per job.

```bash
k6 run --vus=10 --duration=30s simple_job.lua
```

Expected: 10-20 jobs/minute per worker

### 2. Complex Job Latency

Multi-tool calls + reasoning.

```bash
k6 run --vus=5 --duration=60s complex_job.lua
```

Expected: 2-5 jobs/minute per worker

### 3. Long-Running Job (HITL)

Simulate park/resume cycles.

```bash
k6 run --vus=2 --duration=120s hitl_job.lua
```

Expected: 1-2 concurrent per worker

### 4. Multi-Worker Scaling

Test horizontal scaling with 1/2/4/8 workers.

```bash
./run_scaling_test.sh
```

## Results

Results are saved to `benchmark/results/` directory.

See `benchmark/results.md` for analysis.
