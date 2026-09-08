---
id: "01-catalog-and-search"
title: "Settings discovery catalog and search"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/settings-discovery.md"
---

# Task 01: Settings discovery catalog and search

## Acceptance

- Typed domain catalogs resolve translated static and dynamic entries with stable IDs, canonical
  URLs, target IDs, safe aliases, and current permission gates.
- Search deterministically ranks exact/prefix/alias/context matches, supports Unicode/diacritics,
  and prevents ancestor-only result floods.
- Every control has a target; every first-party canonical page is covered or explicitly excluded.

## Verification

`cd apps && pnpm --filter @kandev/web test -- --run lib/settings-discovery`

## Files likely touched

- `apps/web/lib/settings-discovery/**`
- `apps/web/components/settings/general-nav.ts`
- `apps/web/components/app-sidebar/sections/settings/{general,workspaces,agents,executors,system,account}-group.tsx`

## Dependencies

None.

## Parallelism

Sequential; it defines shared contracts for every later task.

## Results

- Added domain-split, typed static and dynamic catalogs with route exclusions and permission gates.
- Added runtime translation/breadcrumb resolution and URL-safe dynamic paths.
- Added deterministic Unicode-aware search with direct-term gating and ranked label/alias matches.
- Reused catalog route metadata from existing Settings tree groups to prevent route drift.
- Passed: `cd apps && pnpm --filter @kandev/web test -- --run lib/settings-discovery components/app-sidebar/sections/settings/settings-tree-render.test.tsx components/app-sidebar/sections/settings/settings-tree.test.ts` (27 tests).
- Passed: `cd apps/web && pnpm run typecheck`.
