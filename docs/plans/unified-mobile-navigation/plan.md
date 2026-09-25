---
created: 2026-09-15
status: implemented
requirements:
  - REQ-UI-MOBILE-MENU-001
  - REQ-UI-MOBILE-MENU-002
  - REQ-UI-MOBILE-MENU-003
  - REQ-UI-MOBILE-MENU-004
  - REQ-UI-MOBILE-MENU-005
  - REQ-UI-MOBILE-MENU-006
  - REQ-UI-MOBILE-MENU-007
system_design:
  - ../../specs/ui/system-design/unified-mobile-navigation.md
legacy_specs: []
---

# Unified mobile navigation implementation plan

## Current revision: quick actions and collapsible integrations

[Task 05](task-05-action-first-menu.md) is implemented and verified after Task 04. The user
accepted the proposed ordering and requested a collapsible Integrations section.
No material question remains. UI owns the shared presentation; domain owners
retain state and availability. Earlier task results remain historical evidence.

### ASCII UI preview

UI-05 phone, hamburger open in a regular workspace:

```text
[Workspace v]
[Home]
[Quick Chat] [Quick terminal]
----------------------------
Tasks v                    +
  Existing task sidebar
----------------------------
Automations >    [Open list]
----------------------------
Integrations >
----------------------------
Utilities
  Settings / Stats / Theme
  Support / conditional health
```

Before: quick actions follow Tasks and integration links are always expanded.
After: quick actions precede Tasks; Integrations starts collapsed. Expanded
Integrations reveals available providers and Integration settings; empty reveals
settings alone. Existing optional local/plugin/canvas/status content retains
its slots described in the system design. Fixed title and one content scroller
remain. Geometry is illustrative; order, grouping, 44px targets and availability
are required. UI-05D: desktop/tablet retain current composition.

Task 05 maps AC-UI-MOBILE-MENU-007.1 through .4 to focused unit and phone browser
checks, plus the existing compact-desktop regression. No new persistent setting,
backend change or delegation. Preserve the populated preview, refresh only its
frontend after validation, and reapply mock integration seed if it is restarted.

## Prior revision: menu hierarchy (2026-09-19)

[Task 04](task-04-menu-hierarchy.md) is implemented and verified. Tasks 01-03 remain completed;
their results below are historical. User feedback removes the redundant global
Tasks/Threads links, aligns the Tasks disclosure with Utilities, preserves its
plus action, and restores Automations and empty-state Integrations discovery.
The UI system owns this reusable presentation; domain state remains shared.
Implementation follows the reviewed Task 04 scope.

### ASCII UI preview

UI-04: Phone hamburger in a regular workspace.

```text
Workspace
Home
---------------------------
Tasks v                   +
  Saved views / task list
Quick Chat / Quick terminal
---------------------------
Automations >   [Open list]
---------------------------
Integrations
  Providers / settings
---------------------------
Utilities
```

Tasks collapsed retains its plus. Empty Automations offers setup; read failures
offer retry. Empty Integrations retains settings. Fixed menu title, one scroller,
44px touch controls. UI-04D: desktop/tablet sidebar composition stays as shipped.
The Tasks heading matches Utilities in weight, alignment, divider and spacing.
Task 04 contains acceptance mapping, exact checks, files, risks and implementation
results: 71 focused unit tests and 62 distinct browser cases passed.

## Current revision: user feedback on 2026-09-17

The completed first iteration below is historical context. Its pinned shortcuts
and separate View options button are superseded by the revised requirement/design
sections and [Task 03](task-03-embedded-tasks.md). Tasks 01/02 remain done; Task 03
is implemented. Existing test counts below apply only to the first iteration;
Task 03 records the revision validation.

Target: retain the unified hamburger, put the actual Tasks sidebar in its middle
as an initially expanded collapsible section, remove separate navigation pins and
Task views, and use Kanban/Threads/List title dropdowns to open view options.
No material product question blocks the revision. Collapse persistence is local
to the mounted host; existing sidebar pin behavior stays within the normal list.

```text
Phone: [Workspace / Kanban v]                     [Menu]
       Listing content

Menu: Workspace / global navigation
      Tasks v [+] -> saved view, filters, task tree
      Workspace actions / utilities

Kanban / Threads / List v -> view options and saved views
Desktop/tablet: existing presentation
```

Delivery: Task 03 extracts the shared sidebar body/controller, embeds it without
a nested scroll region, moves Threads saved-view controls into the title-opened
options surface, and migrates relevant tests. Its work order contains exact
validation commands, affected files, and handoff/rotation risks. The user
authorized implementation on 2026-09-18; the revision is now implemented.

## Overview

Deliver one phone hamburger meaning, with task switching on the task title and
listing controls beside the content. First migrate shared navigation and all
entry points together; then add bounded pinned-task shortcuts. These are
sequential work orders in the primary session.

