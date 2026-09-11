---
id: "06-toolbar-page-picker"
title: "Refine mobile toolbar and page picker"
status: complete
wave: 6
depends_on: ["05-pagination"]
plan: plan.md
requirements:
  - REQ-INTEGRATIONS-GITHUB-MOBILE-001
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-MOBILE-001.2
  - AC-INTEGRATIONS-GITHUB-MOBILE-001.4
  - AC-INTEGRATIONS-GITHUB-MOBILE-001.5
system_design:
  - ../../specs/integrations/system-design/github-dashboard-mobile.md
---
# Refine mobile toolbar and page picker

## Summary and scope

Continue the implemented package from user screenshot feedback: a long saved-view label squeezed the bare result count and updated time into the same row, and pagination opened an operating-system select menu. Separate the toolbar hierarchy and use the existing in-app bottom-drawer interaction for page choice.

## Out of scope

Provider API or persistence changes, new locale keys, desktop pagination redesign, and implementation in the other integration repositories. Those providers have separate [planning tasks](integration-followups.md).

## Acceptance

- A long Views label stays within a single 44px trigger at 320px; its full accessible name and drawer label remain available. Results, updated time, and Refresh occupy a separate row below the input.
- The page button opens a bounded bottom drawer, reveals the selected page, and offers 44px rows. Selecting another page calls the existing navigation once; selecting the current page only closes.
- Dismissal restores focus, page 40 stays reachable, and no horizontal overflow occurs at 320px or either side of the 767/768px boundary. Desktop pagination and shared GitLab toolbar behavior remain intact.

## ASCII UI preview

### UI-06: Toolbar hierarchy and in-app page choice

```text
[Views: Long saved query name... v]
[Repository]
[Query]
Results 75           Updated just now [Refresh]

[Previous] [Page 1 of 3 v] [Next]
Drawer: Choose results page      [Done]
  Page 1 of 3 (selected)
  Page 2 of 3
  Page 3 of 3
```

Geometry and hierarchy are structural; ASCII spacing is illustrative. Product copy uses existing translations.

## Files and ownership

Own `components/github/my-github/mobile-views-picker.tsx`, `mobile-results-pagination.tsx`, `results-pagination.tsx`, the shared `components/integrations/integration-list-toolbar.tsx`, adjacent unit tests, and `e2e/tests/github/mobile-github-results.spec.ts` under `apps/web`. Update the paired integration spec/design and existing public mobile instructions. Do not change other providers' domain logic.

## Dependencies and risks

Task 05 is complete. The shared toolbar affects GitLab and plugin host consumers, so retain its prop contract and desktop composition. Browser tests, not jsdom animation teardown, own actual dialog dismissal and focus-return assertions. Updating the existing disposable demo must preserve its database and in-memory provider fixtures; refresh static assets without restarting its backend.

## Verification

From `apps/web`, run the changed pagination/toolbar Vitest files, focused ESLint, typecheck, and i18n checks. Run managed Playwright with one worker and fresh assets:

```bash
pnpm e2e:run --host --no-build --project mobile-chrome tests/github/mobile-github-results.spec.ts tests/github/mobile-github-view-recovery.spec.ts tests/github/mobile-github-sidebar.spec.ts
pnpm e2e:run --host --no-build --project chromium tests/github/github-scope-bar.spec.ts
pnpm e2e:run --host --no-build --project mobile-chrome tests/gitlab/mobile-gitlab-parity.spec.ts --grep 'browses, quick launches'
```

Run specification/public-doc validators and `git diff --check` from the repository root. Inspect the refreshed isolated demo at phone size; do not touch the main runtime on :9998.

## Parallelism

Sequential implementation in the primary session. No delegated code changes.

## Results

Red tests reproduced the toolbar's refresh button sharing the long Views row (y=62 while the Views trigger ended at y=114), and the old page control being SELECT rather than BUTTON. Updated behavior passes six focused unit tests, ten mobile GitHub browser scenarios, four desktop GitHub regressions, and the GitLab mobile browse/review regression. Final typecheck, focused zero-warning lint, translation checks, spec/doc validators, and whitespace checks passed.

Refreshed the existing isolated instance's assets without restarting its backend or resetting data. Browser checks through its Tailscale HTTPS URL confirmed the 44px long-view trigger at 320px and 393px, separate Results 75 and Updated just now metadata, three-page drawer navigation, focus return, and no horizontal overflow. Inspected captures: `/tmp/kandev-mobile-parity-test-vUutse/browser-artifacts/refined-mobile-toolbar.png` and `refined-mobile-page-picker.png`.

Open PR revalidation on 2026-09-11 now waits for finite drawer animations before measuring page targets. The capped-page fixture shapes the mock provider's unpaged response into GitHub's bounded pages while preserving its total count; the scenario asserts 25 rendered rows and reaches page 40 without rendering all 1,050 seeded rows on every navigation. Both scenarios in `mobile-github-results.spec.ts` pass. Fresh desktop and phone PR screenshots use separate managed fixtures; the stopped demo remains untouched.
