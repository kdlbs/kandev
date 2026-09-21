---
id: "01-preserve-idle-conversations"
title: "Preserve resumable conversations during reconciliation"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-SESSION-STALL-VISIBILITY-001
  - REQ-TASKS-RESTART-ORPHAN-SESSIONS-002
acceptance_criteria:
  - AC-TASKS-SESSION-STALL-VISIBILITY-001.1
  - AC-TASKS-SESSION-STALL-VISIBILITY-001.2
  - AC-TASKS-SESSION-STALL-VISIBILITY-001.3
  - AC-TASKS-SESSION-STALL-VISIBILITY-001.4
  - AC-TASKS-SESSION-STALL-VISIBILITY-001.5
  - AC-TASKS-SESSION-STALL-VISIBILITY-001.6
  - AC-TASKS-SESSION-STALL-VISIBILITY-001.7
  - AC-TASKS-RESTART-ORPHAN-SESSIONS-002.1
  - AC-TASKS-RESTART-ORPHAN-SESSIONS-002.4
  - AC-TASKS-RESTART-ORPHAN-SESSIONS-002.5
  - AC-TASKS-RESTART-ORPHAN-SESSIONS-002.6
  - AC-TASKS-RESTART-ORPHAN-SESSIONS-002.7
system_design:
  - ../../specs/tasks/system-design/session-stall-visibility.md
  - ../../specs/tasks/system-design/restart-orphaned-session-terminalization.md
---

# Task 01: Preserve resumable conversations during reconciliation

## Summary

Keep idle conversations untouched. Reconcile interrupted running conversations
to recoverable waiting state in both sweeps. Preserve abandoned-turn settlement
without cancellation, prompt replay, or workflow advancement.

## In scope

- Add `TestService_ActiveSessionSweepPreservesIdleWaitingSession` before implementation.
  Reuse `newSweepFixture` with five hours of silence. Set `WAITING_FOR_INPUT`
  and no active turn. Expect unchanged state and no `task.stalled` event.
  Current code cancels this fixture, matching the recorded incident.
- Add `TestService_OrphanedSessionReconciliationPreservesInterruptedConversation`
  and `TestService_ActiveSessionSweepRecoversInterruptedConversation` before changes.
  Seed old STARTING/RUNNING sessions without live executions. Expect recoverable
  waiting state and preserved identity. Current code cancels these sessions.
- Cover waiting sessions with an unfinished turn, missing/retained executor rows,
  active-turn read failure, and ordinary `RUNNING`/`STARTING` candidates.
- Cover an idle waiting sibling plus a genuine stale running sibling. Detection
  names only the running sibling; task-wide healing remains blocked.
- Cover a turn completing or being replaced between classification and cancellation.
  Carry observed identity to the write and retain existing SQL predicates.
- Replace orphan cancellation calls in both sweeps with conditional recoverable
  settlement. Reuse `reconcileActiveSessionOnStartup` semantics and existing
  abandoned-turn cleanup without importing orchestrator into task service.
- Preserve questions, deferred work, task ownership, and runtime resume identity.
  Release only stale execution reservations through their current owner.
- Update root AGENTS.md stall guidance and historical orphan-reason comments.

## Out of scope

Task-open launch behavior, thresholds, explicit-stop/archive policy, and new database columns.

## Acceptance

1. Idle waiting sessions produce neither stall events nor orphan cancellations.
2. Unknown turn state skips evaluation. Mixed-session and racing-turn cases preserve healthy work.
3. Interrupted work becomes resumable waiting state in both passes. It never
   becomes CANCELLED because of execution loss, and settlement never advances workflow.

## Verification

From the repository root:

```bash
(cd apps/backend && go test ./internal/task/service -run 'TestService_ActiveSessionSweep|TestService_OrphanedSessionReconciliation' -count=1)
(cd apps/backend && go test ./internal/task/repository/sqlite -run 'Test.*(CancelActiveTaskSessionsByCandidates|RecoverInterrupted)' -count=1)
(cd apps/backend && go test ./internal/orchestrator -run 'Test.*(Startup|Reconcile)' -count=1)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Record the new idle test's expected failure before the production correction.
If a repository predicate changes, also run `go run ./cmd/sqlguard ./internal`
from `apps/backend` and record the result.

## Files likely touched

- `apps/backend/internal/task/service/active_session_stall.go`
- `apps/backend/internal/task/service/active_session_stall_test.go`
- `apps/backend/internal/task/models/models.go` only if candidate identity needs extension.
- `apps/backend/internal/task/repository/sqlite/session.go` and `session_test.go` only for a proven predicate gap.
- `apps/backend/internal/task/service/archived_session_reconciliation.go`
- `apps/backend/internal/task/service/orphaned_session_reconciliation_test.go`
- `apps/backend/internal/task/service/service_tasks.go` (shared settlement effects).
- `apps/backend/internal/orchestrator/service.go` (startup recovery parity, if needed).
- `apps/backend/internal/orchestrator/service_test.go` (startup recovery regressions).
- `AGENTS.md`

## Dependencies

None. Read the paired requirement and design before implementation.

## Risks

Filtering the all-active-session denominator would weaken the existing task-wide guard.
A second turn read must not replace the classified turn identity with an idle snapshot.

## Parallelism

`sequential`

## Inputs

- [Plan evidence](plan.md#confirmed-evidence)
- [Requirements](../../specs/tasks/requirements/session-stall-visibility.md)
- `active_session_stall_test.go` fixture and mixed-sibling tests.
- `idle_session_reaper.go` documents legitimate execution-less waiting sessions.

## Results

Implemented both reconciliation paths with guarded recoverable settlement.
Idle `WAITING_FOR_INPUT` sessions are excluded from stall classification, while
stale `STARTING` and `RUNNING` sessions, including sessions without a runtime
row, return to `WAITING_FOR_INPUT` after their observed unfinished turn is
abandoned. State, activity, message, turn, archive, and liveness predicates
protect the transition from races; stale executor reservations are repaired
without changing the resume identity. Settlement publishes a recoverable state
event and never emits turn completion or advances workflow state.

The service, SQLite repository, and reconciliation regression suites pass,
including idle siblings, turn read failures, missing executors, archive and
explicit-stop exclusions, and completion races.

The review remediation is included in this work order. Both sweeps now persist
the executorless recovery generation, use a fresh bounded effects context after
detached writes, and retry incomplete settlement from the session metadata.
The retry path accepts the same committed session generation after its stale
executor row was repaired, so an abandoned turn cannot lose its remaining
effects after the session becomes idle or after a successor changes the row.
Executor inventory read failures fail closed before the recovery write, and
CREATED sessions receive the same durable recovery token and interruption
marker as STARTING and RUNNING sessions.
