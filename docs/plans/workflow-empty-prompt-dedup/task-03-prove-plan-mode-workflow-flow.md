---
id: "03-prove-plan-mode-workflow-flow"
title: "Prove the plan-mode workflow flow"
status: done
wave: 2
depends_on:
  - "01-deduplicate-empty-workflow-prompts"
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-003
acceptance_criteria:
  - AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-003.1
  - AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-003.2
  - AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-003.4
system_design:
  - ../../specs/tasks/system-design/workflow-step-agent-start-ownership.md
---

# Task 03: Prove the Plan-Mode Workflow Flow

## Summary

Add a Playwright regression for the reported task flow. The test will prove the visible transcript contains one task-description message after the later step entry, with no empty user row or extra turn.

## In scope

- Create a workflow whose first step differs from its automatic-start step.
- Create and start a task in plan mode through the task dialog.
- Wait for the first agent turn to finish.
- Move the idle task into the empty automatic-start step.
- Assert one visible task-description message, no empty user row, and no additional turn.

## Out of scope

- A mobile-only duplicate of the transport-neutral behavior.
- A change to the create-task or workflow UI.
- Assertions that plan-mode placement must use the automatic-start step.

## Acceptance

- The task starts at the first step in plan mode.
- Entry into the empty automatic-start step does not add another description message.
- The final assertion uses the visible transcript after backend session readiness.

## Verification

```bash
(cd apps/web && pnpm e2e:run tests/workflow/start-step-vs-auto-start-step.spec.ts -- --grep "does not repeat the plan-mode task description")
```

## Files likely touched

- `apps/web/e2e/tests/workflow/start-step-vs-auto-start-step.spec.ts`

## Dependencies

- Task 01 supplies the behavior.

## Risks

- The first mock-agent turn can finish before browser subscriptions start. Poll the backend session state before the move.
- Remembered workflow selection can override the seeded workflow. Set the workspace selection explicitly before task creation.

## Parallelism

`sequential`

## Inputs

- The reported Backlog to In Progress reproduction.
- `apps/web/e2e/helpers/api-client.ts` workflow and session helpers.
- Existing task-dialog plan-mode coverage in `apps/web/e2e/tests/task/create-task.spec.ts`.
- Existing step-routing coverage in `start-step-vs-auto-start-step.spec.ts`.

## Results

- Added the requested browser regression using a dedicated two-step workflow:
  plan-mode creation lands on the first step, then an idle task enters an empty
  automatic-start step.
- The test waits for the first turn, backend task placement, the asynchronous
  on-enter session write, and a stable transcript/turn snapshot. It then checks
  the final API transcript and visible chat for exactly one task-description
  message, no empty user row, and no additional turn.
- The production Vite build passed, including the pseudo-locale build used by
  the E2E runner.
- `(cd apps/web && pnpm e2e:run tests/workflow/start-step-vs-auto-start-step.spec.ts -- --grep "does not repeat the plan-mode task description")` passed: 1 test.
- No mobile-only test was added because this is a transport-neutral prompt
  decision with no mobile layout or interaction change; the existing shared
  session surface is exercised by the browser regression.
- Task 04 supersedes the initial placement assertion. It will retain this
  prompt regression by making Plan the first automatic-start destination.
