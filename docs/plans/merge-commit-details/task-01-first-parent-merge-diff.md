---
id: "01-first-parent-merge-diff"
title: "First-parent merge commit diff"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/merge-commit-details.md"
---

# Task 01: First-parent merge commit diff

- **Acceptance:** `GitOperator.ShowCommit` returns the first-parent file map,
  patch, status, additions, and deletions for a clean two-parent merge; it does
  not include a feature-only file already present in the first parent; existing
  single-commit behavior and the `CommitDiffResult` contract remain unchanged.
- **Verification:** Follow strict TDD. Add the regression test and first run
  `cd apps/backend && go test -v -run '^TestShowCommit_MergeCommitUsesFirstParentDiff$' ./internal/agentctl/server/process`;
  confirm it fails because the merge result has no files. After the minimal
  fix, rerun that command, then run
  `cd apps/backend && go test ./internal/agentctl/server/process` and
  `git diff --check` from the repository root.
- **Files likely touched:**
  `apps/backend/internal/agentctl/server/process/git_log.go`,
  `apps/backend/internal/agentctl/server/process/git_test.go`, this task file,
  and `plan.md` for status/results synchronization.
- **Dependencies:** None.
- **Parallelism:** sequential — the test and implementation change the same Go
  package and must be completed as one Red-Green-Refactor cycle.
- **Inputs:** repair spec desired behavior and regression scenarios;
  `GitOperator.ShowCommit` and `parseCommitDiff` in `git_log.go`; existing
  `setupTestRepo`, `runGit`, `TestParseCommitDiff_PathsWithSpaces`, and
  `TestShowCommit_NotCapped` patterns.
- **Output contract:** Report the root-cause-preserving minimal change, exact
  files changed, RED and GREEN command results, process-package result,
  `git diff --check`, remaining risks, and synchronized task/plan statuses.

## Results

- RED: `rtk go test -v -run '^TestShowCommit_MergeCommitUsesFirstParentDiff$'
  ./internal/agentctl/server/process` failed with the expected assertion:
  `merge diff missing incoming first-parent change; files=[]`.
- GREEN: the same focused command passed with 1 test passed.
- Full package: `rtk go test ./internal/agentctl/server/process` passed with 550
  tests in 1 package (exit code 0).
- Hygiene: `git diff --check` passed. `gofmt` ran on the two Go files. Test
  repositories were temporary and cleaned by `t.Cleanup`; no external state,
  browser session, or live instance was modified.
