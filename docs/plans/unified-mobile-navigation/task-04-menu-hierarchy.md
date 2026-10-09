---
id: "04-menu-hierarchy"
title: "Simplify Home and complete menu sections"
status: done
wave: 4
depends_on: ["03-embedded-tasks"]
plan: plan.md
requirements:
  - REQ-UI-MOBILE-MENU-006
acceptance_criteria:
  - AC-UI-MOBILE-MENU-006.1
  - AC-UI-MOBILE-MENU-006.2
  - AC-UI-MOBILE-MENU-006.3
  - AC-UI-MOBILE-MENU-006.4
system_design:
  - ../../specs/ui/system-design/unified-mobile-navigation.md
---

# Task 04: Simplify Home and complete menu sections

## Summary

Remove redundant phone Tasks/Threads route buttons. Make the embedded Tasks
heading match Utilities while retaining its plus action. Expose Automations and
keep Integrations discoverable in unconfigured workspaces.

## Scope and exclusions

Own phone navigation presentation, workspace-scoped automation read reuse,
integration setup discoverability, tests, localized copy, and public instructions.
Preserve desktop/tablet composition, Office local navigation, palette/routes,
provider gates, task sidebar state and dialogs. No credential changes, automatic
runs, new persistence, or backend APIs. Main instance :9998 is excluded.

## Acceptance

1. Home alone represents listing routes; mode selection and current state work
   across Kanban/List/Threads without altering saved preferences (.1).
2. Tasks visually matches Utilities, with collapse and a separate plus that
   preserve drafts, touch size, focus and one-scroller geometry (.2).
3. Automations list/detail/setup and integration setup/provider navigation work
   with configured, empty, loading, failed and workspace-switch states (.3-.4).

## ASCII UI preview

UI-04: Phone hamburger, regular workspace; Tasks expanded.

```text
Menu                                  x
Workspace [Pocket Planner v]
[Home]
---------------------------------------
Tasks v                               +
  [Saved views / filters]
  Task rows
[Quick Chat] [Quick terminal]
---------------------------------------
Automations >                  [Open list]
---------------------------------------
Integrations
  Configured provider links
  [Integration settings]
---------------------------------------
Utilities
  [Stats] [Settings] [Theme] ...
```

Tasks collapsed: heading and plus remain, body hidden. Automations expanded:
existing names/status rows and setup action; empty shows setup, failed shows
retry. Integrations empty: setup/settings remains, provider links absent.
The Tasks and Utilities heading text aligns; dividers/spacing share tokens.
The menu heading stays fixed; all sections share one vertical scroller.

UI-04D: Desktop/tablet keep their current sidebar and palette composition.
This is a phone-only menu revision. See the full preview in [plan](plan.md).

## Implementation sequence

1. Add failing navigation tests for redundant links, Home current state,
   collapsible heading plus behavior, and missing/empty sections.
2. Implement phone presentation using shared domain hooks, verify permission and
   workspace boundaries, and migrate affected navigation tests.
3. Validate screenshots and behavior, update public instructions, then refresh
   only the existing isolated preview and smoke-test its Tailscale URL.

## Verification

Run from repository root; the browser runner builds current assets and uses one
worker. Add the new mobile scenario file before running its command.

