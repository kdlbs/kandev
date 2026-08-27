---
created: 2026-08-27
status: in_progress
requirements:
  - REQ-INTEGRATIONS-GITHUB-AUTHENTICATION-001
system_design:
  - ../../specs/integrations/system-design/github-authentication-01.md
  - ../../specs/integrations/system-design/github-authentication-02.md
  - ../../specs/integrations/system-design/github-authentication-03.md
legacy_specs: []
---

# Fix Plan: Reconcile Origins for Prepared Workspaces

Issue: [#3070](https://github.com/kdlbs/kandev/issues/3070)

## Overview

Every launch or resume must prepare each attached repository before an agent starts. The repair
moves the existing repository-resolution pass before the prepared-workspace branch. The full
launch path then reuses the same result.

This order closes the fast-path gap without another repository pass. The launch-order work is
implemented, but the plan remains in progress until #3069's dynamic, host-aware Git protocol
resolution lands and this change is rebased onto it.

## Confirmed root cause

`LaunchPreparedSession` checks `HasExecutorRunningRow` before it calls
`resolveAllRepoInfoForSession`. A running row can route the request through
`startAgentOnExistingWorkspace`, which returns before repository resolution.

`resolveAllRepoInfoForSession` reaches `ensureRepoLocalPathForSession` and
`reconcileGitHubCheckoutOrigin`. Therefore, the prepared-workspace branch skips origin convergence
after the workspace policy or detected protocol changes.

The smallest reproduction seeds a prepared session with an `executors_running` row and an
in-memory execution. An attached managed checkout has a noncanonical origin. A call to
`LaunchPreparedSession` starts the agent without changing that origin.

## Scope

### In scope

- Run the existing repository-resolution pass before the prepared-workspace branch.
- Reuse that result if the request falls through to the full launch path.
- Reconcile every attached managed GitHub checkout once for each launch operation.
- Fail the launch before agent start when repository preparation fails.
- Add regression coverage at the real `LaunchPreparedSession` fast-path boundary.

### Out of scope

- Dynamic host-aware Git protocol detection, which belongs to #3069.
- Host `gh` credential bridging for HTTPS origins, which belongs to #3072.
- Changes to GitHub credential precedence, checkout ownership, or user-managed local repositories.
- Frontend, persistence, API, migration, and public documentation changes.

## Technical approach

### Prepared launch ordering

In `apps/backend/internal/orchestrator/executor/executor_execute.go`, move the single
`resolveAllRepoInfoForSession` call above the `HasExecutorRunningRow` decision in
`LaunchPreparedSession`.

The existing-workspace path needs the preparation side effects before it returns or starts the
agent. If that path falls through with `ErrStaleExecution` or `ErrAgentCommandMissing`, the full
path must reuse the resolved slice. It must not resolve the repositories again.

The resolver remains the only origin-reconciliation entry point. Do not add another origin update
to `configureExistingWorkspace` or duplicate policy resolution.

### Coordination with #3069

Implement this work after #3069 lands or rebase it onto the #3069 result. Executor-mode origin
selection must call the dynamic, host-aware protocol seam from #3069. This work must not add a
second protocol cache, detector, or host lookup.

## Tests

- `AC-INTEGRATIONS-GITHUB-AUTHENTICATION-001.9`: add
  `TestLaunchPreparedSession_ExistingWorkspace_ReconcilesGitHubOriginsBeforeAgentStart` in
  `apps/backend/internal/orchestrator/executor/executor_resume_clone_transport_test.go`.
- Exercise `LaunchPreparedSession` with a running row and an in-memory execution. Do not call the
  repository helper directly.
- Attach two managed GitHub repositories. Give one a stale origin and the other its canonical
  origin.
- Use real Git repositories and the real origin-update behavior. Lock the canonical repository's
  Git configuration so an attempted rewrite fails.
- Wrap the real cloner to count `SetOriginURL` calls. At agent start, assert that both origins are
  canonical and each managed repository received one call for this launch.
- Keep `TestEnsureRepoLocalPath_DoesNotRewriteUserManagedOrigin` as coverage for
  `AC-INTEGRATIONS-GITHUB-AUTHENTICATION-001.10` and the local-checkout exclusion.

## Work orders

- [x] [Task 01: Reconcile prepared-workspace origins](task-01-reconcile-prepared-workspace-origins.md)

## Verification results

Implemented the launch-order correction and the real prepared-workspace regression. The focused
regression passed, followed by the complete executor and repoclone package command:

```text
go test -tags fts5 ./internal/orchestrator/executor -run 'TestLaunchPreparedSession_ExistingWorkspace_ReconcilesGitHubOriginsBeforeAgentStart$' -count=1
Go test: 1 passed in 1 packages

go test -tags fts5 ./internal/orchestrator/executor ./internal/repoclone
Go test: 620 passed in 2 packages
```

`python3 scripts/lint-spec-files.py --all` also passed. Issue #3069 remains open, so the
executor-mode transport prerequisite is not yet satisfied. This change intentionally consumes
the existing `RepoCloner.BuildCloneURLWithHost` seam and adds no protocol detection; rebase this
plan and its implementation after #3069 lands before marking the plan complete.

## Risks

- #3069 can change the `RepoCloner` protocol-resolution seam before implementation starts. Rebase
  first and adapt the regression fixture to the landed interface.
- Repository preparation can now stop an existing-workspace launch before agent start. This is the
  required fail-closed behavior, but it makes stale checkout ownership and Git errors visible on
  this path.
- A second resolution call can cause repeated work for multi-repository tasks. The regression
  must count preparation per repository.
