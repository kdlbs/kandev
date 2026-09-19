---
id: "04-fail-closed-on-recovery-and-stop-errors"
title: "Fail closed on discarded requeue and stop errors"
status: withdrawn
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-OFFICE-TASKLESS-001
acceptance_criteria: []
system_design:
  - ../../specs/office/system-design/taskless-run-sessions.md
---

# Task 04: Fail closed on discarded requeue and stop errors

> **WITHDRAWN — 2026-09-19.** This work order is not to be built. `AC-OFFICE-TASKLESS-001.5`
> and `.6` were cut from the requirement after five rounds of spec review, and this order
> existed only to satisfy them. It made the discarded requeue result and the discarded `runtime.Stop` error fail closed for `AC-OFFICE-TASKLESS-001.5` and `.6`. Neither criterion exists now; the discarded stop error is recorded as an accepted gap in the system design's scheduling and routing section.
>
> The file is kept, unedited below this banner, because a follow-up that revives the
> deferred flows should start from it rather than re-derive it. The deferral, the accepted
> gaps and the two problems a follow-up must resolve first are recorded under
> [Deferred: stop controls and restart recovery](../../specs/office/requirements/taskless-run-sessions.md#deferred-stop-controls-and-restart-recovery).
> References below to `taskless-run-recovery.md` point at a design file retired in the same
> change; its content is summarized in that section.

## Summary

Two error results are discarded on the run-session launcher's failure paths, and
each one silently produces a state its acceptance criterion forbids.

**Requeue (`AC-OFFICE-TASKLESS-001.6`).** `ReconcileRunSessions`
(`internal/backendapp/office_run_session_launcher.go`) calls
`_, _ = l.repo.RequeueClaimedRun(ctx, session.RunID)` on all three recovery
branches — never-registered, absent-after-restart, and ended-while-down with a
non-finished outcome. Both results are dropped. If the requeue errors or matches
no row, the attempt is already terminal, the run stays `claimed`, and no
execution exists behind it: an attempt claimed indefinitely, which is exactly
what the criterion forbids. Startup still reports success, so nothing surfaces
it.

**Stop (`AC-OFFICE-TASKLESS-001.5` and the design's no-two-live-executions
rule).** `StartRunSession`'s failure paths call
`_ = l.runtime.Stop(...)` and then, on several of them,
`FinishRunSession(..., Failed, ...)`. A stop that failed followed by a terminal
row is the case the system design rules out in as many words: the row leaves the
non-terminal set, so `ListLiveRunSessionsForWorkspace` and
`ListUnfinishedRunSessions` never list it again while the process may still be
running. It is the forbidden two-live-executions window in its least visible
form.

## In scope

- Treat a failed or no-op requeue in `ReconcileRunSessions` the same way an
  unconfirmable owner is treated: return an error, so backend startup fails
  rather than proceeding with a run claimed behind a terminal attempt. The
  boolean matters as much as the error — a requeue that matched no row is the
  same bad state as one that errored, and both must be distinguishable in the
  returned message.
- On `StartRunSession`'s failure paths, stop discarding the `runtime.Stop`
  error. When the stop fails, do **not** write a terminal session state: leave
  the row live with its execution ID so the next sweep and the next startup
  reconciliation re-list it, record the stop failure on the session
  (`error_message`) and in the returned error, and log it at error level with
  run, session, attempt and execution identity.
- Keep the existing behavior when the stop succeeds: terminal state, existing
  message, existing return.

## Out of scope

- The reconciliation branch structure, `sameRunOwner` and `isLiveExecution`.
  Their logic is correct; only the discarded results change.
- Retry or backoff policy for a failed requeue. Failing startup is the
  specified outcome; a retry loop is a different decision and is not taken here.
- Task 03's cancellation seam, which is the same rule applied to a different
  set of callers.

## Tests

- Requeue returns an error: `ReconcileRunSessions` returns an error naming the
  run, and the attempt's terminal state is still written (the attempt is
  genuinely over; it is the run that must not be left claimed silently).
- Requeue returns `(false, nil)`: same, with a message that distinguishes "no
  row matched" from a store error.
- Requeue succeeds: no error, run requeued — the existing happy path stays.
- `StartRunSession` with a failing `runtime.Stop` on the bind-failure path and
  on the start-failure path: the session row is **not** terminal, still carries
  its execution ID, records the stop failure, and the returned error names it.
- `StartRunSession` with a succeeding stop: unchanged terminal behavior.

Assert on the durable row, not only on the returned error. A stub runtime that
returns the same shape everywhere will make every branch look correct.

## Validation

```text
cd apps/backend
go test ./internal/backendapp/ -run 'TestOfficeRunSessionLauncher|TestReconcile|TestStartRunSession' -v -count=1
go test ./internal/backendapp/ -run 'TestOfficeRunSessionLauncher|TestReconcile|TestStartRunSession' -count=5
make lint
```

## Reference

- `apps/backend/internal/backendapp/office_run_session_launcher.go` —
  `StartRunSession` failure paths and `ReconcileRunSessions`
- `apps/backend/internal/office/repository/sqlite/run_sessions.go` —
  `FinishRunSession`, `ListUnfinishedRunSessions`,
  `ListLiveRunSessionsForWorkspace`
- `apps/backend/internal/backendapp/main.go` — the startup caller that aborts
  on a reconciliation error

## Dependencies

None. Shares a file with Task 02, which is test-only: if both run in the same
tree, land Task 02's test file first so this task's behavior changes have
assertions to break.

## Risks

- **Failing startup too eagerly.** Only a requeue that did not take may abort
  startup. A run that was already terminal or already queued is not a failure;
  check what `RequeueClaimedRun` returns for those cases before choosing the
  error condition.
- **Leaking a live row forever.** Leaving a row live on stop failure is
  deliberate, and it is only correct because startup reconciliation and the pause
  sweep both re-list non-terminal rows. Confirm both still reach it.
