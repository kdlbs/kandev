---
id: "02-stale-base-correction"
title: "Correct stale commits-panel base to the integration merge-base"
status: done
wave: 2
depends_on: ["01-git-operator-is-ancestor"]
plan: "plan.md"
spec: "../../specs/platform/requirements/workspace-git-status.md"
---

# Task 02: Correct stale commits-panel base to the integration merge-base

When the resolved base commit (from the stored target branch) is a strict ancestor of the
integration merge-base (`origin/main` → `origin/master`), anchor the commits panel and
cumulative diff to the integration merge-base instead. Fixes the inflated 31-vs-1 commit count
in the stacked-PR / merged-parent case.

## Context / Inputs

- Spec: `docs/specs/platform/requirements/workspace-git-status.md` → "Base-commit staleness and refresh",
  Failure modes rows, and Scenarios 1–4.
- Confirmed root cause (see `plan.md`): stored target = merged/deleted stacked parent →
  `computeMergeBase` falls back to stale local ref (`ed630b8446`, 31 commits) instead of
  `merge-base(HEAD, origin/master)` (`19646efc83`, 1 commit).
- Seam: `runGitLogForRepo` and `computeMergeBase` in
  `apps/backend/internal/agentctl/server/api/git.go` (already prefers `origin/<target>`).
- Priority list to mirror: `branchDiffCandidates` in
  `apps/backend/internal/agentctl/server/process/workspace_git_status.go`
  (`origin/main → origin/master → main → master`). For the merge-base, `computeMergeBase`
  already tries `origin/<name>` first, so pass bare `main`/`master`.
- Depends on `GitOperator.IsAncestor` from task 01.

## Implementation

In `apps/backend/internal/agentctl/server/api/git.go`:

- Add `var integrationMergeBaseCandidates = []string{"main", "master"}`.
- Add `func (s *Server) integrationMergeBase(ctx context.Context, gitOp *process.GitOperator) string`:
  first non-empty `computeMergeBase(HEAD, candidate)` over the candidates; "" if none resolve.
- Add `func (s *Server) correctStaleBase(ctx, gitOp, baseCommit, targetBranch string) string`:
  - Return `baseCommit` unchanged when `targetBranch` is empty or is itself an integration
    candidate (`main`/`master`, with or without `origin/` prefix) — avoid redundant self-compare.
  - Compute `integ := s.integrationMergeBase(...)`; return `baseCommit` if `integ == ""` or
    `integ == baseCommit`.
  - `anc, err := gitOp.IsAncestor(ctx, baseCommit, integ)`; if `err == nil && anc && baseCommit != integ`
    return `integ` (stale base is strictly behind the integration line → correct it). Otherwise
    return `baseCommit` (equal/descendant/diverged/errored → unchanged).
- In `runGitLogForRepo`, after the existing block sets `baseCommit` from the target-branch
  merge-base (and before `gitOp.GetLog`), set
  `baseCommit = s.correctStaleBase(c.Request.Context(), gitOp, baseCommit, req.TargetBranch)`.
  Preserve the existing inline `safeBranchRefPattern` sanitiser barriers; `main`/`master`
  literals are constant and safe.
- Keep `runGitLogForRepo` within golangci limits (≤80 lines / ≤50 statements / complexity ≤15) —
  the logic lives in the extracted helpers above.

Area 2 (cumulative diff parity): confirm whether the live cumulative-diff handler shares this
base resolution. If it computes its own base against the target branch, apply the same
`correctStaleBase` call there. Do NOT touch `lifecycleAdapter.GetCumulativeDiff`
(orchestrator snapshot path anchors to a caller-provided SHA by design).

## Acceptance

- Regression: stale stacked-parent base is replaced by `merge-base(HEAD, origin/main)`; commits
  panel count equals `git rev-list --first-parent --count <integ-merge-base>..HEAD` (1 in repro).
- A base equal to / descendant of the integration merge-base is returned unchanged.
- No `origin/*` and unrelated history: no error, existing fallback preserved.
- `runGitLogForRepo` passes the changed-file linter at the PR base SHA.

## Verification

```shell
cd apps/backend && go test -run 'TestComputeMergeBase|TestRunGitLogForRepo|TestGetLog' ./internal/agentctl/server/api/...
cd apps/backend && golangci-lint run ./internal/agentctl/server/api/... --new-from-rev="<base-sha>" --timeout=5m
```

Regression tests go in
`apps/backend/internal/agentctl/server/api/git_log_merge_base_test.go` (extend, reusing
`setupAPITestRepo`/`runGitAPI`/`writeFileAPI`). Write the stale-parent test FIRST and confirm it
fails before the code change.

## Files likely touched

- `apps/backend/internal/agentctl/server/api/git.go`
- `apps/backend/internal/agentctl/server/api/git_log_merge_base_test.go`
- (only if Area 2 requires it) `apps/backend/internal/agentctl/server/process/workspace_git_status.go`

## Dependencies

Task 01 (`GitOperator.IsAncestor`).

## Parallelism

`sequential`.

## Output contract

Report: helpers added, exact insertion point in `runGitLogForRepo`, Area 2 finding
(shared vs separate cumulative-diff base), regression test red→green evidence, `go test` and
changed-file lint results, blockers/risks. Update this file's `status` and the task-02 checkbox
in `plan.md`.
