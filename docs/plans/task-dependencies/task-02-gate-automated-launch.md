---
id: "02-gate-automated-launch"
title: "Gate automated launches on unresolved dependencies"
status: done
wave: 2
depends_on: ["01-core-dependency-relationship"]
plan: "plan.md"
spec: "../../specs/tasks/system-design/task-dependencies.md"
---

# Task 02: Gate Automated Launches on Unresolved Dependencies

## Acceptance

- `orchestrator.autoStartTaskForStep` skips the launch when the task has any
  unresolved dependency. The check sits next to the existing
  `task.QueuedForStepID != ""` early return and **before**
  `launchDeferredTask`, so a blocked task neither starts a session nor consumes
  its deferred launch intent.
- One check covers every automated entry point. Verified for `task.moved`
  (`handleTaskMovedNoSession`), `task.queue_promoted`
  (`handleTaskQueuePromoted`), and the watcher auto-start path. No second gate
  is added at any call site.
- The gate fails **closed**: if the dependency lookup returns an error, the
  launch is skipped and a warning is logged. A test proves this by making the
  store return an error, not by asserting the happy path.
- A skipped launch leaves the task otherwise untouched — no state change, no
  claim taken, no claim restored, no queue metadata mutation — so a later
  resolution or promotion can launch it.
- Manual start (`StartTask` reached from a user action, HTTP, or WS) is not
  gated. A user can start a blocked task.
- The skip is logged at debug/info with the task id, the step id, and the number
  of unresolved predecessors, following the existing `eventName+": ..."` log
  convention in the file.
- No new goroutine, event subscription, or launch path is introduced by this
  task.

## TDD sequence

1. Failing test: a task with one pending predecessor and an auto-start step
   gains no session on `task.moved`.
2. Failing test: the same task gains no session on `task.queue_promoted`.
3. Failing test: a task with a `deferred_launch` intent and a pending
   predecessor keeps its intent — the claim is not taken.
4. Failing test: with the dependency store returning an error, no session is
   created and a warning is logged.
5. Failing test: manual `StartTask` on a blocked task does create a session.
6. Failing test: a task whose predecessors are all resolved is launched exactly
   as it is today (no regression on the ungated path).
7. Implement the single gate and the log line.

## Verification

```bash
cd apps/backend
go test -tags fts5 ./internal/orchestrator -run 'Test.*(AutoStart|Dependenc|Blocked)' -count=1
go test -tags fts5 ./internal/orchestrator -count=1
golangci-lint run ./... --new-from-rev=origin/main --timeout=5m
```

## Files likely touched

- `apps/backend/internal/orchestrator/event_handlers_workflow.go`
- `apps/backend/internal/orchestrator/service.go` (dependency-reader seam)
- `apps/backend/internal/orchestrator/event_handlers_github.go` (watcher path
  assertion only, if a change proves necessary)
- focused orchestrator tests

## Dependencies

Task 01 — needs the batch derived-state helper and the wired blocker
repository.

## Parallelism

`sequential`

## Output contract

Mark this task `in_progress` before the RED tests and `done` only after the
listed commands pass. Record the exact placement of the gate relative to the
`QueuedForStepID` check and `launchDeferredTask`, the seam used to read
dependency state from the orchestrator, and the fail-closed test's mechanism in
this file and `plan.md`.
