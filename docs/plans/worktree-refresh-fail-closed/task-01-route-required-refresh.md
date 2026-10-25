---
id: "01-route-required-refresh"
title: "Route required repository refresh"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-WORKTREE-BASE-REFRESH-001
acceptance_criteria:
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.2
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.3
system_design:
  - ../../specs/workspaces/system-design/worktree-base-refresh.md
---

# Task 01: Route Required Repository Refresh

## Summary

Choose the strict refresh boundary from repository provider and task Git
credential policy. Preserve executor-inherited SSH origins and use exact-scope
backend credentials only for managed provider routes.

## In scope

- Add failing tests for GitHub managed mode, GitHub executor mode, plugin
  providers, GitLab, and Azure DevOps.
- Resolve the route before lifecycle preparation in initial launch and resume;
  defer a managed refresh until the lifecycle actually materializes or
  recreates a worktree so valid reuse performs no refresh.
- Generalize the existing plugin strict-refresh helper into a
  provider-credential-aware helper.
- Reuse the clone credential path for strict GitLab and Azure DevOps refresh.
- Set `RemoteSyncHandled` only after the deferred strict provider refresh
  succeeds.
- Leave `PullBeforeWorktree` enabled for executor-inherited and local routes so
  the worktree manager owns their fetch.
- Preserve the SSH origin selected by `gitHubCheckoutOriginURL` in executor
  mode.

## Out of scope

- Worktree fallback and ancestry behavior.
- Task error projection or frontend changes.
- New credential sources, provider settings, or schema.
- Changing repository clone behavior when no existing checkout is present.

## Acceptance

- A managed provider checkout runs one authenticated strict refresh when a
  worktree is materialized or recreated, and does not run an unauthenticated
  follow-up fetch. A valid reusable worktree bypasses the refresh.
- An executor-inherited GitHub checkout retains its reconciled origin and
  delegates required fetch to the worktree manager.
- A strict provider refresh error returns from initial launch and resume with
  exact task, session, and repository scope.

## Verification

Start with a regression that proves a managed first-party checkout stops after
its authenticated refresh fails. Confirm that it fails before the production
change. Then run:

```bash
# From apps/backend:
rtk go test ./internal/repoclone ./internal/orchestrator/executor -run 'RefreshWorkspaceRepository|ResolveTaskRepoInfo|CloneTransport' -race
rtk go test ./internal/orchestrator/executor/... -run 'Fresh|Resume|Repository' -race
```

## Files likely touched

- `apps/backend/internal/orchestrator/executor/executor.go`
- `apps/backend/internal/orchestrator/executor/executor_resume.go`
- `apps/backend/internal/orchestrator/executor/executor_resume_clone_transport_test.go`
- `apps/backend/internal/repoclone/clone.go`
- `apps/backend/internal/repoclone/clone_auth_test.go`

## Dependencies

None.

## Risks

- Provider-specific clone credentials can drift from refresh credentials. Use
  the same resolver and redaction path for both operations.
- Executor mode must not call a backend-managed GitHub refresh because that can
  rewrite the SSH origin to HTTPS.
- Exact credential scope must include task ID, session ID, and repository ID.

## Parallelism

`sequential`

## Inputs

- `REQ-WORKSPACES-WORKTREE-BASE-REFRESH-001`.
- `docs/specs/workspaces/system-design/worktree-base-refresh.md`.
- `ADR-2026-07-27-task-git-credential-policy`.
- Existing plugin strict-refresh tests and GitHub origin-reconciliation tests.

## Results

- Added managed-provider refresh routing for GitHub, GitLab, Azure DevOps, and
  plugin repositories while preserving executor-inherited GitHub transport.
- Deferred managed refresh until worktree materialization or recreation, while
  preserving exact repository identity through multi-repository preparation and
  bypassing refresh for valid worktree reuse.
- Added authenticated pull-request-head fetching into `origin/pr/<N>` for
  managed checkouts, including fork pull requests.
- Added strict Azure DevOps basic-auth refresh support that reuses the clone
  credential boundary and validates the exact workspace checkout path.
- Verified with:
  - `rtk go test ./internal/repoclone ./internal/orchestrator/executor -run 'RefreshWorkspaceRepository|ResolveTaskRepoInfo|CloneTransport' -race`
  - `rtk go test ./internal/orchestrator/executor/... -run 'Fresh|Resume|Repository' -race`
