---
id: "02-implement-safe-session-parking"
title: "Route source and destination behavior"
status: pending
wave: 2
depends_on:
  - "01-add-portable-workflow-policy"
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKFLOW-PROFILE-SESSIONS-001
acceptance_criteria:
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.1
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.2
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.3
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.4
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.5
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.6
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.11
system_design:
  - ../../specs/tasks/system-design/workflow-profile-session-lifecycle.md
---

# Task 02: Route Source and Destination Behavior

## Summary

Use the destination step for session selection and the source step for session
retirement.

## In scope

- Split profile-switch routing into start and end inputs.
- Thread the source step or normalized end setting through every entry path.
- Keep credential preflight, promotion, queue rollback, parking, stop intent,
  guard release, and callback suppression.
- Cover legacy, manual, queued, and direct engine transitions.

## Acceptance

- Tests prove all four start-and-end combinations.
- A destination setting cannot change source retirement.
- The workflow engine core gains no action, event, or state.

## Verification

```bash
cd apps/backend && go test -tags fts5 ./internal/orchestrator -run 'Test(SwitchSessionForStep|PrepareWorkflowStepSession|HandleAgentCompleted|HandleAgentStopped|HandleAgentFailed).*ProfileSession' -count=1 -v
cd apps/web && pnpm e2e:run --host --no-build --shards 1 --project chromium tests/workflow/queued-session-ownership.spec.ts -- --retries=0 --repeat-each=3
cd apps/web && pnpm e2e:run --host --no-build --shards 1 --project mobile-chrome tests/workflow/mobile-queued-session-ownership.spec.ts -- --retries=0 --repeat-each=3
cd apps/web && pnpm e2e:run --host --no-build --shards 1 --project mobile-chrome tests/task/mobile-task-create-workflow-step-previews.spec.ts -- --grep 'touch scrolls ten workflow options and selects either end' --retries=0 --repeat-each=3
```

## Files likely touched

- `apps/backend/internal/orchestrator/event_handlers_workflow.go`
- `apps/backend/internal/orchestrator/workflow_callbacks.go`
- `apps/backend/internal/orchestrator/event_handlers_workflow_profile_session_policy_test.go`
- `apps/backend/internal/orchestrator/workflow_profile_session_lifecycle.go`

## Dependencies

Task 01.

## Risks

A direct engine entry can lose the source step after the task moves.

## Parallelism

`sequential`

## Results

### CI regression follow-up, 2026-10-03

Hosted queue-ownership failures showed that an `agent.failed` callback from the
stream closed by a deliberate profile-switch stop bypassed the exact execution
teardown claim. That callback created a recovery message for a parked session.
The handler now consumes only the matching park stop intent or teardown owner,
retires that execution, and suppresses generic session-failure handling. A
different execution cannot consume the stop intent. The owning requirement and
design now state that this is an intentional parked outcome; no storage or
protocol change was needed.

The matching regression failed before the handler change and passes afterward.
The test also verifies the turn remains open, the session remains
`WAITING_FOR_INPUT`, and no recovery message appears. The mismatch test verifies
that a successor execution leaves the old stop intent untouched. The focused
profile-session test command and the orchestrator package suite passed. Changed
scope Go lint reported zero issues.

All three failed hosted E2E cases passed three repetitions with retries
disabled: desktop queue ownership, mobile queue ownership, and the mobile
workflow-preview scroll case. The preview assertion again checks that expected
step names appear in order while allowing additional configured steps.

```bash
cd apps/backend && go test ./internal/orchestrator -run 'TestHandleAgentFailed_ProfileSessionStopIntent' -count=1
cd apps/backend && go test ./internal/orchestrator/... -count=1
cd apps/backend && golangci-lint run ./internal/orchestrator/... --timeout=5m
cd apps/web && pnpm lint
cd apps/web && pnpm exec prettier --check e2e/tests/task/workflow-step-previews-helpers.ts
```
