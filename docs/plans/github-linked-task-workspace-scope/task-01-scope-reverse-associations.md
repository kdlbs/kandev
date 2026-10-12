---
id: "01-scope-reverse-associations"
title: "Scope GitHub reverse associations to the current workspace context"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITHUB-REVERSE-ASSOCIATIONS-001
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-REVERSE-ASSOCIATIONS-001.1
  - AC-INTEGRATIONS-GITHUB-REVERSE-ASSOCIATIONS-001.2
  - AC-INTEGRATIONS-GITHUB-REVERSE-ASSOCIATIONS-001.3
  - AC-INTEGRATIONS-GITHUB-REVERSE-ASSOCIATIONS-001.4
  - AC-INTEGRATIONS-GITHUB-REVERSE-ASSOCIATIONS-001.5
  - AC-INTEGRATIONS-GITHUB-REVERSE-ASSOCIATIONS-001.6
  - AC-INTEGRATIONS-GITHUB-REVERSE-ASSOCIATIONS-001.7
system_design:
  - ../../specs/integrations/system-design/github-pr-task-association-reads.md
---

# Task 01: Scope Reverse Associations

## Summary

Gate the existing reverse-association hook on requested and active workspace plus
cache workspace/generation stamps. Prove stale links disappear and current links
remain through the real hook/store/transport/row path, preserving grouping and
consumer APIs. Implementation and task-defined local checks completed after
ROOT reviewed the design and released the same primary session with an
exclusive local-heavy lease. Hosted review and merge remain external gates.

## In scope

- One hook correction and the scoped regression matrix below.
- Existing grouping fixture adaptation to a valid active/stamped context; use a
  partial API mock for `listWorkspaceTaskPRs`, replacing the hook-wide mock.
- Immediate real row integration in the new hook test file; no new test harness.
- Task-defined checks and accurate owning specification/work-order lifecycle.

## Out of scope

