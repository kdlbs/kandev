---
id: "03-mobile-e2e-and-verification"
title: "Mobile E2E and verification"
status: done
wave: 2
depends_on: ["01-mobile-task-actions", "02-mobile-kanban-navigation"]
plan: "plan.md"
spec: "../../specs/ui/mobile-task-navigation.md"
---

# Task 03: Mobile E2E and verification

## Acceptance

- Focused mobile task-action and Kanban flows pass against a fresh production web build.
- Typecheck, lint, focused unit tests, and desktop-sensitive regressions pass or exact unrelated blockers are recorded.
- Rendered mobile checks cover safe areas, long labels, 44px targets, nested menu overflow, and document width.

## Files likely touched

- `apps/web/e2e/tests/task/mobile-*.spec.ts`
- `apps/web/e2e/tests/kanban/mobile-kanban.spec.ts`
- `apps/web/e2e/pages/mobile-kanban-page.ts`
- implementation files only for integration fixes

## Verification

```bash
cd apps/web && pnpm run typecheck
cd apps && pnpm --filter @kandev/web lint
make build-web
cd apps/web && pnpm e2e:run tests/task/mobile-sidebar-task-actions.spec.ts tests/task/mobile-external-link-menu.spec.ts tests/task/mobile-archive-confirmation-preference.spec.ts tests/kanban/mobile-kanban.spec.ts
```

## Inputs

- Completed Tasks 01 and 02.
- Mobile-parity and E2E skill requirements.

## Output contract

Report commands/results, screenshots or rendered evidence when available, remaining risks, and set plan/spec build statuses accurately.

## Result

- Fresh production mobile Playwright run: 22 passed, covering task Move to, root/nested ContextMenu geometry, representative DropdownMenu geometry, workflow focus/fallback, step drawers, WIP counts, and document width.
- Focused changed-logic unit suite: 41 passed; full web suite: 5,117 passed and 4 skipped across 633 files.
- Typecheck, backend tests, full lint, and public-doc checks passed.
- Full CLI suite stopped on one unrelated pnpm pin mismatch between `.github/workflows/notify-docs.yml` (`10.29.3`) and `apps/package.json` (`9.15.9`); 279 CLI tests passed.
