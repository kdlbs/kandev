---
status: current
system: tasks
requirements:
  - REQ-TASKS-EDIT-SAVE-RETRY-001
---

# Task edit save retry system design

## Purpose and boundaries

Retain the current edit on a rejected save by correcting the shared frontend
submission lifecycle. Task-field persistence continues through the existing
`updateTask` API. This uses the existing in-memory editor lifetime; it introduces
no storage, API, ownership boundary, feature flag, or transaction. No ADR is
needed for this local correction within the established lifecycle.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-TASKS-EDIT-SAVE-RETRY-001` | Submission lifecycle, Confirmed stages, Consumers and responsive behavior, Regression boundary |

## Original failure mechanism

`apps/web/components/task-create-dialog-submit.tsx` supplies
`useTaskSubmitHandlers`. Its two edit handlers initialize `closeDialog = true`.
On rejection they ask `shouldKeepEditDialogOpen`, which recognizes selected
typed errors and otherwise returns the branch-policy refresh result. An ordinary
PATCH rejection is not stale policy, so it falls through to false and closes in
`finally` despite showing a failure toast.

`useDialogFormState` in `task-create-dialog-state.ts` invokes the real
`useFormResetEffects` in `task-create-dialog-reset-effects.ts`. Reopening advances
`openCycle`, runs `resetTaskForm`, and resolves defaults from `initialValues`.
`DialogPromptSection` in `task-create-dialog-form-body.tsx` keys `TaskFormInputs`
by that cycle, so the instructions input remounts with confirmed defaults.
Retaining the open cycle avoids this causal reset without changing its valid
reopen behavior.

## Submission lifecycle

At the catch boundary of both `handleEditSubmit` and
`handleUpdateWithoutAgent`, mark the edit as remaining open for every caught
failure. Set that outcome before awaiting the existing stale-policy refresh;
call the existing `refreshStaleBranchPolicies` so policy recovery still runs,
then show the existing `taskSubmitErrorMessage` failure toast. `finally` always
clears `isCreatingTask`; success keeps its existing close and callback behavior.

The unused `shouldKeepEditDialogOpen` and
`isRepositorySelectionError` predicates are removed. Keep the
repository message map, typed failure wrappers, and all save sequencing intact.
Do not alter create/session handlers, null-result/validation paths, reset hooks,
editor keys, setter normalization, or draft storage. No new copy is needed.

## Confirmed stages

`performTaskUpdate` keeps its current sequence: optional runner switch, task
fields, dependency replacement. A runner switch advances its confirmed baseline
and is not rolled back after a later failure. A dependency failure after a
successful PATCH retains the existing reconciliation to acknowledged task fields
through `saveEditedTaskDependencies`. The ordinary failure contract applies
before that acknowledgement, not to a draft intentionally reconciled after a
committed stage. `LaunchAfterTaskUpdateError` keeps its saved-task/failed-launch
feedback. `onSuccess` and any actual consumer hydration remain success-only.

`updateTask` in `lib/api/domains/kanban-api.ts` calls the production `fetchJson`
with `PATCH /api/v1/tasks/:taskId`; `lib/api/client.ts` creates `ApiError` for
non-success responses and exposes transport rejections. No retry is automatic:
the user corrects or resubmits the current draft. A network failure remains an
unacknowledged outcome, not evidence that the server rolled back.

## Consumers and responsive behavior

`useSubmitHandlersWiring` in `task-create-dialog-setup.ts` passes actual form
state, the description handle, editing target, callbacks, and stale-policy
refresh to this hook. `TaskCreateDialogContent` binds `guardedHandleSubmit` to
the form. The submit hook routes started edits through update-only; the footer
also exposes update-only as the edit split-button alternative.

`TaskActionsMenuEditDialog` in `task/task-actions-menu-dialogs.tsx` and
`SidebarTaskEditDialog` in `task/task-session-sidebar-edit.tsx` supply confirmed
task data and control openness. The sidebar clears its edit target when
`onOpenChange(false)` arrives. `TaskSwitcherDialogs` in
`task/mobile/session-task-switcher-sheet-dialogs.tsx` mounts the same sidebar
editor for touch entry. Kanban also uses the shared dialog. This correction
retains these consumers and their shared submission state.

The existing dialog is a phone full-height surface and an inset desktop dialog.
Its fields, scroll ownership, controls, focus handling, and viewport branches do
not change. `/mobile-parity`'s purely state/data exception applies: targeted
rendered lifecycle integration plus the above real consumer wiring audit can
prove this shared outcome. No new Playwright coverage or browser geometry claim
is made. A discovered structural change requires ROOT disposition and revised
coverage before implementation expands.

## Regression boundary

`components/task-create-dialog-save-retry.test.tsx` independently renders the
real `SidebarTaskEditDialog` with
real `TaskCreateDialog`, providers, form state/reset, inputs, submit handlers,
and `updateTask/fetchJson`, using a controlled fetch transport and seeded
`createAppStore`. Render the actual controls and error feedback; do not replace
the save hook, reset hook, API method, state provider, toast provider, or setters.
Existing mocked shell tests are compatibility evidence, not causal proof.

Happy DOM does not render CSS animations, so the fixture supplies and restores
an empty `Element.getAnimations()` timeline for dialog cleanup. It delegates
the real window timer scheduler unchanged, tracks its handles, and clears them
after unmount. No production module is mocked; fetch responses are the external
boundary. The full editor integration proves shared state behavior rather than
browser geometry.

Exercise both actual edit variants, exact raw values, busy cleanup, unchanged
confirmed data, and retry acknowledgement. Keep separate title and instructions
cases so a first title failure cannot imply independently verified body loss.
Use the pending-task form for ordinary submit, the started-task Update control
for update-only, and the pending edit split-button alternative where needed to
show editable instructions through update-only. Mock only external transports;
if real consumer mounting needs an unknown alternate harness, checkpoint ROOT
before changing the proof boundary.

The work order owns exact test names, caps, controls, and cleanup. Parent evidence
is read-only contextual evidence; it is not imported or rerun as permanent RED.
