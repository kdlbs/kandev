---
id: "01-pr-base-identity"
title: "Preserve qualified PR base identity"
status: done
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
- Retain `base.sha` / `baseRefOid` across REST and GraphQL conversion. Keep
  `gh pr view/list --json` within the supported CLI field set; read the base
  OID through `gh api` REST because `baseRefOid` is not a supported CLI field.
- Validate namespace, number, head repository, and checkout branch together.
  Use existing exact task-PR associations for legacy attachments. For a
  contribution attached to the PR base, validate the head against its source
  repository and the target against the attachment.
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
- A legacy attachment with no stored `comparison_target` rejects a same-number,
  same-branch PR from another head repository.
- `gh pr view/list --json` requests contain no unsupported `baseRefOid` field;
  `GetPR` reads the base SHA through `gh api` without losing the PR on a
  non-cancellation OID-read error.
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

- The initial fork/upstream regression failed as expected: the resolver queried
  `fork-owner/widgets` instead of `upstream/widgets` for PR #42.
- The permanent tests cover upstream lookup, live retargeting, rejection of a
  colliding PR with a different head repository, exact TaskPR selection, and
  REST/GraphQL base OID conversion. GH CLI command-contract tests verify its
  supported `pr view/list --json` fields and REST base SHA/ref/repository read.
- `go test ./internal/github ./internal/backendapp ./internal/orchestrator/executor -run 'Test(PRBase|ResolveTaskRepoInfo|GithubPRBase)' -count=1`: passed.
- `go test ./internal/github ./internal/backendapp ./internal/orchestrator/executor -count=1`: passed.
- `git diff --check`: passed.
- Review correction: the GH CLI does not support `baseRefOid` in its JSON
  field list. `GetPR` reads base SHA, ref, and repository identity through
  `gh api`; PR view/list keep the existing supported fields, and a REST read
  failure does not discard otherwise usable PR details. Command-contract tests
  cover these paths.
- Review correction: live legacy PR data must match both the attached head
  repository and checkout branch. Contribution bindings use the validated
  source repository for that head check and the attachment for the target.
- Review correction: GitHub repository records store `ProviderHost` as an
  HTTPS URL. Identity normalization accepts that canonical form without
  accepting custom ports, paths, or lookalike domains; regression tests cover
  both the supported host and rejected variants.
