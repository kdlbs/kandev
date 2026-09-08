---
id: "02-log-base-branch-fallback"
title: "Make the integration-branch fallback observable"
status: done
wave: 2
parallelism: sequential
depends_on: ["01-push-base-branches-every-workspace"]
plan: "plan.md"
spec: "../../specs/workspaces/requirements/workspace-base-branch-propagation.md"
---

# Task 02: Make the integration-branch fallback observable

## Acceptance

- When `resolveBaseBranch` falls through to `branchDiffCandidates`, it emits one
  log line naming the repository (tracker's repository name) and the candidate
  it selected.
- The line distinguishes the two reasons it can fall through: no stored base
  branch was present, versus a stored base branch that failed to verify in git.
  Those have different fixes, so they must be told apart from the log alone.
- No DB read is added to decide whether a base branch "should" have been
  present — the tracker stays a pure git/state component.
- The line is cheap enough for a status poll and does not spam once per second
  at info level; pick a level and/or de-duplication consistent with the
  surrounding poll logging.
- No behavior change: the resolution result is identical, only observability is
  added.

## Regression test

- Assert the fallback path is reached and reports the selected candidate when no
  stored base branch is set.
- Assert the stored-but-unverifiable case is distinguishable from the
  never-stored case.
- Extend `internal/agentctl/server/process/workspace_git_status_base_branch_test.go`,
  which already covers stored-vs-fallback resolution.

## Verification

Recorded against `4b52037` (the merge-base with `main` at the time of the run),
which is the revision passed to `--new-from-rev`.

```bash
(cd apps/backend && go test ./internal/agentctl/server/process/... -race)
```

PASS — `ok github.com/kandev/kandev/internal/agentctl/server/process 90.732s`.

```bash
(cd apps/backend && golangci-lint run ./... --new-from-rev=4b52037 --timeout=5m)
```

PASS — `0 issues.`

## Files Likely Touched

- `apps/backend/internal/agentctl/server/process/workspace_git_status.go`
- `apps/backend/internal/agentctl/server/process/workspace_git_status_base_branch_test.go`

## Dependencies

Task 01 — same resolution path; keep the edits ordered.

## Inputs

- Spec bullet: a silent fallback that yields a wrong-but-plausible number is not
  acceptable.
- `resolveBaseBranch` / `resolveStoredRef`
  (`workspace_git_status.go`), including the existing `Debug` line in
  `computeBaseCommit`'s neighbours for level/tone consistency.

## Output Contract

`resolveBaseBranch` behavior is unchanged; a fallback is now attributable from
logs alone.
