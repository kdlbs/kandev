---
id: "01-assignment-wake-rate-limit"
title: "Bound agent-initiated assignment wakes per task"
status: pending
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-OFFICE-ASSIGN-RATE-001
  - REQ-OFFICE-ASSIGN-RATE-002
  - REQ-OFFICE-ASSIGN-RATE-003
acceptance_criteria:
  - AC-OFFICE-ASSIGN-RATE-001.1
  - AC-OFFICE-ASSIGN-RATE-001.2
  - AC-OFFICE-ASSIGN-RATE-001.3
  - AC-OFFICE-ASSIGN-RATE-001.4
  - AC-OFFICE-ASSIGN-RATE-001.5
  - AC-OFFICE-ASSIGN-RATE-001.6
  - AC-OFFICE-ASSIGN-RATE-001.7
  - AC-OFFICE-ASSIGN-RATE-001.8
  - AC-OFFICE-ASSIGN-RATE-001.9
  - AC-OFFICE-ASSIGN-RATE-001.10
  - AC-OFFICE-ASSIGN-RATE-001.11
  - AC-OFFICE-ASSIGN-RATE-001.12
  - AC-OFFICE-ASSIGN-RATE-001.13
  - AC-OFFICE-ASSIGN-RATE-002.1
  - AC-OFFICE-ASSIGN-RATE-002.2
  - AC-OFFICE-ASSIGN-RATE-002.3
  - AC-OFFICE-ASSIGN-RATE-002.4
  - AC-OFFICE-ASSIGN-RATE-002.5
  - AC-OFFICE-ASSIGN-RATE-002.6
  - AC-OFFICE-ASSIGN-RATE-003.1
  - AC-OFFICE-ASSIGN-RATE-003.2
  - AC-OFFICE-ASSIGN-RATE-003.3
  - AC-OFFICE-ASSIGN-RATE-003.4
  - AC-OFFICE-ASSIGN-RATE-003.5
  - AC-OFFICE-ASSIGN-RATE-003.6
system_design:
  - ../../specs/office/system-design/assignment-wake-rate-limit-01.md
---

# Task 01: Bound agent-initiated assignment wakes per task

## Summary

Add one admission gate to the Office run queue that refuses an agent-initiated
`task_assigned` wake once a task has already produced `N` of them inside a
rolling window `W`. The gate is the last check before insertion, so "admitted"
means "a row was written" and the count needs no state of its own.

## Scope

- A window-count repository read over the **stored** `runs` rows, filtered by
  `reason`, the payload's `task_id` and `actor_type`, and `requested_at`, using
  `dialect.JSONExtract` and the string-type guard `CoalesceRun` already needs.
- The **incoming** wake's `actor_type` and `task_id` read in Go, by unmarshalling
  the payload `queueRun` was handed, the way `taskIDFromPayload` does.
  `dialect.JSONExtract` is SQL over a column and does not apply to a wake that is
  not a row yet; the two reads are separate.
- The gate in `SchedulerService.queueRun`, positioned after `CheckIdempotencyKey`
  and `CoalesceRun` and before `CreateRun`. That position binds the
  task-mutation producer only (`AC-OFFICE-ASSIGN-RATE-001.12`); no other
  `task_assigned` producer routes through this function.
- `RunContext.ActorType` reaching the payload for the mutation producer's
  agent-actor wakes, pinned by a test
  (`AC-OFFICE-ASSIGN-RATE-001.13`). A wake carrying no `actor_type` is out of
  scope: admit, no allowance consumed, no counter moved.
- `N = 5` and `W = 10 * time.Minute` declared once beside
  `CoalesceWindowSeconds` and `IdempotencyWindowHours`.
- A new `QueueOutcome` value added to **both** declarations
  (`internal/runs/service` and `internal/workflow/engine/adapters.go`).
- A reporting helper beside `ReportWindowedDedup` / `ReportDurableDedup`, a new
  `expvar.Map` beside `office_run_dedup_total`, and a Warn log carrying task id,
  assignee profile id, acting agent id, observed count and allowance.

## Exclusions

- No change to dedup key construction, the coalescing predicate, the 24-hour
  lookback, provenance classification, or budget admission.
- No change to `SetTaskAssigneeAsAgent`'s permission check or to the mutation
  itself: a refused wake still persists the assignee, bumps
  `assignment_generation`, publishes the task update, and interrupts a displaced
  assignee's session.
- No bound on the other four `task_assigned` producers: the task-lifecycle event
  subscriber (`service/event_subscribers.go`), onboarding, the recovery sweep,
  and the orchestrator's workflow auto-start. None carries an actor, and a
  per-task allowance cannot bind a producer that wakes a task just created,
  onboarded or swept. Do not "fix" this by giving one of them an actor.
- No ordering or tiebreak logic. `AC-OFFICE-ASSIGN-RATE-001.10` makes the
  decision order-independent; no `ORDER BY` belongs in the window count.
- No frontend, no migration, no new configuration surface.

## Implementation acceptance conditions

1. `N` agent-actor assignment wakes for one task admit and the next refuses,
   writing no row and returning the new outcome; a `user`-actor wake and an
   empty-`callerAgentID` call are never refused, and neither is a wake carrying
   no `actor_type` at all.
2. A wake merged by coalescing or suppressed by the idempotency lookup neither
   consumes the allowance nor can be refused by it; a refused wake does not
   consume it either.
3. A failing count read and an unattributable task both **admit**, each
   incrementing the counter under its own `reason` label, with no identifier
   used as a label. Unattributable covers an absent, null or empty `task_id` and
   a present-but-non-string one, identically; an unparseable payload is *not*
   unattributable — it fails the actor step first and leaves scope with no
   counter moved.
4. A test enumerates the `task_assigned` producers and asserts the actor type
   each emits, pinning `AC-OFFICE-ASSIGN-RATE-001.12`'s invariant that no other
   producer sets `actor_type` `agent`.
5. A mixed-actor coalesce is covered in both directions and asserts the count the
   gate then observes, which is the accepted off-by-one in the requirement's
   `## Accepted consequence` rather than an exact count.

## Verification

```bash
cd apps/backend && go test ./internal/office/scheduler ./internal/office/service \
  ./internal/runs/service ./internal/workflow/engine
cd apps/backend && make lint
```

## Likely files

- `apps/backend/internal/office/scheduler/run.go`
- `apps/backend/internal/office/scheduler/reactivity.go`
- `apps/backend/internal/runs/service/dedup.go`
- `apps/backend/internal/runs/service/metrics_vars.go`
- `apps/backend/internal/workflow/engine/adapters.go`
- `apps/backend/internal/runs/repository/**` (window-count query, SQLite and
  PostgreSQL twins)

## Risks

- The `QueueOutcome` enum is declared twice and must stay in sync; adding the
  value to one declaration only breaks the engine adapter's reporting.
- A hand-written `json_extract` diverges between SQLite and PostgreSQL. Use
  `dialect.JSONExtract` and guard the JSON string type, or a numeric `task_id`
  in an older payload silently joins another task's allowance.
- Tests must inject time (`synctest`), never `time.Sleep`.
- `CoalesceRun` overwrites the surviving row's payload and does not match on
  `actor_type`, so a mixed-actor merge relabels the actor the count reads. This
  is an accepted consequence, not a bug to fix here: do not add `actor_type` to
  the coalescing predicate, and do not write a test that asserts an exact count
  across such a merge.
