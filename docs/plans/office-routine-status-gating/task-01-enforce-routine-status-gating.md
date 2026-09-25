---
id: "01-enforce-routine-status-gating"
title: "Enforce routine status gating"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-OFFICE-ROUTINE-STATUS-001
  - REQ-OFFICE-ROUTINE-STATUS-002
  - REQ-OFFICE-ROUTINE-STATUS-003
  - REQ-OFFICE-ROUTINE-STATUS-004
  - REQ-OFFICE-ROUTINE-STATUS-006
acceptance_criteria:
  - AC-OFFICE-ROUTINE-STATUS-001.1
  - AC-OFFICE-ROUTINE-STATUS-001.2
  - AC-OFFICE-ROUTINE-STATUS-001.3
  - AC-OFFICE-ROUTINE-STATUS-001.4
  - AC-OFFICE-ROUTINE-STATUS-001.5
  - AC-OFFICE-ROUTINE-STATUS-001.6
  - AC-OFFICE-ROUTINE-STATUS-001.7
  - AC-OFFICE-ROUTINE-STATUS-001.8
  - AC-OFFICE-ROUTINE-STATUS-002.1
  - AC-OFFICE-ROUTINE-STATUS-002.2
  - AC-OFFICE-ROUTINE-STATUS-002.3
  - AC-OFFICE-ROUTINE-STATUS-002.4
  - AC-OFFICE-ROUTINE-STATUS-002.5
  - AC-OFFICE-ROUTINE-STATUS-002.6
  - AC-OFFICE-ROUTINE-STATUS-002.7
  - AC-OFFICE-ROUTINE-STATUS-002.8
  - AC-OFFICE-ROUTINE-STATUS-002.10
  - AC-OFFICE-ROUTINE-STATUS-003.1
  - AC-OFFICE-ROUTINE-STATUS-003.2
  - AC-OFFICE-ROUTINE-STATUS-003.3
  - AC-OFFICE-ROUTINE-STATUS-003.4
  - AC-OFFICE-ROUTINE-STATUS-003.5
  - AC-OFFICE-ROUTINE-STATUS-003.6
  - AC-OFFICE-ROUTINE-STATUS-004.1
  - AC-OFFICE-ROUTINE-STATUS-004.2
  - AC-OFFICE-ROUTINE-STATUS-004.3
  - AC-OFFICE-ROUTINE-STATUS-004.4
  - AC-OFFICE-ROUTINE-STATUS-004.5
  - AC-OFFICE-ROUTINE-STATUS-004.6
  - AC-OFFICE-ROUTINE-STATUS-004.7
  - AC-OFFICE-ROUTINE-STATUS-006.1
  - AC-OFFICE-ROUTINE-STATUS-006.2
  - AC-OFFICE-ROUTINE-STATUS-006.3
  - AC-OFFICE-ROUTINE-STATUS-006.4
  - AC-OFFICE-ROUTINE-STATUS-006.5
  - AC-OFFICE-ROUTINE-STATUS-006.6
system_design:
  - ../../specs/office/system-design/routine-status-gating.md
---

# Task 01: Enforce routine status gating

## Summary

Make routine status a load-bearing gate for cron, manual, and webhook fires.
Suppressed cron slots advance their cursor without writing fire evidence. The
routine list and detail views show the same firing rule as the backend.

## In scope

- Add the shared firing-status predicate and the compare-and-set cursor update.
- Read and decide routine status before claiming a cron trigger.
- Return a typed refusal for manual and webhook requests.
- Localize refusal status labels while preserving unknown values for diagnosis.
- Cover status transitions, cursor behavior, API responses, and UI surfaces.
- Keep the routine cron calculation on the shared `NextCronTime` helper.

## Out of scope

- A workspace-wide kill switch.
- Changes to routine storage schema or trigger ownership.
- Changes to the wakeup dispatcher concurrency policy.

## Verification

```bash
(cd apps/backend && go test ./internal/office/routines)
(cd apps && pnpm --filter @kandev/web test -- --run app/office/lib/routine-not-firing.test.ts)
python3 scripts/lint-spec-files.py --all
```

## Results

The routine service gates all fire paths on status. Suppressed cron cursors use
the shared cron helper and leave no run, wakeup, task, or fire timestamp. The
web client translates known refusal statuses and keeps unknown values visible.
The focused backend and frontend tests and specification lint pass.
