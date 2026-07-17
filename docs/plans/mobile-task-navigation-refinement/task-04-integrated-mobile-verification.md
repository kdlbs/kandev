---
id: "04-integrated-mobile-verification"
title: "Integrated mobile verification"
status: completed
wave: 3
depends_on: ["01-mobile-kanban-hierarchy", "02-mobile-home-task-interaction", "03-dockview-mobile-surfaces"]
plan: "plan.md"
spec: "../../specs/ui/mobile-task-navigation.md"
---

# Task 04: Integrated mobile verification

## Acceptance

- Fresh production mobile runs cover every spec scenario and no removed-sheet selector remains.
- Desktop Pipeline, Kanban menus, task edit, and workflow filtering regressions pass.
- Format, typecheck, tests, lint, rendered narrow-viewport checks, and public docs complete.

## Files likely touched

- `apps/web/e2e/pages/mobile-kanban-page.ts`
- `apps/web/e2e/tests/kanban/mobile-kanban.spec.ts`
- `apps/web/e2e/tests/task/mobile-*.spec.ts`
- `apps/web/e2e/tests/session/mobile-handoff.spec.ts`
- `docs/public/tasks-and-workflows.md`
- plan/task status files

## Inputs

- Completed Tasks 01–03 and all spec scenarios.
- Mobile parity checks: 44px targets, safe areas, internal vertical scroll, no document x-overflow, focus return.

## Verification

Use every command in `plan.md` → `Verification`.

## Output contract

Report command results, rendered evidence, remaining risks, and mark plan/spec statuses accurately.

## Completion

- Full format, typecheck, test, lint, and production web build passed.
- 35 focused mobile browser tests passed.
- 4 desktop Pipeline regression tests passed at a desktop viewport.
- Focused code review, simplification review, and QA audit reported no blockers.
