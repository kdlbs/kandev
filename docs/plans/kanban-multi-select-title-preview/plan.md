---
created: 2026-09-24
status: implemented
requirements:
  - REQ-TASKS-RICH-TASK-TITLE-PREVIEWS-001
system_design:
  - ../../specs/tasks/system-design/rich-task-title-previews.md
legacy_specs: []
---

# Implementation plan: Suppress Kanban title previews during multi-select

## Overview

Kanban card titles currently keep an interactive preview while multi-select is active. The popover can cover adjacent cards and intercept selection clicks. One focused work order passes the existing selection state to the title-preview gates in both column cards and Pipeline rows, then proves the mode transition and selection path in component and browser tests.

## Scope

### In scope

- Close an open Kanban title preview when multi-select starts.
- Prevent pointer and keyboard title previews for selected and unselected cards during multi-select.
- Let title clicks select or deselect a card, and restore ordinary previews when selection mode ends.

### Out of scope

- GitHub PR or GitLab MR badge hovers, task preview panels, sidebar titles, and bulk action semantics.
- New backend or persisted selection state.

## Technical approach

`apps/web/components/kanban-card-content.tsx` already suppresses card actions when `isMultiSelectMode` is true. In `KanbanCardShell`, pass the same state to the `enableTitleHover` prop of `KanbanCardBody`. The Pipeline view has a separate `PipelineRow` → `RowInfoColumn` → `CardTitle` path, so pass its existing mode through that row and disable the same hover prop. Keep `CardTitle` and the shared `TaskTitleHoverCard` unchanged; their existing conditional render removes the trigger and closes an open popover. Retain the current click dispatch paths for title selection and existing coarse-pointer navigation.

## ASCII UI preview

`UI-01: Kanban card title`, desktop, after hovering a previewable title.

```text
Before, selection on:                 After, selection on:
[x] Long task title...               [x] Long task title...
    +---------------------------+         (no title popover)
    | Full title and subtasks   |     Click title -> toggle selection
    | covers the next card      |
    +---------------------------+
[ ] Next task                      [ ] Next task
```

Turning selection on closes an already open preview. Turning it off restores the ordinary desktop preview. The checkbox, title, and card remain in the current order; no new control or copy is added. This structure is required by `AC-TASKS-RICH-TASK-TITLE-PREVIEWS-001.2` through `.4`; ASCII spacing is illustrative.

`UI-02: Phone Kanban card`, coarse pointer, unchanged composition.

```text
[ ] Long task title...
    Tap title -> existing card action
    No hover preview
```

The existing phone Kanban card is the mobile exemplar. Its card content remains the touch navigation or selection target according to the active mode; there is no separate hover surface, extra scroll owner, or change to safe-area and touch-target geometry. The mobile regression check covers direct title navigation.

## Tests

| Criterion                                          | Evidence                                                                                                                                      |
| -------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------- |
| `AC-TASKS-RICH-TASK-TITLE-PREVIEWS-001.2` and `.4` | `apps/web/components/kanban-card-regression.test.tsx`: full card trigger absent in multi-select and present when mode ends.                   |
| `.2`                                               | `apps/web/components/kanban/pipeline-kanban-shared-source.test.tsx`: Pipeline title trigger is absent in multi-select and present outside it. |
| `AC-TASKS-RICH-TASK-TITLE-PREVIEWS-001.3`          | `apps/web/components/kanban-card-click.test.ts` already checks click dispatch; browser test below covers an actual title click.               |

## E2E tests

| Criterion     | Project and path                                                                             | Scenario                                                                                                                                                   |
| ------------- | -------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `.2` and `.3` | Chromium, `apps/web/e2e/tests/kanban/task-title-hover-subtasks.spec.ts`                      | Open a title preview, enable multi-select, confirm it closes and stays absent on hover, click a card title, and verify selection count without navigation. |
| `.4`          | Chromium, same spec                                                                          | End multi-select and verify the preview opens again.                                                                                                       |
| `.4`          | Mobile Chrome, existing `apps/web/e2e/tests/kanban/mobile-task-title-hover-subtasks.spec.ts` | Direct title tap navigates without a hover preview.                                                                                                        |

## Work orders

- [x] [Task 01: Guard Kanban title previews in selection mode](task-01-guard-title-preview.md)

## Verification results

- `pnpm exec vitest run components/kanban-card-regression.test.tsx components/kanban-card-click.test.ts` - passed (2 files, 18 tests).
- `pnpm run typecheck` - passed.
- `pnpm e2e:run --host --project chromium tests/kanban/task-title-hover-subtasks.spec.ts` - passed (4 tests; managed host build and production Vite bundle).
- `pnpm e2e:run --host --project mobile-chrome tests/kanban/mobile-task-title-hover-subtasks.spec.ts` - passed (1 test; managed host build and production Vite bundle).
- Pipeline review fix: `pnpm exec vitest run components/kanban-card-regression.test.tsx components/kanban-card-click.test.ts components/kanban/pipeline-kanban-shared-source.test.tsx` - passed (3 files, 22 tests).
- `pnpm run typecheck` after the Pipeline guard - passed.
- `python3 scripts/list-docs.py validate` and `python3 scripts/lint-spec-files.py --all` after the design update - passed.

## Risks

- Switching mode while the preview owns keyboard focus must close it without leaving an overlay that intercepts input.
- The guard must affect only Kanban title previews; badge hovers and task navigation follow their existing contracts.
- The migrated preview requirement describes subtask-only eligibility, while current card tests also cover descriptions. The new suppression criterion applies to every Kanban title preview; this fix does not change normal-mode eligibility.
