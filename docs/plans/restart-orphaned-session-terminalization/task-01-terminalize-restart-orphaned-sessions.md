---
id: "01-terminalize-restart-orphaned-sessions"
title: "Terminalize restart-orphaned sessions via the reconciliation sweep"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-RESTART-ORPHAN-SESSIONS-001
acceptance_criteria:
  - AC-TASKS-RESTART-ORPHAN-SESSIONS-001.1
  - AC-TASKS-RESTART-ORPHAN-SESSIONS-001.2
  - AC-TASKS-RESTART-ORPHAN-SESSIONS-001.3
  - AC-TASKS-RESTART-ORPHAN-SESSIONS-001.4
  - AC-TASKS-RESTART-ORPHAN-SESSIONS-001.5
  - AC-TASKS-RESTART-ORPHAN-SESSIONS-001.6
  - AC-TASKS-RESTART-ORPHAN-SESSIONS-001.7
  - AC-TASKS-RESTART-ORPHAN-SESSIONS-001.8
  - AC-TASKS-RESTART-ORPHAN-SESSIONS-001.9
  - AC-TASKS-RESTART-ORPHAN-SESSIONS-001.10
system_design:
  - ../../specs/tasks/system-design/restart-orphaned-session-terminalization.md
---

# Task 01: Terminalize Restart-Orphaned Sessions via the Reconciliation Sweep

## Summary

Extend the existing 1-minute task-session reconciliation sweep with a second
pass: terminalize stale `STARTING`/`RUNNING` sessions of unarchived tasks that
no live in-memory execution backs, so a backend restart mid-turn no longer
leaves sessions stuck RUNNING forever (#3711).

## In scope

- `runOrphanedSessionReconciliation` / `reconcileOrphanedSessions` in the
  reconciliation loop file, gated on a wired `TaskExecutionLivenessChecker`.
- Repository methods `ListStaleRunningSessionsOnUnarchivedTasks` and
  `CancelRunningTaskSessionByID` behind the `orphanedSessionRepository`
  capability.
- `SessionOrphanedCancelReason` distinct from archive cancel reasons.
- Extract `notifyCancelledSessions` from `finalizeCancelledSessions` so the
  orphan pass reuses the archive path's post-cancellation effects.
- Wire `HasLiveExecution` in the lifecycle adapter and the orchestrator.

## Out of scope

- Archived tasks' sessions (the archived pass owns them).
- Turn resume, drain-on-shutdown, heartbeats, adapter changes, schema
  changes.

## Acceptance

- After a backend restart mid-turn, an unbacked stale `STARTING`/`RUNNING`
  session reaches `CANCELLED` on the first sweep after the grace window, with
  the orphan reason and a `session.state_changed` publish, while live-backed,
  fresh, and archived-task rows stay untouched and healthy siblings survive.
- Without the liveness seam the sweep does nothing.

## Verification

```bash
(cd apps/backend && go test ./internal/task/service -run 'TestService_OrphanedSessionReconciliation|TestService_ArchivedSessionReconciliation' -count=1 -v)
(cd apps/backend && go test ./internal/task/repository/... -count=1)
(cd apps/backend && go run ./cmd/sqlguard ./internal)
(cd apps/backend && gofmt -l internal/task internal/backendapp)
(cd apps/backend && go vet ./internal/task/... ./internal/backendapp)
python3 scripts/lint-spec-files.py --all
python3 scripts/list-docs.py validate
```

## Files likely touched

- `apps/backend/internal/task/service/archived_session_reconciliation.go`
- `apps/backend/internal/task/service/orphaned_session_reconciliation_test.go`
- `apps/backend/internal/task/service/service.go`
- `apps/backend/internal/task/service/service_tasks.go`
- `apps/backend/internal/task/models/resume_safety.go`
- `apps/backend/internal/task/repository/sqlite/session.go`
- `apps/backend/internal/backendapp/adapters.go`
- `apps/backend/internal/backendapp/orchestrator.go`

## Dependencies

None.

## Risks

- The liveness checker must answer from the in-memory store only — a lazy
  creator would mask the sweep's dead signal.

## Parallelism

`sequential`

## Inputs

- `docs/specs/tasks/requirements/restart-orphaned-session-terminalization.md`
- `docs/specs/tasks/system-design/restart-orphaned-session-terminalization.md`
- `apps/backend/internal/task/service/archived_session_reconciliation.go`

## Results

Implemented in commit `aa7dc293b` (8 files, +458/−32): the second sweep pass,
the two repository methods behind the `orphanedSessionRepository` capability,
the orphan reason constant, the shared `notifyCancelledSessions` extraction,
and the lifecycle-adapter `HasLiveExecution` wiring. Follow-up commits added
the `Rebind` the SQL-portability gate requires, this specification package,
and two review-fix hardenings: the cancel statement re-asserts the staleness
cutoff (`updated_at < staleBefore`) so an in-flight launch refreshing its row
between the sweep's liveness check and the write is never reaped
(AC-.9, `TestService_OrphanedSessionReconciliationSparesRowRefreshedSinceCandidateRead`),
and both cancellation RETURNING clauses now select `is_primary` so the
published event carries the durable primary flag (AC-.10; this also repairs
the pre-existing archive path, which had the same gap). Verification commands
in the plan all pass; the failing packages in the full suite reproduce
identically on the base commit and are environmental.
