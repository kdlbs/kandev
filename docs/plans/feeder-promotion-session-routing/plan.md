---
created: 2026-08-25
status: complete
requirements:
  - REQ-TASKS-WIP-LIMIT-PULL-SYSTEM-001
system_design:
  - ../../specs/tasks/system-design/wip-limit-pull-system.md
legacy_specs: []
---

# Implementation Plan: Preserve Session Routing During Feeder Promotion

## Overview

Fix issue #3016. Task-service feeder promotion must publish the compatible
primary or active session on `task.moved`. Ordinary moves and the orchestrator
feeder-pull path already publish this session. This change restores the existing
destination-profile routing boundary. It does not change the event schema,
workflow configuration, or session-switching policy.

## Confirmed root cause

`Service.promoteFeederQueuedTask` commits the feeder-to-destination move through
the production atomic promoter. Both repository branches then publish
`task.moved` with an explicit empty session ID. As a result,
`handleTaskMoved` takes its no-session auto-start branch. This branch can create
a second session when a compatible primary session is in `WAITING_FOR_INPUT`.

A focused throwaway test reproduced the defect on the current branch. The task
`task-promoted` had primary session `session-primary`. The published event had
an empty `session_id`. The test failed with:

```text
promoted task session_id = "", want session-primary
```

Ordinary `MoveTaskWithOptions` resolves `resolvePrimaryOrActiveSession` before
publication. `workflowStore.pullOneFeederTask` also publishes an active session
ID. Existing profile-entry tests cover compatible-profile reuse and
different-profile switching after the orchestrator receives this ID.

## Scope

### In scope

- Resolve the routing session for the feeder candidate before the atomic
  promotion.
- Preserve the ordinary move rule: prefer an active primary session, otherwise
  use the most recently started active session.
- Keep lookup failure fail-closed and logged so promotion does not silently
  fall through to sessionless auto-start.
- Publish the resolved session ID from both task-service feeder-promotion
  repository branches.
- Add focused regression coverage for session propagation and lookup failure.

### Out of scope

- Changes to destination-profile compatibility or session retirement in the
  orchestrator.
- Changes to `task.moved` fields or external APIs.
- Clarification persistence or transfer between legitimately different
  sessions.
- Refactoring the older `workflowStore.pullOneFeederTask` implementation.
- UI, E2E, schema, migration, or public documentation changes.

## Technical approach

### Candidate preflight and event publication

Update `apps/backend/internal/task/service/service_workflow.go`. Make the
existing feeder-candidate preflight return eligibility and the lifecycle
session. Use the task-session list that the preflight already loads. An active
primary session wins. Otherwise, the newest active session wins. A `STARTING`
or `RUNNING` session continues to block promotion.

Pass the selected session ID into `promoteFeederQueuedTask`. Then pass the ID
into both `publishTaskMovedEvent` calls. Resolve the session before the atomic
task mutation. If the lookup fails, leave the candidate queued and try another
candidate. Do not commit the move with a sessionless event.

Do not use the routing session for step-ledger actor attribution. System-driven
WIP pulls keep their ledger `session_id` null. The event session identifies
lifecycle context. It does not identify the initiating actor.

### Regression boundary

Add a focused test file beside the task service. The first test uses a saturated
destination and a feeder candidate with a primary `WAITING_FOR_INPUT` session.
The test then opens a vacancy. It makes sure that the promoted event contains
the session ID. The second test injects a session-list error. It makes sure that
the candidate stays in the feeder without a promotion event.

Existing orchestrator tests prove both profile outcomes. The orchestrator reuses
a carried session for a compatible profile. It replaces the session for a
different destination profile.

## Tests

- `AC-TASKS-WIP-LIMIT-PULL-SYSTEM-001.2`: add
  `TestService_FeederPromotionCarriesPrimarySession` in
  `apps/backend/internal/task/service/service_workflow_feeder_session_test.go`.
- `AC-TASKS-WIP-LIMIT-PULL-SYSTEM-001.2`: add
  `TestService_FeederPromotionSkipsCandidateWhenSessionLookupFails` in the same
  file.
- `AC-TASKS-WIP-LIMIT-PULL-SYSTEM-001.2`: add
  `TestService_FeederPromotionBlocksOnOlderRunningSession` to make sure a
  primary session does not bypass a move-blocking secondary session.
- `AC-TASKS-WIP-LIMIT-PULL-SYSTEM-001.2`: add
  `TestService_FeederPromotionUsesNewestActiveFallback` for tasks without an
  active primary session.
- Preserve existing feeder admission and blocked-session coverage in
  `service_workflow_test.go`.
- Preserve existing compatible-profile reuse and different-profile switching
  coverage in `event_handlers_workflow_profile_test.go` and
  `event_handlers_workflow_moved_test.go`.

## Work orders

- [x] [Task 01: Propagate the feeder promotion session](task-01-propagate-feeder-promotion-session.md)

## Verification results

Task 01 is complete. The task-service feeder preflight now selects an active
primary session before the newest active fallback. Both task-service promotion
branches publish that session ID. Session-list errors still skip the candidate
before the atomic move.

PR fixup also made the preflight scan every session before selecting the
primary. Any `STARTING` or `RUNNING` session still blocks promotion. The
fallback path now has explicit regression coverage.

The exact work-order checks passed:

```text
go test -tags fts5 ./internal/task/service ... -count=1: 6 passed
go test -tags fts5 ./internal/orchestrator ... -count=1: 23 passed
```

The focused task-service checks also passed with `-race`: 6 tests passed.

## Risks

- Session selection must match ordinary moves when multiple active sessions
  exist. Selection of only the newest session can bypass an active primary.
- A lookup after the atomic promotion cannot fail closed. A transient database
  error can then preserve the duplicate-session problem.
- Step-ledger attribution and lifecycle routing use session IDs for different
  purposes. Coupling them regresses the system-origin WIP pull contract.
