---
id: "01-honor-passthrough-step-profile-routing"
title: "Honor passthrough step profile routing"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKFLOW-SESSION-SETTINGS-001
acceptance_criteria:
  - AC-TASKS-WORKFLOW-SESSION-SETTINGS-001.2
  - AC-TASKS-WORKFLOW-SESSION-SETTINGS-001.3
system_design:
  - ../../specs/tasks/system-design/workflow-step-fixed-profile-routing.md
---

# Task 01: Honor Passthrough Step Profile Routing

## Summary

Make fixed workflow-step profile routing independent of the active session transport. Prove the correction at the orchestrator boundary and in the real mock-TUI browser flow.

## In scope

- Remove the passthrough routing exemption from workflow-step session preparation.
- Remove the passthrough credential-preflight exemption.
- Apply fixed-profile validation to explicit workflow-step launches before task advancement or prompt delivery.
- Replace obsolete skip coverage with destination-session and fail-closed regression tests.
- Add a focused Playwright scenario for TUI-to-fixed-profile routing.

## Out of scope

- UI or localization changes.
- Changes to conditional session settings or transport-specific entry-action support.
- Schema, migration, runtime-flag, or public API changes.

## Acceptance

- A task enters a differently profiled step from a live passthrough session.
  Kandev creates or activates the destination profile session before entry behavior runs.
- The destination session becomes primary, inherits the task environment, and the source passthrough session completes through the existing lifecycle.
- A credential-preflight failure keeps the task on its source step and leaves the source session running and primary.

## Verification

```bash
cd apps/backend && go test ./internal/orchestrator -run '^(TestPrepareWorkflowStepSessionSwitchesPassthroughProfile|TestApplyEngineTransitionRejectsPassthroughTargetProfileBeforePersistingStep|TestSwitchWorkflowDispatcherRoutesOnEnterToDestinationProfileSession)$' -count=1
cd apps/web && pnpm e2e:run tests/terminal/terminal-agent.spec.ts -- --grep "switches from a TUI session to the workflow step profile"
```

## Files likely touched

- `apps/backend/internal/orchestrator/event_handlers_workflow.go`
- `apps/backend/internal/orchestrator/event_handlers_workflow_profile_test.go`
- `apps/backend/internal/orchestrator/event_handlers_workflow_passthrough_profile_test.go`
- `apps/web/e2e/tests/terminal/terminal-agent.spec.ts`

## Dependencies

None.

## Risks

- The orchestrator test double exposes passthrough as a global boolean.
  A transport-specific destination action can falsely classify both sessions as passthrough.
- The E2E flow must wait on session lifecycle state rather than terminal text alone.

## Parallelism

`sequential`

## Inputs

- `docs/specs/tasks/requirements/workflow-session-settings.md`
- `docs/specs/tasks/system-design/workflow-step-fixed-profile-routing.md`
- `apps/backend/internal/orchestrator/event_handlers_workflow.go`
- `apps/backend/internal/orchestrator/event_handlers_workflow_profile_test.go`
- `apps/backend/internal/orchestrator/event_handlers_workflow_profile_preflight_test.go`
- `apps/web/e2e/tests/terminal/terminal-agent.spec.ts`

## Results

- Removed the passthrough exemptions from workflow-step session preparation and credential preflight.
- Added backend regressions for destination-session routing and fail-closed credential admission.
- Added a passthrough regression for explicit workflow-step launches that verifies mismatch rejection before task advancement or prompt delivery.
- Added a browser regression for TUI-to-fixed-profile routing, destination tab visibility, and primary ownership.
- `cd apps/backend && go test ./internal/orchestrator -run '^(TestPrepareWorkflowStepSessionSwitchesPassthroughProfile|TestApplyEngineTransitionRejectsPassthroughTargetProfileBeforePersistingStep|TestSwitchWorkflowDispatcherRoutesOnEnterToDestinationProfileSession)$' -count=1` — passed, 3 tests in 1 package.
- `cd apps/web && pnpm e2e:run tests/terminal/terminal-agent.spec.ts -- --grep "switches from a TUI session to the workflow step profile"` — passed, 1 test in 16.3 seconds.
- `python3 scripts/lint-spec-files.py --all` — passed.
