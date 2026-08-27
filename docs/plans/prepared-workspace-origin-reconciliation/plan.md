---
created: 2026-08-27
status: done
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
implemented on top of #3069's dynamic, host-aware Git protocol resolution from PR #3078.

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

PR #3078, originally headed by commit `b976260e69b5abf421cfa9742dfc5c37d533ac0f`, is now
integrated into `main` as `e4d17b3925`. It provides the dynamic, host-aware protocol resolver and
context-aware `RepoCloner` contract. Executor-mode origin selection uses that seam. This work adds
no second protocol cache, detector, or host lookup.

## Tests

- `AC-INTEGRATIONS-GITHUB-AUTHENTICATION-001.11`: add
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
  `AC-INTEGRATIONS-GITHUB-AUTHENTICATION-001.12` and the local-checkout exclusion.

## Work orders

- [x] [Task 01: Reconcile prepared-workspace origins](task-01-reconcile-prepared-workspace-origins.md)

## Verification results

Implemented the launch-order correction and the real prepared-workspace regression on top of the
#3069 resolver contract. The focused regression passed, followed by the complete executor and
repoclone package command:

```text
go test -tags fts5 ./internal/orchestrator/executor -run 'TestLaunchPreparedSession_ExistingWorkspace_ReconcilesGitHubOriginsBeforeAgentStart$' -count=1
Go test: 1 passed in 1 packages

go test -tags fts5 ./internal/orchestrator/executor ./internal/repoclone
Go test: 620 passed in 2 packages
```

The dynamic protocol reevaluation regression from #3069 also passed:

```text
go test -tags fts5 ./internal/orchestrator/executor -run 'TestEnsureRepoLocalPathReevaluatesGitHubProtocol$' -count=1
Go test: 1 passed in 1 packages
```

`python3 scripts/lint-spec-files.py --all` also passed. PR #3078 supplies the dynamic,
host-aware protocol resolver and context-aware `RepoCloner.BuildCloneURLWithHost` contract; it is
merged into `main` as `e4d17b3925`, and this branch adds no protocol detection.

## Risks

- The context-aware protocol-resolution seam from #3069 is now integrated. Future changes to that
  contract must update this regression fixture and its adapter together.
- Repository preparation can now stop an existing-workspace launch before agent start. This is the
  required fail-closed behavior, but it makes stale checkout ownership and Git errors visible on
  this path.
- A second resolution call can cause repeated work for multi-repository tasks. The regression
  must count preparation per repository.
