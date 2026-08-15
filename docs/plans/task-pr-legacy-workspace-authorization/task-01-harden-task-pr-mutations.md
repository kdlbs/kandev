---
id: "01-harden-task-pr-mutations"
title: "Harden task PR mutations"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/task-pr-legacy-workspace-authorization/spec.md"
---

# Task 01: Harden task PR mutations

## Acceptance

- Both detach and disposition mutations authorize the association's stored or
  task-derived workspace through one shared helper.
- Legacy blank-workspace associations fail closed for a mismatched, absent, or
  unresolved owning task and produce no mutation, metric, or event side effect.
- Modern and correctly resolved legacy associations preserve their existing
  success responses and event behavior without backfilling the association.

## Verification

Run after PR #2614 is present in the implementation base:

```bash
cd apps/backend
go test ./internal/github -run 'Test(AuthorizeTaskPRMutationWorkspace|ServiceDetachTaskPRLegacyWorkspace|SetTaskPRDispositionLegacyWorkspace|HTTPTaskPRMutationsLegacyWorkspace)' -count=1
go test ./internal/github -run 'TaskPR' -count=1
cd ../..
git diff --check
```

## Files likely touched

- `apps/backend/internal/github/service_task_pr_authorization.go`
- `apps/backend/internal/github/service_task_pr_authorization_test.go`
- `apps/backend/internal/github/service_task_pr_detach.go`
- `apps/backend/internal/github/service_task_pr_detach_test.go`
- `apps/backend/internal/github/service_task_pr_disposition.go`
- `apps/backend/internal/github/service_task_pr_disposition_test.go`
- `apps/backend/internal/github/controller_task_pr_legacy_workspace_authorization_test.go`
- `docs/plans/task-pr-legacy-workspace-authorization/plan.md`
- `docs/plans/task-pr-legacy-workspace-authorization/task-01-harden-task-pr-mutations.md`

## Dependencies

- Open PR #2614 must be merged into the base or intentionally present beneath
  this work as a stacked dependency.
- The GitHub service's existing `TaskIssueStore.GetTask` production wiring in
  `apps/backend/internal/backendapp/main.go` remains the task-workspace source.

## Parallelism

`sequential`. Both endpoints and their tests depend on the same helper and test
fixtures.

## Inputs

- Spec permissions, failure modes, and scenarios.
- `docs/decisions/0047-github-authentication-ownership.md`, which requires each
  GitHub entry point to supply or derive a workspace and fail closed when
  ownership is missing.
- Existing `resolveAuthorizedTaskWorkspace` and `TaskIssueStore.GetTask`
  patterns in `apps/backend/internal/github/service_ci_automation.go` and
  `apps/backend/internal/github/service_task_issue.go`.
- Existing legacy behavior test
  `TestServiceDetachTaskPRAllowsLegacyEmptyWorkspace`.

## Output contract

Report the shared authorization behavior, all changed files, exact test command
results, dependency/base state, side-effect boundaries verified, remaining
risks, and synchronized task/plan status in the primary conversation.

## Results

- Added one shared task-PR mutation authorization helper. Modern rows use their
  stored workspace; legacy rows resolve the owning task workspace through the
  existing `TaskIssueStore`.
- Workspace mismatches, missing resolvers, absent tasks, task-not-found errors,
  and blank task workspaces return `ErrTaskPRNotFound` before authorization or
  mutation. Unrelated task lookup errors remain unchanged.
- Wired the helper into detach and disposition before validation, writes,
  metrics, or event publication. Legacy rows remain blank in
  `github_task_prs.workspace_id`.
- Dependency PR #2614 remains open at `708d8d2`; this branch is intentionally
  stacked on `feature/tel-record-why-a-pul-p4w`.
- Verification passed:
  - Focused authorization and mutation tests: 21 passed.
  - `go test ./internal/github -run 'TaskPR' -count=1`: 121 passed.
  - `go test ./internal/github -count=1`: 1,610 passed.
  - `git diff --check` passed.
