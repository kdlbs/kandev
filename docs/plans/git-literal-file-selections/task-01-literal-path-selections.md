---
id: "01-literal-path-selections"
title: "Keep selected Git paths literal"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-WORKSPACE-GIT-STATUS-001
acceptance_criteria:
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.7
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.9
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.10
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.11
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.31
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.33
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.36
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.37
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.38
system_design:
  - ../../specs/platform/system-design/workspace-git-path-details.md
---

# Task 01: Keep selected Git paths literal

## Summary

Prove selected-file index mutations and individual patches exclude wildcard-matching siblings,
then apply the shared literal path representation at the six affected command sites.
Keep the existing transport and lifecycle contracts and synchronize docs and verification results.

## In scope

- All regression cases and command sites in the plan; public-entry-point operator/tracker
  tests plus focused HTTP transport coverage. Snapshot selected/unselected index and worktree
  bytes; use distinct staged/unstaged patch markers and real renamed/deleted entries.
- One process-local helper and six call-site corrections; keep actual directory subtrees,
  multi-path selection, empty-all commands, locking, validation, refs, refresh and budgets.
- Existing `docs/public/git-operations.md` wording, normal hooks and delivery gates.

## Out of scope

Discard changes without new scoped evidence, UI/API/schema, global environment changes,
new pattern contract, broad parser/security/history redesign, new workers or local full suites.

## Acceptance

1. Permanent tracked bracket Stage, Unstage and tracker-patch cases fail for unintended sibling
   selection before production changes. All planned tests pass after correction, including
   native filename variants, directory/multi-path/empty selection, renamed/deleted paths and
   selected/unselected byte assertions. Portable bracket cases do not skip Windows.
2. Both selected mutations and all four patch calls use literal selectors after `--`.
   Public tracker and HTTP results preserve exact file keys and correct comparison facets;
   no unrelated marker enters a selected patch. Ref/flag validation and error propagation remain.
3. Exact checks below and normal hooks pass, public contract wording agrees with the code,
   and plan/work-order results record actual commands. Delivery completes only on verified
   normal squash merge and joining all owned handles.

## Verification

From repo root, after the explicit implementation continuation, mark this work order
`in_progress`, write permanent regressions, and run RED before production edits:

```bash
(cd apps/backend && GOMAXPROCS=2 go test -p 2 ./internal/agentctl/server/process -run '^(TestGitOperatorLiteralSelections|TestWorkspaceGitLiteralPatchSelection|TestWorkspaceGitLiteralCachedFallback)$' -count=1)
```

After the minimum correction run each command sequentially, once:

```bash
set -euo pipefail
(cd apps/backend && GOMAXPROCS=2 go test -race -p 2 ./internal/agentctl/server/process -run '^(TestGitOperatorLiteralSelections|TestWorkspaceGitLiteralPatchSelection|TestWorkspaceGitLiteralCachedFallback|TestWorkspaceGitDetailsPreservesRenameDestinations|TestWorkspaceGitStatusPreservesMixedChangeFacets|TestMixedChangeFacetDiffBudgetCountsAllRepresentations|TestCapDiffOutput_Truncation|TestEnrichWithDiffData_CanceledContextReturnsErrorForTrackedFiles|TestDiffBudgetAndCarryForwardHonorCancellation|TestWorkspaceTrackerRetriesOnlyFailedDiffAfterTransientGitFailure|TestWorkspaceTrackerOnlyRetriesUnavailableDetailsOnExplicitRefresh|TestParseWorkspaceNumstatZ|TestParseWorkspaceNumstatZRejectsInvalidCounts|TestWorkspaceGitMalformedNumstatDetailsUnavailable|TestWorkspaceGitEmptyNumstatDetailsReady)$' -count=1)
(cd apps/backend && GOMAXPROCS=2 go test -race -p 2 ./internal/agentctl/server/process -run '^(TestWorkspaceTrackerRetriesOnlyFailedDiffAfterTransientGitFailure|TestWorkspaceTrackerOnlyRetriesUnavailableDetailsOnExplicitRefresh|TestWorkspaceTrackerQueuesRetryAfterUnavailablePublicationBeforeWorkerSettles)$' -count=1)
(cd apps/backend && GOMAXPROCS=2 go test -race -p 2 ./internal/agentctl/server/api -run '^(TestHandleGitLiteralSelections|TestHandleGitStageAndUnstage)$' -count=1)
(cd apps/backend && GOMAXPROCS=2 golangci-lint run ./internal/agentctl/server/process/... ./internal/agentctl/server/api/... --concurrency=2 --new-from-rev=2f74eaf6a0671c7ae5cac8bdabf0a7f3c16e16a7 --timeout=5m)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
PATH="/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:$PATH" node scripts/validate-public-docs.mjs
git diff --check
git status --short -- docs/plans/git-literal-file-selections
```

