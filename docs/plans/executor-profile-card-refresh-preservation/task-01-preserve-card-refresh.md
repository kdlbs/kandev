---
id: "01-preserve-card-refresh"
title: "Preserve card refresh publication"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-PROFILE-EDITOR-001
acceptance_criteria:
  - AC-EXECUTORS-PROFILE-EDITOR-001.25
  - AC-EXECUTORS-PROFILE-EDITOR-001.26
  - AC-EXECUTORS-PROFILE-EDITOR-001.27
system_design:
  - ../../specs/executors/system-design/profile-editor.md
---

# Task 01: Preserve card refresh publication

## Summary

Publish a card's accepted profile GET only over current owning store state.
Retain unrelated inventory and target metadata, suppress contested target reads,
and fence older overlapping reads. Implement only after a later explicit ROOT
interrupt to the same primary and an exclusive local-heavy grant.

## In scope

- `ExecutorProfilesCard.refreshProfiles` and local read observation/sequence.
- Independent rendered-card/store/transport/registered-WS regressions with
  actual downstream displayed inventory, plus existing navigation controls.
- Synchronize results in this manifest/order and the existing executor owner pair.

## Out of scope

- Global store actions, API/WS handlers or DTOs, other save/delete/refresh paths,
  server arbitration, retry frameworks, per-profile merge or new dependencies.
- Dialog payloads/markup, provider eligibility, routes, desktop/mobile interaction,
  task picker/launch behavior, broad unchanged suites and whole-app builds.
- ROOT protected proof source, child103/PR4388 and all foreign resources.

## Acceptance

1. Independent causal tests reproduce sibling-update reversion and live executor
   creation loss before correction, with ordinary/failure controls passing and
   no setup/unhandled errors. The corrected card passes the scoped matrix with
   catalogue and downstream displayed choice assertions.
2. The owning design's local observer/current-store publication and read guard
   satisfy AC .25-.27, including target absence/restoration, unrelated clones,
   overlapping failure and cleanup, without changing adjacent writer policies.
3. All listed scoped checks and actual changed-path/one-order references pass;
   record real results/joins, keep package statuses accurate and preserve the
   mobile-parity state/data boundary.

## Implementation sequence

After release, mark this order in progress. Read the paired design section and
source anew, preserving others' changes. Independently author fixtures and real
component regressions; never derive them from ROOT's protected candidate. Hold
external fetch at DELETE/POST/GET and await causal request admission, with no
wall-clock sleeps. Render a subscriber supplying current profiles to the real
card and capture its real provider store. Use `registerExecutorsHandlers` and
`registerExecutorProfileHandlers` for updates, additions and removals.

Render production `useExecutorProfileOptions` labels with the task-create/subtask
projection of missing executor name/type from the current owner. Assert current
values/config/status/membership and eligibility as well as names. Keep all
internal card/dialog/store/API/router/handler/options boundaries real. External
capabilities may be isolated only when absent from the test environment, with
the isolation documented; this cannot replace the repair's causal path.

Run the two causals plus ordinary/failure controls RED before production edits.
Then implement the minimum correction inside the card. Read sequence and store
observation are invocation-local; there is no global owner. Compare ordered
profile entries, not cloned container identity. Keep sticky transitions/absence,
publish only eligible returned target profiles over a synchronous current read,
and unsubscribe in finally. Re-run the scoped matrix GREEN and existing card
navigation controls. Run listed static/documentation checks and normal hooks
only under ROOT's grant. Record actual paths/results/joins before marking done.

## Regression matrix

New suite: `components/settings/executor-profiles-card-refresh.test.tsx`.
Fixtures: `executor-profiles-card-refresh.test-helpers.tsx` in the same directory.
Use describe label `executor card refresh publication`; names below are the
required independently authored cases or table-driven rows.

