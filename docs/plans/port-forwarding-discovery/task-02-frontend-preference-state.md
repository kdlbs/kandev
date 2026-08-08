---
id: "02-frontend-preference-state"
title: "Frontend preference state"
status: completed
wave: 2
depends_on: ["01-backend-preference-contract"]
plan: "plan.md"
spec: "../../specs/ui/port-forwarding-discovery.md"
---

# Task 02: Frontend preference state

Thread task metadata and the new mutation through the canonical web task state, then provide one
shared controller for desktop, tablet, and phone surfaces. Do not change the visual launcher or
top-bar composition yet.

## Acceptance

- HTTP/SSR/WS task snapshots expose `metadata.port_forwarding_enabled` without losing it on partial
  task updates.
- The shared controller reports preference/readiness, performs the new API mutation, reconciles
  `task.updated`, and rolls back failed writes with translated feedback.
- The controller has an explicit open-dialog callback/state so a successful enable can implement
  “toggle and open” without each viewport duplicating mutation logic.

## Verification

- If dependencies are absent: `cd apps && pnpm install --frozen-lockfile`
- `cd apps && pnpm --filter @kandev/web test -- lib/kanban/map-task.test.ts lib/ws/handlers/tasks.test.ts components/task/port-forwarding-visibility.test.tsx`
- `cd apps/web && pnpm run typecheck`

## Files likely touched

- `apps/web/lib/api/domains/kanban-api.ts`
- `apps/web/lib/state/slices/kanban/types.ts`
- `apps/web/lib/kanban/map-task.ts`
- `apps/web/lib/kanban/map-task.test.ts`
- `apps/web/lib/ssr/mapper.ts`
- `apps/web/components/task/task-page-content-helpers.ts`
- `apps/web/lib/ws/handlers/tasks.ts`
- `apps/web/lib/ws/handlers/tasks.test.ts`
- `apps/web/components/task/port-forwarding-visibility.tsx`
- `apps/web/components/task/port-forwarding-visibility.test.tsx`
- `apps/web/src/locales/en/task.json`
- `apps/web/src/locales/pt-pt/task.json`
- `apps/web/src/locales/pseudo/task.json`

## Dependencies

Task 01.

## Parallelism

Sequential. The shared controller and canonical task mapping are common state for all UI surfaces.

## Inputs

- Spec sections: What, Data model, API surface, State machine, Failure modes, Persistence guarantees.
- Backend route and event contract from Task 01.
- Existing `toKanbanTask`, SSR snapshot mapping, and partial task-event merge rules.

## Output contract

Report the summary, actual files changed, exact focused test/typecheck results, event-reconciliation
behavior, and synchronized task/plan status. Leave the launcher and top-bar rendering for Task 03.

## Results

- Added the typed preference mutation API and carried task metadata through kanban state, SSR
  hydration, task-page resolution, and partial WebSocket task updates.
- Added a shared visibility provider with readiness gating, optimistic writes, rollback feedback,
  task-change reconciliation, and successful-enable dialog opening.
- Added translated failure feedback in the English, pseudo-locale, and Portuguese locale files.
- Changed files:
  - `apps/web/components/task/port-forwarding-visibility-provider.test.tsx`
  - `apps/web/components/task/port-forwarding-visibility-provider.tsx`
  - `apps/web/components/task/port-forwarding-visibility.test.ts`
  - `apps/web/components/task/port-forwarding-visibility.ts`
  - `apps/web/components/task/task-page-content-helpers.test.ts`
  - `apps/web/components/task/task-page-content-helpers.ts`
  - `apps/web/lib/api/domains/kanban-api.test.ts`
  - `apps/web/lib/api/domains/kanban-api.ts`
  - `apps/web/lib/kanban/map-task.test.ts`
  - `apps/web/lib/kanban/map-task.ts`
  - `apps/web/lib/ssr/mapper.test.ts`
  - `apps/web/lib/ssr/mapper.ts`
  - `apps/web/lib/state/slices/kanban/types.ts`
  - `apps/web/lib/ws/handlers/tasks-port-forwarding.test.ts`
  - `apps/web/lib/ws/handlers/tasks.ts`
  - `apps/web/src/locales/en/task.json`
  - `apps/web/src/locales/pseudo/task.json`
  - `apps/web/src/locales/pt-pt/task.json`
- Event reconciliation: an explicit metadata object replaces the cached metadata, while an omitted
  metadata field preserves the existing preference.
- Verification: `rtk pnpm exec vitest run lib/api/domains/kanban-api.test.ts lib/kanban/map-task.test.ts
  lib/ssr/mapper.test.ts lib/ws/handlers/tasks-port-forwarding.test.ts
  components/task/task-page-content-helpers.test.ts components/task/port-forwarding-visibility.test.ts
  components/task/port-forwarding-visibility-provider.test.tsx` — passed (65 tests across 7 files).
- Verification: `rtk pnpm run typecheck` — passed.
- Synchronized status: Task 02 and the implementation plan are marked completed.
