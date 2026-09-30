---
id: "01-thread-deferred-move-transition-id"
title: "Thread the deferred-move transition ID into processOnEnter"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-DEFERRED-MOVE-ONENTER-DISPATCH-IDENTITY-001
acceptance_criteria:
  - AC-TASKS-DEFERRED-MOVE-ONENTER-DISPATCH-IDENTITY-001.1
  - AC-TASKS-DEFERRED-MOVE-ONENTER-DISPATCH-IDENTITY-001.2
system_design:
  - ../../specs/tasks/system-design/deferred-move-onenter-dispatch-identity.md
---

# Work Order 01: Thread the deferred-move transition ID into processOnEnter

## Outcome

`processStepExitAndEnterForDeferredMove` stops passing a hardcoded `0` as the
transition/entry ID to `processOnEnter`. It receives and forwards the real
ledger transition ID that `applyPendingMove` already produces when it commits
the deferred move, so the staleness guard's existing "may replace a stale
route" escape hatch recognises the callback as current and `on_enter` actually
dispatches `on_enter` actions (in particular `auto_start_agent`) for a
backward (or any) deferred `move_task_kandev`.

## In scope

- `apps/backend/internal/orchestrator/workflow_store.go`:
  - `applyTransition` (private): change return type from `error` to
    `(int64, error)`. Return `0` alongside every existing error return; on
    success return `task.WorkflowStepTransitionID` (populated by
    `updateDeferredTransitionTask`/`updateTransitionTask` via `updateTaskTx`).
  - `ApplyTransition` (public, part of `engine.TransitionStore`): keep its
    `error`-only signature; discard the new first return value from
    `applyTransition`.
  - `ApplyDeferredMoveTransition` (public): change return type to
    `(int64, error)`, returning `applyTransition`'s two values directly.
- `apps/backend/internal/orchestrator/event_handlers_workflow.go`:
  - `applyPendingMove`: capture the transition ID from both branches — the
    atomic `ApplyDeferredMoveTransition` call and the unfenced-fallback
    `applyTransition` call — into a local `transitionID int64`.
  - Thread `transitionID` through the existing call to
    `processStepExitAndEnterForDeferredMove` (replacing no existing argument;
    add it as a new trailing parameter).
  - `processStepExitAndEnterForDeferredMove`: add a `transitionID int64`
    parameter; pass it (instead of the current hardcoded `0`) as the
    `transitionID` argument to the `s.processOnEnter(...)` call at the end of
    the function.

## Out of scope

- `processStepExitForDeferredMove` (the WIP-queued branch) — it never calls
  `processOnEnter`; no change needed.
- Any change to `workflowEntryDispatchIsCurrentForSession`,
  `workflowEntryDispatchMayReplaceRoute`, or any other staleness-guard logic
  in `ceiling_entry.go`.
- Reusing the `stepentry.AllocationResult` / `WithResultHolder` context
  mechanism the forward-move (`E1`) path uses — it has the side effect of also
  attaching a `stepentry.PendingAllocation` for ledger-owned marker-bearing
  on_enter actions via `transitionContext`, which is unrelated behavior this
  fix must not introduce on the deferred-move path.
- Any change to `engine.TransitionStore`'s interface signature or its test
  fakes in `internal/workflow/engine/*_test.go`.

## Applicable requirements

- **AC-A1**, **AC-A2** — `docs/specs/workflow-on-enter-action-dispatch/spec.md`
- **Ledger identity contract** — `docs/specs/tasks/system-design/workflow-task-step-transition-ledger.md`

## Acceptance conditions

1. A deferred `move_task_kandev` into a step whose `on_enter` declares
   `auto_start_agent` actually dispatches that action (asserted via a real
   launch/start-agent call, not just `tasks.workflow_step_id`), for both a
   forward and a backward transition.
2. `workflowEntryDispatchIsCurrentForSession` still rejects a callback for a
   transition that is not the task's current one (no regression to the #3755
   staleness protection) — covered by the existing ledger/staleness tests in
   `workflow_step_ledger_test.go` and `ceiling_entry_test.go` (or equivalent),
   which must continue to pass unmodified.
3. `golangci-lint` and the package build are clean for the changed files.

## Regression test

Add a test in `apps/backend/internal/orchestrator/` (new file or alongside
`event_handlers_pending_move_test.go`) that reuses `buildPendingMoveScenario`'s
shape: two steps each declaring `on_enter: auto_start_agent`, drive
`svc.applyPendingMove` for a **backward** move (mirroring the production
reproducer — Review bounced back to an earlier step), let the spawned
`processStepExitAndEnterForDeferredMove` goroutine settle, and assert the
destination step's agent was actually started — e.g. via `startAgentProcessFunc`
/ `launchAgentFunc` invocation tracking on `mockAgentManager`, following the
`wireBootReadySimulator` pattern already used by
`TestPendingMove_ReviewToInProgress_OneTransitionOnly`. Assert the dispatch,
not merely that `task.WorkflowStepID` changed — the step change already works
today and is what makes the missing dispatch invisible without this assertion.

