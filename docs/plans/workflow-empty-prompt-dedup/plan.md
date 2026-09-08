---
created: 2026-09-02
status: done
requirements:
  - REQ-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-003
  - REQ-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-004
  - REQ-TASKS-MCP-TOOL-NAMES-001
system_design:
  - ../../specs/tasks/system-design/workflow-step-agent-start-ownership.md
  - ../../specs/tasks/system-design/mcp-tool-name-stability.md
legacy_specs:
  - ../../specs/workflow-on-enter-action-dispatch/spec.md
---

# Implementation Plan: Deduplicate Empty Workflow Prompts

## Overview

The backend uses an atomically claimed session prompt boundary before it applies
the task-description fallback. The frontend uses the canonical plan-tool names.

The follow-up work makes immediate agent-start placement independent of agent
mode. This keeps the task in the workflow step that owns automatic starts.

## Scope

### In scope

- Stop repeated task-description prompts on empty automatic-start steps.
- Apply one fallback rule to automatic step entry and explicit workflow-step launch.
- Preserve non-empty step prompts for prompted sessions.
- Preserve the first task-description prompt for unprompted sessions.
- Correct `plan_get` and `plan_update` in the active-plan context.
- Add focused Go, TypeScript, and Playwright regressions.
- Route an immediate plan-mode agent start to the first automatic-start step.
- Prove the routing through desktop and mobile task creation.
- Update the public workflow guidance.

### Out of scope

- Do not change the first-step placement of plan-only prepared sessions when
  `start_agent=false`.
- Do not change the configured start-step behavior for no-agent task creation.
- Do not change workflow prompts that contain text.
- Do not add a database column or migration.
- Do not change MCP tool registration or transport names.

## Technical approach

### Durable prompt history

Add a bounded repository query for `task_session_prompt_seq.last_seq` and an atomic initial-fallback claim. The claim and direct user-message creation use the same per-session persistence boundary, so concurrent automatic and explicit admissions cannot both qualify as the first prompt. The claim uses a zero-valued counter row as its reservation marker, so the first visible message still receives prompt ordinal 1.

Expose this query and claim through the repository contract that the orchestrator uses. Keep message deletion behavior unchanged because the counter never decreases.

### Shared workflow prompt decision

Add an orchestrator helper that composes a workflow-entry prompt for one session. For an empty `WorkflowStep.Prompt` only, use the task description only when the atomic initial-fallback claim succeeds. Non-empty step prompts, including `{{task_prompt}}` expansion, retain their existing semantics.

Call the helper from `launchAfterOnEnterDispatch` before its ACP or passthrough split. Call the same helper from `StartSessionForWorkflowStep`.

Keep `buildWorkflowPromptWithTrustedContext` as the string composer. Keep non-empty `WorkflowStep.Prompt` behavior unchanged.

Apply plan-mode and session-config transforms before deciding whether the composed prompt is empty. If the composed ACP prompt is empty, let `autoStartStepPrompt` inspect any queued handoff first. Return without message creation or dispatch only when the merged prompt and attachments are empty. Attachment-only handoffs are admitted and persisted with their metadata.

### Plan-tool names

Change the active-plan context in `apps/web/hooks/use-message-handler.ts`. Use `get_task_plan_kandev` and `update_task_plan_kandev`.

Export the pure context helper for focused unit coverage. Do not localize this model-facing instruction.

### Immediate-launch placement

Change `task.Service.resolveWorkflowStep` so `StartAgent` is evaluated before
`PlanMode`. A request with both values uses `ResolveAutoStartStep`. A request
with only `PlanMode` continues to use `ResolveFirstStep`.

Keep explicit `workflow_step_id` precedence and the existing automatic-start
fallback chain. No transport, API, persistence, or frontend production change
is required.

Update the existing desktop workflow E2E scenario. Add a mobile scenario that
uses the visible plan-mode action. Update the public task-creation reference
and troubleshooting text.

## Tests

