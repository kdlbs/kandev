---
id: "06-route-source-step-recipients"
title: "Route source-step recipients"
status: complete
wave: 6
depends_on:
  - "05-persist-source-step-targets"
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKFLOW-PROFILE-SESSIONS-001
  - REQ-TASKS-WORKFLOW-PROFILE-SESSIONS-002
acceptance_criteria:
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.1
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.2
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.3
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.4
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.5
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.6
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.11
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-002.4
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-002.5
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-002.6
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-002.7
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-002.8
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-002.9
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-002.10
system_design:
  - ../../specs/tasks/system-design/workflow-profile-session-lifecycle.md
---

# Task 06: Route source-step recipients

## Summary

Extend initial routing with exact source-step session selection. Record bindings
when direct-profile steps execute, and keep initial and profile-only behavior.

## In scope

- Record the source step's latest successful session choice, including a reused
  or already-active session. Couple its binding with the routing commit.
- Exact source-session reuse, same-profile new, skipped-source fresh fallback,
  source profile edits, terminal/deleted bindings, and repeated source visits.
- Read source bindings without rewriting them when another step targets them.
- Use existing prepared/committed route records, preflight, queue/pending move
  transfer, source-end policy, stop intents and all entry-path integrations.
- Restart, stale operation and promotion races, logical dynamic profiles,
  ACP/passthrough and task/workflow authorization.

## Out of scope

- UI source picker or changes to initial provenance.
- Indirect references, later steps, Office fan-out.

## Acceptance

- Two source steps sharing a profile remain distinct. A later step uses the
  selected source's exact latest committed session, not profile recency.
- Skipped, changed-profile, terminal and deleted sources follow documented
  fallback without running the source prompt; malformed references fail visibly.
- Retry and rollback preserve bindings and prompt ownership; initial and all
  profile-only lifecycle regressions remain passing.

## Verification

Add failing tests in `workflow_step_session_target_test.go` and
`workflow_step_session_target_integration_test.go`. From `apps/backend`:

```bash
rtk go test -race ./internal/orchestrator -run 'TestWorkflowStepSessionTarget|TestWorkflowSessionTarget' -count=1
rtk go test ./internal/orchestrator -run 'TestPrepareWorkflowStepSession|TestSwitchSessionForStep|TestResolveStepAgentProfile|TestHandleTaskMoved|TestProcessOnEnter|TestWorkflowStore|TestHandleAgent.*ProfileSessionStopIntent' -count=1
rtk go test ./internal/workflow/engine/... ./internal/workflow/stepentry/... -count=1
```

Test source-step re-entry with stale delivery both before and after the newer
binding commit. Use real repository transactions to prove no orphan binding.

## Files likely touched

- `apps/backend/internal/orchestrator/workflow_session_target.go`
- `apps/backend/internal/orchestrator/{event_handlers_workflow.go,workflow_store.go,workflow_profile_session_lifecycle.go}`
- Task repository routing/promotion transaction and source-binding APIs
- `apps/backend/internal/orchestrator/workflow_step_session_target*_test.go` (new)

## Dependencies

Task 05 supplies source references and binding persistence.

## Risks

Binding before successful selection can make a failed source look available.
Updating the source binding during Review would change its meaning. Old entry
events must not overwrite a newer source visit.

## Inputs

- Design: Source-step bindings; Explicit routing flow.
- Tasks 01-02 initial routing and profile lifecycle regression tests.

## Parallelism

`sequential`

## Results

Implemented source-step resolution and latest-successful-session reuse with
fresh fallback, profile changes, re-entry handling, and unrelated same-profile
session isolation. The source-step runtime E2E delivered the marker to the
recorded source conversation, and orchestrator race coverage passed.
