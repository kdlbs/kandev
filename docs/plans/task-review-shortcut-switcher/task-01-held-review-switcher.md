---
id: "01-held-review-switcher"
title: "Implement held review shortcut switcher"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/task-review-shortcut.md"
---

# Task 01: Implement held review shortcut switcher

## Acceptance

- Multi-review shortcut presses open on row 1, advance deliberately with wrap,
  and open the selected PR or MR only when the configured primary hold modifier
  is released.
- Escape, blur, visibility loss, dismissal, OS key repeat, Shift-only release,
  custom modified bindings, and modifierless bindings follow the spec.
- Picker keeps ArrowUp/Down, Enter, click, mixed-provider order, focus, and
  accessible selected-state behavior without changing touch layout.

## Verification

- `cd apps && pnpm --filter @kandev/web test -- --run components/task/task-pr-open.test.ts components/task/use-task-review-shortcut.test.ts components/task/task-pr-picker-dialog.test.tsx components/task/recent-task-switcher-keys.test.ts`
- `cd apps/web && pnpm run typecheck`

## Files likely touched

- `apps/web/components/task/task-pr-open.ts`
- `apps/web/components/task/task-pr-open.test.ts`
- `apps/web/components/task/use-task-review-shortcut.ts`
- `apps/web/components/task/use-task-review-shortcut.test.ts`
- `apps/web/components/task/task-pr-shortcut.tsx`
- `apps/web/components/task/task-pr-picker-dialog.tsx`
- `apps/web/components/task/task-pr-picker-dialog.test.tsx`
- `apps/web/components/task/recent-task-switcher-keys.ts` (reuse only unless a
  narrowly required helper correction is proven by its test)

## Dependencies

None.

## Parallelism

Sequential. Task 02 requires this behavior and its stable selected-state
selector.

## Inputs

- Spec: `What` and all keyboard scenarios.
- Plan: `Provider-neutral review targets`, `Held shortcut controller`,
  `Controlled picker selection`, and `Mobile design contract`.
- Pattern: `apps/web/components/task/recent-task-switcher-hooks.ts` and
  `apps/web/components/task/recent-task-switcher-keys.ts`.

## Risks

- Stale React closures during rapid modifier release.
- Accidental activation after Escape or focus loss.
- Mixed PR/MR indices drifting from rendered order.

## Output contract

Report RED and GREEN results, changed files, responsive impact, blockers, and
remaining risks. Mark this task `done` and its plan checkbox complete only after
both verification commands pass.