- `AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-003.1`: add a Go test for an unprompted session on an empty automatic-start step.
- `AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-003.2`: add a Go test for a prompted plan-mode session that later enters an empty automatic-start step.
- `AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-003.3`: add a Go test that a non-empty step prompt still dispatches after earlier prompts.
- `AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-003.4`: assert that an empty result creates no user row and no runtime prompt, while an attachment-only handoff remains durable input.
- `AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-003.5`: add a focused `StartSessionForWorkflowStep` regression.
- `AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-003.6`: cover the shared decision before the transport split.
- `AC-TASKS-MCP-TOOL-NAMES-001.3`: add a TypeScript test for canonical plan-tool names.
- `AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-004.1`: update service and
  transport routing tests for `start_agent=true` with `plan_mode=true`.
- `AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-004.2`: retain the existing
  automatic-start fallback test.
- `AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-004.3`: retain the explicit-step
  precedence test.

## E2E tests

Update `apps/web/e2e/tests/workflow/start-step-vs-auto-start-step.spec.ts`.
Start a plan-mode agent through the desktop task dialog. Assert that the task
lands in the first automatic-start step.

Keep the duplicate-prompt scenario by making Plan the first automatic-start
destination. Move the idle task into a second empty automatic-start step.
Assert that the description appears once and that no extra turn exists.

Add `apps/web/e2e/tests/workflow/mobile-start-step-vs-auto-start-step.spec.ts`.
Use the mobile plan-mode button and assert the same destination. These tests
change behavior only. The current task dialog layout and mobile composition
remain unchanged.

The current mobile task-create footer is the nearest mobile exemplar. It keeps
the plan-mode action visible as a touch-sized button. The desktop split menu
and the mobile button continue to use one submission handler.

## Work orders

- [x] [Task 01: Deduplicate empty workflow prompts](task-01-deduplicate-empty-workflow-prompts.md)
- [x] [Task 02: Correct plan-tool names](task-02-correct-plan-tool-names.md)
- [x] [Task 03: Prove the plan-mode workflow flow](task-03-prove-plan-mode-workflow-flow.md)
- [x] [Task 04: Route immediate plan-mode starts](task-04-route-immediate-plan-mode-starts.md)

## Verification results

- Backend focused race suite: 18 tests passed after the PR fixup, including prompt-admission races, plan-only and attachment-only inputs, passthrough draining, and session-ID reuse cleanup.
- Backend full suite: all packages passed with the task-session internal
  configuration handoff variables cleared. The plain command was blocked by
  the inherited launcher-selected `/root/.kandev/config.yaml`, which made
  isolated config-discovery tests select the operator config.
- Backend `make lint`: 0 issues.
- Backend new-from-`origin/main` `golangci-lint`: no issues.
- Frontend focused hook suite: 27 tests passed.
- Frontend typecheck, targeted ESLint, i18n check, and i18n ratchet: passed.
- Frontend production build and targeted Playwright regression: passed.
- Task 04 focused backend suite: 14 tests passed in 2 packages.
- Task 04 desktop Playwright suite: 3 tests passed, including the retained
  duplicate-prompt regression.
- Task 04 mobile Chrome Playwright suite: 1 test passed through the phone-only
  Plan mode action and phone board navigation.
- Public documentation validation: 61 tests passed and 41 published pages
  validated.

## Risks

- A transcript scan can grow with session size. The implementation must use the bounded prompt counter.
- Suppression before queued-message merge can lose a handoff. The ACP path must merge first.
- A broad guard can suppress non-empty step prompts. The claim applies only to the empty `WorkflowStep.Prompt` fallback; non-empty prompts, including `{{task_prompt}}`, retain their existing semantics.
- A session-ID reuse can leak a prompt claim if the counter is not removed with the session. Delete the counter explicitly because the replay-safe table has no foreign key.
- The explicit workflow-step path also manages resume state. Prompt suppression must not change its existing lifecycle behavior.
- The current desktop E2E scenario encodes the old plan-mode destination. Task
  04 must replace that assertion without removing duplicate-prompt coverage.
