---
id: "01-stop-bubble-menu-transaction-loop"
title: "Stop the BubbleMenu transaction loop"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-RESPONSIVE-PLAN-FORMATTING-001
acceptance_criteria:
  - AC-UI-RESPONSIVE-PLAN-FORMATTING-001.1
  - AC-UI-RESPONSIVE-PLAN-FORMATTING-001.2
system_design:
  - ../../specs/ui/system-design/responsive-plan-formatting.md
---

# Task 01: Stop the BubbleMenu Transaction Loop

## Summary

Make the desktop BubbleMenu inputs stable and deduplicate the reactive editor
snapshot. Add a focused regression that models Tiptap's option-update
transaction and proves that the component settles.

## In scope

- Add the failing transaction-loop regression before production changes.
- Keep the desktop BubbleMenu placement options referentially stable.
- Compare all `EditorSnapshot` fields before publishing state.
- Keep focus, selection, code-block, and active-mark changes reactive.
- Run the existing mobile formatting scenario after the focused checks pass.

## Out of scope

- Change rendered layout, action order, touch targets, or keyboard geometry.
- Change Plan content, comments, autosave, or revisions.
- Change Tiptap or ProseMirror package code.
- Add a new browser performance harness.

## Acceptance

- The regression fails before production changes because BubbleMenu option
  updates cause repeated editor transactions.
- The corrected component settles after one option update when the editor
  snapshot is unchanged.
- Real focus, selection, code-block, and active-mark changes still update the
  formatting controls.

## Verification

```bash
cd apps/web
pnpm exec vitest run components/editors/tiptap/plan-bubble-menu.test.tsx
pnpm run typecheck
pnpm exec eslint components/editors/tiptap/plan-bubble-menu.tsx components/editors/tiptap/plan-bubble-menu.test.tsx
pnpm e2e:run --project mobile-chrome tests/task/mobile-plan-formatting-toolbar.spec.ts
```

## Files likely touched

- `apps/web/components/editors/tiptap/plan-bubble-menu.tsx`
- `apps/web/components/editors/tiptap/plan-bubble-menu.test.tsx`
- `docs/plans/plan-bubble-menu-transaction-loop/plan.md`
- `docs/plans/plan-bubble-menu-transaction-loop/task-01-stop-bubble-menu-transaction-loop.md`

## Dependencies

None.

## Risks

- The test fake must model the Tiptap options effect without creating an
  unbounded test loop.
- The snapshot comparison must include every field in `EditorSnapshot`.

## Parallelism

`sequential`

## Inputs

- `REQ-UI-RESPONSIVE-PLAN-FORMATTING-001` acceptance criteria `.1` and `.2`.
- `docs/specs/ui/system-design/responsive-plan-formatting.md`.
- The performance trace attached to the investigation task.
- Tiptap BubbleMenu's option-update effect in `@tiptap/react`.
- Existing PlanBubbleMenu component tests and the mobile formatting E2E.

## Results

- Added a controlled BubbleMenu fake that reproduces repeated option-update
  transactions and fails before the fix.
- Stabilized the desktop BubbleMenu placement options at module scope.
- Deduplicated all `EditorSnapshot` fields before publishing React state.
- Extended the fake editor regression to verify active-mark button state and
  mobile and desktop code-block visibility changes after transactions.
- Focused Vitest, typecheck, changed-file ESLint, and the existing mobile Plan
  formatting E2E all pass.
- No mobile E2E change was needed because this internal normalization preserves
  the existing mobile presentation; the existing scenario covers it.
