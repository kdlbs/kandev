---
created: 2026-09-26
status: done
requirements:
  - REQ-OFFICE-SCHEDULER-001
system_design:
  - ../../specs/office/system-design/scheduler-01.md
legacy_specs: []
---

# Implementation Plan: Close legacy unlinked routine runs

## Overview

Office Beta ISSUE-1: on an upgraded install, a pre-#3488 `office_routine_runs`
row can sit in `task_created` with an empty `linked_task_id`. A lightweight
coordinator routine's fingerprint never changes between fires, so this fossil
row matches every future fire and permanently gates it under
`coalesce_if_active` / `skip_if_active` — the routine can never launch an
agent again. This plan records the delivery package for the fix: closing that
fossil row out the first time a dispatch finds it.

## Scope

- Treat a `task_created` run with an empty `linked_task_id` as inactive in
  `selfHealIfTaskTerminal`, closing it out as `failed` instead of returning it
  unchanged.
- Leave `GetActiveRunForFingerprint`, the retention policy, and heavy-run
  gating (a `task_created` row with a live linked task) unchanged.

## Work orders

- [Task 01: Close legacy unlinked routine runs](task-01-close-legacy-unlinked-routine-runs.md)

## Verification

```bash
cd apps/backend
go test ./internal/office/routines/ -run TestBetaLegacyLightweightGate -count=1
go test ./internal/office/routines/... ./internal/office/retention/... ./internal/backendapp -run 'Routine|Retention' -count=1
go test ./internal/office/routines/... ./internal/office/retention/... -count=1
cd ../..
python3 scripts/lint-spec-files.py --all
python3 scripts/list-docs.py validate
```
