---
id: "02-runs-table-header-delete-all"
title: "Runs table header delete-all"
status: done
wave: 2
depends_on:
  - "01-hook-status-scoped-delete"
plan: "plan.md"
spec: "../../specs/office/requirements/automation-runs-delete-all-by-status.md"
---

# Task 02: Runs table header delete-all

Move the delete-all control into the Recent Runs table header's rightmost
column and scope its confirmation to the active status view.

## Acceptance

- The delete-all button renders inside the last `<TableHead className="w-8">`
  (the per-row delete column), horizontally aligned with the per-row delete
  buttons, and no longer appears in the section header row next to Refresh.
- With a status filter active, confirming the dialog calls
  `deleteAllRuns(visibleRuns.map((r) => r.id))`; with the "All" filter it
  calls `deleteAllRuns()` with no argument.
- The button is hidden when `visibleRuns.length === 0` (including the "No
  runs match this filter" empty view).
- Dialog copy: the existing `deleteAllRunsTitle` / `deleteAllRunsDescription`
  for "All"; the new scoped keys with the localized status label
  (`t` of the active `STATUS_FILTERS` entry's `labelKey`) when filtered.
- New locale keys `deleteAllRunsScopedTitle` and
  `deleteAllRunsScopedDescription` exist in all four catalogs
  (`en`, `pseudo`, `pt-pt`, `zh-cn`), plain punctuation, no em dash.

## Verification

From `apps/` (fresh worktrees must first run `pnpm install --frozen-lockfile`
from `apps/` when `apps/node_modules/` is absent):

```bash
pnpm --filter @kandev/web test -- --run components/automations/runs-section.test.tsx hooks/domains/settings/use-automation-runs.test.ts
```

Then from `apps/web/`:

```bash
pnpm run typecheck
pnpm run i18n:check
pnpm run i18n:ratchet
pnpm run lint -- components/automations/runs-section.tsx components/automations/runs-section.test.tsx
```

Also run `git diff --check` and Prettier on the changed files before commit.

## Files likely touched

- `apps/web/components/automations/runs-section.tsx`
- `apps/web/components/automations/runs-section.test.tsx`
- `apps/web/src/locales/en/automations.json`
- `apps/web/src/locales/pseudo/automations.json`
- `apps/web/src/locales/pt-pt/automations.json`
- `apps/web/src/locales/zh-cn/automations.json`

## Dependencies

Task 01 (the component consumes the hook's new optional run-id argument).

## Parallelism

Sequential. Component behavior and its rendered regression tests share one
change cycle.

## Inputs

- Spec scenarios: header placement, view-scoped deletion, empty-view
  hiding, scoped dialog copy.
- `runs-section.tsx` current structure: `visibleRuns` / `statusFilter`
  state, the `STATUS_FILTERS` list (status → `labelKey`), the 5-column table
  with the trailing `w-8` header cell, and the existing
  `DeleteAllButton`/`AlertDialog` markup to relocate.
- i18n conventions in `apps/web/AGENTS.md`: all new copy through `t()`,
  all four catalogs updated, no em dash in user-facing copy.

## Output contract

Report the RED assertion, files changed, the exact focused commands and
results, any blockers or risks, and synchronized task/plan status in the same
conversation. No backend changes.

## Results

- RED: new scope/placement tests failed before the component change (button
  still beside the heading, no scoped ids, no scoped copy).
- GREEN: `pnpm --filter @kandev/web test -- --run
  components/automations/runs-section.test.tsx
  hooks/domains/settings/use-automation-runs.test.ts` — 34 tests passed
  (final state).
- Added during review rounds: status-scoped `title`/`aria-label`
  (`deleteAllRunsScoped` key), and `deleting`-gated disablement of both the
  delete-all button and the per-row delete buttons.
- `pnpm run typecheck` (from `apps/web`) — passed; `i18n:check` +
  `i18n:ratchet` — passed (keys in all four catalogs, pseudo regenerated);
  changed-file eslint `--max-warnings 0` — passed; `git diff --check` —
  passed.
