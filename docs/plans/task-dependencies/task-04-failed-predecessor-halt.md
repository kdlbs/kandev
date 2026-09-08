---
id: "04-failed-predecessor-halt"
title: "Halt chains on a failed predecessor"
status: done
wave: 4
depends_on: ["03-resolve-and-auto-start"]
plan: "plan.md"
spec: "../../specs/tasks/system-design/task-dependencies.md"
---

# Task 04: Halt Chains on a Failed Predecessor

## Acceptance

- A predecessor entering `FAILED` or `CANCELLED` never resolves its edge. The
  dependent stays blocked and its `blocked_reason` becomes `"failed"` once no
  predecessor is still pending.
- `task.dependency_failed` is published with
  `{task_id, failed_task_id, failed_state}`, once per
  (dependent, predecessor) failure transition. A repeated write of the same
  failed state does not re-publish.
- A notification event type `task.dependency_failed` is added following the
  existing constants in `internal/notifications/service`, and fires once per
  transition.
- No session is created for the dependent, and its `deferred_launch` intent is
  neither consumed nor deleted — it remains available for a retry.
- Retrying the failed predecessor to successful completion unblocks the
  dependent and fires its intent exactly once.
- Removing the edge unblocks the dependent without firing its intent.
- An archived predecessor reads as `pending`, not `failed` and not `resolved`;
  the dependent stays blocked and the predecessor is listed as pending.
- Deleting a predecessor removes the edges in both directions, may unblock the
  dependent, and does **not** fire its intent — deletion is not success.
- Kandev never auto-retries a failed predecessor and never auto-removes an edge.
- A dependent that already launched is not stopped or rolled back when a
  predecessor is later reopened; it reports blocked again and its running
  session is untouched.

## TDD sequence

1. Failing test: `FAILED` predecessor leaves the dependent blocked with
   `blocked_reason: "failed"`, no session, intent intact.
2. Failing test: `CANCELLED` predecessor behaves identically.
3. Failing test: `task.dependency_failed` publishes once; a duplicate terminal
   write does not re-publish.
4. Failing test: one notification is raised per transition.
5. Failing test: retry-to-success unblocks and launches exactly once.
6. Failing test: edge removal unblocks without launching.
7. Failing test: archived predecessor reads as pending.
8. Failing test: predecessor deletion removes both edge directions, unblocks,
   and launches nothing.
9. Failing test: reopening a predecessor after the dependent launched leaves the
   running session alive and reports blocked again.
10. Implement the failure branch, the event, and the notification type.

## Verification

```bash
cd apps/backend
go test -tags fts5 ./internal/orchestrator ./internal/task/service ./internal/notifications/... -run 'Test.*(DependencyFailed|PredecessorFailed|DependencyArchived|DependencyDeleted)' -count=1
golangci-lint run ./... --new-from-rev=origin/main --timeout=5m
```

## Files likely touched

- `apps/backend/internal/orchestrator/event_handlers_workflow.go` or the
  state-change subscriber added in Task 03
- `apps/backend/internal/task/events/`
- `apps/backend/internal/task/service/service_events.go`
- `apps/backend/internal/task/service/service_tasks.go` (delete/archive edge
  cleanup interaction)
- `apps/backend/internal/notifications/service/service.go`
- focused orchestrator, task-service, and notification tests

## Dependencies

Task 03 — the resolution subscriber is where the failure branch lives.

## Parallelism

`sequential`

## Output contract

Mark this task `in_progress` before the RED tests and `done` only after the
listed commands pass. Record the once-per-transition mechanism, the notification
constant, and the delete-versus-archive difference in this file and `plan.md`.
