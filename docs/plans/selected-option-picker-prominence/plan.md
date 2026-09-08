---
spec: docs/specs/ui/requirements/selected-option-picker-prominence.md
created: 2026-08-22
status: implemented
---

# Implementation Plan: Selected option prominence in single-choice pickers

## Overview

Add one pure ordering helper and apply it at the existing single-choice picker
boundaries. The model/config picker, shared combobox, compact task-create pill,
mode menu, and dedicated branch list will keep their current data and selection
handlers while showing the current value first and using the repository's
neutral selected-surface semantics. Focused component tests cover stable
ordering and selected styling; desktop and mobile Playwright checks cover the
rendered user flow.

## Frontend

### Shared selected-option ordering

- `apps/web/lib/utils/selector-options.ts`
- `apps/web/lib/utils/selector-options.test.ts`

Add a generic helper that returns a selected-first copy without mutating the
source collection, preserves the relative order of every other entry, and
leaves collections with no selected value unchanged.

### Shared combobox

- `apps/web/components/combobox.tsx`
- `apps/web/components/combobox.test.tsx`

Apply selected-first ordering to the unfiltered option list used by agent,
branch, repository, executor, and profile consumers. Keep unavailable options
non-selectable, ensure a selected unavailable option is still first, and give
the selected option a reserved neutral/card surface with a primary boundary or
inset ring. Keep keyboard highlighting and existing enabled/disabled grouping
behavior readable.

### Compact task-create pills

- `apps/web/components/task-create-dialog-pill.tsx`
- `apps/web/components/task-create-dialog-pill.test.tsx`

Apply the same selected-first order and persistent selected row treatment to
the compact repository and branch chip pickers. Preserve their custom filter
scoring, refresh/action controls, tooltip behavior, and selection handlers.

### Model and dependent configuration picker

- `apps/web/components/model-config-selector.tsx`
- `apps/web/components/model-config-selector-content.tsx`
- `apps/web/components/model-config-selector.test.tsx`

Prioritize the current model in the top-level model list and the current value
in each nested configuration list. Add persistent selected-row styling that is
separate from cmdk's transient highlighted-row styling. Preserve model loading,
gone-model, provider-description, and keep-open behavior.

### Mode and dedicated branch pickers

- `apps/web/components/task/mode-selector.tsx`
- `apps/web/components/task/branch-picker-list.tsx`
- `apps/web/components/task/branch-picker-list.test.tsx`

Use the shared ordering helper for the current mode and current branch when no
filter is active. Apply the same selected-surface semantics without changing
branch filtering/ranking, branch saving, mode persistence, or the existing
`MobilePickerSheet` composition used by task-launch recovery.

## Tests

- **What:** the ordering helper moves an existing selected value first without
  mutating input, preserves non-selected order, and leaves no-selection lists
  unchanged.
  **File:** `apps/web/lib/utils/selector-options.test.ts`.
  **How:** focused Vitest unit cases.
- **What:** shared combobox consumers render the current option first and keep
  the selected row visibly distinct, including an unavailable current value.
  **File:** `apps/web/components/combobox.test.tsx`.
  **How:** render the real combobox, open it, and assert option order,
  selected styling, keyboard highlight transitions, and disabled behavior.
- **What:** model and nested configuration lists show their current values
  first while preserving selection callbacks and loading/description behavior.
  **File:** `apps/web/components/model-config-selector.test.tsx`.
  **How:** focused component tests using the existing test fixtures.
- **What:** the dedicated branch list puts the current branch first, preserves
  the source order of other branches, and keeps `aria-selected` and the
  selected surface correct.
  **File:** `apps/web/components/task/branch-picker-list.test.tsx`.
  **How:** render tests with local and remote branch fixtures, including a
  filtered list.

## E2E Tests

- **Scenario:** the task chat model selector opens with its current model first,
  keeps the selected surface visible, and still allows a different model and
  dependent option to be selected.
  **File:** `apps/web/e2e/tests/chat/mobile-model-selector.spec.ts` and the
  existing desktop model-selector coverage in
  `apps/web/e2e/tests/chat/model-selector-error.spec.ts`.
  **What to verify:** touch and mouse opening, first-option identity, selected
  row styling, dependent-option interaction, and viewport containment.
