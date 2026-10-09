---
id: "01-preserve-policy-catalogue"
title: "Publish policy acknowledgements over the current catalogue"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-PROFILE-EDITOR-001
acceptance_criteria:
  - AC-EXECUTORS-PROFILE-EDITOR-001.19
  - AC-EXECUTORS-PROFILE-EDITOR-001.20
system_design:
  - ../../specs/executors/system-design/profile-editor.md
---

# Task 01: Publish policy acknowledgements over the current catalogue

## Summary

Independently prove the executor policy form's captured catalogue overwrite,
then publish successful acknowledgements over its current owning AppStore.
Verify accepted policy/draft controls together with the real live-event and
task-start choices path.

## Execution barrier

ROOT released implementation on 2026-10-09 after actual design END/WFI and
review of all four artifact hashes. Exclusive global local-heavy lease belongs
to CHILD98. GREEN runs only the new page suite: it exercises the real
coordinator/options directly, so unchanged suites are not replayed.
Hosted collection and merge still require their later separate grants.

### Historical design barrier

DESIGN ONLY now. No production/permanent tests, dependency install, product
checks, commit or PR until a LATER explicit ROOT implementation interrupt to
task `fa9bcee2-04fc-49ba-b49d-a1bfb0d19282`, same primary session
`6707a8f8-c778-46d4-b068-ccbfbc00591d`, and one global serial local-heavy lease.
Same primary/model/profile; no agents, delegates, recursive tasks or new
sessions/tabs/model changes. End the design turn at handoff, with all four
artifacts unstaged/uncommitted; no approval/model-switch prompt.

Read only the three ROOT metadata/log receipts named in the manifest. Accept
original15953/89ca90->7eb109 joined exit1/two causal failures/one control PASS;
initial58444 is unqualified. NEVER access ROOT's protected `candidate.test.tsx`
in any way. Its mode/hash and original-process evidence remain in the manifest
and task plan. Author every permanent regression independently after release.

## In scope

- Exact policy `handleSave` publication in `ExecutorEditForm`.
- Independent real rendered page/component and task-start view-model regressions.
- Existing owning pair, manifest/work-order lifecycle and command evidence.

## Out of scope

- Backend/API/schema, global AppStore/action types, WS handler/transport changes.
- `ExecutorProfilesCard` refresh and `DeleteExecutorSection`, other settings
  saves, navigation/lifecycle and concurrency arbitration.
- Layout/copy/breakpoints, browser/build/E2E, dependency/config/lockfile changes,
  new framework/helper abstractions or generic migration.

## Acceptance

1. Independently authored deferred-transport causal cases fail for catalogue
   and actual-options loss before the correction, while ordinary controls pass
   and setup/unhandled errors remain zero. All planned .19/.20 cases pass after
   the bounded owning-store correction.
2. Only the policy form changes production behavior: accepted response merges
   into the matching current item without an intervening await or missing-owner
   insertion. Payload, saved baseline, draft, dirty, validation and failed-save
   behavior retain their owners. No adjacent production expansion is assumed.
3. Exact task checks and actual changed-path coverage pass with joined original
   receipts; docs/statuses truthfully distinguish this work from prior packages.
   Ready publication returns the heavy lease before any later hosted grant.

## Implementation sequence

1. After explicit ROOT release, re-read this packet and scoped guidance; record
   current branch/base/source identities and compare relevant source against
   qualified baseline `b8b740f128993b804745704591374f5a9fc7b0ab`. Reconcile a moved
   base with ROOT; do not silently main-only rebase or synthesize merged tests.
   Set this work order `in_progress` only then. Read `/tdd` before code changes.
2. Qualify actual runtime from `apps/`: existing Node24 prefix, `/bin/bash`
   login:false. Dependencies are absent at design inspection. If still absent,
   one pinned frozen install under the lease; no substitute runtime/install,
   retries or cache wipe. Retain original handles and startup/PID/groups.