The user accepted the shared-menu/title-picker direction. Three pinned tasks
and preservation of the existing Task views label are bounded design choices.
Both work orders are implemented in the primary session.

## Scope

### In scope

- Phone Home, Kanban, List, Threads, task workbench, and shared page shells.
- Shared app Drawer, title-triggered task picker, local View options.
- Existing workspace/Office/Settings gates and navigation/dialog behavior.
- Up to three eligible existing pins, translations, entry-point test migration,
  focused rendered checks, and public navigation instructions.

### Out of scope

- New bottom navigation, task mutation flows, recency storage, backend or SDK
  contracts, desktop/tablet composition changes, or automatic publication.

## Technical approach

Follow the [design](../../specs/ui/system-design/unified-mobile-navigation.md)
and [entry-point ADR](../../decisions/2026-09-15-phone-navigation-entry-points.md).
`AppNavSheet` owns the shared phone shell; `AppNavSections` and the navigation
manifest own destinations. `KanbanHeaderMobile` separates app navigation from
listing options. `SessionMobileTopBar` exposes the existing task-switcher
controller through its title. Hoisted dialog state must survive drawer closure
and orientation changes. Existing task selection and workspace read owners
retain authority.

Task 02 adds the small pin projection using `useSidebarTaskPrefs` and the
workspace-scoped sidebar read boundary. No full task picker is mounted just to
display shortcuts.

## ASCII UI preview

Spacing and sample names are illustrative. Control meaning, hierarchy, fixed
versus scrolling regions, and separate task/session pickers are structural.
All rendered labels are localized.

### UI-00: Current phone behavior (source inspection)

```text
Home: [Listing context] [hamburger] -> navigation + display options
Task: [Back] [Task title] [hamburger] -> task list
Other page hamburger -> left-side navigation sheet
```

### UI-01: Shared phone app navigation, open

Entry: any phone hamburger. Covers AC-UI-MOBILE-MENU-001.1 through .3 and .5.

```text
Underlying page dimmed
  +--------------------------------+
  | Menu                       [X] |  fixed heading
  |--------------------------------|
  | Workspace: Product           v |
  | Home                           |
  | Tasks                    <here>|
  | Threads                        |
  | Task views                     |
  | [Local navigation, if present] |
  | [Pinned tasks: up to 3]        |  Task 02
  | Workspace actions             |
  | Integrations / Plugins        |
  | Utilities / Settings          |
  +--------------------------------+
           bottom safe area
```

One internal scroller below the heading. Existing mode/availability rules select
the actual destinations; Office does not show Kanban Task views or pins.

### UI-02: Phone task workbench, closed and picker open

Covers AC-UI-MOBILE-MENU-002.1 through .3 and -001.6.

```text
[Back] [Fix checkout... v] [status] [Menu]
       repo / branch / diff summary
[Agent + current session v]             separate session picker
             Active panel
[Chat] [Plan] [Files] [Changes] [...]    existing bottom nav

Tap task title:
  +--------------------------------+
  | Tasks                    [New] |
  | Workspace v / Saved view v     |
  | Filters                       |
  | Fix checkout          [*] [...]|
  | Refine onboarding         [...]|
  +--------------------------------+
```

Task rows retain their visible actions. Title and Menu are separate 44px touch
targets. Existing task-list loading/error/empty states remain in the picker.

### UI-03: Phone listing options

Covers AC-UI-MOBILE-MENU-001.4 and .6.

```text
[Workspace / listing context]        [Menu]
[View options]  [existing page controls]
                Listing content

Tap View options:
  +--------------------------------+
  | View options               [X] |
  | Kanban / List / Threads        |
  | Search                        |
  | Sort / Filter / Group          |
  | Applicable display settings   |
  | Saved-view actions / recovery |
  +--------------------------------+
```

Only applicable controls render; Threads retains its saved-view picker. The
options body is the only vertical scroller. Hidden search clears the filter.
Pipeline stays unavailable on phone without changing the saved desktop choice.

### UI-04: Phone pins and unavailable data

Covers AC-UI-MOBILE-MENU-003.1 through .3.

```text
Loaded:                       Loading / failed read:
Pinned tasks                  [Loading tasks...]
  Fix checkout                or [Could not load tasks] [Retry]
  Update docs                 Global destinations remain usable
  Review onboarding
```

No eligible pins: omit the section. Access denied: no cached task names.
Long names truncate; there is no nested pin-list scroller. Task views remains
the full task-list path regardless of shortcut state.

### UI-05: Desktop/tablet compatibility

```text
Desktop:
[Existing sidebar] | [Existing task header / layout controls]
                   | [Dockview panels]

Tablet:
[Existing topbar and menu presentation]
[Existing tablet task composition]
```

No new global bottom navigation or touch sizing on desktop. At 768px and 820px,
retain wider presentation; a requested task-dialog draft survives rotation.

