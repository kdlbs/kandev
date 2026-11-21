---
id: "01-bare-root-details-wait"
title: "Settle details-wait on repository-less trackers"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-WORKSPACE-GIT-STATUS-001
acceptance_criteria:
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.21
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.26
system_design:
  - ../../specs/platform/system-design/workspace-git-status.md
---

# Task 01: Settle details-wait on repository-less trackers

## Summary

A `details=wait` status read on a tracker without a Git repository, such as a
bare multi-repository task root, returns the settled `repository_unavailable`
result. It does not return `errGitStatusDetailsUnavailable` and does not log
`git status failed` at ERROR.

## In scope

- `WorkspaceTracker.GetGitStatusWithDetails`: return the observed status with a
  nil error when `StatusState == gitStatusStateUnavailable` and
  `ErrorCode == gitStatusErrorNoRepository`. Apply the check right after the
  initial read, before the `DetailState` checks.
- Both agentctl routes that call it (`handleGitStatus` and the per-repository
  multi-status helper in `server/api/git.go`) need no change. They serialize
  the result through `gitStatusResult`, which keeps `success=false`.

## Out of scope

Backend callers requesting `repo=<name>`, per-repository fan-out, changing log
levels for other failures (including `context_canceled`), and launcher log
field forwarding (#5292).

## Acceptance

1. On a bare multi-repository root, `GetGitStatusWithDetails(ctx, true)` and
   `(ctx, false)` return `StatusState=unavailable`,
   `ErrorCode=repository_unavailable`, `DetailState=unavailable`, and a nil error.
2. `GET /api/v1/git/status?fresh=true&details=wait` on that root returns HTTP 200
   with `success=false`, `status_state=unavailable`, and
   `error_code=repository_unavailable`. It records no ERROR-level log.
3. A real repository whose detail job fails or is superseded still returns
   `errGitStatusDetailsUnavailable` or `errGitStatusEvidenceChanged`. The
   existing progressive and ordering tests continue to pass unchanged.

## Verification

Write both regressions first and record the expected failure
(`git status details unavailable` and one ERROR record). From the repository root:

```bash
(cd apps/backend && go test -trimpath ./internal/agentctl/server/process -run 'GitStatusDetails|BareRoot' -count=1)
(cd apps/backend && go test -trimpath ./internal/agentctl/server/api -run 'GitStatus|DetailsWait|MultiRepo' -count=1)
(cd apps/backend && go test -trimpath -race ./internal/agentctl/server/process ./internal/agentctl/server/api -count=1)
bash scripts/lint-go-changed \
  apps/backend/internal/agentctl/server/process/workspace_git_status.go \
  apps/backend/internal/agentctl/server/process/workspace_git_status_bare_root_test.go \
  apps/backend/internal/agentctl/server/api/git_status_bare_root_test.go
```

## Files likely touched

- `apps/backend/internal/agentctl/server/process/workspace_git_status.go`
- `apps/backend/internal/agentctl/server/process/workspace_git_status_bare_root_test.go` (new)
- `apps/backend/internal/agentctl/server/api/git_status_bare_root_test.go` (new;
  uses `newMultiRepoStatusServer` from `git_status_helpers_test.go` and a
  `zaptest/observer` core on the server logger)

## Dependencies

None.

## Risks

Match the exact no-repository code, not every unavailable status, so real
repository capture failures keep their error path and diagnostics.

## Parallelism

`sequential`

## Inputs

- [Plan](plan.md), #5398 root cause.
- [Workspace Git status design](../../specs/platform/system-design/workspace-git-status.md), status routes.

## Results

Implemented and verified. The process and API regressions pass, including the
full race run for both packages:

```bash
(cd apps/backend && go test -trimpath ./internal/agentctl/server/process -run 'GitStatusDetails|BareRoot' -count=1)
(cd apps/backend && go test -trimpath ./internal/agentctl/server/api -run 'GitStatus|DetailsWait|MultiRepo' -count=1)
(cd apps/backend && go test -trimpath -race ./internal/agentctl/server/process ./internal/agentctl/server/api -count=1)
```
