---
id: "01-add-workflow-move-handoff-regression"
title: "Add workflow move handoff regression"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-003
acceptance_criteria:
  - AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-003.8
system_design:
  - ../../specs/tasks/system-design/workflow-step-agent-start-ownership.md
---

# Task 01: Add Workflow Move Handoff Regression

## Summary

Add a backend regression test for the text-handoff loss in GitHub issue #3409.
The test will prove that a prepared new session stores and dispatches the same
complete prompt.

## In scope

- Create a `CREATED` prepared-session test case.
- Use a non-empty replacement step prompt.
- Queue a textual move handoff.
- Assert complete and single delivery at storage and executor boundaries.

## Out of scope

- Change production prompt composition.
- Change dynamic continuation truncation.
- Add frontend or browser tests.
- Backport the correction.

## Acceptance

- The stored visible message contains the evaluated step prompt and handoff
  once.
- The execution description contains the same visible content once.
- The queue has no remaining handoff after accepted initial delivery.

## Verification

```bash
go test -race -tags fts5 ./internal/orchestrator -run '^(TestWorkflowAutoStartCreatedNonEmptyPromptPreservesQueuedHandoff|TestWorkflowAutoStartNonEmptyPrompt|TestWorkflowAutoStartEmptyPrompt)$' -count=1
```

Run the command from `apps/backend`.

## Files likely touched

- `apps/backend/internal/orchestrator/event_handlers_initial_prompt_dedup_test.go`

## Dependencies

None.

## Risks

- The process start is asynchronous. The synchronous execution-description
  call is the reliable agent-dispatch boundary for this test.

## Parallelism

`sequential`

## Inputs

- `AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-003.8`.
- The workflow-entry prompt flow in the system design.
- `startCreatedSessionWithComposedPrompt` in `task_operations.go`.
- The prepared-session fixture in
  `event_handlers_initial_prompt_dedup_test.go`.

## Results

- Added `TestWorkflowAutoStartCreatedNonEmptyPromptPreservesQueuedHandoff` as
  current-head contract coverage.
- The test uses a real SQLite repository and the prepared-session executor
  boundary.
- The focused race-enabled command passed eight selected test cases.
