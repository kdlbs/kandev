---
status: current
system: tasks
requirements:
  - REQ-TASKS-SESSION-STOP-SETTLEMENT-001
created: 2026-10-20
owners:
  - kandev
---

# Session Stop Settlement Design

## Boundary and mapping

| Criteria | Implementation boundary |
| -------- | ----------------------- |
| .1       | `orchestrator.Service.reapPromptUnreadyExecution` with the exact `executionTeardownClaim` |
| .2       | `orchestrator.Service.StopSession` and `settleExecutionLessSessionStop` |

## Completed teardown owner

`reapPromptUnreadyExecution` stops the execution only when it wins the forced
cleanup claim. When the claim is held by another path, it reads that exact
claim and continues only if `cleanupCompleted` is set, matching
`cleanupAgentExecutionWithReason`. An unfinished owner keeps the existing
"teardown is already owned" error.

The winning reaper retains its exact claim token. A successful stop marks
that token complete so subsequent cleanup can proceed without a duplicate
stop; a failed stop releases only that token so recovery remains retryable.

## Execution-less stop

When `executor.Stop` returns `lifecycle.ErrNoExecutionForSession`,
`StopSession` asks `settleExecutionLessSessionStop`. Settlement must first
try-acquire the existing session lifecycle lock, then try-acquire the
cancellation/prompt admission guard, releasing ownership on either contention.
It never waits for either lock: existing auto-start and dynamic-successor
launch paths can hold the prompt guard before requesting lifecycle ownership.
Blocking on that guard while owning lifecycle would deadlock those launches.
Cancelled contexts retain their context error.
It refuses an active cancellation or resume attempt and rechecks runtime
absence under those guards, propagating lookup errors. The guards remain held
through the session-state read and turn closure. This excludes both new-turn
admission and admission that would reuse the same captured turn.

Only `WAITING_FOR_INPUT`, `COMPLETED`, `FAILED`, and `CANCELLED` settle, through
`completeTurnForTaskSessionCheckedOwned` with the captured turn ID, which
refuses a different active turn. A missing session row settles with nothing
to close. Other states, admission contention, and newly observed executions
retain the original error. Runtime teardown is never performed under these
settlement guards.

## Verification

Orchestrator tests cover a completed teardown owner during prompt-readiness
recovery, successful and retryable failed reaper cleanup, and execution-less
stops of waiting and terminal sessions. Admission-boundary tests use the real
prompt claim and SQLite turns to expose same-turn reuse; additional cases
preserve successor turns, in-flight lifecycle/cancellation/resume ownership,
and newly registered executions. Created, starting, running, and idle states
remain refused, and lookup/turn-completion errors propagate.

A channel-coordinated lock-order regression drives `StopSession` against the
real `StartCreatedSession` entry point under the existing inverse-order guard
ownership. It verifies that settlement yields, the launch crosses its lifecycle
boundary, the captured turn remains open, and no guard reference leaks.
