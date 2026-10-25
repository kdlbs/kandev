---
id: "03-reduce-test-setup"
title: "Reduce frontend test setup"
status: in_progress
wave: 2
depends_on: ["07-measure-ci-efficiency"]
plan: "plan.md"
requirements:
  - REQ-PLATFORM-CI-PERFORMANCE-003
  - REQ-PLATFORM-CI-PERFORMANCE-004
  - REQ-PLATFORM-CI-PERFORMANCE-005
acceptance_criteria:
  - AC-PLATFORM-CI-PERFORMANCE-003.1
  - AC-PLATFORM-CI-PERFORMANCE-003.2
  - AC-PLATFORM-CI-PERFORMANCE-004.2
  - AC-PLATFORM-CI-PERFORMANCE-005.2
system_design:
  - ../../specs/platform/system-design/ci-performance.md
---

# Task 03: Reduce frontend test setup

## Summary

Separate explicitly reviewed Node-compatible helpers from browser tests.
Load all locales only for suites that need them, while retaining isolated test state.

## In scope

- Test dependency inventory, explicit project selection, minimal Node setup, and multilingual setup.
- A selection contract that detects omissions and overlapping file assignments.
- Comparable baseline/candidate unit-test reports and phase timings.

## Out of scope

- Workflow sharding, disabled isolation, worker-budget increases, application behavior, and deleted tests.

## Acceptance

- Every existing test file belongs to exactly one project, including files under E2E helper directories.
- Browser and multilingual regressions pass under the inherited production environment; unknown dependencies retain full setup.
- At least three comparable runs per configuration show lower median test execution with identical selected identities and no new failures.

## Verification

Run commands from the repository root. Install workspace dependencies first in a fresh worktree.

```bash
(cd apps/web && pnpm exec vitest run scripts/vitest-project-selection.test.ts scripts/vitest-worker-budget.test.ts vitest-environment.test.tsx lib/i18n)
(cd apps/web && NODE_ENV=production pnpm run test --reporter=json --outputFile=/tmp/kandev-ci-unit-candidate.json)
(cd apps/web && pnpm run typecheck)
python3 .github/scripts/frontend-tests-workflow-contract_test.py
git diff --check
```

Before editing setup, run the full JSON command with a baseline output filename.
Run a complete before/after pair for coverage and outcome equivalence. Repeat a
stratified setup sample three times per configuration on the same source and
runner class, changing only setup configuration. Keep full-suite timing separate
from sample medians; hosted full-suite adoption evidence remains pending.
Compare normalized file/test identities, skips, failures, and per-phase times. Keep raw reports outside the repository.

## Files likely touched

- `apps/web/vitest.config.ts`
- `apps/web/vitest.setup.ts`
- `apps/web/vitest.setup.node.ts (new)`
- `apps/web/vitest.setup.locales.ts (new)`
- `apps/web/scripts/vitest-project-selection.test.ts (new)`
- `apps/web/lib/i18n/*.test.*`
- `apps/web/vitest-environment.test.tsx`

## Dependencies

Task 07. Cache repair code already exists; its remaining hosted miss/save evidence does not block this work.

## Risks

File suffixes do not prove DOM independence. Inventory indirect locale helpers and preserve all default cleanup and network guards.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/platform/requirements/ci-performance.md), acceptance IDs in frontmatter.
- [System design](../../specs/platform/system-design/ci-performance.md), corresponding implementation boundary.
- [Plan](plan.md), baseline and companion-package status.
- Existing workflow contract tests under `.github/scripts/`.

## Results

Framework implemented 2026-09-12. Classification and hosted acceptance remain incomplete. At that snapshot, the explicitly reviewed Node test list used a minimal
Node project, two explicitly reviewed browser tests retain happy-dom and React
setup without loading all locale catalogs, and every other discovered test uses
the full browser and locale setup. New tests do not receive reduced setup from
their directory or source markers. The selection contract uses Vitest's
canonical test-file glob and excludes the existing Playwright-only paths.

