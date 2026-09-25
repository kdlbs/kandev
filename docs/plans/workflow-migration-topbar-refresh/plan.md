---
created: 2026-09-25
status: complete
requirements:
  - REQ-TASKS-CHANGE-WORKFLOW-001
system_design:
  - ../../specs/tasks/system-design/change-workflow.md
legacy_specs: []
---

# Workflow migration top bar refresh

## Overview

An open task moved from Kanban/Review to Feature/Analysis can continue to show
Kanban/Review in its desktop top bar until a browser reload. Make the open task
follow the destination workflow event and load that workflow's steps. The
existing AC-TASKS-CHANGE-WORKFLOW-001.8 already requires this behavior, so this
package repairs its implementation without adding another product requirement.

## Root cause and reproduction

The `task.updated` handler removes the task from the source `kanban.tasks` and
inserts it into `kanbanMulti.snapshots[destination]`. `useTaskDetails` reads only
`kanban.tasks`; `resolveEffectiveTask` then retains the old local
`workflow_id`. `useWorkflowStepsMapped` reads `kanban.steps`, so the top bar
continues to render the source workflow. A full reload obtains destination
task details and loads its steps, which explains the reported recovery.

The smallest regression is a task detail page open on Kanban/Review: migrate
that task to Feature/Analysis through the existing Change workflow form, keep
the page open, and observe that the stepper retains Kanban/Review even after the
backend task reports Feature/Analysis. Existing change-workflow E2E checks the
backend result but does not assert the open page's stepper before reload.

## Scope

### In scope

- Reconcile the open task's workflow ID and step ID from its live destination
  projection while preserving full task detail fields.
- Load ordered steps for the task's resolved workflow and render its current
  step in the desktop top bar without changing the board workflow selection.
- Cover a destination with no cached step definitions, delayed HTTP snapshots,
  a stale source-board step, and another client's move while the task page is open.
- Preserve the existing phone migration action and task placement behavior.

### Out of scope

- Backend move semantics, workflow definitions, agent choices, and session
  routing.
- A new phone stepper or a change to the workflow-change form layout.

## Technical approach

In `task-page-content.tsx`, resolve the live task from the active workflow or
its owning `kanbanMulti` snapshot by task ID. Extend
`resolveEffectiveTask` in `task-page-content-helpers.ts` so a newer live
placement updates workflow and step together, while older HTTP task details or
snapshots cannot revert it. Keep title, repository, and other detailed fields
from the task detail response where the board projection omits them.

Have the task page request/consume step definitions for the resolved task
workflow directly by ID, even if it is absent from `workflows.items`. Fetch only
the task's workflow instead of loading all workspace snapshots from the task
page. Use the existing workflow-aware step selector or equivalent shared
mapping instead of the unscoped `kanban.steps`. In `task-page-inner.tsx`, use
the freshness-resolved task step first and use a current-workflow cache step
only when task details omit the step. Do not change the selected workflow of a
separate board tab.

## ASCII UI preview

### UI-01: Desktop open task after migration

Current observed state after backend success, before reload:

```text
Task: Investigate binary size increase   [Backlog] - [In Progress] - [Review*] - [Done]
                                        (old Kanban steps remain)
```

Required state on the same open page:

```text
Task: Investigate binary size increase   [Backlog] - [Analysis*] - [Implement] - [Review] - [PR]
                                        (Feature steps; Analysis is current)
```

`*` marks the current step. Step names are illustrative; the destination
workflow supplies the actual ordered labels. The desktop top bar keeps its
existing layout and controls. AC-TASKS-CHANGE-WORKFLOW-001.8 owns the change.

### UI-02: Phone task action and destination placement

```text
Task actions > Change workflow... > select Feature / Analysis > Change workflow
Task remains open; task lists and subsequent actions use Feature / Analysis.
```

The phone task page has no desktop stepper. Its action entry, form, and layout
remain as shipped; the shared task projection changes underneath them. Existing
phone E2E checks the form, and the targeted extension checks visible destination
placement without reloading. No new copy or control geometry is required.

## Tests

| Acceptance                     | Evidence                                                                                                                                                                                                                                                  |
| ------------------------------ | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| AC-TASKS-CHANGE-WORKFLOW-001.8 | `task-page-content-workflow.test.ts`: newer destination placement wins; older and partial updates cannot restore source placement; task-only details remain; the freshest projection wins; fresh task details win over an older same-workflow step cache. |
| AC-TASKS-CHANGE-WORKFLOW-001.8 | `use-workflow-steps-by-id.test.ts`: a placeholder destination snapshot updates to its ordered workflow steps when they load.                                                                                                                              |
| AC-TASKS-CHANGE-WORKFLOW-001.8 | `use-all-workflow-snapshots.test.ts`: a task workflow absent from the catalog is fetched by ID, with only that snapshot loaded and board selection preserved.                                                                                             |

The first helper assertion for destination workflow ID should fail against the
current implementation before the fix.

## E2E tests

| File / project                                                | Scenario                                                                                                                                                           |
| ------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `tests/task/change-workflow.spec.ts` / `chromium`             | Open task page, migrate from its action, assert destination step names and current marker before any page reload; verify backend placement and unchanged task URL. |
| `tests/task/change-workflow.spec.ts` / `chromium`             | Move a task through the API while its task page remains open; assert the receiving page switches to destination steps and current marker without navigation.       |
| `tests/task/mobile-change-workflow.spec.ts` / `mobile-chrome` | Complete the existing phone flow and assert the destination workflow/step in the visible task surface before reload.                                               |

## Work orders

- [x] [Task 01: Reconcile open task workflow placement](task-01-open-task-workflow-placement.md) (done)

One focused frontend slice covers the live projection, step definitions, and
desktop/phone regression evidence. Implementation was completed sequentially in
this session.

## Verification results

- Focused unit tests: 7 files, 108 tests passed.
- TypeScript typecheck passed.
- Scoped ESLint passed with no warnings.
- Production Vite build passed. Vite reported existing chunk-size, deprecated
  option, and ineffective dynamic import warnings.
- Desktop Chromium change-workflow E2E: 4 tests passed, including an external
  move event received by an open task page.
- Phone mobile-chrome change-workflow E2E: 1 test passed.
- `CAPTURE_PR_ASSETS=1 pnpm e2e:run --host --no-build --project chromium e2e/tests/task/change-workflow.spec.ts`: 4 passed; captured the destination stepper on the open task page.
- `CAPTURE_PR_ASSETS=1 pnpm e2e:run --host --no-build --project mobile-chrome e2e/tests/task/mobile-change-workflow.spec.ts`: 1 passed; captured the destination step in phone task actions.
- Specification catalog and specification lint passed.
- `git diff --check` passed.

## Risks

- HTTP snapshot responses can race a newer live move; accepting the old
  response would briefly restore the old workflow.
- A destination placeholder has a task row but no step definitions until its
  workflow snapshot loads.
- The source-board step fallback can outlive the task's source placement during
  event reconciliation.
