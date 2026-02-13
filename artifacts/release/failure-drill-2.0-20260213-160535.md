# Failure Drill Report (2.0)

- API URL: http://localhost:8080
- Generated at: 2026-02-13T08:05:35Z
- Passed: 0
- Failed: 4
- Skipped: 1

## Drill Results
- Drill A (worker crash recovery): FAIL (create job failed)
- Drill B (api restart): FAIL (create job failed)
- Drill C (postgres outage): SKIP (set RUN_DB_DRILL=1 to enable)
- Drill D (replay/trace): FAIL (create job failed)
- Drill E (forensics): FAIL (create job failed)

## Verdict
- Gate passed: no
