---
id: "02-cover-restart-reconciliation"
title: "Cover restart reconciliation of unfinished taskless attempts"
status: pending
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-OFFICE-TASKLESS-001
acceptance_criteria: []
system_design:
  - ../../specs/office/system-design/taskless-run-sessions.md
---

# Task 02: Cover restart reconciliation of unfinished taskless attempts

> **PENDING FOLLOW-UP — 2026-09-19.** This work order is not part of the current
> coverage change. `AC-OFFICE-TASKLESS-001.6` remains an outstanding requirement,
> and this order covers its restart reconciliation behavior.
>
> The file is kept, unedited below this banner, because a follow-up should start
> from it rather than re-derive it. The current gaps and the two problems a
> follow-up must resolve first are recorded under
> [Outstanding: stop controls and restart recovery](../../specs/office/requirements/taskless-run-sessions.md#outstanding-stop-controls-and-restart-recovery).
> References below to `taskless-run-recovery.md` point at a design file superseded in the same
> change; its content is summarized in that section.

## Summary

`AC-OFFICE-TASKLESS-001.6` is the restart criterion: unfinished attempts are
reconciled against runtime evidence before a replacement launch, a possibly live
predecessor blocks a replacement until it is stopped or proven absent,
interrupted work gets an explicit outcome, and no taskless attempt stays claimed
indefinitely just because it has no task. The system design's verification
section requires testing "restart with a live/absent/unknown predecessor".

The behavior has shipped in
`apps/backend/internal/backendapp/office_run_session_launcher.go`
(`ReconcileRunSessions`, `sameRunOwner`, `isLiveExecution`), and it is on the
startup path: `internal/backendapp/main.go:1134` aborts backend startup when it
returns an error. None of it has a test. `office_run_session_launcher.go` has no
`_test.go` sibling, and the taskless tests that exist
(`office/service/taskless_lifecycle_test.go`,
`office/repository/sqlite/run_sessions_test.go`) cover launch, late completion,
usage deduplication and reservation CAS, not restart recovery.

This work order adds that coverage. It adds no production behavior. Task 04
changes two discarded error results in the same file; where a case below names
behavior Task 04 introduces, this work order asserts the behavior Task 04
specifies rather than the current discard.

## In scope

Add `apps/backend/internal/backendapp/office_run_session_launcher_test.go`
covering every branch `ReconcileRunSessions` can take for a durable attempt left
in `preparing` or `running`:

- **Never registered** — `execution_id` empty. Expect the attempt finished as
  `interrupted` with `"runtime execution was not registered"` and its run
  requeued.
- **Absent predecessor** — `GetExecution` returns a not-found error. Expect any
  retained runtime resource stopped, the attempt finished as `interrupted` with
  `"runtime execution was not found after restart"`, and the run requeued.
- **Live predecessor** — status `pending`, `starting`, `running` or `ready`.
  Expect the attempt left non-terminal and the run *not* requeued, so no
  replacement can launch beside it. Cover all four statuses; a table test is
  fine.
- **Ended while down** — status `completed` finishes the attempt and the run;
  `failed` records the runtime error message and requeues; any other terminal
  status is `interrupted` and requeues.
- **Unknown owner** — a returned execution whose `ExecutionOwner` disagrees with
  the durable record on any of kind, workspace, run, run session, attempt or
  agent profile. Expect an error, not a silent skip and not a replacement.
  Assert per-field, not only on a wholesale mismatch, because `sameRunOwner` is
  a six-way conjunction and a single dropped term would still pass a single
  blunt case.
- **Inspect failure** — a non-not-found error from `GetExecution` returns an
  error rather than treating the predecessor as absent.
- **Requeue failure** — the store's requeue errors, and separately matches no
  row. Expect `ReconcileRunSessions` to return an error rather than reporting
  success with the run left claimed behind a terminal attempt. This is the case
  the current code cannot pass, because both results are discarded; Task 04 is
  the production change that makes it passable, and re-skipping or softening it
  is not an option.

Also assert the startup consequence once, at the seam that owns it: a
reconciliation error must fail backend startup rather than being logged and
stepped past. If driving `main.go`'s path is impractical, assert on the
`ReconcileRunSessions` error being returned through
`office/service/service.go:105` and record in the test why the startup caller is
covered by inspection instead.

## Out of scope

- Any production change to `ReconcileRunSessions` or its helpers **beyond the
  two discarded results Task 04 owns**. If some other test cannot be written
  without changing production behavior, that is a finding against
  `REQ-OFFICE-TASKLESS-001`, not a licence to edit the code under test.
- Cancellation sweeps (`office/pause`), which serve `AC-OFFICE-TASKLESS-001.5`
  and already have `office/pause/service_test.go`.
- Postgres parity for the reconciliation query itself; `ListUnfinishedRunSessions`
  is covered by the repository's existing migration harness.

## Validation

```text
cd apps/backend
go test ./internal/backendapp/ -run 'TestOfficeRunSessionLauncher|TestReconcile' -v -count=1
go test ./internal/backendapp/ -run 'TestOfficeRunSessionLauncher|TestReconcile' -count=5
make lint
```

Goroutine ownership applies: if the test stands up anything that starts a
goroutine, instrument it with goleak per `apps/backend/AGENTS.md`.

## Reference

- `apps/backend/internal/backendapp/office_run_session_launcher.go:218` —
  `ReconcileRunSessions`, and `:280`/`:286` for `sameRunOwner` / `isLiveExecution`
- `apps/backend/internal/backendapp/main.go:1134` — the startup caller that
  aborts on error
- `apps/backend/internal/office/repository/sqlite/run_sessions.go:200` —
  `ListUnfinishedRunSessions`, the input to reconciliation
- `apps/backend/internal/office/service/taskless_lifecycle_test.go` — existing
  taskless test style to match

## Dependencies

None. Independent of Task 01: different file, different criterion. Both may run
in wave 1.

## Risks

- **Fake-passing through an over-stubbed runtime.** The interesting branches are
  distinguished by what `GetExecution` returns. A stub that returns the same
  shape everywhere will make every case look correct. Drive each branch from a
  distinct stub response and assert the durable row, not just the absence of an
  error.
- **Asserting the message instead of the outcome.** The reason strings are
  useful evidence but are not the criterion. Assert the attempt's terminal state
  and whether the run was requeued; check the message as a secondary assertion.
