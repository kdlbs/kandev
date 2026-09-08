---
id: "03-end-to-end-verification"
title: "End-to-end verification"
status: done
wave: 2
depends_on: ["01-preview-and-hierarchy", "02-contribution-status-and-state"]
plan: "plan.md"
spec: "../../specs/tasks/requirements/rich-task-title-previews.md"
---

# Task 03: End-to-end verification

- **Acceptance:** Desktop pointer and keyboard flows work on real task data. Mobile direct navigation and task-tree access work on a coarse pointer.
- **Verification:** `cd apps/web && pnpm e2e:run tests/task/task-title-hover-surfaces.spec.ts && pnpm e2e:run --project mobile-chrome tests/kanban/mobile-task-title-hover-subtasks.spec.ts`.
- **Files likely touched:** the desktop and mobile task-title Playwright specifications.
- **Dependencies:** Tasks 01 and 02.
- **Parallelism:** sequential.

## Results

`cd apps/web && pnpm e2e:run tests/task/task-title-hover-surfaces.spec.ts` first found the missing child-link focus. After the fix, all four Chromium tests passed. `cd apps/web && pnpm e2e:run --project mobile-chrome tests/kanban/mobile-task-title-hover-subtasks.spec.ts` passed one test. The managed runner removed its temporary backends and fixture data. External side effects: local temporary E2E data only.