## Tests

The following suites cover the implemented entry points and recovery behavior.

| Acceptance | Unit/component evidence |
| --- | --- |
| -001.1-.3, .5-.6 | `components/navigation/app-nav-sheet.test.tsx`: shared drawer, route indication, mode gates, handoff and breakpoint controller lifetime |
| -001.4, .6 | `components/kanban/kanban-header-mobile.test.tsx`, `mobile-menu-sheet.test.tsx`: separate controls, preserved options and search focus |
| -002.1-.3 | `components/task/mobile/session-mobile-top-bar.test.tsx`: title opens picker, hamburger opens navigation, loading name and state |
| -003.1-.3 | `components/navigation/mobile-pinned-task-items.test.ts`: eligibility/order/deduplication/cap; `mobile-pinned-tasks.test.tsx`: read states and dispatch |

## E2E tests

- `tests/layout/mobile-unified-navigation.spec.ts` (mobile-chrome):
  `same menu from listings, task and page shells` (-001.1-.3),
  `separate listing controls preserve preferences` (-001.4, .6),
  `drawer geometry and focus survive breakpoint changes` (-001.5-.6),
  `title picker switches tasks without changing hamburger meaning` (-002.1-.3).
- `tests/layout/mobile-navigation-pins.spec.ts` (mobile-chrome):
  `pins are ordered and workspace isolated`, `shortcut history matches origin`,
  `read failure leaves navigation usable` (-003.1-.3).
- Migrate and run [affected mobile specs](affected-mobile-specs.txt), collected
  from current hamburger/menu selectors. Inspect helper/page-object usages too;
  preserve every task-action assertion and add any transitive affected specs
  to this manifest before the implementation check. This is a regression set,
  not a request for the full suite.
- Run `tests/layout/compact-desktop-responsive.spec.ts` (chromium) for wider
  chrome; the new phone suite checks computed presentation at 767/768/820px.

## Work orders

- [x] [Task 05: Prioritize quick actions and collapse Integrations](task-05-action-first-menu.md)

- [x] [Task 04: Simplify Home and complete menu sections](task-04-menu-hierarchy.md)

- [x] [Task 03: Embed Tasks and use listing-title options](task-03-embedded-tasks.md)

- [x] [Task 01: Unify navigation and preserve entry points](task-01-shared-navigation.md)
- [x] [Task 02: Add bounded pinned-task shortcuts](task-02-pinned-shortcuts.md)

## Compatibility documents

The completed [GitHub mobile parity package](../github-mobile-parity/plan.md)
and its Task 03 remain historical evidence. Its task-view access and rotation
tests are retained in Task 01. Existing mobile-task-chrome and listing contracts
remain authoritative except for the explicitly superseded hamburger entry.
Do not rewrite historical verification counts or mark old packages pending.

Public task-navigation instructions are updated in
`docs/public/sessions-and-review.md`. This is a how-to update.

## Verification results

### Unit and static checks

- One focused Vitest run: 13 files, 109 tests passed. Suites cover app navigation,
  destination rows, pin projection/read states, page shell, phone listing header,
  listing menu, shared selection context, task-switcher sheet/hooks, phone task
  title/repository metadata, and task-layout state preservation.
- Web typecheck and targeted ESLint passed. Final edited files are formatted.
- `pnpm run i18n:check` and `pnpm run i18n:ratchet` passed, including all five
  languages, generated pseudo locale, and new-file copy checks.
- Public documentation validation and its unit test passed.

Focused unit command (from `apps/web`):

```bash
pnpm exec vitest run components/navigation/app-nav-sheet.test.tsx components/navigation/destination-rows.test.tsx components/navigation/mobile-pinned-task-items.test.ts components/navigation/mobile-pinned-tasks.test.tsx components/page-shell.test.tsx components/kanban/kanban-header-mobile.test.tsx components/kanban/mobile-menu-sheet.test.tsx components/task/mobile/task-sheet-selection-context.test.tsx components/task/mobile/session-task-switcher-sheet.test.tsx components/task/mobile/session-task-switcher-sheet-hooks.test.ts components/task/mobile/session-mobile-top-bar.test.tsx components/task/mobile/session-mobile-top-bar-repository.test.tsx components/task/task-layout-repository.test.tsx
```

### Browser checks

The affected regression manifest contains 82 existing mobile specs. Together
with the two new navigation specs, the serial run exercised 242 scenarios:
237 passed initially; five failures were investigated. The final build fixes
workbench remounting during workspace metadata refreshes; the cached-chat
scroll-restoration case now passes. Four existing tests now address the correct
View options surface or title text node without weakening behavior assertions.

