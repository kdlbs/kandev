---
id: "02-tree-and-target-navigation"
title: "Settings tree search and target navigation"
status: completed
wave: 2
depends_on: ["01-catalog-and-search"]
plan: "plan.md"
spec: "../../specs/ui/requirements/settings-discovery.md"
---

# Task 02: Settings tree search and target navigation

## Acceptance

- Empty Settings search preserves the current tree; a query renders only matching grouped results
  with labels, breadcrumbs, clear/Escape, and an accessible empty/status state.
- Cross-page targets use guarded routing; same-page, delayed, repeated, and history targets scroll,
  focus, and briefly highlight without changing values.
- Desktop and phone reuse the same logic, existing scroll owners, and at least 44 px phone actions.

## Verification

`cd apps && pnpm --filter @kandev/web test -- --run components/app-sidebar/sections/settings components/settings/settings-target-provider.test.tsx`

## Files likely touched

- `apps/web/components/app-sidebar/sections/settings/settings-tree.tsx`
- `apps/web/components/app-sidebar/sections/settings/settings-search.tsx`
- `apps/web/components/app-sidebar/sections/settings/settings-nav-primitives.tsx`
- `apps/web/components/settings/settings-target-provider.tsx`
- `apps/web/components/settings/settings-target.tsx`
- `apps/web/components/settings/settings-layout-client.tsx`
- `apps/web/components/settings/settings-card.tsx`
- `apps/web/components/settings/settings-section.tsx`
- `apps/web/app/globals.css`

## Dependencies

Task 01.

## Parallelism

Sequential; tree and target behavior share navigation contracts.

## Results

- Added persistent Settings-tree search with grouped ranked hits, breadcrumbs, clear/Escape, and accessible result status.
- Added guarded cross-page and blocker-free same-page fragment navigation with repeat/history events.
- Added registry-backed async target resolution, focus, centered scroll, reduced-motion handling, and one-shot highlight.
- Added wrapper-free target support to shared settings cards and sections; wired initial terminal targets.
- Increased all phone Settings Sheet navigation actions to at least 44 px.
- Passed target, tree, and typecheck suites.
