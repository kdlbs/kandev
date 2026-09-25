---
id: "05-action-first-menu"
title: "Prioritize quick actions and collapse Integrations"
status: done
wave: 5
depends_on: ["04-menu-hierarchy"]
plan: plan.md
requirements:
  - REQ-UI-MOBILE-MENU-007
acceptance_criteria:
  - AC-UI-MOBILE-MENU-007.1
  - AC-UI-MOBILE-MENU-007.2
  - AC-UI-MOBILE-MENU-007.3
  - AC-UI-MOBILE-MENU-007.4
system_design:
  - ../../specs/ui/system-design/unified-mobile-navigation.md
---

# Task 05: Prioritize quick actions and collapse Integrations

## Summary

Group the two built-in quick actions directly under Home and make Integrations
collapsible like Automations. Keep the existing shared menu/controller and
workspace/provider boundaries. The populated isolated preview is already seeded.

## In scope

Phone-only action grouping, section ordering, Integrations disclosure, Settings
before Stats, localized label fit, focused regressions and refreshed preview.

## Out of scope

Backend/provider changes, saved disclosure preferences, new navigation routes,
desktop redesign, workspace-picker redesign, changing task/sidebar domain logic,
commits, pushes or delegation.

## Acceptance

1. Home precedes the two-column quick-action row, which precedes local navigation
   and Tasks. Both launchers retain workspace, focus and activity behavior.
   Named sections and utilities follow the agreed order (AC .1 and .2).
2. Integrations starts collapsed, toggles accessibly, and reveals configured
   providers and settings without stale workspace entries. Empty still offers
   settings. Wider default behavior is retained (AC .3 and .4).
3. Phone geometry, task/dialog lifetime and provider gates pass targeted tests;
   final populated Tailscale preview demonstrates both integration and automation
   links. Main :9998 and the seeded workspaces/tasks are untouched (AC .4).

## ASCII UI preview

