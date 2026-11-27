---
id: "01-scope-status-results"
title: "Keep GitHub browse status results in their requested batch"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITHUB-BROWSE-STATUS-001
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-BROWSE-STATUS-001.1
  - AC-INTEGRATIONS-GITHUB-BROWSE-STATUS-001.2
  - AC-INTEGRATIONS-GITHUB-BROWSE-STATUS-001.3
  - AC-INTEGRATIONS-GITHUB-BROWSE-STATUS-001.4
  - AC-INTEGRATIONS-GITHUB-BROWSE-STATUS-001.5
  - AC-INTEGRATIONS-GITHUB-BROWSE-STATUS-001.6
  - AC-INTEGRATIONS-GITHUB-BROWSE-STATUS-001.7
system_design:
  - ../../specs/integrations/system-design/github-browse-status-batches.md
---

# Task 01: Scope Status Results

## Summary

Stamp the existing hook's local result with its producer batch key and gate its
return on the current requested key. Prove the immediate real-list badge outcome
and preserve scheduling, cancellation, equal-content reuse and failure behavior.
ROOT reviewed the full design package and granted this sequential implementation
in the same primary session. Hosted review and merge remain external gates.

## In scope

- One production hook correction and the real regression matrix below.
- New `use-pr-statuses.scope.test.tsx`, including actual `PRList` integration.
- Existing key/hook, launch and immediate badge controls; adjust an existing
  fixture only when the changed local snapshot requires it.
- Task-defined bounded checks and truthful lifecycle/results updates.

## Out of scope

