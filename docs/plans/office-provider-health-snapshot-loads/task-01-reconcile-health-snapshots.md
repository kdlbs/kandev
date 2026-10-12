---
id: "01-reconcile-health-snapshots"
title: "Reconcile provider-health snapshots with observed rows"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-OFFICE-LIVE-UPDATES-002
acceptance_criteria:
  - AC-OFFICE-LIVE-UPDATES-002.1
  - AC-OFFICE-LIVE-UPDATES-002.2
  - AC-OFFICE-LIVE-UPDATES-002.3
  - AC-OFFICE-LIVE-UPDATES-002.4
  - AC-OFFICE-LIVE-UPDATES-002.5
  - AC-OFFICE-LIVE-UPDATES-002.6
  - AC-OFFICE-LIVE-UPDATES-002.7
  - AC-OFFICE-LIVE-UPDATES-002.8
  - AC-OFFICE-LIVE-UPDATES-002.9
  - AC-OFFICE-LIVE-UPDATES-002.10
system_design:
  - ../../specs/office/system-design/live-updates-02.md
---

# Task 01: Reconcile provider-health snapshots with observed rows

## Summary

Make accepted provider-health snapshots retain actual current rows changed or
added since their request began. Hydrate unaffected rows and retain the current
selection/request guards, using one immediate existing setter publication.

## In scope

