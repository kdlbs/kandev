---
id: "01-bulk-color-editing"
title: "Apply personal colors to multi-selected tasks"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-006
acceptance_criteria:
  - AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-006.1
  - AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-006.2
  - AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-006.3
  - AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-006.4
  - AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-006.5
  - AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-006.6
  - AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-006.7
system_design:
  - ../../specs/ui/system-design/bulk-task-colors.md
---

# Task 01: Bulk color editing

## Summary

Deliver bulk manual-color editing end to end using the current personal settings
patch API. Extend the existing single-task color path and both selection surfaces.

## In scope

- Batch mutation, optimistic reconciliation, bounded chunks, pending/error state.
- Shared palette for sidebar bulk menu and board toolbar, with mobile picker
  and an explicit phone Select tasks / Cancel selection entry point.
- Localization, focused regression tests, and public task-guide instructions.

## Out of scope

New shared task fields, automatic-color priority changes, new board markers,
mobile sidebar selection mode, or unrelated selection refactoring.

## Acceptance

1. All seven linked criteria pass on the intended desktop/phone surfaces, with
   existing single-task and bulk destructive actions preserved.
2. Bulk requests use one patch per 500 IDs or fewer; selected IDs are captured,
   unselected settings survive, and stale/failed responses preserve newer edits.
3. Focused tests prove persistence, clearing, mixed state, automatic precedence,
   retry, and touch geometry; the public how-to explains the entry points.

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

Full combined preview: [plan](plan.md#ascii-ui-preview).

## Implementation sequence and tests

Use /tdd: first reproduce absent bulk Color behavior and write failing tests.
Extend use-task-color tests for one/many IDs, duplicates, empty input, null,
500/501 IDs, partial failure, unrelated settings, separate hook instances,
overlapping single and bulk edits, newer settings events, and stale responses.
Component tests cover selected versus unselected context targets, mixed values,
None enablement, preserving selection, pending state, and palette labels.

Add task/bulk-task-colors.spec.ts for sidebar and board entry points, persistence
across reload, clearing, unselected controls, and automatic-rule precedence.
Add kanban/mobile-bulk-task-colors.spec.ts starting with zero selection. Tap the
new Select tasks control, select cards without modifiers, cancel and re-enter,
then choose and clear a color, verify stored values and sidebar markers on reload,
44px targets, focus return, contained drawer, and no horizontal overflow.
Exercise phone Actions to preserve existing bulk actions; verify just below and
above 768px without rewriting saved desktop preferences. Use causal waits.

## Verification

From repo root; install dependencies first if this checkout lacks apps/node_modules.
The E2E runner builds artifacts by default; do not pass --no-build.
New test paths below are outputs of this work order.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run hooks/use-task-color.test.tsx hooks/use-task-color-migration.test.tsx components/task/task-switcher-context-menu.test.tsx components/task/task-switcher-color-menu.test.tsx components/kanban/task-multi-select-toolbar.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint hooks/use-task-color.ts components/task/task-switcher-color-menu.tsx components/task/task-switcher-context-menu-items.tsx components/kanban/task-multi-select-toolbar.tsx components/kanban-board.tsx)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --project chromium -- tests/task/bulk-task-colors.spec.ts tests/task/sidebar-multi-select.spec.ts tests/task/sidebar-task-color-sync.spec.ts tests/kanban/task-multi-select.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome -- tests/kanban/mobile-bulk-task-colors.spec.ts tests/task/mobile-sidebar-task-color-sync.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

Use the existing i18n:zh-hant generator for the Traditional Chinese pair when
new locale keys are required. Lint every additional implementation file introduced
by extraction. Run E2E projects sequentially; do not override worker budgets.

## Files likely touched

- apps/web/hooks/use-task-color.ts and use-task-color.test.tsx
- apps/web/components/task/task-switcher-color-menu.tsx and new companion test
- apps/web/components/task/task-switcher-context-menu-items.tsx and existing task-switcher-context-menu.test.tsx
- apps/web/components/kanban-board.tsx (phone selection entry point)
- apps/web/components/kanban/task-multi-select-toolbar.tsx and new companion test
- apps/web/components/task/mobile/mobile-picker-sheet.tsx (reuse; modify only if required)
- apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw}/ task and kanban catalogs as needed
- apps/web/e2e/tests/task/bulk-task-colors.spec.ts (new)
- apps/web/e2e/tests/kanban/mobile-bulk-task-colors.spec.ts (new)
- docs/public/ task guide selected during implementation

## Dependencies

None. Reuse existing personal settings API and automatic-color requirements.

## Risks

Cross-instance optimistic mutation races, settings capacity rejection, portaled
menu focus, and mobile toolbar overflow. Do not solve concurrency by overwriting
the whole settings map or changing automatic-color precedence.

## Parallelism

sequential

## Inputs

Owning requirement/design links in frontmatter, AGENTS.md and apps/web/AGENTS.md,
existing use-task-color tests, sidebar-multi-select E2E, task-multi-select E2E,
and mobile-sidebar-task-color-sync E2E.

## Results

Implemented bulk manual colors across sidebar and board selection. A shared per-store mutation coordinator deduplicates IDs, sends sequential 500-ID patches, preserves overlapping newer edits, retains confirmed chunks after partial failure, and restores only unsaved owned entries. The desktop and phone pickers preserve selection, expose pending and retry feedback, and keep automatic-rule precedence.

Verification passed: 50 focused Vitest tests; TypeScript typecheck; focused ESLint; complete i18n validation; 24 desktop Chromium E2E tests; 2 mobile Chrome E2E tests; 62 public-doc validator tests plus all 47 published pages; document catalog validation; 36 specification-linter tests; full specification lint; and `git diff --check`. PR remediation added regressions for in-flight optimistic colors, superseded save counts, focus restoration, the persistent live region, and cleared mobile action state. The inspected 393 px phone screenshot matched the planned inset drawer, labeled 44 px rows, single scrolling surface, focus-return behavior, and viewport containment.
