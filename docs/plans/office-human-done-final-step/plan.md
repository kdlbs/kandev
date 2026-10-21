---
created: 2026-10-20
status: done
requirements:
  - REQ-OFFICE-HUMAN-DONE-001
  - REQ-OFFICE-HUMAN-DONE-002
system_design:
  - ../../specs/office/system-design/human-done-final-step.md
legacy_specs: []
---

# Implementation Plan: Office Human Done Moves to the Final Step

## Overview

Dropping an Office task on the status board's Done column, or picking Done in
its status picker, currently fails with "Cannot mark done: move the task to the
final workflow step first" when the task is not on the final workflow step.

Confirmed root cause:

- `DashboardService.applyApprovalGate`
  (`apps/backend/internal/office/dashboard/service_tasks.go`) treats a human
  Done request like an agent's. On a step that is not last by position, it
  rewrites the state to `REVIEW`.
- `UpdateTaskStatus` then persists that state, broadcasts it, and runs
  reactivity. `cascadeReviewRequested` (`office/scheduler/reactivity.go`) wakes
  every workflow-seated reviewer and approver, while the task stays on its
  earlier step.
- The handler then returns 409 `reason: "workflow_step"`. The board
  (`use-board-drag.ts`) settles the card on In Review and shows the toast.

A second gap surfaced while tracing. A human step move into a completing step
writes `COMPLETED` in the move transaction
(`task/service.syncTaskStateForWorkflowMove`). The Office approver seam later
skips the task because it is already complete, so pending approvers never
block a manual move.

Order: first make human moves respect approvers (Task 01), then route human
Done through that move (Task 02). Task 02 depends on Task 01's guard, so that
the board's Done drop cannot complete an unapproved task.

## Scope

### In scope

- A pre-write approver guard on human moves of Office tasks into completing
  steps, with the 409 `task_approvals_pending` contract and the Kanban/task-page
  message.
- A human Office Done request on a non-final step becomes a move to the final
  step: no `in_review` redirect, and nothing changes on refusal.
- Settling the board card and the status picker on the returned status, the new
  copy in every catalog, and desktop and phone E2E.

### Out of scope

- The agent Done path, the engine/automation move paths, and the existing
  final-step Done gate (AC-OFFICE-HUMAN-DONE-001.4, -002.3).
- Evaluating the completion gate on the final-step PATCH path (exclusion in the
  requirement).
- Making the Done column a disabled drop target.

## Technical approach

See the [system design](../../specs/office/system-design/human-done-final-step.md).

- **Task 01** (task service, task handlers, backendapp wiring, Kanban error
  copy):
  - `OfficeApprovalGuard` + `SetOfficeApprovalGuard` and
    `ApprovalsPendingMoveError` in `apps/backend/internal/task/service`.
  - The guard call in `MoveTaskWithOptions`, after
    `syncTaskStateForWorkflowMove` and before the admission write.
  - A 409 mapping in `task/handlers/errors.go` `handleSelectedMoveError`.
  - A dashboard adapter wired in `backendapp/main.go`.
  - `task_approvals_pending` in `components/task/task-move-error-message.ts`.
- **Task 02** (dashboard, backendapp, Office web):
  - `DashboardService.UpdateTaskStatusAsHuman`, called by the handler for
    requests with no agent caller.
  - `TaskFinalStepMover` + setter, and the `backendapp` adapter that picks
    the highest-position step and classifies refusals.
  - `FinalStepMoveRefusedError` and the 409 without `status` in
    `respondStatusUpdateError`.
  - 200 `{ok, status, workflow_step}`.
  - `office-status-gate.ts` returns the result and throws
    `StatusChangeRefusedError`.
  - `use-board-drag.ts` and `use-optimistic-task-mutation.ts` settle on the
    result; the board and `status-picker.tsx` show the info toast.
  - i18n keys in `en`, `pseudo`, `pt-pt`, `ja`, `ko`, `zh-cn`, `zh-hk`, `zh-tw`.

## ASCII UI preview

