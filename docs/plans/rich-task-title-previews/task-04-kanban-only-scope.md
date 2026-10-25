---
id: "04-kanban-only-scope"
title: "Scope preview to Kanban parents"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/tasks/rich-task-title-previews.md"
---

# Task 04: Scope preview to Kanban parents

- **Acceptance:** Kanban parent cards with active direct subtasks retain the
  fine-pointer title preview and its existing keyboard/subtask navigation.
- **Acceptance:** Childless Kanban cards, left-sidebar task rows, and `/tasks`
  rich-list rows do not mount or open the task-title preview.
- **Acceptance:** Coarse-pointer Kanban navigation and separate GitHub/GitLab
  contribution-badge hovers remain unchanged.
- **Verification:**
  `cd apps && pnpm --filter @kandev/web test -- --run components/task/task-title-hover-card.test.tsx components/kanban-card-content.test.tsx`
  and
  `cd apps/web && pnpm e2e:run tests/kanban/task-title-hover-subtasks.spec.ts tests/task/task-title-hover-surfaces.spec.ts`
  and
  `cd apps/web && pnpm e2e:run --project mobile-chrome tests/kanban/mobile-task-title-hover-subtasks.spec.ts`.
- **Files touched:** `apps/web/components/task/task-item.tsx`,
  `apps/web/components/task/task-item-content.tsx`,
  `apps/web/app/tasks/rich-task-list-row.tsx`,
  `apps/web/components/task/task-title-hover-card.tsx`,
  `apps/web/components/task/task-title-hover-card.test.tsx`,
  `apps/web/components/kanban-card-content.test.tsx`, and the affected
  desktop/mobile Playwright specifications, and
  `apps/web/e2e/tests/pr/pr-status-badge.spec.ts`.
- **Dependencies:** None. The existing title-preview implementation and task
  hierarchy hooks are available.
- **Parallelism:** sequential.
- **Inputs:** the `What`, `Failure modes`, `Scenarios`, and `Out of scope`
  sections of the spec; the `Scope refinement`, `Tests`, and `E2E Tests`
  sections of `plan.md`; `apps/web/AGENTS.md`; and the `/mobile-parity`
  contract for preserving coarse-pointer direct navigation.

## Results

- `cd apps && pnpm --filter @kandev/web test -- --run components/task/task-title-hover-card.test.tsx components/kanban-card-content.test.tsx` passed: 2 files and 22 tests.
- `cd apps/web && pnpm e2e:run tests/kanban/task-title-hover-subtasks.spec.ts tests/task/task-title-hover-surfaces.spec.ts` passed: 5 Chromium tests.
- `cd apps/web && pnpm e2e:run --project mobile-chrome tests/kanban/mobile-task-title-hover-subtasks.spec.ts` passed: 1 mobile Chrome test.
- `cd apps/web && pnpm run typecheck` passed.
- `cd apps/web && pnpm run lint` passed.
- `git diff --check` passed.
- The managed E2E runner cleaned its temporary backend, fixture repository, and test data. No external side effects remain.
- PR fixup extracted the sidebar task-content renderer into
  `apps/web/components/task/task-item-content.tsx` to satisfy the repository's
  600-line file limit. This is an internal, behavior-preserving refactor, so
  the product behavior and acceptance criteria are unchanged.
- PR fixup updated the existing PR badge E2E assertion for the removed sidebar
  title-preview tab stop. The focused managed Chromium test passed: 1 test.
