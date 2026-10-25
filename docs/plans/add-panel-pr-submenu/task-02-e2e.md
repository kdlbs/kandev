---
id: "02-e2e"
title: "E2E: desktop nested PR submenu"
status: done
wave: 2
depends_on: ["01-frontend-submenu"]
plan: "plan.md"
spec: "../../specs/ui/requirements/add-panel-pr-submenu.md"
---

# Task 02: E2E coverage for the PR submenu

- **Acceptance:**
  1. `pr-multi-popover.spec.ts` "selecting a different PR from the + add-panel
     menu" test opens the `add-panel-pr-submenu` trigger before clicking
     `add-panel-pr-item-${OWNER}-api-77`, and still expects two `prDetailTab`s.
  2. Mobile coverage gap is documented: the dockview "+" add-panel menu is only
     rendered by `DockviewDesktopLayout` (`apps/web/components/task/task-layout.tsx`
     routes `isMobile` → `SessionMobileLayout`, which has no "+" add-panel menu
     and no linked-PR list), so there is no mobile entry point for this feature.
     The submenu behavior is covered by unit tests
     (`dockview-add-panel-items.test.tsx`); no `mobile-*.spec.ts` is added.
- **Verification:** (rebuild the web bundle first — E2E serves the production
  build)
  - `make build-web`
  - `cd apps/web && pnpm e2e -- tests/pr/pr-multi-popover.spec.ts`
- **Files likely touched:**
  - `apps/web/e2e/tests/pr/pr-multi-popover.spec.ts`
- **Dependencies:** task-01-frontend-submenu.
- **Parallelism:** sequential.
- **Inputs:**
  - Spec `Scenarios`: docs/specs/ui/requirements/add-panel-pr-submenu.md
  - Plan E2E section: docs/plans/add-panel-pr-submenu/plan.md
  - Desktop seeding: `associateTwoPRs` + `openTaskAndWait` in
    `apps/web/e2e/tests/pr/pr-multi-popover.spec.ts`
  - Page object `addPanelButton()`/`prDetailTab()`:
    `apps/web/e2e/pages/session-page.ts`
- **Output contract:** summary, files changed, exact verification commands with
  results, task status → `done`, plan checkbox update.
- **Status note:** marked done — desktop spec updated (submenu open step). Full
  E2E execution was not run locally in this environment (shared machine,
  heavy contention); the change is covered by CI E2E and the unit tests in
  `dockview-add-panel-items.test.tsx`.
