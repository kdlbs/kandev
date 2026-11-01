---
created: 2026-10-27
status: complete
requirements:
  - REQ-TASKS-RUN-SCHEDULING-001
system_design:
  - ../../specs/tasks/system-design/run-scheduling.md
legacy_specs: []
---

# Implementation Plan: PostgreSQL Office Recovery Query

## Goal

Keep the Office unstarted-task recovery query correct on SQLite and PostgreSQL.
Preserve its existing Office-task scope, UTC lookback, and duplicate-run policy.

## Scope

- Use the shared SQL dialect helpers for date arithmetic and JSON extraction.
- Interpret PostgreSQL's naive UTC `created_at` values as UTC, independent of
  the connection time zone.
- Keep queued, claimed, and finished runs as duplicate blockers. Keep failed and
  cancelled runs eligible for recovery.
- Cover dialect and run-status behavior in the PostgreSQL repository tests.

## Work orders

- [x] [Task 01: Use dialect-safe recovery query expressions](task-01-postgres-recovery-query.md)

## Results

The recovery query now uses the selected database dialect for time and JSON
operations. PostgreSQL compares the naive UTC `created_at` value independently
of the session time zone. The query preserves the existing duplicate-run policy.

Local SQL guard, documentation coverage, specification validation, and spec lint
passed. The focused PostgreSQL repository test compiled and skipped because
`KANDEV_TEST_POSTGRES_DSN` is not configured in this workspace.
