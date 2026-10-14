---
id: "09-settle-orphaned-workloads"
title: "Settle workloads whose owning agent process is gone"
status: complete
wave: 8
depends_on:
  - "02-observations-and-recovery"
plan: "plan.md"
requirements:
  - REQ-AGENTS-BACKGROUND-WORK-002
acceptance_criteria:
  - AC-AGENTS-BACKGROUND-WORK-002.3
  - AC-AGENTS-BACKGROUND-WORK-002.5
system_design:
  - ../../specs/agents/system-design/background-work.md
---

# Task 09: Settle workloads whose owning agent process is gone

## Summary

Claude's detached shells, monitors and async subagents are recorded as
`running` and nothing ever moves them to a terminal state, because the ACP
adapter does not forward Claude's task completion to Kandev. Rows from
cancelled, completed and idle-suspended sessions stay `running` forever and
inflate the task's background-job count (issue #4459).

## In scope

- Task service `SettleOpenBackgroundWorkloads(sessionID)` moves every
  non-terminal workload of a session to `ended` with `finished_at` set and the
  revision bumped. Terminal rows are untouched; the existing revision guard keeps
  a late `running` replay from reopening a settled row.
- Repository `ListSessionIDsWithOpenBackgroundWork` (SQLite and PostgreSQL).
- The orchestrator settles and publishes `background_work.updated` when a session
  enters COMPLETED, FAILED or CANCELLED (including archive's bulk cancellation),
  after a successful idle suspension stops the ACP process, and at startup for
  every session with open work and no live agent execution.

## Out of scope

Forwarding Claude's own `task_notification` completion through the ACP
`asyncTasks` capability. That would give exact per-workload outcomes while the
agent is alive, and is a separate protocol change.

## Validation

- `go test ./internal/task/service -run TestSettle`
- `go test ./internal/task/repository/sqlite -run OpenBackgroundWork`
  (PostgreSQL variant with `KANDEV_TEST_POSTGRES_DSN`)
- `go test ./internal/orchestrator -run 'SettlesOpenBackgroundWork|KeepsBackgroundWorkOpen|StartupSettles|WithoutSettler|IdleSuspension|ArchiveCancellation'`
