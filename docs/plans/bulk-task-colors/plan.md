---
status: draft
---

# Bulk task colors

## Outcome and scope

Add personal Color actions to the sidebar bulk menu and board selection toolbar.
Reuse existing palette, persistence, and automatic-rule precedence. Phone users
perform the same operation from board selection. This package stops at design;
implementation requires a later explicit request. No production code changed.

## Sources

- [Requirements](../../specs/ui/requirements/bulk-task-colors.md): REQ-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-006, draft extension.
- [System design](../../specs/ui/system-design/bulk-task-colors.md#bulk-manual-color-editing): draft bulk design extending the current personal-color contract.
- [ADR 0041](../../decisions/0041-backend-owned-portable-user-settings.md): reuse portable personal settings ownership; no new ADR needed.

## Work package

| Order | Work order | Status | Dependencies |
| --- | --- | --- | --- |
| 1 | [Task 01: Bulk color editing](task-01-bulk-color-editing.md) | pending | None |

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

/docs-maintainer assessment: this turn changes design only, so public docs stay
unchanged. Implementation adds a short how-to section to the existing task guide
chosen from docs/public after searching current task navigation documentation.

## Results

Review remediation: add an explicit phone selection trigger and move the new
contract into draft artifacts, preserving the shipped personal-color specs.
Document catalog validation, all 36 specification-linter tests, full specification
lint, and diff whitespace validation passed after these corrections.
Implementation and rendered verification remain pending.
