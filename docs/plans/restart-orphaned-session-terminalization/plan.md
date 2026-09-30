---
created: 2026-09-20
status: implemented
requirements:
  - REQ-TASKS-RESTART-ORPHAN-SESSIONS-001
system_design:
  - ../../specs/tasks/system-design/restart-orphaned-session-terminalization.md
legacy_specs: []
---

# Implementation Plan: Restart-Orphaned Session Terminalization

## Overview

A backend restart while an ACP prompt turn was open left `task_sessions`
stuck in `STARTING`/`RUNNING` forever on unarchived tasks: the actor that
would have transitioned them died with the process, and the existing 1-minute
reconciliation sweep only covered archived tasks. Extend that same sweep with
a second pass that terminalizes stale `STARTING`/`RUNNING` sessions of
unarchived tasks when no live in-memory execution backs them, after a grace
window of one launch budget so an in-flight launch is never reaped.

## Scope

### In scope

- A second pass in the existing reconciliation loop covering unarchived
  tasks' stale `STARTING`/`RUNNING` sessions.
- Candidate cutoff derived from `constants.AgentLaunchTimeout` plus a 5-minute
  allowance.
- An execution-liveness seam (`TaskExecutionLivenessChecker` /
  `HasLiveExecution`) answered from the in-memory store only.
- Per-session cancellation (`CancelRunningTaskSessionByID`) with a dedicated
  `"orphaned by backend restart"` reason, distinct from archive reasons.
- Shared post-cancellation effects extracted from the archive path
  (`notifyCancelledSessions`).
- Service-level regression tests: unbacked sessions terminalized, live-backed
  and fresh rows untouched, healthy siblings survive, inert without the
  liveness seam.

### Out of scope

- Turn resume or transcript replay; drain-on-shutdown; agent-side heartbeats;
  manual SQL recovery; archived tasks (existing pass owns them).

## Technical approach

Reuse the archived pass's shape verbatim: same ticker, same continuation
context, same retry-forever semantics. The candidate query filters in SQL so
fresh rows are never loaded; the liveness re-check happens at write time so a
launch racing the grace window is never reaped. Cancellation is session-scoped
(`UPDATE ... RETURNING` on rows still in `STARTING`/`RUNNING`) so healthy
sibling sessions survive. The `orphanedSessionRepository` capability keeps the
two new repository methods off the `SessionRepository` interface and its test
doubles.

## Tests

- `TestService_OrphanedSessionReconciliationTerminalizesUnbackedSessions`
  (`apps/backend/internal/task/service/orphaned_session_reconciliation_test.go`):
  covers AC-TASKS-RESTART-ORPHAN-SESSIONS-001.1, .2, .3, .4, .5, .8, and .10 —
  stale unbacked `RUNNING`/`STARTING` sessions reach `CANCELLED` with the
  orphan reason and a `session.state_changed` publish whose `is_primary`
  matches the durable flag; live-backed, fresh, archived-task, and
  `WAITING_FOR_INPUT` sibling rows stay untouched.
- `TestService_OrphanedSessionReconciliationRequiresLivenessChecker`: covers
  AC-TASKS-RESTART-ORPHAN-SESSIONS-001.6 — without the liveness seam the sweep
  is inert.
- `TestService_OrphanedSessionReconciliationSparesRowRefreshedSinceCandidateRead`:
  covers AC-TASKS-RESTART-ORPHAN-SESSIONS-001.9 — a row refreshed between the
  candidate read and the cancel write (the in-flight-launch race) is spared.
- `TestSessionOrphanedCancelReasonIsNotArchiveReason`: covers
  AC-TASKS-RESTART-ORPHAN-SESSIONS-001.4's distinctness invariant.

## E2E tests

None: the sweep is a periodic backend behavior with a one-minute tick and a
launch-budget grace window; the service-level test pins the full transition.
A repro (start session, kill backend mid-turn, restart) is recorded in
issue #3711's verification plan.

## Work orders

- [x] [Task 01: Terminalize restart-orphaned sessions via the reconciliation sweep](task-01-terminalize-restart-orphaned-sessions.md)

## Verification results

- `(cd apps/backend && go test ./internal/task/service -run 'TestService_OrphanedSessionReconciliation|TestService_ArchivedSessionReconciliation' -count=1)` — pass.
- `(cd apps/backend && go test ./internal/task/repository/... -count=1)` — pass.
- `(cd apps/backend && go run ./cmd/sqlguard ./internal)` — clean after rebinding the candidate query's placeholder.
- `(cd apps/backend && gofmt -l internal/task internal/backendapp)` — no output; `go vet` clean.
- `python3 scripts/lint-spec-files.py --all` and `python3 scripts/list-docs.py validate` — pass.
- Full `go test ./internal/task/...`: three pre-existing failures
  (`TestDesktopDiscoveryRootPersists…`, `TestArchiveTaskCleanup…`,
  `TestTaskLifecycleCleanup_MissingWorktree`) reproduce identically on the
  base commit in a scratch worktree, so none is attributable to this change.

## Risks

- Reaping a live session whose execution legitimately left the store would
  lose work; the liveness re-check at write time and the fresh-row grace
  window bound that risk to zero in the steady state.
- The sweep cannot distinguish "execution gone because restart" from
  "execution gone because of a runtime bookkeeping bug"; both are terminal
  sessions by the same argument — nothing will transition them — so
  terminalizing is correct in either case.
