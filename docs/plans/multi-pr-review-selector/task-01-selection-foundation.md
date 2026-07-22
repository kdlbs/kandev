---
id: multi-pr-review-01
title: Add task-scoped Review PR selection
status: done
wave: 1
depends_on: []
plan: ./plan.md
---

## Acceptance

- Resolver defaults to primary PR, honors a stable override, isolates tasks, and falls back when selected PR disappears.
- Zustand UI state stores only task-keyed in-session overrides.
- PR key utility lives outside React panel code.

## Files

- `apps/web/components/github/pr-utils.ts`
- Existing `prTaskKey` importers
- `apps/web/hooks/domains/github/use-review-pr-selection.ts`
- `apps/web/hooks/domains/github/use-review-pr-selection.test.ts`
- `apps/web/lib/state/slices/ui/types.ts`
- `apps/web/lib/state/slices/ui/ui-slice.ts`
- `apps/web/lib/state/default-state.ts`
- `apps/web/lib/state/store.ts`
- `apps/web/lib/state/store-overrides.ts`

## Inputs

- Spec Frontend section
- `useTaskPR()` / `getPrimaryTaskPR()` in `use-task-pr.ts`
- Derived fallback pattern in `multi-pr-ci-popover.tsx`

## Method

Use strict RED → GREEN → REFACTOR. First run must fail on behavior, not missing import.

## Verification

`cd apps && pnpm --filter @kandev/web test -- --run hooks/domains/github/use-review-pr-selection.test.ts lib/state/store.test.ts`

## Output contract

Report summary, files, RED and GREEN results, blockers, and remaining risks; mark status done only after focused verification passes.
