# Failure Drill Report (2.0)

- API URL: http://localhost:8080
- Generated at: 2026-02-13T09:20:58Z
- Passed: 4
- Failed: 0
- Skipped: 1

## Drill Results
- Drill A (worker crash recovery): PASS (terminal=completed)
- Drill B (api restart): PASS (terminal=completed)
- Drill C (postgres outage): SKIP (set RUN_DB_DRILL=1 to enable)
- Drill D (replay/trace): PASS (terminal=completed)
- Drill E (forensics): PASS (terminal=completed)

## Verdict
- Gate passed: yes
