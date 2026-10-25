---
id: "01-fence-shared-branch-reads"
title: "Fence shared branch reads"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-BRANCH-READS-001
acceptance_criteria:
  - AC-WORKSPACES-BRANCH-READS-001.1
  - AC-WORKSPACES-BRANCH-READS-001.2
  - AC-WORKSPACES-BRANCH-READS-001.3
  - AC-WORKSPACES-BRANCH-READS-001.4
  - AC-WORKSPACES-BRANCH-READS-001.5
  - AC-WORKSPACES-BRANCH-READS-001.6
  - AC-WORKSPACES-BRANCH-READS-001.7
  - AC-WORKSPACES-BRANCH-READS-001.8
system_design:
  - ../../specs/workspaces/system-design/repository-branch-reads.md
---

# Task 01: Fence Shared Branch Reads

## Summary

After explicit implementation continuation, prove the three accepted failures
with permanent real-hook/store regressions, then implement the smallest shared
store/source owner fence. Preserve existing cache, source, API, retry, and
consumer behavior and complete the authorized normal PR/merge workflow.

## In scope

- Read `/tdd`; mark this work order in progress only after continuation.
- Rename/replace the mocked-store hook test with real-provider/store tests,
  covering every scenario in the plan's test matrix. Capture meaningful RED
  before production changes, then GREEN with one worker.
- Add a local WeakMap keyed by `StoreApi<AppState>` with unique per-key tokens
  to the existing hook. Guard every accepting branch write and loading cleanup.
  Shared initial demand checks reuse active requests. Explicit refresh starts
  a new owner. Keep retry code unchanged and do not abort on consumer cleanup.
- Include a rendered real hook/store to `BranchPickerList` integration test;
  run existing workspace slice and representative component regressions.
- Update requirement/design statuses after accepted implementation and keep
  plan/work-order validation and delivery evidence accurate.

## Out of scope

Backend/schema, production slice shape/setter changes, consumers' production
markup or selection logic, retries/polling, new abstractions/dependencies,
browser E2E/app/build, locale copy, workers, persistent tasks, or sessions.

## Acceptance

1. Permanent production-store regressions demonstrate the three original
   defects in RED and pass in GREEN; siblings share ownership, distinct keys
   and stores remain independent, and failure/lifecycle/control cases pass.
2. All exact checks below pass with normal hooks; preserve loaded data on
   failure and id/path API/cache contracts with no stale success/finally writes.
## Delivery completion

Local work-order completion records implementation and the checks above.
Overall task completion additionally requires that the ready focused PR receives
required hosted CI and authenticated configured current-head FULL semantic
review for every changed file, actionable items are dispositioned, and normal
expected-head squash merge/content verification and joined owned cleanup are
recorded. PR creation alone is not completion. Current-head delivery receipts
are recorded in the live task plan and PR without publishing documentation-only
commits while CI is running.

## Verification

Run from the repository root, sequentially, one heavy command at a time.
In this environment the existing Node 24 tools need PATH activation in the
execution shell (the preflight used the absolute Node binary successfully):

```bash
export PATH="$HOME/.local/share/mise/installs/node/24/bin:$PATH"
```

Design discovery confirmed both `apps/node_modules` and `apps/web/node_modules`
are absent. After continuation, install once before pnpm tests/lint/hooks:

```bash
(cd apps && pnpm install --frozen-lockfile)
```

Parent root install run 74153 was joined successfully; it does not prove this
worktree has dependencies. Do not reinstall an already installed worktree.

RED: run the new hook suite before production changes. GREEN: run all four
targeted files once the implementation is ready; record actual counts/results.

```bash
(cd apps/web && pnpm exec vitest run hooks/domains/workspace/use-repository-branches.test.tsx --maxWorkers=1)
(cd apps/web && pnpm exec vitest run hooks/domains/workspace/use-repository-branches.test.tsx lib/state/slices/workspace/workspace-slice.test.ts components/task/branch-picker-list.test.tsx components/watcher-repository-fields.test.tsx --maxWorkers=1)
(cd apps/web && pnpm exec eslint hooks/domains/workspace/use-repository-branches.ts hooks/domains/workspace/use-repository-branches.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:ratchet --base 69fa561795f52ee69ef15513e9f55c3c65ddc3f7)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short -- docs/plans/repository-branch-read-ordering
```

No copy is expected. If needed, externalize and translate all seven catalogs
including ko, then add the necessary changed-copy checks; do not expand scope
without recording the reason. If more files change for a necessary correction,
include those exact files in changed-file lint and their affected targeted tests.

Documentation coverage preflight (planned production scope plus package):

