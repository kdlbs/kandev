---
created: 2026-10-08
status: done
requirements:
  - REQ-UI-SIDEBAR-CUSTOMIZATION-001
  - REQ-UI-SIDEBAR-CUSTOMIZATION-004
  - REQ-UI-SIDEBAR-CUSTOMIZATION-005
  - REQ-UI-SIDEBAR-CUSTOMIZATION-006
  - REQ-COORDINATOR-NEEDS-YOU-006
system_design:
  - ../../specs/ui/system-design/sidebar-customization.md
  - ../../specs/coordinator/system-design/needs-you.md
legacy_specs: []
---

# Update flagged sidebar entries

## Overview

Give Coordinators the current navigation style and an independent layout entry.
Use shared eligibility for the sidebar, its customization menus, and Settings.
Deliver this small vertical change in one sequential work order.

UI owns reusable personal composition. Coordinator keeps its resource and badge contracts.
This package extends existing artifacts rather than creating another sidebar specification.

## Scope

### In scope

- Coordinator navigation presentation, fast actions, saved visibility, and order.
- Configuration choices for eligible flagged entries, including Canvases and Needs-you Inbox.
- Disabled-node retention, existing-layout compatibility, reset, and workspace isolation.
- Desktop right-click, Settings draft editing, and phone customization.
- Targeted tests and the public customization guide after implementation.

### Out of scope

- Flag defaults, rollout, registry changes, and new features.
- Coordinator behavior, routes, proposal counts, resource sorting, or permissions.
- Fixed Office sections, Tasks composition, plugin SDK extensions, or command palette changes.
- New storage, schema versions, and unrelated sidebar refactors.

## Technical approach

The current `CoordinatorsSection` uses the uppercase section presentation.
The saved renderer attaches it to Automations, then provides a separate fallback.
The phone renderer repeats that placement. Neither configuration path owns a coordinator node.

Extend `models.DefaultSidebarLayout` and its projection with `coordinators` before `automations`.
Materialize missing nodes without a write, revision increment, or changes to existing node order.
Mirror compatibility in `lib/sidebar/layout-types.ts` for old client payloads.
Keep the version-1 scoped CAS patch and unsupported-version recovery.

Introduce the small builtin presentation table described in the paired UI design.
Adapt current labels, icons, and eligibility to it; do not replace the destination manifest.
Navigation and direct customization receive eligible nodes, including hidden ones.
Settings retains its full draft and marks unavailable nodes without enabling their destinations.
Preview and picker use the same eligibility. Full saves retain disabled nodes and geometry.
Registered plugin entries continue through their existing catalog and materialization.

Render Coordinators only through its independent saved node on desktop and phone.
Remove both Automations coupling and fallback insertion.
Use `presentation="navigation"`, the Coordinator icon, and the existing collapse state.
Keep initial expansion, readiness, setup link, active rows, and badge summary.
Respect `sidebarFastActionsEnabled`; retain a labelled body destination when the icon is hidden.
Ordinary icon controls use 28px on desktop and at least 44px on coarse pointers.

Nearest phone exemplars: `AppNavSheet`, `MobileCoordinatorsSection`, and `MobileSidebarCustomization`.
The navigation drawer keeps primary actions, saved tools, then required task navigation.
The temporary customization drawer shares choices and explicit moves.
Settings remains a full page because layout editing can involve several steps.
Retain one drawer scroller, viewport bounds, safe-area clearance, and 44px targets.
Do not write phone ordering over saved desktop ordering.

No material intent question remains. Enabled features participate in customization.
Disabled features cannot activate, and returning features recover saved visibility and position.
An empty coordinator list remains configurable.

## ASCII UI preview

### UI-01: Desktop navigation and right-click menu

Entry: regular workspace, Coordinator flag enabled, empty coordinator list.
The screenshots and current source establish the uppercase header as the previous behavior.

```text
Before                    After
COORDINATORS [list] >      [coord] Coordinators       >
  Set up a coordinator      Open coordinators
                            Set up a coordinator
[bolt] Automations >      [bolt] Automations          >

Right-click Sidebar settings
+--------------------------------+
| Sidebar settings               |
| [x] New Task                   |
| [x] Home                       |
| [x] Coordinators               |
| [x] Automations                |
| [x] Canvases       (if enabled) |
| [x] Integrations               |
| ...existing choices...         |
| Show fast action icons         |
| ...existing style/move actions..|
| Sidebar layout settings        |
+--------------------------------+
```

Fast actions are off in this example. When enabled, the list icon sits beside the disclosure.
The name and chevron toggle only expansion. The list destination remains a separate action.
Hidden Coordinators remains an unchecked configuration choice.
Map to AC-UI-SIDEBAR-CUSTOMIZATION-001.6/.8, 006.1/.3, and AC-COORDINATOR-NEEDS-YOU-006.1/.3.

### UI-02: Layout settings, both pointer modes

Entry: Settings > Layout > Sidebar, active workspace, Coordinator flag enabled.

```text
+--------------------------------------------------+
| Layout / Sidebar                Workspace: Demo  |
| ...existing presentation preferences...          |
| New Task         [on]  [up] [down]               |
| Home             [on]  [up] [down]               |
| Coordinators     [on]  [up] [down]               |
| Automations      [on]  [up] [down]               |
| Canvases         [on]  [up] [down]               |
| ...existing nodes and shortcut groups...         |
| ...existing draft preview...                     |
|                         [shared Save changes]    |
+--------------------------------------------------+

Saved feature disabled:
| Coordinators (unavailable)  [saved choice]        |
```

