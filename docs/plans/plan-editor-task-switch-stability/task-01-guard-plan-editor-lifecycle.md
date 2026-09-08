---
id: "01-guard-plan-editor-lifecycle"
title: "Guard the plan editor lifecycle"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/plan-editor-task-switch-stability.md"
---

# Task 01: Guard the Plan Editor Lifecycle

## Acceptance

- A task switch cannot expose an editor that belongs to the outgoing task.
- Plan search does not access a null or destroyed editor.
- Plan-comment commands do not access an editor from the outgoing task.
- Plan search works after the selected task editor becomes available.
- A sidebar task switch with the Plan panel open does not show route recovery.

## Verification

1. If dependencies are absent, run `cd apps && pnpm install --frozen-lockfile`.
2. RED: Run the focused editor-lifecycle test before production changes.
3. Unit: Run `cd apps && pnpm --filter @kandev/web test -- components/task/use-plan-find-shortcut.test.tsx components/task/task-plan-panel.session-switch.test.tsx`.
4. Type check: Run `cd apps/web && pnpm run typecheck`.
5. Lint: Run `cd apps && pnpm --filter @kandev/web lint`.
6. Internationalization: Run `cd apps/web && pnpm run i18n:check`.
7. Desktop E2E: Run `cd apps/web && pnpm e2e:run tests/search/plan-search.spec.ts -- --grep "switches sidebar tasks" --retries=0`.
8. Repository check: Run `git diff --check` from the repository root.

Verify that Playwright discovers the expected test before using the browser run
as evidence. The managed runner builds and removes its isolated processes.

## Files likely touched

- `apps/web/components/task/use-plan-find-shortcut.ts`
- `apps/web/components/task/use-plan-find-shortcut.test.tsx`
- `apps/web/components/task/task-plan-panel.tsx`
- `apps/web/components/task/task-plan-panel.session-switch.test.tsx`
- `apps/web/e2e/tests/search/plan-search.spec.ts`

## Dependencies

None.

## Parallelism

Sequential. Editor ownership, command guards, and regression coverage form one
TDD slice.

## Inputs

- Behavioral contract: `docs/specs/ui/requirements/plan-editor-task-switch-stability.md`.
- System design: `docs/specs/ui/system-design/plan-editor-task-switch-stability.md`.
- Root-cause trace and implementation plan: `plan.md`.
- Plan search hook: `apps/web/components/task/use-plan-find-shortcut.ts`.
- Plan panel state: `apps/web/components/task/task-plan-panel.tsx`.
- Existing browser coverage: `apps/web/e2e/tests/search/plan-search.spec.ts`.

## Output contract

Report the RED result, implementation summary, changed files, exact test
results, cleanup evidence, blockers, and risks. Update this task and `plan.md`.

## Results

- RED: The two editor-lifecycle tests failed because the original search hook
  read the `commands` getter after the editor was destroyed.
- GREEN: The focused Vitest run passed 2 files and 5 tests after task-scoped
  editor ownership and destroyed-editor guards were added.
- Browser regression: The managed Playwright run passed the sidebar task-switch
  scenario in 15.0 seconds. The route remained loaded, the selected Plan
  content changed, and plan search highlighted matches after the switch.
- Static checks: Frontend typecheck, ESLint, i18n validation, specification
  lint, and `git diff --check` passed.
- Evidence: The Playwright run captured
  `plan-search--plan-task-switch-stable.png` from synthetic E2E data for the PR.
- Cleanup: The managed E2E runner completed successfully and removed its
  isolated runtime.
- Residual risk: The regression uses the desktop layout because the failure is
  in shared editor lifecycle code. Mobile composition and interaction behavior
  are unchanged.