Structural requirements: the card's resting column and the toast text. Spacing
is illustrative. The board composition is shared by desktop and phone; the
phone scrolls the columns horizontally, a drag starts with a press-and-hold,
and toasts render at the bottom.

### UI-01: Board, Done drop on a Work-step task (AC-OFFICE-HUMAN-DONE-001.1-.3)

Before (current main):

```text
 Todo | In progress | In review     | Done |
      |             | [Fix login]   |      |   <- card lands in In review
 toast(error): Cannot mark done: move the task to the final workflow step first
```

After, final step completes on entry, no pending approvers:

```text
 Todo | In progress | In review | Done          |
      |             |           | [Fix login]   |
 (no toast)
```

After, final step does not complete on entry (AC-OFFICE-HUMAN-DONE-001.2):

```text
 Todo | In progress   | In review | Done |
      | [Fix login]   |           |      |   <- status the move produced
 toast(info): Moved to "Done". This step doesn't complete the task.
```

After, refused, approver pending (AC-OFFICE-HUMAN-DONE-001.3):

```text
 Todo | In progress   | In review | Done |
      | [Fix login]   |           |      |   <- back in its original column
 toast(error): Cannot mark done: awaiting approval from CEO
```

### UI-02: Status picker (AC-OFFICE-HUMAN-DONE-001.5)

```text
 Status  [In progress v]  -> choose Done
   success: [Done]
   refused: [In progress] + the same error toast as UI-01
```

Phone: the picker opens as the existing picker on the task page. Outcomes and
toasts match desktop.

## Tests

| AC | Evidence |
| --- | --- |
| AC-OFFICE-HUMAN-DONE-002.1, .2 | `task/service/office_approval_guard_test.go` `TestMoveTask_OfficeHumanIntoCompletingStep_ApproversPending_RefusedBeforeWrite`, `TestMoveTask_OfficeHumanApprovalGuardError_AbortsMove` |
| AC-OFFICE-HUMAN-DONE-002.3 | same file: `TestMoveTask_OfficeEngineMove_SkipsApprovalGuard`, `TestMoveTask_NonOfficeTask_SkipsApprovalGuard`, `TestMoveTask_OfficeHumanIntoNonCompletingStep_SkipsApprovalGuard`, `TestMoveTask_OfficeHumanWithoutGuard_Completes` |
| AC-OFFICE-HUMAN-DONE-002.1 | `task/handlers/move_approvals_error_test.go` `TestHandleSelectedMoveError_ApprovalsPending409`; `office/dashboard/pending_approvers_completion_test.go` (incl. `TestPendingApproversForCompletion_CountsTemplateApproverAtDestinationStep`); `backendapp/adapters_office_completion_test.go`; the guard receives the destination step in `TestMoveTask_OfficeHumanIntoCompletingStep_NoPendingApprovers_Completes` |
| AC-OFFICE-HUMAN-DONE-002.4 | `components/task/task-move-error-message.test.ts` "maps task_approvals_pending to the approvals-pending message" |
| AC-OFFICE-HUMAN-DONE-001.1 | `office/dashboard/human_done_final_step_test.go` `TestUpdateTaskStatusAsHuman_DoneAtWork_MovesToFinalStep`, `TestUpdateTaskEndpoint_HumanDoneAtWork_MovesToFinalStep`; `backendapp/adapters_office_final_step_test.go` `TestOfficeFinalStepMover_MovesToHighestPositionStepAsHuman` |
| AC-OFFICE-HUMAN-DONE-001.2 | `TestUpdateTaskStatusAsHuman_FinalStepDoesNotComplete_ReturnsMovedStatus`; `TestUpdateTaskEndpoint_HumanDoneQueued_ReportsQueued`; `TestOfficeFinalStepMover_ReportsWIPQueuedMove`; `use-board-drag-final-step.test.ts` "explains a move queued behind the final step's WIP limit"; `office-status-gate.test.ts` "returns the status and step the backend moved the task to", `finalStepMoveNotice`; `use-board-drag-final-step.test.ts` "settles on a non-done move result and reports the step"; `use-optimistic-task-mutation-settled-status.test.tsx` |
| AC-OFFICE-HUMAN-DONE-001.3 | `TestUpdateTaskStatusAsHuman_DoneAtWork_MoveRefused_NoStateChangeNoWake`, `TestUpdateTaskEndpoint_HumanDoneRefused_409WithoutStatus`, `TestUpdateTaskEndpoint_HumanDoneRefusedMoveConflict_CarriesCode`, `TestOfficeFinalStepMover_TranslatesRefusals`; `office-status-gate.test.ts` refusals; `use-board-drag-final-step.test.ts` "rolls a refused human done back to the original column" |
| AC-OFFICE-HUMAN-DONE-001.1 (determinism, overlap) | `TestFinalWorkflowStep_BreaksPositionTiesByID`; `use-board-drag-final-step.test.ts` "does not apply an older move result while a newer drop is still in flight"; `use-optimistic-task-mutation-settled-status.test.tsx` "ignores an older reported status while a newer status write is pending" |
| AC-OFFICE-HUMAN-DONE-001.4 | `TestUpdateTaskStatusAsHuman_TerminalStep_KeepsDirectGate`; existing `approval_gate_step_test.go` agent cases |

