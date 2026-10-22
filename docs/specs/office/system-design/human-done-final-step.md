---
status: draft
system: office
requirements:
  - REQ-OFFICE-HUMAN-DONE-001
  - REQ-OFFICE-HUMAN-DONE-002
---

# Office Human Done System Design

## Purpose and boundaries

Two existing paths meet here:

- `PATCH /api/v1/office/tasks/:id` with `status: "done"`:
  `dashboard.Handler.applyTaskMutations` -> `DashboardService.UpdateTaskStatus`
  -> `applyApprovalGate` (`office/dashboard/service_tasks.go`). On a step that
  is not last by position, the gate rewrites the state to `REVIEW`, and the rest
  of `UpdateTaskStatus` persists it, publishes `office.task.status_changed` and
  `task.updated`, and runs reactivity. Reactivity's `in_review` branch
  (`office/scheduler/reactivity.go` `cascadeReviewRequested`) wakes every
  workflow-scoped reviewer and approver. The handler then returns 409
  `reason: "workflow_step"`.
- `POST /api/v1/tasks/:id/move`: `task/handlers.httpMoveTask` ->
  `task/service.MoveTaskWithOptions`. `syncTaskStateForWorkflowMove` sets
  `COMPLETED` when the target step is terminal (`wfmodels.IsTerminalStep`: no
  next step and `CompleteTaskOnEnter`). The state is written in the move's
  admission transaction, where the completion gate
  (`guardTaskCompletionTransitionTx`) also runs. The orchestrator's Office seam
  (`markOfficeTaskCompletedForTerminalStep`) later skips the task because it is
  already complete, so Office approvers are never consulted.

The design adds a pre-write approver guard to the human move, then routes a
human Office Done request on a non-final step through that move. The agent
path, the engine path, and the final-step Done path are unchanged.

## Human move approver guard (REQ-OFFICE-HUMAN-DONE-002)

`task/service` gains one optional seam, wired like its other setters:

```go
// OfficeApprovalGuard reports the approvers whose decision still blocks an
// Office task from completing.
type OfficeApprovalGuard interface {
    PendingApproversForCompletion(ctx context.Context, taskID, targetStepID string) ([]PendingApprover, error)
}

type PendingApprover struct {
    AgentProfileID string
    Name           string
}

type ApprovalsPendingMoveError struct{ Pending []PendingApprover }
```

`MoveTaskWithOptions` calls the guard after `syncTaskStateForWorkflowMove` and
before the admission write, only when all of these hold:

- `opts.StepHistoryActor == wfmodels.StepTransitionActorHuman`;
- `task.IsFromOffice`;
- the sync changed the state to `COMPLETED` (the task was not already
  terminal and the move was admitted, not queued).

A non-empty result returns `*ApprovalsPendingMoveError` before any write. A
guard read error aborts the move with that error (fail closed). With no guard
wired the move proceeds unchanged, which keeps non-Office deployments and unit
tests on the current path.

`backendapp` wires the guard with an adapter over
`DashboardService.PendingApproversForCompletion`, which reads approver seats
as if the task stood at the destination step
(`Repository.ListTaskParticipantsAtStep`: per-task seats across the destination workflow
plus the destination step's template seats, even when the task has no current
step) and resolves names. "Pending"
keeps one definition, approver seats without a current approval, and a
template approver on the destination step counts even though the task has
not reached it yet.

`task/handlers.handleSelectedMoveError` maps the error to 409
`{code: "task_approvals_pending", error, pending_approvers: [{agent_profile_id, name}]}`.

The guard reads seats and decisions outside the admission transaction. An
approver seated, or an approval superseded, between that read and the write is
not seen by this move. The existing PATCH gate has the same read-then-write
window; closing it would need the Office decision read inside the task
repository transaction, which this design does not add.

## Human Done on a non-final step (REQ-OFFICE-HUMAN-DONE-001)

`dashboard.Handler.applyTaskMutations` routes a status change with no agent
caller (`agentIDFromCtx` empty) to `DashboardService.UpdateTaskStatusAsHuman`.
Agent requests, the runtime agent action and the orchestrator seam keep calling
`UpdateTaskStatus`, so they keep today's gate.

`dashboard` gains one seam, set from `backendapp`:

```go
type TaskFinalStepMover interface {
    MoveTaskToFinalStep(ctx context.Context, taskID string) (*FinalStepMoveResult, error)
}

type FinalStepMoveResult struct {
    State    string // persisted task state after the move (set by the mover)
    Status   string // Office status, filled by UpdateTaskStatusAsHuman
    StepID   string
    StepName string
}
```

