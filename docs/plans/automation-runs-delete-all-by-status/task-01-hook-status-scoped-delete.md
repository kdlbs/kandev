---
id: "01-hook-status-scoped-delete"
title: "Hook status-scoped delete-all"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/office/requirements/automation-runs-delete-all-by-status.md"
---

# Task 01: Hook status-scoped delete-all

Extend `useAutomationRuns.deleteAllRuns` so a filtered view can delete exactly
the runs it shows, while the unfiltered call keeps the single bulk-delete
behavior.

## Acceptance

- `deleteAllRuns(runIds)` removes exactly those run ids from the store
  optimistically and calls `deleteAutomationRun(id, workspaceId)` once per id.
- On success it re-applies the removal (guarding against an in-flight refresh
  resurrecting rows), mirroring the existing `deleteRun` guard.
- When any delete in the batch fails: one `toast.error` and one recovery
  `listAutomationRuns` refresh; if the recovery refresh also fails, the
  pre-delete snapshot is restored with a `couldNotRefreshRuns` toast. One
  toast and one refresh per failed batch, not per id.
- `deleteAllRuns()` (no argument) keeps calling `deleteAllAutomationRuns(
  automationId, workspaceId)` exactly once with the existing clear/revert
  semantics — existing hook tests pass unchanged.
- `deleteAllRuns([])` is a no-op: no store mutation, no API call.

## Verification

From `apps/` (fresh worktrees must first run `pnpm install --frozen-lockfile`
from `apps/` when `apps/node_modules/` is absent):

```bash
pnpm --filter @kandev/web test -- --run hooks/domains/settings/use-automation-runs.test.ts hooks/domains/settings/use-automation-runs-delete.test.ts
```

Also run `git diff --check` and Prettier on the changed files before commit.

## Files likely touched

- `apps/web/hooks/domains/settings/use-automation-runs.ts`
- `apps/web/hooks/domains/settings/use-automation-runs.test.ts`

## Dependencies

None.

## Parallelism

Sequential. The production callback and its unit tests share one behavior and
belong in one TDD cycle.

## Inputs

- Spec scenarios: filtered delete-all scope, failure revert contract.
- Existing patterns in `use-automation-runs.ts`: `deleteRun`'s optimistic
  remove + success re-remove + failure refresh + snapshot restore;
  `deleteAllRuns`' aggregated toast/revert shape. Reuse the store actions
  already exposed (`removeRun`, `setRuns`) — do not add a store slice action.
- The `DeleteAllButton` gating in `runs-section.tsx` is out of scope here
  (task 02); the component may still call `deleteAllRuns()` today.

## Output contract

Report the RED assertion, files changed, the exact focused command and
result, any blockers or risks, and synchronized task/plan status in the same
conversation. No backend, store-slice, or API-layer changes.

## Results

- RED: new race-condition tests failed before the guards existed (stale
  refresh resurrecting deleted rows; early recovery on first rejection).
- GREEN: `pnpm --filter @kandev/web test -- --run
  hooks/domains/settings/use-automation-runs.test.ts
  hooks/domains/settings/use-automation-runs-delete.test.ts
  components/automations/runs-section.test.tsx lib/state/store.test.ts` —
  48 tests passed (final state; the suite was split across the two hook
  test files as it grew).
- Hardening added during review rounds: `fetchRuns`/`revertAfterFailedDelete`
  are epoch-guarded; `executeDeleteAll` uses `Promise.allSettled` and
  recovers only after every delete settles; destructive mutations are
  serialized via generation-tagged store state
  (`beginAutomationRunDelete`/`endAutomationRunDelete`,
  `automationRuns.mutationEpoch`/`deleting`) so overlapping or cross-instance
  deletes cannot race; `executeDeleteRun` joins the same mechanism; success
  paths reconcile with an authoritative post-delete refresh.
- `git diff --check` and Prettier — passed. No backend or API-layer changes.