3. Author `executor-policy-catalogue-publication.test.tsx` independently. Mount
   real `ExecutorEditPage`, `StateProvider` and real `SettingsSaveProvider`;
   obtain each store through a probe's actual `useAppStoreApi`. Use real form
   edits and coordinator `saveAll` or actual save controls, not private callbacks.
   Observe real `useExecutorProfileOptions` with production fallback metadata.
   Keep page/form/contributor/store/actions/router/handlers/options real.
   Control only external `fetch` or WS request and external Monaco renderer;
   preserve loader exports and use real `setWebSocketClient` for branch selection.
4. Run the unchanged success/failure controls first. Assert actual PATCH or WS
   action/payload, system name omission/non-system name, accepted own config,
   profiles omitted from response, clean matching draft or failed dirty draft.
   Keep REST fallback adapter real; no mock of `updateExecutorAction`.
5. Add causal cases while transport remains held: registered created/updated/
   deleted events on a DIFFERENT owner, whole-owner insertion/removal using real
   current store publication, and mixed catalogue containing both live and
   removed rows. Prove events/choices are applied before release. Assert both
   exact catalogue and actual usable option values/labels/type/name/eligibility
   after accepted acknowledgement. Before production change, run the causal
   group once and retain genuine assertion RED with no setup/unhandled errors.
6. Implement only `useAppStoreApi` and the current-array acknowledgement map in
   policy `handleSave`; retain `{ ...item, ...updated }` and saved-policy baseline.
   Remove only that form's captured `executors.items` input. Do not change the
   parent subscription or delete/card writers. No await between read/publication.
7. Finish faithful coverage for saved-owner current profile membership, removed
   saved-owner absence, mixed owners, failure after live updates, accepted
   normalized policy baseline/discard with raw draft unchanged, captured submission
   versus newer draft, and independent concurrently mounted live providers using
   equal executor IDs. Verify only the originating store receives its response;
   other-store live choices remain intact. Exercise both transport branches with
   their existing response/payload semantics; do not claim backend concurrency.
8. Unmount all providers and settle held transport/coordinator promises in cleanup
   on every outcome. Restore active connection/history/navigation guards and drain
   OWN timers; no foreign resources. Run exact GREEN and task checks once. Any
   resource/timeout/unknown/out-of-scope failure checkpoints ROOT before alternatives.
9. Record actual command/raw receipt paths/hashes, joins and own groups gone;
   mark work order done/manifest implemented only after all relevant checks pass.
   Keep broad owner active/current and earlier package history truthful. Follow
   later commit/push/PR skills and normal hooks only under ROOT's release.

## Test organization and phone boundary

