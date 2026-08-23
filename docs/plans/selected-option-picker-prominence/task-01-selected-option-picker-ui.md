---
id: "01-selected-option-picker-ui"
title: "Prioritize selected picker options"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/selected-option-picker-prominence.md"
---

# Task 01: Prioritize selected picker options

## Acceptance

- The current value is first in every unfiltered model/config, shared
  combobox, compact task-create pill, mode, and dedicated branch option list;
  all other values preserve source order, and no-current-value lists remain
  unchanged.
- Every current row has a persistent neutral/card selected surface with a
  primary boundary or inset ring and its check indicator, while hover and
  keyboard highlight remain distinct and unavailable current values stay
  disabled.
- Existing filtering, loading, dependent-option, mobile-sheet, and selection
  behavior remains intact.

## Verification

```bash
cd apps && rtk pnpm install --frozen-lockfile
cd apps && rtk pnpm --filter @kandev/web test -- --run lib/utils/selector-options.test.ts components/combobox.test.tsx components/model-config-selector.test.tsx components/task-create-dialog-pill.test.tsx components/task/branch-picker-list.test.tsx components/task/selector-trigger-class.test.tsx
cd apps/web && rtk pnpm run typecheck
```

## Files likely touched

- `apps/web/lib/utils/selector-options.ts`
- `apps/web/lib/utils/selector-options.test.ts`
- `apps/web/components/combobox.tsx`
- `apps/web/components/combobox.test.tsx`
- `apps/web/components/task-create-dialog-pill.tsx`
- `apps/web/components/task-create-dialog-pill.test.tsx`
- `apps/web/components/task-create-dialog-branch-options.tsx`
- `apps/web/hooks/use-pill-tooltip-suppression.ts`
- `apps/web/components/model-config-selector.tsx`
- `apps/web/components/model-config-selector.test.tsx`
- `apps/web/components/task/mode-selector.tsx`
- `apps/web/components/task/branch-picker-list.tsx`
- `apps/web/components/task/branch-picker-list.test.tsx`

## Dependencies

None.

## Parallelism

Sequential. These components share the selected-option contract and styling.

## Inputs

- All `What` and `Scenarios` sections of the spec.
- `docs/decisions/2026-07-29-interactive-accent-surface-semantics.md`.
- Existing `CommandItem`, `Combobox`, `ModelConfigSelector`,
  `BranchPickerList`, `ModeSelector`, and mobile picker patterns.
- The mobile design contract in `plan.md`.

## Output contract

Report the helper and component files changed, the exact selected-first and
selected-surface tests run, mobile implications, blockers, risks, and the
synchronized task/plan status.

## Results

Focused implementation tests pass: 6 files, 55 tests. TypeScript typecheck
passes. The shared combobox, compact task-create pills, model/config picker,
mode menu, and dedicated branch picker now share selected-first ordering and
persistent selected-row styling. Keyboard navigation coverage confirms cmdk's
transient highlight remains distinct from the persistent current-row surface.
The model content, branch option utilities, and pill tooltip suppression are
split into focused modules so the changed files remain within repository size
limits. Mobile behavior keeps the existing command and branch-sheet
compositions, with 44px touch rows and no new overflow. The changed frontend
files pass targeted ESLint with zero warnings and Prettier.
