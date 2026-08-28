---
spec: docs/specs/ui/requirements/sidebar-task-completion-icons.md
created: 2026-07-30
status: completed
---

# Implementation Plan: Sidebar Task Completion Icons

## Overview

Use the workflow-step ordering already loaded by the shared task switcher to distinguish a
finished turn from a finished workflow. The change remains frontend-only: derive final-step
membership while rendering each shared row, select the appropriate Tabler icon in `TaskItem`,
and prove the behavior in unit tests and both desktop and phone task-switcher surfaces.

## Frontend

### Shared task-row derivation

- In `apps/web/components/task/task-switcher.tsx`, compare each task's `workflowStepId` with the
  last entry in `stepsByWorkflowId[task.workflowId]`.
- Pass the result into `TaskItem`. An absent workflow, absent step list, empty step list, or
  unmatched step resolves to `false`, so incomplete information never claims workflow
  completion.
- Reuse the ordered step arrays produced by `aggregateSidebarTasks`; no store, API, or snapshot
  shape changes are needed.

### Completion icon rendering

- In `apps/web/components/task/task-item.tsx`, add `IconProgressCheck` from
  `@tabler/icons-react`.
- In the existing `classifyTask(...) === "review"` branch, render the current green
  `IconCircleCheck` only when the task is on its workflow's last step. Otherwise render the
  green `IconProgressCheck`.
- Give the two states stable semantic test selectors and migrate existing sidebar status tests
  from the generic review selector. Keep the established icon size, alignment, and green color.
- Leave the permission, clarification, foreground/background activity, preparing, running, and
  backlog branches unchanged and ahead of completion rendering.

## Tests

- **What:** a finished turn on a non-final or unknown step renders `progress-check`, while the
  same state on the final step renders `circle-check`.
  **Files:** `apps/web/components/task/task-item.test.tsx` and
  `apps/web/components/task/task-switcher.test.tsx`.
  **How:** component tests cover icon selection and verify that the switcher derives final-step
  membership from the task's own workflow steps.
- **What:** active and pending states still override either completion icon.
  **File:** `apps/web/components/task/task-item.test.tsx`.
  **How:** retain and update the existing precedence tests.

## E2E Tests

- **Scenario:** finished tasks on non-final and final steps show the dashed progress check and
  continuous completion check in the desktop sidebar.
  **File:** `apps/web/e2e/tests/task/sidebar-workflow-completion-icon.spec.ts`.
  **What to verify:** seed two `REVIEW` tasks in one workflow, open a task detail route, and
  assert the two semantic icon selectors in their sidebar rows.
- **Scenario:** the same two tasks show the same distinction in the phone task-switcher drawer.
  **File:** `apps/web/e2e/tests/task/mobile-sidebar-workflow-completion-icon.spec.ts`.
  **What to verify:** open the task switcher from the mobile session top bar and assert both row
  icons inside the drawer.
- Update `apps/web/e2e/tests/task/sidebar-settled-spinner.spec.ts` so its existing non-final
  settled-turn assertion expects the progress-check selector.

## Mobile Parity

The nearest shipped mobile exemplar is
`apps/web/components/task/mobile/session-task-switcher-sheet.tsx`. It already renders the shared
`TaskSwitcher` and `TaskItem`, so the phone drawer receives the same state derivation without a
separate presentation or duplicated business rule. This is an icon-only content change: the
existing drawer entry point, hierarchy, tap behavior, touch targets, safe-area handling, and
scroll owner remain unchanged. The focused `mobile-chrome` Playwright scenario is the rendered
mobile check.

## Implementation Waves

Wave 1 (sequential):

- [x] [Task 01: Differentiate sidebar completion icons](task-01-sidebar-completion-icons.md) — done

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- \
  components/task/task-item.test.tsx \
  components/task/task-switcher.test.tsx
cd apps/web && pnpm e2e:run tests/task/sidebar-workflow-completion-icon.spec.ts \
  -- --project=chromium
cd apps/web && pnpm e2e:run tests/task/mobile-sidebar-workflow-completion-icon.spec.ts \
  -- --project=mobile-chrome
cd apps/web && pnpm run typecheck
```

## Risks

- Workflow step arrays must remain ordered; `aggregateSidebarTasks` currently sorts every
  workflow by `position` before passing them to `TaskSwitcher`.
- Missing or temporarily stale workflow metadata must not display a false workflow-complete
  check, hence the exact-match rule and progress-check fallback.
- Existing tests and external selectors may still refer to `task-state-review`; migrate all
  in-repository uses in the same change and keep the new semantic selectors scoped to the two
  completion outcomes.

## Documentation Impact

No public documentation changes are required. This plan adds the internal product spec
`docs/specs/ui/requirements/sidebar-task-completion-icons.md`; the change does not alter commands, settings,
APIs, workflows, or public terminology. No ADR is warranted because the implementation reuses
existing frontend state and establishes no new architectural boundary.
