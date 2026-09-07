---
created: 2026-09-05
status: complete
requirements:
  - REQ-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-003
system_design:
  - ../../specs/tasks/system-design/workflow-step-agent-start-ownership.md
legacy_specs: []
---

# Implementation Plan: Workflow Move Handoff Delivery

## Overview

Add regression coverage for the text-handoff loss reported in GitHub issue
3409. The production correction is already on `main` in commit `613b9a4eb`.
That commit added a launch path for prompts that the orchestrator already
composed.

The new test will exercise the complete orchestrator-to-executor boundary. No
production change is planned because the current path passes the reproduction.

## Confirmed root cause

In v0.93.0, `autoStartStepPrompt` evaluated the step prompt and appended the
queued handoff. It then stored the merged user message and called
`StartCreatedSession`.

`StartCreatedSession` applied the current step prompt again. A non-empty step
prompt without `{{task_prompt}}` replaces its input. Therefore, the second
composition removed the appended handoff before agent dispatch. The stored
message kept the handoff because storage occurred before the second
composition.

Commit `613b9a4eb` routes this case through
`startCreatedSessionWithComposedPrompt`. That path skips the second workflow
composition. A focused diagnostic test showed that `main` sends the step prompt
and handoff through `SetExecutionDescription`.

## Scope

### In scope

- Add a regression test for a `CREATED` prepared session.
- Use a non-empty step prompt without `{{task_prompt}}`.
- Queue a textual move handoff before automatic start.
- Prove that storage and dispatch each retain the step prompt and handoff once.

### Out of scope

- Change the current production prompt-composition path.
- Change workflow step placeholder behavior.
- Change dynamic continuation size limits or truncation order.
- Backport the correction to an earlier release.
- Add browser or frontend coverage for this backend dispatch contract.

## Technical approach

Add `TestWorkflowAutoStartCreatedNonEmptyPromptPreservesQueuedHandoff` to
`apps/backend/internal/orchestrator/event_handlers_initial_prompt_dedup_test.go`.
Reuse `newInitialPromptDedupFixture` and the existing prepared execution row.

Set the session state to `CREATED` and configure an agent profile. Then add a
non-empty step prompt and a queued handoff. Call `autoStartStepPrompt` through
the same path that starts a prepared workspace.

Inspect the stored user message and the `SetExecutionDescription` call in the
mock manager. Compare their visible content after system
context removal. Each value must contain the evaluated step prompt and the
handoff once.

## Tests

- `AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-003.8`: Add
  `TestWorkflowAutoStartCreatedNonEmptyPromptPreservesQueuedHandoff`.
- Keep `TestWorkflowAutoStartNonEmptyPrompt` and the queued-handoff suppression
  case in the focused command. These tests protect both adjacent branches.

## Work orders

- [x] [Task 01: Add workflow move handoff regression](task-01-add-workflow-move-handoff-regression.md)

## Verification results

- The focused race-enabled command passed eight selected test cases.
- The new test proves that storage and executor dispatch retain identical
  visible content.
- The queue is empty after the prepared session accepts the initial prompt.

## Risks

- A storage-only assertion can pass while dispatch still loses the handoff.
  The test must inspect the executor description boundary.
- An asynchronous process-start assertion can make the test unstable. The test
  will assert the synchronous `SetExecutionDescription` call instead.
