---
status: shipped
created: 2026-05-03
owner: cfl
---

# Office: Multi-step blocker cycle detection

## Why

Today the office task system rejects two kinds of blocker cycles:

1. **Self-blocking** — `A blocks A`. Caught at the DB level via
   `CHECK (task_id != blocker_task_id)` on `task_blockers`.
2. **Direct two-node cycles** — `A blocks B` followed by `B blocks A`.
   Caught in `dashboard.AddTaskBlocker` with a single-step inverse
   lookup: when adding `B blocks A`, list `A`'s existing blockers
   and reject if `B` is among them.

It does **not** catch longer cycles. With three or more participants,
the user can construct a cycle the system silently accepts:

```
A blocks B
B blocks C
C blocks A   ← accepted today, creates a 3-node cycle
```

The cost shows up in the reactivity pipeline. `cascadeBlockersResolved`
in `scheduler/reactivity.go` walks each task's blockers and calls
`allBlockersResolvedExcept` to decide whether to wake the dependent.
A cycle member can never satisfy that condition because every member
is waiting on another. The result is a silent deadlock — none of the
cycle members ever wakes back up. The UI just shows "blocked by …"
forever, and the user has to manually break the cycle by removing a
blocker row.

This isn't a security issue and the UI eventually surfaces the stuck
state, but it's a real data-integrity hazard that's cheap to prevent
at the moment of insertion.

We matched this posture in v1 of `office-ux-parity`. Closing it now is purely
defensive.

## What

A pre-flight cycle check inside `dashboard.AddTaskBlocker`:

1. Before the insert, do a breadth-first walk starting at the
   proposed `blockerTaskID`, traversing **its** blocker chain via
   `repo.ListTaskBlockers`. Maintain a visited set to bound the walk.
2. If the source `taskID` is reachable from `blockerTaskID`, reject
   with a typed error.
3. The error response includes the cycle path so the user sees what
   they tried to do (e.g. `"would create cycle: A → B → C → A"`).
4. The frontend `blockers-picker` catches the error, surfaces the
   message in a toast, and rolls back its optimistic chip.

### Out of scope

- Background job to scan existing data for pre-existing cycles.
  The data is assumed clean (the fact that 99% of the wakeup path
  works fine implies cycles are rare in the wild).
- Detection on parent-child relationships. Parent cycles are a
  different domain (the spec already takes a "trust the data" stance
  there).
- Notifying agents that their blocker addition was rejected.
  The toast on the user's screen is the feedback channel.

## Acceptance

1. With existing rows `A blocks B` and `B blocks C`, attempting
   `POST /tasks/A/blockers { blocker_task_id: C }` returns 400 with
   a body containing the cycle path.
2. The blockers picker on the task detail page shows a toast
   reading something like *"Would create a blocker cycle: A → B → C → A"*
   and the optimistic chip is removed.
3. Existing single-step rejection (`A blocks B` → reject `B blocks A`)
   continues to work — no regression on the previous check.
4. Adding a non-cycling blocker (`D blocks A` when `A` blocks
   `B blocks C`) continues to succeed.
5. Backend tests cover: 3-node cycle, 4-node cycle, deep chain that
   does not cycle, and the existing 2-node cycle.
