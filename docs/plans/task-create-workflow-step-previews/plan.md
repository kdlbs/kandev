---
created: 2026-09-29
status: implemented
requirements:
  - REQ-TASKS-CREATE-WORKFLOW-STEPS-001
system_design:
  - ../../specs/tasks/system-design/task-create-workflow-step-previews.md
legacy_specs: []
---

# Implementation Plan: Task creation workflow step previews

## Overview

Make every workflow option show its steps independently of board navigation.
One sequential work order delivered the loader, UI states, and regression tests.
Implementation is complete under Task 01.

Tasks owns this contract because it owns task creation and workflow definitions.
The existing launch-preview requirement covers the selected destination, not
all option previews. This package adds the missing behavior as a separate,
cohesive task-creation contract. It does not amend launch routing.

## Evidence and root cause

The supplied screenshot shows Feature selected with Analysis beside the
selector, but only Kanban has an option preview. Source inspection identifies
these independent paths:

- `workflow-selector-row.tsx` reads only `snapshots[wf.id].steps` for options.
- `useTaskCreateDialogData` subscribes to `kanbanMulti.snapshots` without
  loading missing snapshots.
- `useWorkflowStepsEffect` fetches a selected non-context workflow into
  `fetchedSteps`. `resolveDialogLaunchPreview` uses those steps, but the
  dropdown does not receive them.
- Task pages call `useWorkflowSnapshotById` for the current task. The board
  calls `useAllWorkflowSnapshots` for the workspace.

This is a missing loading dependency. It can persist after network activity
settles. The exact browser cache at screenshot time was not captured.

Smallest regression: provide Feature, Review, and Kanban options with only a
Kanban snapshot. Open the selector and resolve step API responses for all three.
The current component issues no preview requests and leaves two rows blank.
The implementation must first demonstrate this failure in the component test.

## Scope

### In scope

- Step-only loading for all visible task-create options on selector open.
- Independent loading, empty, failure, and retry states.
- Request isolation across close, reopen, workspace changes, and membership changes.
- Metadata preservation, localization, and contained desktop/phone rendering.

### Out of scope

- Backend changes, task snapshot fetching, persistent caches, and runtime flags.
- Changes to launch destination, prompt composition, selection memory, or task creation gates.
- General workflow editing, automation redesign, or global selector refactors.

## Technical approach

The implementation follows the [system design](../../specs/tasks/system-design/task-create-workflow-step-previews.md).
`useWorkflowOptionPreviews` lives in
`apps/web/hooks/use-workflow-option-previews.ts`. `WorkflowSelectorRow` uses it
behind the optional `previewWorkspaceId` prop, which flows from `DialogFormBody`
through `WorkflowSection`. Automation keeps snapshot rendering when the prop is
absent.

Use the existing step API and local state with generation guards. Do not write
synthetic snapshots: a step-only result must not look like a complete task list.
No new library, endpoint, database migration, or public contract is required.

## ASCII UI preview

UI-01: Desktop, Create Task > workflow selector. Current source-backed defect:

```text
Workflow
  Feature                         [selected]
  Contributor PR Review
  Kanban
    Backlog > In Progress* > Review > Done
```

UI-01: Implemented loaded and partial-failure states:

```text
Workflow
  Feature                         [selected]
    Analysis > Implement > Review > Done
  Contributor PR Review
    Loading steps...
  Kanban
    Backlog > In Progress* > Review > Done

  Failed workflow
    Could not load steps.         [Retry]
  Empty workflow
    No steps in this workflow
```

UI-02: Phone, same entry point and states, contained click picker:

```text
+-------------------------------+
| Workflow                      |
| Feature              [check]  |
|   Analysis > Implement >      |
|   Review > Done               |
| Contributor PR Review         |
|   Could not load steps.       |
|   [Retry]                     |
| Kanban                        |
|   Backlog > In Progress* >    |
|   Review > Done               |
+-------------------------------+
```

The list scrolls vertically inside the viewport-bound popover. Step groups
wrap in order. Selection and retry remain separate hit targets. The selector
returns focus to its trigger when dismissed. These structural choices are
required; spacing, example step names, and copy are illustrative. All product
copy is localized. Views map to AC-001.1 through AC-001.6, using the full
`AC-TASKS-CREATE-WORKFLOW-STEPS` prefix.

## Tests

All files below are under `apps/web/`; named tests verify the acceptance criteria.

| Acceptance criteria | Test file and named case                                                                                                                                                                                                                                                                                          |
| ------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 001.1, 001.5        | `components/workflow-selector-row.test.tsx`: `loads every option with only one cached snapshot` (initial red regression)                                                                                                                                                                                          |
| 001.2               | `hooks/use-workflow-option-previews.test.ts`: `distinguishes loading from successful empty results and orders step metadata`                                                                                                                                                                                      |
| 001.3               | Same hook suite: `retains successful rows when one request fails and retries only that row`; selector suite: `shows independent loading and failure states and retries without selecting`                                                                                                                         |
| 001.4               | Same hook suite: `masks old scopes synchronously and ignores their late responses`; `refreshes removed steps when the selector reopens`; `ignores results for workflows removed from the open selector`                                                                                                           |
| 001.1, 001.4        | Same hook suite: `does not duplicate requests on ordinary rerenders`                                                                                                                                                                                                                                              |
| 001.3, 001.5, 001.6 | Selector suite: `shows independent loading and failure states and retries without selecting` (one translated status region, stable option name, and keyboard retry focus); desktop and phone E2E verify the same announcement and retry behavior                                                                  |
| 001.5               | Selector suite: `preserves loaded step order, colors, start markers, and agent badges`; `retains snapshot rendering for selectors without task-create scope`; form-body suite: `passes the workspace and visible workflows while retaining locked behavior`; desktop E2E verifies the task draft and launch label |

