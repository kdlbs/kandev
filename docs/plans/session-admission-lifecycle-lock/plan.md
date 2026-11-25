---
status: done
requirements:
  - REQ-TASKS-BOUNDED-QUEUE-ADMISSION-001
  - REQ-AGENTS-AGENT-STALL-RECOVERY-001
system_design:
  - ../../specs/tasks/system-design/bounded-session-queue-admission.md
  - ../../specs/agents/system-design/agent-stall-recovery.md
---

# Session Admission Lifecycle Lock-Order Repair

## Outcome and authority

Repair [issue #5469](https://github.com/kdlbs/kandev/issues/5469): after a
failed first launch was retried, every session-admission operation on that
session timed out for about 75 minutes, including 21 focus resumes and three
deferred `move_task` calls.

- [Requirements](../../specs/tasks/requirements/bounded-session-queue-admission.md):
  AC-TASKS-BOUNDED-QUEUE-ADMISSION-001.3 and .7, AC-TASKS-BOUNDED-QUEUE-ADMISSION-001.8, and
  AC-TASKS-BOUNDED-QUEUE-ADMISSION-001.9 and .10.
- [System design](../../specs/tasks/system-design/bounded-session-queue-admission.md),
  section "Lock order with session lifecycle".
- [Agent stall recovery](../../specs/agents/system-design/agent-stall-recovery.md):
  AC-AGENTS-AGENT-STALL-RECOVERY-001.1 and .2 now also hold for dispatch-only
  prompts (work order 04).

## Confirmed root cause

A lock-order inversion between the session lifecycle lock and queue admission.

1. **Retry launch** runs `RecoverSession(..., "fresh_start")`.
   `resumeTaskSessionWithContinuationAndReservation` holds the session
   lifecycle lock until its continuation returns. The continuation,
   `replayInitialPromptSubmission`, calls `promptTask` with
   `dispatchOnly=false`, so it returns only when the agent's turn ends. The
   turn was one long interview of `ask_user_question` rounds, from 01:16:33 to
   02:43:19.
2. While a question was pending (session `WAITING_FOR_INPUT`), the web client
   sent a focus launch. `focusTaskSession` acquired prompt admission, then
   `refreshFocusedLiveSession` called the blocking `acquireSessionLifecycleLock`
   and waited for the replay turn to end while still holding admission.
3. Every later admission (focus, `SetPendingMove`) waited 30 s and failed with
   `ErrSessionAdmissionTimeout`. The first success after the turn completed was
   at 02:57.

The idle reaper's `suspendWorkspaceIdleCandidate` has the same inversion
(admission, then blocking lifecycle lock).

**Evidence:** the reporter's backend log on this host shows the gate held
exactly from the replay prompt dispatch to the end of that turn. The session's
`last_agent_error.recovery_actions` is `["retry_launch"]`, and no clarification
watchdog fired. A temporary test reproduced the hang: fresh-start recovery with
a blocking provider prompt, session set to `WAITING_FOR_INPUT`, then
`focusTaskSession`. After that, `WithSessionAdmission` with a 1 s budget
returned `context deadline exceeded`, and `SessionAdmissionActive` was true.
Without the focus call, admission stayed free.

## Work orders

Sequential. Each one can be verified on its own.

| Order | Work order                                                                 | Result                                                      |
| ----- | -------------------------------------------------------------------------- | ----------------------------------------------------------- |
| 1     | [Admission holders skip a busy lifecycle](task-01-admission-holders-skip-busy-lifecycle.md) | Focus and reaper never wait on the lifecycle lock under admission |
| 2     | [Release fresh-start replay at acceptance](task-02-release-replay-at-acceptance.md)        | Recovery stops holding the lifecycle lock for a whole turn  |
| 3     | [Retryable deferred-move timeout](task-03-deferred-move-retryable-timeout.md)              | Agent gets `UNAVAILABLE` and can retry `move_task`          |
| 4     | [Review follow-ups](task-04-review-follow-ups.md)                                          | Focus resume outside admission, no lost focus, stall notice for dispatch-only prompts |

Work order 1 alone removes the reported hang. Work order 2 removes the
long-held lifecycle lock that caused it, which other lifecycle callers would
also wait on. Work order 3 is the error mapping that the issue asks for. Work order 4 closes
three gaps that an independent review found in work orders 1 and 2.

## Risks

- **Work order 2:** with dispatch-only replay, a provider failure after
  acceptance no longer comes back through the recovery request. It goes through
  the normal agent event path, the same as an ordinary first launch. The
  existing fresh-start suites (`TestFreshStart*`) must stay green, especially
  the hold, cancellation, successor-error, and runtime-disappears cases.
- **Work order 4:** every dispatch-only prompt now gets the inactivity
  watchdog, not only the replay. Automation, peer-message, and continuation
  turns that stay silent for five minutes now show the same advisory notice and
  **Cancel turn** action as a user prompt. One manager-owned loop runs the
  check, so no goroutine outlives the manager.

## Exclusions

- `ResumeTaskSessionAndPrompt` (internal message retry) still holds the
  lifecycle lock until its turn ends. It holds no queue admission. The idle
  reaper's reclaim probe can wait on that lock and delay the shared reaper tick;
  the review flagged this as pre-existing, and it is a separate change.
- No change to the admission gate, its 30 s budget, or FIFO semantics. No
  schema, settings, or UI change.

## Verification

Each work order lists its exact commands. Package checks:

```text
make -C apps/backend fmt
(cd apps/backend && go test -trimpath -race -count=1 ./internal/agent/runtime/lifecycle/ -run 'TestDispatchOnly|TestStallTiming|TestPromptAcceptanceRestore')
(cd apps/backend && go test -trimpath -race -count=1 ./internal/orchestrator/ -run 'TestFocusTaskSession|TestFocusDuring|TestFocusResume|TestIdleSession|TestIdleParking|TestIdleSuspension|TestHandleAgentStalled|TestFreshStart|TestSessionAdmission')
(cd apps/backend && go test -trimpath -count=1 ./internal/mcp/handlers/ -run 'TestMoveTask')
bash scripts/lint-go-changed apps/backend/internal/agent/runtime/lifecycle/dispatched_prompt_stall_test.go apps/backend/internal/agent/runtime/lifecycle/manager_dispatched_stall.go apps/backend/internal/agent/runtime/lifecycle/manager_lifecycle.go apps/backend/internal/agent/runtime/lifecycle/session.go apps/backend/internal/agent/runtime/lifecycle/stall_acceptance_restore_test.go apps/backend/internal/agent/runtime/lifecycle/stall_snapshot.go apps/backend/internal/mcp/handlers/config_task_handlers.go apps/backend/internal/mcp/handlers/move_task_admission_timeout_test.go apps/backend/internal/orchestrator/event_handlers_stall.go apps/backend/internal/orchestrator/event_handlers_stall_test.go apps/backend/internal/orchestrator/fresh_start_replay_lifecycle_release_test.go apps/backend/internal/orchestrator/idle_parking_recovery_test.go apps/backend/internal/orchestrator/idle_session_admission_lock_order_test.go apps/backend/internal/orchestrator/idle_session_focus.go apps/backend/internal/orchestrator/idle_session_focus_resume_admission_test.go apps/backend/internal/orchestrator/idle_session_focus_test.go apps/backend/internal/orchestrator/idle_session_reaper.go apps/backend/internal/orchestrator/initial_submission_recovery.go apps/backend/internal/orchestrator/service.go apps/backend/internal/orchestrator/task_operations.go
make -C apps/backend test
```

## Implementation verification

All four work orders are done. Work order 04 records the backend CI failures
and the focused race checks that passed after their fixes. The original
implementation also passed the full race suites for
`internal/agent/runtime/lifecycle`, `internal/orchestrator`,
`internal/orchestrator/messagequeue`, and `internal/mcp/handlers`.

A live instance with the E2E mock agent reproduced the issue on the pre-fix
build: Retry launch held the lifecycle lock for the whole replayed turn, and
focus requests timed out with `queue session admission timed out`. On the fixed
build, Retry launch returns at acceptance and focus answers in milliseconds.
The agent's mid-turn `move_task` is deferred and then applied at turn end. A
silent replayed turn shows the stall notice.