| Case / test name | Stimulus and expected result | AC |
| --- | --- | --- |
| retains a saved sibling name after pending refresh | Hold GET after real DELETE, deliver real executor.updated, prove renamed visible option before/after GET | .25 |
| retains an executor created during pending refresh | Real executor.created plus profile.created; prove new eligible displayed choice before/after GET | .25 |
| keeps unrelated removals absent in a mixed catalogue | Real executor/profile deletes plus retained updated and created choices; current order/metadata intact | .25 |
| retains target metadata while refreshing its profiles | Real target name/config/status update, ordinary returned target profiles still adopted | .25, .26 |
| publishes an ordinary target refresh | Real DELETE followed by GET, no concurrent target changes; accepted rows/options and metadata | .26 |
| publishes an empty returned profile list | Remove only target profiles, retain unrelated choices | .26 |
| retains the current target list after a live profile transition | Table rows for real target create/update/delete and change-restore; stale GET does not overwrite current list | .26 |
| accepts refresh after unrelated profile deletion clones | Real profile.deleted for another owner clones target containers; uncontested target result still adopted | .26 |
| does not restore a missing target | Real target deletion, initially absent target at GET admission, and delete/reintroduce controls; stale profiles not published | .26 |
| keeps the newer overlapping read result | Start two real deletes/GETs, resolve newer first then older; only newer eligible profiles remain | .27 |
| discards the older read when the newer read fails | Newer GET rejects before older settles; current inventory retained | .27 |
| retains current inventory after GET failure | Real DELETE ACK, later live update/create, GET rejects; no stale publication | .27 |
| does not refresh after failed deletion | Actual DELETE rejects; GET not requested, live state unchanged | .27 |
| navigates after creation refresh settles | Real rendered built-in dialog POST, delayed GET, then real canonical navigation; rejection/skipped-read rows preserve navigation too | .27 |
| isolates separate owning stores | Hold reads in two real providers; each publication/observer stays with its captured store | .25-.27 |

Before each held GET is released, prove transport admission and rendered live
inventory. After settlement, assert real store and displayed rows/options, unique
membership for the returned list fixture, removal absence, metadata and eligibility.
Do not claim duplicate-event remediation. Settle/reject every deferred response,
unmount, restore fetch/history/navigation guards and drain owned work on every
exit. No detached promise, subscription or timer may remain after a case.

## Mobile and public documentation

This is state/data normalization with unchanged markup, layout, copy, touch,
scrolling, navigation and breakpoint interaction. Mobile-parity explicitly
permits focused real component tests here; no new mobile Playwright, UI sketch
or frontend build. Reassess before any interaction/geometry change. Existing
executor guide, README and screenshots describe unchanged user steps. Internal
requirement/design/plan/order updates suffice; no new ADR or public instructions.

## Verification

Run from repository root. After explicit release, a fresh worktree missing its
own dependencies requires `(cd apps && pnpm install --frozen-lockfile)` once
under ROOT's exclusive lease. Never alter or borrow another worktree's deps.
Retain every actual command handle and join it; do not duplicate a running check.

```bash
(cd apps/web && timeout --signal=TERM --kill-after=10s 3m pnpm exec vitest run --project=browser-locales --maxWorkers=1 --no-file-parallelism components/settings/executor-profiles-card-refresh.test.tsx -t 'retains a saved sibling name|retains an executor created|publishes an ordinary target refresh|retains current inventory after GET failure')
(cd apps/web && timeout --signal=TERM --kill-after=10s 5m pnpm exec vitest run --project=browser-locales --maxWorkers=1 --no-file-parallelism components/settings/executor-profiles-card-refresh.test.tsx components/settings/executor-profiles-card.test.tsx)
(cd apps/web && timeout --signal=TERM --kill-after=10s 3m pnpm exec eslint --max-warnings 0 components/settings/executor-profiles-card.tsx components/settings/executor-profiles-card-refresh.test.tsx components/settings/executor-profiles-card-refresh.test-helpers.tsx hooks/domains/settings/use-refresh-executor-profiles.ts)
(cd apps/web && timeout --signal=TERM --kill-after=10s 10m pnpm run typecheck)
(cd apps/web && timeout --signal=TERM --kill-after=10s 3m pnpm run i18n:check)
(cd apps/web && timeout --signal=TERM --kill-after=10s 3m pnpm run i18n:ratchet)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short -- docs/plans/executor-profile-card-refresh-preservation
```

The first Vitest command is the RED selection before correction; the full run
is GREEN after correction, without rerunning the passing selection separately.
Typecheck and i18n are static contract gates, not whole-app builds or broad
unchanged test replay. New failures or edits justify affected rechecks only.

Documentation preflight below uses actual tracked AND untracked paths, all four
artifacts, and independently checks exactly one order and its references even
when coverage classifies the docs-only checkpoint as exempt. After implementation
require covered status. Run under ROOT's appropriate lease, from repository root:

