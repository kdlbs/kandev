---
id: "06-ignore-idle-cancel"
title: "Ignore cancel on an idle session"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-CANCEL-TURN-PROGRESS-001
acceptance_criteria:
  - AC-UI-CANCEL-TURN-PROGRESS-001.13
system_design:
  - ../../specs/ui/system-design/cancel-turn-availability.md
---

# Task 06: Ignore cancel on an idle session

## Summary

A cancel that reaches a fully idle session succeeds without side effects, so a click just after a turn ends no longer reports a cancelled turn.

## In scope

- Add the idle-target check inside the guard in `runExplicitCancellationOwned`.
- Cover each state in the idle definition.

## Out of scope

- Control visibility in the browser.
- Send Now and internal turn cancellation.

## Acceptance

- An idle target creates no turn or status message, and it leaves Auto-run and the workflow unchanged, even with queued entries.
- Starting, running, clarification, resume, and in-flight dispatch cases keep their behavior.
- Pending progress clears after the no-op.

## Verification

```bash
(cd apps/backend && env -u KANDEV_DEBUG_AGENT_MESSAGES go test -trimpath ./internal/orchestrator -run 'Cancel' -count=1)
```

## Files likely touched

- `apps/backend/internal/orchestrator/task_operations.go`
- `apps/backend/internal/orchestrator/cancel_idle_target_test.go`

## Dependencies

None

## Risks

- An agent-started turn can begin just after the check. It then behaves as a new turn, which is the same as a later cancel click.

## Parallelism

`sequential`

## Inputs

- [Cancel design](../../specs/ui/system-design/cancel-turn-availability.md#cancel-that-arrives-after-the-turn-ends)

## Results

Done.

- `runExplicitCancellationOwned` checks the idle definition under the guard and
  returns success without creating a turn, recording a status message, pausing
  Auto-run, or evaluating workflow completion.
- The idle definition also treats an in-flight prompt admission (held or waiting
  `messagequeue` session admission) as live, so a cancel that races a dispatch
  still reaches the runtime.
- `cancel_idle_target_test.go` covers every state, each refinement (reserved
  turn, queue dispatch, resume attempt, clarification), and the user-visible
  no-op. `TestCancelAgent_DeduplicatesConcurrentCalls` and
  `TestAutoStartAgentCallback_CancellationBeforeResumeRegistration` were updated
  to the new no-op semantics; the latter still proves the stale workflow cannot
  bind a resume.
