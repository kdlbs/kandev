---
created: 2026-09-09
status: complete
requirements:
  - REQ-OFFICE-RUN-HISTORY-RETENTION-001
  - REQ-OFFICE-RUN-HISTORY-RETENTION-002
  - REQ-OFFICE-RUN-HISTORY-RETENTION-003
  - REQ-OFFICE-RUN-HISTORY-RETENTION-004
  - REQ-OFFICE-RUN-HISTORY-RETENTION-005
system_design:
  - ../../specs/office/system-design/run-history-retention.md
  - ../../specs/office/system-design/run-history-retention-operations.md
legacy_specs: []
---

# Implementation Plan: Office Run History Retention

## Overview

Office writes `office_routine_runs` and `run_events` rows on every routine firing and
run lifecycle transition, and nothing ever deletes them on a schedule. A `*/5 * * * *`
routine produces roughly 105,000 `office_routine_runs` rows a year; one reference
install had already accumulated 323 consecutive `coalesced` rows from a routine that
was doing nothing useful. This plan adds one scheduled sweep, on its own interval
(never the 5s Office tick), that bounds `office_routine_runs`, `runs`, and their
satellites (`run_events`, `office_run_route_attempts`, `office_run_skills`) by age,
with a per-owner floor, identical behavior on SQLite and PostgreSQL, and an operator
surface (Settings > System > Data & Logs) that reports policy, counts, previews, and
warnings before and while rows are deleted.

The two halves of the contract are split the same way the specs are split: the sweep
itself, its eligibility rules, and engine parity are
[run history retention](../../specs/office/system-design/run-history-retention.md)
(REQ-001, REQ-002, REQ-005); the settings record, preview marker, health warnings, and
System page surface are
[run history retention operations](../../specs/office/system-design/run-history-retention-operations.md)
(REQ-003, REQ-004).

## Scope

### In scope

- A `internal/office/retention` package: settings store, eligibility/count/delete
  queries shared by preview and delete, a session-scoped PostgreSQL advisory lock with
  a SQLite single-process equivalent, a scheduler goroutine on its own interval, and an
  HTTP handler for `GET`/`PUT /api/v1/system/retention`.
- Status-only classification of history vs. live state for both `office_routine_runs`
  and `runs`, a per-owner floor, oldest-first chunked batch deletion, and satellite rows
  deleted in the same transaction as their parent run.
- A per-table preview (report, delete nothing) on each table's first evaluation, and
  `health.Issue` warnings before the cap and on sweep/count failure.
- A `RetentionSettingsCard` on Settings > System > Data & Logs showing policy, retained
  counts, preview state, last sweep, and backlog/error warnings.
- Expression indexes serving the sweep's filter/order on both engines.

### Out of scope

- Filesystem/container cleanup (owned by storage maintenance).
- Routine or workspace deletion (already deletes runs structurally; unaffected).
- Any change to run lifecycle, routine dispatch, or task/session/checkout data.

## Tasks

- [x] [Task 01: Bound run history with a scheduled retention sweep](task-01-bound-run-history-with-retention-sweep.md)

## Follow-up presentation package

The [settings storage tabs package](../settings-storage-tabs/plan.md) owns the planned header tabs and maintenance presentation changes.
This package retains its historical implementation results. The follow-up owns new route, copy, and regression verification.
