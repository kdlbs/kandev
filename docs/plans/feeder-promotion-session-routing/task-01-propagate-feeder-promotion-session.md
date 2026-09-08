---
id: "01-propagate-feeder-promotion-session"
title: "Propagate the feeder promotion session"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-WIP-LIMIT-PULL-SYSTEM-001
acceptance_criteria:
  - AC-TASKS-WIP-LIMIT-PULL-SYSTEM-001.2
system_design:
  - ../../specs/tasks/system-design/wip-limit-pull-system.md
---

# Task 01: Propagate the Feeder Promotion Session

## Summary

Carry the compatible primary or active session through task-service feeder
promotion. The existing destination-profile preflight then decides whether to
reuse or switch sessions. Keep promotion fail-closed when the session preflight
cannot read its data.

## In scope

- Add red regression tests for feeder-promotion session propagation and lookup
  failure.
- Select an active primary session before the newest active fallback from the
  feeder-candidate session preflight.
- Pass the selected session ID through `promoteFeederQueuedTask` to both
  `publishTaskMovedEvent` calls.
- Preserve current blocking behavior for `STARTING` and `RUNNING` sessions.
- Run the focused task-service and orchestrator checks.

## Out of scope

- Orchestrator profile-routing policy changes.
- Event, API, persistence, or UI contract changes.
- Clarification handover across a genuine profile change.
- Step-ledger attribution changes.
- Public documentation changes.

## Acceptance

- A promoted feeder task with an active primary session publishes that session
  ID on `task.moved`. Existing profile reuse and profile switching then run.
- A session-list error leaves the candidate queued and does not publish a
  sessionless promotion event.
- A move-blocking session anywhere in the candidate session list prevents
  promotion, even when another active session is primary.
- A task without an active primary session uses its newest active fallback.
- Existing feeder blocking, promotion, profile reuse, and profile-switch tests
  remain green without schema, API, UI, or ledger changes.

## Verification

```bash
cd apps/backend
rtk go test -tags fts5 ./internal/task/service -run 'TestService_(FeederPromotionCarriesPrimarySession|FeederPromotionSkipsCandidateWhenSessionLookupFails|FeederPromotionBlocksOnOlderRunningSession|FeederPromotionUsesNewestActiveFallback|MoveTaskPullsNextFeederTaskOnVacate|MoveTaskPullSkipsBlockedFeederCandidate)' -count=1
rtk go test -tags fts5 ./internal/orchestrator -run 'Test(HandleTaskMovedWithSession|ProcessOnEnter_ProfileSwitch|SwitchSessionForStep_ReusesExistingProfileSession)' -count=1
```

## Files likely touched

- `apps/backend/internal/task/service/service_workflow.go`
- `apps/backend/internal/task/service/service_workflow_feeder_session_test.go`

## Dependencies

None.

## Risks

- The session selected for lifecycle routing must remain distinct from the
  system actor recorded for the WIP pull ledger row.
- Session lookup must happen before the atomic promotion to retain fail-closed
  behavior.

## Parallelism

`sequential`

## Inputs

- `REQ-TASKS-WIP-LIMIT-PULL-SYSTEM-001` and
  `AC-TASKS-WIP-LIMIT-PULL-SYSTEM-001.2`.
- `docs/specs/tasks/system-design/wip-limit-pull-system.md`, especially normal
  destination entry after feeder promotion.
- Issue #3016 and the failing event-payload reproduction.
- Existing ordinary move session resolution in
  `Service.MoveTaskWithOptions`.
- Existing feeder session propagation in
  `workflowStore.pullOneFeederTask`.
- Existing profile routing from issue #2692 / PR #2697.

## Results

Implemented the session propagation fix in
`apps/backend/internal/task/service/service_workflow.go`.

- `feederCandidateSession` preserves active-primary priority and newest-active
  fallback selection.
- `promoteFeederQueuedTask` publishes the selected session ID in both atomic
  and admission-repository branches.
- Session-list errors keep the candidate queued before promotion.
- Added `service_workflow_feeder_session_test.go` with propagation,
  lookup-failure, older-blocker, and newest-fallback coverage.

PR fixup remediation:

- `feederCandidateSession` scans every session before selecting the primary,
  so an older `STARTING` or `RUNNING` session cannot be skipped.
- The fallback path is covered with two active sessions and asserts the newest
  active session ID is published.

TDD evidence:

```text
RED: TestService_FeederPromotionCarriesPrimarySession failed with session_id = "".
GREEN: the two new tests passed after the production change.
RED (PR fixup): TestService_FeederPromotionBlocksOnOlderRunningSession promoted
the candidate while a primary session was found first.
GREEN (PR fixup): the blocker and fallback regressions pass after the full scan.
```

Verification:

```text
go test -tags fts5 ./internal/task/service ... -count=1: 4 passed
go test -tags fts5 ./internal/orchestrator ... -count=1: 23 passed
go test -tags fts5 -race ./internal/task/service ... -count=1: 4 passed
```