The [plan exclusions](plan.md#scope) apply. In particular, do not change
`use-task-pr.ts`, `fetchedRef`, request scheduling, store/global caches, backend,
settings, permissions, revisions, reconciliation, unlink, identity normalization,
other providers, issues, consumer production code, package declarations, test
configuration or generated tracked files without new causal need and ROOT review.
No browser/build/E2E run is justified for the pure state change.

## Acceptance

1. Real-hook regressions fail causally before the correction for stale workspace,
   generation, null request and actual old-task button; all matrix cases pass
   afterward with positive current-context and independent-store controls.
2. Only the existing hook changes in production. Public signature, key format,
   association ordering/identity, grouping and actual current-task controls are
   preserved, including updates that retain `byTaskId` identity.
3. Task-defined checks pass and record actual evidence; authoring uses
   `in_progress`, and the work order is marked done only after all checks pass.

## Regression matrix

New suite: `hooks/domains/github/use-pr-key-to-tasks.scope.test.tsx`.
All 16 cases below passed in the final affected run. The AC mapping records
executed behavior; it does not claim browser or hosted verification.

| Executed test | AC suffixes | Meaningful stimulus and observation |
| --- | --- | --- |
| `hides A links while requested B is pending` | .2, .5 | Active B/new generation, A-stamped cache, real pending list call for B; empty map |
| `hides the old task button on requested B row` | .2, .7 | Separate real hook-to-row fixture with active B and A-stamped cache; assert existing empty control and no old single/multiple task control |
| `rejects a requested workspace different from active` | .2, .4 | A cache and active A; prop-only request B then A; empty then original A records without remount |
| `hides links without a requested workspace` | .2, .7 | Null request against a populated active/stamped A store; no list request, empty map/row |
| `rejects old generations`; `rejects a missing cache %s after initializer normalization` | .3, .4 | Active A/new generation with old A stamp; separately remove each cache stamp from the real initialized snapshot; empty map despite unchanged collection |
| `reacts to A B A and same-workspace generation changes` | .3, .4 | Capture real store via `useAppStoreApi`; change selection through `setActiveWorkspace` and generation through `resetKanbanWorkspaceContext`; preserve cache collection and assert old A never reappears |
| `reacts when only cache scope metadata changes` | .1, .4 | Keep `byTaskId` reference, replace metadata through real store `setState`; matching→stale→matching alters map without network settlement |
| `preserves current cached links during pending or failed reads` | .1, .5, .7 | Valid A cache, held then rejected real API promise; original A object and real single-task button remain |
| `keeps stale links empty after failure`; `accepts a current response and renders its task button` | .4, .5 | Separate stale-cache failure and B-success fixtures; reject one, resolve other with B rows; empty versus B map/button |
| `keeps an obsolete response unreadable after a context change` | .3, .5 | Start A/generation 1 list, advance A generation before resolution; real loader publishes old stamp; empty map, no forced rescheduling assertion |
| `clears the reverse read on a current empty response` | .5 | Eligible cache followed by successful current empty list; no key/button, existing empty indicator |
| `isolates same-key links across independent stores` | .6, .7 | Separate top-level providers with current A, current B and obsolete A cache in active B, all on one PR key; current providers show only their own task, obsolete provider stays empty and cannot affect them |
| `preserves record identity and multi-task multi-repo grouping` | .1, .6, .7 | Current stamp, multiple tasks sharing one key plus one task with multiple repositories/PRs; original object references/order and real multiple-task control |

Existing `use-pr-key-to-tasks.test.ts` retains all `prKey` controls, grouping,
different-key and non-array hydration checks with compatible scope fixtures.
Run the existing `components/github/my-github/pr-row-task-indicator.test.tsx`
suite unchanged unless its immediate fixture needs a causal correction.

## Fixture constraints and TDD

Read the accepted original proof as evidence only; never replay, copy wholesale,
modify or remove `/tmp/kandev-github-pr-reverse-scope-repro.test.tsx`. Write new
permanent regressions in the owned test file. First run the causal RED subset
below against unchanged production, then make the narrow hook correction, then
run the full affected GREEN set.

Keep `usePRKeyToTasks`, `useWorkspacePRs`, `StateProvider`, composed `createAppStore`,
`PRRowTaskIndicator`, shared `TaskRowIndicator`, native custom router, localization
and tooltips real. Only `listWorkspaceTaskPRs` is partially mocked with explicit
deferred resolve/reject controls. Do not mock a production hook/store/row or copy
the predicate. Capture the real provider store to change context; nested
providers share stores, so use top-level providers for independence.

Assert `Map` entries, record object identities and actual empty/single/multiple
controls. An empty-label text assertion alone is insufficient. Unmount first,
then settle held responses and flush their continuations; leave no held timers,
requests or mounted providers. Distinguish expected rejected transport fixtures
from an actual transport/setup/resource failure.

## Mobile and docs

Use the [pure-state assessment](plan.md#mobile-and-public-documentation-assessment).
No layout/copy/navigation/touch changes or public-doc edits are planned. Real
shared-row component tests satisfy this mobile-parity exception. Record the
precise reason browser/build/Playwright were not run.

## Verification

Commands below were used during the later implementation turn. Run from
the repo root in `/bin/bash` with `login:false`; each subshell has its own cwd.
Set the existing Node PATH once. Use pinned Corepack for every package command,
never the default pnpm12, and no runtime installation. Before each bounded
command publish native PID/group/start/absolute cutoff/argv/log; retain the
original handle, actually join it, then prove its group gone before the next
command. No overlap or auto-retry.

```bash
export PATH="/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:$PATH"
export NODE_OPTIONS="--max-old-space-size=4096"
# Only after ROOT-exclusive heavy lease; one install if deps are absent.
if [ ! -d apps/node_modules ] || [ ! -d apps/web/node_modules ]; then
  (cd apps && timeout --signal=TERM --kill-after=10s 120s corepack pnpm@9.15.9 install --frozen-lockfile)
fi

# RED: intended causal failures plus a current-context fixture control, before any production edit.
(cd apps/web && timeout --signal=TERM --kill-after=10s 120s corepack pnpm@9.15.9 exec vitest run --project browser-locales hooks/domains/github/use-pr-key-to-tasks.scope.test.tsx -t 'hides A links while requested B is pending|rejects old generations|hides links without a requested workspace|hides the old task button on requested B row|preserves current cached links during pending or failed reads' --maxWorkers=1 --no-file-parallelism)

# GREEN: after the correction; covers all changed suites and immediate row controls.
(cd apps/web && timeout --signal=TERM --kill-after=10s 120s corepack pnpm@9.15.9 exec vitest run --project browser-locales hooks/domains/github/use-pr-key-to-tasks.test.ts hooks/domains/github/use-pr-key-to-tasks.scope.test.tsx components/github/my-github/pr-row-task-indicator.test.tsx --maxWorkers=1 --no-file-parallelism)
(cd apps/web && timeout --signal=TERM --kill-after=10s 120s corepack pnpm@9.15.9 exec eslint --max-warnings 0 hooks/domains/github/use-pr-key-to-tasks.ts hooks/domains/github/use-pr-key-to-tasks.test.ts hooks/domains/github/use-pr-key-to-tasks.scope.test.tsx)
(cd apps/web && timeout --signal=TERM --kill-after=10s 120s corepack pnpm@9.15.9 run typecheck)
(cd apps/web && timeout --signal=TERM --kill-after=10s 120s corepack pnpm@9.15.9 run i18n:check)
(cd apps/web && timeout --signal=TERM --kill-after=10s 120s corepack pnpm@9.15.9 run i18n:ratchet)

timeout --signal=TERM --kill-after=10s 60s python3 scripts/list-docs.py validate
timeout --signal=TERM --kill-after=10s 60s python3 scripts/lint-spec-files.test.py
timeout --signal=TERM --kill-after=10s 60s python3 scripts/lint-spec-files.py --all
timeout --signal=TERM --kill-after=10s 60s git diff --check
```

The normal project `run typecheck` includes tracked `pretypecheck` preparation
of ignored local release-note/changelog JSON. Do not substitute direct `tsc` or
edit scripts/source/harness/global settings when ignored generated files are
missing. ESLint must include the row test if its fixture actually changes.

Run this repository documentation-reference preflight with the absolute existing
Node, bounded at 60s/kill10 under the same native collector discipline. It reads
the actual owned files and actual changed checkout inventory; it does not
fabricate execution coverage or hosted evidence. Design-only reference checking
uses the planned hook trigger separately, as recorded in the plan.

```bash
timeout --signal=TERM --kill-after=10s 60s /home/jcfs/.local/share/mise/installs/node/24.21.0/bin/node <<'NODE'
const fs = require('node:fs');
const assert = require('node:assert/strict');
const { execFileSync } = require('node:child_process');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const paths = [
  'docs/specs/integrations/requirements/github-pr-task-association-reads.md',
  'docs/specs/integrations/system-design/github-pr-task-association-reads.md',
  'docs/plans/github-linked-task-workspace-scope/plan.md',
  'docs/plans/github-linked-task-workspace-scope/task-01-scope-reverse-associations.md',
];
const fileContents = Object.fromEntries(paths.map(p => [p, fs.readFileSync(p, 'utf8')]));
const tracked = execFileSync('git', ['diff', '--name-only', '-z', 'HEAD'], { encoding: 'utf8' }).split('\0').filter(Boolean);
const untracked = execFileSync('git', ['ls-files', '--others', '--exclude-standard', '-z'], { encoding: 'utf8' }).split('\0').filter(Boolean);
assert.ok(tracked.includes('apps/web/hooks/domains/github/use-pr-key-to-tasks.ts'));
const changedFiles = [
  ...tracked.map(filename => ({ filename, status: 'modified' })),
  ...untracked.map(filename => ({ filename, status: 'added' })),
];
const result = validateCoverage({ changedFiles, fileContents });
console.log(JSON.stringify(result, null, 2));
assert.equal(result.ok, true);
assert.deepEqual(result.errors, []);
NODE
```

Record actual RED failures, GREEN suite/test counts, each check exit, source
inventory and AC-to-executed-test mapping. No generic coverage percentage or
separate broad QA/review/verify pass. Resource/timeout/transport/setup/unknown or
out-of-scope result means ROOT checkpoint END, no automatic retry; routine owned
causal fixture/lint corrections stay affected-only. A crash is UNKNOWN/NO
VERDICT until original process handles and results are reconciled.

## Files likely touched

- `apps/web/hooks/domains/github/use-pr-key-to-tasks.ts` (sole production file).
- `apps/web/hooks/domains/github/use-pr-key-to-tasks.test.ts` (existing controls).
- `apps/web/hooks/domains/github/use-pr-key-to-tasks.scope.test.tsx` (new regressions).
- `apps/web/components/github/my-github/pr-row-task-indicator.test.tsx` only for
  an evidenced immediate compatibility fixture correction.
- The four owning design-package files for lifecycle/results updates.

## Dependencies

No work-order dependency. ROOT concrete review, later explicit implementation
interrupt and exclusive local-heavy lease were satisfied before execution.
One conditional frozen install completed because dependencies were absent;
installed Node24.21.0 was reused. Preserve
managed worktree, branch, dependency tree, proof, logs, shared caches and foreign
references. The live task plan preserves later publication, CI/review wait,
ROOT-only separate merge authorization and independent completion gates.

## Risks

Missing memo dependencies can miss stamp-only changes; collection-scoped checks
must not become record filters or silently repair excluded stale writes. Old
grouping tests need valid active context to remain useful positive controls.
Safe emptiness does not prove a replacement request or recovery occurred.

## Parallelism

`sequential`. Same primary/profile; no delegates, new sessions/tabs or model switch.

## Inputs

- [Requirement and AC .1 through .7](../../specs/integrations/requirements/github-pr-task-association-reads.md).
- [Read design](../../specs/integrations/system-design/github-pr-task-association-reads.md).
- [Baseline proof and limits](plan.md#baseline-and-accepted-evidence).
- Scoped `apps/web/AGENTS.md`; repository `/tdd` and `/mobile-parity` skills.

## Results

Design ended at 13:36:29.508, before permanent changes. ROOT reviewed the four
exact artifact hashes in `/tmp/kandev-root-child52-design-review-20261006.json`;
all matched before authoring began. Authoring ended at 13:48:21 with no product
execution. ROOT then granted this same primary session exclusive local-heavy
ownership after accepting CHILD51's actual return.

The sole production correction selects the existing cache and active context,
returns an empty map on any scope mismatch and memoizes against every selected
snapshot plus requested workspace. The inversion loop, record references,
ordering, key shape and consumers are preserved. `use-task-pr.ts` is unchanged.

| Check | Actual outcome |
| --- | --- |
| Conditional pinned frozen install | Exit 0, 1.958s; installed once, existing runtime reused |
| Permanent causal RED, unchanged production | Four intended scope/real-button failures; current pending/failure control passed; 11 skipped; exit 1, 5.585s |
| Final affected GREEN | Three suites, 30 tests passed: 16 scope, 9 key/grouping and 5 existing row controls; exit 0, 8.402s |
| Final affected ESLint | Exit 0, no warnings, 1.229s |
| Normal project typecheck | Exit 0, 8.308s, including tracked pretypecheck JSON preparation |
| i18n check / new-code ratchet | Exit 0, 9.973s / 1.594s; no copy or catalog changes |
| Documentation catalog / validator tests / full spec lint | 355 decisions and 1,402 specifications valid; all 36 tests passed; all spec files passed |
| Actual documentation coverage | Actual seven changed files, sole-hook trigger, covered, `ok: true`, `errors: []` |
| Whitespace and inventory | Exact seven owned paths; protected proof checksum and excluded loader blob unchanged; prior original processes independently gone |

Initial affected GREEN also passed 30 tests. The first ESLint run reported six
owned fixture warnings; constants and smaller describe blocks corrected them.
The first typecheck reported two missing required `WorkspaceState.items` fixture
fields; both gained `items: []`. Their failures are retained, and affected GREEN,
ESLint and normal project typecheck passed after the final fixture correction.
No source/harness/global-setting workaround or excluded scheduling change was
made. The regression matrix above maps AC .1 through .7 to these executed tests.

Native collectors were serial, each with upfront PID/group/start/absolute UTC
cutoff/argv/log, actual original-handle join and group-gone proof. Product checks
used 120s TERM/kill10, Node4GiB, Node24.21.0 and pinned Corepack pnpm9.15.9;
documentation/inventory checks used 60s TERM/kill10. Logs and JSON receipts are
`/tmp/kandev-github-scope-{red,green,green-final,green-typed-fixtures,eslint,
eslint-final,eslint-typed-fixtures,typecheck,typecheck-typed-fixtures,i18n-check,
i18n-ratchet,catalog,spec-tests,spec-lint,actual-references,inventory}.{log,json}`.
Final GREEN PID1827025, ESLint1828009 and typecheck1828268 all joined exit 0
and their process groups were gone. Failed RED PID1819214, initial ESLint1821045
and initial typecheck1824020 also joined and were gone before subsequent work.

No browser, build or Playwright run: this is pure cache-read eligibility with no
layout, copy, navigation, touch or server change. Real shared-row integration
supplies the required user-visible evidence and mobile pure-state exception.
Public documentation assessment remains unchanged.

Task 01 is done for implementation and task-defined local verification;
requirement active, design current and plan implemented. Normal-hook publication,
full current-head hosted review/CI and a separate ROOT merge grant remain pending
at this committed checkpoint. Local completion does not complete the platform
task or authorize merging.
