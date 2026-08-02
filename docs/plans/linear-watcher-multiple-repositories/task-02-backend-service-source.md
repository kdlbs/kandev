---
id: "02-backend-service-source"
title: "Backend: service validation and source mapping"
status: pending
wave: 1
depends_on: ["01-backend-model-store"]
plan: "plan.md"
spec: "../../specs/linear-watcher-multiple-repositories/spec.md"
---

# Task 02: Backend — service validation and source mapping

## Acceptance

- `resolveRepositoryBinding` is replaced by `resolveRepositoryBindings(ctx, workspaceID, reqs) ([]IssueWatchRepository, error)`: empty input ⇒ `nil` (unbound); per-entry trim + drop of empty `repositoryId`; per-entry `securityutil.IsValidBaseBranchRef` check for non-empty branches; **dedupe by `repositoryId` keep-first**; with a wired `RepositoryLookup`, missing or cross-workspace repo ⇒ `ErrInvalidConfig` and empty `baseBranch` is filled with the repo's `DefaultBranch`; unwired lookup ⇒ fail-open (accept as-is).
- `CreateIssueWatch` normalizes plural-then-singular input, resolves bindings, persists `Repositories` AND syncs the legacy `RepositoryID`/`BaseBranch` fields from `Repositories[0]` (service is the single writer for both representations).
- `UpdateIssueWatch` + `applyIssueWatchPatch`: plural present ⇒ replace binding; plural absent + singular present ⇒ existing single-repo PATCH semantics preserved (old callers/tests keep working), then converted into `Repositories`; both absent ⇒ unchanged. Re-resolve only when the binding actually changed, so an unchanged binding with a since-deleted repo never blocks prompt/filter edits.
- `publishNewLinearIssueEvent` emits `Repositories: w.Repositories`; `NewLinearIssueEvent` no longer carries singular fields.
- `LinearWatcherSource.BuildTaskRequest` maps `e.Repositories` onto `IssueTaskRequest.Repositories` in order; **an empty list yields `Repositories == nil`** (the unbound invariant, asserted by a dedicated test).

## Verification

```bash
cd apps/backend && go test ./internal/linear/... ./internal/orchestrator/... -count=1
cd apps/backend && go build ./...
```

## Files likely touched

- `apps/backend/internal/linear/service_issue_watch.go`
- `apps/backend/internal/linear/models.go` (DTOs already extended in task 01; event field swap lands here if not done there)
- `apps/backend/internal/linear/service_issue_watch_repository_test.go` (extend: multi-repo create/update, dedupe, cross-workspace/missing rejection, default-branch fill, singular-only create ⇒ one entry, update omit/clear semantics)
- `apps/backend/internal/linear/service_issue_watch_test.go` (existing repo-binding tests: keep green via the singular-compat path; add unbound invariant)
- `apps/backend/internal/orchestrator/source_linear.go`
- `apps/backend/internal/orchestrator/source_linear_test.go` (update bound/unbound tests to the new event shape; add 2-repo case asserting order + branches + nil-for-unbound)

## Dependencies

Task 01 (model/store contract). Consumed by nothing else in this plan; E2E (task 05) exercises it end-to-end.

## Inputs

- Spec: `API surface`, `Permissions`, `Failure modes`, scenarios 1–2, 6–7, 9.
- Plan: `Design > Service`, `Source`.
- Existing patterns: `resolveRepositoryBinding` (`service_issue_watch.go:388-418`), `normalizeFilter` dedupe, `preflightDeletedRepository` (`orchestrator/watcher_dispatch.go:257-282` — already list-aware, must stay green).

## Risks

- PATCH tri-state of the slice: Go's `nil` slice on an absent JSON key is what distinguishes "unchanged" from "clear" — verify with a test that omits `repositories` and one that sends `[]`.
- The singular-compat path must not regress the "rebind resets branch to new default" behaviour pinned by the existing tests.
- Cross-package: the orchestrator tests construct `NewLinearIssueEvent` with singular fields — update all of them in the same change.

## Output contract

Report service/source changes, exact test results, and which existing tests were updated vs added; mark this task `done` and update its checkbox in `plan.md`.
