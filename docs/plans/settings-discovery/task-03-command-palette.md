---
id: "03-command-palette"
title: "Search-only settings commands"
status: completed
wave: 3
depends_on: ["01-catalog-and-search", "02-tree-and-target-navigation"]
plan: "plan.md"
spec: "../../specs/ui/requirements/settings-discovery.md"
---

# Task 03: Search-only settings commands

## Acceptance

- Granular settings commands are absent with no query and appear after typing with muted hierarchy.
- Catalog commands retain legacy page aliases, deterministic ranking, stable dynamic IDs, and exact
  target navigation.
- Dynamic settings data loads only while the Commands palette is open.

## Verification

`cd apps && pnpm --filter @kandev/web test -- --run lib/commands/search.test.ts components/command-panel-content-search.test.tsx components/global-commands.test.tsx`

## Files likely touched

- `apps/web/lib/commands/types.ts`
- `apps/web/lib/commands/search.ts`
- `apps/web/components/command-panel-footer.tsx`
- `apps/web/components/settings/settings-discovery-commands.tsx`
- `apps/web/components/global-commands.tsx`

## Dependencies

Tasks 01 and 02.

## Parallelism

Sequential; it consumes catalog and target navigation.

## Results

- Registered catalog-backed settings commands with stable legacy IDs and aliases.
- Kept granular entries search-only while retaining the top-level Go to Settings destination.
- Added two-line result presentation with non-searchable owning context.
- Reused exact target navigation and gated dynamic settings loads to an open Commands palette.
- Passed command search, panel, global-command, builder, and typecheck suites.