The selection contract passed and found all 1,999 test files at that snapshot in exactly
one of `node`, `browser`, or `browser-locales`. The focused production-mode
run passed 12 files and 106 tests. The final unsharded candidate report passed
1,999 files with 17,180 passed tests, 4 pending tests, and 0 failures. The
first candidate attempt exposed one Node-project timeout under contention; the
Node project now has a 15-second test timeout and the repeat passed. Typecheck
passed after adding the selection module's explicit test type.

The required three comparable setup runs on a hosted runner have not been
collected. Local evidence proves selection and correctness, not a hosted
median speed improvement, so this work order remains in progress.

## October continuation

Use [the complete inventory](frontend-audit-2026-10-08.csv) as a review queue, not a generated allowlist.
Start with the 32 files in `experiments/setup-selection.json`, then review the remaining candidates in bounded batches.
Record each accepted assignment, dependency review, baseline/candidate command, and identity/outcome result.
Keep unknown cases in `browser-locales`; do not change their assertions or mock translations to force migration.

The inventory identifies 450 Node candidates, 1,554 English-browser candidates, and 753 retained or unresolved files.
No candidate count is an automatic migration target. Review external package effects and indirect helpers too.
Complete full-suite comparison after the final assignment change, under the inherited production environment and existing local worker budget.
The target is 40% lower hosted execution and 30% fewer frontend runner-minutes; record actual results even when the target is missed.
The three comparable full-suite samples remain the adoption evidence. The 68.1% local sample improvement is not a substitute.

Additional likely file: `apps/web/scripts/vitest-project-selection.ts`.
The setup files marked new above now exist; extend them rather than create a parallel framework.

### Validation refinement during execution

The guarded local full baseline takes over 35 minutes. Repeating its unchanged
bodies six times would dominate the investigation. Full before/after reports
still cover every identity and outcome; three stratified pairs isolate setup
cost across Node and browser candidates. This changes the measurement method,
not selection, assertions, isolation, or worker limits. Do not extrapolate a
sample percentage into a claimed full-workflow or hosted median improvement.

### October local implementation

The explicit manifest is `apps/web/scripts/vitest-reviewed-projects.json`.
It assigns 420 files to Node and 867 to English-browser setup. The remaining
1,470 files retain full locale setup. The selection test also rejects duplicate,
overlapping, or nonexistent reviewed paths. New files retain full setup.

The first complete candidate preserved 2,757 files and all 24,573 assertions,
but one assertion failed because `clipboard-attachments.test.ts` spies on
`DOMParser`. That file now uses browser setup; its assertions are unchanged.
The focused corrected run passed 14 files and 143 tests under production
`NODE_ENV`, including locale, environment, worker-budget, and selection guards.
Web typecheck, changed-file ESLint, and Prettier passed. The complete corrected
candidate passed with all 2,757 files, 24,569 passed tests, and four skips. Exact
file/title/outcome multisets matched the baseline. Local report elapsed fell
from 2,627.355 to 1,641.299 seconds (37.5%); this single-pair result falls short
of the 40% target and does not establish a hosted median. Three stratified setup
pairs also passed: 96 files and 925 tests per run, exact identity/outcome parity,
median command elapsed 60.608s baseline versus 20.400s candidate (66.3% lower).
The curated full and sample reports are linked from the October evidence.
Hosted acceptance remains pending, so this work order stays in progress.

### October 12 continuation

Reviewed 581 additional files: 47 Node and 534 English browser assignments.
Before this batch, the October 11 E2E cleanup added
`e2e/helpers/backend-fixture-lifecycle.test.ts` to Node, taking the prior count
from 420 to 421. The 47 reviewed migrations then brought it to 468.
The registry at that snapshot contains 468 Node and 1,401 browser files; 995 unreviewed/multilingual
files retain full setup at this 2,864-file source snapshot. Test bodies, assertions,
skips, setup implementations, isolation and worker budgets are unchanged for this
batch. See [the review ledger](../../audits/frontend-test-setup-2026-10-12.md) and
[per-file evidence](../../audits/frontend-test-setup-2026-10-12.csv).

