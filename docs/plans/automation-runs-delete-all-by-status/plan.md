---
spec: docs/specs/office/requirements/automation-runs-delete-all-by-status.md
created: 2026-08-12
status: completed
---

# Implementation Plan: Status-scoped delete-all for automation runs

## Overview

Scope the Recent Runs delete-all to the active status view and move the
control into the table header's rightmost column. The frontend hook gains an
optional run-id list on `deleteAllRuns`; a filtered view deletes exactly the
visible runs through the existing per-run `automation.run.delete` path, while
the "All" view keeps the single `automation.runs.delete_all` call. No backend
changes: `Archived`/`Cancelled` are read-time-derived statuses that only exist
in the loaded payload, so the visible set is the only exact scope.

Adversarial review (rounds 1-3) hardened the delete flow: list fetches are
epoch-guarded against stale responses, delete mutations are serialized per
automation in shared store state (generation-tagged, so an unmounted editor
instance cannot clobber a remounted one), partial batch failures recover only
after every delete settles, and success paths reconcile with an authoritative
post-delete refresh instead of a blanket clear.

## Backend

None. The per-run delete path already deletes each run's associated task, and
per-run deletes are id-targeted so the orphaned-task race guarded by
`DeleteAllRuns`' per-automation run lock (a broad DELETE catching a
concurrently-created run) cannot occur.

## Frontend

### Hook — `apps/web/hooks/domains/settings/use-automation-runs.ts`

Extend `deleteAllRuns(runIds?: string[])`:

- No argument → unchanged: optimistic `clearRuns(automationId)` + one
  `deleteAllAutomationRuns(automationId, workspaceId)` call; on success the
  store is reconciled with an authoritative post-delete `fetchRuns` (not a
  blanket re-clear, so a run created after the backend delete completed
  survives); on failure → toast + recovery refresh, refresh failure →
  pre-clear snapshot restore.
- Empty array → no-op (defensive; the UI never renders the button for an
  empty view).
- Non-empty ids → snapshot the pre-delete list, `removeRun` each id
  optimistically, then `Promise.allSettled(ids.map((id) => deleteAutomationRun(
  id, workspaceId)))`. All fulfilled → authoritative post-delete refresh. Any
  rejection → one aggregated toast + recovery refresh, run only after every
  delete settles (never on the first rejection); refresh failure → snapshot
  restore. Aggregated: one toast, one refresh per failed batch, not per id.

Race hardening (added during adversarial review):

- **Epoch-guarded fetches.** The store holds a per-automation `mutationEpoch`
  (`automationRuns.mutationEpoch`). Every list fetch captures the epoch at
  start and discards its response if a delete bumped it meanwhile — a refresh
  started before a delete can never resurrect deleted rows.
- **Serialized mutations.** `automationRuns.deleting` holds the in-flight
  mutation's generation (or `false`). `beginAutomationRunDelete` atomically
  bumps the epoch and claims the slot; `deleteRun`/`deleteAllRuns` are no-ops
  while it is held; `endAutomationRunDelete(generation)` releases only its own
  generation. The flag is shared store state, so an editor that unmounts
  mid-delete cannot leave a remounted instance unsynchronized, and a stale
  callback cannot lift a newer mutation's slot.
- The hook exposes `deleting` so the UI disables both the delete-all button
  and the per-row delete buttons while a delete is in flight.

### Store slice — `apps/web/lib/state/slices/automations/`

`AutomationRunsState` gains `mutationEpoch: Record<string, number>` and
`deleting: Record<string, number | false>`, plus
`beginAutomationRunDelete` / `endAutomationRunDelete` actions. No migration
required; the fields default to `{}`.

### Component — `apps/web/components/automations/runs-section.tsx`

- Remove the `DeleteAllButton` from the section header row (next to the
  refresh button).
- Render it inside the last `<TableHead className="w-8">` (the column holding
  the per-row delete buttons), gated on `visibleRuns.length > 0` and
  `expanded` (the table only renders when expanded):
  `onConfirm={() => deleteAllRuns(statusFilter === "all" ? undefined : visibleRuns.map((r) => r.id))}`.
