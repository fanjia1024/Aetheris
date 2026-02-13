# Failure Drill Report (2.0)

- API URL: http://localhost:8080
- Generated at: 2026-02-13T09:13:00Z
- Passed: 1
- Failed: 3
- Skipped: 1

## Drill Results
- Drill A (worker crash recovery): FAIL (terminal=timeout)
- Drill B (api restart): PASS (terminal=completed)
- Drill C (postgres outage): SKIP (set RUN_DB_DRILL=1 to enable)
- Drill D (replay/trace): FAIL (create job failed code=)
- Drill E (forensics): FAIL (create job failed code=)

## Verdict
- Gate passed: no
