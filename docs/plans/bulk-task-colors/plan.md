---
created: 2026-09-19
status: implemented
requirements:
  - REQ-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-006
system_design:
  - ../../specs/ui/system-design/bulk-task-colors.md
legacy_specs: []
---

# Bulk task colors

## Outcome and scope

Adds personal Color actions to the sidebar bulk menu and board selection toolbar.
Reuses the existing palette, persistence, and automatic-rule precedence. Phone users
perform the same operation from board selection. The implementation reuses the existing personal settings patch contract and preserves automatic-rule precedence.

## Sources

- [Requirements](../../specs/ui/requirements/bulk-task-colors.md): REQ-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-006, active extension.
- [System design](../../specs/ui/system-design/bulk-task-colors.md#bulk-manual-color-editing): current bulk design extending the current personal-color contract.
- [ADR 0041](../../decisions/0041-backend-owned-portable-user-settings.md): reuse portable personal settings ownership; no new ADR needed.

## Work package

| Order | Work order                                                   | Status | Dependencies |
| ----- | ------------------------------------------------------------ | ------ | ------------ |
| 1     | [Task 01: Bulk color editing](task-01-bulk-color-editing.md) | done   | None         |

One vertical slice, sequential execution. No subagents are authorized.
Estimated implementation and targeted checks: 3-5 hours, subject to E2E setup.

## ASCII UI preview

UI-01: Desktop sidebar selected-row menu and board toolbar, idle state.

```text
Sidebar: 3 selected        Board: 3 selected
+--------------------+     [Move to] [Color v] [Archive] [Delete] [Clear]
| Pin 3 tasks        |                 +----------------------+
| Color            > |                 | Color for 3 tasks    |
+--------------------+                 | Red                  |
                                      | Orange               |
                                      | Yellow               |
                                      | Green                |
                                      | Blue                 |
                                      | Purple               |
                                      | Pink                 |
                                      | None                 |
                                      +----------------------+
```

UI-02: Phone board selection and expanded color picker.

```text
Before selection, above the board:
[Select tasks]
In selection mode: [Cancel selection]

Fixed selection bar, above safe area:
[3 selected] [Color] [Actions] [Clear]

Inset picker:
+--------------------------------+
| Color for 3 tasks       [Close] | <- fixed title
| Automatic rules may override   |
| the visible color.             |
|--------------------------------|
| Red                            | <- one scrolling list
| Orange                         |
| Yellow                         |
| Green                          |
| Blue                           |
| Purple                         |
| Pink                           |
| None                           |
+--------------------------------+
```

UI-03: Save states (same targeting across viewports).

```text
Saving: [Color: Saving...] (disabled)
Failure: Could not save colors. Try again.
Partial: Colors saved for 500 of 501 tasks. Try again for the rest.
```

Required structure: a visible color entry, named options, mixed-state accuracy,
selection retained after dismissal/save, and a contained touch surface. A common
value has a check; mixed values have no checked color. Phone Actions contains
existing move/archive/delete actions. Empty selection hides the toolbar.
Spacing and English labels are illustrative; implementation uses shared tokens
and localized copy. Map UI-01/02 to AC-006.1 through .4 and .6; UI-03 to .5/.7,
where AC-006 abbreviates AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-006.

## Verification strategy

Task 01 defines exact unit, component, desktop/mobile E2E, localization, and
specification commands. Existing backend multi-key patch behavior is reused.
Test actual persisted colors and unaffected tasks, not just menu visibility.
Validate phone geometry at 393px and toolbar composition on either side of
768px. Capture a phone screenshot during the focused E2E run.

## Risks and exclusions

- Concurrent optimistic writes must preserve newer and unrelated changes.
- Large selections require sequential bounded patches and explicit partial failure.
- Board colors reuse personal sidebar markers; no new board card styling.
- No new mobile task-switcher multi-selection, automatic-rule changes, custom
  palette, task API, database migration, or live cross-browser guarantee.

## Documentation

Public docs updated: `docs/public/tasks-and-workflows.md` now explains desktop and phone bulk color selection, clearing, automatic-rule precedence, and retry behavior.

## Results

Implemented the shared mutation coordinator, sidebar and board Color controls, and the phone-native selection and picker flow. Focused unit/component tests passed (50), desktop Playwright passed (24), and mobile Playwright passed (2). Typecheck, focused ESLint, i18n validation, public-doc validation (62 tests and 47 pages), document catalog validation, all 36 specification-linter tests, full specification lint, and diff whitespace validation passed. PR remediation added regressions for in-flight optimistic colors, superseded save counts, focus restoration, the persistent live region, and cleared mobile action state. The 393 px phone render matched the planned inset picker with labeled touch rows, safe containment, and no horizontal overflow.
