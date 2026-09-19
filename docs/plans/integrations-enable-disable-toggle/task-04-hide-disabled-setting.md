---
id: "04-hide-disabled-setting"
title: "'Hide disabled integrations from left panel navigation' setting"
status: done
wave: 3
depends_on: ["03-index-page-sliders"]
plan: "plan.md"
spec: "../../specs/integrations/requirements/enable-disable-toggle.md"
---

# Task 04: "Hide disabled integrations from left panel navigation" setting

- **Acceptance:**
  1. `useHideDisabledIntegrationsInNav()` exists, defaults to
     `hideDisabled: false`, persists via `setHideDisabled`, broadcasts its
     own sync event, and degrades to `false` (never throws) when
     `localStorage` is unavailable.
  2. `/settings/integrations` renders a new row below the integration cards
     with the exact label "Hide disabled integrations from left panel
     navigation", default off, using the shared draft/save-bar convention.
  3. New locale keys are added and the pseudo-locale is regenerated via the
     project's i18n tooling.
- **Verification:** `cd apps && pnpm --filter @kandev/web test -- hooks/domains/integrations/use-hide-disabled-integrations-in-nav app/settings/integrations/page` then `cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet`
- **Files likely touched:**
  - `apps/web/hooks/domains/integrations/use-hide-disabled-integrations-in-nav.ts` (new)
  - `apps/web/hooks/domains/integrations/use-hide-disabled-integrations-in-nav.test.ts` (new)
  - `apps/web/app/settings/integrations/page.tsx`
  - `apps/web/app/settings/integrations/page.test.tsx`
  - `apps/web/src/locales/en/settings.json`, `apps/web/src/locales/pseudo/settings.json`
- **Dependencies:** `03-index-page-sliders`.
- **Parallelism:** sequential.
- **Inputs:** spec "What"/"Data model"/"Failure modes", plan Task 04
  section, `use-integration-enabled.ts`'s hook shape,
  `archive-confirmation-settings.tsx`'s row-UI pattern.
- **Output contract:** summary, files changed, exact test/i18n commands run
  and their output, blockers/risks, task/plan status update.

## Results

- Added `useHideDisabledIntegrationsInNav()`: a standalone (non-per-integration)
  `useSyncExternalStore` hook backed by `kandev:integrations:hideDisabledInNav:v1`,
  broadcasting `kandev:integrations:hide-disabled-in-nav-changed`, defaulting
  to `false` on missing key, unreadable literal, or a throwing `localStorage`.
- Added `HideDisabledIntegrationsSetting` to the index page: a plain
  `<Switch>` + `<Label>` row (mirroring `archive-confirmation-settings.tsx`'s
  layout) wired through `useDraftedIntegrationEnabled` for the shared
  draft/save-bar behavior, rendered below the cards grid.
- Added locale keys `settings:hideDisabledIntegrationsFromNav` and
  `settings:hideDisabledIntegrationsFromNavDescription`; regenerated the
  pseudo-locale via `pnpm run i18n:pseudo` (not hand-edited).
- Extended `page.test.tsx` with a test asserting the new switch renders
  `aria-checked="false"` by default and flips its own (drafted) state
  immediately on click while leaving `localStorage` untouched until Save —
  and updated the slider-count assertion from 7 to 8.
- Commands:
  - `cd apps && pnpm --filter @kandev/web test -- use-hide-disabled-integrations-in-nav` → `Test Files 1 passed (1)`, `Tests 9 passed (9)`.
  - `cd apps/web && pnpm run typecheck` → clean.
  - `cd apps && pnpm --filter @kandev/web test -- app/settings/integrations/page` → `Test Files 1 passed (1)`, `Tests 4 passed (4)`.
  - `cd apps/web && pnpm run i18n:pseudo` → regenerated pseudo locale.
  - `cd apps/web && pnpm run i18n:check` → `✓ i18n keys OK` (908 pre-existing zh-cn advisory issues, unrelated to this change; no new orphans).
  - `cd apps/web && pnpm run i18n:ratchet` → `✓ i18n new-code ratchet — 0 added + 7 modified file(s) clean`, `✓ guard allowlist intact`.
- Files changed: the five files/pairs listed under "Files likely touched".
- Blockers/risks: none.
