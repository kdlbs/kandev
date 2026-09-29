---
id: "12-goals-backend"
title: "Goals backend"
status: pending
wave: 3
depends_on:
  - "01-shared-interface"
  - "03-activity-log-backend"
plan: "plan.md"
requirements:
  - REQ-COORDINATOR-GOALS-001
  - REQ-COORDINATOR-GOALS-003
acceptance_criteria:
  - AC-COORDINATOR-GOALS-001.1
  - AC-COORDINATOR-GOALS-001.2
  - AC-COORDINATOR-GOALS-001.3
  - AC-COORDINATOR-GOALS-001.4
  - AC-COORDINATOR-GOALS-001.5
  - AC-COORDINATOR-GOALS-001.6
  - AC-COORDINATOR-GOALS-001.7
  - AC-COORDINATOR-GOALS-001.8
  - AC-COORDINATOR-GOALS-001.10
  - AC-COORDINATOR-GOALS-003.1
  - AC-COORDINATOR-GOALS-003.2
  - AC-COORDINATOR-GOALS-003.3
  - AC-COORDINATOR-GOALS-003.4
system_design:
  - ../../specs/coordinator/system-design/goals.md
---

# Task 12: Goals Backend (WP-10)

## Summary

Serve the goal: its routes, the instruction section a conversation opens
with, the baseline recorded in the goal's transaction and the measures
computed at read time.

## In scope

- Goal: `GET goal`, `PUT goal` (create or update in place, criteria keep
  their done state by id; an unknown or repeated id is 400 naming
  `criteria[i].id`; an omitted criterion is removed), `POST
  goal/criteria/:crid` done toggle under the per-coordinator lock,
  `POST goal/met` (optional `goal_id`, 409 like `PUT`; a retry naming the last met goal returns it), 403 for readers (`001.1` to `001.5`,
  `001.8`, `001.10`: an optional `goal_id` on `PUT` and `met` that is not the active goal is 409, checked before criterion ids).
- Goal instruction section with done states, or "No goal is set"
  (`001.6`); `resetConversation` on name, due, criteria, met and new goal,
  not on a done toggle (`001.7`).
- Baselines in the activation transaction: Open tasks over watched tasks,
  Approved and Rejected over 7 days, none when the coordinator is younger
  than 7 days (`003.1`, `003.2`); current values and direction with the
  threshold of 2 at read time (`003.3`, `003.4`).

## Out of scope

- The Goal section and the goal note (task 11).
- Standing orders (task 05).

## Acceptance

- A goal's baseline is written once, with the goal, and never recomputed.
- A change to what a conversation is told resets it; a done toggle does not.

## Verification

```bash
make -C apps/backend test PKG=./internal/coordinator/...
```

Tests: the instruction builder output with a met, active or absent goal
(golden text); baseline with a coordinator 6 and 8 days old; direction at
deltas 1, 2 and -2; open_tasks excludes automation_run tasks; a stale `goal_id` with old criterion ids is 409 not 400; met with a stale `goal_id` is 409; a goal read failure omits only the goal section; two concurrent creates serialize on the
lock and the second applies as an update of the first (the unique index is a
backstop); a stale `goal_id` is 409.

## Likely files

- `apps/backend/internal/coordinator/goals.go`, `goal_routes.go`,
  `measures.go` (goal section is `GoalInstructionSection` in `goals.go`, wired in `apps/backend/internal/backendapp/coordinator.go`)

## Dependencies

- Task 01 (tables, `resetConversation`); task 03 (the summary read shares
  the proposal counts used by measures).

## Risks

- Measures read many tasks; they use one new `Store.CountOpenWatchedTasks`
  count query in `measures.go` (phase 1 has no such query), not a per-task
  loop. The baseline log counts use the new exec-taking
  `Store.ActivityCountsIn` on the locked handle, not `Service.ActivitySummary`.
- Task 05 adds its section to the same instruction builder; each work order
  adds only its own section.
