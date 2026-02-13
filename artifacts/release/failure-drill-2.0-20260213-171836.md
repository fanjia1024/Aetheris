# Failure Drill Report (2.0)

- API URL: http://localhost:8080
- Generated at: 2026-02-13T09:18:46Z
- Passed: 3
- Failed: 1
- Skipped: 1

## Drill Results
- Drill A (worker crash recovery): PASS (terminal=completed)
- Drill B (api restart): PASS (terminal=completed)
- Drill C (postgres outage): SKIP (set RUN_DB_DRILL=1 to enable)
- Drill D (replay/trace): PASS (terminal=completed)
- Drill E (forensics): FAIL (terminal=completed export_ok=0 verify_ok=1)

## Verdict
- Gate passed: no
