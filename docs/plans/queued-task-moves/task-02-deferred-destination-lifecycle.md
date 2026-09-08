---
id: "02-deferred-destination-lifecycle"
title: "Defer destination lifecycle until promotion"
status: completed
wave: 2
depends_on: ["01-atomic-move-admission"]
plan: "plan.md"
spec: "../../specs/tasks/system-design/wip-limit-pull-system.md"
---

# Task 02: Defer Destination Lifecycle Until Promotion

## Acceptance

- `task.moved` carries enough admission information for consumers to
  distinguish an admitted move from a destination-resident queued move.
- A queued move runs source `on_exit` exactly once and does not run target
  `on_enter`, target state synchronization, terminal behavior, context reset,
  plan-mode behavior, or auto-start.
- `task.queue_promoted` is the single destination-entry boundary. It applies
  the target state and behavior exactly once for tasks with or without an
  active session.
- Workflow-engine transitions use destination admission and do not advance
  runtime state ahead of queue promotion.
- An automatic transition to a full limited step succeeds as a queued move.
  The destination entry behavior waits for admission.
- Destination-resident queued tasks are selected before feeder-resident tasks;
  deterministic ordering inside each pool remains position, priority, queue
  time, creation time, and task ID.
- Duplicate/replayed move or promotion events do not repeat lifecycle effects.

## TDD Sequence

1. Add failing watcher/orchestrator tests for source exit at queue time and
   destination entry at promotion time, with and without a session.
2. Extend the move event contract and split move handling at the committed
   admission boundary.
3. Expand promotion handling to execute deferred destination entry once.
4. Add workflow-store tests for queued transitions and destination-before-
   feeder selection.
5. Run focused packages GREEN and refactor shared entry logic.

## Verification

```bash
cd apps/backend
go test -tags fts5 ./internal/orchestrator/... \
  -run 'Test.*(TaskMoved|QueuePromoted|QueuedTransition|PullNext)' -count=1
go test -tags fts5 ./internal/task/service/... \
  -run 'Test.*(TaskMovedEvent|Promotion|QueueOrder)' -count=1
```

## Implementation Result

- `task.moved` now carries committed admission metadata and a promotion marker
  when a queued task becomes admitted.
- Queued moves run source exit once and defer destination state, terminal
  effects, context reset, plan mode, and auto-start until promotion.
- Queue promotion handles both existing-session and no-session tasks through a
  one-shot metadata claim. Destination-resident queued tasks are selected
  before feeder candidates.
- Workflow-engine transitions use the same admission boundary and stop before
  destination entry when the target is queued.
- Replay prerequisite failures leave lifecycle claim metadata available for a
  later retry, and promotion state synchronization errors abort that attempt.

The focused backend run passed:

```text
rtk go test -tags fts5 ./internal/task/service ./internal/orchestrator -run 'TestService_MoveTaskQueuesFullWIPLimitedTarget|TestClaimTaskEventMetadataIsOneShot|TestExecuteStepTransition_FullTargetRunsExitAndDefersEntry|TestService_MoveTaskPullsNextFeederTaskOnVacate|TestService_MoveTaskPullSkipsBlockedFeederCandidate' -count=1
5 passed in 2 packages
```

The post-review focused backend run also passed the full affected package set:

```text
rtk go test -tags fts5 ./internal/task/repository/... ./internal/task/service ./internal/orchestrator -count=1
Go test: 3150 passed in 5 packages
```

## Files Likely Touched

- `apps/backend/internal/task/service/service_events.go`
- `apps/backend/internal/task/service/service_workflow.go`
- `apps/backend/internal/orchestrator/watcher/watcher.go`
- `apps/backend/internal/orchestrator/event_handlers_workflow.go`
- `apps/backend/internal/orchestrator/event_handlers_workflow_queue.go`
- `apps/backend/internal/orchestrator/workflow_store.go`
- `apps/backend/internal/orchestrator/event_handlers_workflow_moved_test.go`
- `apps/backend/internal/orchestrator/event_handlers_workflow_test.go`
- `apps/backend/internal/orchestrator/workflow_store_test.go`
- `apps/backend/internal/task/service/service_workflow_test.go`

## Dependencies

Task 01 supplies the committed admitted-or-queued move result.

## Parallelism

`sequential`

## Output Contract

Record RED/GREEN lifecycle evidence, event payload changes, idempotency
mechanism, queue-priority evidence, files changed, and exact command results.
Update this task and `plan.md` status in the same implementation conversation.
