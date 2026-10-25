---
id: "02-aggregate-submodule-git-data"
title: "Aggregate root and submodule Git data"
status: done
wave: 1
depends_on: ["01-discover-nested-repository-scopes"]
plan: "plan.md"
spec: "../../specs/ui/requirements/submodule-review.md"
---

# Task 02: Aggregate root and submodule Git data

## Acceptance

- Unpinned status, log, and cumulative-diff requests include a real workspace root plus all initialized submodule scopes in stable order, while bare multi-repository task roots remain excluded.
- Cumulative files carry nested `repository_name`, repository-relative `path`, and the parent-derived `base_ref`; one scope's failure preserves successful results according to the existing endpoint contract.
- Root file/ref access, LSP/search consumers, cancellation, deterministic merges, and class-aware Git admission retain their existing behavior when named submodule scopes exist.

## Verification

```bash
cd apps/backend && go test ./internal/agentctl/server/api -run 'Test.*SubmoduleReview|TestMultiRepoReview|TestGitStatusMulti'
cd apps/backend && go test ./internal/agentctl/server/process -run 'TestManager_.*RepoScopes|TestManager_RepoSubpaths'
```

## Files likely touched

- `apps/backend/internal/agentctl/server/process/manager.go`
- `apps/backend/internal/agentctl/server/api/git.go`
- `apps/backend/internal/agentctl/server/api/workspace.go`
- `apps/backend/internal/agentctl/server/api/git_multi_repo_review_test.go`
- `apps/backend/internal/agentctl/server/api/git_status_fresh_concurrency_test.go`
- `apps/backend/internal/agentctl/server/process/manager_multi_repo_stream_test.go`
- Runtime client or gateway status tests only if the unchanged JSON shape exposes a compatibility gap

## Dependencies

Task 01.

## Parallelism

`sequential` — it consumes and finalizes the manager contract introduced by Task 01.

## Inputs

- Spec **API surface** and partial-failure scenarios.
- Existing multi-repository fan-out in `api/git.go` and root-content guard in `api/workspace.go`.
- ADR `2026-08-02-class-aware-git-subprocess-admission.md`.

## Output contract

Report endpoint shapes, ordering, partial failures, root-access audit, files changed, exact tests and counts, blockers/risks, and synchronized task/plan status.

## Results

Implemented ordered root/direct/nested repository-scope fan-out for status, log, and cumulative diff, preserving repository-relative paths, nested names, stable parent-derived base refs, partial failures, and root file access. Bare task roots retain their existing named-only behavior.

Verification:

- `go test ./internal/agentctl/server/api -run 'Test(NestedSubmoduleReviewEndpointsIncludeRootAndStableChildBase|MultiRepoReviewEndpoints)' -count=1` — 3 passed.
- `make -C apps/backend test` — passed; `make -C apps/backend build` — completed.