```bash
timeout --signal=TERM --kill-after=10s 60s node <<'NODE'
const fs = require('node:fs');
const cp = require('node:child_process');
const path = require('node:path').posix;
const { validateCoverage, parseFrontmatter } = require('./.github/scripts/pr-docs.cjs');
const base = '1f73bee3eb69623396e821279633eaa2ef54d7fb';
const tracked = cp.execFileSync('git', ['diff', '--name-only', '-z', base], { encoding: 'utf8' }).split('\0');
const untracked = cp.execFileSync('git', ['ls-files', '--others', '--exclude-standard', '-z'], { encoding: 'utf8' }).split('\0');
const paths = [...new Set([...tracked, ...untracked].filter(Boolean))].sort();
const dir = 'docs/plans/executor-profile-card-refresh-preservation';
const req = 'docs/specs/executors/requirements/profile-editor.md';
const design = 'docs/specs/executors/system-design/profile-editor.md';
const plan = `${dir}/plan.md`;
const order = `${dir}/task-01-preserve-card-refresh.md`;
const docs = [req, design, plan, order];
const fileContents = Object.fromEntries(docs.map(p => [p, fs.readFileSync(p, 'utf8')]));
const changedFiles = paths.map(filename => ({ filename, status: fs.existsSync(filename) ? 'modified' : 'removed' }));
const coverage = validateCoverage({ changedFiles, fileContents });
const errors = [];
const orders = fs.readdirSync(dir).filter(p => /^task-\d{2}-.*\.md$/.test(p));
if (orders.length !== 1 || orders[0] !== path.basename(order)) errors.push('Expected exactly one work order');
for (const doc of docs) if (!paths.includes(doc)) errors.push(`Artifact absent from actual changed paths: ${doc}`);
const wo = parseFrontmatter(fileContents[order]).data;
const manifest = parseFrontmatter(fileContents[plan]).data;
const sd = parseFrontmatter(fileContents[design]).data;
const resolve = (from, ref) => path.normalize(path.join(path.dirname(from), ref));
if (resolve(order, wo.plan) !== plan) errors.push('Incorrect plan reference');
for (const ref of wo.system_design) {
  if (resolve(order, ref) !== design || !manifest.system_design.some(p => resolve(plan, p) === design)) errors.push('Incorrect design reference');
}
for (const id of wo.requirements) {
  if (!sd.requirements.includes(id) || !manifest.requirements.includes(id) || !fileContents[req].includes(`### ${id}:`)) errors.push(`Unresolved requirement ${id}`);
}
for (const id of wo.acceptance_criteria) {
  if (!wo.requirements.some(r => id.startsWith(r.replace(/^REQ-/, 'AC-') + '.')) || !fileContents[req].includes(`**${id}:**`)) errors.push(`Unresolved criterion ${id}`);
}
const owned = [...docs, 'apps/web/components/settings/executor-profiles-card.tsx', 'apps/web/hooks/domains/settings/use-refresh-executor-profiles.ts', 'apps/web/components/settings/executor-profiles-card-refresh.test.tsx', 'apps/web/components/settings/executor-profiles-card-refresh.test-helpers.tsx'];
for (const p of paths) if (!owned.includes(p)) errors.push(`Unexpected changed path; preserve and report: ${p}`);
const docsOnly = paths.every(p => docs.includes(p));
if (!coverage.ok || coverage.errors.length || coverage.status !== (docsOnly ? 'exempt' : 'covered')) errors.push('Coverage failed');
if (!docsOnly && coverage.workOrders.length !== 1) errors.push('Implementation coverage requires one work order');
process.stdout.write(JSON.stringify({ paths, docs, orders, coverage, errors }, null, 2) + '\n');
if (errors.length) process.exitCode = 1;
NODE
```

## Files likely touched

- `apps/web/components/settings/executor-profiles-card.tsx`
- `apps/web/hooks/domains/settings/use-refresh-executor-profiles.ts`
- `apps/web/components/settings/executor-profiles-card-refresh.test.tsx`
- `apps/web/components/settings/executor-profiles-card-refresh.test-helpers.tsx`
- `docs/specs/executors/requirements/profile-editor.md`
- `docs/specs/executors/system-design/profile-editor.md`
- `docs/plans/executor-profile-card-refresh-preservation/plan.md`
- `docs/plans/executor-profile-card-refresh-preservation/task-01-preserve-card-refresh.md`

Read-only integration dependencies are the two card callers, API transport,
real state provider/store, registered WS handlers, dialogs/router and task
options hook. No predecessor work order. Existing editor/create/policy/connection
packages retain their scopes and recorded results; do not replay them.

## Risks

- Using the pre-request closure again, or adding an await between current read
  and write, would reopen catalogue loss.
- Comparing array identity would suppress valid unrelated-clone refresh;
  comparing only final state would miss observed restoration and absence.
- A contested whole-list skip intentionally does not merge response-only target
  changes. Unobserved server ordering remains outside this repair.
- Real dialogs invoke onSaved without awaiting; transport cleanup and navigation
  assertions must settle owned asynchronous work without changing this contract.

## Results

Implemented after ROOT's later explicit release in the same primary under the
exclusive local-heavy lease. Production changed only the card's local refresh
publication. Independent regressions kept the rendered card/dialog/router,
provider/store, registered handlers, API adapters and displayed options real.
The protected ROOT proof source was not accessed.

| Required command / check | Actual result and original join |
| --- | --- |
| Own-worktree `pnpm install --frozen-lockfile` from apps | Handle 29427, exit 0, join 12de89; 935 reused, zero downloads |
| RED four-case Vitest selection above | Handle 61910, exit 1, join 782da1; two causal failed/two controls passed, no setup/unhandled errors |
| Expanded baseline Vitest matrix before correction | Handle 43702, exit 1, join d4bbdd; 18 failed/6 passed; three wrong-capitalization creation rows excluded from causal evidence and corrected |
| GREEN exact two-file Vitest command above | Final handle 11157, exit 0, join 11a564; 24 passed across two suites; no stderr/unhandled errors |
| Scoped ESLint command above | Final handle 11157, exit 0, join 11a564; zero warnings/errors |
| `pnpm run typecheck` with its normal generation prerequisite | Final handle 6813, exit 0, join 212d76 |
| `pnpm run i18n:check` and `pnpm run i18n:ratchet` | Handle 79484, exit 0, join 488005; each command exit 0 |
| Catalog/spec/whitespace/actual tracked+untracked preflight above | Handle 9698, exit 0, join 293ae8; covered, seven actual paths, one order, zero errors |

Initial ESLint attempts found only test registration-length/repeated-literal
warnings; split registrations and fixture constants without changing assertions.
Initial typecheck handle 44848 exited 2 (join 59baaa) for new test-helper generic
envelope typing and incomplete executor deletion fixtures. Corrected the fixtures
to real notification/action/payload envelopes and complete executor payloads;
final unit/lint/typecheck runs cover those changes. Production never changed
after its successful GREEN run. Generated prerequisite output remained ignored
and did not enter the seven-path inventory.

No backend, broad unchanged suites, Playwright, build, global WS/API/store or
caller edits were made. Mobile-parity's documented state/data exception applies.
All deferred transport work settled, providers unmounted, globals/history
restored; no detached process or subscription remains from local checks.
Logs and coverage receipts are retained outside Git under
`/tmp/kandev-child104-card-refresh-design-20261010/`. Normal hooks/publication
and exact-head hosted delivery remain external operational gates.


### Required hook-placement correction

ROOT authorized a behavior-preserving extraction for the frontend's required
hook location. `useRefreshProfiles` and `observeProfiles` moved into
`apps/web/hooks/domains/settings/use-refresh-executor-profiles.ts`, with only
the hook export added; a byte comparison confirmed the extracted body unchanged.
The card imports it. No regression assertions, interactions or dependencies changed.

- Actual affected test command: `pnpm exec vitest run components/settings/executor-profiles-card-refresh.test.tsx components/settings/executor-profiles-card.test.tsx`, from apps/web under a three-minute timeout. Handle 54099, exit 0, join 5b77a3: 24 passed, no setup/unhandled errors.
- Scoped ESLint command above, including the new hook: same handle/join, exit 0, zero warnings/errors.
- Typecheck command above: handle 62116, exit 0, join fe6b29.
- Catalog/spec/whitespace and actual coverage preflight: handle 77354, exit 0, join 851ed4; eight actual paths covered by this one work order, no errors.

No RED replay or broad unchanged suite was needed for the unchanged hook body.
Normal hooks and one corrective push remain external operational gates.