Run the coverage preflight from root at design and implementation checkpoints:

```bash
PATH="/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:$PATH" node <<'JS'
const fs = require('node:fs');
const {validateCoverage} = require('./.github/scripts/pr-docs.cjs');
const docs = [
  'docs/plans/git-literal-file-selections/plan.md',
  'docs/plans/git-literal-file-selections/task-01-literal-path-selections.md',
  'docs/specs/platform/requirements/workspace-git-status.md',
  'docs/specs/platform/system-design/workspace-git-path-details.md',
];
const source = [
  'apps/backend/internal/agentctl/server/process/git.go',
  'apps/backend/internal/agentctl/server/process/git_pathspec.go',
  'apps/backend/internal/agentctl/server/process/workspace_git_diff.go',
];
const result = validateCoverage({
  changedFiles: [...docs, ...source],
  fileContents: Object.fromEntries(docs.map(p => [p, fs.readFileSync(p, 'utf8')])),
});
console.log(JSON.stringify({ok: result.ok, status: result.status, errors: result.errors}));
process.exitCode = result.ok ? 0 : 1;
JS
```

Use `/tdd` during implementation and delivery skills for commit/push/PR. Normal hooks run
without bypasses; install frozen pnpm dependencies from `apps/` once only if missing.
Do not repeat unrelated passing checks or run local full suites. Prompt focused review
corrections warrant a corrected push without waiting for obsolete CI to finish.

## Compatibility verification

The empty-entry regression uses this RED before the helper guard, followed by all new caller regressions after the final production edit:

```bash
(cd apps/backend && GOMAXPROCS=2 go test -p 2 ./internal/agentctl/server/process -run '^TestGitOperatorLiteralSelections/(stage|unstage)/invalid-empty$' -count=1)
(cd apps/backend && GOMAXPROCS=2 go test -race -p 2 ./internal/agentctl/server/process -run '^(TestGitOperatorLiteralSelections|TestWorkspaceGitLiteralPatchSelection|TestWorkspaceGitLiteralCachedFallback)$' -count=1)
```

## Files likely touched

- `apps/backend/internal/agentctl/server/process/git.go`
- `apps/backend/internal/agentctl/server/process/git_pathspec.go` (new)
- `apps/backend/internal/agentctl/server/process/git_pathspec_test.go` (new)
- `apps/backend/internal/agentctl/server/process/workspace_git_diff.go`
- `apps/backend/internal/agentctl/server/process/workspace_git_literal_paths_test.go` (new)
- `apps/backend/internal/agentctl/server/process/workspace_git_status_progressive_test.go` (existing per-file fault injection selectors)
- `apps/backend/internal/agentctl/server/api/git_literal_paths_test.go` (new)
- `docs/public/git-operations.md`
- `docs/specs/platform/requirements/workspace-git-status.md`
- `docs/specs/platform/system-design/workspace-git-path-details.md`
- `docs/plans/git-literal-file-selections/plan.md`
- `docs/plans/git-literal-file-selections/task-01-literal-path-selections.md`

## Dependencies

None. Parent design review and a later explicit implementation continuation are the checkpoint.

## Risks

Tracked and untracked matching differ. Fixture construction must not use wildcard selectors.
Actual native filesystem restrictions determine skips; bracket names remain portable.
Literal representation must not leak into file keys, filesystem reads or generic query helpers.
Do not reinterpret passing Discard controls or change global Git environment behavior.

## Parallelism

`sequential`

