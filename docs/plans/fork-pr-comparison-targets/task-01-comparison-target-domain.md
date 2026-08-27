---
id: "01-comparison-target-domain"
title: "Persist repository-qualified comparison targets"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/platform/requirements/workspace-git-status.md"
---

# Task 01: Comparison target domain and attachment persistence

Create the versioned comparison-target model and the exact task-attachment mutation contract. This task
does not configure Git remotes or invoke provider services.

## Red tests first

Add failing tests for:

- round-trip of a GitHub fork PR binding with head and target repository IDs/URLs/branches;
- rejection of unknown versions, credentials/query/fragment in URLs, unsafe refs, missing identities, and
  inconsistent host/provider data;
- stable `compare-<hash>` remote naming and distinct names for different target identities;
- merge-preserving metadata write and source-identity-aware removal;
- exact attachment resolution by attached repository identity plus live checkout branch;
- no mutation for a repository-only match, a historical branch, or ambiguous same-repository siblings;
- same-repository PR targets using normal origin comparison rather than an unnecessary extra remote; and
- manual base-branch update atomically removing a provider-derived target while preserving other metadata.

## Implementation

- Add `apps/backend/internal/task/models/comparison_target.go` with version, provider, change kind/number,
  head repository/branch, target repository/branch, validation, metadata helpers, display identity, and
  deterministic remote/ref helpers. Reuse `RemoteContributionRepository` validation where the contracts
  are identical; do not duplicate URL parsing.
- Extend `TaskRepoRepository` with a narrow comparison-target metadata mutation implemented in
  `apps/backend/internal/task/repository/sqlite/task_repository.go`. Preserve unrelated metadata and avoid
  full-row stale writes. Return the updated attachment and distinguish not-found from no-op.
- Add `apps/backend/internal/task/service/service_comparison_target.go` with provider-neutral candidate and
  result types. Resolve the attachment using provider repository identity and current session/worktree
  branch evidence. Require exactly one match.
- Keep change identity in the binding so background retarget and detach operations can prove ownership.
- Update `UpdateRepositoryBaseBranch` so its durable write removes `comparison_target` in the same mutation;
  keep its existing base-SHA reset, event, and live base-branch fan-out behavior.
- Add structured, secret-free diagnostics for no match and ambiguity. Never log clone URLs or provider errors
  that may contain credentials.

## Acceptance

- Persisted state is sufficient to reconstruct the exact target after restart without local remote names.
- A task with two branches of one repository cannot update the wrong attachment.
- Manual comparison selection reliably returns to repository-local branch semantics.
- No schema migration is needed; malformed persisted targets fail closed.

## Likely files

- `apps/backend/internal/task/models/comparison_target.go` (new)
- `apps/backend/internal/task/models/comparison_target_test.go` (new)
- `apps/backend/internal/task/repository/interface.go`
- `apps/backend/internal/task/repository/sqlite/task_repository.go`
- `apps/backend/internal/task/repository/sqlite/task_repository_test.go`
- `apps/backend/internal/task/service/service_comparison_target.go` (new)
- `apps/backend/internal/task/service/service_comparison_target_test.go` (new)
- `apps/backend/internal/task/service/service_branch_update.go`
- `apps/backend/internal/task/service/service_branch_update_test.go`

## Verification

```bash
cd apps/backend && go test ./internal/task/models ./internal/task/repository/sqlite ./internal/task/service
```

## Parallelism

`sequential`. Task 02 consumes the new model and deterministic ref helpers.

## Output contract

Record the red/green commands, final metadata shape, exact attachment-matching rule, and any deviations.
Set this file to `done` and tick Task 01 in `plan.md` only after the focused tests pass.

## Completion record

- Green: `go test ./internal/task/models ./internal/task/repository/sqlite ./internal/task/service ./internal/task/statussummary`.
- Metadata is version 1 under `task_repositories.metadata.comparison_target`; writes preserve unrelated
  metadata and manual base-branch updates clear the target atomically.
- Matching requires provider repository identity plus normalized live checkout branch, with ambiguous,
  stale, historical, and repository-only matches returning typed no-ops.
