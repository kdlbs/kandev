---
id: "01-enforce-delete-boundary"
title: "Enforce the dirty-worktree deletion boundary"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/tasks/requirements/runtime-cleanup.md"
---

# Task 01: Enforce the Dirty-Worktree Deletion Boundary

## Outcome

Direct and cascade task deletion require explicit consent before they discard a
dirty owned worktree. Consented cleanup remains fail-closed for every other path
and Git identity check.

## In scope

- Add the typed service conflict and HTTP 409 response.
- Preflight all direct or cascade targets before task mutation.
- Carry discard consent in cleanup snapshots.
- Let consent bypass only the clean-checkout audit.
- Stop retrying an unconsented dirty refusal found after task mutation.

## Exclusions

- Frontend changes and localized copy.
- Changes to archive, session deletion, or workspace reset.
- Relaxing owner, path, registration, commit, branch, or shared-reference checks.

## Requirements and design

- `REQ-TASKS-RUNTIME-CLEANUP-001`
- `AC-TASKS-RUNTIME-CLEANUP-001.10`
- `AC-TASKS-RUNTIME-CLEANUP-001.11`
- `AC-TASKS-RUNTIME-CLEANUP-001.12`
- `docs/specs/tasks/system-design/dirty-worktree-deletion.md`
- `docs/specs/tasks/system-design/runtime-cleanup.md`

## Acceptance

- An unconsented direct or cascade request returns the complete typed conflict
  before it changes any selected task or cleanup job.
- A consented request removes dirty owned worktrees only when every existing
  non-cleanliness audit passes, and it still preserves branches with unique
  commits.
- A legacy or raced unconsented dirty refusal becomes terminal after one worker
  claim and does not enter `retry_wait`.

## Verification

```bash
(cd apps/backend && go test ./internal/worktree ./internal/task/service ./internal/task/handlers -count=1)
```

Write the direct-delete regression first. It must fail by showing that the task
row is currently deleted before the cleanup worker rejects the dirty checkout.

## Files likely touched

- `apps/backend/internal/worktree/manager_cleanup.go`
- `apps/backend/internal/worktree/manager_cleanup_audit.go`
- `apps/backend/internal/worktree/manager_cleanup_audit_test.go`
- `apps/backend/internal/task/models/resource_cleanup.go`
- `apps/backend/internal/task/service/service_tasks.go`
- `apps/backend/internal/task/service/handoff_service.go`
- `apps/backend/internal/task/service/handoff_cascade.go`
- `apps/backend/internal/task/service/resource_cleanup_jobs.go`
- `apps/backend/internal/task/service/resource_cleanup_jobs_test.go`
- `apps/backend/internal/task/service/resource_cleanup_worktree_retry_test.go`
- `apps/backend/internal/task/service/handoff_cascade_test.go`
- `apps/backend/internal/task/handlers/task_http_handlers.go`
- `apps/backend/internal/task/handlers/task_http_handlers_test.go`

## Dependencies

None.

## Parallelism

Sequential. This task owns the API and cleanup contract required by Task 02.

## Risks

- Do not inspect a user-supplied path. Use the audited owned-worktree snapshot.
- Lock or snapshot ordering must prevent partial cascade mutation.
- Existing cleanup snapshots have no consent field and must remain fail-closed.

## Output contract

Report the RED test, response shape, admission ordering, force-audit boundary,
changed files, and exact test results. Update this work order and `plan.md` in
the same conversation.

## Results

- Added `DirtyWorktree` inspection with owned-path, no-follow, bounded Git-status, and sorted-file
  safeguards. Direct and cascade deletion now resolves and preflights the complete target set
  before changing task rows or creating cleanup work.
- Added the typed `task_delete_dirty_worktree` service conflict and HTTP 409 payload with every
  affected worktree. Cleanup snapshots persist `discard_worktree_changes`, while old snapshots
  remain unconsented and fail closed.
- Consent bypasses only the dirty-checkout audit. Path identity, ownership, registration, expected
  `HEAD`, shared references, branch ancestry, and unique-commit preservation remain enforced.
  A raced or legacy unconsented dirty refusal becomes terminal after its first claim.
- The direct regression first captured the old delete-before-refusal behavior, then passed with the
  task row and dirty file preserved. Consent cleanup and raced terminal-state regressions also pass.
- Verification: `go test ./internal/worktree ./internal/task/service ./internal/task/handlers
  -count=1` passed 2,685 tests across the three packages.
- Review follow-up joins every cleanup failure instead of retaining only the last error. The new
  batch-order regression proves that a retryable metadata failure followed by a dirty-worktree
  refusal remains retryable, and the missing-metadata inspection regression proves admission fails
  closed after cache enrichment.
- The cascade service regression drives `DeleteTaskTreeWithOptions` through the complete root,
  child, and grandchild set, proving refusal is atomic and the discard option reaches admission.
- The E2E reset boundary now explicitly opts into discarding disposable worktree changes, with a
  focused regression covering the option passed to task deletion.
- Follow-up verification: `go test ./internal/worktree ./internal/task/service
  ./internal/task/handlers -count=1` passed 2,693 tests across the three packages; specification
  lint also passed.
