---
created: 2026-11-25
status: complete
requirements:
  - REQ-TASKS-SESSION-STOP-SETTLEMENT-001
system_design:
  - ../../specs/tasks/system-design/session-stop-settlement.md
legacy_specs: []
---

# Implementation Plan: Execution-less stop clears orphaned background activity

## Overview

This package covers the part of GitHub issue #4459 that is still open. Earlier
fixes (#4495, #5082, #5405, #4983) already settle Claude shells, Monitors,
subagents, and work whose process has ended. One gap remains. A session can
keep a nonzero `active_subagent_count` after its execution is gone, and a
supported `session.stop` never clears it.

**Root cause.** `StopSession` handles `lifecycle.ErrNoExecutionForSession`
through `settleExecutionLessSessionStop`, which returns as soon as the turn is
settled. That early return skips the `settleOpenBackgroundWork` call that
follows a normal stop. No `agent.stopped` event exists for this path either, so
`retireExecutionActivityAndPublish` never runs. As a result, the session's
in-memory `turnActivity.background` registrations stay in place.
`ActiveSubagentCount` derives from those registrations, so the count never
drops. Those same registrations make idle suspension treat the session as
having `known_work`.

**Reproduction.** A temporary orchestrator test (deleted after the run) used
`newExecutionLessStopService(t, WAITING_FOR_INPUT)` and registered a subagent
for execution `dead-exec`. It then called `StopSession`, which returned
success, and observed `ActiveSubagentCount == 1` afterwards.

The fix is one work order: reconcile inside the existing settlement guards.

## Scope

### In scope

- `settleExecutionLessSessionStop` (`session_stop_settle.go`). On a successful
  settlement, it retires the in-memory activity of every execution ID that
  owns state in the session's `turnActivity`. It uses one bounded cleanup
  context from `backgroundWorkCleanupContext(ctx)` for activity publication
  and durable background row settlement.
- A small `turnActivity` helper that lists the owning execution IDs
  (`turn_activity.go`).
- Regression tests in a new file,
  `apps/backend/internal/orchestrator/session_stop_settle_background_test.go`.

### Out of scope

- Claude `asyncTasks` lifecycle capability (AC-AGENTS-BACKGROUND-WORK-010.2,
  rejected by ADR `2026-11-01-claude-async-task-lifecycle-capability.md`).
- Live-execution stops, `StopSessionSynchronously`, and startup or idle-reclaim
  settlement, which are already covered.
- Registrations without an execution ID, and any clearing based on age.

## Technical approach

`settleExecutionLessSessionStop` already holds the session lifecycle lock and
the cancellation/prompt admission guard. It has also re-confirmed that the
runtime reports no execution for the session. Under those guards, every
execution ID recorded in `turnActivity` belongs to a dead execution. After
`settleExecutionLessSessionTurn` returns `settled == true`, settlement does
two things:

1. It creates a bounded cleanup context with
   `backgroundWorkCleanupContext(ctx)` and calls
   `s.executionIDsWithActivity(sessionID)`, a new locked read that
   returns the distinct non-empty IDs from `background`, `tools`, and
   `executionClaims`. For each ID it calls `retireExecutionActivityAndPublish`
   with the cleanup context, task ID, session ID, and execution ID. This
   context is cancellation-independent and bounded, so publication reads
   cannot hold the session guards indefinitely. Retirement already keeps a
   record that an in-flight prompt or dispatch token uses, so successor fences
   are preserved.
   `settleExecutionLessSessionTurn` returns the loaded task ID; a missing
   session row returns an empty task ID, which retires activity without
   publishing a session event.
2. It calls `s.settleBackgroundWorkLocked(cleanupCtx, sessionID, "")` using
   the same bounded context. This reuses
   the liveness probe, execution-identity recheck, and retained-owner guards,
   and only the lifecycle lock is needed, which is already held.
   `probeAgentRunning` calls the agent manager directly and takes no
   orchestrator admission lock.

This is the simplest correct fix. It reuses the existing retirement and
settlement functions and the guards the stop path already holds. There is no
new state, timer, or event.

## Tests

| AC | Evidence |
| -- | -------- |
| AC-TASKS-SESSION-STOP-SETTLEMENT-001.3 | `TestStopSession_ExecutionLessSettlementRetiresDeadExecutionActivity`: a subagent and a shell registered for `dead-exec`. After stop, `ActiveSubagentCount == 0` and the background map is empty. Fails before the fix. |
| AC-TASKS-SESSION-STOP-SETTLEMENT-001.3 | `TestStopSession_ExecutionLessSettlementSettlesOpenBackgroundRows`: an open durable workload row is settled through the settler when the probe reports no live process. With a live or failing probe, the row stays open. |
| AC-TASKS-SESSION-STOP-SETTLEMENT-001.3 | `TestStopSession_ExecutionLessRefusalKeepsActivity`: an `IDLE` state, lifecycle-lock contention, or a newly registered execution returns the original outcome, and the registration and count are unchanged. |
| AC-TASKS-SESSION-STOP-SETTLEMENT-001.3 | `TestStopSession_ExecutionLessSettlementSettlesRowsAfterRequestCancellation`: cancellation after turn closure does not prevent durable settlement, and publication's session read remains deadline-bounded. |
| AC-TASKS-SESSION-STOP-SETTLEMENT-001.2 | The existing `session_stop_settle_review_test.go` and `session_stop_settle_lock_order_test.go` suites keep passing unchanged. |

## E2E tests

None. The observable change is a backend count and durable rows that existing
websocket events already deliver. No UI contract changes. There is no
deterministic browser path that reaches an execution-less stop with surviving
registrations without test-only hooks.

## Work orders

- [x] [Task 01: Retire orphaned activity on execution-less stop](task-01-retire-orphaned-activity.md) — complete

## Verification results

- Focused execution-less stop tests and the full requested race test command
  passed; see Task 01 Results for exact commands.
- `go build -trimpath ./internal/orchestrator` and
  `make -C apps/backend build-kandev` passed.
- Changed-file Go lint passed with 0 issues.
- Specification catalog validation and spec lint passed.

## Risks

- Lock order: retirement takes the activity publication guard and
  `foregroundActivityMu` while lifecycle and admission are held. Prompt
  admission takes the same activity locks after admission, never the reverse,
  so no cycle is expected. The lock-order suite must stay green.
- Publishing a retired-activity snapshot from the stop path is new for this
  path. It reuses the existing publication contract that `agent.stopped` uses.
