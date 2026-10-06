---
created: 2026-10-05
status: complete
requirements:
  - REQ-UI-NAV-HIERARCHY-001
  - REQ-UI-NAV-HIERARCHY-003
system_design:
  - ../../specs/ui/system-design/navigation-hierarchy.md
legacy_specs: []
---

# Restore sidebar action placement

## Overview

Restore the desktop controls requested after PR #4063.
One work order implements the creation row, header shortcuts, and direct Stats control together.
The UI system owns this reusable navigation contract across pages.

Sources: [requirements](../../specs/ui/requirements/navigation-hierarchy.md),
[design](../../specs/ui/system-design/navigation-hierarchy.md), source HEAD
`48adb0ce739`, and earlier controls in `dd7dfa81634^`.
The [original package](../navigation-hierarchy/plan.md) retains its completed delivery evidence.

## Scope

In scope: desktop New Task row, quick-action icons, Canvases shortcut order,
Integrations header icons, Stats placement, accessible names, touch sizing,
focused regressions, and documentation alignment.

Out of scope: task-panel changes, navigation order changes, saved-layout migrations,
provider availability changes, new preferences, phone drawer redesign, and publication.

The request establishes desktop placement. The phone drawer retains labelled touch actions.
Excess plugin controls can wrap, while the built-in creation controls remain on one row.

## Technical approach

- Restore `AppSidebarNavItem` and `IconSquarePlus` in `AppSidebarNewTaskItem`.
  Place Terminal then Quick Chat on the right through the existing `SurfaceAction` primitive.
  Preserve launchers, activity names, request subscriptions, and dialog ownership.
  Keep `NewTaskButton` for `MobileNewTaskRow`.
- Update `NavigationSectionHeader` to render label toggle, sibling action slot, then chevron.
  Keep the labelled toggle's ref and disclosure semantics.
  Canvases retains `OpenCanvasSettingsShortcut` and its current workspace route.
- Restore `IntegrationHeaderShortcuts` from the pre-#4063 source using current resolved destinations.
  Show up to four first-party shortcuts for fine pointers, and two for coarse pointers.
  Keep all named children, empty-workspace setup, and plugin-layout ownership.
- Resolve built-in `stats` from existing static insight destinations.
  Render `FooterIconButton` before `ThemeToggle`; exclude Stats from `SidebarFooterMenu`.
  Keep plugin utilities, Improve Kandev, release notes, auth gates, and settings guards.
- During implementation, remove the stale desktop New Task sizing exception from `apps/web/AGENTS.md`.
  Update `docs/public/use-kandev.md` to explain the actual control placement.
  Keep public documentation unchanged until the behavior exists.

No schema, backend, navigation catalog, or API change is required.
The existing navigation-manifest and phone-entry-point ADRs remain sufficient.

## Mobile design contract

The existing hamburger opens `AppNavSurface`, the nearest shipped phone exemplar.
Its inset drawer serves a temporary navigation choice with shallow labelled destinations.
The header and workspace picker remain fixed above one content scroller.
The surface retains dynamic viewport bounds and bottom safe-area clearance.

New Task remains the primary phone action. `MobileQuickActions` retains its labelled pair.
Tool disclosures precede Tasks; Stats remains a labelled Utilities destination.
Phone actions close navigation before launch and preserve focus handoff.
Shared launchers, workspace state, and layout preferences retain their current ownership.
No phone composition is saved over desktop preferences.

Coarse-pointer desktop controls retain 44px targets.
Header capacity reduces on coarse pointers to keep controls inside the existing sidebar width.
All eligible integration destinations remain available as named children.

## ASCII UI preview

UI-01: default expanded desktop sidebar, Canvases and Integrations closed.