The [plan exclusions](plan.md#scope) apply. Do not touch reverse-association hooks,
provider transport/backend/cache, request scheduling/retries, global store, new
generations/counters, HEAD/timestamp refresh, other providers, package/test config
or consumer production without genuine causal evidence and ROOT review.
No browser/build/E2E or public guide change is needed for this pure state fix.

## Acceptance

1. One permanent causal RED selection fails for pending old/new workspace,
   changed membership and actual first scoped list commit, while the current
   acknowledgement control passes. The full affected GREEN covers every matrix
   case and existing controls afterward.
2. Only `use-pr-statuses.ts` changes in production. Public Map/key APIs, current
   map/status identities, scheduling, failure and cancellation behavior remain
   consistent with the paired design, including both A-to-B-to-A paths.
3. All task-defined checks pass with actual results and serial original-handle
   joins/group-gone receipts. Mark local work done and promote paired specs only
   after those checks; hosted/merge gates remain explicitly pending.

## Regression matrix

Executed new describe groups: `usePRStatuses requested batch`. All 26 new cases
passed in affected GREEN. The table maps ACs to their real transport/lifecycle
and first-render/list-commit evidence; it does not claim browser verification.

| Test name or case | AC suffixes | Stimulus and observation |
| --- | --- | --- |
| `hides previous workspace while current request is pending` | .1, .4 | Resolve A for PR7; request B for PR7 and hold response; first hook render has no old summary; verify real B transport args |
| `hides previous batch while new membership is pending` | .1 | Resolve PR7; request PR7+PR8, and separately remove/reorder membership; old shared-key summary is absent before any new response |
| `first scoped PRList commit omits prior green badge` | .1, .7 | Real A row shows 8/8; rerender B with same PR identity/current title; commit observer before passive effects sees current title/action and no old badge |
| Null workspace and empty list | .2, .7 | Start populated; independently request null or []; record first render empty, no new call; real null-workspace row has no old badge |
| `renders current acknowledged batch` | .4, .7 | Real pending row has no status badge; resolve current transport with distinct summary; actual badge and current title/action render |
| Equal-content completed and pending lists | .3 | Rebuild array/objects retaining ordered identities; same completed Map/status references and no refetch; held pending read is not cancelled and publishes once |
| Current failure and successful empty response | .4 | Reject current initial/changed read; empty remains and unrelated same-input render causes no automatic retry; current success with missing/empty statuses stays empty and completed |
| Obsolete success and failure | .5 | A pending -> B pending/current success; resolve or reject A afterward; B result and badge are neither replaced nor cleared |
| Unmount | .5 | Unmount real hook/list before success or rejection; settle held promises and continuations; no update/extra request |
| StrictMode | .5 | Mount under real StrictMode; hold both effect requests; cancelled first outcome cannot publish/clear; second current acknowledgement becomes visible |
| Independent hook instances | .6 | Same PR key in separate A/B consumers; settle in reverse order with distinct status objects; switching/clearing one cannot affect the other |
| Completed A -> pending B -> A | .3, .6 | Same original A Map/status identities return; no extra A request; later B success/failure ignored by cleanup |
| Pending A1 -> B -> pending A2 | .5, .6 | Earlier A1 success/failure cannot win despite equal A keys; A2 acknowledgement alone publishes current result |
| Completed A -> failed B -> A | .4, .6 | B failure clears retained A while completed key remains A; A stays empty with existing no-new-request behavior, without inventing recovery |

Keep `use-pr-statuses.test.ts` key/empty/success/equal-content/refetch/failure
controls, `pr-list.test.tsx` enriched-launch/fallback controls and
`pr-status-badges.test.tsx` singular/plural controls unchanged unless a directly
causal fixture correction is necessary. All three run alongside the new suite.

## Fixture constraints and TDD

Read the immutable accepted proof only as evidence. Never replay, copy wholesale,
modify or remove its archive/logs/receipts. Author new permanent tests only after
the later ROOT author grant. Do not make production changes before the causal RED.

Keep `usePRStatuses`, `PRList`, `PRStatusBadges`, `StateProvider`/composed store,
native router, Tooltip and locales real. Partially mock only `getPRStatusesBatch`
using `importOriginal` and deferred native Promises with explicit resolve/reject.
No mocked production hook/store/row, copied predicate, source inspection assertion
or helper test that merely mirrors the implementation.

Capture real-hook values from the first new-input render. For the actual list,
use a test-only container/commit observer in a layout effect before passive
effects, so an effect-only clear cannot falsely satisfy the contract. Do not
export production instrumentation or build a new shared harness. Compare real
Map/status references and actual badge/title/task controls, with distinct old
and current data. Transport call counts/arguments prove preserved scheduling.

Record held requests explicitly. After each test, unmount first, then settle all
remaining requests and flush continuations through `act`; leave no mounted
providers or pending asynchronous work. Fixture rejections are expected stimuli;
actual resource/setup/transport errors are checkpoints, not causal RED results.

## Mobile and docs

Use the [pure-state assessment](plan.md#mobile-and-public-documentation-assessment).
The same shared row consumes the Map on desktop and phone. No geometry, copy,
navigation, touch, scroll or breakpoint change; real component evidence satisfies
the mobile-parity exception. Record why browser/build/Playwright were not run.

## Verification

These commands were used after the later implementation grant.
Run from repository root in Bash with `login:false`, explicit installed Node
PATH and pinned Corepack. ONE GLOBAL LOCAL HEAVY lease is mandatory for install,
product tests, ESLint, typecheck and heavy hooks. No overlap or automatic retry.

Before each command record start, absolute cutoff, argv, log, unique shell PID
and PGID. Retain every original outer session/chunk, actually join it and prove
the entire original group gone before proceeding. Publish an explicit HEAVY
RETURN after clean aligned publication and all local joins, before hosted wait.

```bash
export PATH="/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:$PATH"
export NODE_OPTIONS="--max-old-space-size=4096"
# Once only, after ROOT grants the exclusive lease, if dependencies are absent.
if [ ! -d apps/node_modules ] || [ ! -d apps/web/node_modules ]; then
  (cd apps && timeout --signal=TERM --kill-after=10s 120s corepack pnpm@9.15.9 install --frozen-lockfile)
fi

# RED against unchanged production; current ACK is the positive fixture control.
(cd apps/web && timeout --signal=TERM --kill-after=10s 120s corepack pnpm@9.15.9 exec vitest run --project browser-locales components/github/my-github/use-pr-statuses.scope.test.tsx -t 'hides previous workspace while current request is pending|hides previous batch while new membership is pending|first scoped PRList commit omits prior green badge|renders current acknowledged batch' --maxWorkers=1 --no-file-parallelism)

# GREEN: only affected suites and unchanged immediate consumer controls.
(cd apps/web && timeout --signal=TERM --kill-after=10s 120s corepack pnpm@9.15.9 exec vitest run --project browser-locales components/github/my-github/use-pr-statuses.scope.test.tsx components/github/my-github/use-pr-statuses.test.ts components/github/my-github/pr-list.test.tsx components/github/my-github/pr-status-badges.test.tsx --maxWorkers=1 --no-file-parallelism)
(cd apps/web && timeout --signal=TERM --kill-after=10s 120s corepack pnpm@9.15.9 exec eslint --max-warnings 0 components/github/my-github/use-pr-statuses.ts components/github/my-github/use-pr-statuses.scope.test.tsx)
(cd apps/web && timeout --signal=TERM --kill-after=10s 120s corepack pnpm@9.15.9 run typecheck)
(cd apps/web && timeout --signal=TERM --kill-after=10s 120s corepack pnpm@9.15.9 run i18n:ratchet)

timeout --signal=TERM --kill-after=10s 60s python3 scripts/list-docs.py validate
timeout --signal=TERM --kill-after=10s 60s python3 scripts/lint-spec-files.test.py
timeout --signal=TERM --kill-after=10s 60s python3 scripts/lint-spec-files.py --all
git diff --check
git status --short
```

If an existing test fixture actually changes, add its exact path to changed
ESLint. The normal project `run typecheck` includes existing ignored JSON
pretypecheck generation; do not replace it with direct `tsc`, install another
runtime or repair configuration. i18n ratchet covers actual changed source;
no new copy/catalog edits are planned. Do not run a broad suite for this fix.

Run this light reference preflight from repository root, bounded at 60s/kill10
with the same upfront command receipts. It validates actual changed paths and
checks the four documents with the repository's coverage resolver. Design-only
changes are exempt, so a separately labelled planned-hook reference check is
needed during design; it does not claim an actual production diff or test pass.

```bash
timeout --signal=TERM --kill-after=10s 60s /home/jcfs/.local/share/mise/installs/node/24.21.0/bin/node <<'NODE'
const fs = require('node:fs');
const assert = require('node:assert/strict');
const { execFileSync } = require('node:child_process');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const paths = [
  'docs/specs/integrations/requirements/github-browse-status-batches.md',
  'docs/specs/integrations/system-design/github-browse-status-batches.md',
  'docs/plans/github-pr-status-batch-scope/plan.md',
  'docs/plans/github-pr-status-batch-scope/task-01-scope-status-results.md',
];
const fileContents = Object.fromEntries(paths.map(p => [p, fs.readFileSync(p, 'utf8')]));
const tracked = execFileSync('git', ['diff', '--name-only', '-z', 'HEAD'], { encoding: 'utf8' }).split('\0').filter(Boolean);
const untracked = execFileSync('git', ['ls-files', '--others', '--exclude-standard', '-z'], { encoding: 'utf8' }).split('\0').filter(Boolean);
const changedFiles = [
  ...tracked.map(filename => ({ filename, status: 'modified' })),
  ...untracked.map(filename => ({ filename, status: 'added' })),
];
const actual = validateCoverage({ changedFiles, fileContents });
console.log(JSON.stringify({ actual }, null, 2));
assert.equal(actual.ok, true);
assert.deepEqual(actual.errors, []);
if (tracked.includes('apps/web/components/github/my-github/use-pr-statuses.ts')) {
  assert.equal(actual.status, 'covered');
} else {
  assert.deepEqual([...untracked].sort(), [...paths].sort());
  assert.deepEqual(tracked, []);
  const plannedReference = validateCoverage({
    changedFiles: [...changedFiles, { filename: 'apps/web/components/github/my-github/use-pr-statuses.ts', status: 'modified' }],
    fileContents,
  });
  console.log(JSON.stringify({ designOnlyPlannedReference: plannedReference }, null, 2));
  assert.equal(plannedReference.status, 'covered');
  assert.equal(plannedReference.ok, true);
  assert.deepEqual(plannedReference.errors, []);
}
NODE
```

Design catalog check must confirm both new specs are actually discovered. Record
RED causal failures and positive control, GREEN actual suite/test counts, each
check exit, changed-path coverage and full command/process receipts. Resources,
timeout, actual transport/setup failure, unknown outcome or out-of-scope changes
checkpoint ROOT without automatic retry. Routine owned causal fixture/lint
corrections remain within the affected scope. Interruption is UNKNOWN/NO VERDICT.

## Files likely touched

- `apps/web/components/github/my-github/use-pr-statuses.ts` (sole production).
- `apps/web/components/github/my-github/use-pr-statuses.scope.test.tsx` (new real regressions/list fixture).
- `apps/web/components/github/my-github/use-pr-statuses.test.ts` only for a causal existing-control fixture adjustment.
- Four design-package files for lifecycle, history and actual results.

Existing `pr-list.test.tsx` and `pr-status-badges.test.tsx` are unchanged controls,
not planned edit targets. Consumer production requires new cause and ROOT review.

## Dependencies

No prior work order. ROOT full-file/hash review, later same-primary author grant
and exclusive local-heavy lease were satisfied before implementation/execution.
An exclusive local-heavy lease remains required for any later heavy fixup. Preserve
worktree/branch/deps/logs, foreign processes/refs/caches/FETCH_HEAD, protected
paused work and unproved volume. External task plan owns publication/wait/merge
gates, user edits, system marker and task/session identity.

## Risks

The producer key and Map must publish together. Same-key late A responses still
need cancellation. A render-time fence must not mutate refs or reset completed
scheduling state. Preserving failure behavior can leave empty data without a
new request; safety is not a recovery promise. Tests must observe first commits.

## Parallelism

`sequential`. Same primary; no delegates, tasks, sessions or model switch.

## Inputs

- [Requirement, AC .1-.7](../../specs/integrations/requirements/github-browse-status-batches.md).
- [Result and scheduling design](../../specs/integrations/system-design/github-browse-status-batches.md).
- [Accepted evidence](plan.md#baseline-and-accepted-evidence).
- `apps/web/AGENTS.md`, `/tdd` and `/mobile-parity` for the later implementation.

## Results

Historical design documentation checks completed on 2026-10-06:

| Design check | Actual outcome |
| --- | --- |
| Catalog validation and discovery | Exit 0; 355 decisions, 1,404 specs; both new specs discovered |
| Specification-linter tests | Exit 0; all 36 passed |
| Full specification lint | Exit 0; all specification files passed |
| Actual changed-path reference classification | Exit 0; exactly four docs paths, exempt, `ok: true`, `errors: []` |
| Separately labelled planned-hook reference validation | Exit 0; covered, owning requirement/design/AC chain accepted, `errors: []` |

The four serial light collectors each recorded upfront command/start/cutoff/log
and shell PID/PGID, then actually joined exit 0 with their groups gone. Logs and
full JSON receipts are `/tmp/kandev-pr-status-design-abeff095-{catalog,spec36,
speclint,references}.{log,json}`; original outer chunks are preserved in the
external task plan. No product execution is represented by these checks.
Final exact hashes, whitespace/inventory and independent process-gone evidence
are preserved in that task plan for ROOT full-file review.

ROOT accepted all four exact design hashes at 16:01:25.673862Z in
`/tmp/kandev-root-child53-reviewed-package-20261006.json`, then granted authoring
and exclusive global local-heavy ownership. The existing Node24.21.0 runtime was
reused; one conditional frozen pinned install completed. Accepted proof remained
read-only and unchanged.

| Implementation check | Actual outcome |
| --- | --- |
| Conditional frozen install | Exit 0; 1.852s; pinned pnpm9.15.9, reused packages, no runtime installation |
| Single causal permanent RED, unchanged source | Exit 1; 5 intended failures, current-ACK control passed, 20 skipped; 5.670s |
| Affected GREEN with corrected root StrictMode fixture | Exit 0; four suites, 36 tests (26 new, 10 existing); 9.508s |
| Changed-file ESLint | Exit 0; no warnings; 1.751s |
| Normal project typecheck | Exit 0; existing ignored JSON pretypecheck ran; 64.076s |
| Actual-source i18n ratchet | Exit 0; 0 added + 1 modified source clean; 643 guard entries intact; 1.600s |
| Final catalog / specification-linter tests / full spec lint | Exit 0 each; 355 decisions, 1,404 specs, all 36 linter tests and all specs passed |
| Actual six-file documentation-reference coverage | Exit 0; hook covered by owning requirement/design/work order; `errors: []` |
| Changed TypeScript formatting | Exit 0; both files already formatted |

Initial affected GREEN had two fixture failures and 34 passing tests: nesting
StrictMode below the test root did not replay initial effects. The installed
testing-library implementation/types expose `reactStrictMode` on the root;
using that supported option exercised both cancelled-first-effect cases and all
36 tests passed. No production or framework/configuration change was needed.

Only `use-pr-statuses.ts` changed in production. It pairs producer key and Map,
returns the snapshot only for a matching nonempty requested key, and otherwise
returns a local stable empty Map. Completed-key scheduling, effect dependencies
and cancellation remain; failed-B return-A recovery is intentionally unchanged.
Existing key/hook, launch and badge tests are unchanged. Real PRList first-commit
evidence proves the actual badge/title/task outcome through real providers,
router/tooltips/locales with transport-only deferred mocks.

Full implementation logs/receipts are `/tmp/kandev-pr-status-impl-abeff095-
{install,red,green,green-root-strict,eslint,typecheck,i18n-ratchet}.{json,log}`.
All original heavy tool handles actually joined and their groups were gone before
the next command. Both original design collector failures (1 and 2) remain
failed in their preserved receipts. The first implementation-context collector
also retained exit 2 for a read-only `.ts` lookup of an existing `.tsx` file;
that was neither product setup failure nor a repeated install.

Final documentation gates passed. The first reference collector incorrectly
forced ES module input for the existing CommonJS preflight and exited 1; running
the unchanged preflight with its documented input mode passed. Both original
receipts remain preserved. Exact local inventory and hook publication receipts
are maintained in the external plan. No browser,
build or E2E ran: shared-row pure-state eligibility changes no layout, copy,
navigation, touch, scrolling or viewport behavior. Public docs remain accurate.
Normal active-hook publication, full current-head hosted review and a separate
ROOT merge grant remain pending. Local implementation does not authorize merge.
