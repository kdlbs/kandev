---
id: "01-preserve-journal-freshness"
title: "Preserve freshness in sidebar read journals"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-SIDEBAR-ARCHIVED-FILTER-002
acceptance_criteria:
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.7
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.8
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.16
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.18
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.20
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.23
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.24
system_design:
  - ../../specs/ui/system-design/sidebar-shared-task-state.md
---

# Task 01: Preserve Freshness in Sidebar Read Journals

## Summary

Coalesce repeated in-flight partial task patches with the existing task timestamp
and summary revision rules. Prove that a delayed archived page retains the
newest accepted rename through the actual registered event/store/cache path.
Execute only after ROOT explicitly releases this reviewed package to the same primary.

## In scope

- Freshness-aware accumulation in `recordTaskOverviewChange` using the existing
  merge rules; partial-safe local typing where needed.
- Permanent registered-handler/store/cache transport regression and focused
  controls listed below, without replaying ROOT's original proof.
- Exact targeted checks, document coverage, normal hooks, and delivery evidence
  in the order and authority boundaries of [the plan](plan.md#execution-and-delivery-checkpoint).

## Out of scope

Backend, schemas/APIs, new event ordering abstractions/clocks, membership/source
policy, archive/navigation changes, broad board/settings/runtime work, UI/copy,
page retention policy, full local suites, delegates, sessions, tabs, model changes,
ROOT's resources/proof, and implementation during this design turn.

## Acceptance

1. The independently authored registered-path regression causally fails before
   the change and passes afterward; chronological, HTTP-newer, resident, and
   independent-clock controls pass. No empty selection or transport/setup error
   is accepted as RED.
2. Partial availability, fallback/tie behavior, sticky tombstones, bounded bytes
   and IDs, generation, read/context isolation, and owner lifetime are preserved
   through observable settlement results. Production changes stay in the merge
   module, with immediate type/test glue only when causally necessary.
3. Every listed scoped check passes with retained actual joins; results and
   statuses reflect the exact validated revision. Later delivery satisfies ROOT's
   collector/review/merge/association gates, without bypassing hooks or authority.

## Implementation sequence

1. Direct-read the requirement AC .20, design reconciliation/retention sections,
   this work order, and scoped `apps/web/AGENTS.md`. Load `/tdd`. Read live source
   for the registered handler and immediate cache consumers again after release;
   do not rebase on moving main or rerun ROOT proof.
2. After the single global local-heavy lease, verify the pinned tools and missing
   dependency condition. Install once from `apps/` if still absent. Record the
   original install handle and actual join. A failed install checkpoints ROOT;
   no automatic retry or alternate package manager.
3. Add only the first two permanent registered-path tests described below and
   run the anchored RED selector once. Require the reversed nonresident case
   to fail on its latest-title assertion with chronological positive PASS.
4. Correct repeated journal coalescence with existing merge semantics. Share
   partial-safe internals rather than duplicate freshness logic. Keep resident
   return types/reference reuse; never cast a journal patch into a complete task.
   Preserve the null branch, own-field omission, limits, and byte accounting.
5. Add the remaining meaningful controls; run the full new suite and named
   directly affected files once as GREEN. No automatic passing replay or optional
   polishing. Any corrective finding warrants only the causally affected rerun.
6. Run lint, typecheck, i18n, and cheap docs/actual-path coverage serially. Record
   exact outputs/counts/exits/joins; update work-order and plan results. Mark
   the work order done/plan implemented only after task checks pass. Do not
   promote the wider draft shared-state design merely for this local repair.
7. Follow later delivery gates in the plan only when ROOT releases that phase.

## Permanent test contract

File: `apps/web/lib/sidebar/sidebar-archived-update-freshness.test.ts`.
Describe: `archived sidebar update freshness`. Use these exact test names so
the RED selector below is anchored and unambiguous.

| Test name | Required stimulus and observation |
| --- | --- |
| `preserves newest nonresident archived title across reversed updates and older HTTP` | Real workspace store, absent task and empty active boards; start real cache request with deferred API; dispatch registered archived T2 Latest then T1 Superseded events with explicit archive identity; resolve HTTP T0. Returned archived membership references canonical Latest/T2, response is provisional and not reusable, task stays off active boards. This is the causal RED. |
| `accepts chronological archived updates before older HTTP` | Same nonresident route with T1 then T2 before HTTP T0. Latest/T2 survives; establishes positive ordering control. |
| `accepts newer HTTP task fields after archived events` | Exercise both nonresident and display-owned resident archived fixtures, with T2/T1 and T1/T2 arrival orders followed by HTTP T3; T3 title/time wins. Verify response ID and independent summary selection, without interpreting provisional counts as authoritative. |
| `preserves resident archived title across reversed updates and older HTTP` | Seed through a real display owner, not a raw journal. Registered T2/T1, then old HTTP, preserves T2 and archived membership; release read without losing display-owned record. |
| `merges task timestamps and summary revisions independently` | Older task/newer summary, then newer task/older summary, in both arrival directions and resident/nonresident fixtures during real deferred reads. Title/time and revision each keep their own winner after older and newer HTTP settlement. |
| `preserves summary-only updates and equal revision queue fields` | Registered summary handler interleaves revisions with archived rename events. Lower revisions lose; equal-revision incoming projections in task payload/HTTP accept changed queue fields. No summary-only patch advances task updatedAt. |
| `preserves nanoseconds offsets and timestamp fallbacks` | One-nanosecond distinction within a millisecond, equivalent offset instants, equal timestamps, missing/empty/malformed timestamp controls. Assert the existing merge policy, including last incoming task fields where no strict older comparison is possible. |
| `preserves omitted partial fields and eligible explicit clears` | Newest archived patch retains a field absent from a later patch; valid newer explicit nullable/empty values clear their fields. No task defaults fabricated in the partial journal. Check accepted response values, not helper output alone. |
| `keeps empty and failed transports distinct and releases reads` | Success with zero entries does not invent an unseen task from a partial patch. Transport rejection remains rejection with no reusable page or leaked read after release. Existing hook controls retain recovery behavior. |
| `keeps deletion tombstones through later patches and older HTTP` | Registered delete then update while the same old page is outstanding; deleted row never returns or enters cache. Protection disappears only at settlement/release. |
| `preserves journal ID and byte limits without accumulating stale arrivals` | Exercise 1,000-ID and 1 MiB boundaries with bounded fixtures. Repeated same-ID stale large patches cannot add bytes from rejected fields; coalescence replaces rather than cumulatively charges. Crossing either existing cap increments generation, clears protection and rejects the pending cache result. No orphan ownership/read after release. |
| `isolates journals between reads workspaces accounts and stores` | Two staggered real reads: a pre-second-read event is protected only by the first journal; later events reach both. Another task ID stays independent. Explicit foreign-workspace updates are ignored; workspace/account context changes fence old completion. Separate stores never share patches/pages. Release every handle. |

Only the transport boundary `querySidebarTasks` is mocked. Use real
`registerTasksHandlers`, `createAppStore`, `SidebarTaskPageCache`, and wire payload
mapping, asserting membership and `taskOverview.byId` values consumed by rows.
No production-helper replacement, homemade store, manually injected journal in
the causal regression, fixture that makes both arrivals identical, or invented
delay as a synchronization gate. Arm the deferred transport before events;
settle explicitly and await the original promise. Isolated lower-level boundary
tests can supplement this integration where dispatching 1,001 events would add
unrelated side effects, but must still prove actual read rejection/cleanup.

Always release requests in `finally` and dispose only test-owned timers/state.
Assert the task was absent immediately before nonresident event dispatch.
Keep old/new task and summary clocks deliberately opposed. Use real observed
row values for success, not an assertion that restates the helper expression.

## Verification

Run from repository root, serially, only after ROOT release and the global lease.
Use explicit Node24 PATH and the pinned pnpm9.15.9 entrypoint; never invoke
unpinned Corepack from root. The variables below are task-specific.

```bash
export PATH="/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:$PATH"
export NODE_OPTIONS="--max-old-space-size=4096"
child78_pnpm=/home/jcfs/.cache/node/corepack/v1/pnpm/9.15.9/bin/pnpm.cjs
node --version
node "$child78_pnpm" --version
# Only once, if dependencies remain absent, after the ROOT lease:
if [ ! -d apps/node_modules ]; then
  (cd apps && node "$child78_pnpm" install --frozen-lockfile)
fi
```

Record each original PID/group, UTC deadline, exit, and actual join in owned
evidence. Install deadline 10 minutes; RED 90 seconds; GREEN 3 minutes; scoped
lint 3 minutes; typecheck 10 minutes; i18n 3 minutes each; docs gates 40 seconds
each. Use a retained process group with timeout/kill-after bounds, never drop a
session handle or use a duplicate run to infer its result. Unexpected resource,
timeout, transport, setup, selection, or unknown results checkpoint ROOT.

RED, once before production edits (expected one causal failure, one positive PASS):

```bash
(cd apps/web && node "$child78_pnpm" exec vitest run --project browser-locales --maxWorkers=1 lib/sidebar/sidebar-archived-update-freshness.test.ts --testNamePattern='^archived sidebar update freshness (preserves newest nonresident archived title across reversed updates and older HTTP|accepts chronological archived updates before older HTTP)$')
```

GREEN and existing affected controls, once after implementation:

```bash
(cd apps/web && node "$child78_pnpm" exec vitest run --project browser-locales --maxWorkers=1 lib/sidebar/sidebar-archived-update-freshness.test.ts lib/sidebar/sidebar-page-overviews.test.ts lib/sidebar/sidebar-task-page-cache.test.ts lib/state/slices/task-overview.test.ts lib/state/slices/task-overview-mutations.test.ts lib/ws/handlers/tasks-update-ordering.test.ts lib/task-status-summary.test.ts hooks/domains/kanban/use-sidebar-task-progress.test.tsx)
(cd apps/web && node "$child78_pnpm" exec eslint --max-warnings 0 lib/state/slices/task-overview-merge.ts lib/sidebar/sidebar-archived-update-freshness.test.ts)
(cd apps/web && node "$child78_pnpm" run typecheck)
(cd apps/web && node "$child78_pnpm" run i18n:check)
(cd apps/web && node "$child78_pnpm" run i18n:ratchet)
```

If immediate type/test glue is changed, append those exact changed files to
lint and GREEN selection before the single run; no broad glob or new suite.
The focused files currently belong to `browser-locales`; Node project selection
would omit them. Do not use `--passWithNoTests` or a changed Vitest configuration.
`pnpm run typecheck` includes the repository's required generated-note preparation;
preserve existing generated files and checkpoint any unrelated mutation.

Cheap document gates (also allowed during this design turn):

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs docs/plans/archived-sidebar-update-freshness
git status --short -- docs/specs/ui/requirements/sidebar-task-pagination.md docs/specs/ui/system-design/sidebar-shared-task-state.md docs/plans/archived-sidebar-update-freshness
```

Actual planned production-path documentation coverage preflight, from root:

```bash
node <<'NODE'
const fs = require('node:fs');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const paths = [
  'docs/specs/ui/requirements/sidebar-task-pagination.md',
  'docs/specs/ui/system-design/sidebar-shared-task-state.md',
  'docs/plans/archived-sidebar-update-freshness/plan.md',
  'docs/plans/archived-sidebar-update-freshness/task-01-preserve-journal-freshness.md',
];
const result = validateCoverage({
  changedFiles: [
    { filename: 'apps/web/lib/state/slices/task-overview-merge.ts', status: 'modified' },
    ...paths.map(filename => ({ filename, status: 'modified' })),
  ],
  fileContents: Object.fromEntries(paths.map(path => [path, fs.readFileSync(path, 'utf8')])),
});
console.log(JSON.stringify(result, null, 2));
if (!result.ok || result.status !== 'covered' || result.workOrders.length !== 1 || result.errors.length) process.exit(1);
NODE
```

This supplies the actual proposed runtime path, rather than passing a docs-only
exemption. At implementation replace the changed-file inventory with the actual
diff including immediate glue if any; keep the same four contract documents.
This is documentation traceability, not runtime coverage or fabricated GREEN.

No mobile/browser run is scheduled: this scope qualifies for the mobile-parity
data-only exception and exercises the real shared transport path plus affected
hook controls. No public validator is required because public docs are audited
but unchanged. No full backend, frontend, E2E, benchmark, or fixture suite locally.

## Files likely touched

- `apps/web/lib/state/slices/task-overview-merge.ts` (production owner).
- `apps/web/lib/sidebar/sidebar-archived-update-freshness.test.ts` (new permanent integration).
- `apps/web/lib/state/slices/task-overview-types.ts` only if necessary for partial typing.
- Existing affected tests named above only if needed for a meaningful supporting assertion.
- The existing requirement/design pair and this plan/work order for delivery results.

Read-only immediate consumers include `task-overview.ts`,
`task-overview-normalize.ts`, `task-overview-patch.ts`, registered `tasks.ts`,
`task-overview.ts` / `task-status-summary.ts` handlers, `sidebar-page-overviews.ts`,
`sidebar-task-page-cache.ts`, and the sidebar page/workspace hooks.

## Dependencies

None between work orders. Later ROOT implementation release and single
global local-heavy lease are mandatory. Dependencies were absent at design; the one conditional frozen installation
completed under the later ROOT local-heavy grant.

## Risks

Partial task typing, independent revision clocks, equal-revision summary queue
fields, stale-patch byte charging, tombstone lifetime, and accidental fixture
residency can each make a passing helper test conceal a real transport bug.
Use the concrete registered-path matrix above and retain the original receipts.

## Parallelism

`sequential`. No delegation, recursive tasks, extra sessions or tabs.

## Inputs

- [Requirements](../../specs/ui/requirements/sidebar-task-pagination.md), listed ACs, especially .20.
- [Shared state](../../specs/ui/system-design/sidebar-shared-task-state.md), canonical records, reconciliation, retention, tests.
- [Plan](plan.md), evidence, admitted identity, mobile/docs audit, and delivery gates.
- Existing actual cache/store/registered-handler tests; ROOT proof metadata only.

## Results

Local implementation and task checks completed on 2026-10-08. ROOT reviewed the
four artifacts and released the same primary, first for two-case authoring and
then for the sole local-heavy lease. The causal registered-handler/store/cache
RED failed only on the newest-title assertion; the chronological control passed.
The partial-safe correction is confined to `task-overview-merge.ts`.

The original eight-file run had 74 PASS and two fixture expectation failures.
Both parent-clear assertions were corrected to the existing mapper semantics
and passed their anchored rerun. Scoped lint required test registration extraction;
the resulting full new suite passed 24 tests. Seven unchanged affected suites
had 52 PASS in the original run and were not repeated. Changed eslint, typecheck,
i18n check/ratchet and all specified document gates passed. Actual six-file
coverage returned `covered`, one work order, `errors: []`. See the plan results
for counts, causal corrections and retained original receipt location.

All completed original processes were actually joined with absent groups.
Production/test ownership and foreign resources are preserved. Normal hook,
publication, hosted CI/review, canonical association and merge outcomes remain
external delivery checkpoints in the live primary plan; this work-order status
records local implementation completion, not a claim of merged delivery.
