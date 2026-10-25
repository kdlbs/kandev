---
id: "01-close-legacy-unlinked-routine-runs"
title: "Close legacy unlinked routine runs"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-OFFICE-SCHEDULER-001
acceptance_criteria:
  - AC-OFFICE-SCHEDULER-001.13
system_design:
  - ../../specs/office/system-design/scheduler-01.md
---

# Task 01: Close legacy unlinked routine runs

## Summary

`selfHealIfTaskTerminal` (`apps/backend/internal/office/routines/service.go`)
returned an active run unchanged whenever its `LinkedTaskID` was empty,
treating a taskless run as permanently live. Current code never writes a
`task_created` row with an empty link (`materialiseHeavyRoutineRun` always
sets both together in one write), so a row in that shape can only be a
pre-#3488 fossil. Its dispatch fingerprint for a lightweight routine never
changes, so the fossil matched every future fire and
`coalesce_if_active` / `skip_if_active` gated the routine forever.

## In scope

- Regression test (`legacy_gate_test.go`) seeding a fossil `task_created` /
  empty-link row and firing a lightweight routine three times under each
  concurrency policy, plus a control confirming a live heavy run still gates
  and a fresh `received` row is untouched.
- `selfHealIfTaskTerminal`: close an empty-linked active run via the existing
  `closeOutRun(ctx, active, "missing")` (maps to `failed`, guarded by
  `UpdateRunStatusIfTaskCreated`) and return `nil` so the caller re-queries.
  On close-out failure, log and return the run unchanged (fail closed).

## Out of scope

- `GetActiveRunForFingerprint` SQL and its partial index.
- The retention policy and `retention/sweep_lookup_integrity_test.go`, which
  pins the repository lookup returning an unlinked `task_created` row
  unfiltered (AC-OFFICE-RUN-HISTORY-RETENTION-005.3).
- Trigger reconciliation, migrations, and any live cron arming.
- Frontend — no new UI; existing run-history views already render `done` and
  `failed`.

## Verification

```bash
cd apps/backend
go test ./internal/office/routines/ -run TestBetaLegacyLightweightGate -v -count=1
go test ./internal/office/routines/... ./internal/office/retention/... ./internal/backendapp -run 'Routine|Retention' -count=1
go test ./internal/office/routines/... ./internal/office/retention/... -count=1
golangci-lint run ./internal/office/routines/... --timeout=5m
```

## Results

`TestBetaLegacyLightweightGate` fails on `main` for the root-caused reason
(every fire under `coalesce_if_active`/`skip_if_active` was `coalesced` or
`skipped`, never `done`) and passes after the fix. The fossil row closes to
`failed` with `completed_at` set on first dispatch, persists across a
repository reopen against the same DB file, and no subsequent run coalesces
into it. A genuinely active heavy run (linked, live task) still gates its
fingerprint under both policies. `always_create` and a fresh `received` row
are unaffected controls. Full `routines`, `retention`, and `backendapp`
Routine/Retention suites pass; `golangci-lint` reports 0 issues on the
touched package.
