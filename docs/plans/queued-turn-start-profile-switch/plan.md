---
created: 2026-10-20
status: done
requirements:
  - REQ-TASKS-WORKFLOW-PROFILE-SESSIONS-001
system_design:
  - ../../specs/tasks/system-design/workflow-profile-session-lifecycle.md
legacy_specs: []
---

# Implementation Plan: Retain the queued claim guard through a profile switch

## Overview

A queued prompt is claimed while the source session's lifecycle guard is held,
and its after-claim callback runs `on_turn_start` before the guard is released.
When that action moves the task to a step with a different agent profile and
the source step parks its session, profile-switch parking tried to acquire the
same non-reentrant guard and the queued dispatch deadlocked. The task never
reached the destination step, the prompt was never delivered, and the session's
queue stayed blocked. Task 01 passes the existing guard ownership into the
synchronous workflow handling.

Removing the deadlock exposed a silent loss. The switch transferred the source
queue (and, with durable queue storage, the in-flight dispatch claim) to the
destination and bumped the source generation. The source worker had already
recorded the user message on the source; its prompt then failed with `session
reset in progress` and its requeue was rejected with `queue dispatch claim
changed`, so the message ended up in no queue and was never prompted. A queued
plan comment kept its reservation in the source queue, which made the transfer
fail and left the task on the destination step with the source still primary.
Task 02 delivers such a message exactly once to the session the switch makes
primary.

## Scope

In scope: `queuedMessageAfterClaim` and the plan-comment delivery boundary in
`apps/backend/internal/orchestrator/event_handlers_agent.go`, the queued
on_turn_start restart in `queued_turn_start_profile_switch.go`, the prompt-claim
acknowledgement in `task_operations.go`, reserved-delivery requeue in the
SQLite queue repository, permanent orchestrator tests, and the profile-session
lifecycle requirement and design.

Excluded: guard acquisition, session identity checks, stamped parking, deferred
runtime teardown, recovery of an already-stalled process, and lifecycle
(PR automation) and managed-input deliveries, which keep their in-place
`on_turn_start` path.

## Technical approach

`parkSessionForProfileSwitch` already supports callers that own the guard
through `withWorkflowProfileSwitchGuardHeld`; it then commits the park claim
in place and schedules runtime teardown after the caller returns. The queued
after-claim callback now marks its context the same way before calling
`processOnTurnStartViaEngine`.

For delivery, the queue path mirrors the direct send path, where
`on_turn_start` runs before the user message is recorded and
`resolveSessionAfterTurnStart` picks the session to prompt. When the current
step has an `on_turn_start` transition, the after-claim callback returns the
claimed entry to the head of its source queue with
`turn_start_already_processed` set, then runs `on_turn_start` under the held
guard. A profile switch moves the entry and its attachments through the
ordinary queue transfer. The callback ends the claim with
`queuedTurnStartRestartError`; the worker rolls back the source claim without
recording or prompting and drains the session that is primary afterwards. If
the transition fails and the source stays primary, the entry stays queued on
the source. The plan-comment transcript is recorded in the after-claim callback
after this step, so it lands on the delivering session.

## Work orders

- [x] [Task 01: Retain the queued claim guard through a profile switch](task-01-retain-claim-guard.md)
- [x] [Task 02: Deliver a queued message to the switched-to session](task-02-deliver-to-switched-session.md)

## Changed-path coverage

The work order owns the `event_handlers_agent.go` change, the permanent
`queued_turn_start_profile_switch_test.go` regression test, the extended
`docs/specs/tasks/requirements/workflow-profile-session-lifecycle.md`
acceptance criteria, and the
`docs/specs/tasks/system-design/workflow-profile-session-lifecycle.md` control
flow step 11. Task 02 owns `queued_turn_start_profile_switch.go`, the
`event_handlers_agent.go` delivery changes, the `task_operations.go` claim
acknowledgement, the `messagequeue/repository_sqlite.go` reserved-delivery
requeue, the rewritten `queued_turn_start_profile_switch_test.go`, acceptance
criterion 001.16, and the design's queued-delivery paragraph.

## Verification

`TestQueuedTurnStartProfileSwitchParksSourceUnderClaimGuard` drains a real
queued prompt through the claim and after-claim callback on both the legacy and
workflow-engine paths. It times out on the previous source with the dispatch
goroutine blocked in `parkSessionForProfileSwitch` and passes with the change.

Task 02 replaces that test with
`TestQueuedTurnStartProfileSwitchDeliversOnceToDestination`,
`TestQueuedTurnStartProfileSwitchFailureKeepsMessageQueuedOnSource` and
`TestQueuedPlanCommentTurnStartProfileSwitchDeliversToDestination` on the
legacy and engine paths. The first two also run on the SQLite queue
repository; the plan-comment case uses the in-memory queue because a reserved
plan comment drained through the SQLite FIFO path is not prompted even without
a profile switch, a separate existing defect.
On the Task 01 source the first and third find no prompt and the second finds
the message prompted to the source; all pass with the change.

## Risks

- Marking the guard as held outside its real owner would let parking race
  terminal callbacks; the marker is set only inside the synchronous
  after-claim callback that runs under the claim's guard.
- A queued entry on a step with an `on_turn_start` transition now takes one
  extra queue round trip before delivery; steps without such a transition keep
  the in-place path.