## Inputs

- Plan evidence, scope and regression matrix.
- REQ-PLATFORM-WORKSPACE-GIT-STATUS-001 criteria listed above.
- Workspace Git path details: literal selected paths and preserved execution/quality contracts.
- Existing Git argument validation, admission helpers and API fixture patterns.
- Archived `/tmp/kandev-literal-git-paths-repro.go` (read-only inherited evidence).

## Results

Implementation authorized by the later parent continuation on 2026-10-02.

- Process RED command above: exit 1, package 5.564s, expected selected/unselected index and
  patch failures. HTTP RED (`go test -p 2 ./internal/agentctl/server/api -run
  '^TestHandleGitLiteralSelections$' -count=1`, `GOMAXPROCS=2`): exit 1, package 0.329s.
- Initial focused 15-function process race command: 13 functions passed; two stale injection
  selectors failed (package 9.984s). Updated three per-file fault selectors to literal argv,
  with no weakened assertion. The documented three-function injection race run passed 1.728s.
- Invalid-empty-entry RED above: exit 1, package 0.149s; both operations wrongly succeeded
  and changed sibling index bytes before the guard. Final new-caller race command above:
  exit 0, package 7.672s. All portable bracket cases remain enabled on Windows.
- API race command above after final production edit: exit 0, package 1.625s.
- Scoped lint first found an unchecked enrichment error in the new fallback test; fixed the
  test to assert the error result. Final scoped lint passed, zero issues, concurrency 2.
- No tautological concatenation test was added, as directed by the parent.
- No Discard, numstat/parser, API/UI/schema or global Git environment change.

Catalog validation (340 decisions, 1295 specifications), specification lint, public-doc
validation (47 pages), coverage preflight (`covered`, no errors), and whitespace checks passed.
Normal hook and external CI/review/merge evidence remains in the task plan; implementation
completion does not assert merge.


## Review remediation verification

Real caller environment regressions fail before the scoped environment correction:

```bash
(cd apps/backend && GOMAXPROCS=2 go test -p 2 ./internal/agentctl/server/process -run '^(TestGitOperatorLiteralPathspecEnvironment|TestWorkspaceGitLiteralPathspecEnvironment|TestWorkspaceGitLiteralCachedFallback)$' -count=1)
(cd apps/backend && GOMAXPROCS=2 go test -p 2 ./internal/agentctl/server/api -run '^TestHandleGitLiteralSelections/(stage|unstage)/literal-env-1$' -count=1)
```

After correction, all new/changed caller regressions and the affected streaming contracts run:

```bash
(cd apps/backend && GOMAXPROCS=2 go test -race -p 2 ./internal/agentctl/server/process -run '^(TestGitOperatorLiteralSelections|TestGitOperatorLiteralPathspecEnvironment|TestWorkspaceGitLiteralPatchSelection|TestWorkspaceGitLiteralPathspecEnvironment|TestWorkspaceGitLiteralCachedFallback|TestCapDiffOutput_Truncation|TestDiffBudgetAndCarryForwardHonorCancellation)$' -count=1)
(cd apps/backend && GOMAXPROCS=2 go test -race -p 2 ./internal/agentctl/server/process -run '^TestCapDiffOutputCancellationClosesReader$' -count=1)
(cd apps/backend && GOMAXPROCS=2 go test -race -p 2 ./internal/agentctl/server/api -run '^(TestHandleGitLiteralSelections|TestHandleGitStageAndUnstage)$' -count=1)
```

Follow with the same scoped lint, document/coverage/whitespace gates and normal hooks above.
No full-suite or passing unrelated checks are added.

Review remediation RED: process exit 1, package 0.412s; HTTP exit 1, package 0.260s.
Process caller GREEN: exit 0, package 7.722s, seven selected functions. The initial pattern
also named a nonexistent cancellation function; the actual cancellation contract was then
run by the exact name above and passed (package 1.059s). API GREEN: exit 0, package 1.984s. Scoped process/API lint passed with concurrency 2 and
zero issues. Catalog/specification/public-doc (47 pages)/coverage/whitespace gates passed.
Normal hooks and exact-head external delivery evidence remain in the task plan.
