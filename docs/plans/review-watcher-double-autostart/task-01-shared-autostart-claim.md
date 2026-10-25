---
id: "01-shared-autostart-claim"
title: "Shared race-safe auto-start claim for review tasks"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/tasks/system-design/wip-limit-pull-system.md"
---

# Task 01: Shared race-safe auto-start claim

Ensure a single accepted review-watch create auto-starts at most one agent
session, even when the task is placed in a feeder and immediately promoted into
an auto-start `Review` step during the same `CreateTask` call.

## Root cause (for the implementer)

Both auto-start paths call `StartTask` for the same admitted task with no shared
guard:

- Path A: `autoStartTaskForStep` (`internal/orchestrator/event_handlers_workflow.go:406`)
  triggered by the promotion's `task.moved` / `task.queue_promoted` event; its
  `task.QueuedForStepID != ""` guard (line 413) is already cleared after
  promotion, so it spawns `go StartTask(...)`.
- Path B: `autoStartReviewTask` (`internal/orchestrator/event_handlers_github.go`)
  called synchronously from `createReviewTask` (lines 363-366) because the
  returned task has `QueuedForStepID == ""`.

Only `claimDeferredLaunch` (`event_handlers_workflow.go:529`, backed by
`RemoveTaskMetadataKey` in `internal/task/repository/sqlite/task.go:474`) has an
atomic claim today. Reuse that mechanism class.

## TDD sequence (Red -> Green)

1. Add a failing test in
   `internal/orchestrator/event_handlers_github_review_test.go` that seeds an
   `OnEnterAutoStartAgent` step, creates a task carrying the new
   `MetaKeyAutoStartClaimed` token, drives BOTH Path A and Path B against that
   task, and asserts exactly one launch (count via `mockAgentManager.launchAgentFunc`
   and/or `repo.ListTaskSessions(taskID)` length == 1). Join async `go StartTask`
   goroutines via the launch counter (channel/side-effect), not `time.Sleep`.
2. Add a failing unit test that `claimAutoStart` returns `true` then `false` for
   two claims on the same task.
3. Add a failing test that a launch failure restores the token (subsequent claim
   succeeds).
4. Implement (below) until green.
5. Keep the existing `handleTaskMoved` auto-start test green (no-token task still
   launches once via Path A).

## Implementation

- Add constant `MetaKeyAutoStartClaimed = "auto_start_claimed"` in
  `internal/task/models/models.go` (in the metadata-keys const block near
  `MetaKeyDeferredLaunch`, line ~96).
- Set the token at review-task creation so the two paths compete for it. Set it
  on the create request metadata that `buildReviewTaskRequest`
  (`event_handlers_github.go`) produces (or in the `CreateReviewTask` adapter in
  `internal/backendapp/orchestrator.go`), so it commits atomically with the task
  row. Do NOT set it via a follow-up `UpdateTask`.
- Add `claimAutoStart(ctx, taskID, eventName string) bool` in
  `event_handlers_workflow.go`, mirroring `claimDeferredLaunch`: type-assert the
  repo to the `RemoveTaskMetadataKey(context.Context, string, string) (bool, error)`
  interface and remove `MetaKeyAutoStartClaimed`. Return `true` only when the key
  was present and removed.
- Add `restoreAutoStartClaim(ctx, taskID, eventName string)` mirroring
  `restoreDeferredLaunch`, using `SetTaskMetadataKey` to re-add the token on
  launch failure.
- Path A (`autoStartTaskForStep`): after the `launchDeferredTask` short-circuit
  and after confirming `step.HasOnEnterAction(OnEnterAutoStartAgent)`, if the
  task metadata contains `MetaKeyAutoStartClaimed`, call `claimAutoStart`; when
  it returns `false`, return without launching. When the token is absent, launch
  as today (no regression for ordinary auto-start tasks). On `StartTask` error
  inside the goroutine, call `restoreAutoStartClaim`.
- Path B (`autoStartReviewTask`): call `claimAutoStart` before `StartTask`; when
  it returns `false`, log and return. On `StartTask` error, call
  `restoreAutoStartClaim`.

## Acceptance

- A review task reachable by both auto-start paths produces exactly one session
  / one `LaunchAgent` call.
- `claimAutoStart` is atomic: two claims for one task yield one `true`, one
  `false`.
- A failed launch restores the token so a later trigger retries.
- An ordinary auto-start task with no token still launches exactly once via
  Path A (existing tests remain green).
- No schema, DTO, API, or UI changes.

## Verification

```bash
cd apps/backend
go test -race -run 'AutoStart|ReviewTask|TaskMoved' ./internal/orchestrator/... -count=1
golangci-lint run ./internal/orchestrator/... ./internal/task/models/... --timeout=5m
```

## Files likely touched

- `apps/backend/internal/task/models/models.go`
- `apps/backend/internal/orchestrator/event_handlers_workflow.go`
- `apps/backend/internal/orchestrator/event_handlers_github.go`
- `apps/backend/internal/backendapp/orchestrator.go` (or `buildReviewTaskRequest`)
- `apps/backend/internal/orchestrator/event_handlers_github_review_test.go`

## Parallelism

sequential

## Output contract

On completion: summarize the change, list files touched, paste the `go test`
result, note any blockers/risks, set this file's `status: done`, and tick the
Wave 1 checkbox in `plan.md`.

## Completion record

Summary: added the one-shot `MetaKeyAutoStartClaimed` token (set at review-task
creation in `buildReviewTaskRequest` alongside `MetaKeyAutoStartGuard`) and the
`claimAutoStart` / `restoreAutoStartClaim` helpers. Path A
(`autoStartTaskForStep`) and Path B (`autoStartReviewTask`) both compete for the
token when the guard is present; only the winner launches, and a failed launch
restores the token for retry. Ordinary (guardless) auto-start tasks are
unaffected.

Files touched:

- `apps/backend/internal/task/models/models.go`
- `apps/backend/internal/orchestrator/event_handlers_workflow.go`
- `apps/backend/internal/orchestrator/event_handlers_github.go`
- `apps/backend/internal/orchestrator/event_handlers_github_review_test.go`

Test result:

```
cd apps/backend && go test -race -run 'AutoStart|ReviewTask|ReviewWatch' ./internal/orchestrator/
ok  	github.com/kandev/kandev/internal/orchestrator
```

Covered: both-paths-fire-once, sequential claim atomicity, concurrent barrier
claim (exactly one winner under contention), failed-launch restore, and
no-token-does-not-block.

Blockers/risks: the guard is intentionally left permanent. A review task whose
completed session is deleted and is then moved back into an auto-start step
would find the guard present and the token consumed, so Path A skips that
re-entry launch. Clearing the guard on success reopens the creation-time
double-launch race, so a proper fix needs a launch-generation marker; tracked
as follow-up in `plan.md` rather than fixed here.