Use describes `executor policy publication controls` and
`executor policy catalogue preservation`; planned case names and AC mapping
are in the [manifest](plan.md#tests). Split fixture mechanics to the optional
same-directory helper only if needed for file/function limits. Do not copy the
protected source or use store-action-only tests in place of the real page.

Pure publication changes qualify for the mobile-parity state/data exception:
real rendered component, live registered events and actual task-start view-model
verification cover shared phone/desktop semantics. No layout/touch/scroll/
navigation/breakpoint behavior changes, UI sketch or browser build is needed.

## Verification

All commands below are DEFERRED product checks until ROOT release and heavy
lease. Execute sequentially from repo root through `/bin/bash` login:false.
Each original tool/native handle and subprocess must actually join, with saved
argv/cwd/env/bounds/startup/PID/PGID/raw logs/exits/hashes and OWN groups gone.
Use GNU timeout's displayed bounds and 10s kill grace. A timeout is a checkpoint,
not permission to repeat or switch runtime. No overlapping suites or broad replay.

Runtime qualification and conditional one-time install (apps cwd):

```bash
export PATH=/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:$PATH
(cd apps && command -v node && node --version && node -p 'process.execPath' && command -v corepack && timeout --signal=TERM --kill-after=10s 60s corepack pnpm@9.15.9 --version)
if [ ! -d apps/node_modules ]; then
  (cd apps && timeout --signal=TERM --kill-after=10s 10m env NODE_OPTIONS=--max-old-space-size=4096 corepack pnpm@9.15.9 install --frozen-lockfile)
fi
```

Control then causal RED (before production change); exact file and test group:

```bash
(cd apps/web && timeout --signal=TERM --kill-after=10s 3m env NODE_OPTIONS=--max-old-space-size=4096 corepack pnpm@9.15.9 exec vitest run 'app/settings/executor/[id]/executor-policy-catalogue-publication.test.tsx' --maxWorkers=1 --no-file-parallelism -t '^executor policy publication controls ')
(cd apps/web && timeout --signal=TERM --kill-after=10s 3m env NODE_OPTIONS=--max-old-space-size=4096 corepack pnpm@9.15.9 exec vitest run 'app/settings/executor/[id]/executor-policy-catalogue-publication.test.tsx' --maxWorkers=1 --no-file-parallelism -t '^executor policy catalogue preservation ')
```

GREEN (new actual-page suite only), changed-file lint, typecheck and i18n:

```bash
(cd apps/web && timeout --signal=TERM --kill-after=10s 5m env NODE_OPTIONS=--max-old-space-size=4096 corepack pnpm@9.15.9 exec vitest run 'app/settings/executor/[id]/executor-policy-catalogue-publication.test.tsx' --maxWorkers=1 --no-file-parallelism)
(cd apps/web && timeout --signal=TERM --kill-after=10s 3m env NODE_OPTIONS=--max-old-space-size=4096 corepack pnpm@9.15.9 exec eslint --max-warnings 0 'app/settings/executor/[id]/page.tsx' 'app/settings/executor/[id]/executor-policy-catalogue-publication.test.tsx' 'app/settings/executor/[id]/executor-policy-catalogue-publication.test-helpers.tsx')
(cd apps/web && timeout --signal=TERM --kill-after=10s 5m env NODE_OPTIONS=--max-old-space-size=4096 corepack pnpm@9.15.9 run typecheck)
(cd apps/web && timeout --signal=TERM --kill-after=10s 3m env NODE_OPTIONS=--max-old-space-size=4096 corepack pnpm@9.15.9 run i18n:check)
(cd apps/web && timeout --signal=TERM --kill-after=10s 3m env NODE_OPTIONS=--max-old-space-size=4096 corepack pnpm@9.15.9 run i18n:ratchet --base b8b740f128993b804745704591374f5a9fc7b0ab)
```

The fixture helper was created and is included explicitly in changed-file
ESLint. The page suite imports it, covering every changed test file. Reconcile the ratchet base to ROOT's released base if it changes.
Formatting remains subject to normal hooks; no bypass or amend.

Docs-only gates permitted at design time (repo root, 60s per command):

```bash
timeout --signal=TERM --kill-after=10s 60s python3 scripts/list-docs.py validate
timeout --signal=TERM --kill-after=10s 60s python3 scripts/lint-spec-files.test.py
timeout --signal=TERM --kill-after=10s 60s python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs docs/plans/executor-policy-catalogue-preservation
git status --short -- docs/specs/executors/requirements/profile-editor.md docs/specs/executors/system-design/profile-editor.md docs/plans/executor-policy-catalogue-preservation
```

Actual changed-path coverage (repo root; 60s, existing Node24, no product build).
Run in design and implementation; design `exempt` is never implementation proof:

```bash
export PATH=/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:$PATH
timeout --signal=TERM --kill-after=10s 60s env NODE_OPTIONS=--max-old-space-size=4096 node <<'NODE'
const fs = require('node:fs');
const cp = require('node:child_process');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const base = 'b8b740f128993b804745704591374f5a9fc7b0ab';
const paths = [...new Set([
  ...cp.execFileSync('git', ['diff', '--name-only', '-z', base], {encoding: 'utf8'}).split('\0'),
  ...cp.execFileSync('git', ['ls-files', '--others', '--exclude-standard', '-z'], {encoding: 'utf8'}).split('\0'),
].filter(Boolean))];
const docs = [
  'docs/specs/executors/requirements/profile-editor.md',
  'docs/specs/executors/system-design/profile-editor.md',
  'docs/plans/executor-policy-catalogue-preservation/plan.md',
  'docs/plans/executor-policy-catalogue-preservation/task-01-preserve-policy-catalogue.md',
];
const fileContents = Object.fromEntries(docs.map(p => [p, fs.readFileSync(p, 'utf8')]));
const changedFiles = paths.map(filename => ({filename, status: fs.existsSync(filename) ? 'modified' : 'removed'}));
const result = validateCoverage({changedFiles, fileContents});
process.stdout.write(JSON.stringify(result, null, 2) + '\n');
if (!result.ok || result.errors.length || (result.requiresCoverage && (result.status !== 'covered' || result.workOrders.length !== 1))) process.exitCode = 1;
NODE
```

For design reference validation only, feed the four actual document paths plus
the single planned production page to `validateCoverage`; label this separate
result planned-trigger `covered`, never actual implementation evidence. Confirm
all ACs exist and belong to the work-order requirement and paired design. Repeat
actual changed-path coverage after implementation, requiring `covered`, errors[]
and one changed work order. Do not manufacture production files during design.

## Files likely touched

- `apps/web/app/settings/executor/[id]/page.tsx` (policy form only).
- `apps/web/app/settings/executor/[id]/executor-policy-catalogue-publication.test.tsx` (new, after release).
- `apps/web/app/settings/executor/[id]/executor-policy-catalogue-publication.test-helpers.tsx` (optional fixture mechanics).
- `docs/specs/executors/requirements/profile-editor.md`.
- `docs/specs/executors/system-design/profile-editor.md`.
- This manifest and work order.

## Dependencies

No other implementation work order. ROOT later release and serial heavy lease
are mandatory. Current owner/event/consumer contracts already exist.

## Risks

Normalized accepted baseline may differ from raw draft; preserve existing dirty
comparison. The saved executor's returned metadata/config can overwrite a
concurrent same-owner change; no broader arbitration is claimed. The card's
separate refresh writer is excluded, not represented as repaired.

## Parallelism

`sequential`. No delegation authorized.

## Inputs

- Owning .19/.20 and design section linked in this packet.
- `apps/web/AGENTS.md`, `/tdd`, `/mobile-parity`, `/docs-maintainer`.
- Real page/provider/coordinator/connection/action/handler/options code.
- Existing permanent creation-publication tests as a pattern only, not ROOT's
  protected fixture; earlier companion plan history remains untouched.

## Results

### Historical design checkpoint

At the 2026-10-09 design checkpoint, catalog validation,
36 spec-linter tests, full spec lint and diff check passed. Actual four-path
docs-only coverage was `exempt`, errors[]; planned-trigger references were
`covered`, errors[], one work order. This is not product GREEN evidence.
Original native18620/b7a52b->17d9ff actually joined exit0; six original
subprocesses joined exit0 and exact OWN groups plus wrapper were observed gone.
Raw receipts/logs/hashes and completion proof are retained under
`/tmp/kandev-child98-policy-design-20261009/`. No install or local-heavy lease
was acquired, and none is held. Four artifacts remain unstaged/uncommitted.

No permanent tests or production edits performed at design handoff.
Later execution and hosted/merge authority obey the manifest and durable task
plan: explicit heavy return, later hosted release, then separate serial MERGE
grant. Preserve raw receipts and managed resources for ROOT archival.

### Implemented result (2026-10-09)

ROOT's later release assigned the exclusive heavy lease to this same primary.
The independent permanent suite exercised the real page, owning providers,
coordinator, REST action/WS selection, registered profile events and actual
options. Ten controls passed first. Before the correction, 13 causal cases
failed on catalogue/options loss, with two failure controls passing and no
unhandled/setup errors. Only the policy form's catalogue read changed.

After the final fixture grouping/typing changes, all 25 page cases passed.
Scoped lint initially identified only new-test size/duplicate warnings; grouping
and constants resolved them. The initial typecheck identified only the own
fixture's union-handler TS2345; discriminant narrowing resolved it. No rule,
assertion, fixture contract or production scope was weakened.

| Command evidence | Original native / chunks | Raw result |
| --- | --- | --- |
| Runtime qualification | inline / b62b8c | Node v24.21.0, pnpm9.15.9, exit0 |
| ONE frozen install | 8866 / a8eff4 -> d87466 | exit0, 935 reused, zero downloaded |
| Controls | 24062 / e215d1 -> e3c852 | exit0, 10 PASS |
| Causal RED | 4838 / c1bfcb -> 0f4f8b | exit1, 13 failed, 2 PASS, 10 skipped |
| Initial GREEN | 30696 / e47b98 -> 1cd51c | exit0, 25 PASS |
| Final changed suite | 18380 / 8ccb0e -> 000b9e | exit0, 25 PASS |
| Final scoped lint (page + both test files) | 60829 / 0a3019 -> 7c893a | exit0 |
| Final typecheck | 68151 / fa46a0 -> faf7bd | exit0 |
| i18n:check | 63879 / 825291 -> 971a5f | exit0 |
| i18n:ratchet at baseline b8b740f1 | 8745 / 8b08e1 -> 98e221 | exit0 |
| Catalog | inline / 23431e | exit0, 368 decisions/1493 specs |
| Spec-linter tests | inline / 797bac | exit0, 36 PASS |
| Full spec lint | inline / 6700b9 | exit0 |
| Actual changed-path coverage | inline / fb4598 | exit0, 7 actual paths, covered, errors[], 1 work order |

Full original argv/cwd/env/bounds, subprocess PIDs/groups, joins, raw outputs,
exits and hashes are retained under
`/tmp/kandev-child98-policy-implementation-20261009/`. `native-joins.json`
reconciles the original tool handles, including known failed lint/type attempts
and formatting. All completed subprocesses actually joined; their exact own
groups were gone at join. Final fresh cleanup reconciliation is required before
heavy return. Normal hooks/publication follow without product reruns or scope
expansion. Hosted collection and merge remain separately gated.

## Later targeted CI remediation (2026-10-09, implemented)

ROOT released the same primary with an exclusive serial heavy lease after the
original implementation and diagnostic handoffs. Actual current-head Shard11
failed at the Pierre visible-anchor readiness assertion with NULL before refresh.
ROOT independently qualified the matching shared-helper cause, permanent RED and
Pierre/Monaco/mobile GREEN at source candidate
`cd8ffc66a937e33e6fec1eab9e9e8c53129a33cf`. Those original causal receipts are
accepted without replay: `/tmp/kandev-root-child97-causal-helper-checks-qualified-20261009.json`.

This later exception to the original browser/E2E exclusion admits exactly
`apps/web/e2e/tests/git/git-refresh-continuity-helpers.ts`,
`apps/web/e2e/tests/git/diff-refresh-continuity.spec.ts`, and
`apps/web/e2e/tests/git/git-continuity-preview-history.ts`. The readiness poll
aligns the selected section and scroll root using their common viewport
rectangles on each existing attempt. A private three-commit preview history
establishes the 14-file regression and restores its original private repository
head in the existing cleanup. Existing timeout, refresh assertions, strict WS
accounting and fail-on-flaky policy remain intact. This changes only E2E setup;
the executor-policy production fix, owning requirement/design and page tests
retain their exact previously verified bytes. No new plan or specification owner.

Transfer is only the ROOT-reviewed patch with SHA256
`a8e7e332c3214fdb8b1835df4db5489769512b21a5449a2f7a4a9fe41d591f22`,
manifest `/tmp/kandev-root-shared-pierre-correction-20261009/manifest.json`;
both original changed code blobs match this candidate exactly. One new helper
was absent. No sibling worktree/protected proof or source build artifact reused.

Only ONE fresh candidate desktop Pierre continuity GREEN is admitted, with
managed fresh backend/Vite/plugin builds, normal guards, one shard/worker,
retries0/repeat1, strict fail-on-flaky, GNU15m/kill10, GOMAXPROCS2,
GOFLAGS=-p=2, GOMEMLIMIT512MiB and Node4GiB. Exact selection is
`chromium`, `tests/git/diff-refresh-continuity.spec.ts`, anchored grep
`desktop Git diff refresh continuity retains counts and reading state with pierre-diffs$`.
Changed-three-file ESLint/format, docs validation and actual ten-path coverage
plus discovery and normal commit hooks precede the authorized fixup commit/push.
No broad page/type/i18n/backend-lint replay or expanded E2E typecheck is admitted.

The prepared mobile observation remains unapplied/unrun. The distinct Shard5
mobile setup/follow cause is unproved; its assertion/fixture/production behavior
is unchanged. A fresh automatic first CI attempt on the substantive helper
correction is distinct from a manual hosted retry, whose budget remains unspent
and unauthorized. No CI collector, manual review request or merge during this
local turn; later hosted and serial merge releases remain mandatory.

Original raw failed checks remain truthful. Retain every original process/native
join/argv/cwd/env/bound/raw hash and fresh exact own process/group absence under
`/tmp/kandev-child98-ci-remediation-20261009/`. Resource, timeout, unknown or
out-of-scope failures checkpoint ROOT before alternatives. Publish one frozen
candidate, preserve canonical association/FIVE flags FALSE/unchecked author body
and bot additions, return heavy explicitly, then END/WFI. No foreign cleanup.

### Own candidate remediation result

The reviewed three-file transfer matched its admitted patch SHA and all new
code blobs. On this candidate, guarded discovery found exactly one intended
desktop Pierre case. The ONE original managed GREEN rebuilt this worktree's
backend, Vite assets and fixture plugin from its current source, then passed
one test in 18.8s, with zero retries/failed/flaky/skipped results. Normal guards
reported host/shard1/chromium/strict1/workers1; no source build reused or freshness
bypass. The original native90356/b99060 -> 1694af actually joined exit0; all
148 observed exact own descendant identities were absent at its join. Five
fresh artifact hashes/mtimes and exact transferred blobs are retained in
`green-qualified.json`, raw managed log SHA256
`738a175f460cafdda886ccea6bf4c34eab1546ea666e0dfcf39003923ecdb8e2`.

Changed-three-file Prettier and ESLint passed (original29582/96cb46 -> 86216b;
4947/372b1e -> f774f6). Catalog validation passed (80772/449f57 -> b65bb2),
as did full spec lint (inline117b7c). The original docs-spec-lint returned
inline exit0 before a checkpoint writer rejected an absent optional session
field; its exact result was retained before that writer error and reconciled
without replay. There was no product/check failure or unknown live process.
Final diff/actual ten-path coverage and normal hooks/publication retain their
separate original receipts in the same evidence directory.

The original page fix, its two permanent test files and owning requirement/design
remain byte-identical to the earlier frozen head. No public documentation
change: the correction affects private E2E setup/readiness only, with no new
user workflow, copy, configuration, API or executor behavior. The shared helper's
Monaco/mobile controls were already causally qualified by ROOT and are not
duplicated. The distinct mobile-last-prompt observation remains unapplied/unrun;
its cause and eventual new-head CI outcome are not represented as resolved.
No passing product/type/i18n/backend suites replayed. Historical failed checks
and earlier barriers remain retained; same work order, no new plan. The task is
not complete merely because this local remediation is implemented: later ROOT
hosted and serial merge grants remain mandatory after explicit heavy return.
