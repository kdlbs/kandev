---
id: "01-deduplicate-empty-workflow-prompts"
title: "Deduplicate empty workflow prompts"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-003
acceptance_criteria:
  - AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-003.1
  - AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-003.2
  - AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-003.3
  - AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-003.4
  - AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-003.5
  - AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-003.6
  - AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-003.7
system_design:
  - ../../specs/tasks/system-design/workflow-step-agent-start-ownership.md
---

# Task 01: Deduplicate Empty Workflow Prompts

## Summary

Add a durable prompt-history counter and atomic first-prompt claim for workflow prompt composition. Only an empty step prompt can use the task description as the one-time fallback.

## In scope

- Add a bounded prompt-history query to the task repository.
- Apply the query in both workflow-step prompt paths.
- Keep ACP and passthrough behavior aligned.
- Preserve non-empty step prompts and ACP handoffs.
- Add deterministic repository and orchestrator tests.

## Out of scope

- Plan-mode step placement.
- Start-step resolution.
- New persisted fields.
- Frontend tool-name text.

## Acceptance

- An empty automatic-start step does not repeat a task description for a prompted session.
- An unprompted session still receives the task description once.
- A non-empty step prompt still dispatches, and an empty result creates no turn or message.
- Attachment-only handoffs are persisted and dispatched with their attachment metadata.
- Concurrent direct and automatic first-prompt admissions allow at most one initial fallback.

## Verification

```bash
(cd apps/backend && go test -race -tags fts5 ./internal/task/repository/sqlite ./internal/orchestrator -run '^(TestHasUserPromptHistory|TestClaimInitialPromptFallbackSerializesPromptAdmission|TestDeleteTaskSessionRemovesPromptHistoryClaim|TestWorkflowAutoStartEmptyPrompt|TestWorkflowAutoStartNonEmptyPrompt|TestStartSessionForWorkflowStepEmptyPrompt|TestWorkflowAutoStartPlanModeOnlyPrompt|TestWorkflowAutoStartPlanModeOnlyPromptForCreatedSession|TestStartSessionForWorkflowStepPlanModeOnlyPrompt|TestWorkflowAutoStartAttachmentOnlyHandoffIsRecorded|TestWorkflowAutoStartPassthroughDrainsSuppressedHandoff)' -count=1)
make -C apps/backend test
make -C apps/backend lint
(cd apps/backend && golangci-lint run ./... --new-from-rev="$(git merge-base HEAD origin/main)" --timeout=5m)
```

## Files likely touched

- `apps/backend/internal/task/repository/interface.go`
- `apps/backend/internal/task/repository/sqlite/message_prompt_index.go`
- `apps/backend/internal/task/repository/sqlite/prompt_history_test.go`
- `apps/backend/internal/orchestrator/service.go`
- `apps/backend/internal/orchestrator/task_operations.go`
- `apps/backend/internal/orchestrator/event_handlers_workflow.go`
- `apps/backend/internal/orchestrator/event_handlers_initial_prompt_dedup_test.go`
- `apps/backend/internal/task/handlers/process_handlers_test.go`

## Dependencies

None.

## Risks

- Repository interface changes can require test-double updates.
- The no-content branch can bypass a queued handoff if it runs too early.
- Prompt-history errors can leave the session waiting until a user retries.

## Parallelism

`sequential`

## Inputs

- `REQ-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-003`
- Prompt fallback ownership and workflow-entry prompt flow in the system design.
- Existing prompt-counter allocation in `message_prompt_index.go`.
- Existing auto-start recovery tests in `event_handlers_duplicate_autostart_test.go`.

## Results

- Added the durable `HasUserPromptHistory` query plus the atomic
  `ClaimInitialPromptFallback` admission boundary and applied one entry-prompt
  decision to automatic and explicit workflow-step launches.
- Added coverage for prompted and unprompted empty steps, non-empty step
  prompts, passthrough suppression, prompt-history errors, plan-mode-only
  instructions, attachment-only handoffs, prompt-admission races, session-ID
  reuse cleanup, and the no-turn/no-message empty result.
- The focused race suite passed with 18 tests, including both prompt-admission orderings, concurrent fallback claims, plan-only prompts, attachment-only handoffs, passthrough draining, and session-ID reuse cleanup.
- `env KANDEV_INTERNAL_CONFIG_FILE= KANDEV_INTERNAL_CONFIG_HOME_FILE= make -C apps/backend test` passed for every backend package. The unmodified
  `make -C apps/backend test` form was also attempted, but this task session's
  injected launcher config caused unrelated home-config discovery failures.
- `make -C apps/backend lint` passed with 0 issues.
- `golangci-lint run ./... --new-from-rev=0064b9fb5a903feeb6cffc2ce6b7db1c16218e6e --timeout=5m` passed with no issues.