- `DeleteAllButton` gains the current filter so the dialog picks its copy:
  - "all" → existing `deleteAllRunsTitle` / `deleteAllRunsDescription`
    (e2e asserts `permanently remove all run records`).
  - filtered → new `deleteAllRunsScopedTitle` / `deleteAllRunsScopedDescription`
    with `{{status}}` = the localized status label (`t` of the matching
    `STATUS_FILTERS` entry's `labelKey`, e.g. "Skipped", "Archived").

### Locales — `apps/web/src/locales/{en,pseudo,pt-pt,zh-cn}/automations.json`

Add, in all four catalogs (plain punctuation, no em dash):

- `deleteAllRunsScopedTitle`: `Delete all {{status}} runs?`
- `deleteAllRunsScopedDescription`: `This will permanently remove the {{status}} runs shown in this view and their associated tasks. This cannot be undone.`

## Tests

- **What:** scoped delete-all removes exactly the given run ids and calls the
  per-run API per id.
  **File:** `apps/web/hooks/domains/settings/use-automation-runs-delete.test.ts`
  **How:** mock `deleteAutomationRun`; assert per-id calls with
  `WORKSPACE_ID`, optimistic removal of exactly those ids, success
  reconciliation with the authoritative post-delete list (a run created
  after the deletes completed survives), single aggregated failure toast +
  recovery refresh, and double-failure snapshot restore. Add a no-op
  assertion for `deleteAllRuns([])`.
- **What:** delete-all lives in the table header and is view-scoped.
  **File:** `apps/web/components/automations/runs-section.test.tsx`
  **How:** assert the button renders inside the table header (not beside the
  Recent Runs heading); filtered view confirm calls `deleteAllRuns` with the
  visible ids; "All" view calls `deleteAllRuns()` with no ids; empty filtered
  view renders no button; filtered dialog shows the scoped title/description;
  both delete controls are disabled while `deleting` is true.
- **What:** the "All" view still uses the single bulk delete.
  **File:** `apps/web/hooks/domains/settings/use-automation-runs-delete.test.ts`
  **How:** existing no-arg tests already assert `deleteAllAutomationRuns` is
  called with `(AUTOMATION_ID, WORKSPACE_ID)`; they must keep passing
  unchanged.
- **What:** delete race conditions stay closed.
  **File:** `apps/web/hooks/domains/settings/use-automation-runs-delete.test.ts`
  **How:** stale refresh started before a delete and resolving after it is
  discarded; partial batch failure does not recover until every delete
  settles; a second delete-all or per-run delete fired while one is in flight
  is a no-op; the serialization slot is shared across hook instances (an
  unmounted instance's endDelete releases it); every recovery terminal path
  resets `deleting` to `false`; older list responses cannot overwrite newer
  ones.

## E2E Tests

- **Scenario:** Skipped-filtered delete-all removes only the skipped rows.
  **File:** `apps/web/e2e/tests/automations-settings.spec.ts`
  **What to verify:** seed 2 skipped + 1 succeeded run; expand Recent Runs;
  filter Skipped (2 rows); assert `delete-all-runs` sits inside the table
  header; click it and confirm; dialog shows the scoped copy
  (`permanently remove the Skipped runs shown in this view`); table shows 1
  row; switch to All and assert the succeeded row remains.
- **Scenario:** All-view delete-all keeps the existing copy and placement.
  **File:** `apps/web/e2e/tests/automations-settings.spec.ts`
  **What to verify:** the existing "delete individual and all runs from
  Recent Runs" test keeps passing unchanged (it already asserts header
  visibility and the unqualified dialog copy).

## Verification Results

- Hook + component + store unit suites: `pnpm --filter @kandev/web test --
  --run hooks/domains/settings/use-automation-runs.test.ts
  hooks/domains/settings/use-automation-runs-delete.test.ts
  components/automations/runs-section.test.tsx lib/state/store.test.ts` —
  48 tests passed (final state after review rounds).
- `pnpm --filter @kandev/web typecheck` (from `apps/web`) — passed.
- `pnpm --filter @kandev/web lint -- <changed files>` (eslint
  `--max-warnings 0`) — passed.
- `pnpm --filter @kandev/web i18n:check` + `i18n:ratchet` — passed; new keys
  in en/pseudo/pt-pt/zh-cn, pseudo regenerated, no em dashes.
- E2E: `pnpm --filter @kandev/web e2e:raw -g "delete" --workers=4
  automations-settings.spec.ts` — delete-related tests passed; the full
  chromium project suite passed on an earlier run.
- `git diff --check` — passed. Pre-commit hooks (prettier, web lint, i18n
  guard, commitlint) passed on every commit.

## Implementation Waves And Parallel Candidates

Wave 1 (sequential):

- [x] [task-01-hook-status-scoped-delete](task-01-hook-status-scoped-delete.md) — done
- [x] [task-02-runs-table-header-delete-all](task-02-runs-table-header-delete-all.md) — done
- [x] [task-03-e2e-status-scoped-delete](task-03-e2e-status-scoped-delete.md) — done

The hook and the component are a vertical slice (the component consumes the
hook's new signature in the same change cycle); E2E follows both. Eight
adversarial review rounds (Luna, one fresh sub-task per round) ran after
implementation; each round's findings were fixed, re-verified, and committed
before the next. Round 8 reported `NO NEW FINDINGS` with a ready-to-merge
verdict, ending the loop.

## Open Questions

None.
