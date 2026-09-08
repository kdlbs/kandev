---
id: "01-sidebar-completion-icons"
title: "Differentiate sidebar completion icons"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/sidebar-task-completion-icons.md"
---

# Task 01: Differentiate Sidebar Completion Icons

## Acceptance

- Finished tasks on non-final, unknown, or unmatched workflow steps render the green Tabler
  `progress-check`; finished tasks on the final ordered step render the existing green
  `circle-check`.
- The desktop sidebar and mobile task-switcher drawer use the same derivation and selectors,
  while every higher-priority active or pending icon remains unchanged.
- Focused component, desktop Playwright, mobile Playwright, and frontend typecheck commands pass.

## TDD Sequence

1. RED: add component cases proving non-final, final, and missing-step behavior plus switcher
   derivation from the task's own workflow.
2. RED: add desktop and `mobile-chrome` Playwright scenarios that distinguish the two rendered
   icons in real task rows.
3. GREEN: thread final-step membership through the shared task switcher and render
   `IconProgressCheck` or `IconCircleCheck` in the existing review branch.
4. REFACTOR: keep the final-step comparison local and readable, migrate generic review-icon
   selectors, and preserve the current state-precedence ordering.

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

## Files likely touched

- `apps/web/components/task/task-switcher.tsx`
- `apps/web/components/task/task-switcher.test.tsx`
- `apps/web/components/task/task-item.tsx`
- `apps/web/components/task/task-item.test.tsx`
- `apps/web/e2e/tests/task/sidebar-settled-spinner.spec.ts`
- `apps/web/e2e/tests/task/sidebar-workflow-completion-icon.spec.ts`
- `apps/web/e2e/tests/task/mobile-sidebar-workflow-completion-icon.spec.ts`
- `docs/specs/ui/requirements/sidebar-task-completion-icons.md`
- `docs/specs/INDEX.md`
- `docs/plans/sidebar-task-completion-icons/plan.md`
- `docs/plans/sidebar-task-completion-icons/task-01-sidebar-completion-icons.md`

## Dependencies

None. The shared sidebar already receives ordered steps for every loaded workflow.

## Parallelism

Sequential. Production rendering, shared desktop/mobile behavior, and selector migration touch
the same components and tests.

## Inputs

- `docs/specs/ui/requirements/sidebar-task-completion-icons.md`, especially the exact-match fallback and state
  precedence scenarios.
- `docs/plans/sidebar-task-completion-icons/plan.md`.
- Existing status precedence in `apps/web/components/task/task-item.tsx`.
- Existing per-workflow ordering in
  `apps/web/components/task/task-session-sidebar-aggregate.ts`.
- Mobile exemplar `apps/web/components/task/mobile/session-task-switcher-sheet.tsx`.

## Risks

- Do not infer completion from `TaskState.COMPLETED` alone; workflow completion is defined by
  current-step membership.
- Do not let a missing step list render the continuous workflow-complete check.
- Keep the shared row path so desktop and mobile cannot drift.

## Verification Results

- Component tests: 40 passed.
- Desktop completion-icon E2E: passed.
- Mobile completion-icon E2E: passed.
- Sidebar status regression E2E: passed.
- Frontend lint and typecheck: passed.

## Output contract

Report RED failures, the final-step derivation, selectors migrated, exact files changed,
component/E2E/typecheck results, blockers or risks, and the task/plan status updates.
