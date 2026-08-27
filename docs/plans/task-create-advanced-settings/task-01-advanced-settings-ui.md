---
id: "01-advanced-settings-ui"
title: "Add the advanced settings disclosure"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/tasks/requirements/task-dependencies-create-dialog-advanced-settings.md"
---

# Task 01: Add the advanced settings disclosure

Implement the compact collapsed disclosure around the existing task-create
dependency selector. Preserve the current workflow behavior, dependency state,
selector semantics, and payload path.

## Acceptance

- A create-mode, unstarted task dialog shows a localized, muted, very small
  `Advanced settings` trigger at the bottom of the form, below the model,
  executor, and workflow selector controls, and the trigger is collapsed by
  default.
- The trigger is a semantic collapsible control with `aria-expanded`, a stable
  test identity, and a mobile hitbox of at least 44 CSS pixels.
- The dependency selector is absent while collapsed and appears inside the
  expanded content with the existing `No dependency` default, icon, picker,
  task rows, info help, and multi-selection behavior. The expanded content uses
  a two-column desktop option grid, with the localized `Depends on` label and
  contextual help to the left of the selector in the same column. The grid
  collapses to one column on narrow screens.
- Collapsing and reopening the section does not clear selected predecessor IDs.
- Workflow visibility, workflow locking, agent/executor behavior, and the
  existing `blockedBy` to `blocked_by` payload path remain unchanged.
- Session mode, edit mode, and started-task forms do not gain the disclosure.
- New copy is localized in the task namespace, and English plus pseudo catalogs
  pass the repository checks.
- Component tests cover collapsed and expanded rendering, semantic state,
  touch-safe sizing, selector mounting, and selection persistence.

## TDD sequence

1. Add failing component tests for collapsed state, expansion, and persistence.
2. Add the disclosure component and wire it into the existing dependency area.
3. Refactor workflow rendering only as needed to keep its behavior independent
   from the dependency presentation.
4. Add localized copy, run focused tests, then clean up styling and test IDs.

## Verification

```bash
cd apps
pnpm install --frozen-lockfile
pnpm --filter @kandev/web test -- task-create-dialog-advanced-settings task-create-dialog-form-body task-create-dialog-dependencies task-create-dialog-helpers
pnpm --filter @kandev/web run typecheck
pnpm --filter @kandev/web exec eslint --max-warnings 0 components/task-create-dialog-advanced-settings.tsx components/task-create-dialog-advanced-settings.test.tsx components/task-create-dialog-form-body.tsx components/task-create-dialog-form-body.test.tsx components/task-create-dialog.tsx
pnpm --filter @kandev/web run i18n:check
pnpm --filter @kandev/web run i18n:ratchet
git diff --check
```

## Files likely touched

- `apps/web/components/task-create-dialog-advanced-settings.tsx`
- `apps/web/components/task-create-dialog-advanced-settings.test.tsx`
- `apps/web/components/task-create-dialog-form-body.tsx`
- `apps/web/components/task-create-dialog-form-body.test.tsx`
- `apps/web/components/task-create-dialog.tsx`
- `apps/web/src/locales/en/task.json`
- `apps/web/src/locales/pseudo/task.json`

## Dependencies and risks

None. This task depends on the implemented dependency selector and should not
change its API. The main risk is accidentally moving or hiding the workflow
selector while extracting the old paired row, so the workflow visibility tests
must remain explicit.

## Parallelism

`sequential`. The disclosure placement and component contract define the E2E
selectors used by Task 02.

## Inputs

- Spec: `docs/specs/tasks/requirements/task-dependencies-create-dialog-advanced-settings.md`
- Plan: `docs/plans/task-create-advanced-settings/plan.md`
- Existing dependency behavior in
  `apps/web/components/task-create-dialog-dependencies.tsx`
- Existing workflow rendering in
  `apps/web/components/task-create-dialog-form-body.tsx`
- Existing mobile picker and touch-target expectations in the implemented
  dependency selector tests

## Output contract

Report the component and dialog files changed, focused test results, i18n and
typecheck results, any placement or accessibility risks, and the updated task
and plan status before starting Task 02.

## Results

- Added `TaskCreateAdvancedSettings` as an inline, collapsed disclosure below
  the model, executor, and workflow controls. It keeps the dependency selector
  inside an extensible content region, labels the setting with contextual help,
  and preserves `blockedBy` state and disabled behavior.
- Kept workflow rendering independent and unchanged for its existing visibility,
  locking, and single-workflow rules.
- Added localized English and pseudo-locale copy plus component coverage for
  collapsed and expanded semantics, the mobile-safe hitbox, dependency
  mounting, state persistence, and unavailable modes.
- Focused component tests passed: 40 tests. Typecheck, focused ESLint, i18n
  checks, and `git diff --check` passed. No remaining placement or accessibility
  risk was found after the managed mobile run validated the 44 CSS pixel target.
