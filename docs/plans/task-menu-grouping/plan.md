---
created: 2026-09-10
status: complete
requirements:
  - REQ-TASKS-MENU-GROUPING-001
system_design:
  - ../../specs/tasks/system-design/task-menu-grouping.md
legacy_specs: []
---

# Implementation Plan: Task menu grouping

## Overview

Group existing task actions by purpose across task rows, cards, preview, and desktop detail menus.
Implement task-row composition first, then the shared card builder and its consumers.
Each work order includes focused unit and browser evidence.

## Scope

### In scope

- The agreed action order, thin dividers, conditional groups, and plugin placement.
- Existing single-task, bulk, archived, and unresolved menu variants.
- Desktop context and dropdown menus, plus existing phone touch menus.
- Documentation that describes the previous plugin or task menu position.

### Out of scope

- New actions, card Pin/Color parity, working Duplicate, backend changes, or new plugin groups.
- New mobile shells, group headings, or localization keys.
- Commits, PR creation, or implementation during this design turn.

## Technical approach

Task rows use `SingleSelectionMenuItems` and `BulkSelectionMenuItems`.
The implementation makes action eligibility explicit enough to omit empty groups.
Leaf-owned separators move to the group composition without changing other callers.
Existing selection callbacks, confirmations, and event guards remain intact.

Cards use `buildKanbanCardMenuEntries` for both renderers.
Compose nonempty entry arrays and insert one separator between them.
`buildTaskActionsMenuEntries` inherits normal ordering and groups its reduced variants consistently.
Preserve Edit plugin nesting on cards and flat Edit on preview/detail surfaces.

The task system owns the requirement because the contract concerns task actions across surfaces.
The existing preview/detail pair links to the new grouping pair for ordering.
No linked companion plan exists for that pair in this checkout.
Existing priority plans remain historical records of their action behavior, which this change preserves.

## ASCII UI preview

UI-01: Task-row context menu or visible ellipsis, normal task.
Source baseline: Pin, Edit, Priority, Rename, Create subtask, Duplicate, Archive,
Color, Nest under, plugins, Link, movement, Detach, Delete. Availability varies.

Proposed groups:

```text
+---------------------------+
| Pin                       |
| Color                   > |
| Priority                > |
+---------------------------+
| Edit                      |
| Rename                    |
| Duplicate (disabled)      |
+---------------------------+
| Create subtask            |
| Nest under              > |
| Link                    > |
| Detach from parent*       |
+---------------------------+
| Move to                 > |
| Send to workflow        > |
+---------------------------+
| Primary plugin actions*   |
+---------------------------+
| Archive                   |
| Delete                    |
+---------------------------+
```

UI-02: Card right-click or three-dot menu, normal task.
Preview/detail use the same order, with flat Edit and their existing eligibility.
Source baseline: Edit, Priority, Move to, Send to workflow, plugins, Link,
Archive, Detach, separator, Delete.

```text
+---------------------------+
| Priority                > |
+---------------------------+
| Edit                    * |
+---------------------------+
| Link                    > |
| Detach from parent*       |
+---------------------------+
| Move to                 > |
| Send to workflow        > |
+---------------------------+
| Primary plugin actions*   |
+---------------------------+
| Archive                   |
| Delete                    |
+---------------------------+
```

An asterisk marks conditional content, not literal copy.
Card Edit becomes a submenu only when admitted Edit plugin actions exist.
All existing eligibility rules apply. Empty groups and adjacent dividers disappear.
Archive is neutral. Delete is red. Group order is required, while spacing is illustrative.

UI-03: Phone, visible row/card ellipsis, below 640px.
The existing inset menu uses the same applicable rows as UI-01 or UI-02.

```text
+-----------------------------+
| Task list or task drawer    |
| Task title             ... |
|                             |
|   dimmed underlying surface  |
| +-------------------------+ |
| | Pin / Color / Priority  | |
| |------------------------| | |
| | Edit / Rename / ...    | | | <- internal scroll
| |------------------------| | |
| | Relationships         | | |
| | Movement / plugins    v | |
| |-------------------------| |
| | Archive                 | |
| | Delete                  | |
| +-------------------------+ |
|       safe-area spacing     |
+-----------------------------+
```

