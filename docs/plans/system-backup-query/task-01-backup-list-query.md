---
id: "01-backup-list-query"
title: "Move backup-list ownership to Query"
status: in_progress
updated: 2026-10-06
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-SYSTEM-PAGE-BACKUP-GUIDANCE-001
  - REQ-SYSTEM-PAGE-DATABASE-STATS-SNAPSHOT-001
  - REQ-SYSTEM-PAGE-DATA-STORAGE-PAGES-002
acceptance_criteria:
  - AC-SYSTEM-PAGE-BACKUP-GUIDANCE-001.1
  - AC-SYSTEM-PAGE-BACKUP-GUIDANCE-001.2
  - AC-SYSTEM-PAGE-BACKUP-GUIDANCE-001.3
  - AC-SYSTEM-PAGE-BACKUP-GUIDANCE-001.4
  - AC-SYSTEM-PAGE-BACKUP-GUIDANCE-001.5
  - AC-SYSTEM-PAGE-BACKUP-GUIDANCE-001.6
  - AC-SYSTEM-PAGE-DATABASE-STATS-SNAPSHOT-001.5
  - AC-SYSTEM-PAGE-DATA-STORAGE-PAGES-002.1
  - AC-SYSTEM-PAGE-DATA-STORAGE-PAGES-002.5
system_design:
  - ../../specs/system-page/system-design/backup-list-query-cache.md
---

# Task 01: Move Backup-List Ownership to Query

## Summary

Move the Backups list and its request state to the shared identity-scoped
TanStack Query owner after QUERY-02's frontend migration merged in PR #4225
at `059260b30fc68bbcbead629f7fa7e80f1ee0a5e8`. Remove
backup-only Zustand ownership and preserve current list, mutation, permission,
directory-description, and relaunch behavior.

## In scope

- Add the backup-list query and adapt useBackups without changing the
  Promise<SnapshotInfo[]> reload success or original-error rejection contract.
  Preserve the hook's fields. Derive `loaded` from successful data presence and
  `isLoading` from `isFetching`. Refetch failure keeps a loaded empty list loaded.
  Bind `fetchBackups` to the captured API base URL and forward the native signal.
  Keep BackupsTable's existing local mutation-error display.
- Preserve concurrent-read sharing, finite mutable freshness, native request
  cancellation, last-successful-data behavior, and identity transitions.
- Set `staleTime=30s`, `gcTime=5m`, `refetchOnMount="always"`, stale-only
  focus/reconnect refetch, `retry=false`, `networkMode="always"`, and no
  interval. Preserve last-good data while an inactive entry is retained. The
  mount-always read intentionally replaces Zustand's shared `loaded` guard so
  a later Backups visit observes external maintenance without background
  polling. explicit reload always reads regardless of freshness. Do not reuse
  the DatabaseStats polling schedule.
- Refresh only the captured backup-list query after successful delete and
  locally correlated Tool Payload Retention preparation or Factory Reset
  outcomes that may have published a snapshot. Cancel an earlier exact-key
  read before the read after the writer's result, including an initial read
  without cached data. Default refetch cancellation alone is insufficient.
  Concurrent reloads can share an eligible read. Reload rejects request errors
  and never converts cancellation into a successful empty list.
- Before a backup-choice save, capture its full backup-query identity and
  candidate next policy revision (`submitted revision + 1`). Only a change
  that requires new preparation creates a save candidate. Reuse the existing
  review logic and saved baseline. Ignore carried historical ready state from
  a save without new review. Establish a local
  attempt only when its successful save response confirms that revision with
  backup choice and a preparation state of pending, running, ready, or failed.
  Observe the save response itself. discard the request candidate on rejection
  or a nonmatching response while preserving its result/error contract. A first
  status read of pending/running with backup choice can independently establish
  an observed attempt. Do not rely on the operation ID, which changes on
  success. Preserve save, cancel, and poll errors. Invalidate once at
  ready/failed or when a previously observed pending/running attempt ends
  through cancellation or policy replacement.
  Settle the older attempt before considering a newer revision. ignore
  historical terminal status, unrelated cleanup, repeated polls,
  and stale identity responses. Clear active records before invalidation.
  Keep a bounded settled-revision marker so duplicate responses cannot re-arm
  a settled revision. Clear all correlation state on full identity or lifetime changes.
  Preserve the existing read-generation and serialized-mutation guards.
  End the old retention request epoch and reset its remote state on identity change.
  Old finalizers cannot release a new mutation lock or clear new pending state.
  Guard the draft Save contributor after its await. Old save results cannot
  update the current draft or clear its choice.