```text
CURRENT (#4063)                 PROPOSED
[        + New Task        ]    [+ New Task] [terminal] [chat]
[ Quick Chat | Terminal    ]    Home
Home                           Automations                  >
Automations                >   Canvases       [settings]    >
Canvases      > [settings]      Integrations   [GH] [GL]     >
Integrations               >   ------------------------------
----------------------------   TASKS [existing view/filter]
TASKS [existing view/filter]      [existing task rows]
----------------------------   ------------------------------
Settings [theme] [...]         Settings [Stats] [theme] [...]
  More: Stats + utilities        More: remaining utilities
```

The header and footer stay fixed outside task scrolling.
New Task matches the 36px navigation row; quick-action icons use compact 24px targets on fine pointers.
Both expand to at least 44px for coarse pointers.
Header shortcut links remain separate from the disclosure toggle.
Optional plugin controls follow the built-in actions and wrap when necessary.
Saved desktop order and visibility override this default example.

UI-02: phone drawer from Home or task workbench, retained composition.

```text
Menu                       [x]   fixed
[Workspace v]                    fixed
------------------------------------
[ + New Task                     ]
Home
[ Quick Chat | Terminal           ]
Automations                       >
Canvases                          >
Integrations                      v
  GitHub
  Integration settings
TASKS [existing touch task rows]
Utilities: Stats, Settings, ...
[safe-area clearance]
```

The phone body has one scroller and labelled 44px touch targets.
The drawer and action dialogs retain their existing lifetime and focus behavior.

UI-03: expanded and empty integration states, desktop.

```text
Integrations [GH] [GL] v         Integrations >
  GitHub                          no provider shortcut icons
  GitLab                        Integrations v
  [other eligible providers]      Integration settings
  [eligible plugin links]
  Integration settings
```

Control order and independent activation are required.
ASCII glyphs, spacing, provider examples, and sample copy are illustrative.
All product labels use existing localization keys.
UI-01 maps to AC-UI-NAV-HIERARCHY-001.1 through 001.7.
UI-02 maps to 003.1, 003.2, 003.3, 003.5, and 003.6.
UI-03 maps to 001.2, 001.3, and 001.6.

## Tests

All paths in this table are relative to `apps/web/`.

| Test file and planned assertion | Criteria |
| --- | --- |
| `components/app-sidebar/app-sidebar-new-task-item.test.tsx`: icon-only quick actions follow New Task; each launcher runs once; activity names and workspace plugin context survive | 001.1, 001.4, 001.6 |
| `components/app-sidebar/app-sidebar-section.test.tsx`: header action lies between label and chevron; independent action does not toggle; labelled toggle retains disclosure state | 001.2, 001.7 |
| `components/app-sidebar/sections/canvases-section.test.tsx`: workspace settings shortcut remains available with closed and open disclosure | 001.7 |
| `components/app-sidebar/sections/integrations-section.test.tsx`: first-party shortcut eligibility/order/capacity, plugin exclusion, named children, and empty setup | 001.3, 001.6 |
| `components/app-sidebar/app-sidebar-footer.test.tsx`: direct Stats before theme, absent from menu, remaining utilities and settings/auth guards | 001.5, 001.6 |
| `components/navigation/app-nav-sheet.test.tsx`: phone retains labelled creation, quick actions, and utilities | 003.1, 003.2, 003.6 |

Numbers abbreviate `AC-UI-NAV-HIERARCHY-`.
Update assertions for the former utility bar and Stats menu instead of retaining conflicting expectations.
Keep existing creation-routing, plugin error-boundary, and settings-guard coverage.

## E2E tests

- Update `layout/navigation-hierarchy.spec.ts` in `chromium`.
  Prove same-row action geometry, independent launchers, header shortcut order and routing,
  direct Stats placement, menu exclusion, rail access, and 768px coarse-pointer containment.
  Map to 001.1 through 001.7.
- Update `layout/mobile-navigation-hierarchy.spec.ts` in `mobile-chrome`.
  Add a phone smoke scenario for New Task, both labelled quick actions, and direct Utilities Stats access.
  Retain existing provider Issues access from Home and a task workbench.
  Map to 003.1, 003.2, 003.3, 003.5, and 003.6.
