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

1. The full handler-to-runtime race produces one initial launch and one later
   follow-up delivery. Auto-run OFF leaves the follow-up pending.
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
promotion and `on_turn_start` preparation. The cancel-in-flight guard is acquired
non-blockingly, so contention returns typed busy and releases the lifecycle lock
instead of inverting the established lock order. Losing messages queue against
the captured session incarnation. The handler removes only its own undispatched
message and preserves the winning launch's turn.

`TestPeerMessageInitialLaunch_QueuesBeforeWorkflowTurnPreparation` starts the
initial creation through `LaunchSession` with `InitialCreatePrompt`, then sends
the follow-up through the real MCP message handler while runtime startup blocks.
Its configured transition moves step A to step B before the runtime start
blocks. The follow-up queues without evaluating step B's second transition.
Assertions cover the task step and state, session profile and primary recipient,
initial-create evidence, pending signal, active turn, and queued entry.

`TestPeerMessageInitialLaunch_QueueRejectionDoesNotRollbackWinningProgress`
fills the queue, makes the competing start lose after the original launch
advances, and verifies the queue-full error preserves RUNNING state, task
progress, session and task metadata, the active turn, and every existing queued
entry. No user-message row remains after rejection.

The executor rechecks active-agent evidence before profile replacement, prompt
description, turn binding, runtime configuration, or session-state writes. The
production `NewService` callback classifies a RUNNING CAS winner as typed busy;
`TestNewServiceSessionStartingCallbackClassifiesRunningCASConflict` exercises
the installed callback. Both ordinary start and initial-create failure funnels
skip destructive cleanup for that unowned outcome.
`TestLaunchInitialCreatePromptBusyDoesNotFailLiveSession` covers the outer
initial-create error path. Prepared workspace starts and owned workflow rollback
remain covered by the full package suite.

The pre-fix regressions reproduced workflow advancement and metadata clearing,
queue-rejection rollback of a winning launch, and an untyped CAS conflict through
`NewService`'s installed callback.

Verification passed:

- `go test ./internal/mcp/handlers -run '^TestPeerMessageInitialLaunch_' -count=1`.
- `go test -race ./internal/mcp/handlers ./internal/orchestrator ./internal/orchestrator/executor ./internal/orchestrator/messagequeue -count=1`.
- `go test -race ./internal/mcp/handlers -run '^(TestPeerMessageInitialLaunch_QueuesBeforeWorkflowTurnPreparation|TestPeerMessageInitialLaunch_QueueRejectionDoesNotRollbackWinningProgress)$' -count=1`.
- `go test ./internal/mcp/... -run '^$'`.
- Go lint on all changed backend packages: zero issues.
- `make -C apps/backend build` (passed; macOS outputs remained unsigned because codesign tools are unavailable in the environment).
- `python3 scripts/list-docs.py validate`: 309 decisions and 1185 specifications.
- `python3 scripts/lint-spec-files.py --all`.
- `git diff --check`.

No lifecycle package or conditional SQL changed.
