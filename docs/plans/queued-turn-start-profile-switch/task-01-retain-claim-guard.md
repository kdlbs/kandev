---
id: "01-retain-claim-guard"
title: "Retain the queued claim guard through a profile switch"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKFLOW-PROFILE-SESSIONS-001
acceptance_criteria:
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.4
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.15
system_design:
  - ../../specs/tasks/system-design/workflow-profile-session-lifecycle.md
---

# Task 01: Retain the queued claim guard through a profile switch

## Summary

Stop a queued prompt from deadlocking when its `on_turn_start` action switches
the task to a step with a different agent profile and the source step parks
its session.

## In scope

- `queuedMessageAfterClaim` wraps the context it passes to
  `processOnTurnStartViaEngine` with `withWorkflowProfileSwitchGuardHeld` for
  the queued session.
- A permanent orchestrator test drains a queued prompt through the real claim
  on the legacy and workflow-engine paths.

## Out of scope

Guard acquisition order, identity checks, stamped parking, deferred runtime
teardown, and unblocking a process that is already stalled.

## Acceptance

1. A queued prompt whose `on_turn_start` moves the task to a step with another
   profile finishes dispatching; the task reaches the destination step.
2. The source session is parked (nonprimary, nonterminal).

## Verification

- `go test ./internal/orchestrator/ -run TestQueuedTurnStartProfileSwitchParksSourceUnderClaimGuard`
  fails on the previous source after 10 seconds per subtest with the dispatch
  goroutine blocked in `parkSessionForProfileSwitch`, and passes with the
  change, including 10 repeated runs under `-race`.
- Task 02 replaced that test with tests that also check exactly-once delivery
  to the destination session.

## Files likely touched

- `apps/backend/internal/orchestrator/event_handlers_agent.go`
- `apps/backend/internal/orchestrator/queued_turn_start_profile_switch_test.go`
- `docs/specs/tasks/requirements/workflow-profile-session-lifecycle.md`
- `docs/specs/tasks/system-design/workflow-profile-session-lifecycle.md`

## Dependencies

None.

## Inputs

- [Owning requirement](../../specs/tasks/requirements/workflow-profile-session-lifecycle.md)
- [Owning design](../../specs/tasks/system-design/workflow-profile-session-lifecycle.md)
- [Plan](plan.md)

## Risks

The held-guard marker must only be set while the claim's guard is actually
held; it is applied inside the synchronous after-claim callback only.

## Parallelism

`sequential`