- Retain `layout/mobile-unified-navigation.spec.ts` and `layout/mobile-menu-hierarchy.spec.ts`
  as regression evidence for focus, drawer order, and task navigation.

Desktop screenshots cover dark/light themes and Portuguese labels.
Check 360px, 393px, and 767px phone layouts, including a narrow fine-pointer window.
Check 768px coarse-pointer targets, many plugin registrations, and document overflow.
Use the managed runner for fresh production builds and isolated mock data.

## Work orders

- [x] [Task 01: Restore sidebar action placement](task-01-restore-sidebar-actions.md)

Execution is sequential in the primary conversation.
The work order owns exact validation commands and the documentation updates.

## Verification results

Design-package checks passed on 2026-10-05:

- `python3 scripts/list-docs.py validate`: 351 decisions and 1366 specifications validated.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: all specifications passed.
- The work order's Node `validateCoverage` preflight: `covered`, one work order, zero errors.
  This is a design preflight using the planned production entry point.
- `git diff --check`: passed.

Implementation and rendered verification passed on 2026-10-05:

- Focused component tests: 106 tests passed across six suites.
- Managed desktop E2E: 9 tests passed, including compact row actions, integration shortcut navigation, footer order, touch bounds, customization, and dark/light/Portuguese captures.
- Managed phone E2E: 19 tests passed, including phone creation, both quick actions, Utilities Stats, focus return, and containment at 360px, 393px, and 767px.
- `pnpm run typecheck`, `pnpm exec eslint components/app-sidebar --max-warnings 0`, and `pnpm run i18n:check` passed.
- `node --test scripts/validate-public-docs.test.mjs`: 62 tests passed; `node scripts/validate-public-docs.mjs` validated 47 published pages.
- `python3 scripts/list-docs.py validate` validated 351 decisions and 1366 specifications; `python3 scripts/lint-spec-files.test.py` passed 36 tests; `python3 scripts/lint-spec-files.py --all` passed.
- PR-documentation preflight returned `covered` with zero errors; `git diff --check` passed.

PR review follow-up on 2026-10-05:

- Integration shortcuts and Stats identify their current route with `aria-current` and the shared visible active treatment.
- Quick-action labels remain available on keyboard focus; the shortcut focus indicator and active-route cues have rendered E2E coverage.
- Focused component tests: 62 tests passed across three suites; managed Chromium E2E: 3 tests passed.

PR CI fixup on 2026-10-05:

- The first phone drawer geometry fix waited for finite animations before comparing parent and child bounds; a later CI run showed separate bounding-box reads could still straddle an animation frame.

PR CI fixup on 2026-10-06:

- Updated the plugin action UX E2E to expect the 24px native sidebar action and plugin controls inline when they fit, wrapping below when needed.
- The complete `plugin-action-ux.spec.ts` passed all three tests in the constrained CI image with retries disabled.
- The phone drawer geometry assertion now reads the menu and action bounds in one browser task; its target test passed four repeated runs and the full mobile navigation hierarchy passed all four tests with retries disabled.
- Scoped the disabled-integration navigation assertion to the integration section, excluding its separate always-visible header shortcut; the integration navigation spec passed with retries disabled.
- Updated the Quick Chat focus test for the New Task, Quick Terminal, Quick Chat tab order and asserted the terminal tooltip on keyboard focus; both previously failing tests passed with retries disabled.
- ESLint and Prettier passed for the four affected E2E files, and the production Vite build passed.

## Risks

- Many workspace plugin actions can squeeze a compact row. Wrapping must preserve the built-in controls.
- Large touch targets can crowd provider icons. Limit coarse-pointer header capacity and retain named children.
- Existing tests expect the removed utility bar and Stats menu. Update both desktop assertions and tablet assertions.
- The shared NewTaskButton serves phones. A desktop restoration must not shrink phone creation targets.
