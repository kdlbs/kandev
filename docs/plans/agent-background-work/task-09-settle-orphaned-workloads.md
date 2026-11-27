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

- Task service `SettleOpenBackgroundWorkloads(sessionID)` atomically moves open
  workloads and runs to `ended`, sets finish times, and increments workload
  revisions. Failed writes roll back both projections and remain retryable.
  Terminal outcomes survive late non-terminal observations; late terminal
  provider observations can supply the actual result.
- Repository `ListSessionIDsWithOpenBackgroundWork` (SQLite and PostgreSQL)
  includes open runs under terminal parents to repair older partial writes.
- The orchestrator settles and publishes `background_work.updated` when a session
  enters COMPLETED, FAILED or CANCELLED (including archive's bulk cancellation),
  after successful teardown or idle suspension, and at startup, provided a runtime
  liveness probe confirms the process is gone. Failed stops and failed probes
  preserve open work. Workflow terminal-state ordering remains unchanged.

## Out of scope

Forwarding Claude's own `task_notification` completion through the ACP
`asyncTasks` capability. That would give exact per-workload outcomes while the
agent is alive, and is a separate protocol change.

## Validation

- `go test ./internal/task/service -run TestSettle`
- `go test ./internal/task/repository/sqlite -run OpenBackgroundWork`
  (PostgreSQL variant with `KANDEV_TEST_POSTGRES_DSN`)
- `go test ./internal/orchestrator -run 'SettlesOpenBackgroundWork|KeepsBackgroundWorkOpen|StartupSettles|WithoutSettler|IdleSuspension|ArchiveCancellation'`

## Review remediation validation

- Controlled exact-function proofs reproduced retry loss, reopened runs, and
  settlement on live or unknown processes. Repository and runtime boundaries in
  these proofs are stubbed; they are not SQLite, PostgreSQL, or lifecycle
  integration acceptance.
- Permanent service regressions cover real SQLite and PostgreSQL rollback on
  run-write failure, successful retry publication, legacy partial-write repair,
  and late run observations with terminal outcomes preserved.
- PostgreSQL multi-connection coverage verifies the delayed-run advisory lock.
- The production PostgreSQL boot integration test exercises rollback, retry,
  stale output, late outcomes and legacy repair through the configured task
  service. Both existing PostgreSQL 16 and 18 hosted gates select this test.
- Lifecycle regressions cover live/unknown probes, failed workflow stops, and
  successful stop after the terminal session transition.
- Hosted verification results are recorded with the closing commit in the PR.

Cleanup review also covers retained remote recovery: a successful generic
cleanup result is not proof that its process stopped. Settlement after actual
completion or synchronous stop matches the stopped execution against the durable
owner, preserving an uncertain successor after a delayed predecessor callback.
Source-adjacent regressions cover both boundaries; the controlled proof uses
explicit repository/runtime stubs and does not substitute for hosted integration.
