---
id: "01-preserve-launch-ownership"
title: "Preserve initial launch ownership"
status: complete
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-PARENT-CHILD-MESSAGE-INTERRUPT-001
acceptance_criteria:
  - AC-TASKS-PARENT-CHILD-MESSAGE-INTERRUPT-001.1
  - AC-TASKS-PARENT-CHILD-MESSAGE-INTERRUPT-001.2
  - AC-TASKS-PARENT-CHILD-MESSAGE-INTERRUPT-001.3
system_design:
  - ../../specs/tasks/system-design/parent-child-message-interrupt.md
---

# Task 01: Preserve initial launch ownership

## Summary

Queue peer messages behind an accepted initial launch. Prevent competing starts,
stale rollback, and failure cleanup from changing or stopping the winning agent.

## In scope

- Write `TestPeerMessageInitialLaunch_Delivery` first and reproduce the duplicate
  launch or destructive state transition before production changes.
- Add the remaining deterministic cases from the plan's test matrix.
- Serialize launch admission before peer-message turn preparation.
- Return typed busy ownership from stale starts and admit the message to its queue.
- Guard executor mutations before description, turn, configuration, and state writes.
- Fence failure and rollback writes against concurrent startup progress.
- Preserve ordinary prepared-session start, genuine failure recovery, cancellation,
  queue identity, initial brief, Auto-run, and sender metadata.

## Out of scope

Public API changes, UI, schema migrations, historical repairs, automatic retries,
global submission deduplication, and new scheduling infrastructure.

## Acceptance

1. The full handler-to-runtime race starts the original brief once and retains
   the follow-up in the queue while the initial turn is active. Auto-run OFF
   keeps the accepted follow-up pending.
2. Losing starts cannot rewrite, fail, roll back, or stop the winning attempt,
   including retries that share an execution ID.
3. Owned failures still settle correctly. Cancellation and prepared-only launches
   retain their existing behavior. Existing queue-readiness tests pass.

## Verification

Run from the repository root. Record the initial expected failure before the fix.

```bash
(cd apps/backend && go test ./internal/mcp/handlers -run '^TestPeerMessageInitialLaunch_' -count=1)
(cd apps/backend && go test -race ./internal/mcp/handlers ./internal/orchestrator ./internal/orchestrator/executor ./internal/orchestrator/messagequeue -count=1)
(cd apps/backend && go test ./internal/mcp/... -run '^$')
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short -- docs/plans/peer-message-initial-launch
```

If lifecycle implementation changes, also run:

```bash
(cd apps/backend && go test -race ./internal/agent/runtime/lifecycle -count=1)
```

If conditional SQL changes, add SQLite and environment-gated PostgreSQL
concurrency cases under the repository guidance. Record the PostgreSQL result
or missing test environment explicitly. Do not claim dialect coverage from mocks.

## Files likely touched

- `apps/backend/internal/mcp/handlers/handlers.go`
- `apps/backend/internal/mcp/handlers/message_task_initial_launch_test.go` (new)
- `apps/backend/internal/mcp/handlers/message_task_readiness_test.go`
- `apps/backend/internal/orchestrator/task_operations.go`
- `apps/backend/internal/orchestrator/task_create_prompt.go`
- `apps/backend/internal/orchestrator/service.go`
- `apps/backend/internal/orchestrator/peer_message_initial_launch_test.go` (new)
- `apps/backend/internal/orchestrator/executor/executor_execute.go`
- `apps/backend/internal/orchestrator/executor/launch_failure.go`
- `apps/backend/internal/orchestrator/executor/executor_start_ownership_test.go` (new)

Use new focused helpers instead of growing oversized production files.
Runtime and repository collaborators remain conditional scope only if existing
ownership operations cannot provide the required atomicity.

## Dependencies

None. Preserve the completed MCP queue-wakeup implementation already on this base.

## Risks

Lock inversion, a lost follow-up after a busy outcome, and stale snapshot restoration.
Do not turn a state-conflict error into a blanket success or disable owned cleanup.

## Parallelism

`sequential`

## Inputs

