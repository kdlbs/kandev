---
id: "02-implement-safe-session-parking"
title: "Implement safe session parking"
status: done
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
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.9
system_design:
  - ../../specs/tasks/system-design/workflow-profile-session-lifecycle.md
---

# Task 02: Implement Safe Session Parking

## Summary

Apply the workflow policy during fixed-profile routing. Preserve answerable
sessions without keeping runtimes alive, and suppress only the delayed lifecycle
events caused by the exact execution stopped for a parked switch.

## In scope

- Policy-aware destination selection and source-session outcome.
- Execution-stamped workflow-switch stop intent and compare-and-remove handling.
- Completion/stopped event suppression and runtime activity cleanup.
- Reuse/new identity, terminal-candidate race, restart, and failure-path tests.

## Out of scope

- Frontend behavior.
- Office and automation session lifecycles.

## Acceptance

- `A -> B -> A` yields two sessions for `park_reuse` and three for `park_new`.
- Parked sessions are nonprimary, `WAITING_FOR_INPUT`, resumable, and retain
  their provider conversation identity.
- A delayed old-execution completion cannot evaluate the destination step, while
  a later execution's genuine completion still can.

## Verification

```bash
cd apps/backend && go test -tags fts5 ./internal/orchestrator -run 'Test(SwitchSessionForStep|HandleAgentCompleted|HandleAgentStopped).*ProfileSessionPolicy' -count=1 -v
```

## Files likely touched

- `apps/backend/internal/orchestrator/event_handlers_workflow.go`
- `apps/backend/internal/orchestrator/event_handlers_agent.go`
- `apps/backend/internal/orchestrator/service.go`
- `apps/backend/internal/orchestrator/event_handlers_workflow_profile_test.go`
- `apps/backend/internal/orchestrator/event_handlers_test.go`
- `apps/backend/internal/task/models/models.go`
- `apps/backend/internal/task/repository/sqlite/session.go`

## Dependencies

Task 01.

## Risks

- Stop callbacks can arrive synchronously, asynchronously, duplicated, or after
  a replacement execution starts.
- Full-row metadata writes can erase unrelated session state. Use atomic key
  updates and stamped removal.

## Parallelism

`sequential`

## Inputs

- System-design Control flow and Failure and recovery sections.
- Existing `completeAndStopSession`, terminal candidate promotion, and
  `RemoveSessionMetadataKeyIfStamp` patterns.

## Results

Implemented policy-aware fixed-profile switching with execution-stamped parked
session stop intents. Parked sessions remain nonprimary and
`WAITING_FOR_INPUT`, preserve their provider resume identity, and suppress only
the matching stopped/completed execution callback. Consumed stop intents remain
durable across delayed delivery and restart. Runtime teardown failure does not
abort a committed park, and failed parking restores transferred queue state.
Added round-trip tests for `park_reuse` and `park_new`, delayed callback identity
tests, intent-write failure coverage, destination-promotion failure coverage,
teardown-failure coverage, and queue-rollback coverage. The focused
orchestrator verification passed with 11 tests.