- Capture the identity and caller lifetime for create/delete/reset operations.
  Stop old create polling on unmount, logout, or identity change. A-to-B-to-A
  transitions end the old lifetime. Late results cannot refresh another identity
  or recreate obsolete entries.
  An explicit hook reload captures the active generation when invoked, after
  the identity commit. Every create-poll read stays bound to the writer's
  initiating scope, so an identity transition neither aborts a new create's
  poll nor redirects an old writer to a new identity.
- Correlate reset observation with its accepted job ID and initiating lifetime.
  Settle succeeded/failed once across duplicate WS/poll updates. Preserve the
  terminal reset or retention result if backup-list refresh fails.
- Keep manual-create polling as the authoritative result check. Keep restore
  list data until the existing relaunch creates a new process identity.
- Remove backup-only System Zustand state, actions, defaults, hydration, and
  tests while preserving unrelated state.
- Preserve member-readable GET, admin-only mutations/download, and
  DatabasePanel backup_directory rendering.
- Add focused hook, provider, consumer, mutation, state, desktop, and phone
  regression evidence through `/tdd`. Use the plan's acceptance and evidence map.
  Prove cache behavior with controlled timers, focus/online events, and deferred requests.
  Do not add tests that only repeat the option values.
- Update only QUERY-03's tracker row and scoped delivery/design records.
  Open one focused PR to main after the listed checks. Handle exact-head CI
  and actionable review findings through `/pr-fixup`. Do not merge.

## Out of scope

- Backend routes, snapshot semantics, retention, restore/reset process flow,
  confirmations, and authorization changes.
- QueryClient replacement, whole-cache clearing, shell remounting, or a
  generic WebSocket invalidation layer.
- Disk usage, boot-payload expansion, or any visual redesign.

## Acceptance

- One identity-scoped Query entry is authoritative for the backup list. failed
  reads and mutations never replace its last successful list with an empty
  result, and explicit reload preserves its return and rejection contract.
- Each in-scope list writer refreshes only the current backup key, while
  restore and external startup/maintenance follow the process-identity or
  next-read behavior in the system design.
- Existing directory copy, empty/loading/error presentation, member access,
  admin action gates, desktop behavior, phone behavior, and unrelated shell
  state remain correct.

## Verification

If the worktree has no usable apps/node_modules, first run:

    (cd apps && pnpm install --frozen-lockfile)

Then run:

    (cd apps && pnpm --filter @kandev/web exec vitest run hooks/domains/system/use-backups.test.tsx hooks/domains/system/use-backups-freshness.test.tsx hooks/domains/system/backup-list-query.test.ts hooks/domains/system/backup-preparation-correlation.test.ts hooks/domains/system/use-database-stats.test.ts hooks/domains/system/use-tool-payload-retention.test.ts hooks/domains/system/use-tool-payload-retention-draft.test.tsx hooks/domains/system/use-system-info.test.tsx components/settings/system/backups-table.test.tsx components/settings/system/tool-payload-retention-card.test.tsx components/settings/system/system-confirmation-dialogs.test.tsx components/settings/system/system-route-copy.test.ts lib/state/slices/system/system-slice.test.ts lib/api/domains/system-api.test.ts)
    (cd apps && pnpm --filter @kandev/web run typecheck)
    (cd apps && pnpm --filter @kandev/web run lint)
    scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --project chromium e2e/tests/system/backups-page.spec.ts e2e/tests/system/factory-reset-dialog.spec.ts
    scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --project auth e2e/tests/auth/system-data-storage-member-gating.spec.ts
    scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --project mobile-chrome e2e/tests/auth/mobile-system-data-storage-member-gating.spec.ts
    python3 scripts/lint-architecture.py --all
    python3 scripts/list-docs.py validate
    python3 scripts/lint-spec-files.py --all
    git diff --check

