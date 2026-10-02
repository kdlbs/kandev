---
id: "01-preserve-numstat-paths"
title: "Preserve workspace numstat paths"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-WORKSPACE-GIT-STATUS-001
acceptance_criteria:
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.7
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.9
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.31
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.33
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.36
system_design:
  - ../../specs/platform/system-design/workspace-git-status.md
  - ../../specs/platform/system-design/workspace-git-path-details.md
---

# Task 01: Preserve workspace numstat paths

## Summary

Use Git's NUL-framed numstat records to associate workspace details with exact status keys.
Prove the correction through focused public tracker regressions before minimal production changes.

## In scope

- Permanent RED for the six original real-tracker cases, then staged/unstaged/mixed special paths,
  actual rename destinations, binary rows, and no-content changes as listed in the plan.
- One shared NUL parser and the three path-bearing numstat callers; cursor consumption of actual old/new records.
- Exact affected checks, normal hooks, and synchronized results in this package.

## Out of scope

UI, schema, API, watcher/porcelain/history redesign, branch totals, extra workers, broad local suites,
and speculative diagnostic or polish work.

## Acceptance

1. Permanent `TestWorkspaceGitDetailsPreservesExactPaths` fails on the existing implementation for
   the parent's original six cases; all new parser/tracker tests pass after the fix.
2. All three callers use NUL records; actual rename destinations match membership keys, mixed layer
   counts/patches remain independent, and legitimate zero-count rows remain usable.
3. Existing affected state, cancellation, budget, carry-forward, and history text-parser checks pass;
   command arguments, comparison identity, and existing API shape remain compatible.

## Verification

From repo root, run RED first before production edits:

```bash
(cd apps/backend && GOMAXPROCS=2 go test -p 2 ./internal/agentctl/server/process -run '^TestWorkspaceGitDetailsPreservesExactPaths$' -count=1)
```

After implementation, run this single focused Go selection once:

```bash
(cd apps/backend && GOMAXPROCS=2 go test -race -p 2 ./internal/agentctl/server/process -run '^(TestParseWorkspaceNumstatZ|TestWorkspaceGitDetailsPreservesExactPaths|TestWorkspaceGitDetailsPreservesExactMixedPaths|TestWorkspaceGitDetailsPreservesRenameDestinations|TestWorkspaceGitDetailsPreservesBinaryPaths|TestWorkspaceGitStatusPreservesMixedChangeFacets|TestWorkspaceGitStatusPreservesAddedMixedChangeFacets|TestMixedChangeFacetDiffBudgetCountsAllRepresentations|TestCarryForwardMixedChangeFacets|TestCarryForwardMixedChangeFacetsRespectsBudget|TestEnrichStagedDiff_RenamedFile|TestResolveNumstatPath|TestNumstatByPath|TestEnrichWithDiffData_CanceledContextReturnsErrorForTrackedFiles|TestDiffBudgetAndCarryForwardHonorCancellation|TestCarryForwardFileDiffs|TestCarryForwardFileDiff|TestWorkspaceTrackerRetriesOnlyFailedDiffAfterTransientGitFailure|TestWorkspaceTrackerOnlyRetriesUnavailableDetailsOnExplicitRefresh)$' -count=1)
(cd apps/backend && GOMAXPROCS=2 golangci-lint run ./internal/agentctl/server/process/... --concurrency=2 --new-from-rev=6979c7642d2e73f6c765155a417713c6c05ac088 --timeout=5m)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run the repository documentation-coverage preflight from root on the complete linked package:

```bash
PATH="/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:$PATH" node <<'JS'
const fs = require('node:fs');
const {validateCoverage} = require('./.github/scripts/pr-docs.cjs');
const docs = [
  'docs/plans/workspace-numstat-exact-paths/plan.md',
  'docs/plans/workspace-numstat-exact-paths/task-01-preserve-numstat-paths.md',
  'docs/specs/platform/requirements/workspace-git-status.md',
  'docs/specs/platform/system-design/workspace-git-status.md',
  'docs/specs/platform/system-design/workspace-git-path-details.md',
];
const result = validateCoverage({
  changedFiles: [...docs, 'apps/backend/internal/agentctl/server/process/workspace_git_diff.go'],
  fileContents: Object.fromEntries(docs.map(p => [p, fs.readFileSync(p, 'utf8')])),
});
console.log(JSON.stringify({ok: result.ok, status: result.status, errors: result.errors}));
process.exitCode = result.ok ? 0 : 1;
JS
```

Invoke repository commit hooks normally. Read the delivery skills
at that phase. If hook setup needs pnpm, install from `apps/` with the frozen lockfile once.
Do not expand this work order to broad backend/E2E audits.

## Files likely touched

- `apps/backend/internal/agentctl/server/process/workspace_git_diff.go`
- `apps/backend/internal/agentctl/server/process/workspace_git_numstat.go` (new)
- `apps/backend/internal/agentctl/server/process/workspace_git_numstat_test.go` (new)
- `apps/backend/internal/agentctl/server/process/workspace_git_diff_paths_test.go` (new)
- Requirement/design and this plan/work order for finalized intent/status/results.

## Dependencies

Parent implementation continuation received 2026-10-02. No prerequisite work order.

## Risks

Native Windows filename restrictions, Git rename similarity thresholds, and accidental changes to
the shared history text parser. Scope only invalid real-filesystem cases by platform. Preserve raw
parser coverage on all platforms. Keep parser iteration linear and cancellable.

## Parallelism

`sequential`. No additional workers/tasks/sessions.

## Inputs

- [Requirement](../../specs/platform/requirements/workspace-git-status.md), especially `.9` and `.36`.
- [Path design](../../specs/platform/system-design/workspace-git-path-details.md).
- Existing `workspace_git_diff.go`, `workspace_git_mixed_changes_test.go`, `git_log.go`
  and `git_log_diffstats_test.go` patterns.
- Parent fixture `/tmp/kandev-workspace-numstat-paths-repro.go`; plan's Git 2.43.0 framing evidence.

## Results

Completed 2026-10-02 in this session.

- Narrow permanent RED: six expected failing cases, package 0.676s, exit 1.
- Focused GREEN selection above with race checking: exit 0, package 7.110s. All 19 selected test names were verified against actual source before delivery.
- Scoped lint above: exit 0, zero issues.
- Catalog validation: exit 0 (340 decisions, 1295 specifications).
- Specification lint and `git diff --check`: exit 0.
- Documentation-coverage preflight above: covered, zero errors.
- Native Windows skips only invalid real filenames; all raw parser cases are platform independent.

Commit hook receipts and external PR/CI/review/merge evidence belong in the task plan. No broad local suite, E2E run, or additional worker was used.