The complete production-environment pair passed identical files, test identities,
and outcomes: 25,554 passed and four skipped. Vitest elapsed time fell 1,455.77 to
1,350.39 seconds (7.2% less). Three alternating 36-file/234-test sample pairs at
two workers reduced median wall time from 25.625 to 9.602 seconds (62.5%). These
are separate measurements; the sample percentage does not describe the full suite.
A missing Portuguese preload deliberately failed its announcement assertion, and
the restored full-setup keeper passed. Raw report hashes are recorded in the ledger.

Hosted Actions comparison is pending. Three comparable hosted full samples and
the 40% execution target remain unmet; keep this work order in progress.

### Additional 50% target

The October 12 user request raises the execution target to 50%. Use committed
head `dc36fe3f709ebe35bce51dbdb203c66ae213f4eb` as this continuation's reference:
2,008 seconds for frontend Run tests, with a target of 1,004 seconds. Report
workflow elapsed time and runner-minutes separately. The E2E continuation uses
2,159 seconds for its longest normal test step, targeting 1,079.5 seconds.
These targets are not achieved by a sample benchmark or an earlier baseline.

The next batch reviews 897 additional English-browser assignments, restores two
interop-sensitive browser suites to full setup, and measures
JSON catalog parsing plus a cold Node compile cache. No test-body,
isolation, worker-budget or selection-trigger change is included. A VM-thread
experiment was slower and rejected; global dependency optimization was rejected after
full verification exposed module-identity and mock-interoperability failures.
The scoped candidate bundles dependencies only in the explicitly reviewed browser
project; the three affected suites retain native loading in the full project.
Complete corrected-suite verification and hosted measurements remain pending; see [the continuation audit](../../audits/ci-half-runtime-2026-10-12.md).

The scoped complete run passes 2,864 files, 25,554 tests and four skips, with
exact file/title/outcome parity. Vitest time is 731.89 seconds versus 1,350.39
seconds at the reference head (45.8% lower locally). Three alternating 75-file
pairs preserve 617 passes and four skips each; median command time is 39.882
versus 30.506 seconds (23.5% lower). These are separate measurements. A forks
pool experiment was slower and rejected. Typecheck, ESLint, formatting, the
nine frontend workflow contract tests, discovery and documentation validation
pass. Hosted acceptance and the 50% target remain open.

The first hosted optimized run (`38322865444`, head `b81c562`) passed in
1,091 seconds for Run tests versus 2,008 seconds at `dc36fe3`: 45.7% lower,
87 seconds above the 50% target. Workflow elapsed was 1,448 seconds versus
2,595 seconds. The tested merge base advanced: all 2,872 reference files remain,
with five additional files and extra cases from upstream (2,877 files, 25,668
passes, four skips). This is one historical pair, not the required hosted
median. The work order and stricter continuation target remain open.

### Merge-readiness typecheck remediation

After incorporating main, hosted frontend run 38381773696 failed before unit
tests: the complete TypeScript graph exhausted Node's default 4 GiB heap.
The same command reproduced the heap failure locally; with an 8 GiB heap it
completed successfully. The workflow now scopes that heap ceiling to the
typecheck step alone. Unit-test workers, isolation, selection and memory
configuration remain unchanged. This restores mandatory verification rather
than claiming an execution-time improvement. Targeted workflow contracts and
the complete typecheck are the remediation gates; fresh hosted evidence is
required after the fix is pushed. The fresh full typecheck with an isolated
incremental-cache path passed with the 8 GiB ceiling. All nine frontend workflow
contracts, workflow YAML parsing and step-scope checks, documentation catalog
validation and full specification lint passed. Hosted verification is pending.
