# Performance Baseline Report (2.0)

- API URL: http://localhost:8080
- Samples: 3
- Elapsed seconds: 7
- Generated at: 2026-02-13T09:20:51Z

## Result Summary

- Completed: 3
- Failed: 0
- Cancelled: 0
- Timeout: 0
- Request failure: 0
- Completion ratio: 1.00
- Throughput (completed jobs/min): 25.71

## Latency (ms)

- POST /api/agents/:id/message P95: 9
- GET /api/jobs/:id P95: 3
- GET /api/jobs/:id/events P95: 4

## Gate Thresholds

- POST message P95 <= 500
- GET job P95 <= 200
- GET events P95 <= 500
- Throughput >= 10 jobs/min
- Completion ratio >= 0.95

## Gate Verdict

- Gate passed: yes
