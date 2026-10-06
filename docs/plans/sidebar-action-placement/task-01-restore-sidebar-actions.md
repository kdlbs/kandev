---
id: "01-restore-sidebar-actions"
title: "Restore sidebar action placement"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-NAV-HIERARCHY-001
  - REQ-UI-NAV-HIERARCHY-003
acceptance_criteria:
  - AC-UI-NAV-HIERARCHY-001.1
  - AC-UI-NAV-HIERARCHY-001.2
  - AC-UI-NAV-HIERARCHY-001.3
  - AC-UI-NAV-HIERARCHY-001.4
  - AC-UI-NAV-HIERARCHY-001.5
  - AC-UI-NAV-HIERARCHY-001.6
  - AC-UI-NAV-HIERARCHY-001.7
  - AC-UI-NAV-HIERARCHY-003.1
  - AC-UI-NAV-HIERARCHY-003.2
  - AC-UI-NAV-HIERARCHY-003.3
  - AC-UI-NAV-HIERARCHY-003.5
  - AC-UI-NAV-HIERARCHY-003.6
system_design:
  - ../../specs/ui/system-design/navigation-hierarchy.md
---

# Task 01: Restore sidebar action placement

## Summary

Restore the earlier desktop action arrangement with the current navigation hierarchy.
Keep action ownership, workspace eligibility, and phone navigation intact.

## In scope

- Read `/tdd`, `/mobile-parity`, `/e2e`, and `apps/web/AGENTS.md` before implementation.
  Install workspace dependencies once if this worktree has no valid installation.
- Add failing behavioral and geometry assertions for the requested placement.
  Retain existing routing, activity, plugin failure, and settings-guard tests.
- Restore the compact creation row with Terminal then Quick Chat icons on its right.
  Reuse `AppSidebarNavItem`, `SurfaceAction`, and current launchers.
  Keep plugin slot context stable and permit excess plugin controls to wrap.
- Place navigation header actions before the rightmost chevron as separate controls.
  Restore eligible first-party integration shortcuts with four-icon fine-pointer capacity and two-icon coarse-pointer capacity.
- Move built-in Stats out of the utilities menu into `FooterIconButton` before `ThemeToggle`.
  Preserve plugin menu ordering, release notifications, auth identity, and navigation guards.
- Update the scoped engineering guide and the existing public navigation explanation.
  Use existing localized labels; translate new copy only if implementation requires it.
- Extend the desktop navigation E2E and phone regression scenario.
  Capture focused rendered screenshots as test artifacts.

## Out of scope

Backend changes, provider APIs, saved-layout migrations, task rows, default ordering,
phone composition changes, new flags/preferences, commits, push, and PR publication.

## Acceptance

1. Desktop geometry and activation satisfy 001.1 through 001.7 in default, saved, and collapsed layouts.
   New Task, Terminal, and Quick Chat share one row; header shortcuts navigate without toggling.
2. Stats has one footer control before theme switching and no menu entry.
   Remaining plugin utilities, settings guards, account gates, and release notes remain functional.
3. Targeted tests pass against a fresh build, including phone regressions and coarse-pointer containment.
   Public documentation and the scoped guide describe the implemented placement.

## ASCII UI preview

