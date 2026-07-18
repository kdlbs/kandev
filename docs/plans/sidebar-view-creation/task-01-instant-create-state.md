---
id: "01-instant-create-state"
title: "Instant-create sidebar view state"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/sidebar-view-creation.md"
---

# Task 01: Instant-Create Sidebar View State

Implement the dedicated optimistic state action through RED/GREEN tests before UI wiring.

## Acceptance

- `createSidebarView()` appends and activates an independent canonical-default view using the lowest available exact automatic name.
- The action sends the complete saved-view/active-view/draft settings payload and reuses existing retry, rollback, and sync-error behavior.
- An active draft or 50 saved views makes the action a no-op; existing `Save as…` and duplicate semantics do not change.

## Verification

```bash
cd apps
pnpm --filter @kandev/web test -- lib/state/slices/ui/sidebar-view-actions.test.ts
cd web
pnpm run typecheck
```

## Files likely touched

- `apps/web/lib/state/slices/ui/sidebar-view-builtins.ts`
- `apps/web/lib/state/slices/ui/sidebar-view-actions.ts`
- `apps/web/lib/state/slices/ui/types.ts`
- `apps/web/lib/state/store.ts`
- `apps/web/lib/state/slices/ui/sidebar-view-actions.test.ts`

## Dependencies

None.

## Inputs

- Spec: canonical defaults, naming, limit, persistence, and failed-create scenarios.
- Plan: Frontend > Instant-create state contract; Tests.
- Existing patterns: `makeId`, `mutateViews`, `snapshotSidebar`, and `toSidebarSettingsPayload` in `sidebar-view-actions.ts`; `DEFAULT_VIEW` in `sidebar-view-builtins.ts`; backend `maxSidebarViews` in `apps/backend/internal/user/service/service.go`.
- ADR 0041: backend user settings remain the only durable source.

## Output contract

Report state behavior, files changed, focused test/typecheck results, blockers, and residual risks. Set this task to `done` and update its checkbox in `plan.md` only after acceptance and verification pass.
