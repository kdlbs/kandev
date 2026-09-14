---
spec: docs/specs/tasks/requirements/atomic-terminal-routing.md
related_specs:
  - docs/specs/tasks/system-design/atomic-terminal-routing.md
created: 2026-09-14
status: in_progress
---

# Implementation Plan: Atomic Terminal Routing

## Objective

Make every workflow route claim and commit one source-step generation before
any route-owned prompt or lifecycle side effect occurs. A terminal winner
settles deferred rows and their pending route operations in the same task
transaction, so an exact retry cannot revive an obsolete destination.

## Design constraints

- The task repository owns the source-step compare-and-swap for every route,
  including manual, feeder, queue, and deferred paths.
- CAS rebasing retains only metadata owned by the move operation, including
  intentional deletion of lifecycle markers; it preserves unrelated concurrent
  task edits from the fresh row.
- Deferred prompts remain in `EntryOptions` until the winning route commits.
  Rollback or cancellation emits neither a prompt nor a queue side effect.
- Terminal settlement changes pending operations to an absorbing outcome and
  deletes their exact pending rows atomically on SQLite and PostgreSQL.
- Lifecycle effects claim their route effect before execution, preserve the
  claimant through transient completion retries, and do not rerun an executing
  external effect.

## Work sequence

1. Add task-step CAS guards to the common move write paths and cover stale
   source, zero-transition, feeder, and concurrent manual-move cases.
2. Carry route-owned pending, queue-exit, promotion, and lifecycle metadata
   through CAS rebasing while preserving unrelated fresh metadata.
3. Persist deferred entry options without prequeuing prompts; consume them only
   after the route winner commits.
4. Settle pending route operations and pending rows with terminal task state in
   one transaction for SQLite and PostgreSQL.
5. Verify lifecycle claim fencing, lease recovery, feeder continuation, exact
   retry behavior, queue E2E, and SQL parity.

## Validation

- `cd apps/backend && go test ./internal/task/service ./internal/orchestrator ./internal/orchestrator/messagequeue ./internal/mcp/handlers`
- `cd apps/backend && go test ./internal/task/repository/sqlite -run 'TestPostgresRepository_(ReplaceSessionRejectsSnapshotAfterTerminalSettlement|TerminalRouteAbsorbsConcurrentPendingMoveAdmission)' -count=1`
- `cd apps/web && pnpm e2e:run tests/workflow/workflow-manual-move-queue.spec.ts`