Run E2E sequentially with the repository-managed resource limits and its production build.
The focused list includes new backup-query, preparation-correlation, and
retention-draft suites. QUERY-02's shared provider and database-statistics
tests remain in the paths listed above.
The changed test-suite inventory must match the final verification command.

Run this workspace documentation-coverage preflight from the repository root:

```bash
node <<'NODE'
const fs = require('node:fs');
const { execFileSync } = require('node:child_process');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const lines = (args) => execFileSync('git', args, { encoding: 'utf8' })
  .split('\n').filter(Boolean);
const paths = [...new Set([
  ...lines(['diff', '--name-only', 'origin/main...HEAD']),
  ...lines(['diff', '--name-only', 'HEAD']),
  ...lines(['ls-files', '--others', '--exclude-standard']),
])];
const scope = paths.some((name) => name.startsWith('apps/'))
  ? 'workspace changes' : 'design source-path probe';
if (scope === 'design source-path probe') {
  paths.push('apps/web/hooks/domains/system/use-backups.ts');
}
const documents = [
  'docs/plans/system-backup-query/plan.md',
  'docs/plans/system-backup-query/task-01-backup-list-query.md',
  'docs/specs/system-page/system-design/backup-list-query-cache.md',
  'docs/specs/system-page/requirements/backup-location-actions.md',
  'docs/specs/system-page/requirements/database-statistics-snapshot.md',
  'docs/specs/system-page/requirements/system-data-storage-pages.md',
];
const fileContents = Object.fromEntries(documents.map((name) =>
  [name, fs.readFileSync(name, 'utf8')]));
const changedFiles = paths.map((filename) => ({ filename,
  status: fs.existsSync(filename) ? 'modified' : 'removed' }));
const result = validateCoverage({ changedFiles, fileContents });
console.log(JSON.stringify({ scope, status: result.status,
  workOrders: result.workOrders, errors: result.errors }, null, 2));
if (!result.ok) process.exitCode = 1;
NODE
```

This preflight includes committed, staged, unstaged, and untracked paths.
A docs-only review uses the explicit planned source-path probe to check references.
That probe is not implementation evidence. After the dependency lands, reconcile
its referenced designs if this work order adds them. Final CI coverage must refer
to the exact PR head/base revisions.
Also inspect untracked-file whitespace because `git diff --check` skips new unstaged files.

## Retention regression cases

- Successful preparation and failure after the backup file was visibly
  published each invalidate the exact backup list once.
- An accepted backup-choice save response that is already ready/failed
  invalidates once. A save returning pending/running establishes a revision
  record. an initial status read of pending/running with backup choice can do
  the same. Initial historical ready/failed status does not invalidate without a qualifying save response or observed active attempt.
  No attempt ID or event distinguishes it safely.
- Repeated ready/failed polls after settlement do not request additional list
  reads. Cancellation that resets Preparation to none, and policy replacement
  that changes the revision, each settle an observed pending/running attempt
  once, including when a published file survives. Do not compare terminal
  Operation IDs because success replaces the backup Operation with cleanup.
- An unchanged or less aggressive enabled policy can carry an old ready
  preparation into a new revision. Even with backup choice, it does not create
  a new attempt. Duplicate accepted responses and delayed pending/running at
  a settled revision or an older revision cannot re-arm it.
- Skip choice and unrelated cleanup do not create their own invalidation.
  A skip-choice replacement still settles a previously observed backup attempt. A later backup-choice
  attempt at a new revision is tracked and invalidated independently.
