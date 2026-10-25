---
created: 2026-09-21
status: implemented
requirements:
  - REQ-TASKS-DEFERRED-MOVE-ONENTER-DISPATCH-IDENTITY-001
system_design:
  - ../../specs/tasks/system-design/deferred-move-onenter-dispatch-identity.md
legacy_specs:
  - ../../specs/workflow-on-enter-action-dispatch/spec.md
  - ../../specs/tasks/system-design/workflow-task-step-transition-ledger.md
---

# Implementation Plan: Deferred move on_enter dispatch identity

## Outcome

A deferred `move_task_kandev` (E2, applied at turn end via `applyPendingMove` ->
`processStepExitAndEnterForDeferredMove`) currently commits the workflow-step
transition and then has its own `on_enter` dispatch rejected as stale, so
`auto_start_agent` (and any other on_enter action) never runs. This fix threads
the transition ledger ID that the deferred-move commit already produces into
`processOnEnter`, so the existing "may replace a stale route" escape hatch
(`workflowEntryDispatchMayReplaceRoute` in `ceiling_entry.go`, added for #3755)
recognises this callback as the current, legitimate entry instead of falling
through to the route-mismatch rejection.

## Root cause

`processStepExitAndEnterForDeferredMove` (`apps/backend/internal/orchestrator/event_handlers_workflow.go:5086`)
calls `s.processOnEnter(ctx, identity.TaskID, fresh, entryStep, taskDescription, 0, fromStep)`
with a hardcoded `transitionID` of `0`. `processOnEnter`'s staleness guard
(`workflowEntryDispatchIsCurrentForSession` -> `workflowEntryDispatchRouteIsCurrent`
in `ceiling_entry.go:303`) checks `workflowEntryDispatchMayReplaceRoute` first,
which requires `entryIDs[0] > 0` to match the callback against
`task.WorkflowStepTransitionID` via `workflowEntryDispatchMatchesCurrentTaskEntry`
(`ceiling_entry.go:367`). With `0`, that check short-circuits to `false`
(`ceiling_entry.go:373`: `len(entryIDs) == 0 || entryIDs[0] <= 0`), so the guard
falls through to comparing the task's stored `workflow_session_route` against
the destination step. That route is only updated to name the destination
*inside* `maybySwitchSessionForProfile`, which runs **after** the staleness
guard (PR #3826 moved route preparation there). The route therefore still
names the source step, the comparison fails, and `processOnEnter` returns
before dispatching `on_enter`.

The transition ledger ID this callback needs already exists: the deferred-move
commit path (`workflowStore.applyTransition`, called via
`ApplyDeferredMoveTransition` or the unfenced `applyTransition` fallback) writes
it via `updateTaskTx` (`apps/backend/internal/task/repository/sqlite/task.go:1258`,
`task.WorkflowStepTransitionID = transitionID`) into the same `*models.Task`
value `applyTransition` holds locally — but neither `ApplyTransition` nor
`ApplyDeferredMoveTransition` return it, so it is discarded before reaching
`applyPendingMove` and the goroutine it spawns.

The forward-move path (`applyEngineTransitionWithCommitMode`,
`event_handlers_workflow.go:7666`) already solves this correctly for `E1`,
using the existing `stepentry.AllocationResult` / `stepentry.WithResultHolder`
context out-parameter to capture `stepEntry.TransitionID` and thread it into
`launchProcessOnEnter`. That mechanism is deliberately **not** reused here: it
also causes `transitionContext` to attach a `stepentry.PendingAllocation` for
ledger-owned marker-bearing on_enter actions
(`workflow_store.go:460`), which is unrelated behavior this fix must not
introduce as a side effect on the deferred-move path. Instead, `applyTransition`
is changed to also return the `int64` transition ID it already computes, and
that value is threaded explicitly through the two existing call sites down to
`processOnEnter`.

## Requirement and system-design mapping

- **AC-A1**, **AC-A2** (`docs/specs/workflow-on-enter-action-dispatch/spec.md`):
  every declared `on_enter` action must execute, in order, exactly once per
  step entry, on every entry path including `E2` (manual move, which includes
  both immediate and deferred `move_task_kandev`). A backward deferred move
  into a step declaring `auto_start_agent` currently executes none of its
  `on_enter` actions, violating both criteria for that entry path.
- **Ledger identity contract** (`docs/specs/tasks/system-design/workflow-task-step-transition-ledger.md`):
  `task.WorkflowStepTransitionID` is the immutable per-write ledger identity
  the repository layer populates after every step-changing write, including
  the `mcp_deferred_move` trigger. This fix is purely a consumer-side gap: the
  identity is already produced correctly and durably; it was never threaded to
  the one caller that needs it to identify its own dispatch as current.

## Constraints

- `event_handlers_workflow.go` / `workflow_session_target.go` / `ceiling_entry.go`
  are under heavy concurrent upstream churn (8 commits by carlosflorencio,
  2026-09-10 to 2026-09-20). Keep the diff to call-site plumbing only.
- Do not weaken, bypass, or delete `workflowEntryDispatchIsCurrentForSession` or
  `workflowEntryDispatchMayReplaceRoute`. They correctly reject a genuinely
  stale callback (AC-TASKS-QUEUED-SESSION-OWNERSHIP-002.6-style protection);
  the defect is that a legitimate entry had no way to identify itself.
- `engine.TransitionStore.ApplyTransition` is a broad interface implemented by
  several test fakes across `internal/workflow/engine`. Its public signature
  (`error` only) is unchanged. Only `workflowStore.ApplyDeferredMoveTransition`
  — which has exactly one production call site and no direct test fakes — gains
  an `int64` return.

## Architecture

`workflowStore.applyTransition` (private, `workflow_store.go:344`) already
leaves the transition ID on its local `task.WorkflowStepTransitionID` after a
successful `updateDeferredTransitionTask`/`updateTransitionTask` call. Change
its signature to `(int64, error)`, returning `task.WorkflowStepTransitionID` on
success and `0` alongside every existing error return.

- `workflowStore.ApplyTransition` (public, part of `engine.TransitionStore`):
  keep its `error`-only signature; discard the new first return value.
- `workflowStore.ApplyDeferredMoveTransition` (public, one call site): change
  to return `(int64, error)`.
- `Service.applyPendingMove` (`event_handlers_workflow.go:4938`): capture the
  transition ID from both the atomic (`ApplyDeferredMoveTransition`) and
  unfenced-fallback (`applyTransition`) branches.
- `Service.processStepExitAndEnterForDeferredMove` (`event_handlers_workflow.go:5050`):
  add a `transitionID int64` parameter, pass it into the `processOnEnter` call
  at line 5086 instead of the hardcoded `0`.
- `applyPendingMove`'s call to `processStepExitAndEnterForDeferredMove`
  (`event_handlers_workflow.go:5005`): pass the captured transition ID through.

`processStepExitForDeferredMove` (the WIP-queued branch, which does not call
`processOnEnter`) is unchanged — its later promotion re-enters through the
ledger dispatcher (`DispatcherLedger`), which already has its own dispatch
identity from the promotion write, not from this call.

## Verification strategy

A regression test at the `orchestrator` package level drives a deferred
`move_task_kandev` backward transition (source step B -> destination step A,
where A declares `on_enter: auto_start_agent`) end to end through
`applyPendingMove`, and asserts the auto-start dispatch actually ran (not just
that `tasks.workflow_step_id` changed) — matching the existing shape of
`TestApplyTransitionRecordsEngineTransitionFromSessionID` and neighboring tests
in `workflow_step_ledger_test.go`, but exercising the deferred-move goroutine
path (`processStepExitAndEnterForDeferredMove`) specifically, since the direct
`ApplyTransition`/`applyTransition` ledger tests do not reach `processOnEnter`
at all.

## Work orders

- [task-01-thread-deferred-move-transition-id.md](task-01-thread-deferred-move-transition-id.md)

## Risks

- Low blast radius: `ApplyDeferredMoveTransition` has one production caller and
  no test fakes to update. `applyTransition` is private with two callers
  besides the public wrappers, both in this same file.
- The unfenced-fallback branch (`SupportsAtomicDeferredMoveTransition() ==
  false`) is exercised by the in-memory message queue backend used in tests;
  the regression test should cover the path actually reachable in the test
  environment and note which backend it exercises.
