---
created: 2026-09-13
status: implemented
requirements:
  - REQ-UI-TASK-LISTING-DISPLAY-PREFERENCES-004
system_design:
  - ../../specs/ui/system-design/task-listing-display-preferences.md
legacy_specs: []
---

# Implementation Plan: Grouped Homepage View Settings

## Overview

Implement the accepted expandable settings layout as one end-to-end work order.
UI owns this independent presentation contract; task filtering, ordering, and
portable settings remain with their existing owners. No material questions
remain from the accepted ASCII preview. The implementation is complete.

## Scope

Group existing Home display controls, derive current-value summaries, preserve
page visibility and plugin callbacks, localize copy, and cover desktop and phone.
Exclude new filters, saved views, preference migrations, column-menu redesign,
and sidebar behavior changes. Do not add Group by or Automatic colors to Home.

## Technical approach

Follow the [requirement](../../specs/ui/requirements/task-listing-display-preferences.md)
and its [design](../../specs/ui/system-design/task-listing-display-preferences.md#grouped-display-settings).
Extract shared disclosure presentation only as needed; preserve the sidebar
wrapper. Compose Home groups around existing fields/actions and mobile flags.
Reset only expansion on close. Keep the phone drawer scroll/safe-area contract.

The existing `threads-home-default` and `task-listing-display-preferences` plan
packages are completed historical scope. This package adds requirement 004;
it does not reopen their startup/navigation work or change their results.
ADR 0041 remains authoritative for preference ownership. Task-system priority
requirements remain authoritative for empty selections and unranked tasks.

## ASCII UI preview

### UI-01: Desktop display menu

Entry: Home display-settings button. Before: flat Workflow, Repository, Board
sort, Priority filter, and Preview panel sections, as shown by current source.
After, with all groups collapsed:

```text
+------------------------------------------+
| FILTERS                                > |
| All workflows, All repositories          |
| All priorities                           |
+------------------------------------------+
| SORT                                   > |
| Newest first                             |
+------------------------------------------+
| PREVIEW PANEL                          > |
| Off                                      |
+------------------------------------------+
```

UI-01 expanded Filters (Sort/Preview can also remain open):

```text
+------------------------------------------+
| FILTERS                                v |
| Workflow                                 |
| [All Workflows                         v]|
| Repository                               |
| [All repositories                      v]|
| Priority                                 |
| [ ] Critical       [ ] High              |
| [ ] Medium         [ ] Low               |
| [Plugin filter controls, when present]   |
+------------------------------------------+
| SORT                                   > |
| Newest first                             |
+------------------------------------------+
| PREVIEW PANEL                          > |
| Off                                      |
+------------------------------------------+
```

Expanded Sort contains the existing Board sort select. Expanded Preview panel
contains Open preview on click and its existing explanation. List adds a List
rows group with Show task details and its explanation. Threads omits inapplicable
groups. Empty/loading repositories keep the disabled selector and current
placeholder. Long summaries wrap; options scroll within existing Select menus.

### UI-02: Phone Home menu drawer

Entry: existing Home menu button, Board active; Filters expanded.

```text
+--------------------------------+
| Menu                           | fixed
+--------------------------------+
| [Existing workspace/navigation]|
| [Existing view/search controls]|
| Display options                |
| FILTERS                      v |
| Repository                     |
| [All repositories            v]|
| Priority                       |
| [ ] Critical    [ ] High       |
| [ ] Medium      [ ] Low        |
| SORT                         > |
| Newest first                   |
| PREVIEW PANEL                > |
| Off                            |
| [Existing Columns controls]    |
| [Other existing menu content]  |
+--------------------------------+
| Safe-area clearance            |
+--------------------------------+
```

The body is one scroll region. Workflow stays in existing phone Board navigation.
Phone List/Threads follow existing visibility flags. Headers and changed touch
controls have >=44px hit areas. Tablet keeps its existing Sheet composition.
Group order, summary hierarchy, independent expansion, and containment are
structural requirements (AC 004.1-004.6); spacing, capitalization, and the
illustrative two-column priority grid use existing tokens and available width.

## Tests

Extend `components/kanban-display-dropdown.test.tsx` with tests named
"groups start collapsed and expand independently", "summaries follow current
settings", "group toggles never write preferences", and "page and plugin
controls retain their scope" (004.1-004.4). Include reopening, all four priority
values versus none, missing/loading repository names, plugin collisions, and
long values. Extend `components/kanban/mobile-menu-sheet.test.tsx` for mobile
visibility and summary parity (004.2-004.4/004.6). Keep the sidebar disclosure
controlled/uncontrolled tests passing if its presentation is extracted.

## E2E tests

- `tests/kanban/display-settings-groups.spec.ts`, chromium: collapsed summaries,
  multiple expanded groups, nested selection, persisted setting after reopen,
  keyboard activation, focus return and short-viewport scrolling (004.1-004.5).
- `tests/kanban/mobile-display-settings-groups.spec.ts`, mobile-chrome: real taps,
  change priority/sort and observe the board, reopen without value loss, check
  >=44px targets, final control reachability, internal scroll, long text,
  no horizontal overflow, and 767/768px responsive geometry (004.1-004.6).
- Adapt existing filter/sort/preview/List tests to open the applicable group.
  Audit direct locators and shared page helpers with `rg`; run every changed
  suite in its owning project. The work-order command block lists the core
  regression set; record additional discovered suites with their exact commands.

## Work orders

- [x] [Task 01: Group display settings](task-01-group-display-settings.md)

## Verification results

Design checks passed before implementation. Implementation added the shared
disclosure and summary helpers, grouped desktop and phone display controls,
localized copy, focused component tests, and desktop/mobile E2E coverage.
The managed desktop and phone group suites passed. The complete command results
are recorded in Task 01.

## Risks

- Existing E2E helpers assume controls are immediately visible; hidden fields
  require explicit expansion across all affected tests.
- Nested Select portals can affect dismissal and focus inside the dropdown.
- Shared sidebar extraction must preserve its callers, IDs and controlled state.
- Phone Board hides the workflow selector intentionally; preserve that boundary.

## Documentation and delivery

The display-menu paragraph in `docs/public/tasks-and-workflows.md` now describes
the grouped controls and phone drawer. The work order records implementation and
verification results, including the desktop Popover keyboard model. The paired
design is current and requirements retain active status.
