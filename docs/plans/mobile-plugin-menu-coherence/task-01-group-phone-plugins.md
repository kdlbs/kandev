---
id: "01-group-phone-plugins"
title: "Group phone plugin controls"
status: done
wave: 1
depends_on: []
plan: plan.md
requirements:
  - REQ-UI-MOBILE-MENU-007
  - REQ-UI-MOBILE-TASK-CHROME-001
acceptance_criteria:
  - AC-UI-MOBILE-MENU-007.2
  - AC-UI-MOBILE-MENU-007.4
  - AC-UI-MOBILE-TASK-CHROME-001.6
  - AC-UI-MOBILE-TASK-CHROME-001.7
system_design:
  - ../../specs/ui/system-design/unified-mobile-navigation.md
  - ../../specs/ui/system-design/mobile-task-chrome.md
---

# Task 01: Group Phone Plugin Controls

## Summary

Compose workspace and task plugin widgets inside the same phone Plugins section,
then place fallback metrics after navigation. Preserve plugin context and all
existing controls; publish synthetic mobile screenshots with the PR.

## In scope

Phone composition, empty-state gating, touch sizing, unit/E2E regressions,
scoped guidance, public navigation copy, and isolated PR capture.

## Out of scope

Plugin SDK/backend changes, plugin deduplication, new settings, and desktop redesign.

## Acceptance

1. Main-toolbar, sidebar workspace, and task contributions appear under one
   Plugins heading. Context labels distinguish workspace and task groups when
   both exist. Empty sections disappear; saved-layout links are not duplicated.
2. Resources no longer interrupt plugin controls. Existing preference gates,
   context, actions, focus return, and the sole menu scroller remain intact.
3. Touch controls are contained and at least 44px; focused tests pass, and
   fresh isolated phone screenshots are inspected before PR publication.

## ASCII UI preview

UI-01 excerpt from [the plan](plan.md#ui-01-phone-app-navigation-plugins-installed):

```text
Tasks > / Automations >
Plugins
  Workspace: [controls]
  Task:      [controls]
  [destinations]
Integrations >
System metrics
Utilities
```

The menu keeps its fixed header and single internal scroller. UI-01D keeps
desktop/tablet controls in their existing sidebar and inline toolbars.
Maps to AC-UI-MOBILE-MENU-007.2/.4 and AC-UI-MOBILE-TASK-CHROME-001.6/.7.

## Files likely touched

- `apps/web/components/navigation/app-nav-{sheet,sections}.tsx` and existing tests.
- `apps/web/components/plugins/mobile-plugin-nav-section.tsx` and existing tests.
- `apps/web/components/app-sidebar/app-sidebar-workspace-actions.tsx`.
- `apps/web/components/kanban/main-top-bar-plugin-actions.tsx`.
- `apps/web/e2e/tests/plugins/mobile-plugin-topbar.spec.ts`.
- Scoped web guidance and existing navigation/plugin public documentation.

## Verification

```bash
(cd apps/web && pnpm exec vitest run components/navigation/app-nav-sheet.test.tsx components/plugins/mobile-plugin-nav-section.test.tsx components/kanban/main-top-bar-plugin-actions.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && CAPTURE_PR_ASSETS=1 pnpm e2e:run --host --project mobile-chrome tests/plugins/mobile-plugin-topbar.spec.ts tests/settings/mobile-resource-metrics-display.spec.ts tests/layout/mobile-menu-hierarchy.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

Run targeted ESLint/Prettier on changed frontend files as well.

## Dependencies

None. Existing plugin-menu and unified-navigation packages are implemented.

## Parallelism

Sequential, in the primary session.

## Inputs

- Unified mobile navigation requirements/design, action-first composition.
- Mobile task chrome requirements/design, plugin presentation/context.
- Existing packaged E2E plugin and mobile plugin/resource scenarios.

## Risks

Opaque plugin markup may be wider than the menu. Preserve existing mobile
containment and test host buttons and raw sidebar controls separately.

## Results

- RED: the unit grouping assertion failed on `closest(...) === null`, and the
  browser assertion returned false for workspace controls inside Plugins.
- Focused Vitest command: 44 tests passed. Typecheck and changed-file
  ESLint/Prettier passed without warnings; the i18n ratchet passed.
- Final browser command: `pnpm run build:e2e`, followed by the listed managed
  capture command with `--no-build`, passed all 15 scenarios. Backend and fixture
  packages were freshly built in the preceding managed runs. The existing
  hierarchy assertion was updated from the old 24px gap to the intended 16px.
- Four PNGs captured in the ignored PR-asset manifest using isolated E2E data;
  dark/light task views were visually checked against UI-01. Listing-only and
  no-plugin states retain resources and navigation without an empty plugin group.
- Catalog/specification lint passed. Public-doc validators passed 62 tests and
  47 pages. `git diff --check` passed. Desktop/tablet composition is covered by
  retained slot tests and the resource suite's 768px boundary; this phone-only
  surface does not require an unrelated desktop screenshot.

### PR CI and review follow-up

- Added direct AppNavSheet coverage of both status-bar preference branches and
  selected the Plugins region by its accessible name in grouping assertions.
- The mobile autopilot CI failure reproduced locally with retries disabled.
  Its resumed child can finish before a state poll sees RUNNING; assert the
  durable second turn, as the desktop test already does.
- The history-recovery flake did not reproduce in six fresh two-core runs or
  after its four preceding CI specs. An experiment that settled the initial
  turn and avoided the reload-capable entry helper still failed on repetition
  two; it was not retained. The cause remains unresolved. Failure artifacts
  and the experimental patch are handed to the owner of PR #3890, which will
  consolidate this work and own combined verification and new screenshots.
- Reproduction used `taskset -c 0,1 pnpm e2e:run --host --no-build --project
  mobile-chrome` with `--retries=0 --max-failures=1`. Six isolated history runs
  passed, followed by all ten tests across display settings, automation
  webhooks, PR link copying, port forwarding, history recovery, and autopilot.
  The history suite's retry override was temporarily removed for these runs
  and restored when the unsuccessful experiment was removed.
  No production behavior or screenshot changed during this follow-up.

## Subsequent toolbar-selection correction

The [September 24 follow-up](../mobile-plugin-deduplication/plan.md) supersedes
the Workspace/Task subheadings and simultaneous toolbar rendering described
here. Phone navigation now selects each plugin's task toolbar when task actions
exist, while preserving workspace-only controls and independent sidebar actions.
The follow-up records current regression coverage and seeded screenshots; the
results above remain the historical September 23 delivery record.
