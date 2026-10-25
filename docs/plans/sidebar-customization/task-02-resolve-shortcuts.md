---
id: "02-resolve-shortcuts"
title: "Resolve shortcut references and layout operations"
status: done
wave: 2
depends_on:
  - "01-persist-layouts"
plan: "plan.md"
requirements:
  - REQ-UI-SIDEBAR-CUSTOMIZATION-001
  - REQ-UI-SIDEBAR-CUSTOMIZATION-002
  - REQ-UI-SIDEBAR-CUSTOMIZATION-004
acceptance_criteria:
  - AC-UI-SIDEBAR-CUSTOMIZATION-001.1
  - AC-UI-SIDEBAR-CUSTOMIZATION-001.4
  - AC-UI-SIDEBAR-CUSTOMIZATION-001.5
  - AC-UI-SIDEBAR-CUSTOMIZATION-002.1
  - AC-UI-SIDEBAR-CUSTOMIZATION-002.4
  - AC-UI-SIDEBAR-CUSTOMIZATION-002.6
  - AC-UI-SIDEBAR-CUSTOMIZATION-004.4
  - AC-UI-SIDEBAR-CUSTOMIZATION-004.5
system_design:
  - ../../specs/ui/system-design/sidebar-customization.md
---

# Task 02: Resolve shortcut references and layout operations

## Summary

Build the shared typed layout operations and shortcut projection. Resolve existing domain references without copying navigation or execution authority.

## In scope

- Stable IDs, normalization, limits, reorder/move operations, default projection, protected entries, and unavailable references.
- Adapters for built-in destinations, registered plugin navigation, canvas/automation links, and allowed host launchers.

## Out of scope

- New plugin SDK API, arbitrary callbacks/URLs, and rendered surfaces.

## Acceptance

- Typed resolution preserves access gates, owner-qualified plugin identities, and workspace isolation.
- Moves preserve instance identity and reject duplicate targets within one group.
- Automation and canvas shortcuts resolve navigation only; hidden defaults do not remove palette commands.

## Verification

Run from the repository root. Install dependencies once with
`(cd apps && pnpm install --frozen-lockfile)` if this worktree has no install.
Use TDD for new logic and the named E2E scenarios. Managed E2E commands rebuild
assets; run desktop and phone commands sequentially without worker overrides.

```bash
(cd apps/web && pnpm exec vitest run lib/sidebar/layout-operations.test.ts lib/sidebar/layout-projection.test.ts lib/sidebar/shortcut-catalog.test.ts lib/navigation/core-destinations.test.ts)
(cd apps/web && pnpm run typecheck)
```

## Files likely touched

- `apps/web/lib/sidebar/layout-types.ts (new)`
- `apps/web/lib/sidebar/layout-operations.ts (new)`
- `apps/web/lib/sidebar/layout-projection.ts (new)`
- `apps/web/lib/sidebar/shortcut-catalog.ts (new)`
- `apps/web/lib/navigation/{resolve-destinations,plugin-destinations,surface-policy}.ts`
- `apps/web/hooks/domains/sidebar/`

## Dependencies

01-persist-layouts.

## Risks

Do not confuse hidden with unavailable. Preserve unresolved identities during plugin loading and workspace transitions.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ui/requirements/sidebar-customization.md), IDs above.
- [System design](../../specs/ui/system-design/sidebar-customization.md), corresponding sections.
- [Plan](plan.md), test matrix and previews.
- Scoped `apps/web/AGENTS.md` and `apps/backend/AGENTS.md` where applicable.

## Results

Implemented typed shortcut references, catalog resolution for built-ins, host
actions, plugins, canvases, and automations, unavailable-target projection, and
identity-preserving reorder/move operations. Verification passed with the
sidebar operation, projection, catalog, and navigation Vitest coverage and
`pnpm run typecheck`.