Use deferred promises to control response order. Include one mixed fixture
containing successful, pending, failed, and empty rows. Verify the loader never
writes board state or requests full task snapshots. Existing launch-preview
and form-body tests remain required regression guards.

## E2E tests

`e2e/tests/task/task-create-workflow-step-previews.spec.ts` covers `chromium`:
seed three workflows and a task in Kanban, then enter `/t/:taskId` directly.
Open Create Task through its visible entry point, then open the selector.
Verify every workflow's step names and order before selecting Feature.
Verify selection, the Analysis launch label, and task draft preservation.
Cover one failed step request followed by keyboard row retry, with another row
successful. Keep the response pending to verify Retry retains focus and its
name, then returns focus to the workflow option after success. Verify the
single translated status region identifies the loading row. Check that an
unbroken step name stays within its group and retry sizing matches wide and
narrow fine-pointer controls. These scenarios cover AC-001.1, 001.2, 001.3,
001.5, and 001.6.

`e2e/tests/task/mobile-task-create-workflow-step-previews.spec.ts` covers
`mobile-chrome`. Use the same direct task route, tap the visible creation entry,
and verify all previews and retry. Include long step names, viewport containment,
internal scrolling, one translated status region, and 44px action targets
(AC-001.6).
The desktop test also verifies that an unbroken step name remains inside its
preview group. It measures the retry control at 28px on a wide fine-pointer
viewport and at the standard narrow fine-pointer size. The phone test checks a
computed 48px retry minimum and a rendered target of at least 44px.

Do not visit the board to prepare the browser cache. Seed through fixture APIs.
Use causal response waits or controlled route interception, not fixed sleeps.
Run existing desktop and mobile launch-preview suites as compatibility checks.

## Work orders

- [x] [Task 01: Load and render complete workflow previews](task-01-workflow-previews.md)

## Verification results

Implementation checks passed on 2026-09-29:

- Focused Vitest suite: 44 tests passed; frontend typecheck and targeted ESLint passed.
- `pnpm run i18n:zh-hant` and `pnpm run i18n:check` passed. The workflow/status announcement uses a new key translated in all supported locales.
- Desktop E2E (`chromium`, preview and task-creation suites): 19 tests passed.
- Phone E2E (`mobile-chrome`, preview and launch-preview suites): 2 tests passed.
- Backend build and managed E2E Vite builds passed.
- Public docs tests: 62 passed; public docs validation: 47 pages.
- Spec catalog validation: 327 decisions and 1245 specifications; full spec lint passed.
- `git diff --check` passed.

Review remediation completed on 2026-09-29:

- The browser regression first reproduced clipping: an unbroken step-name box
  extended to x=579px while its preview group ended at x=449px. After adding
  shrinkable, anywhere-wrapping text, the step stays within the group at 640px.
- Retry sizing preserves the 28px standard control at a 1280px fine-pointer
  viewport and the existing 44px narrow viewport size. Coarse-pointer styling
  supplies a computed 48px minimum while retaining at least a 44px hit target.
- The focused desktop Chromium workflow-preview test passed (1 test); the mobile
  workflow-preview and launch-preview tests passed (2 tests). The focused Vitest
  suites passed (44 tests), frontend typecheck and targeted ESLint passed, and
  managed backend/Vite builds passed.
- Accessibility review remediation adds one translated status region, keeps the
  option name fixed to the workflow name, and keeps keyboard focus on Retry
  while a retry is pending. Successful retry returns focus to the workflow
  option; another failure retains Retry focus. A pending-route desktop E2E and
  the phone E2E cover these behaviors.

PR fixup completed on 2026-09-29:

- The unbroken step assertion now checks every rendered text line against its
  preview group at a 640px viewport. Retry sizing is verified at 28px on a wide
  fine-pointer desktop, at 44px in the narrow fine-pointer layout, and with a
  48px minimum and 44px hit target on a coarse-pointer phone.
- The phone run exposed a focus race when the Tasks drawer closed after opening
  Create Task. Its delayed focus return could move focus to the task-picker
  trigger and dismiss the workflow picker. Drawer close now leaves focus in the
  active task dialog, which owns the pending focus transition. The selector
  popover also uses the task dialog's portal container.
- Final verification passed: 52 focused Vitest tests, desktop preview E2E (1),
  phone preview E2E (3 consecutive repeats), typecheck, E2E Vite build, targeted
  ESLint, and `git diff --check`.

## Risks

- Unstable hook dependencies can repeatedly fetch all options; test rerenders.
- A late result can leak stale scope data unless rendering and commits both check scope.
- Shared selector changes can affect automations; retain and test its fallback.
- Long step lists can overflow the existing unbounded popover; test geometry.
- Unbroken step names need anywhere wrapping and a containment check against the
  workflow's own step group.
- Loading is proportional to visible workflows. Fetch only while open, once per
  workflow per cycle; avoid polling and full task payloads.

## Documentation impact

The public task guide now explains how to compare workflow steps and retry a
failed preview. No implementation details or unrelated screenshots were added.
