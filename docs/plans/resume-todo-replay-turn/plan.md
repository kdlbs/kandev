---
created: 2026-09-29
status: done
requirements:
  - REQ-TASKS-WORKFLOW-EXPLICIT-COMPLETION-SIGNAL-004
system_design:
  - ../../specs/tasks/system-design/workflow-explicit-completion-signal.md
legacy_specs: []
---

# Fix plan: todo replay on resume opens a stale turn

## Overview

[Issue #4047](https://github.com/kdlbs/kandev/issues/4047) reports that
`step_complete_kandev` is rejected for a whole rework turn after a reviewer
bounces a task back to Work. The turn that receives the Work prompt was opened
earlier, without a prompt, while the task was in Review. One work order stops
todo replay from opening that turn.

## Evidence and root cause

Investigation base: `cb9a53000`. Production evidence comes from kandev v0.95.0
on two tasks; turn rows, transition rows, and backend log lines are in the issue.

On task `c9d91d7d`, the parked worker session is resumed with
`seam: resumeTaskSession, origin: automatic` three seconds after the task moves
to Review. The resume writes a `lifecycle_only` turn, then the replayed todo
list arrives. `persistTodoMessage` calls `getActiveTurnID`, which starts a turn
when none is active. `CreateTurnWithStepStamp` stamps it with the task's current
step, Review. Nothing prompts that turn, so it stays open.

When the reviewer moves the task back to Work, the Work prompt is dispatched to
the reused worker session. `startTurnForSessionWithOwnershipChecked` adopts the
open turn instead of starting one. `stepCompletionLaunchStep` returns the Review
stamp, and every completion attempt is rejected.

`handleSessionStatusEvent` already avoids this for the "Session resumed" message
by using `currentTurnIDForSession`, which never creates a turn. That path is
covered by `TestResumedSessionStatusDoesNotCreateTurnForWaitingSession`.
The todo path had no such guard.

What triggers the automatic resume is not established here. This package does
not change resume scheduling.

## Scope

### In scope

- Resolve the todo message's turn without creating one.
- Regression tests for the no-active-turn and active-turn cases.

### Out of scope

- Resume scheduling, `replayCeilingLaunchResume`, or session reuse policy.
- Restamping turns, relaxing the launch-stamp guard, or turn adoption rules.
- Other lazy-turn producers that do not run on resume replay.
- UI, migrations, or public docs; no user-visible setting or command changes.

## Technical approach

Follow the [system design](../../specs/tasks/system-design/workflow-explicit-completion-signal.md)
section "Resume replay turn ownership". In `persistTodoMessage`, replace
`getActiveTurnID` with `currentTurnIDForSession` and return early when it is
empty. `handleSessionTodosEvent` keeps publishing `SessionTodosUpdated` first.

## Tests

| Acceptance | Regression evidence |
| --- | --- |
| 004.1 | `event_handlers_streaming_test.go:TestReplayedTodosWithoutActiveTurnDoNotCreateTurn` |
| 004.2 | `event_handlers_streaming_test.go:TestTodosAttachToExistingActiveTurn` |
| 004.3 | Follows from 004.1: with no open turn, `startTurnForSessionWithOwnershipChecked` creates one stamped by `CreateTurnWithStepStamp`, already covered by the turn stamp tests |

All short suffixes refer to `AC-TASKS-WORKFLOW-EXPLICIT-COMPLETION-SIGNAL-004.*`.
The 004.1 test failed on the unmodified code (one open turn) and passes with the fix.

## Work orders

- [x] [Task 01: resolve the todo turn without creating one](task-01-todo-turn-ownership.md)