`UpdateTaskStatusAsHuman` calls the mover only when the requested state is
`COMPLETED`, the mover is wired, and `IsTaskWorkflowStepTerminal` reports
`hasStep && !terminal`. Otherwise it delegates to `UpdateTaskStatus`. On the
move path it writes no state, activity or event of its own: the move publishes
`task.updated`, `task.state_changed` and `task.moved`, and the orchestrator
owns completion side effects. A step lookup error aborts the request.

The `backendapp` adapter `officeFinalStepMoverAdapter` reads the task, takes
the step with the highest position from `ListStepsByWorkflow` (a position tie
goes to the larger step ID), and calls
`MoveTaskWithOptions` with `StepHistoryActor: human` and
`AllowActivePrimarySession: true`, the same options as `httpMoveTask`. It
classifies refusals into `*dashboard.FinalStepMoveRefusedError{Reason, Code, Pending}`
with the task handlers' exported `CompletionGateCode` and `MoveConflictCode`:

| Move error | `reason` | Extra fields |
| --- | --- | --- |
| `*ApprovalsPendingMoveError` | `approvals` | `pending_approvers` |
| completion-gate errors | `completion_gate` | `code` |
| move conflicts | `workflow_move` | `code` |

Any other error passes through unchanged and keeps the existing 400 response.

`respondStatusUpdateError` returns 409 with `reason`, `error` and the extra
fields, and **no `status` field**: the absence of `status` tells the client
that nothing changed.

On success the PATCH returns 200
`{ok: true, status, queued, workflow_step: {id, name}}`. `queued` is true when
the final step's WIP limit queued the move (`QueuedForStepID` is the final
step); the task then keeps its prior state until the step admits it. Other
PATCH requests keep returning `{ok: true}`.

The step read and the move are not one transaction. If another actor moves the
task in between, `MoveTaskWithOptions` revalidates the task and either applies
the move to the final step or refuses it like any other move.

## Frontend

`lib/api/domains/office-status-gate.ts`:

- `updateTaskStatusOrTranslateGate` returns `{status, workflowStepName?}` from
  the PATCH body, defaulting `status` to the requested value.
- A 409 with a `status` field keeps throwing `ApprovalGateError` (agent-style
  redirect, still produced by the final-step approver gate).
- A 409 without `status` throws `StatusChangeRefusedError` (no redirect) with
  a translated message: pending approvers through
  `formatPendingApproversMessage`, `completion_gate` through
  `task:cannotMarkDoneCompletionCriteria`, `workflow_move` through
  `getTaskMoveErrorMessage` with `task:cannotMarkDoneMoveRefused` as the
  fallback.

`app/office/tasks/use-board-drag.ts` `applyStatusDrop` settles the card on the
returned status. `hooks/use-optimistic-task-mutation.ts` settles a status
mutation on the status its API call returns, which covers the status picker
(`components/task/simple/components/status-picker.tsx`). A
`StatusChangeRefusedError` restores the pre-mutation snapshot like any failed
write. A response only corrects the displayed status while its write is the
latest one for the task (`shouldRestoreAfterFailedWrite`); otherwise the
reconciliation of the last write to settle decides. When the returned status
is not the requested `done`, both show an info toast with the step name
(`finalStepMoveNotice`): `task:movedToFinalStepQueued` for a queued move,
otherwise `task:movedToFinalStepNotCompleted`.

`components/task/task-move-error-message.ts` maps `task_approvals_pending` to
`task:taskMoveErrorApprovalsPending` for Kanban and task-page moves.

All new copy is added in every catalog (`en`, `pseudo`, `pt-pt`, `ja`, `ko`, `zh-cn`, `zh-hk`, `zh-tw`).

## Observability

No new metric. Refusals are visible as 409 responses with a stable `reason`
or `code`. The move path logs as today.

## Compatibility

- `TestUpdateTaskEndpoint_409OnWorkflowStepGate` keeps covering the redirect
  when no mover is wired. The agent cases in `approval_gate_step_test.go` stay.
- `apps/web/e2e/tests/office/approval-flow.spec.ts` "gated done transition"
  still passes: the refused move leaves the task in `in_review`, where the test
  put it, and the toast still names the approver.
- A direct human move of an Office task with pending approvers into a
  completing step now fails with 409 instead of completing.