- [Plan and incident evidence](plan.md)
- [Requirements](../../specs/tasks/requirements/parent-child-message-interrupt.md)
- [Design, Initial launch ownership](../../specs/tasks/system-design/parent-child-message-interrupt.md#initial-launch-ownership)
- Existing `message_task_readiness_test.go` handler-to-orchestrator fixture.
- Existing `executor_launch_failure_classification_test.go` failure-ownership cases.
- Existing session lifecycle lock and executor session lock.

## Results

Peer-message admission now reserves the session lifecycle lock before task-state
promotion and `on_turn_start` preparation. It releases that lock before waiting
for a cancel-in-flight guard, then rechecks the session and rejects a terminal
winner. This keeps the lock order safe without mistaking cancellation for a
running launch. Losing messages queue against the captured session incarnation.
Parent interrupt intent follows the losing-start path and targets the exact
queued entry.

`TestPeerMessageInitialLaunch_HandlerKeepsFollowUpQueuedUntilTurnBoundary`
starts the original brief through the real MCP handler and blocks runtime
startup. It then sends a second message through that handler. The follow-up
stays in the identity-aware queue with Auto-run OFF, and the initial turn stays
active after the process starts.

`TestPeerMessageInitialLaunch_QueuesBeforeWorkflowTurnPreparation` starts the
initial creation through `LaunchSession` with `InitialCreatePrompt`, then sends
the follow-up through the real MCP message handler while runtime startup blocks.
Its configured transition moves step A to step B before the runtime start
blocks. The follow-up queues without evaluating step B's second transition.
Assertions cover the task step and state, session profile and primary recipient,
initial-create evidence, pending signal, active turn, and queued entry.

`TestPeerMessageInitialLaunch_QueueRejectionDoesNotRollbackWinningProgress`
uses the full MCP message handler. It fills the queue, blocks the competing
start, advances the winning session to RUNNING, and changes task state, turn,
metadata, and queue ownership before releasing the loser. Queue-full rejection
preserves that state and leaves no undispatched user-message row.
`TestPeerMessageInitialLaunch_RollbackFencesSameStateSessionProgress` races an
owned start failure against same-state session progress. The task-only case
independently advances task state, workflow step, and queue. The sibling-session
case and the review/non-review session-switch cases verify that failed
preparation preserves a newly selected session and its transferred queue.
Session rollback compares lifecycle state and owned fields (error, completion,
primary, profile, profile snapshot, and metadata), then uses the current row
revision for an atomic session and metadata write. This permits the failed
message's own turn-recording writes to advance `updated_at` without allowing
independent field changes to be erased. Task rollback checks the task state and
workflow step captured after preparation in the same repository statement as
the session ownership check.

The executor rechecks active-agent evidence before profile replacement, prompt
description, turn binding, runtime configuration, or session-state writes. The
production `NewService` callback classifies a RUNNING CAS winner as typed busy;
`TestNewServiceSessionStartingCallbackClassifiesRunningCASConflict` exercises
the installed callback and preserves the winning RUNNING row. Both ordinary
start and initial-create failure funnels skip destructive cleanup for that
unowned outcome. `TestLaunchInitialCreatePromptBusyDoesNotFailLiveSession`
covers the outer initial-create error path. Bootstrap failure commits now
compare a persisted startup-attempt identity as well as execution identity and
error stamp. This keeps a losing failure from clobbering a retry that reuses the
same execution ID;
`TestCommitBootstrapFailureIfCurrentAttemptRejectsSameExecutionRetry` exercises
the SQLite CAS, and
`TestBootstrapFailureCASMissDoesNotStopSameExecutionRetry` proves a losing
same-execution failure cannot force-stop the retry. Prepared workspace start,
cancel handling, and genuine owned failure remain covered by
`TestExistingWorkspaceStart_PreparedWorkspaceCanStart`,
`TestBeginPeerMessageStartWaitsForCancelAndRejectsTerminalSession`, and
`TestBootstrapFailureProjection`.

The handler tests reproduce workflow advancement before admission, full-handler
queue rejection, and stale same-state, task-only, and sibling-session rollback.
The production-service test reproduces the RUNNING CAS conflict. The executor
and SQLite tests reproduce a failure CAS miss against a retry that reuses the
same execution ID. The scheduling regression is covered by
`TestScheduleTaskForSessionDoesNotRegressConcurrentInProgress`.

Verification passed:

- `go test ./internal/mcp/handlers -count=1` passed after the final handler and
  queue-rejection test changes.
- `go test -race ./internal/mcp/handlers ./internal/orchestrator ./internal/orchestrator/executor ./internal/orchestrator/messagequeue -count=1` passed.
- `make -C apps/backend build` passed with `TMPDIR` directed to the workspace
  cache after the default `/tmp` filesystem ran out of space. The build emitted
  its expected warning that macOS binaries are unsigned because codesign tools
  are unavailable.
- `go test ./internal/mcp/... -run '^$'` compiled all MCP packages successfully.
- `go test ./internal/task/repository/sqlite -run '^TestPostgresBootstrapFailureIfCurrentAttemptRejectsSameExecutionRetry$' -count=1` passed with the PostgreSQL case skipped because `KANDEV_TEST_POSTGRES_DSN` is unset.
- `python3 scripts/list-docs.py validate` validated 309 decisions and 1185
  specifications; `python3 scripts/lint-spec-files.py --all` passed.
- `git diff --check` passed.
- Focused handler regressions for owned failure rollback, queue rejection,
  task-only progress, and partial workflow-preparation failure passed.
- Focused orchestrator tests for `TestReviewAdmissionWaitDoesNotBlockIndependentScheduling`
  and `TestScheduleTaskForSessionDoesNotRegressConcurrentInProgress` passed.

The fix changes conditional SQLite session and bootstrap-failure writes. The
SQLite CAS tests cover same-state progress and same-execution retry. No
PostgreSQL-backed execution was possible without `KANDEV_TEST_POSTGRES_DSN`;
production currently wires the SQLite repository.
