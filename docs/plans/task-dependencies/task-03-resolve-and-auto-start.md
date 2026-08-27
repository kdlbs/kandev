---
id: "03-resolve-and-auto-start"
title: "Resolve dependencies and auto-start on unblock"
status: done
wave: 3
depends_on: ["02-gate-automated-launch"]
plan: "plan.md"
spec: "../../specs/tasks/system-design/task-dependencies.md"
---

# Task 03: Resolve Dependencies and Auto-Start on Unblock

## Acceptance

- A subscriber reacts to the task-state changes and workflow-step moves that can
  make a predecessor resolved, looks up dependents with `ListTasksBlockedBy`,
  and re-evaluates all predecessors of each dependent.
- When the last unresolved predecessor of a task resolves successfully,
  `task.dependencies_resolved` is published with
  `{task_id, resolved_by_task_id}` and the dependent is handed to
  `autoStartTaskForStep`. No bespoke launch call is added.
- A dependent with no `deferred_launch` intent unblocks and starts nothing.
- A dependent that is not WIP-admitted starts nothing on resolution; its intent
  survives and the existing WIP promotion path launches it once, later.
- A dependent whose step has no `on_enter: auto_start_agent` and no intent
  unblocks and starts nothing.
- Removing the last blocking edge unblocks the task and does **not** consume its
  intent or launch it.
- A chain A → B → C, each with an intent, launches each task exactly once in
  order and never restarts an earlier task.
- Exactly one session is created when dependency resolution and WIP promotion
  race, guaranteed by the existing atomic deferred-launch/auto-start claim. A
  claim loser logs and stops; a failed launch restores the claim so the task
  stays retryable and visibly unstarted.
- Startup reconciliation, running after the existing WIP queue reconciler,
  sweeps non-archived tasks that hold a `deferred_launch` intent and have edges,
  and launches those whose predecessors are all resolved. It is bounded and
  batched so a large graph does not delay readiness, and it is idempotent across
  repeated runs.
- A restart between a predecessor's completion and the dependent's launch yields
  exactly one session after startup.
- `cascadeBlockersResolved`, Office assignment, and `TriggerOnBlockerResolved`
  wiring are unchanged. With `Features.Office` true and a task eligible under
  both reactions, exactly one session is created.
- Any goroutine this task starts has a single owner with idempotent
  `Start`/`Stop`, cancels on `ctx.Done()`, and is covered by the package's
  `goleak` `TestMain`.

## TDD sequence

1. Failing test: last-predecessor-resolves publishes
   `task.dependencies_resolved` once and launches the dependent exactly once
   with the intent's recorded agent profile, executor, executor profile, prompt,
   and plan-mode choice.
2. Failing test: a non-last resolution publishes nothing and launches nothing.
3. Failing test: resolution via a move into a final `Done` step counts, not just
   `state = COMPLETED`.
4. Failing test: a dependent without an intent unblocks and starts nothing.
5. Failing test: a queued (not WIP-admitted) dependent starts nothing on
   resolution and starts exactly once on later promotion.
6. Failing test: removing the last edge unblocks without launching.
7. Failing test: A → B → C chain launches each once, in order.
8. Failing test: simultaneous resolution and promotion produce one session; the
   loser logs; a launch failure restores the claim.
9. Failing test: restart between completion and launch yields one session after
   startup reconciliation; a second reconciliation run launches nothing more.
10. Failing test: with Office enabled, a task eligible under both reactions gets
    one session.
11. Implement the subscriber, the event, and the startup reconciliation.

## Verification

```bash
cd apps/backend
go test -tags fts5 ./internal/orchestrator ./internal/task/service ./internal/office/scheduler -run 'Test.*(DependenciesResolved|DependencyChain|AutoStartUnblock|StartupReconcil)' -count=1
go test -tags fts5 -race ./internal/orchestrator ./internal/task/service -count=1
golangci-lint run ./... --new-from-rev=origin/main --timeout=5m
```

## Files likely touched

- `apps/backend/internal/orchestrator/event_handlers_workflow.go`
- `apps/backend/internal/orchestrator/event_handlers_task.go` or the equivalent
  state-change subscriber
- `apps/backend/internal/task/events/` (new event type)
- `apps/backend/internal/task/service/service_events.go`
- `apps/backend/internal/backendapp/main.go` (startup reconciliation ordering)
- focused orchestrator and task-service tests

## Dependencies

Task 02 — the gate must exist first, otherwise resolution-driven launches would
bypass it and the WIP interaction could not be asserted.

## Parallelism

`sequential`

## Output contract

Mark this task `in_progress` before the RED tests and `done` only after the
listed commands pass. Record which events the subscriber listens to, where
startup reconciliation runs relative to the WIP queue reconciler, its batch
bound, and the race-test mechanism in this file and `plan.md`.