The focused rerun passed all nine new navigation scenarios plus cached-chat,
Quick Chat configuration, tablet transition, and long-title truncation. The workspace-switch test now deletes its temporary workspace and restores
the seed workspace preference; its four pin cases and the Home-preference
case pass together (5/5). Thus all 242 mobile scenarios have passing evidence
across the broad run and focused reruns. The two compact-desktop Chromium
scenarios also passed (2/2).

Phone app navigation, task-title chrome, and View options screenshots were
inspected at 393px and 767px. Pin, long-name truncation, error/denial and recovery
screenshots were inspected. Browser assertions cover 44px targets, viewport
containment, focus return, 768px/820px wider presentation, navigation history,
workspace isolation, and task-creation drafts surviving rotation.

Commands (from `apps/web`, after a fresh `pnpm run build:e2e`):

```bash
E2E_PORT_OFFSET=29 xargs -d '\n' pnpm e2e:run --host --no-build --project mobile-chrome tests/layout/mobile-unified-navigation.spec.ts tests/layout/mobile-navigation-pins.spec.ts < ../../docs/plans/unified-mobile-navigation/affected-mobile-specs.txt
E2E_PORT_OFFSET=29 pnpm e2e:run --host --no-build --project mobile-chrome tests/chat/mobile-auto-scroll-toggle.spec.ts:109 tests/chat/mobile-quick-chat-entry.spec.ts:90 tests/kanban/mobile-display-settings-groups.spec.ts:132 tests/settings/mobile-startup-page.spec.ts:26 tests/task/mobile-task-topbar-long-title.spec.ts tests/layout/mobile-unified-navigation.spec.ts tests/layout/mobile-navigation-pins.spec.ts
E2E_PORT_OFFSET=29 pnpm e2e:run --host --no-build --project mobile-chrome tests/layout/mobile-navigation-pins.spec.ts tests/settings/mobile-startup-page.spec.ts:26
E2E_PORT_OFFSET=29 pnpm e2e:run --host --no-build --project chromium --config e2e/navigation-check.playwright.config.ts tests/layout/compact-desktop-responsive.spec.ts
```

The desktop command used a temporary config importing `e2e/playwright.config.ts`
and replacing only Chromium's first `testIgnore` expression with
`/\/mobile-[^/]*\.spec\.ts$/`. The normal expression matched `mobile-` in this
worktree's parent directory and selected zero tests. All remaining project and
runner settings were retained; the temporary config was removed afterward.

Final documentation checks passed: catalog validation (273 decisions, 936 specs),
full specification lint, public-docs validation (46 pages) and its unit test,
and `git diff --check`.

Runs are serial, with one worker. An initial two-shard attempt was discarded
because a restarted worker collided with the other shard's backend port; its
fixture failures are not counted as product results. No freshness guard was
bypassed. No commit, push, PR, or deployment was performed.

## Risks

- Generic Task views push navigation must not replace in-workbench switching.
- Duplicate drawer controllers can lose focus or task-dialog drafts.
- Extracting listing options can lose search, saved-view recovery, or plugin
  actions; each existing action must retain a visible home.
- Old workspace caches must never leak task shortcuts after a switch or denial.
- Many existing tests encode the old hamburger meaning; update entry points
  and retain behavior assertions rather than aliasing the old meaning.

## Revision completion (2026-09-18)

Task 03 is done: 100 distinct focused unit tests and 66 distinct browser cases
have passing results. Typecheck, targeted ESLint, build, i18n validation/ratchet,
specification/public-doc checks, and diff checks passed. The existing isolated
Tailscale preview was refreshed and smoke-tested with its seed preserved.
See Task 03 for commands, logs, regression fixes, and shutdown instructions.

## Task 04 completion (2026-09-19)

Home is the only phone listing destination. The collapsible Tasks heading
matches Utilities and retains its independent plus. Automations and integration
settings remain discoverable, including empty workspaces. 71 focused unit tests,
60 mobile browser cases and two desktop cases passed, with startup/navigation
timeouts resolved by focused reruns. Typecheck, scoped lint, i18n, production
build and documentation checks passed. Task 04 records exact cases and logs.

The isolated preview at http://100.105.155.17:48439 contains the final build
and passed direct Tailscale smoke checks. Main :9998 was untouched. Stop only
this preview with `python3 /tmp/kandev-mobile-menu-test-hw9njt_n/stop.py`.

## Task 05 completion (2026-09-19)

Quick Chat and Quick terminal now form a labelled two-column row directly under
Home. Integrations starts collapsed, exposes providers/settings on expansion,
and follows Automations before Utilities. Settings precedes Stats on phones.
Plugin controls, metrics, workspace gates and wider composition retain their
existing behavior. 46 focused units and 29 distinct browser cases passed, along
with typecheck, scoped lint, i18n, build and docs gates. Task 05 records the
animation-aware test correction and exact verification results. The populated
preview remains at http://100.105.155.17:48439; main :9998 was not changed.
