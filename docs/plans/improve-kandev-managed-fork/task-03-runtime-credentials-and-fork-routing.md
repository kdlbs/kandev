---
id: "03-runtime-credentials-and-fork-routing"
title: "Runtime credentials and fork routing"
status: complete
wave: 3
depends_on: ["02-creation-preparation-and-bootstrap"]
plan: "plan.md"
spec: "../../specs/workspaces/requirements/improve-kandev.md"
---

# Task 03: Runtime Credentials and Fork Routing

## Acceptance

- Every launch/resume path loads the validated destination, preserves canonical `origin`, adds the stable
  fork remote, and routes the task's current branch to the same branch name on that fork.
- Managed sessions receive the canonical lease plus one exact destination lease, and the broker rejects all
  unrelated, malformed, cross-task, cross-session, and cross-workspace requests. The destination lease
  proves source, target, and parent provider IDs and the bound credential connection/generation against
  live provider state.
- A policy change to executor-owned access, or an explicit executor `GH_TOKEN`/`GITHUB_TOKEN`, clears the
  server-managed destination and does not issue a managed destination lease. Changing the workspace
  automation connection or its generation invalidates the old binding.
- Raw `git push` and agentctl push use the fork destination; base fetches and change-request targeting remain
  canonical. Existing ordinary and already-open remote-contribution tasks do not regress.

## Verification

```bash
cd apps/backend
rtk go test ./internal/backendapp ./internal/orchestrator/executor ./internal/agent/runtime/lifecycle ./internal/agentctl/server/process -run 'Test.*(ContributionDestination|ForkRouting|BrokerScope|ReconcileGitHubOrigin)'
```

## Files likely touched

- `apps/backend/internal/orchestrator/executor/executor_resume.go`
- `apps/backend/internal/orchestrator/executor/executor_credentials.go`
- `apps/backend/internal/orchestrator/executor/executor_execute.go`
- focused executor resolution and credential tests
- `apps/backend/internal/backendapp/services.go`
- `apps/backend/internal/backendapp/services_github_broker_test.go`
- lifecycle request/config/materialization files and tests for Worktree, Local, Docker, SSH, and Sprites
- `apps/backend/internal/agentctl/server/process/git.go`
- `apps/backend/internal/agentctl/server/process/git_test.go`
- agentctl workspace configuration and tracker tests

## Dependencies

Task 02's persisted, server-authored destination on the canonical attachment.

## Parallelism

Sequential security and runtime slice. Credential authorization must land with routing so no accepted scope
is unused and no enabled push path lacks authorization.

## Inputs

- Spec: canonical origin, exact managed scope, restart reconstruction, and local-row exemption.
- ADRs: task-bound destination plus existing remote-contribution remote/lease pattern.
- Existing patterns: `issueGitHubCredentialScopes`, `githubBrokerScopeAuthorizer`, contribution setup
  scripts, `GitOperator` explicit remote/branch fields, and managed-origin reconciliation.

## Risks

- A valid metadata map alone does not prove task/session ownership; retain every existing broker ownership
  check before matching the destination.
- Shared Git repositories/worktrees need collision-resistant remotes and identity checks before reuse.
- Preserve the local checkout exemption. Only Kandev-managed provider clones are reconciled.
- Do not alter `gh` shim fallback order or add ambient authentication after a managed mismatch.

## TDD sequence

1. Add failing scope-authorization and temporary-Git routing/resume tests.
2. Thread the typed binding through launch configuration without primitive field sprawl.
3. Generalize remote setup and push routing, then enable the second scope.
4. Refactor common contribution routing and rerun ordinary plus remote-contribution regressions.

## Output contract

Report authorization proofs, runtime/executor coverage, origin/upstream behavior, files changed, red/green
commands, remaining risks, divergence, and task/plan status updates.

## Completion

Completed 2026-08-13. The destination is projected through executor, lifecycle, materialization, Worktree,
Local, Docker, SSH, Sprites, and agentctl paths. Runtime setup adds one exact push remote and branch
pushRemote while preserving canonical `origin` and pull/upstream behavior. Managed credential scopes and
the broker authorizer prove the exact task-bound destination. Raw pushes, agentctl pushes, and GitHub PR
creation use the destination and canonical target respectively; malformed or conflicting state fails closed.
Lease issuance and redemption also revalidate the credential binding and live source/target/parent IDs, and
executor-owned policy or explicit tokens remove the managed route.

Verification: `rtk make -C apps/backend test` and `rtk make -C apps/backend lint` both passed. Focused
runtime, credential, lifecycle, Git, PR, and broker tests passed before the full suite; the new process test
also verifies the explicit `--repo kdlbs/kandev --head owner:branch` invocation.
