---
spec: docs/specs/ui/mobile-task-navigation.md
created: 2026-07-17
status: completed
---

# Implementation Plan: Mobile Task Navigation

## Overview

First prove missing mobile task actions and multi-workflow scroll behavior with focused Playwright coverage. Then implement task-menu containment and focused Kanban navigation in parallel-safe frontend slices, followed by integrated mobile and desktop verification.

## Frontend

### Mobile task actions

- `apps/web/components/task/task-switcher-context-menu.tsx`: fall back to `useTaskWorkflowMove` for a single task when the mobile caller has no injected same-workflow handler.
- `apps/web/components/task/task-item.tsx`: keep the Task actions affordance visible below the app's 640px mobile breakpoint without covering diff stats.
- `apps/web/app/globals.css`: give shared context/dropdown menu popper surfaces viewport-fixed bottom-sheet geometry, safe-area spacing, contained scrolling, and 44px rows only below the app's 640px mobile breakpoint.
- Keep existing desktop menu components and destructive confirmation dialogs intact.

### Mobile Kanban

- `apps/web/components/kanban/swimlane-container.tsx`: derive a transient valid focused workflow and mount only that workflow on mobile.
- Add `apps/web/components/kanban/mobile-workflow-picker.tsx`: 44px trigger plus bottom drawer with workflow names and task counts.
- `apps/web/components/kanban/mobile-column-tabs.tsx`: replace horizontal tabs with previous/current/next controls and a step drawer.
- `apps/web/components/kanban/swipeable-columns.tsx`: use parent flex height instead of per-workflow `100dvh` math.
- `apps/web/components/kanban/mobile-drop-targets.tsx`: contain drag targets vertically instead of adding horizontal overflow.
- `apps/web/components/kanban/mobile-menu-sheet.tsx`: expose All workflows explicitly.

## Tests

- **Mobile task Move to:** `apps/web/e2e/tests/task/mobile-sidebar-task-actions.spec.ts`; open visible Task actions, choose Move to, assert persisted step.
- **Mobile action layout:** same spec; assert root menu and rows fit the viewport and use touch-sized targets.
- **Affected mobile task actions:** update archive/link specs that currently depend on long press.
- **Focused workflow:** `apps/web/e2e/tests/kanban/mobile-kanban.spec.ts`; seed several workflows, assert one mounted board, switch workflow, assert no document horizontal overflow.
- **Focused step:** same spec; open step drawer, select another step, assert its task and WIP/count state.
- **Desktop regression:** existing Kanban workflow filter, task menu, and move specs remain unchanged and are included in verification when feasible.

## E2E Tests

- **GIVEN** the mobile Dockview task drawer, **WHEN** Task actions → Move to → another step is selected, **THEN** the backend task reflects the new step.
- **GIVEN** All workflows on mobile, **WHEN** the workflow drawer selects another workflow, **THEN** one workflow layout remains mounted and only that workflow's task is shown.
- **GIVEN** several steps, **WHEN** the step drawer selects another step, **THEN** its cards become visible without document horizontal overflow.

## Implementation Waves

Wave 1 (parallel after RED tests):

- [x] [Task 01 — Mobile task actions](task-01-mobile-task-actions.md)
- [x] [Task 02 — Mobile Kanban navigation](task-02-mobile-kanban-navigation.md)

Wave 2:

- [x] [Task 03 — Mobile E2E and verification](task-03-mobile-e2e-and-verification.md)

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- --run components/task/task-item.test.tsx
cd apps/web && pnpm run typecheck
cd apps && pnpm --filter @kandev/web lint
make build-web
cd apps/web && pnpm e2e:run tests/task/mobile-sidebar-task-actions.spec.ts tests/task/mobile-external-link-menu.spec.ts tests/task/mobile-archive-confirmation-preference.spec.ts tests/kanban/mobile-kanban.spec.ts
```

## Risks

- Radix Popper wrappers require rendered mobile-width verification; explicit Drawer components remain fallback if wrapper positioning is inconsistent.
- Nested Sheet/context-menu focus must return to the mobile task drawer after dismiss.
- Focused workflow fallback must handle search and WebSocket-driven removals without state-update loops.

## Verification outcome

- Fresh production build plus the four mobile Playwright specs: 22 passed.
- Web unit suite: 5,117 passed, 4 skipped across 633 files; focused changed-logic suite: 41 passed.
- Backend tests, repository typecheck, repository lint, and public-doc validation passed.
- Full CLI suite has one unrelated repository-state failure: `.github/workflows/notify-docs.yml` pins pnpm `10.29.3` while `apps/package.json` pins `9.15.9`; the other 279 CLI tests passed.
