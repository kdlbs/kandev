---
spec: docs/specs/tasks/system-design/wip-limit-pull-system.md
created: 2026-07-29
status: complete
---

# Implementation Plan: Single Auto-Start Per Review Task

## Overview

Fix a duplicate-agent bug: a GitHub review-watch task created for an auto-start
`Review` step that has a WIP limit plus a `pull_from_step_id` feeder can launch
two agent sessions. The fix adds one race-safe, per-task auto-start claim shared
by the two auto-start paths so exactly one launches. No schema, API, or UI
changes are required.

---

## Confirmed root cause

`task/service.Service.CreateTask` places the review task in the feeder, then
**synchronously** promotes it into `Review` inside the same call
(`pullTasksFromNewFeederWork` -> `promoteFeederQueuedTask`), re-reads the task,
and returns the promoted task (`queued_for_step_id` cleared). Two paths then
each call `StartTask` for that admitted task with no shared guard:

- **Path A (event-driven):** the promotion publishes `task.moved` /
  `task.queue_promoted`; the orchestrator handler
  `autoStartTaskForStep` (`event_handlers_workflow.go:406`) passes its
  `QueuedForStepID != ""` guard (already cleared) and spawns
  `go StartTask(...)`.
- **Path B (synchronous):** `createReviewTask`
  (`event_handlers_github.go:363-366`) sees the returned task with
  `QueuedForStepID == ""`, so it calls `autoStartReviewTask` ->
  `StartTask(...)`.

For Kanban (non-Office) tasks `StartTask` always mints a fresh session, so both
paths create a session. Only the deferred-launch path
(`claimDeferredLaunch` + `RemoveTaskMetadataKey`) has an atomic claim today; the
review-watcher/on-enter auto-start paths have none.

---

## Backend design

### Shared auto-start claim helper

Add a single race-safe claim, reusing the existing atomic metadata-claim
mechanism class (`RemoveTaskMetadataKey`, already used by
`claimDeferredLaunch`). Introduce a new metadata key
`MetaKeyAutoStartClaimed` in
`apps/backend/internal/task/models/models.go` (alongside
`MetaKeyDeferredLaunch`).

- Creation adapters that produce an auto-start-eligible task set
  `MetaKeyAutoStartClaimed` in the task metadata at create time (atomic with the
  task row, same commit as other create metadata). This is the token both paths
  compete to consume.
- New helper in `event_handlers_workflow.go`, e.g.
  `claimAutoStart(ctx, taskID, eventName) bool`, that calls
  `RemoveTaskMetadataKey(ctx, taskID, MetaKeyAutoStartClaimed)` and returns
  `true` only for the caller that removed the key. Mirrors
  `claimDeferredLaunch`'s atomic semantics.
- On launch failure the claim is restored (`SetTaskMetadataKey`) so the task can
  be retried, mirroring `restoreDeferredLaunch`.

### Wire both paths through the claim

- **Path A** — `autoStartTaskForStep` (`event_handlers_workflow.go:406`): after
  the existing `launchDeferredTask` short-circuit and after confirming the step
  has `OnEnterAutoStartAgent`, call `claimAutoStart`. If it returns `false`,
  return without launching. Only claim when the token is present so ordinary
  (non-watcher) auto-start tasks that never set the token keep launching exactly
  once via this single path (see "Token scope" below).
- **Path B** — `autoStartReviewTask` (`event_handlers_github.go`): call
  `claimAutoStart` before `StartTask`; skip when it returns `false`.

### Token scope (avoid regressing normal auto-start)

Only tasks that can be reached by BOTH paths need the token. That is the
watcher-created task whose destination is a WIP+feeder auto-start step. To keep
the change minimal and not regress the many tasks that legitimately auto-start
through Path A alone:

- The claim is a no-op skip when the key is absent AND the caller is Path A
  (event-driven): absence means "no competing synchronous path exists", so Path
  A launches as before.
- Path B always requires a successful claim: it only runs for watcher-created
  tasks, which set the token at create time.

This makes the guard active precisely for the double-start window (token
present) and transparent everywhere else. Decision recorded in the task file;
if review prefers an unconditional claim for every auto-start, that is the
alternative called out in Open Questions.

---

## Tests

- **What:** A single review task reachable by both auto-start paths launches
  exactly one session. **File:**
  `apps/backend/internal/orchestrator/event_handlers_github_review_test.go`.
  **How:** seed an `OnEnterAutoStartAgent` step; create a task carrying
  `MetaKeyAutoStartClaimed`; invoke Path A (`autoStartTaskForStep` /
  `handleTaskQueuePromoted`) and Path B (`autoStartReviewTask`) against the same
  task; assert exactly one launch. Count launches via `mockAgentManager`'s
  `launchAgentFunc` and/or `repo.ListTaskSessions(taskID)` == 1. Join the async
  `go StartTask` goroutines via the launch counter side effect (per backend
  AGENTS.md goroutine-join guidance), not `time.Sleep`.
- **What:** `claimAutoStart` is atomic — two concurrent claimers, one winner.
  **File:** same test file. **How:** call the claim helper twice for one task;
  assert first returns `true`, second `false`.
- **What:** failed launch restores the claim so a later trigger can retry.
  **File:** same test file. **How:** force the launch path to error, assert the
  key is present again and a subsequent claim succeeds.
- **What:** an ordinary auto-start task with NO token still launches once via
  Path A. **File:** same test file (or `event_handlers_workflow_moved_test.go`).
  **How:** existing `handleTaskMoved` auto-start test remains green.

Run: `cd apps/backend && go test -race -run 'AutoStart|ReviewTask' ./internal/orchestrator/...`

---

## Implementation Waves And Parallel Candidates

Small fix — sequential, single task.

```
Wave 1:
- [x] [task-01-shared-autostart-claim](task-01-shared-autostart-claim.md)
```

## Verification

Implemented on branch `feature/fix-double-agents`. The shared race-safe claim
(`MetaKeyAutoStartClaimed`) is set at review-task creation
(`buildReviewTaskRequest`) alongside the `MetaKeyAutoStartGuard` marker; both
`autoStartTaskForStep` (Path A) and `autoStartReviewTask` (Path B) compete for
it via `claimAutoStart`, and a failed launch restores the token via
`restoreAutoStartClaim`.

Command:
`cd apps/backend && go test -race -run 'AutoStart|ReviewTask' ./internal/orchestrator/...`
— pass. Coverage includes both-paths-fire-once, sequential claim atomicity, a
concurrent barrier claim (exactly one winner under contention), failed-launch
restore, and no-token-does-not-block.

Known limitation (tracked for follow-up): the guard is not cleared after the
one-shot token is consumed, so a review task whose completed session is deleted
and is then moved back into an auto-start step would find the guard present and
the token gone, and Path A would skip that re-entry launch. Clearing the guard
on success reopens the creation-time double-launch window, so a correct fix
needs a launch-generation marker rather than a simple guard clear.

---

## Open Questions

- Should the claim be unconditional for every auto-start path (simpler, but
  touches all Path A creation sites to set the token) rather than
  token-present-only? Current plan uses token-present-only to keep the blast
  radius to watcher-created tasks. Confirm during review.