- **Scenario:** task creation opens the agent, repository, and branch selectors
  with their current values first.
  **File:** `apps/web/e2e/tests/task/create-task.spec.ts` and
  `apps/web/e2e/tests/task/create-task-branch-selector.spec.ts`.
  **What to verify:** selected agent/branch order, current selected styling,
  and unchanged branch filter ranking.
- **Scenario:** the phone recovery branch picker keeps the first available
  replacement branch visible in its existing bottom sheet.
  **File:** `apps/web/e2e/tests/task/mobile-launch-failure-recovery.spec.ts`.
  **What to verify:** the first replacement row is visible before scrolling,
  touch-sized, viewport-contained, and free of document horizontal overflow.

## Mobile design contract

- **Desktop outcome and mobile entry:** users confirm or change the current
  model, config value, agent, or branch. Desktop opens the existing popover or
  menu; phone opens the existing compact toolbar picker or branch
  `MobilePickerSheet` through the current visible trigger.
- **Nearest shipped exemplars:** `apps/web/e2e/tests/chat/mobile-model-selector.spec.ts`
  for the task toolbar picker and
  `apps/web/components/task/mobile/mobile-picker-sheet.tsx` plus
  `apps/web/components/task/task-launch-branch-picker.tsx` for the phone branch
  surface. Reuse their trigger, focus, containment, and scroll behavior.
- **Mobile hierarchy and primary action:** the picker title/current context is
  already supplied by the existing trigger or sheet header; the first list row
  is the current choice, followed by alternatives. Tapping a row remains the
  primary action.
- **Presentation and rationale:** keep the current popover/command surface for
  short searchable choices and the existing inset bottom sheet for branch
  recovery. This is a short selection task, so a new route or full-height
  surface would add unnecessary navigation.
- **Scroll owner and geometry:** preserve the existing command list or
  `BranchPickerList` scroll owner, internal overflow behavior, dynamic viewport
  sizing, safe-area padding, and document horizontal-overflow guarantees.
- **Shared logic:** ordering, selected-value matching, filtering, and handlers
  are shared across viewports. Only the existing presentation wrapper differs.
- **Mobile proof:** the mobile model selector test asserts the current row is
  first and visibly selected before any scroll; the branch-sheet path asserts
  containment where that path is available.

## Verification Results

Completed on 2026-08-22.

- `cd apps && rtk pnpm --filter @kandev/web test -- --run lib/utils/selector-options.test.ts components/combobox.test.tsx components/model-config-selector.test.tsx components/task-create-dialog-pill.test.tsx components/task/branch-picker-list.test.tsx components/task/selector-trigger-class.test.tsx` - 6 files, 55 tests passed.
- `cd apps/web && rtk pnpm run typecheck` - passed.
- `cd apps/web && rtk pnpm e2e:run --project chromium tests/chat/model-selector-error.spec.ts tests/task/create-task.spec.ts tests/task/create-task-branch-selector.spec.ts` - fresh backend/frontend build, 35 tests passed.
- `cd apps/web && rtk pnpm e2e:run --project mobile-chrome tests/chat/mobile-model-selector.spec.ts tests/settings/mobile-no-silent-model-fallback.spec.ts tests/task/mobile-launch-failure-recovery.spec.ts` - fresh backend/frontend build, 3 tests passed.
- Targeted ESLint with `--max-warnings 0` and Prettier checks for all changed frontend and E2E files - passed.
- `rtk git diff --check` - passed.
- Fixup review findings addressed: active listbox scoping, distinct keyboard
  highlight styling, focused module extraction for size limits, and direct
  helper-class coverage.

## Implementation Waves And Parallel Candidates

Wave 1:

- [x] [task-01-selected-option-picker-ui](task-01-selected-option-picker-ui.md)

Wave 2:

- [x] [task-02-selected-option-picker-e2e](task-02-selected-option-picker-e2e.md)

Tasks are sequential because the E2E selectors and assertions depend on the
final selected-row markup.
