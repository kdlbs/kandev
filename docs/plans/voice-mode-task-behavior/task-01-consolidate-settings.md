---
id: "01-consolidate-settings"
title: "Consolidate Voice Mode settings"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/voice-mode-task-behavior.md"
---

# Task 01: Consolidate Voice Mode Settings

## Acceptance

- Task Behavior renders the existing Voice Mode section after message queue with unchanged voice
  controls and shared save/discard semantics.
- The Settings menu has no Voice Mode row, and `/settings/voice-mode` is removed without a
  compatibility redirect.
- Voice page/control discovery results belong to Task Behavior, retain stable target IDs, and open
  the matching section or control.

## Verification

```bash
cd apps && pnpm install --frozen-lockfile
cd apps && pnpm --filter @kandev/web test -- --run components/settings/voice-mode-settings.test.tsx components/app-sidebar/sections/settings/settings-tree-render.test.tsx components/app-sidebar/sections/settings/settings-nav-copy.test.ts src/settings-routes.test.ts lib/settings-discovery/catalog.test.ts
cd apps/web && pnpm run typecheck
cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet
```

Follow RED-GREEN-REFACTOR: first change the focused menu, route, and discovery expectations so they
fail against the standalone page, then implement the smallest consolidation that passes.

## Files Likely Touched

- `apps/web/components/settings/task-behavior-settings.tsx`
- `apps/web/components/settings/voice-mode-settings.tsx` only if a test seam is needed
- `apps/web/components/settings/voice-mode-settings.test.tsx`
- `apps/web/components/app-sidebar/sections/settings/settings-menu-sections.ts`
- `apps/web/components/app-sidebar/sections/settings/settings-tree-render.test.tsx`
- `apps/web/components/app-sidebar/sections/settings/settings-nav-copy.test.ts`
- `apps/web/src/settings-routes.tsx`
- `apps/web/src/settings-routes.test.ts`
- `apps/web/lib/settings-discovery/catalog/preferences.ts`
- `apps/web/lib/settings-discovery/catalog/standalone.ts`
- `apps/web/lib/settings-discovery/catalog.test.ts`
- relevant locale catalogs only if existing copy keys require relocation
- this task file and `plan.md`

## Inputs

- Spec `What` and the menu, removed-route, discovery, and persistence scenarios.
- Existing `VoiceModeSettings`, `TaskBehaviorSettings`, and settings route registry.
- Settings discovery target IDs and the shared save coordinator contract in ADR 0046.

## Mobile Design Contract

Reuse the existing Task Behavior direct-navigation page and shared Settings shell. Desktop and
mobile share the same linear section order and voice controls; the settings content stays the one
scroll owner, with no new drawer, nested route, or mobile-only state.

## Risks

- Preserve every discovery target ID and mount one voice draft provider to avoid broken search
  scrolling or duplicate save contributors.
- Remove obsolete route, breadcrumb, and href constants so internal navigation cannot emit the
  intentionally unsupported URL.

## Results

- Focused web tests pass: 5 files, 95 tests.
- `pnpm run typecheck` passes.
- `pnpm run i18n:check` and `pnpm run i18n:ratchet` pass.
- The standalone route, menu row, breadcrumb ownership, and discovery target were removed without
  a compatibility redirect; Voice Mode now renders once under Task Behavior.
