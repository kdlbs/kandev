---
id: "01-git-operator-is-ancestor"
title: "Add GitOperator.IsAncestor helper"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/platform/requirements/workspace-git-status.md"
---

# Task 01: Add `GitOperator.IsAncestor` helper

Add a thin `GitOperator` method that answers "is A a strict ancestor of B?" so the
commits-panel base resolver (task 02) can detect a stale stored base without shelling out
inline.

## Context / Inputs

- Spec: `docs/specs/platform/requirements/workspace-git-status.md` → "Base-commit staleness and refresh"
  (a stored base is stale when it is a strict ancestor of `merge-base(HEAD, origin/<base_branch>)`).
- Existing helpers to mirror for style/error handling: `GetMergeBase` and `GetRevParse` in
  `apps/backend/internal/agentctl/server/process/git_log.go` (both use `runGitCommand`).
- `git merge-base --is-ancestor A B` exits 0 (true), 1 (false), or >1 (real error). The current
  `runGitCommand` returns a non-nil error on any non-zero exit, so this helper MUST inspect the
  exit code to distinguish "false" (exit 1) from a genuine failure.

## Implementation

- Add to `apps/backend/internal/agentctl/server/process/git_log.go`:
  ```go
  // IsAncestor reports whether `ancestor` is a strict ancestor of `descendant`
  // (git merge-base --is-ancestor). Exit 0 => true, exit 1 => false, anything
  // else => error. Note: git treats a commit as an ancestor of itself, so an
  // equal SHA returns true; callers wanting a STRICT check compare SHAs first.
  func (g *GitOperator) IsAncestor(ctx context.Context, ancestor, descendant string) (bool, error)
  ```
- Use the lowest-level command runner that exposes exit status (check how `runGitCommand`
  surfaces `*exec.ExitError`; if it collapses exit codes, add a sibling that returns the
  exit code, or use `errors.As` on the returned error to read `ExitError.ExitCode()`).
- Empty `ancestor` or `descendant` returns `(false, nil)` without invoking git.

## Acceptance

- `IsAncestor` returns `(true, nil)` for a real ancestor, `(false, nil)` for a
  non-ancestor / diverged commit, and a non-nil error only for genuine git failures
  (bad repo, unknown SHA that git reports as error).
- Equal SHAs return `(true, nil)` (documented; strictness is enforced by the caller).

## Verification

```shell
cd apps/backend && go test -run 'IsAncestor' ./internal/agentctl/server/process/...
```

Add a table-driven test in
`apps/backend/internal/agentctl/server/process/git_log_test.go` using `setupTestRepo`
covering: ancestor, non-ancestor (sibling branch), equal SHA, and unknown-SHA error.

## Files likely touched

- `apps/backend/internal/agentctl/server/process/git_log.go`
- `apps/backend/internal/agentctl/server/process/git_log_test.go`

## Dependencies

None.

## Parallelism

`sequential`. (Task 02 imports this helper.)

## Output contract

Report: helper signature added, exit-code handling approach, test cases, `go test` result,
any blockers. Update this file's `status` to `in_progress` at start and `done` at finish, and
tick the task-01 checkbox in `plan.md`.