The phone list uses touch controls and the existing page scroll owner.
Saved unavailable nodes retain position and visibility. Their destinations cannot activate.
Shared save/discard and existing conflict feedback retain ownership.
Map to AC-UI-SIDEBAR-CUSTOMIZATION-001.7/.9, 004.2/.3/.4, and 005.2/.4.

### UI-03: Phone navigation and customization

Entry: visible app-menu trigger, Coordinator flag enabled.

```text
+--------------------------------+
| Navigation               Close | fixed header
| [New Task]                     |
| Home / quick actions           | one scroll body
| [coord] Coordinators         v |
|   Open coordinators            |
|   Planner                  [2] |
| Automations                  > |
| ...saved tools...              |
| [Customize sidebar]            |
| Tasks / local navigation       |
+--------------------------------+

Customize sidebar (temporary inset drawer)
+--------------------------------+
| Customize sidebar         Done | fixed header
| [x] Coordinators               | one scroll body
|     [Move up] [Move down]      |
| [x] Automations                |
| ...eligible existing choices...|
| Sidebar layout settings        |
+--------------------------------+
```

Saved tools retain relative order; primary controls retain the existing phone composition.
Safe-area clearance and at least 44px hit targets are required.
No action requires long press. Unchecking Coordinators removes it without a fallback.
Map to AC-UI-SIDEBAR-CUSTOMIZATION-001.5/.6/.7, 005.2/.3/.5, and 006.7.

Icon text, sample data, and ASCII spacing are illustrative.
Grouping, independent controls, saved state, and eligibility are required.
All labels use shipped localization keys or complete locale additions.

## Tests

| Evidence | Contract |
| --- | --- |
| Backend model/service tests: defaults, old projection, reset, round trip | 001.9; existing revision and workspace rules |
| Layout projection and new builtin table tests: flags, workspace mode, hidden nodes | 001.6/.7/.9 |
| Editor and customization tests: eligible labels, unavailable retention, clean and dirty drafts | 001.6/.7; 004.2/.3/.4; 006.6 |
| Desktop/mobile coordinator component tests: navigation header, fast actions, empty/list/folded/rail | 001.8; Coordinator 006.1/.2/.3 |

Use TDD for changed logic. Assert observable behavior instead of copying descriptor constants.
Include enable/disable/enable around a saved hidden and reordered node.
Audit current feature gates: Coordinator, Canvases, Needs-you Inbox, Office Inbox, and gated plugin destinations.
Office's fixed sections remain excluded, as the existing customization contract requires.

## E2E tests

Add `settings/sidebar-flagged-entries.spec.ts` for Chromium.
Use the existing coordinator feature fixture and API setup.
Prove context-menu discovery, hide/restore, independent Automations visibility, reload, Settings save, and explicit reorder.
Also prove empty-list configuration and Coordinator fast-action placement.

Add `settings/mobile-sidebar-flagged-entries.spec.ts` for Mobile Chrome.
Prove drawer discovery, hide/restore, explicit move, Settings save, and coordinator destination navigation.
Measure touch targets, viewport containment, and horizontal overflow.
Exercise a narrow fine-pointer viewport for the same phone control contract.

Feature-off and returning-feature cases cover 001.7; the remaining flows cover the preview mappings.
Retain existing direct-customization and coordinator component suites as regression evidence.
Use causal waits and isolated runtime overrides with cleanup.
Build through the managed runner and execute desktop and mobile commands sequentially.

## Work orders

- [x] [Task 01: Unify flagged sidebar entries](task-01-unify-flagged-entries.md) (done)

## Verification results

Design checks passed on 2026-10-08:

- `python3 scripts/list-docs.py validate`: 364 decisions and 1455 specifications.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `.github/scripts/pr-docs.cjs` `validateCoverage`: covered, with no reference errors.
  The local preflight supplied this package and its four paired specification files.
  It included the planned coordinator source path to test the implementation coverage gate.
- `git diff --check -- docs/specs docs/plans/flagged-sidebar-entries`: passed.

Implementation checks passed on 2026-10-08:

- Backend sidebar model and service tests passed.
- The focused frontend suite passed: 10 files and 80 tests. Typecheck and changed-file ESLint passed.
- `make -C apps/backend build` and `pnpm --filter @kandev/web build:vite` passed.
- `pnpm run i18n:check` passed with 9,489 referenced keys across seven complete locales.
- Managed Chromium E2E passed 7/7 tests; managed Mobile Chrome E2E passed 2/2 tests. Both runs built the Go backend and Vite E2E assets.
- Public-doc validation passed: 62 tests and 47 published pages.
- Specification validation and lint passed. Actual-path PR documentation coverage reported covered with no reference errors. `git diff --check` passed.
- UI-01: desktop Coordinator renders at its saved position, remains independently configurable, and keeps the labelled list action when fast actions are off. Empty-list setup and fast-action placement passed browser coverage.
- UI-02: Settings retains feature-disabled nodes as unavailable and preserves hidden/reordered choices when the feature returns.
- UI-03: phone navigation follows saved tool order; customization, direct navigation, 44px targets, viewport containment, and narrow fine-pointer behavior passed.

The managed and production Vite builds emitted existing chunk-size and dynamic-import warnings; all builds and browser suites completed successfully.

## Risks

- A missing-node repair must preserve hidden choices and unrelated ordering.
- Filtering a complete Settings draft can remove disabled entries from a save.
- A fallback can resurrect hidden Coordinators or render duplicates.
- Feature changes must not discard an unsaved Settings draft.
- Header action changes must preserve the list destination and disclosure focus behavior.

## Related delivery packages

Existing sidebar-customization, sidebar-presentation-preferences, mobile-saved-navigation,
workspace-coordinator, and workspace-coordinator-p2 packages record previous deliveries.
This package owns the incremental change and its final evidence.
Do not reopen completed tasks or replace their historical results.
