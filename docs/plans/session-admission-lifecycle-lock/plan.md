---
status: done
requirements:
  - REQ-TASKS-BOUNDED-QUEUE-ADMISSION-001
system_design:
  - ../../specs/tasks/system-design/bounded-session-queue-admission.md
---

# Session Admission Lifecycle Lock-Order Repair

## Outcome and authority

Repair [issue #5469](https://github.com/kdlbs/kandev/issues/5469): after a
failed first launch was retried, every session-admission operation on that
session timed out for about 75 minutes, including 21 focus resumes and three
deferred `move_task` calls.

- [Requirements](../../specs/tasks/requirements/bounded-session-queue-admission.md):
  AC-TASKS-BOUNDED-QUEUE-ADMISSION-001.3 (amended to name deferred workflow
  moves) and new AC-TASKS-BOUNDED-QUEUE-ADMISSION-001.5.
- [System design](../../specs/tasks/system-design/bounded-session-queue-admission.md),
  section "Lock order with session lifecycle".

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

Work order 1 alone removes the reported hang. Work order 2 removes the
long-held lifecycle lock that caused it, which other lifecycle callers would
also wait on. Work order 3 is the error mapping that the issue asks for.

## Risks

- **Work order 2:** with dispatch-only replay, a provider failure after
  acceptance no longer comes back through the recovery request. It goes through
  the normal agent event path, the same as an ordinary first launch. The
  existing fresh-start suites (`TestFreshStart*`) must stay green, especially
  the hold, cancellation, successor-error, and runtime-disappears cases.
- **Work order 1:** skipping a focus refresh while the lifecycle lock is busy
  leaves the idle-parking focus floor unchanged for that event. This is
  acceptable because a session with a lifecycle operation in progress is not
  settled (AC-EXECUTORS-IDLE-PARKING-001.6), and the next focus event refreshes
  it.

## Exclusions

- The focus path that resumes an idle-suspended session still calls
  `ResumeTaskSessionWithOptions` under admission, which takes the lifecycle
  lock itself. Work order 2 removes the replay's long hold.
  `ResumeTaskSessionAndPrompt` (internal message retry) still holds the lock
  until its turn ends. Reworking either path is a separate change.
- No change to the admission gate, its 30 s budget, or FIFO semantics. No
  schema, settings, or UI change.

## Verification

Each work order lists its exact commands. Package checks:

```text
make -C apps/backend fmt
go test -trimpath -race -count=1 ./internal/orchestrator/ -run 'TestFocusTaskSession|TestIdleSession|TestFreshStart|TestSessionAdmission'
go test -trimpath -count=1 ./internal/mcp/handlers/ -run 'TestMoveTask'
bash scripts/lint-go-changed <changed-go-files>
make -C apps/backend test
```

## Implementation verification

All three work orders are done. Each regression test failed before its fix and
passes after it. `go vet` is clean, and the full `internal/orchestrator`,
`internal/orchestrator/messagequeue`, and `internal/mcp/handlers` packages pass
under `go test -race`. `golangci-lint` on the changed packages
(`--new-from-rev`) reports 0 issues.
