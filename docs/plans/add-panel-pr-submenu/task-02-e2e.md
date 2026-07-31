---
id: "02-e2e"
title: "E2E: desktop + mobile nested PR submenu"
status: pending
wave: 2
depends_on: ["01-frontend-submenu"]
plan: "plan.md"
spec: "../../specs/ui/add-panel-pr-submenu.md"
---

# Task 02: E2E coverage for the PR submenu

- **Acceptance:**
  1. `pr-multi-popover.spec.ts` "selecting a different PR from the + add-panel
     menu" test opens the `add-panel-pr-submenu` trigger before clicking
     `add-panel-pr-item-${OWNER}-api-77`, and still expects two `prDetailTab`s.
  2. New `mobile-add-panel-pr-submenu.spec.ts` (under
     `apps/web/e2e/tests/task/`) proves on a mobile viewport that the nested
     submenu opens from the "+" menu, its PR row is ≥44px tall and tappable
     (opens the PR panel), and the nested bottom-sheet stays within the viewport
     with no document horizontal overflow.
- **Verification:** (rebuild the web bundle first — E2E serves the production
  build)
  - `make build-web`
  - `cd apps/web && pnpm e2e -- tests/pr/pr-multi-popover.spec.ts`
  - `cd apps/web && pnpm e2e --project=mobile-chrome -- tests/task/mobile-add-panel-pr-submenu.spec.ts`
- **Files likely touched:**
  - `apps/web/e2e/tests/pr/pr-multi-popover.spec.ts`
  - `apps/web/e2e/tests/task/mobile-add-panel-pr-submenu.spec.ts` (new)
- **Dependencies:** task-01-frontend-submenu.
- **Parallelism:** sequential.
- **Inputs:**
  - Spec `Scenarios`: docs/specs/ui/add-panel-pr-submenu.md
  - Plan E2E section: docs/plans/add-panel-pr-submenu/plan.md
  - Desktop seeding: `associateTwoPRs` + `openTaskAndWait` in
    `apps/web/e2e/tests/pr/pr-multi-popover.spec.ts`
  - Mobile nested-menu pattern:
    `apps/web/e2e/tests/task/mobile-external-link-menu.spec.ts`
  - Overflow helper `assertNoDocumentHorizontalOverflow`:
    `apps/web/e2e/tests/gitlab/mobile-gitlab-parity.spec.ts`
  - Page object `addPanelButton()`/`prDetailTab()`:
    `apps/web/e2e/pages/session-page.ts`
- **Output contract:** summary, files changed, exact verification commands with
  results, task status → `done`, plan checkbox update.
