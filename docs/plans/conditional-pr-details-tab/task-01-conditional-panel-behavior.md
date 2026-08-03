---
id: "01-conditional-panel-behavior"
title: "Conditional panel behavior"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/task-layout-profiles.md"
---

# Task 01: Conditional panel behavior

Restore conditional desktop review-tab behavior while preserving explicit layout ownership.

## Acceptance

- Code-defined Default and compact layouts omit `pr-detail`, but reusable-layout validation and the editor still accept it.
- A linked GitHub PR or GitLab MR adds one inactive canonical panel with `autoAddedForReview: true` beside Agent when no canonical panel exists; repeated syncs update identity without duplication or focus theft.
- Review loss removes only a marked conditional panel. Explicit layout panels remain, and closing a conditional panel suppresses re-creation for that session without blocking explicit open/add actions.

## TDD sequence

1. Change focused preset and review-sync unit expectations and run them to capture failures against the current built-in/parameter-only behavior.
2. Add pure conditional-panel decision/application coverage, including task-switch, restoration, maximize, provider precedence, and dismissal cases.
3. Implement the smallest preset, synchronization, and session-storage changes.
4. Re-run the focused suite, typecheck, lint, and `git diff --check`.

## Files likely touched

- `apps/web/lib/state/layout-manager/presets.ts`
- `apps/web/lib/state/layout-manager/presets.test.ts`
- `apps/web/lib/layout/layout-profiles.ts`
- `apps/web/lib/layout/layout-profiles.test.ts`
- `apps/web/lib/state/layout-manager/merger.test.ts`
- `apps/web/components/task/dockview-review-panel-sync.ts`
- `apps/web/components/task/dockview-review-panel-sync.test.ts`
- `apps/web/lib/local-storage.ts`
- `apps/web/lib/local-storage.test.ts`

## Verification

- `cd apps && pnpm install --frozen-lockfile`
- `cd apps && pnpm --filter @kandev/web test lib/state/layout-manager/presets.test.ts lib/layout/layout-profiles.test.ts lib/state/layout-manager/merger.test.ts components/task/dockview-review-panel-sync.test.ts lib/local-storage.test.ts`
- `cd apps/web && pnpm run typecheck`
- `cd apps && pnpm --filter @kandev/web lint`
- `git diff --check`

## Dependencies

None.

## Parallelism

Sequential. The preset and synchronization changes share behavior and tests.

## Inputs

- `docs/specs/ui/task-layout-profiles.md` What, Persistence guarantees, and Scenarios
- `docs/plans/conditional-pr-details-tab/plan.md` Frontend and Tests
- Pre-regression conditional behavior in parent of commit `f8c363f72`, adapted so explicit layout panels are preserved

## Output contract

Report the red/green evidence, changed files, exact command results, blockers, residual risks, and task/plan status updates in the primary conversation.

## Results

Completed.

- Red: preset/profile expectations, conditional removal, insertion, and storage tests failed against the embedded PR default and parameter-only sync.
- Green: focused Vitest suite passed (`86` tests across presets, profiles, merger, review sync, and local storage).
- `cd apps/web && pnpm run typecheck` passed.
- `cd apps && pnpm --filter @kandev/web lint` passed with zero warnings.
- `git diff --check` passed.
