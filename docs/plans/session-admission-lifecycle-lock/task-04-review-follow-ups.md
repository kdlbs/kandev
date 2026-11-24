---
id: "session-admission-lifecycle-lock-04"
title: "Review follow-ups: focus resume, focus floor, dispatch-only stall watch"
status: done
wave: 4
depends_on: ["session-admission-lifecycle-lock-01", "session-admission-lifecycle-lock-02"]
plan: "plan.md"
requirements:
  - REQ-TASKS-BOUNDED-QUEUE-ADMISSION-001
  - REQ-AGENTS-AGENT-STALL-RECOVERY-001
acceptance_criteria:
  - AC-TASKS-BOUNDED-QUEUE-ADMISSION-001.5
  - AC-AGENTS-AGENT-STALL-RECOVERY-001.1
  - AC-AGENTS-AGENT-STALL-RECOVERY-001.2
system_design:
  - ../../specs/tasks/system-design/bounded-session-queue-admission.md
  - ../../specs/agents/system-design/agent-stall-recovery.md
---

# Work Order 04: Review follow-ups

## Outcome

An independent review of work orders 01 to 03 found three gaps. This work
order closes them:

1. **Focus resume held admission.** Focus on an idle-suspended session called
   `ResumeTaskSessionWithOptions` under admission. That call waits for the
   route and lifecycle locks and for the session restore, so a slow restore
   held admission past the 30 s budget.
2. **A focus could be lost.** When the lifecycle lock was busy, for example
   during a reaper liveness probe, focus returned without restarting the idle
   interval. The same reaper tick could then suspend the session the user had
   just focused.
3. **The replayed turn lost its stall notice.** Dispatch-only prompts have no
   prompt waiter, and the waiter was the only producer of `agent.stalled`. The
   dispatch-only replay from work order 02 therefore showed no "Still waiting"
   notice or **Cancel turn** action. Every other dispatch-only prompt had the
   same gap, against AC-AGENTS-AGENT-STALL-RECOVERY-001.1.

## Scope

- `idle_session_focus.go`: `focusTaskSessionAdmitted` reports
  `resumeSuspended`, and `focusTaskSession` runs `resumeFocusedIdleSuspension`
  after admission is released. The resume keeps its existing
  `RequireIdleSuspensionProvenance` check under its locks.
- `idle_session_focus.go`: `refreshFocusedLiveSession` calls
  `resetIdleParkingCandidates` when the lifecycle lock is busy. Admission still
  serializes this with suspension revalidation. The persisted focus record is
  still written only under the lifecycle lock.
- `lifecycle/manager_dispatched_stall.go`: the `Manager` starts one
  `dispatchedPromptStallLoop` beside its remote status loop, registered on its
  WaitGroup and ended by `Stop`. Each tick, `checkDispatchedPromptStalls` runs
  the stall check for every execution whose dispatch-only prompt is pending,
  once per generation. `waitForPromptDone` and the loop share
  `reportPromptStall` (`lifecycle/session.go`).

## Rejected alternative

A watcher goroutine per dispatch-only prompt also worked, but it lived until
its next tick after settlement and broke the single-owner goroutine rule. The
package goleak check caught leftover watchers.

Keeping the replay as a full-turn prompt on a detached goroutine kept the stall
notice, but broke turn-end handling. Once the continuation returned, the resume
attempt was finished, so `promptTask`'s post-turn check failed with
`resume attempt cancelled`. The deferred move and the workflow's
on-turn-complete step did not run (verified on a live instance).

## Regression tests (each failed before the fix)

- `TestFocusResumeOfSuspendedSessionKeepsAdmissionFree`
  (`idle_session_focus_resume_admission_test.go`): an admission probe succeeds
  while a gated restore is in progress.
- `TestFocusDuringBusyLifecycleDefersIdleSuspension` and
  `TestFocusDuringBusyLifecycleKeepsAdmissionFree`
  (`idle_session_admission_lock_order_test.go`).
- `TestDispatchOnlyPromptReportsOneStall`, plus
  `TestDispatchOnlyStallCheckSkipsSettledPrompt` and
  `TestDispatchedPromptStallLoopEndsAtStop`
  (`lifecycle/dispatched_prompt_stall_test.go`).

Test hygiene: `runWithinBudget` returns the worker's error to the test
goroutine and joins the worker in cleanup. The replay fixture reports
acceptance before a full-turn result, as the runtime does
(`mockAgentManager.promptAcceptedBeforeResult`), and joins any started turn
before teardown.

## Verification

```text
cd apps/backend
go test -trimpath -race -count=1 ./internal/agent/runtime/lifecycle/ ./internal/orchestrator/ ./internal/orchestrator/messagequeue/ ./internal/mcp/handlers/
bash ../../scripts/lint-go-changed <changed-go-files>
```

## Results

Implemented. Each regression test above failed before its fix and passes
after it. A live instance (E2E mock-agent profile, 5 s stall threshold)
confirmed the following. Retry launch returns at acceptance, and focus during
the replayed turn answers in about 10 ms. The agent's `move_task` is deferred
and then applied at turn end. "Still waiting for the agent." with
**Cancel turn** appears on a silent replayed turn.
