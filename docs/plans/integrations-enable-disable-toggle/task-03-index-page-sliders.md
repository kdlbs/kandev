---
id: "03-index-page-sliders"
title: "Per-row enable/disable slider on the integrations index page"
status: done
wave: 2
depends_on: ["01-enabled-hooks"]
plan: "plan.md"
spec: "../../specs/integrations/enable-disable-toggle.md"
---

# Task 03: Per-row enable/disable slider on the integrations index page

- **Acceptance:**
  1. `/settings/integrations` (and its per-workspace equivalent, same
     component with a `workspaceId` prop) renders one slider per integration
     row for all seven integrations, each reflecting and persisting that
     integration's `useXEnabled()` state.
  2. No slider is nested inside a navigating `<a>` — clicking a slider MUST
     NOT navigate to the integration's detail page; clicking the
     label/description MUST still navigate, exactly as before.
  3. `apps/web/e2e/tests/integrations/integrations-index-layout-helpers.ts`'s
     `expectStableIntegrationCardLayout` keeps passing against the
     restructured cards.
- **Verification:** `cd apps && pnpm --filter @kandev/web test -- app/settings/integrations/page` then `cd apps/web && pnpm e2e -- integrations-index-layout`
- **Files likely touched:**
  - `apps/web/app/settings/integrations/page.tsx`
  - `apps/web/app/settings/integrations/page.test.tsx` (new)
  - `apps/web/e2e/tests/integrations/integrations-index-layout-helpers.ts`
- **Dependencies:** `01-enabled-hooks`.
- **Parallelism:** `parallel-safe` with `02-own-page-sliders`. NOT
  parallel-safe with `04-hide-disabled-setting`, which edits the same
  `page.tsx`.
- **Inputs:** spec "What"/scenarios, plan Task 03 section, existing card
  markup + layout e2e test/helper, `drafted-integration-enabled-control.tsx`.
- **Output contract:** summary, files changed, exact test/e2e commands run
  and their output, blockers/risks, task/plan status update.

## Results

- Restructured each card: `Card` is the outer element (no longer wrapped in
  a navigating `<a>`); the icon+label link and the per-integration
  `EnabledControl` share one top row (`flex items-center justify-between
  gap-2`), with `EnabledControl` rendered beside the title link rather than
  on a separate row below it, and the description is a separate link below
  that. This avoids nesting an interactive `<Switch>` inside an `<a>`
  (invalid DOM / fragile click propagation) without any absolute-positioning
  trick.
- Selecting the right slider per row required one statically-imported
  `useXEnabled` hook per component (rules of hooks forbid picking a hook
  dynamically by slug): added `ENABLED_CONTROL_BY_SLUG: Record<IntegrationSlug, ComponentType>`
  mapping each slug to its Task 01/02 `<XEnabledControl>` component.
- Iterated on the header layout under `expectStableIntegrationCardLayout`'s
  guard (`MAX_ICON_TOP_INSET_PX`) before landing on the final shape:
  - First attempt (slider inline with the label row, `items-center`) pushed
    the icon down to a measured 31px inset (label text wrapped to 2 lines
    once squeezed by the slider pill) — rejected loosening the threshold;
    fixed the root cause instead.
  - Final shape (slider on its own row) reproduces the pre-existing
    single-line title row exactly, measuring 19px — well under the original
    22px budget. The budget in
    `integrations-index-layout-helpers.ts` was **not** changed.
  - Verified via a temporary manual backend instance +
    `xd://browser` DOM/screenshot inspection before re-running the real e2e
    spec each iteration; the temporary diagnostic spec file was deleted
    before finishing.
- Added `apps/web/app/settings/integrations/page.test.tsx`: 4 tests (later
  extended to include Task 04's row — see task-04) covering slider count,
  initial checked state, label-click navigation, and slider-click
  non-navigation.
- Updated `integrationCards()` in the e2e helper to locate cards via
  `[data-testid^="integration-card-"]` directly instead of deriving them
  from `a[href*="/integrations/"] > [data-slot="card"]` (no longer valid —
  `Card` is not an anchor's child).
- Commands:
  - `cd apps && pnpm --filter @kandev/web test -- app/settings/integrations/page` → `Test Files 1 passed (1)`, `Tests 3 passed (3)` (pre-Task-04 state).
  - `cd apps/web && pnpm run typecheck` → clean.
  - E2E (via a managed background process, `apps/backend/bin/kandev` +
    `apps/web/dist` built locally with `make build-backend`/`make build-web`/
    `make -C apps/backend e2e-plugin-package`):
    `npx playwright test --config e2e/playwright.config.ts e2e/tests/integrations/integrations-index-layout.spec.ts` → `1 passed`.
- Files changed: the three files listed under "Files likely touched".
- Blockers/risks: none remaining. Risk noted and resolved: the initial
  layout regression (icon inset) was caught by the existing e2e test exactly
  as intended — the fix changed markup, not the test's tolerance.
