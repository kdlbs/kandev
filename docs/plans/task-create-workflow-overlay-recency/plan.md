---
created: 2026-09-05
status: implemented
requirements:
  - REQ-TASKS-TASK-CREATE-WORKFLOW-MEMORY-001
system_design: []
legacy_specs: []
---

# Implementation Plan: Task Create Workflow Overlay Recency

## Overview

Correct the frontend task-create queued overlay so the latest successful
workflow selection wins when several task creations occur in one workspace
before backend settings publication clears the overlay. One focused work order
adds the failing regression first, reverses the conflicting map precedence, and
verifies the unchanged task-create dialog on its existing desktop and mobile
paths.

The confirmed defect violates
`AC-TASKS-TASK-CREATE-WORKFLOW-MEMORY-001.1`: the backend records the latest
workflow, but the frontend can continue to present an older queued workflow.
No requirement or product behavior change is needed.

There is no dedicated current system-design document for this migrated
requirement. The technical boundaries remain those in the completed
[Task Create Workflow Memory plan](../task-create-workflow-memory/plan.md),
[ADR 0028](../../decisions/0028-task-create-last-used-source-of-truth.md),
[ADR 0041](../../decisions/0041-backend-owned-portable-user-settings.md), and
[the workspace-scoped workflow-memory ADR](../../decisions/2026-08-08-workspace-scoped-task-create-workflow-memory.md).

## Scope

### In scope

- Preserve queued workflow entries for other workspaces across successful task
  creations.
- Let the newest successful submission replace an older queued workflow for
  the same workspace.
- Add deterministic regression coverage for consecutive same-workspace
  submissions before settings convergence.
- Keep the existing shared desktop and mobile workflow-resolution behavior.

### Out of scope

- Backend persistence or database changes.
- Changes to workflow-resolution precedence, selector layout, or option order.
- Persisting cancelled or failed selector changes.
- Changes to task-create repository, branch, agent, or executor defaults.

## Technical approach

Update `queueTaskCreateLastUsedFromPayload` in
`apps/web/components/task-create-dialog-handlers.ts`. Preserve the queued
`workflowIdsByWorkspace` history as the merge base, then apply the successful
payload as the newer patch. `mergeTaskCreateLastUsedState` already implements
patch-wins semantics, so no new abstraction is required.

Keep `queueTaskCreateLastUsedFromPayload` replacing the queued scalar defaults
with values from the latest successful task. Do not change the existing absent
or null workflow behavior: only a payload with both workspace and workflow IDs
adds or replaces a workflow history entry.

Add a regression to
`apps/web/components/task-create-dialog-handlers.test.ts` that queues workflow
one and then workflow two for the same workspace and expects workflow two.
Retain the existing different-workspace test to prove both history entries are
preserved.

This is shared state normalization inside the existing responsive dialog. It
does not change composition, navigation, scrolling, focus, safe areas, or touch
targets. The existing Create Task dialog remains the desktop and mobile
surface, and no mobile-specific production branch is introduced.

## Tests

- `AC-TASKS-TASK-CREATE-WORKFLOW-MEMORY-001.1`: a focused unit regression in
  `apps/web/components/task-create-dialog-handlers.test.ts` proves that the
  latest successful workflow wins for a repeated workspace key.
- Existing handler tests continue to prove that queued workflow history for
  different workspaces and unrelated scalar defaults is preserved.
- Existing settings-overlay tests continue to prove queued values bridge
  backend publication and clear after convergence.

## E2E tests

- `AC-TASKS-TASK-CREATE-WORKFLOW-MEMORY-001.1`: run the existing
  `apps/web/e2e/tests/task/create-task.spec.ts` remembered-workflow scenario to
  verify that the production-built task-create dialog still restores workflow
  memory over a conflicting board filter.
- No new mobile Playwright scenario is required because the correction is
  viewport-independent state normalization in unchanged shared dialog code.
  Existing mobile task-create coverage continues to exercise the same surface.

## Work orders

- [x] [Task 01: Preserve latest queued workflow](task-01-preserve-latest-queued-workflow.md)

## Verification results

- RED: The focused regression expected workflow two but received workflow one.
- GREEN: The focused regression passed after the merge-order correction.
- The focused Vitest command passed 3 files and 37 tests.
- `pnpm run typecheck` passed.
- The production-build remembered-workflow E2E passed 1 test with retries disabled.
- The managed E2E runner removed its isolated environment.

## Risks

- Reversing precedence incorrectly can preserve the latest workspace entry
  while dropping queued entries for other workspaces.
- Replacing the whole overlay rather than only changing map precedence can
  regress repository, branch, agent-profile, or executor-profile restoration.
- Browser settings publication can occur before or after the create response.
  The in-memory overlay must remain valid for either ordering.
