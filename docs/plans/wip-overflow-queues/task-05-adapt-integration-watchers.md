---
id: "05-adapt-integration-watchers"
title: "Adapt integration watchers"
status: completed
wave: 5
depends_on:
  - "04-defer-queued-task-launch"
plan: "plan.md"
spec: "../../specs/tasks/system-design/wip-limit-pull-system.md"
---

# Task 05: Adapt Integration Watchers

## Acceptance

- GitHub review, GitHub issue, and generic watcher dispatch treat queued task
  creation as accepted work.
- Watcher reservation/linkage stores the queued task ID so subsequent polls do
  not create duplicates.
- Watchers do not directly auto-start queued tasks; promotion uses the generic
  deferred launch path.
- Admitted watcher tasks preserve current auto-start behavior.
- A full configured feeder remains a classified capacity deferral with no task
  or permanent reservation.
- Seven review PRs targeting an empty no-feeder WIP-2 auto-start step create
  seven tasks, start exactly two, and queue five.

## TDD sequence

1. Change the review-watch WIP regression to expect seven durable tasks and
   five queued successes.
2. Add issue-watcher and generic-dispatch queued-success tests.
3. Add reservation dedup and full-feeder rollback tests.
4. Update adapters to branch on returned placement/admission state.
5. Retain the boot-ready versus genuine turn-complete workflow regression.

## Verification

```bash
cd apps/backend
go test -tags fts5 ./internal/orchestrator -run 'Test.*(ReviewTask|IssueTask|WatcherDispatch).*(Queued|Overflow|WIP|Reservation)' -count=1
```

## Files likely touched

- `apps/backend/internal/orchestrator/event_handlers_github.go`
- `apps/backend/internal/orchestrator/watcher_dispatch.go`
- watcher backend adapters
- `apps/backend/internal/orchestrator/event_handlers_github_review_test.go`
- GitHub issue and generic watcher dispatch tests

## Dependencies

- Task 04 supplies generic queued create-and-start behavior.

## Parallelism

`sequential`

## Output contract

Record each watcher path audited, reservation semantics, admitted-versus-queued
start counts, conflict behavior, files changed, and exact verification output.
