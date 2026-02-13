# Performance Baseline Report (2.0)

- API URL: http://localhost:8080
- Samples: 20
- Elapsed seconds: 2
- Generated at: 2026-02-13T08:04:58Z

## Result Summary

- Completed: 0
- Failed: 0
- Cancelled: 0
- Timeout: 0
- Request failure: 20
- Completion ratio: 0.00
- Throughput (completed jobs/min): 0.00

## Latency (ms)

- POST /api/agents/:id/message P95: 155
- GET /api/jobs/:id P95: 0
- GET /api/jobs/:id/events P95: 0

## Gate Thresholds

- POST message P95 <= 500
- GET job P95 <= 200
- GET events P95 <= 500
- Throughput >= 10 jobs/min
- Completion ratio >= 0.95

## Gate Verdict

- Gate passed: no