## E2E tests

| Flow | AC | File, project |
| --- | --- | --- |
| Drag a first-step card to Done; it completes on the final step | 001.1 | `e2e/tests/office/board-done-final-step.spec.ts`, `chromium` |
| Drag with a pending approver; the card stays out of Done, the toast names the approver, the backend is unchanged | 001.3, 002.1 | same file, `chromium` |
| Existing gated-done picker flow still passes | 001.4 | `e2e/tests/office/approval-flow.spec.ts`, `chromium` |
| Phone: the status picker's Done completes a first-step task; a pending-approver refusal is visible and changes nothing | 001.5, 002.4 | `e2e/tests/office/mobile-board-done-final-step.spec.ts`, `mobile-chrome` |

## Work orders

- [x] [Task 01: Human step moves respect Office approvers](task-01-move-approver-guard.md)
- [x] [Task 02: Human Done moves the task to the final step](task-02-human-done-final-step.md)

## Verification results

2026-10-20, on `origin/main` a02114193:

- Go: `task/service`, `task/handlers`, `backendapp`, `office/dashboard`,
  `orchestrator` pass. Under `TMPDIR=/dev/shm`, 21 git-fixture tests in
  `task/handlers`, `backendapp` and `orchestrator` fail at their `git commit`
  setup step; they pass with the default `TMPDIR`.
- `bash scripts/lint-go-changed <changed files>`: 0 issues.
- Web: targeted Vitest suites pass; `pnpm run typecheck`, ESLint on changed
  files, `i18n:check`, `i18n:ratchet`, `e2e:sleep-ratchet` and `make fmt` clean.
- E2E `chromium`: `board-done-final-step` (2), `approval-flow` and `tasks` (8)
  pass. `mobile-chrome`: `mobile-board-done-final-step` (2) pass.

Review fixup (PR #4642), 2026-10-21: the approver guard reads the
destination step's template approvers; the final step picks a stable winner on
position ties; a WIP-queued move reports `queued` and shows its own notice; an
older status response no longer overrides a newer in-flight choice. Focused
Go packages (`office/dashboard`, `office/repository/sqlite`, `task/service`,
`backendapp`) and the web suites above pass after the fixup.

## Risks

- `approval-flow.spec.ts` and other E2E specs that PATCH `done` from a
  non-final step as a human change outcome. Search
  `apps/web/e2e/tests/office` for `status: "done"` and `moveTaskToTerminalStep`
  before Task 02 and update them in that work order.
- The Office DTO has no workflow fields. The final step is resolved on the
  backend only; the UI learns the step name from the PATCH response.
- The approver guard has a read-then-write window (system design, *Human
  move approver guard*).

## Open questions

None.