```bash
(cd apps/web && pnpm exec vitest run components/navigation/app-nav-sheet.test.tsx components/navigation/destination-rows.test.tsx components/navigation/mobile-automations-section.test.tsx components/integrations/integrations-menu.test.ts components/app-sidebar/sections/automations-section.test.tsx lib/navigation/core-destinations.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && E2E_PORT_OFFSET=29 pnpm e2e:run --host --project mobile-chrome tests/layout/mobile-menu-hierarchy.spec.ts tests/layout/mobile-navigation-tasks.spec.ts tests/layout/mobile-unified-navigation.spec.ts tests/github/mobile-task-view-access.spec.ts tests/settings/mobile-startup-page.spec.ts tests/mobile-automation-detail.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run targeted ESLint on changed production files and any newly extracted section
unit tests. Search all phone tests for global Tasks/Threads links and migrate
only those entry points to the Home dropdown. Include every changed suite in
recorded checks. Use the established temporary filename-anchored Chromium config
for `compact-desktop-responsive.spec.ts`; remove it afterward. Inspect dark/light
393px and 767px screenshots and 768px composition, hit areas, overflow, plus
behavior while collapsed, and reachable Utilities after long lists.

## Files likely touched

- `apps/web/components/navigation/app-nav-sheet.tsx`, `app-nav-sections.tsx`,
  `destination-rows.tsx` and their tests.
- `apps/web/components/task/mobile/task-picker-surface.tsx`.
- `apps/web/components/integrations/integrations-menu.tsx` and tests.
- A mobile automation section alongside navigation, reusing
  `components/runs/use-workspace-automations.ts`, `use-automation-summaries.ts`,
  `automation-rows.ts`, and `runs-view.ts`.
- Relevant mobile E2E files, locales, public docs and scoped guidance.

## Dependencies and risks

Task 03 is done. Do not remove manifest routes: desktop, palette, and deep links
still need them. Desktop automation reads are phone-gated, so direct component
reuse would miss live summaries. Preserve integration availability rules and
use setup navigation for unavailable providers. Distinguish an empty automation
list from a failed request; stale reads must not leak across workspaces.

## Parallelism

Sequential in the primary session. No delegation authorized.

## Results

Implemented and verified on 2026-09-19 after explicit user authorization.
Final results: 71 focused unit tests and 62 distinct browser cases passed.

- Home-only phone destination with existing listing/palette routes retained.
  Tasks and Automations use Utilities-aligned headings with transparent expanded
  state, independent 44px actions, and the existing outer scroller.
- Phone Automations defers workspace reads until expansion, shares existing
  domain hooks and status derivation, masks stale workspace rows, and offers
  retry without exposing internal errors. Integrations retains settings while
  configured-provider links keep existing availability filtering.
- Three new browser cases failed against Task 03 before implementation.
  All 71 focused unit tests passed. Final changed navigation and automation
  suites were rerun successfully (22 and 4 cases respectively).
- Typecheck, targeted ESLint, i18n validation/ratchet, production build, public
  documentation checks, specification validation, and diff checks passed.
  Translation generators' unrelated rewrites were removed.
- Backend sources were unchanged. Localization generation touched embedded
  catalogs, requiring rebuilt host test binaries and the fixture plugin package
  for E2E freshness. The unnecessary cross-platform build was stopped after
  verifying its process identity; the host-only build completed.
- Refreshed http://100.105.155.17:48439 with the final build and preserved seed.
  Direct Tailscale smoke passed after startup: Home-only menu, Tasks collapse,
  Automations/setup, Integrations settings, dropdown options, seeded workbench.
  Inspected the phone screenshots and corrected inherited expanded-button fill.
  Main :9998 was not modified. Shutdown command remains
  `python3 /tmp/kandev-mobile-menu-test-hw9njt_n/stop.py`.

### Browser results

60 distinct mobile cases passed across the initial batch and focused reruns.
The initial 54-case batch passed 47: four failures were backend readiness,
two were page/navigation timeouts, and one was the now-ambiguous partial
Settings selector after adding Integration settings. Exact accessible-name
matching corrected the affected settings-entry tests. The new combined
Home-route test was split into three independently bounded route cases.
The 13-case rerun passed 12; the GitHub case stalled on a blank page before
navigation UI appeared and passed alone (9.6 seconds). No product workaround
or timeout increase was introduced. Four additional Office/system selector
regressions passed in the focused run.

All browser commands use `E2E_PORT_OFFSET=29 pnpm e2e:run --host --no-build`
from `apps/web`, one worker, after the successful final frontend build:

- Initial mobile-chrome batch: `tests/layout/mobile-menu-hierarchy.spec.ts`,
  `tests/layout/mobile-navigation-tasks.spec.ts`,
  `tests/layout/mobile-unified-navigation.spec.ts`,
  `tests/github/mobile-task-view-access.spec.ts`,
  `tests/settings/mobile-startup-page.spec.ts`,
  `tests/mobile-automation-detail.spec.ts`, `tests/kanban/mobile-kanban.spec.ts`.
- Focused mobile-chrome rerun: GitHub, Kanban, hierarchy and unified-navigation
  files above, plus `tests/system/mobile-storage-maintenance.spec.ts`,
  `tests/system/mobile-message-queue-settings.spec.ts`,
  `tests/office/mobile-office-navigation.spec.ts`; grep
  `Home owns|GitHub app navigation|Kanban navigation can|mobile menu exposes settings navigation|opens Storage from mobile navigation|mobile navigation reaches|mobile configuration lock|offers office sections|same menu from listings|separate listing controls|title picker switches`.
- Final mobile-chrome GitHub run: `tests/github/mobile-task-view-access.spec.ts`
  with `--grep 'GitHub app navigation'`.

Logs: `/tmp/menu4-browser.log`, `/tmp/menu4-rerun.log`,
`/tmp/menu4-github-final.log`; other checks use `/tmp/menu4-*.log`.

Desktop: both cases in `tests/layout/compact-desktop-responsive.spec.ts` passed
with `--config e2e/navigation-check.playwright.config.ts --project chromium`
(`/tmp/menu4-desktop.log`). The temporary config anchored the existing mobile
filename exclusion because this worktree path contains `on-mobile-`; it was
removed after verification. Final E2E-file ESLint also passed.