UI-01 excerpt: desktop default expanded sidebar. Full [preview](plan.md#ascii-ui-preview).

```text
[+ New Task] [terminal] [chat]
Home
Automations                  >
Canvases       [settings]    >
Integrations   [GH] [GL]     >
TASKS [existing panel]
Settings [Stats] [theme] [...]
```

UI-02 excerpt: retained phone drawer.

```text
Menu                    [x]   fixed
[Workspace v]                 fixed
[ + New Task              ]
Home
[ Quick Chat | Terminal    ]
[tools before Tasks]
TASKS [existing touch rows]
Utilities: Stats, Settings
[safe-area clearance]
```

UI-01 maps to 001.1 through 001.7.
UI-02 maps to 003.1, 003.2, 003.3, 003.5, and 003.6.
The phone drawer retains one scroller, 44px touch targets, and close-before-launch behavior.
Header actions remain siblings of the label toggle and chevron.
No nested interactive controls or saved-layout rewrites are permitted.

## Verification

Run from repository root. Each command has its own working directory.
If dependencies are missing, first run `(cd apps && pnpm install --frozen-lockfile)`.

```bash
(cd apps/web && pnpm exec vitest run components/app-sidebar/app-sidebar-new-task-item.test.tsx components/app-sidebar/app-sidebar-section.test.tsx components/app-sidebar/sections/canvases-section.test.tsx components/app-sidebar/sections/integrations-section.test.tsx components/app-sidebar/app-sidebar-footer.test.tsx components/navigation/app-nav-sheet.test.tsx)
(cd apps/web && pnpm e2e:run --project chromium tests/layout/navigation-hierarchy.spec.ts tests/settings/sidebar-customization.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/layout/mobile-navigation-hierarchy.spec.ts tests/layout/mobile-unified-navigation.spec.ts tests/layout/mobile-menu-hierarchy.spec.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint components/app-sidebar --max-warnings 0)
(cd apps/web && pnpm run i18n:check)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

The managed E2E commands build current production assets and own teardown.
Run desktop and phone commands sequentially without worker overrides.
Add assertions for 24px fine-pointer quick-action icons and 44px coarse-pointer actions.
Assert canvas/integration shortcut bounds before the chevron and inside the sidebar.
Use rendered bounds for the Stats/theme order and one-row footer.
Update every old Stats-menu assertion, including the tablet scenario.
The phone scenario taps New Task, Quick Chat, Terminal, and Utilities Stats through real visible controls.
Check drawer containment, focus handoff, and document overflow at 360px, 393px, and 767px.
Capture dark/light desktop images and a Portuguese long-label image.

Run this PR-documentation preflight from repository root:

```bash
node <<'NODE'
const fs = require('node:fs');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const packagePaths = [
  'docs/plans/sidebar-action-placement/plan.md',
  'docs/plans/sidebar-action-placement/task-01-restore-sidebar-actions.md',
  'docs/specs/ui/requirements/navigation-hierarchy.md',
  'docs/specs/ui/system-design/navigation-hierarchy.md',
];
const fileContents = Object.fromEntries(packagePaths.map(path => [path, fs.readFileSync(path, 'utf8')]));
const result = validateCoverage({
  changedFiles: [
    { filename: 'apps/web/components/app-sidebar/app-sidebar-new-task-item.tsx', status: 'modified' },
    { filename: packagePaths[1], status: 'added' },
  ],
  fileContents,
});
console.log(JSON.stringify(result, null, 2));
if (!result.ok) process.exitCode = 1;
NODE
```

This preflight uses the planned production entry point before implementation.
Require `ok: true` and record the result.

## Files likely touched

- `apps/web/components/app-sidebar/app-sidebar-new-task-item.tsx` and its test.
- `apps/web/components/app-sidebar/app-sidebar-workspace-actions.tsx`.
- `apps/web/components/app-sidebar/app-sidebar-section.tsx` and its test.
- `apps/web/components/app-sidebar/sections/canvases-section.test.tsx`.
- `apps/web/components/app-sidebar/sections/integrations-section.tsx` and its test.
- `apps/web/components/app-sidebar/app-sidebar-footer.tsx` and its test.
- `apps/web/components/navigation/app-nav-sheet.test.tsx`.
- `apps/web/e2e/tests/layout/navigation-hierarchy.spec.ts`.
- `apps/web/e2e/tests/layout/mobile-navigation-hierarchy.spec.ts`.
- `apps/web/AGENTS.md` and `docs/public/use-kandev.md`.
- The requirement/design pair and this package's status/results after delivery.

## Dependencies

None. The merged PR #4063 is the current source baseline.

## Risks

The compact row needs bounded built-in controls and wrapping plugin controls.
Header shortcuts must use current eligibility instead of copied provider lists.
The phone creation primitive must retain its existing size and dialog lifetime.

## Parallelism

`sequential`

## Inputs

- [Navigation requirements](../../specs/ui/requirements/navigation-hierarchy.md), requirements 001 and 003.
- [Navigation design](../../specs/ui/system-design/navigation-hierarchy.md), desktop composition, footer, and phone composition.
- [Control sizing](../../specs/ui/requirements/control-sizing.md).
- [Navigation manifest ADR](../../decisions/2026-08-04-navigation-manifest-boundaries.md).
- [Phone entry-point ADR](../../decisions/2026-09-15-phone-navigation-entry-points.md).
- Current sidebar components/tests and the earlier presentation in `dd7dfa81634^`.

## Results

Delivered the compact desktop creation row, independent section-header actions, first-party integration shortcuts, and direct Stats footer control. The labelled phone drawer and existing launchers remain intact. Updated the scoped guide and public sidebar navigation explanation.

Verification passed on 2026-10-05:

- Focused component tests: 106 tests passed across six suites.
- Managed Chromium E2E: 9 tests passed. Captured `navigation-dark.png`, `navigation-light.png`, and `navigation-portuguese-long-label.png` as test artifacts.
- Managed mobile-Chrome E2E: 19 tests passed. Checked 360px, 393px, and 767px drawer bounds, focus return, direct Stats navigation, and document overflow.
- `pnpm run typecheck`, `pnpm exec eslint components/app-sidebar --max-warnings 0`, and `pnpm run i18n:check` passed.
- `node --test scripts/validate-public-docs.test.mjs`: 62 tests passed; `node scripts/validate-public-docs.mjs` validated 47 published pages.
- `python3 scripts/list-docs.py validate` validated 351 decisions and 1366 specifications; `python3 scripts/lint-spec-files.test.py` passed 36 tests; `python3 scripts/lint-spec-files.py --all` passed.
- PR-documentation preflight returned `covered` with zero errors; `git diff --check` passed.

PR review follow-up passed on 2026-10-05:

- Integration shortcuts and Stats identify their current route with `aria-current` and the shared visible active treatment.
- Quick-action labels remain available on keyboard focus; a desktop E2E assertion verifies the integration shortcut's visible keyboard focus indicator.
- A focused phone-test comment records that Quick Terminal uses the shared Quick Chat dialog host.
- Focused component tests: 62 tests passed across three suites; managed Chromium E2E: 3 tests passed.

PR CI fixup on 2026-10-05:

- The first phone drawer geometry fix waited for finite animations before comparing parent and child bounds; a later CI run showed separate bounding-box reads could still straddle an animation frame.

PR CI fixup on 2026-10-06:

- Replaced stale plugin-action geometry assumptions with the 24px native action size and inline-or-wrapped plugin placement contract.
- The complete `plugin-action-ux.spec.ts` passed all three tests in the constrained CI image with retries disabled.
- The phone drawer geometry assertion now reads the menu and action bounds in one browser task; its target test passed four repeated runs and the full mobile navigation hierarchy passed all four tests with retries disabled.
- Scoped the disabled-integration navigation assertion to the integration section, excluding its separate always-visible header shortcut; the integration navigation spec passed with retries disabled.
- Updated the Quick Chat focus test for the New Task, Quick Terminal, Quick Chat tab order and asserted the terminal tooltip on keyboard focus; both previously failing tests passed with retries disabled.
- ESLint and Prettier passed for the four affected E2E files, and the production Vite build passed.