```bash
node <<'NODE'
const fs = require('node:fs');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const docs = [
  'docs/specs/workspaces/requirements/repository-branch-reads.md',
  'docs/specs/workspaces/system-design/repository-branch-reads.md',
  'docs/plans/repository-branch-read-ordering/plan.md',
  'docs/plans/repository-branch-read-ordering/task-01-fence-shared-branch-reads.md',
];
const result = validateCoverage({
  changedFiles: ['apps/web/hooks/domains/workspace/use-repository-branches.ts', ...docs],
  fileContents: Object.fromEntries(docs.map(path => [path, fs.readFileSync(path, 'utf8')])),
});
console.log(JSON.stringify(result, null, 2));
if (!result.ok) process.exitCode = 1;
NODE
```

Use `/commit`, `/pr`, and `/pr-fixup` for authorized delivery. Use the normal PR
template/hooks, ready status, and one `scripts/pr-await` monitor. Inspect the
automatic FULL report first; do not request duplicate current-head review.
Join every owned handle; reconcile any lost handle before a verdict or rerun.
Fix failed CI leaves promptly while unrelated checks run; publish necessary
corrections promptly and then freeze the head through CI. Do not rebase only
for main movement while checks run, or replay passing tests without cause.
Before merge, verify expected remote head and use normal squash merge. Verify
the actual merged SHA and owned content independently. Preserve unrelated
branches, worktrees, caches, and the parent's reproduction archive. Report the
merge and cleanup receipt; parent owns task archival and archive removal.

## Files likely touched

- `apps/web/hooks/domains/workspace/use-repository-branches.ts`
- `apps/web/hooks/domains/workspace/use-repository-branches.test.ts` (replace by
  `.test.tsx`, retaining and strengthening its two existing controls)
- `apps/web/hooks/domains/workspace/use-repository-branches.test.tsx`
- The four documents in this package (status and actual validation evidence).

Existing `workspace-slice.test.ts`, `branch-picker-list.test.tsx`, and
`watcher-repository-fields.test.tsx` are verification inputs, not planned edits.

## Dependencies

None. Implementation requires the later explicit parent continuation. First
turn ends at the design handoff with no production or permanent test edits.

## Risks

See the [plan risks](plan.md#risks) and design's
[failure and lifecycle](../../specs/workspaces/system-design/repository-branch-reads.md#failure-and-lifecycle).
Ensure newest failure cannot revive an older success, and cleanup releases only
its own token even if a subscriber starts a newer read during a success write.

## Parallelism

`sequential`. No other workers, tasks, or sessions are authorized.

## Inputs

- [Requirements](../../specs/workspaces/requirements/repository-branch-reads.md)
- [System design](../../specs/workspaces/system-design/repository-branch-reads.md)
- Existing hook, production workspace slice/types, `createAppStore`, and
  `StateProvider`/`useAppStoreApi`.
- Store-scoped WeakMap patterns in `use-office-workspace-data.ts` and
  `lib/state/prompts-loader.ts`; real-store tests in
  `use-repository-branch-policies.test.tsx`.
- Parent's accepted three-failure/one-control evidence in the plan.
- Mobile-parity state/data-only exception recorded in the plan; no new rendered
  structure or copy. Public-docs assessment also recorded there.

## Results

Parent reviewed the four documents and authorized implementation on 2026-10-02.
The production change is confined to the existing hook: per-store/source unique
symbol ownership guards success and finally, including synchronous subscriber
refreshes, and shares initial reads. No slice, API, consumer, retry, layout,
locale, or backend changes were made.

Permanent real-provider/production-store coverage replaces the mocked-store
suite and covers all eight acceptance criteria, including reentrant publication,
remount after terminal failure, and rendered BranchPickerList integration.

- Frozen worktree install: exit 0, 2 seconds, no lockfile change.
- RED: original races and reentrant/sibling/outcome failures reproduced; see
  [plan verification](plan.md#verification-results) for intermediate counts.
- Final targeted hook suite: 26 passed with `--maxWorkers=1` (146 ms, 2.98 seconds).
- Four-file targeted command: three unchanged suites passed all 21 existing
  tests; hook suite was subsequently corrected and passed as above. Total
  accepted targeted evidence: 47 passing tests across the four named files.
- Changed-file eslint with `--max-warnings 0`: exit 0, zero warnings/errors.
- Final `pnpm run typecheck`: exit 0; generated release-note/changelog output
  remained unchanged.
- `i18n:ratchet --base 69fa561795f52ee69ef15513e9f55c3c65ddc3f7`: exit 0;
  modified hook clean and 643 allowlist entries intact.
- Catalog validation, 36 spec-linter tests, and full specification lint: exit 0.
- Documentation coverage preflight: exit 0, covered, no errors.
- Final whitespace/package status checks passed before commit. The normal active
  hook receipt is recorded in the live task plan after the commit finishes.

Hosted CI, FULL semantic review, actual normal merge/content verification, and
owned cleanup remain pending externally. No delivery-complete claim is made.
