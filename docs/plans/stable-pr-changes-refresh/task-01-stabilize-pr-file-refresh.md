---
id: "01-stabilize-pr-file-refresh"
title: "Stabilize PR file refresh"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/tasks/requirements/multi-branch.md"
---

# Task 01: Stabilize PR File Refresh

## Acceptance

- Advancing `last_synced_at` for the same workspace/task/PR triggers one replacement request without making the previously resolved PR files disappear while it is pending.
- The current replacement response swaps the visible file set atomically, while workspace/task/PR switches and PR removal clear incompatible retained data.
- A failed background refresh retains the last resolved file set; a failed initial load remains empty.
- The Changes timeline needs no component or responsive-layout changes; its desktop and mobile PR Changes section remains mounted because `hasPRFiles` does not transiently fall to false.

## Verification

```bash
cd apps && pnpm install --frozen-lockfile && pnpm --filter @kandev/web test hooks/domains/github/use-pr-workspace-scope.test.ts && pnpm --filter @kandev/web lint && pnpm --filter @kandev/web typecheck
```

## Files likely touched

- `apps/web/hooks/domains/github/use-active-task-pr-files.ts`
- `apps/web/hooks/domains/github/use-pr-workspace-scope.test.ts`
- this task file and `plan.md`

## Dependencies

None.

## Parallelism

Sequential. Production behavior and regression coverage share the same hook contract.

## Inputs

- Multi-branch spec frontend behavior and background-refresh scenarios.
- Confirmed root cause in `plan.md`.
- Existing `fetchKey`, workspace response guard, in-flight/fetched request tracking, and `pruneByKeySet` behavior in `use-active-task-pr-files.ts`.
- Existing deferred workspace-response test in `use-pr-workspace-scope.test.ts`.

## TDD sequence

1. Add the deferred `last_synced_at` refresh regression and confirm it fails because `filesByPRKey` becomes empty before the second response resolves.
2. Add scope/removal assertions required by the selected stable identity and confirm existing stale-response behavior remains protected.
3. Implement the smallest cache-identity change, rerun the focused test, then run lint and typecheck.

## Output contract

Report RED/GREEN evidence, the stable display identity and timestamped request identity, scope-invalidation behavior, exact commands/results, files changed, residual risks, and synchronize this task plus `plan.md` in the same primary conversation.

## Results

- RED: the deferred timestamp-refresh regression failed with `expected [] to deeply equal [{ filename: "stable.ts" }]` before production changes.
- RED: the controlled background-refresh rejection failed with the same empty-list mismatch before failure retention was implemented.
- GREEN: `cd apps && pnpm --filter @kandev/web test hooks/domains/github/use-pr-workspace-scope.test.ts && pnpm --filter @kandev/web lint && pnpm --filter @kandev/web typecheck` passed with 5 tests, zero lint warnings, and clean TypeScript compilation.
- `cd apps && pnpm install --frozen-lockfile` completed successfully before testing and committing.
- The cache now scopes resolved files by workspace and task, retains them by logical PR identity during timestamped refreshes, atomically replaces them on success, and retains them on background failure. Removed PRs and scope changes still clear incompatible files.
- Review remediation adds current desired-key validation per logical PR. A controlled T3-before-T2 regression now proves a superseded response cannot overwrite newer files; the focused suite passes 6 tests after the fix.
- No Playwright test or screenshot was added because the change is hook-only state normalization with no markup, responsive composition, touch behavior, scrolling, or navigation changes. Existing mobile Changes coverage remains applicable.
- Temporary diagnostic edits: none remain. External side effects and security/trust boundary changes: none.
