---
id: "02-deliver-to-switched-session"
title: "Deliver a queued message to the switched-to session"
status: done
wave: 2
depends_on:
  - "01-retain-claim-guard"
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKFLOW-PROFILE-SESSIONS-001
acceptance_criteria:
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.15
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.16
system_design:
  - ../../specs/tasks/system-design/workflow-profile-session-lifecycle.md
---

# Task 02: Deliver a queued message to the switched-to session

## Summary

After Task 01, a queued message whose `on_turn_start` switched the task to
another profile session was silently lost: the prompt to the source failed with
`session reset in progress` and the requeue was rejected with `queue dispatch
claim changed`. A queued plan comment kept its reservation, so the transfer
failed and the comment was never delivered. Deliver both exactly once to the
session the switch makes primary, the way the direct send path does.

## In scope

- When the current step has an `on_turn_start` transition, the after-claim
  callback returns the claimed entry to the head of its source queue with
  `turn_start_already_processed`, runs `on_turn_start` under the held guard,
  and ends the claim with `queuedTurnStartRestartError`.
- The worker rolls back the source claim without recording or prompting and
  drains the session that is primary afterwards; a failed transition leaves the
  entry queued on the source.
- The plan-comment transcript is recorded in the after-claim callback, after
  `on_turn_start`; the delivery boundary no longer runs `on_turn_start`.
- The SQLite queue requeues a reserved delivery through its reservation token
  instead of requiring an ordinary dispatch claim.

## Out of scope

Lifecycle (PR automation) and managed-input deliveries keep the in-place
`on_turn_start` path. Guard acquisition, identity checks, stamped parking and
deferred runtime teardown are unchanged.

## Acceptance

1. A queued message whose `on_turn_start` switches profiles is recorded and
   prompted once on the destination session; its attachment moves with it; no
   entry remains on either queue.
2. A queued plan comment in the same situation is recorded and prompted once on
   the destination session; the task is on the destination step with the
   destination primary.
3. If the destination promotion fails, the message stays queued on the source,
   is not recorded or prompted, and the source stays primary.

## Verification

- `go test ./internal/orchestrator/ -run 'QueuedTurnStartProfileSwitch|QueuedPlanCommentTurnStartProfileSwitch'`
  runs each case on the legacy and engine paths; the message and failure cases
  also run on the SQLite queue repository. On the Task 01 source the delivery
  cases find no prompt and the failure case finds the message prompted to the
  source; all pass with the change.

## Files likely touched

- `apps/backend/internal/orchestrator/event_handlers_agent.go`
- `apps/backend/internal/orchestrator/queued_turn_start_profile_switch.go`
- `apps/backend/internal/orchestrator/task_operations.go`
- `apps/backend/internal/orchestrator/messagequeue/repository_sqlite.go`
- `apps/backend/internal/orchestrator/queued_turn_start_profile_switch_test.go`
- `docs/specs/tasks/requirements/workflow-profile-session-lifecycle.md`
- `docs/specs/tasks/system-design/workflow-profile-session-lifecycle.md`

## Dependencies

Task 01.

## Inputs

- [Owning requirement](../../specs/tasks/requirements/workflow-profile-session-lifecycle.md)
- [Owning design](../../specs/tasks/system-design/workflow-profile-session-lifecycle.md)
- [Plan](plan.md)

## Risks

The restart costs one extra queue round trip for entries on steps with an
`on_turn_start` transition. The entry carries `turn_start_already_processed`
before `on_turn_start` runs, so a failed transition is not retried by the next
delivery of that entry.

## Parallelism

`sequential`