- An active list observer refetches. an inactive entry becomes stale without
  a request. A failed list refresh preserves a successful save/cancel response.
  Defer status, save, and cancel responses across a full query
  identity change. when they resolve, they cannot update retention state or
  invalidate the current backup key. An old save result cannot update a new
  retention draft or clear its backup choice.
- Reset regressions cover succeeded/failed, repeated WS/poll terminal updates,
  rejected job acceptance, stale job identity, and late failure after snapshot publication.
- List regressions cover initial deferred reads without data and cached reads across a writer boundary.
  Also cover one remaining observer, StrictMode replay, same-tick reloads, offline local reads, and rapid A-to-B-to-A transitions.
- Verify that a later mount fetches external maintenance changes, including while cached data remains fresh.
  Verify stale-only focus/reconnect reads and explicit reload while fresh.
  Verify that no periodic backup-list polling occurs.

## Files likely touched

- apps/web/hooks/domains/system/backup-list-query.ts and its new focused test
  (backup-only key/options/invalidation helpers, using QUERY-02's identity)
- apps/web/hooks/domains/system/use-backups.ts and its new focused test
- apps/web/hooks/domains/system/use-tool-payload-retention.ts and its test
- apps/web/hooks/domains/system/use-tool-payload-retention-draft.ts for the
  preparation-review predicate and post-save lifetime guard, covered by
  retention hook/card tests
- apps/web/components/settings/system/backups-table.tsx and its test
- apps/web/components/settings/system/factory-reset-dialog.tsx and related
  System confirmation tests
- apps/web/lib/state/slices/system/index.ts
- apps/web/lib/state/slices/system/types.ts
- apps/web/lib/state/slices/system/system-slice.ts
- apps/web/lib/state/slices/system/system-slice.test.ts
- apps/web/components/system-info-query-provider.tsx and the shared query
  identity implementation delivered by QUERY-02 (register the backup prefix
  in its targeted obsolete-query cleanup; do not redesign shared identity)
- apps/web/lib/api/domains/system-api.test.ts
- apps/web/components/settings/system/system-route-copy.test.ts and QUERY-02's
  DataLogsSettings/DatabasePanel contract, only if a post-gate correction is needed
- apps/web/e2e/tests/system/backups-page.spec.ts
- apps/web/e2e/tests/system/factory-reset-dialog.spec.ts
- apps/web/e2e/tests/auth/system-data-storage-member-gating.spec.ts
- apps/web/e2e/tests/auth/mobile-system-data-storage-member-gating.spec.ts

- docs/architecture-maintenance/server-state-migrations.md (QUERY-03 only)
- This plan, work order, owning query design, and ownership ADR
- docs/specs/system-page/system-design/backup-location-actions.md only if the
  merged dependency leaves obsolete backup-directory ownership prose

## Dependencies

Satisfied: child task 95ad66df-588d-48d1-afee-5c674c928013's frontend
database-statistics Query ownership PR #4225 merged at
`059260b30fc68bbcbead629f7fa7e80f1ee0a5e8`. The initial implementation base
was refreshed to `95c040e84951773dc8ed6f684fff0425d7b63f96`; current-main
validation is recorded below. Backend PR #4001 was not used as a substitute.

## Risks

- The shared provider now registers the backup-list prefix with QUERY-02's
  exact identity cleanup. No separate provider or identity is introduced.
- Backup preparation may publish a manual snapshot before reporting failure.
  Success replaces the Operation, and cancellation/policy replacement clears
  preparation state. Correlate with the save response, expected policy
  revision, and local transition record. refresh once after every observed
  terminal path and retain existing Query data if that read also fails.
- Successful reset and restore still require their current quit/relaunch flow.

## Parallelism

sequential

## Inputs

- REQ-SYSTEM-PAGE-BACKUP-GUIDANCE-001 and
  REQ-SYSTEM-PAGE-DATABASE-STATS-SNAPSHOT-001 and
  REQ-SYSTEM-PAGE-DATA-STORAGE-PAGES-002
- The Backup List Query Cache System Design
- The merged QUERY-02 provider, identity, cancellation, and cleanup code
- Existing Backups page, Tool Payload Retention, Factory Reset, and restore
  callers and tests

## Results

Implementation is complete locally against the merged QUERY-02 contract.

- Dependency: PR #4225 merged at
  `059260b30fc68bbcbead629f7fa7e80f1ee0a5e8`. Initial implementation base:
  `95c040e84951773dc8ed6f684fff0425d7b63f96`. Initial exact-current-main
  validation used `05c41b11e830a9861534200949c3bbac473b57f7` after unrelated
  PR #4262. Remote main later advanced through unrelated PR #4266 to
  `c373ba436c3451f046acd921d5de9a081977a2a7`.
  Exact-current-main merge-tree validation passed for implementation commit
  `4fe499fe4e68a9eb51fe87b12aaf61d9dc1d7b99`, producing tree
  `e45391aa7811b5a84be83bfc49e27aa561ef1a5f`; it will be rerun on the final
  pushed head. The code-fixup commit `c37fac05b6bdb40d1843bd74b1fe31730b7f5138`
  also merged cleanly with the later main tip, producing tree
  `48b767f74120fe256732c7cc7ef0c04939c9a710`.
- Focused PR: [#4271](https://github.com/kdlbs/kandev/pull/4271),
  `refactor: move backup list to Query`, targeting `main`. The PR is open and
  linked to Kandev task `24c8f330-1bbd-4cb7-84af-1d86c9b335ca`. The exact
  final PR head and exact-head CI/review disposition will be recorded in the
  task handoff after `/pr-fixup` completes.
- `pnpm --filter @kandev/web exec vitest run ...` with all 14 listed suites:
  passed, 161 tests. Coverage includes shared consumers, reload/error
  contracts, freshness, identity cleanup, retention attempt correlation,
  create/delete/reset boundaries, permissions, and preserved shell state.
- `pnpm --filter @kandev/web run typecheck`: passed.
- `pnpm --filter @kandev/web run lint`: passed with zero warnings.
- Regression for create polling after a deferred old-identity read: passed.
  It failed before reload used the generation assigned to the committed
  identity, and before create polls stayed on the initiating writer scope.
- A second A-to-B-to-A regression proves an old A reload callback cannot
  revive after the identity returns to A.
- A third regression proves an accepted create poll retains its initiating
  identity and cannot redirect later reads after that identity changes.
- Managed Chromium Backups/reset E2E: 4 passed. Desktop member-gating E2E: 2
  passed. Mobile member-gating E2E: 2 passed.
- `python3 scripts/lint-architecture.py --all`,
  `python3 scripts/list-docs.py validate`, and
  `python3 scripts/lint-spec-files.py --all`: passed, along with the harness
  lint, harness tests, targeted `harness-lint` hook, web typecheck and web lint.
- `git diff --check`: passed for the current fixup changes; it will run again
  before commit and against the final pushed head.
- Documentation-coverage preflight: covered by this work order, no errors.
- PR capture: one disposable mobile-project spec captured and validated four
  synthetic desktop/phone Backups and reset-confirmation assets. The spec was
  removed after capture; compressed PNGs are kept outside the PR branch for
  description publication.
- Public docs: no public documentation change. This migration changes internal
  cache ownership and retains the existing API, copy, permissions, and operator
  procedure; desktop and phone E2E preserve the user-facing behavior.
- `apps/web/AGENTS.md` now describes the backup list's finite Query lifecycle
  and links its decision and system design; no obsolete Zustand ownership
  guidance remains in the scoped web guide.

Before completion, record the final `git diff --check`, PR URL, exact base SHA,
final head SHA, exact-head CI/review status, residual risks, and dependency
order in the delivery record and task handoff. Mark this work order done only
after implementation, checks, and delivery are complete. Keep QUERY-03's
tracker status in progress until its PR actually merges.
