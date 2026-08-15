---
spec: docs/specs/task-pr-legacy-workspace-authorization/spec.md
created: 2026-08-15
status: implemented
---

# Implementation Plan: Task PR legacy workspace authorization

## Overview

Replace the two task-PR mutation endpoints' permissive blank-workspace check
with one shared authorization helper. Modern rows continue to authorize their
stored workspace; legacy blank rows resolve the owning task's workspace through
the task store already wired into the GitHub service and fail closed when that
ownership cannot be established.

Implementation depends on the disposition endpoint from open PR #2614 being
present in the implementation base. Prefer rebasing this branch after #2614
merges; if the work is intentionally stacked, rebase onto its current head
before starting the task and record that dependency in the PR.

---

## Backend

### Shared task-PR mutation authorization

- Add `apps/backend/internal/github/service_task_pr_authorization.go` with a
  helper that accepts the loaded `TaskPR` and caller-supplied workspace ID.
- For a non-empty association workspace, compare that stored value with the
  supplied value before calling `authorizeWorkspaceAccess`.
- For a blank association workspace, use the existing `TaskIssueStore.GetTask`
  dependency to read `TaskPR.TaskID`, derive the task workspace, compare it with
  the supplied value, and authorize the derived value.
- Map an absent task, a task-not-found sentinel, a blank task workspace, a
  missing task-store dependency, or a workspace mismatch to
  `ErrTaskPRNotFound`. Preserve unrelated lookup errors so the controllers keep
  returning an internal error without mutating data.
- Keep authorization before every mutation, metric increment, and event
  publication.

### Mutation endpoints

- Update `DetachTaskPR` in
  `apps/backend/internal/github/service_task_pr_detach.go` to call the shared
  helper after loading the association.
- Update `SetTaskPRDisposition` in
  `apps/backend/internal/github/service_task_pr_disposition.go` to call the same
  helper before patch normalization or validation.
- Do not update `github_task_prs.workspace_id`; the owning task is the fallback
  authority for legacy rows.

---

## Tests

- **What:** modern associations authorize the stored workspace without a task
  lookup; legacy associations authorize only the owning task's workspace; a
  missing resolver, missing task, blank task workspace, mismatch, and task
  lookup failure all fail before mutation as specified.
  **File:**
  `apps/backend/internal/github/service_task_pr_authorization_test.go`.
  **How:** table-driven unit tests around the shared helper with a recording
  task store and workspace authorizer.
- **What:** legacy detach rejects a caller-supplied workspace that differs from
  the owning task workspace and leaves the association active; the correct
  derived workspace still permits detach and preserves existing event behavior.
  **File:**
  `apps/backend/internal/github/service_task_pr_detach_test.go`.
  **How:** service tests using the real GitHub SQLite store and fake task store.
- **What:** legacy disposition rejects the same mismatch and leaves all
  disposition columns, metrics, and events unchanged; the correct derived
  workspace still permits the write.
  **File:**
  `apps/backend/internal/github/service_task_pr_disposition_test.go`.
  **How:** service tests using the real GitHub SQLite store and fake task store.
- **What:** DELETE and PATCH return the existing not-found response for a
  legacy association owned by another task workspace.
  **File:**
  `apps/backend/internal/github/controller_task_pr_legacy_workspace_authorization_test.go`.
  **How:** controller integration tests covering handler to service to real
  GitHub store, with the task workspace supplied by the existing fake task
  store.

## Verification Results

- Dependency PR #2614 remains open at `708d8d2`; this implementation is stacked
  on `feature/tel-record-why-a-pul-p4w` and targets the base-repository mirror
  branch `stack/pr-2614-708d8d2`.
- The focused authorization and mutation command passed: 22 tests.
- `go test ./internal/github -run 'TaskPR' -count=1` passed: 122 tests.
- `go test ./internal/github -count=1` passed: 1,611 tests.
- PR review follow-up added a blank-task-ID fail-closed guard and reused the
  shared task-store test double.
- Legacy disposition events carry the derived workspace in an event-only copy,
  and the controller PATCH regression covers the correctly ordered route and
  task-PR not-found body.
- `git diff --check` passed.

## Implementation Waves And Parallel Candidates

Sequential execution in the primary conversation:

- [x] [task-01-harden-task-pr-mutations](task-01-harden-task-pr-mutations.md)

The work is not parallel-safe because both endpoint changes depend on the same
authorization helper and regression-test fixtures.

## Risks

- PR #2614 is open, so the current `main` branch does not contain
  `SetTaskPRDisposition`. Starting production edits before that dependency is
  present would produce an incomplete one-endpoint fix.
- The existing task lookup enforces task ownership under authenticated
  contexts. Task-not-found errors must be normalized to `ErrTaskPRNotFound` so
  the task-PR endpoints retain their non-enumerating 404 contract.
