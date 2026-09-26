---
id: app-status-bar-14
title: Sidebar-aligned status bar
status: done
wave: 9
depends_on: [app-status-bar-13]
plan: docs/plans/app-status-bar/plan.md
spec: docs/specs/ui/requirements/app-status-bar.md
---

# Sidebar-aligned status bar

## Inputs

[Spec: responsive and layout contract](../../specs/ui/requirements/app-status-bar.md#responsive-and-layout-contract),
the root `AppSidebar` layout reservation, the existing `AppStatusSurfaceProvider`,
and the shipped desktop/mobile status-surface E2E coverage.

## Implementation

- Make the root shell a full-viewport row whose first child is `AppSidebar` and
  whose second child is the `min-w-0 flex-1` status-surface column.
- Keep route content as that column's `min-h-0 flex-1` region and keep the 24 px
  desktop/tablet bar as its in-flow footer. This reuses the sidebar's flex
  reservation for alignment instead of calculating its width again.
- Preserve current sidebar snap-and-overlay animation, status item rendering,
  drawer state, metrics subscriptions, and fixed-overlay clearance.

## Mobile design contract

- **Desktop outcome:** the bar begins at the visible sidebar layout edge and the
  sidebar reaches the viewport bottom.
- **Mobile entry and exemplar:** retain the shipped `AppStatusDrawer` and native
  triggers; `mobile-status-drawer.spec.ts` remains the closest mobile proof.
- **Hierarchy and presentation:** phone still has no persistent bar and opens the
  same inset bottom drawer. No action, ordering, or information hierarchy changes.
- **Scroll, viewport, safe area:** the shell remains `h-dvh` with one route scroll
  owner; drawer internal scrolling and safe-area clearance remain unchanged.
- **Shared state:** `AppStatusSurfaceProvider` continues to own responsive
  selection, drawer state, active context, and status data for both presentations.

## Files likely touched

- `apps/web/app/layout.tsx`
- `apps/web/components/app-status-bar/app-status-surface-provider.tsx`
- `apps/web/components/app-status-bar/app-status-surface-provider.test.tsx`
- `apps/web/e2e/tests/layout/app-status-bar.spec.ts`

## Acceptance

1. On sidebar-visible viewports, the settled status-bar left edge equals the
   sidebar right edge while the bar right edge equals the viewport right edge;
   this holds expanded, collapsed, and after direct sidebar resize.
2. The sidebar reaches the viewport bottom, route content retains the remaining
   height above the 24 px bar, and no document-level overflow is introduced.
3. Sidebar-hidden tablet layouts keep a full-width bar. Phone keeps no persistent
   bar and its existing Status drawer path remains usable and viewport-contained.

## Verification

```sh
cd apps && pnpm --filter @kandev/web test -- components/app-status-bar/app-status-surface-provider.test.tsx
cd apps/web && pnpm run typecheck
cd apps/web && pnpm e2e:run tests/layout/app-status-bar.spec.ts
cd apps/web && pnpm e2e:run tests/plugins/mobile-status-drawer.spec.ts -- --project=mobile-chrome
```

The desktop E2E must compare sidebar/status-bar bounding boxes before and after
collapse and resize. Existing mobile E2E supplies parity coverage because phone
composition and interaction stay unchanged.

## Verification results

- `cd apps && pnpm --filter @kandev/web test -- components/app-status-bar/app-status-surface-provider.test.tsx` — 4 passed.
- `cd apps/web && pnpm run typecheck` — passed.
- `cd apps/web && pnpm e2e:run tests/layout/app-status-bar.spec.ts` — 3 passed;
  the new assertion first failed against the old full-width bar, then passed
  after the shell change.
- `cd apps/web && pnpm e2e:run tests/plugins/mobile-status-drawer.spec.ts -- --project=mobile-chrome` — 2 passed.

## Dependencies

Task 13. Sequential: it amends the shipped shell geometry.

## Risks

- A `w-full` flex child beside the sidebar can overflow the viewport; the
  status-surface column must use `min-w-0 flex-1`.
- Moving the shell boundary can accidentally make a route own `h-dvh` again or
  change drawer/provider lifetime. Preserve current parent-height and provider
  ownership contracts.
- Sidebar visual width animates after its layout reservation snaps. Tests should
  assert settled alignment and live resize without changing that intentional
  animation behavior.

## Output contract

Report desktop expanded/collapsed/resized geometry, sidebar-hidden and phone
behavior, files changed, exact tests run, and blockers. Mark this task done and
return the plan/spec to shipped only after focused verification passes.
