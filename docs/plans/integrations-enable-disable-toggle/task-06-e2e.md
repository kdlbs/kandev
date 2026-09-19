---
id: "06-e2e"
title: "E2E coverage for index-page sliders and nav-hiding behavior"
status: done
wave: 5
depends_on: ["02-own-page-sliders", "03-index-page-sliders", "05-nav-availability"]
plan: "plan.md"
spec: "../../specs/integrations/requirements/enable-disable-toggle.md"
---

# Task 06: E2E coverage for index-page sliders and nav-hiding behavior

- **Acceptance:**
  1. A new Playwright spec proves every one of the seven integration rows on
     `/settings/integrations` has a working slider, toggling GitHub's
     index-page slider updates GitHub's own settings-page slider to the same
     state, and clicking a slider never navigates.
  2. A new Playwright spec proves: with a configured/healthy, disabled
     GitHub and "hide disabled" off (default), GitHub's sidebar nav entry is
     visible; turning "hide disabled" on hides it without a reload; turning
     GitHub back on reveals it again.
  3. `apps/web/e2e/tests/integrations/integrations-index-layout.spec.ts`
     still passes unmodified.
- **Verification:** `cd apps/web && pnpm install --frozen-lockfile && pnpm e2e -- integrations-index-enabled-toggle hide-disabled-integrations-nav integrations-index-layout`
- **Files likely touched:**
  - `apps/web/e2e/tests/integrations/integrations-index-enabled-toggle.spec.ts` (new)
  - `apps/web/e2e/tests/integrations/hide-disabled-integrations-nav.spec.ts` (new)
- **Dependencies:** `02-own-page-sliders`, `03-index-page-sliders`,
  `05-nav-availability`.
- **Parallelism:** sequential (last task; exercises the full feature).
- **Inputs:** spec Scenarios, plan E2E Tests section,
  `integrations-index-layout-helpers.ts`, `mobile-integrations-nav.spec.ts`'s
  `apiClient.mockGitHubSetUser` fixture and visibility-check style.
- **Output contract:** summary, files changed, exact e2e command run and its
  output, blockers/risks, task/plan status update, synchronize `plan.md`'s
  Verification Results.

## Results

- `integrations-index-enabled-toggle.spec.ts`: navigates to
  `/settings/integrations`, asserts all seven `#<slug>-enabled` switches are
  visible, toggles GitHub's off, asserts the URL never left the index page,
  saves via the shared floating save bar, then navigates to
  `/settings/integrations/github` and asserts its own-page slider reflects
  the same persisted "off" state.
- `hide-disabled-integrations-nav.spec.ts`: makes GitHub
  configured/authenticated via `apiClient.mockGitHubSetUser`, disables
  GitHub's slider and saves, navigates to `/tasks` (leaving the Settings
  takeover, which replaces the sidebar with the Settings tree — confirmed by
  inspecting a live DOM snapshot before writing this) and asserts the
  sidebar's GitHub link is still visible (hide-disabled off, the default);
  returns to Settings, turns "Hide disabled integrations from left panel
  navigation" on and saves, navigates back to `/tasks`, asserts the GitHub
  link is now hidden; re-enables GitHub and saves, navigates back to
  `/tasks`, asserts the link reappears.
- This exposed a real bug during development, fixed by the corrected Task
  03 markup: nesting the slider inline with the label made "Azure DevOps"
  wrap to two lines under the layout spec's real browser measurements — the
  fix landed in Task 03, and this task's runs are the confirming evidence
  that it holds end-to-end.
- Environment note: running Playwright locally required building
  prerequisites this sandbox does not have pre-built —
  `make build-backend` (needed a Go 1.26 toolchain found at
  `/tmp/go1.26/bin`, not on the default `PATH`), `make build-web`, and
  `make -C apps/backend e2e-plugin-package` — each run once, then re-run
  after every source change under test.
- Commands (via a managed background process so the tool-call timeout
  couldn't truncate a multi-minute Playwright run):
  `npx playwright test --config e2e/playwright.config.ts e2e/tests/integrations/integrations-index-enabled-toggle.spec.ts e2e/tests/integrations/hide-disabled-integrations-nav.spec.ts e2e/tests/integrations/integrations-index-layout.spec.ts`
  → `3 passed (30.5s)`.
- Files changed: the two new spec files listed above (Task 03 already
  updated the layout spec's helper; this task only re-ran it, unmodified,
  as the regression check — no fix was needed here).
- Blockers/risks: none. All background Playwright/Chromium processes and
  the temporary manual backend instance used for diagnosis were terminated
  after verification.