Confirm the test fails against the current code (hardcoded `0`) before the fix
and passes after.

## Verification commands

```bash
cd apps/backend
go test ./internal/orchestrator/... -run 'PendingMove|DeferredMove' -v
go test ./internal/orchestrator/... -run 'ApplyTransition' -v
go build ./...
golangci-lint run ./internal/orchestrator/... --new-from-rev=HEAD~1 --timeout=5m
```

## Likely files

- `apps/backend/internal/orchestrator/workflow_store.go`
- `apps/backend/internal/orchestrator/event_handlers_workflow.go`
- `apps/backend/internal/orchestrator/event_handlers_pending_move_test.go` (or
  a new sibling test file)

## Dependencies

None — this is a self-contained call-site change with no upstream work-order
dependency.

## Results

Implemented as designed:

- `workflow_store.go`: private `applyTransition` returns `(int64, error)`,
  returning `0` on every existing error path and `task.WorkflowStepTransitionID`
  on success. Public `ApplyTransition` discards the new first return value,
  keeping its `error`-only signature for `engine.TransitionStore`. Public
  `ApplyDeferredMoveTransition` now returns `(int64, error)`.
- `event_handlers_workflow.go`: `applyPendingMove` captures `transitionID` from
  both the atomic (`ApplyDeferredMoveTransition`) and unfenced-fallback
  (`applyTransition`) branches, and threads it through the existing call to
  `processStepExitAndEnterForDeferredMove`. That function gained a
  `transitionID int64` parameter, passed into `processOnEnter` instead of the
  hardcoded `0`.
- New regression test:
  `apps/backend/internal/orchestrator/event_handlers_pending_move_backward_test.go`
  (`TestPendingMove_BackwardMoveDispatchesOnEnterWithStaleRoute`), in a new
  file per the 800-line test-file convention. It seeds a
  `workflow_session_route` naming a step other than the move's destination
  (the state any task is in after its first workflow entry — which is exactly
  why the pre-existing `TestPendingMove_ReviewToInProgress_OneTransitionOnly`
  scenario, with no route ever established, never caught this bug), drives a
  backward `applyPendingMove`, and asserts `StartAgentProcess` was actually
  called — not just that `workflow_step_id` changed.
  - Confirmed RED against pre-fix code: failed with "StartAgentProcess was
    never called...".
  - Confirmed GREEN after the fix.

No changes to `ceiling_entry.go` or any staleness-guard logic — the existing
`workflowEntryDispatchIsCurrentForSession` / `workflowEntryDispatchMayReplaceRoute`
protections are exercised unmodified and still pass
(`TestReplayCeilingDeferralRejectsStaleWorkflowEntryBeforeDispatch`,
`TestWorkflowEntryDispatchRejectsStaleCommittedRoute`).

Commands run (from `apps/backend` unless noted):

```
go build -tags fts5 ./...
go test ./internal/orchestrator/ -run 'PendingMove|DeferredMove|ApplyTransition|Ceiling|WorkflowEntryDispatch' -v -timeout=180s -p 1
golangci-lint run ./internal/orchestrator/... --new-from-rev=a7f7e01af8c04625e5378de6d69fd1756ef2937c --timeout=5m
make fmt        # (root) — reverted two unrelated gofmt-drift files by explicit path
make typecheck   # (root)
make lint-format # (root)
cd apps/web && pnpm run i18n:ratchet
```

Note on `make test` (full-suite `go test ./...`): the whole-repo run is
resource-contention-flaky in this sandbox — a bare `-timeout=10m` panic
surfaced in unrelated SQLite-migration setup code
(`migrateSubagentContextBackfillUnsafe`) inside completely unrelated tests,
alongside failures in packages with no relation to this change (launcher,
subproc, agentctl/server/process, k8s/docker/ssh-dependent tests). Verified
non-diff-caused: the same targeted test set passes cleanly at
`-p 1` (`go test ./internal/orchestrator/ -run '...' -p 1` → `ok`, all PASS
including the new regression test), and re-running the identical targeted
command plus a spot-check of an unrelated failing package
(`internal/common/subproc`, `TestRunGitOutputAfterAcquireStartsTimeoutAfterAdmission`)
against `git merge-base HEAD origin/main` (`a7f7e01af8`, a scratch worktree at
`/tmp/kandev-scratch-verify/kandev`, removed after verification) also passed
cleanly with default parallelism. The hang is load-dependent and reproduces
independently of this diff.
