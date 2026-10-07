---
id: "01-disk-usage-query"
title: "Move disk usage to Query with job-event recovery"
status: in_progress
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-SYSTEM-PAGE-SYSTEM-PAGE-001
acceptance_criteria:
  - AC-SYSTEM-PAGE-SYSTEM-PAGE-001.2
system_design:
  - ../../specs/system-page/system-design/disk-usage-query-cache.md
---

# Task 01: Move disk usage to Query with job-event recovery

## Summary

Move only the disk snapshot and GET request state to TanStack Query. Preserve
the existing disk-walk job stream, card behavior, refresh sequence, and
conditional recovery while removing the disk snapshot from the broad Zustand
System slice.

## In scope

  - Reconciled the landed QUERY-03 identity helper, stable QueryClient, and
    obsolete-key cleanup before shared provider edits.
- Add the disk-usage query and exact-key cleanup using the shared identity.
- Add one disk-specific observer over `system.jobs` that baselines retained
  rows and invalidates on terminal disk-walk success or failure.
  Use a synchronous store subscription and retain no second job map.
  - Verified the in-flight event schedule against TanStack Query 5.104.0 with
  deferred responses before choosing the smallest cancellation/invalidation
  mechanism. Exercise delayed cancel completion across A→B→A and guard both
  the captured event callback and its baseline/subscription lifetime.
- Remove only disk-usage Zustand state, defaults, and actions; keep all other
  System state and consumers intact.
- Keep the existing card's copy, loading, error, refresh, and responsive
  presentation. Preserve the hook caller contract: `reload` and `refresh`
  resolve after handled failures, every GET (including background, manual,
  and event reads) contributes to `isLoading`, and failed refetches retain the
  last-good snapshot. Query owns GET state/error; only separate refresh POST
  feedback remains local.

## Out of scope

- Backend or WebSocket protocol changes, reconnect refreshes, and changes to
  system-job ownership.
- Changes to database, backups, storage, metrics, or other Query resources.
- New UI, translations, global event infrastructure, or blanket cache resets.

## Acceptance

1. TanStack Query is the sole disk snapshot and GET request-state owner; local
   state is limited to separate refresh POST feedback. The query uses the
   shared full backend/boot/auth identity, consumes the native fetch signal,
   and removes only obsolete disk resource identities while preserving the
   shell and A→B→A cache isolation.
2. The real `system.job.update` handler updates Zustand jobs unchanged. One
   disk-specific observer revalidates the exact current disk key on terminal
   success and failure, ignores unrelated events and retained baseline jobs,
   suppresses duplicate and out-of-order terminal events, and bounds its FIFO
   at 64 IDs per identity. Cover a running replay followed by repeated
   terminal updates. Replay of an evicted ID while its retained store row is
   terminal does not issue another request. Also document/test the bounded
   limit: after FIFO eviction, row removal or nonterminal replay can erase
   terminal evidence. A subsequent terminal replay can revalidate. Cover both
   paths, without changing the handler's full-row upsert contract.
   Cover terminal→running→terminal updates in one React batch and deep-merged
   job additions after subscription. No React render can erase an accepted
   terminal transition.
3. Deferred tests prove that a terminal event during a no-data initial GET or
   cached refetch is followed by an authoritative read, even when the older
   response later resolves with stale `computing=false` or `computing=true`,
   including a transport response that settles after its signal is already
   aborted. Use the installed Query APIs. A delayed cancellation completion
   combined with A→B→A must not invalidate, refetch, or remove the newly
   current A entry or cause duplicate reads; stale callbacks must check the
   captured full identity and bridge subscription/baseline generation before
   continuing, and all operations must target the captured exact key. Exercise
   a terminal-event burst with N accepted updates and at most N replacement
   GETs beyond the already pending GET. Duplicates and unrelated updates add
   zero GETs. Any coalesced read starts after the last accepted update.
   Assert the FIFO never exceeds 64 IDs. If Query's native APIs do not meet these
   cases, stop and report the evidence before expanding the mechanism.
4. Refresh remains POST then GET. Both terminal outcomes revalidate. A missed
   event is recovered by 1500ms polling only after the latest successful
   snapshot says `computing=true`; polling stops on authoritative
   `computing=false`, and continues periodically after errors if retained data
   remains computing. Query's fetching state keeps `isLoading` true during
   initial and refetch GETs. GET failures remain visible while the prior
   snapshot remains available. POST failure remains visible in the existing
   card error row, while `refresh` still resolves after handling it. An
   obsolete identity's late GET or POST result cannot clear or set the current
   identity's loading/error feedback. No unconditional focus/reconnect read
   is introduced.
   An obsolete successful POST starts no GET, including after A→B→A or hook
   remount. Follow the design's error reset and precedence rules. A failed
   POST starts no follow-up GET, and POST success followed by GET failure
   shows the GET error.