Slash-separated lines abbreviate separate action rows, not combined controls.
The entire menu body scrolls, including Archive/Delete. There is no fixed action footer.
Touch rows retain at least 44px targets. Submenus retain the existing touch presentation.
At 640px and above, existing anchored placement remains in force.

UI-04: Reduced states.

```text
Bulk row             Archived detail      Unresolved detail
Pin N tasks          [plugin actions]     [plugin actions]
----------------     ----------------     ----------------
Move to >            Delete               Archive
Send to workflow >                        Delete
----------------
Archive N tasks
Delete N tasks
```

Plugin rows and their divider disappear when no primary action is admitted.
Archived row menus preserve their own existing eligibility rather than adopting the detail entry set.
Disabled and pending actions retain existing styling and cannot invoke a second operation.

UI-01/02 cover criteria .1-.5 and .8. UI-03 covers .7-.8. UI-04 covers .3-.6.

## Tests

| Evidence | Criteria |
| --- | --- |
| `task-switcher-context-menu.test.tsx`: grouped normal/subtask order, hidden groups, disabled Duplicate | .1, .3, .4 |
| Same suite: bulk and archived membership, callbacks and event guards | .4, .6, .8 |
| `kanban-card-menu-items.test.tsx`: exact group boundaries and both renderer parity | .2-.4 |
| Same suite: primary registration order and visibility changes, Edit/Link placement | .5 |
| `task-actions-menu-entries.test.ts`: normal, archived, unresolved, no-plugin variants | .2, .3, .6 |

Paths above are under `apps/web/components/task`, `apps/web/components`, and `apps/web/lib/kanban` respectively.
Use behavioral assertions for visible order and separators, with callback assertions for moved actions.

## E2E tests

- New `tests/task/task-menu-grouping.spec.ts`, chromium: row grouping, Color before Priority, keyboard order, and one persisted priority update.
- Existing `tests/task/mobile-sidebar-task-actions.spec.ts`, mobile-chrome: tap opener, grouping, nested choices, internal scrolling, and archive confirmation cancellation.
- Existing `tests/kanban/card-menu-delete-archive.spec.ts`, chromium: both card openers, grouping, and existing removal outcomes.
- Existing `tests/kanban/task-actions-menu-preview.spec.ts` and `task-actions-menu-detail.spec.ts`, chromium: inherited grouping and existing focus/confirmation outcomes.
- Existing `tests/kanban/mobile-task-priority.spec.ts`, mobile-chrome: card touch menu grouping and priority persistence.

Phone checks include long-menu containment, 44px targets, last-action reachability,
and no document horizontal overflow. Capture desktop and phone screenshots for preview comparison.
Keep all existing mutation scenarios. Add ordering checks alongside them.

## Work orders

- [x] [Task 01: Group task-row actions](task-01-task-row-groups.md)
- [x] [Task 02: Group card actions](task-02-card-groups.md)

Execute sequentially. Work orders do not authorize delegation.
Exact verification commands are in each work order.

## Verification results

Implementation checks (2026-09-10):

- Focused Vitest: 3 files passed, 50 tests passed.
- ESLint on changed source, test, and E2E files: passed with 0 errors; the repository's existing file/function-size and complexity rules reported warnings only.
- `pnpm run typecheck`: passed.
- `pnpm run i18n:check`: passed, including 3018 guarded files.
- Desktop row E2E: 1 passed after the managed runner build; the corrected fixture uses a root task because sidebar Pin is not offered for subtasks.
- Mobile sidebar E2E: 17 passed, including grouping, internal scroll, touch-target, and removal reachability checks.
- Desktop card/preview/detail E2E: 9 passed.
- Mobile card priority E2E: 1 passed.
- Fresh desktop and phone menu screenshots were captured, inspected, PNG-compressed, and validated against the two-entry PR asset manifest.

Document checks (2026-09-10):

- `python3 scripts/lint-spec-files.test.py`: passed, 36 tests.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- `git diff --check`: passed.

## Risks

- Children that return null can leave dividers around empty groups.
- Leaf-owned dividers can duplicate the new group boundaries.
- Plugin placement descriptions currently name the old location.
- Added dividers increase phone menu height and require real scrolling evidence.
- Existing tests can encode the previous order. Update order assertions without removing behavior coverage.
