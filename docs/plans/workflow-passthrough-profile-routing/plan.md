---
created: 2026-08-28
status: done
requirements:
  - REQ-TASKS-WORKFLOW-SESSION-SETTINGS-001
system_design:
  - ../../specs/tasks/system-design/workflow-step-fixed-profile-routing.md
legacy_specs: []
---

# Implementation Plan: Honor Passthrough Workflow Profile Routing

## Overview

Fix GitHub issue #3110 by applying fixed workflow-step profile routing before transport-specific behavior.
The CLI-passthrough state must not bypass profile resolution, credential admission, or the existing session lifecycle.

## Confirmed root cause

`prepareWorkflowStepSession` returns immediately when `IsPassthroughSession` is true.
This return occurs before Kandev resolves the destination step profile.
`preflightWorkflowStepCredentials` has the same early return.
The workflow commits the move and sends entry behavior to the source passthrough session.
This error occurs when the step selects another profile.

A focused diagnostic run set the source session of the existing dispatcher scenario to passthrough.
The unchanged code did not create a destination session.
The diagnostic patch removed only the two passthrough short-circuits.
The dispatcher then created and promoted the destination session.
It preserved the task environment and completed the source session.
All diagnostic edits were removed after the test.

## Scope

### In scope

- Resolve and apply fixed step profiles when the active session uses CLI passthrough.
- Validate the effective profile on explicit workflow-step launches before advancing the task or sending a prompt.
- Run the existing managed Git credential preflight before persisting such a transition.
- Preserve the existing session reuse, task-environment inheritance, queue transfer, primary-session promotion, and source-session completion behavior.
- Add backend and browser regression coverage for a passthrough-to-fixed-profile transition.

### Out of scope

- Reconfiguring a running CLI process to impersonate another profile.
- Changes to profile precedence, conditional session settings, passthrough terminal rendering, or ACP-only entry actions.
- Frontend controls, localization, schema, migration, runtime flags, or public documentation.

## Technical approach

Update `apps/backend/internal/orchestrator/event_handlers_workflow.go`.
Remove the source transport exemption from `prepareWorkflowStepSession` and `preflightWorkflowStepCredentials`.
Keep passthrough guards at transport-specific operation sites.

Replace the existing test that records the incorrect override behavior.
Add focused tests in a new orchestrator test file.
The current workflow profile test file exceeds the preferred size.
Cover destination-session creation and pre-transition credential rejection for a passthrough source session.

Keep the explicit `StartSessionForWorkflowStep` launch path transport-neutral.
Reject a fixed-profile mismatch before task-step advancement or prompt delivery.

Extend the existing TUI passthrough Playwright spec.
Use a CLI-passthrough source profile and a different fixed destination profile.
Start the task through the terminal UI, then move the task.
Use task-session state to validate the destination profile.
Use the visible session tab to validate primary ownership.

## Tests

- `AC-TASKS-WORKFLOW-SESSION-SETTINGS-001.2`: add `TestPrepareWorkflowStepSessionSwitchesPassthroughProfile` in `apps/backend/internal/orchestrator/event_handlers_workflow_passthrough_profile_test.go`.
- `AC-TASKS-WORKFLOW-SESSION-SETTINGS-001.3`: add `TestApplyEngineTransitionRejectsPassthroughTargetProfileBeforePersistingStep` in the same file.
- `AC-TASKS-WORKFLOW-SESSION-SETTINGS-001.3`: add `TestStartSessionForWorkflowStepRejectsPassthroughProfileMismatchBeforePrompt` in `apps/backend/internal/orchestrator/task_operations_passthrough_test.go`.
- Preserve the existing ACP destination-session dispatcher and profile reuse tests.

## E2E tests

- `AC-TASKS-WORKFLOW-SESSION-SETTINGS-001.2`: add `switches from a TUI session to the workflow step profile` to `apps/web/e2e/tests/terminal/terminal-agent.spec.ts`.
  The test starts a mock TUI process and moves the task to a fixed-profile step.
  It validates the new primary session through backend state and the visible session tab.

## Work orders

- [x] [Task 01: Honor passthrough step profile routing](task-01-honor-passthrough-step-profile-routing.md)

## Verification results

- `cd apps/backend && go test ./internal/orchestrator -run '^(TestPrepareWorkflowStepSessionSwitchesPassthroughProfile|TestApplyEngineTransitionRejectsPassthroughTargetProfileBeforePersistingStep|TestSwitchWorkflowDispatcherRoutesOnEnterToDestinationProfileSession)$' -count=1` — passed, 3 tests in 1 package.
- `cd apps/backend && go test ./internal/orchestrator -run '^(TestStartSessionForWorkflowStepRejectsProfileMismatchBeforePrompt|TestStartSessionForWorkflowStepRejectsPassthroughProfileMismatchBeforePrompt)$' -count=1` — passed, 2 tests in 1 package.
- `cd apps/web && pnpm e2e:run tests/terminal/terminal-agent.spec.ts -- --grep "switches from a TUI session to the workflow step profile"` — passed, 1 test in 16.3 seconds.
- `python3 scripts/lint-spec-files.py --all` — passed.

## Risks

- The destination profile can itself be passthrough or ACP. Routing must create the session first and let the destination profile choose its transport.
- Credential admission must remain before the persisted step transition.
  A check in `switchSessionForStep` occurs after the task enters the destination step.
- A test-wide passthrough boolean can misclassify the destination session.
  Backend coverage must validate routing directly or use session-specific behavior.