5. Auth, backend, boot, logout, and unmount schedules cancel or isolate only
   the relevant query. Existing shell state and every non-disk System resource
   remain unchanged.
   Cover auth-disabled identity, StrictMode subscription replay, multiple disk
   consumers sharing a GET, and zero listeners after bridge cleanup.
   With no disk consumer, events create no query or GET. An existing inactive
   query retains last-good data and invalidation for the next mount, including
   route departure between cancellation and invalidation.
6. The existing desktop disk-usage E2E and focused mobile Status-card E2E pass.
   The card's layout and viewport-dependent behavior remain unchanged.
   Both refresh flows assert POST then GET and the resulting breakdown and
   timestamp. Strengthen the existing desktop test, which only observes POST.
   The missed-event proof belongs to deterministic hook tests. The existing
   first-load browser test does not force a cold cache or dropped event.

## Verification

Run the deferred Query API tests first, before implementing the bridge.
After the dependency refresh, reconcile this list with the landed provider,
identity, and API test paths. Run every suite changed during implementation.
The commands below start from the repository root. The managed E2E runner
rebuilds the product unless `--no-build` is supplied. Run both projects
sequentially with its default memory and worker limits.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run hooks/domains/system/use-disk-usage.test.tsx hooks/domains/system/disk-usage-query.test.ts hooks/domains/system/disk-usage-query-options.test.ts components/system-disk-usage-query-bridge.test.tsx hooks/domains/system/use-system-info.test.tsx hooks/domains/system/use-database-stats.test.ts hooks/domains/system/use-backups.test.tsx hooks/domains/system/use-backups-freshness.test.tsx hooks/domains/system/use-tool-payload-retention.test.ts lib/ws/handlers/system-events.test.ts lib/state/slices/system/system-slice.test.ts lib/state/hydration/hydrator.test.ts)
(mkdir -p /root/tmp-kandev-e2e)
(cd apps/web && TMPDIR=/root/tmp-kandev-e2e pnpm e2e:run --host --project chromium e2e/tests/system/disk-usage.spec.ts)
(cd apps/web && TMPDIR=/root/tmp-kandev-e2e pnpm e2e:run --host --project mobile-chrome e2e/tests/system/mobile-disk-usage.spec.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run lint)
python3 scripts/lint-architecture.py --all
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short -- docs/plans/disk-usage-query-migration
```

Run the local PR-documentation coverage API against the final diff and linked
documents. This command makes no GitHub writes and needs no web dependencies.

```bash
node <<'NODE'
const fs = require('node:fs');
const { execFileSync } = require('node:child_process');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const base = execFileSync('git', ['merge-base', 'HEAD', 'origin/main'], { encoding: 'utf8' }).trim();
const paths = [
  ...execFileSync('git', ['diff', '--name-only', '-z', base], { encoding: 'utf8' }).split('\0'),
  ...execFileSync('git', ['ls-files', '--others', '--exclude-standard', '-z'], { encoding: 'utf8' }).split('\0'),
].filter(Boolean);
const documents = [
  'docs/plans/disk-usage-query-migration/plan.md',
  'docs/plans/disk-usage-query-migration/task-01-disk-usage-query.md',
  'docs/specs/system-page/system-design/disk-usage-query-cache.md',
  'docs/specs/system-page/requirements/system-page.md',
];
const result = validateCoverage({
  changedFiles: [...new Set(paths)].map(filename => ({ filename })),
  fileContents: Object.fromEntries(documents.map(path => [path, fs.readFileSync(path, 'utf8')])),
});
console.log(JSON.stringify(result, null, 2));
if (!result.ok || result.status !== 'covered') process.exitCode = 1;
NODE
```

The final implementation diff must produce `covered`, with this changed work
order and no reference errors. A documents-only diff can be `exempt` and does
not prove implementation coverage.

## Files changed

- `apps/web/components/system-info-query-provider.tsx`
- `apps/web/AGENTS.md` (System Query ownership)
- `apps/web/components/system-disk-usage-query-bridge.tsx`
- `apps/web/components/system-disk-usage-query-bridge.test.tsx`
- `apps/web/src/app-error-boundary.test.tsx` (complete the Zustand mock for the
  disk bridge introduced by this task)
- `apps/web/hooks/domains/system/system-info-query.ts`
- `apps/web/hooks/domains/system/disk-usage-query.ts`
- `apps/web/hooks/domains/system/disk-usage-query.test.ts`
- `apps/web/hooks/domains/system/disk-usage-query-options.test.ts`
- `apps/web/hooks/domains/system/use-disk-usage.ts`
- `apps/web/hooks/domains/system/use-disk-usage.test.tsx`
- `apps/web/lib/state/app-state-types.ts`
- `apps/web/lib/state/hydration/hydrator.ts`
- `apps/web/lib/state/slices/system/types.ts`
- `apps/web/lib/state/slices/system/system-slice.ts`
- `apps/web/lib/state/slices/system/system-slice.test.ts`
- `apps/web/e2e/tests/system/disk-usage.spec.ts`
- `apps/web/e2e/tests/system/mobile-disk-usage.spec.ts`
- `apps/web/e2e/tests/session/session-resume-recovery.spec.ts` (CI fixup:
  wait for session recovery to reach its ready state before checking the
  relocation response)
- `docs/specs/system-page/system-design/system-page-01.md`
- `docs/specs/system-page/system-design/disk-usage-query-cache.md`
- `docs/decisions/2026-10-05-disk-usage-query-invalidation.md`
- `docs/plans/disk-usage-query-migration/plan.md`
- `docs/plans/disk-usage-query-migration/task-01-disk-usage-query.md`
- `docs/architecture-maintenance/server-state-migrations.md` (QUERY-04 row
  only)

## Dependencies

Satisfied: QUERY-02 database Query migration merged as
`059260b30fc68bbcbead629f7fa7e80f1ee0a5e8`; QUERY-03 backup Query migration
merged as `d149627883ca74de0dde98c1900415729af44bbd`. The dependency-prepared
worktree base is `b3f207b2e7f08f39c628db8b1d5d0a1374e1b3d7`; both dependency
merges are ancestors of that base. This work does not alter the disk backend
or job-stream protocol.

## Risks

- Deferred tests prove the installed Query version replaces an in-flight
  no-data request when the bridge cancels then invalidates its exact key.
- The runner parses `--host` and `--project <name>` as runner options and
  forwards the test path to Playwright. Each command uses its own
  repository-root-relative subshell so directory changes cannot redirect
  later validators.
- Retained jobs have no backend URL or auth-generation field. The current app
  uses one page-static backend URL, and the bridge baselines retained rows on
  identity changes. Do not infer that WebSocket reconnect replays missed jobs.
- Repeated errors with retained `computing=true` data continue periodic reads
  at 1.5-second intervals until data changes or the observer leaves.

## Parallelism

`sequential`

## Inputs

- [Disk Usage Query Cache](../../specs/system-page/system-design/disk-usage-query-cache.md)
- [Disk Usage Query Invalidation ADR](../../decisions/2026-10-05-disk-usage-query-invalidation.md)
- Existing disk usage requirement and backend/system-job source.
- The Query identity and provider contract merged by QUERY-03.

## Results

Implemented on dependency-prepared base
`b3f207b2e7f08f39c628db8b1d5d0a1374e1b3d7`. The PR initially targeted `main`
at `1204f0e5488418d0d9aa9aaaf6a31988ccdfcd8f`. During review fixup, `main`
advanced to `330e02a47808c11ca315ae30456fcce7f4806db5` with the workspace
recovery change. `apps/web/lib/state/app-state-types.ts` was the only overlapping
file; the main change adds a separate projection field. A synthetic merge of
PR head `d6e5e03ab3a0e8e8f1df05bfff0a3c0e679acaf0` into that base was conflict
free; 145 focused tests and `pnpm run typecheck` passed on the merged tree.
No rebase or shared-contract expansion was needed.

- TanStack Query 5.104.0 deferred tests passed for no-data initial GETs and
  cached-data refetches. Exact-key cancel followed by invalidation starts an
  authoritative post-event GET; late already-aborted responses reporting
  stale `computing=false` or `true` do not replace it. A delayed cancellation
  continuation is isolated across A→B→A.
- Real handler → Zustand map → bridge → Query tests cover successful and failed
  terminal jobs, retained baselines, duplicates/out-of-order events,
  hydration additions, FIFO eviction, identity changes, inactive queries,
  unrelated state, unmount, and StrictMode cleanup. A synchronous burst of 16
  distinct terminal IDs measured 17 GETs total: one initial read plus 16
  replacements. Duplicate and out-of-order replays added zero.
- Focused web tests: 12 suites, 145 tests passed. `pnpm run typecheck` and
  full `pnpm run lint` passed.
- Desktop managed E2E: 5/5 passed. Mobile managed E2E: 1/1 passed at 390×844.
  Both verified POST then GET and the refreshed card data; mobile also verified
  no horizontal overflow. Card layout and viewport-dependent behavior remain
  unchanged.
- The CI recovery-test fixup waits for `WAITING_FOR_INPUT` before asserting the
  relocation response. The focused scenario passed 5/5 on a synthetic
  current-main merge in the CI runtime image with 2 CPUs and 4 GiB; no product
  code changed.
- Architecture lint, docs catalog validation (360 decisions, 1424 specs),
  spec lint and its 36 tests, PR documentation coverage (`covered`, no errors),
  and `git diff --check` passed.

PR [#4291](https://github.com/kdlbs/kandev/pull/4291) is open against
`main`. The initial implementation commit is
`7f5319f3f191c9f8310314cf2e3201fa0fbc6f48`; review-fixup commit
`f19509441558d3354f09774b496246a694854e9a` addresses stale GET-error
presentation during a refresh POST, updates the mobile causal wait, and corrects
the scoped ownership guide. The observed base
was `1204f0e5488418d0d9aa9aaaf6a31988ccdfcd8f` at PR creation. Required checks
and AI reviews were pending at that time. Complete authorized exact-head
fixup before handoff; do not merge.
