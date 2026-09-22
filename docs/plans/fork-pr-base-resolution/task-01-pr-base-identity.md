---
id: "01-pr-base-identity"
title: "Preserve qualified PR base identity"
status: pending
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-WORKTREE-BASE-REFRESH-001
acceptance_criteria:
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.11
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.15
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.16
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.17
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.19
system_design:
  - ../../specs/workspaces/system-design/worktree-base-refresh.md
---

# Task 01: Preserve qualified PR base identity

## Summary

Preserve provider repository, branch, and base OID through the PR base resolver.
Resolve the namespace from the exact attachment instead of assuming origin.

## In scope

- Introduce a typed resolver result that reuses validated `ComparisonTarget`
  identity and carries a transient optional observed OID.
- Retain `base.sha` / `baseRefOid` across REST, GH CLI, and GraphQL conversion.
  Update the relevant query fields, custom JSON decoding, and mock fixtures.
- Validate namespace, number, head repository, and checkout branch together.
  Use existing exact task-PR associations for legacy attachments.
- Preserve validated stored targets on provider outage. Reject known cross-
  repository inputs that lack a valid target. Preserve same-repository fallback.
- Add the failing executor regression before changing the resolver contract.

## Out of scope

Git materialization, recovery mutation, database migrations, and public UI.

## Acceptance

- `TestResolveTaskRepoInfo_PRBaseUsesTargetRepository` queries upstream, even
  when the fork has an unrelated PR with the same number.
- Provider conversion tests retain target branch/OID and reject incomplete or
  mismatched identity. Same-repository and non-PR tests still pass.
- A live retarget changes only the intended launch target. An ambiguous or
  historical association cannot change another attachment.

## Verification

```bash
(cd apps/backend && go test ./internal/github ./internal/backendapp ./internal/orchestrator/executor -run 'Test(PRBase|ResolveTaskRepoInfo|GithubPRBase)' -count=1)
(cd apps/backend && go test ./internal/github ./internal/orchestrator/executor -count=1)
git diff --check
```

Use the named prefixes for new tests so the focused command executes them.
Record the expected red regression before the implementation and green results.

## Files likely touched

- `apps/backend/internal/github/models.go`
- `apps/backend/internal/github/pat_client.go`
- `apps/backend/internal/github/gh_client.go`
- `apps/backend/internal/github/graphql.go`
- `apps/backend/internal/github/pr_base_identity_test.go` (new)
- `apps/backend/internal/backendapp/orchestrator.go`
- `apps/backend/internal/backendapp/pr_base_resolver_test.go` (new)
- `apps/backend/internal/orchestrator/executor/executor.go`
- `apps/backend/internal/orchestrator/executor/executor_resume.go`
- `apps/backend/internal/orchestrator/executor/executor_pr_base_resolver_test.go`
- `apps/backend/internal/task/models/pr_base.go` (new transient contract)

## Dependencies

None. Read the current task-PR matching and comparison reconciliation helpers.

## Risks

A bare PR number can identify an unrelated fork PR. Query success is not proof
of ownership. Custom REST decoding must preserve the new SHA field.

## Parallelism

`sequential`

## Inputs

- Worktree base refresh design: Identity and provider lookup.
- `github/service_comparison_target.go` and `task/models/comparison_target.go`.
- Existing executor PR-base tests and the plan's temporary reproduction.

## Results

Pending.
