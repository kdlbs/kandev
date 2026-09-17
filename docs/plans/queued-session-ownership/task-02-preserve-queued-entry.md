---
id: "02-preserve-queued-entry"
title: "Preserve queued entry lifecycle"
status: in_progress
wave: 2
depends_on:
  - "01-preserve-parked-sessions"
plan: "plan.md"
requirements:
  - REQ-TASKS-QUEUED-SESSION-OWNERSHIP-002
acceptance_criteria:
  - AC-TASKS-QUEUED-SESSION-OWNERSHIP-002.1
  - AC-TASKS-QUEUED-SESSION-OWNERSHIP-002.2
  - AC-TASKS-QUEUED-SESSION-OWNERSHIP-002.3
  - AC-TASKS-QUEUED-SESSION-OWNERSHIP-002.4
  - AC-TASKS-QUEUED-SESSION-OWNERSHIP-002.5
  - AC-TASKS-QUEUED-SESSION-OWNERSHIP-002.6
system_design:
  - ../../specs/tasks/system-design/queued-session-ownership.md
---

# Task 02: Preserve queued entry lifecycle

## Summary

Make task state and replay respect the exact queued workflow entry. A sibling
ready event must not hide Luna's queue, and a stale replay must not affect a
newer entry.

## In scope

- Bind workflow deferrals to the committed route/step/session identity.
- Guard review-state reconciliation with working-session and valid-deferral
  predicates, including restoration from legacy REVIEW plus accepted deferral.
- Serialize enqueue/state writes and observed-record dispatch/clear against
  successor records. Preserve prompt, turn, and session identity across retries.
- Dispose obsolete/deleted/terminal destinations without retargeting; preserve
  records on transient failures. Keep archive/cancel and Office precedence.
- Extend the desktop E2E flow to release capacity and verify Luna's one delivery.

## Out of scope

Queue rendering and summary schema belong to Task 03. No new scheduler, generic
retry framework, WIP policy, global ordering, or profile-selection algorithm.

## Acceptance

1. `TestQueuedSessionOwnership` fails before the fix when a sibling boot-ready
   event turns SCHEDULING into REVIEW. Mixed CREATED+idle/failed/cancelled cases
   preserve Scheduling; explicit sibling running and settlement retain queue ownership.
2. `TestQueuedEntryReplay` proves one destination delivery, restart recovery,
   old-record compatibility, invalidation on step change/deletion/cancellation,
   and transient-error retention without retargeting or duplicate prompts.
3. Barrier-controlled races prove an old REVIEW write or replay clear cannot
   overwrite a new record. Stale callbacks cannot activate parked Astra.

## Verification

```bash
(cd apps/backend && go test -race ./internal/orchestrator -run 'TestQueuedSessionOwnership|TestQueuedEntryReplay|Test.*Ceiling|Test.*ReviewState|Test.*BootReady' -count=1)
(cd apps/backend && go test ./internal/task/repository/sqlite -run 'Test.*DeferredLaunch|Test.*QueuedEntry' -count=1)
(cd apps/backend && test -n "$KANDEV_TEST_POSTGRES_DSN" && go test ./internal/task/repository/sqlite -run 'TestPostgresQueuedEntry' -count=1)
(cd apps/backend && go test ./internal/task/service -run 'Test.*DeferredLaunch|Test.*Workflow' -count=1)
make -C apps/backend build
(cd apps/web && pnpm e2e:run --project chromium tests/workflow/queued-session-ownership.spec.ts tests/workflow/workflow-session-targeting.spec.ts)
git diff --check
```

Use a controllable runtime and real repository fixtures for RED and race cases.
Do not substitute timing sleeps for an ordered conflicting operation. Record
actual database coverage. `TestPostgresQueuedEntry` is a planned isolated-schema
test using `KANDEV_TEST_POSTGRES_DSN` from Task 01. A missing database blocks this
check; a skipped test is not verification. Keep the work order pending until its
required database checks pass.

## Files likely touched

- `apps/backend/internal/orchestrator/event_handlers_streaming.go`,
  `event_handlers_agent.go`, `event_handlers_workflow.go`.
- `apps/backend/internal/orchestrator/ceiling_defer.go`, `ceiling_replay.go`,
  `ceiling_seam2.go`, `ceiling_seam3.go`, and workflow route integration.
- `apps/backend/internal/task/models/ceiling_launch.go` and route model helpers.
- `apps/backend/internal/task/repository/interface.go`,
  `sqlite/deferred_launch_cas.go`, `sqlite/task.go`, and corresponding tests.
- New `apps/backend/internal/orchestrator/queued_session_ownership_test.go`.
- Workflow E2E file/helper introduced in Task 01.

## Dependencies

Task 01 establishes parked policy and passive launch outcomes. Its inspection
tests remain mandatory compatibility evidence.

## Risks

Task-state serialization alone does not protect against an independently written
deferred record. Guard identity at the persistence boundary. Validity checks
must distinguish source read errors from proven obsolete work.

## Parallelism

`sequential`

## Inputs

[Design: deferred entry ownership and task reconciliation](../../specs/tasks/system-design/queued-session-ownership.md#deferred-entry-ownership).
Read existing `ceiling_replay_test.go`, `ceiling_defer_test.go`,
`event_handlers_workflow_deferred_launch_race_test.go`, and deferred CAS tests.

## Results

Implementation is present. Deferred entries retain the exact workflow route,
step, destination session, prompt, and queue time. Review-state reconciliation,
replay clearing, and Send Now admission use the destination identity and
compare-and-set rules. A created destination now takes the explicit manual
Send Now path, and a failed dispatch remains restorable by the queue worker.

Passing checks:

- `go test -race ./internal/orchestrator -run
  'TestQueuedSession|TestQueuedEntry|Test.*Ceiling.*Surface|Test.*LaunchQueue'
  -count=1`
- `go test ./internal/task/repository/sqlite -run
  'Test.*DeferredLaunch|Test.*QueuedEntry' -count=1`
- `go test ./internal/task/service -run
  'Test.*DeferredLaunch|Test.*Workflow' -count=1`
- `go test ./internal/orchestrator -run
  'TestPromptSendNowClaimStartsCreatedSessionManually|TestPromptSendNowClaimSkipsOnTurnStartWhenAlreadyProcessed'
  -count=1`
- `make -C apps/backend build`
- `pnpm e2e:run --project chromium tests/workflow/queued-session-ownership.spec.ts
  tests/workflow/workflow-session-targeting.spec.ts` (6 tests passed)
- `git diff --check`

The required PostgreSQL queued-entry check remains outstanding because
`KANDEV_TEST_POSTGRES_DSN` is unset. The work order remains in progress until
the isolated PostgreSQL verification runs.
