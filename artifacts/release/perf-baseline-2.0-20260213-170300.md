# Performance Baseline Report (2.0)

- API URL: http://localhost:8080
- Samples: 3
- Elapsed seconds: 112
- Generated at: 2026-02-13T09:04:52Z

## Result Summary

- Completed: 2
- Failed: 0
- Cancelled: 0
- Timeout: 1
- Request failure: 0
- Completion ratio: 0.67
- Throughput (completed jobs/min): 1.07

## Latency (ms)

- POST /api/agents/:id/message P95: 5198
- GET /api/jobs/:id P95: 5
- GET /api/jobs/:id/events P95: 5

## Gate Thresholds

- POST message P95 <= 500
- GET job P95 <= 200
- GET events P95 <= 500
- Throughput >= 10 jobs/min
- Completion ratio >= 0.95

## Gate Verdict

- Gate passed: no