UI-05 phone, excerpt from the [full preview](plan.md#ascii-ui-preview):

```text
[Home]
[Quick Chat] [Quick terminal]
Tasks v                       +
  Task sidebar
Automations >       [Open list]
Integrations >
Utilities
  Settings / Stats / Theme / Support
```

Expanded Integrations: provider links then Integration settings. Empty expanded:
Integration settings only. Collapsed: heading/chevron only. UI-05D wider: retain
existing sidebar/menu composition. Order/grouping and touch sizes are required;
ASCII spacing is illustrative. The fixed title and sole content scroller remain.

## Implementation sequence

1. Add focused failing unit checks for ordering and disclosure. Add phone
   scenarios to the existing hierarchy suite for quick actions and populated/
   empty Integrations, preserving existing browser assertions by expanding the
   section before querying descendants.
2. Separate built-in quick-action presentation from the other content in
   MobileListingMenuActions; retain all launch hooks and caller defaults. Wire
   phone grouping, explicit Integrations disclosure and local utility ordering.
3. Run checks, inspect 393px/767px screenshots in both themes and translated
   label wrapping, refresh only the existing isolated frontend and smoke test.

## Files likely touched

- `apps/web/components/navigation/app-nav-sheet.tsx` and `.test.tsx`.
- `apps/web/components/navigation/app-nav-sections.tsx`.
- `apps/web/components/kanban/mobile-listing-menu-actions.tsx` and a focused test
  for any extracted quick-action boundary.
- `apps/web/components/integrations/integrations-menu.tsx`, existing `.test.ts`,
  and a new `mobile-integrations-section.test.tsx` for rendered disclosure.
- `apps/web/e2e/tests/layout/mobile-menu-hierarchy.spec.ts`.
- `apps/web/e2e/tests/layout/mobile-unified-navigation.spec.ts`, existing quick
  chat/terminal and plugin tests only where their navigation assumptions change.
- `docs/public/sessions-and-review.md` and scoped web guidance where affected.

## Tests and verification

Unit mapping: app-nav-sheet tests own AC .1/.2, rendered integrations tests own
AC .3/.4, existing integration resolver tests retain permission behavior. Use
real hooks/resolvers where practical and mock API/provider boundaries.

Browser mapping: hierarchy suite owns AC .1-.4 with ordered bounding boxes,
44px targets, collapsed/expanded populated and empty integrations, keyboard
activation, and actual link/quick-action outcomes. Verify phone Home and seeded
workbench, workspace changes and 767/768 boundary. Include 393px/767px dark/light
screenshots and a long-label/pseudo-locale geometry check. Preserve plugin and
fallback metric access; only built-in quick actions enter the two-column row.

Exact commands from repository root (add planned test before its command):

```bash
(cd apps/web && pnpm exec vitest run components/navigation/app-nav-sheet.test.tsx components/navigation/destination-rows.test.tsx components/navigation/mobile-automations-section.test.tsx components/integrations/integrations-menu.test.ts components/integrations/mobile-integrations-section.test.tsx)
(cd apps/web && pnpm exec eslint components/navigation/app-nav-sheet.tsx components/navigation/app-nav-sections.tsx components/kanban/mobile-listing-menu-actions.tsx components/integrations/integrations-menu.tsx components/integrations/mobile-integrations-section.test.tsx e2e/tests/layout/mobile-menu-hierarchy.spec.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm run build:vite)
(cd apps/web && E2E_PORT_OFFSET=29 pnpm e2e:run --host --no-build --project mobile-chrome tests/layout/mobile-menu-hierarchy.spec.ts tests/layout/mobile-unified-navigation.spec.ts tests/chat/mobile-quick-chat-entry.spec.ts tests/terminal/mobile-quick-terminal.spec.ts tests/plugins/mobile-plugin-nav.spec.ts tests/plugins/mobile-plugin-topbar.spec.ts tests/integrations/mobile-integrations-nav.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run the existing two compact-desktop cases with the Task 04 temporary
filename-anchored exclusion workaround, then delete that temporary config:

```bash
(cd apps/web && E2E_PORT_OFFSET=29 pnpm e2e:run --host --no-build --config e2e/navigation-check.playwright.config.ts --project chromium tests/layout/compact-desktop-responsive.spec.ts)
```

Keep one browser worker; do not overlap suites or rebuild during a run. Add
exact commands/results for any newly extracted test or migrated suite before
completion. Refresh the preview from final assets; adapt its temporary smoke
script to expand Integrations and assert both sample automation links and GitHub.

## Inputs and continuation context

REQ-UI-MOBILE-MENU-007 and the paired design's Action-first composition section.
Task 04 is done (71 units, 62 browser cases); its results remain historical.
User confirmed order and explicitly requested collapsible Integrations. Initial
collapse is a routine consistency choice matching Automations, not new persistence.
No material unresolved question. UI owns reusable navigation presentation;
integration configuration and availability remain integration-owned.

Preview: http://100.105.155.17:48439. Root:
`/tmp/kandev-mobile-menu-test-hw9njt_n`. Reapply only mock provider fixtures after
restart with `python3 /tmp/kandev-mobile-menu-test-hw9njt_n/enrich-menu.py` (it
retains existing automation IDs). Main :9998 must never be used. Shutdown:
`python3 /tmp/kandev-mobile-menu-test-hw9njt_n/stop.py`.

## Dependencies and risks

Task 04 complete. Moving the entire actions slot would move plugin controls and
metrics into the quick row; separate built-ins. Shared integrations also has
wider consumers; opt in explicitly. Partial Settings selectors can match
Integration settings; use exact names. Keep 44px targets and wrapping translated
labels. Do not remount task controllers or duplicate quick launch state.

## Parallelism

Sequential in the primary session; no delegation authorized.

## Results

Implementation authorized by user on 2026-09-19. Phone composition and
Integrations disclosure implemented. Three unit regressions failed before
production changes (quick-action ordering and collapsed integration behavior).
Final focused unit run: 46 tests passed across five files. After correcting
unit-only Testing Library option types, the two changed unit suites passed
again (26 tests). Typecheck, targeted ESLint, i18n validation/ratchet, production
build, public-doc tests/validation, specification validation and diff checks passed.

27 distinct mobile browser cases and two compact-desktop cases passed. The
initial mobile batch passed 25; two new geometry checks measured different
frames of the opening drawer animation. Waiting for finite Web Animations
before reading bounds fixed the tests without changing product code or timeouts.
The focused rerun command was the same mobile runner with
`tests/layout/mobile-menu-hierarchy.spec.ts --grep 'quick actions precede'`.
The temporary desktop config was removed after both desktop cases passed.

Screenshots and browser assertions cover 393px/767px, dark/light and Portuguese
label fit. Quick Chat and terminal flows, workspace context, task rotation,
provider navigation and plugin/metric access passed. The direct Tailscale preview
smoke passed on the final production bundle with the seeded GitHub connection
and both automations. Only frontend assets were refreshed, publishing index.html
last; the process and mock provider state remained intact. Main :9998 untouched.

Browser logs: `/tmp/menu5-browser.log`, `/tmp/menu5-rerun.log`,
`/tmp/menu5-desktop.log`, `/tmp/menu5-preview.log`. Preview screenshots:
`/tmp/kandev-mobile-menu-test-hw9njt_n/action-first-menu.png` and
`/tmp/kandev-mobile-menu-test-hw9njt_n/populated-menu.png`.

Logs: `/tmp/menu5-*.log`. The existing integrations browser test now explicitly
expands its section before navigating. Its suite is included above.

### Collapsed spacing follow-up

User screenshot exposed Utilities retaining desktop footer anchoring (`mt-auto`)
inside the growing phone drawer. A direct Tailscale browser reproduction measured
349px instead of the normal 24px at 393px width / 1200px height. The permanent
hierarchy regression checks 393px and 767px. Two initial managed runner attempts
stopped at backend readiness before executing the test; the already-running
isolated preview supplied the behavioral red evidence. The fix scopes the
automatic margin to non-phone menus. Final scoped ESLint and production build
passed. The managed browser regression passed (1 case, 393px and 767px), and
the direct Tailscale check measured the expected 24px gap at both widths.
Visual inspection at 393px confirmed consistent section spacing. Specification
catalog/lint and diff checks passed. Logs: `/tmp/menu-gap-{red,red2,green}.log`,
`/tmp/menu-gap-preview-{red,green}.log`, `/tmp/menu-gap-build.log`.

Exact browser command from `apps/web`:
`E2E_PORT_OFFSET=29 pnpm e2e:run --host --no-build --project mobile-chrome tests/layout/mobile-menu-hierarchy.spec.ts --grep 'flexible space'`.
The isolated frontend was refreshed; seeded providers/automations and main :9998
were untouched.