- `useProviderHealth` request-local row capture and accepted-success merge.
- One fetch-only, real-hook/store/API/registered-WS-handler regression suite
  implementing the [manifest matrix](plan.md#tests).
- Existing predecessor lifecycle suite as scoped compatibility evidence.
- Existing page hydration fixture's owning-store API compatibility, keeping
  all three dashboard assertions.
- Actual verification results here and in the manifest after implementation.

## Out of scope

Routing preview changes, run attempts, backend/API/schema/store/action redesign,
generic coordinator, global/cross-tab caching, new revision/timestamp policy,
deletion journal, routing policy, polling/retries, component UI/copy/layout/
browser/build/E2E, unrelated docs/policy, new tasks/tabs/delegates or model
switches. Preserve ROOT proof and predecessor files. No permanent test or
production edit until a later ROOT implementation interrupt and global local-heavy grant.

## Acceptance

1. The actual initial and manual deferred GET/registered-WS path fails causally
   before production edits, then preserves full changed/new live rows by the
   full triple while hydrating unaffected HTTP rows (`.9`, `.10`).
2. Empty/fresh HTTP reads, earlier/later events, direct atomic setter behavior,
   current failures, latest-read fences, lifecycle and independent-store/
   workspace/consumer semantics satisfy the complete manifest matrix and
   preserve prior `.1` through `.8` without a coordinator or action change.
3. The two scoped hook suites and exact changed-file/typecheck/i18n/docs gates
   pass with actual terminal joins; mobile/public-doc assessment remains
   truthful. Record every meaningful failure and actual final result.

## Implementation sequence

After ROOT releases this reviewed package in this same primary session and
grants the exclusive global local-heavy lease, read the package, scoped
AGENTS.md and TDD skill; mark this order `in_progress`. Check existing Node and
pinned pnpm; perform at most one frozen apps install if dependencies are absent.
Write the two permanent initial/manual regressions from the accepted proof's
real boundary, not by copying/altering/replaying ROOT's temporary archive.
Run the RED command and verify stale healthy replaces already-observed
degraded in the real store. Setup, teardown, timeout or collection errors are
not causal RED. Then implement the bounded hook publication correction and
complete meaningful matrix cases and controls.

Capture actual `store.getState()` rows before GET; compare current/start
identity and presence for the full key after the existing ownership guard.
Use one synchronous setter, preserving complete protected rows and untouched
snapshot hydration. Use `useAppStoreApi`, include store identity in callback
ownership, and keep prior layout cleanup/newest-request fences intact. Keep
stable empty selectors and localized error fallback. No new runtime action,
store schema, request coordinator or dependency is anticipated.

Tests keep StateProvider/createAppStore, selectors/actions, hook, API client
and registered WS handler real; only global fetch is stubbed. For the new
suite use describe title `Office provider health snapshot publication` and
the two exact RED titles named below. Observe live health before resolving
HTTP and full keyed health after it, including unaffected hydration. Never
replace the handler with direct synthetic publication or mirror a proposed
predicate. Fixture cleanup settles all deferred responses, joins refresh
promises, unmounts consumers and restores fetch, even when assertions fail.

## Verification

Run from repository root in Bash with `login=false`, serially, ONLY after
ROOT's implementation/resource release. Retain every returned session handle
and join each operation to terminal completion before the next. Timeouts are
checkpoint bounds, not retry permission. No broad suite or passing replay.

```bash
export PATH="/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:$PATH"
export NODE_OPTIONS="--max-old-space-size=4096"

# Expected pnpm9.15.9; one conditional frozen apps install if absent.
(cd apps && timeout --signal=TERM --kill-after=5s 30s corepack pnpm@9.15.9 --version)
if [ ! -d apps/node_modules ] || [ ! -d apps/web/node_modules ]; then
  (cd apps && timeout --signal=TERM --kill-after=10s 600s corepack pnpm@9.15.9 install --frozen-lockfile)
fi

# RED before the hook correction: these two exact real-path assertions.
(cd apps/web && timeout --signal=TERM --kill-after=10s 120s corepack pnpm@9.15.9 exec vitest run hooks/domains/office/use-provider-health-snapshot-loads.test.tsx --project browser-locales --maxWorkers=1 --no-file-parallelism -t '^Office provider health snapshot publication retains live health across delayed (initial|manual) snapshot$')

# GREEN: new transport-only matrix plus all existing workspace/lifetime guards.
(cd apps/web && timeout --signal=TERM --kill-after=10s 120s corepack pnpm@9.15.9 exec vitest run hooks/domains/office/use-provider-health-snapshot-loads.test.tsx hooks/domains/office/office-diagnostic-workspace-reads.test.tsx --project browser-locales --maxWorkers=1 --no-file-parallelism)
(cd apps/web && timeout --signal=TERM --kill-after=10s 120s corepack pnpm@9.15.9 exec eslint --max-warnings 0 hooks/domains/office/use-provider-health.ts hooks/domains/office/use-provider-health-snapshot-loads.test.tsx hooks/domains/office/office-diagnostic-workspace-reads.test.tsx)
(cd apps/web && timeout --signal=TERM --kill-after=10s 300s corepack pnpm@9.15.9 run typecheck)
(cd apps/web && timeout --signal=TERM --kill-after=10s 120s corepack pnpm@9.15.9 run i18n:check)
(cd apps/web && timeout --signal=TERM --kill-after=10s 120s corepack pnpm@9.15.9 run i18n:ratchet --base 3cfb02ffb1162066df8ffd99ce8e3665438077d2)
timeout --signal=TERM --kill-after=5s 60s python3 scripts/list-docs.py validate
timeout --signal=TERM --kill-after=5s 60s python3 scripts/lint-spec-files.py --all
timeout --signal=TERM --kill-after=5s 30s node <<'NODE'
const fs = require('node:fs');
const { execFileSync } = require('node:child_process');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const docs = [
  'docs/specs/office/requirements/live-updates.md',
  'docs/specs/office/system-design/live-updates-02.md',
  'docs/plans/office-provider-health-snapshot-loads/plan.md',
  'docs/plans/office-provider-health-snapshot-loads/task-01-reconcile-health-snapshots.md',
];
const names = [
  ...execFileSync('git', ['diff', '--name-only', '3cfb02ffb1162066df8ffd99ce8e3665438077d2'], { encoding: 'utf8' }).trim().split('\n'),
  ...execFileSync('git', ['ls-files', '--others', '--exclude-standard'], { encoding: 'utf8' }).trim().split('\n'),
].filter(Boolean);
const result = validateCoverage({
  changedFiles: [...new Set(names)].map(filename => ({ filename, status: 'modified' })),
  fileContents: Object.fromEntries(docs.map(file => [file, fs.readFileSync(file, 'utf8')])),
});
console.log(JSON.stringify(result, null, 2));
if (!result.ok) process.exitCode = 1;
NODE
git diff --check
git status --short
```

The RED nonzero exit is expected only for the causal assertions. The GREEN
path filters select the two exact scoped files; the anchored RED name must
collect exactly two cases. Collection drift is a fixture/command issue, never
green evidence. If fixtures need a helper file to stay within lint limits,
keep it test-only and add that changed file to the eslint/coverage evidence.
Typecheck includes its normal generation hook; inspect generated changes
without discarding foreign work. No install/runtime/PATH configuration edits.

When implementation passes, mark this order `done`, synchronize the manifest
and record real commands/exits/counts. Keep the broader migrated requirement
and design draft; do not claim their unrelated history is implemented. Execute
normal active hooks under the resource grant before standing authorized ready
PR delivery. Exact-head CI/full semantic review and a separate ROOT serial
MERGE lease remain external gates documented in the task plan and manifest.

## Files likely touched

- `apps/web/hooks/domains/office/use-provider-health.ts`
- `apps/web/hooks/domains/office/use-provider-health-snapshot-loads.test.tsx` (new)
- `apps/web/hooks/domains/office/office-diagnostic-workspace-reads.test.tsx` (health-specific sibling expectation only)
- `apps/web/app/office/page-client.test.tsx` (stable store API adapter in its existing mock only)
- This work order and `plan.md` for actual results.

Read-only integration inputs: `components/state-provider.tsx`, `lib/state/store.ts`,
`lib/state/slices/office/office-slice.ts`, `routing-types.ts`,
`lib/ws/handlers/office.ts`, `lib/ws/router.ts`, the API client and current
card/banner/page consumers. Existing `office-diagnostic-workspace-reads.test.tsx`
retains its 46 lifecycle cases; its health-specific sibling expectation includes
rows published during that sibling's request, as required by the reviewed
reconciliation contract. Routing-preview expectations stay unchanged. No
dependency, lockfile, action or schema edits.

## Dependencies

None between work orders. PR4241 already landed at the recorded base; its
workspace-selection contract is retained. Later ROOT release plus global
local-heavy lease is required before implementation/tests/install/hooks.
If a deeper dependency is necessary, record concrete evidence in the plan
and checkpoint ROOT, then END WAITING without implementation.

## Mobile and public docs

Shared state/data publication only: no layout, touch, scrolling, navigation,
viewport branch or copy change. Mobile parity permits real-context tests plus
this explicit assessment without new mobile E2E/visual preview. The existing
provider routing guide and WS reference were read; neither specifies this
race. Public configuration, protocol, classification and instructions remain
accurate for this fix. No public page change is required.

## Risks

Full triple identity and actual admission state matter. Whole-list suppression
would drop unaffected hydration; permanently preferring live rows would break
fresh refresh. Sibling writes are conservatively retained without promising
global latest-response ordering. Unknown resource/transport/out-of-scope
failures require ROOT checkpoint with no automatic recovery/retry.

## Parallelism

`sequential`

## Inputs

- [REQ-OFFICE-LIVE-UPDATES-002](../../specs/office/requirements/live-updates.md#req-office-live-updates-002-current-workspace-diagnostic-reads), `.1` through `.10`.
- [Provider-health snapshot design](../../specs/office/system-design/live-updates-02.md#provider-health-snapshot-publication) and prior selection/lifetime section.
- [Manifest](plan.md): actual evidence/source audit, test matrix and exclusions.
- Durable task plan: actual identity, system marker, ROOT authorization/resource
  barriers, joined handles, current ownership and next step.

## Results

ROOT released this reviewed order and the sole global local-heavy lease in
the same primary session. One conditional frozen apps install used explicit
corepack pnpm9.15.9; the host direct pnpm12 was not substituted. Node4GiB and
one Vitest worker were used throughout.

The exact anchored RED command exited 1 with two causal initial/manual
assertions: real degraded WS rows and metadata were overwritten by healthy
HTTP. No timeout/setup/teardown error counted as RED. The first scoped GREEN
passed the new 25-case matrix but exposed one predecessor sibling expectation
inconsistent with the reviewed conservative preservation of any current row
changed during admission. Only that health expectation changed, retaining all
independent consumer assertions and leaving preview expectations intact.

After two additional meaningful same-value-event and same-store/different-
workspace controls, final GREEN passed all 73 tests (27 new plus 46 existing).
Changed three-file ESLint, normal web typecheck, `i18n:check` and the base
ratchet all exited 0. No generated tracked changes or lockfile changes were
introduced. Catalog validation (351 decisions, 1369 specifications), all spec
lint and whitespace passed; actual seven-file documentation coverage returned
`covered`, `ok: true`, and no errors. Exact receipts remain in the task plan.

The production diff is confined to `useProviderHealth`: actual-store baselines,
full-key row identity/presence and one unchanged synchronous setter. Broader
specs stay draft. State-only mobile assessment and public-doc audit apply;
there is no browser/build/E2E or UI/copy/backend/API/action/schema expansion.
All owned handles must be terminal and PIDs gone before the heavy lease is
returned after normal hooks/ready publication. Hosted CI/full semantic review
and a separate ROOT serial merge grant remain pending delivery gates.

Hosted Frontend Tests attempt 1 exposed a fixture contract regression in all
three existing `OfficePageClient boot hydration` tests: their StateProvider
mock omitted `useAppStoreApi`, now read by the real health hook through the
mounted health card. The stack reached the new hook dependency before the
dashboard assertions; this is causal own-scope evidence, not a transient
inference. The frontend leaf and its dependent aggregate failed, while the
27 new health cases and 46 lifecycle cases passed in the same run. No failed
test artifacts were uploaded. Actual logs, run/job IDs, source blobs and
terminal joins are preserved in the task plan and terminal-cause receipt.

ROOT granted a test-only correction: the existing mock now exports
`useAppStoreApi` backed by one stable `getState()` adapter over its original
fixture state. Production and real hook/store/transport tests are unchanged.
The three affected cases passed in one exact anchored run (3/3, 2.88s, exit 0)
with Node 4 GiB, one worker, no file parallelism and the original 120s bound.
Changed-file ESLint exited 0. No new TypeScript types required a typecheck;
no install, passing 73-case replay, browser/build/E2E or CI retry occurred.
Cheap actual-path documentation coverage and normal active hooks precede the
new corrective commit/push; fresh exact-head hosted gates remain mandatory.
Catalog/spec lint and whitespace passed; all eight actual changed paths
returned documentation coverage `covered`, `ok: true`, with no errors.
