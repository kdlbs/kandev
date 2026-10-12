---
id: "01-initial-create-prompt"
title: "Admit the initial creation prompt through turn-start"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-004
  - REQ-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-006
acceptance_criteria:
  - AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-004.1
  - AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-004.2
  - AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-004.3
  - AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-006.1
  - AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-006.2
  - AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-006.3
  - AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-006.4
  - AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-006.5
  - AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-006.6
system_design:
  - ../../specs/tasks/system-design/workflow-step-agent-start-ownership.md
---

# Task 01: Admit the initial creation prompt through turn-start

## Summary

Connect immediate explicit-step creation to the existing user-message transition boundary.
Keep one prompt owner through session replacement, WIP admission, and passthrough running notifications.

## In scope

- Add the private create-prompt marker before destination inference in REST and MCP adapters.
- Implement shared prepared-session admission in `task_create_prompt.go` and route marked launch requests through it.
- Process the trigger before composition; reload destination state, honor WIP queuing, and retain initial prompt metadata.
- Bind passthrough suppression to one initial execution/turn; retire it on completion, failure, cancellation, or replacement.
- Add all tests in the plan's matrix with TDD. Prove the primary regression fails before implementation.
- Update the `on_turn_start` explanation in `docs/public/workflow-tips.md` after behavior works.
  This explanation page must distinguish initial explicit-step input from automatic workflow-entry prompts.

## Out of scope

Dependency-deferred creates, attachment-only creates, historical repair, provider changes, UI changes, and new public launch fields.
Do not change the original context-reset or asynchronous prompt-preservation packages.

## Acceptance

1. Both creation adapters select the corrected path only for eligible original requests, after existing settlement and admission gates.
2. The first prompt-bearing dispatch observes the destination step/session/settings and one message; queue replay and running notifications do not trigger another transition.
3. All negative controls retain their behavior, failures prevent stale dispatch, and the documented verification commands pass.

## Implementation sequence

1. Add `TestInitialCreatePrompt_TransitionsBeforeDispatch` using the existing scheduler, SQLite, workflow engine, and mock agent fixtures.
2. Observe its failure with a pinned Backlog task and a move-to-Spec action.
3. Add shared admission and transport wiring; avoid duplicating transition logic in handlers.
4. Add the queue, passthrough, profile-switch, failure, replacement, and non-eligible controls from the plan.
5. Add `TestWorkflowE2E_InitialCreatePrompt` with a dispatch observer and completion transition.
6. Update public documentation and run every verification command.

## Verification

Run from the repository root. The first command selects the new regression family during TDD.
The package command covers all modified Go suites and their existing creation, queue, and lifecycle regressions.

```bash
(cd apps/backend && go test -tags fts5 ./internal/orchestrator -run 'TestInitialCreatePrompt_|TestWorkflowE2E_InitialCreatePrompt' -count=1 -v)
(cd apps/backend && go test -tags fts5 -race ./internal/orchestrator ./internal/task/handlers ./internal/mcp/handlers)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

## Files likely touched

- `apps/backend/internal/orchestrator/session_launch.go`
- `apps/backend/internal/orchestrator/task_create_prompt.go` (new)
- `apps/backend/internal/orchestrator/task_create_prompt_test.go` (new)
- `apps/backend/internal/orchestrator/task_operations.go`
- `apps/backend/internal/orchestrator/event_handlers_workflow.go`
- `apps/backend/internal/orchestrator/event_handlers_agent.go`
- `apps/backend/internal/orchestrator/event_handlers_queue_general_test.go`
- `apps/backend/internal/orchestrator/event_handlers_passthrough_lifecycle_test.go`
- `apps/backend/internal/orchestrator/workflow_e2e_test.go`
- `apps/backend/internal/task/handlers/task_http_handlers.go`
- `apps/backend/internal/task/handlers/task_http_handlers_test.go`
- `apps/backend/internal/mcp/handlers/handlers.go`
- `apps/backend/internal/mcp/handlers/handlers_test.go`
- `apps/backend/internal/mcp/handlers/create_task_external_id_test.go`
- `apps/backend/internal/mcp/handlers/destination_step_test.go`
- `docs/public/workflow-tips.md`
- This plan and work order, for implementation results.

## Dependencies

None. Execute in the primary session after a later implementation request.

## Risks

Do not hold the cancellation guard across another call that acquires it.
Do not infer explicit selection from the resolved MCP destination or the `AutoStart` flag.
Do not bypass launch ceilings, first-message admission, attachment claims, or session replacement checks.
If helpers hide an engine failure, return that failure before creation dispatch without changing unrelated callers' behavior.
Retain existing step-prompt composition; do not introduce a new prompt concatenation policy.

## Parallelism

`sequential`

## Inputs

- [Plan and diagnostic evidence](plan.md).
- [Requirements 004 and 006](../../specs/tasks/requirements/workflow-step-agent-start-ownership.md).
- [Initial creation prompt admission](../../specs/tasks/system-design/workflow-step-agent-start-ownership.md#initial-creation-prompt-admission).
- `wsAddMessage` in `message_handlers.go`: transition, resulting session, queue admission, and composition ordering.
- `TestStartCreatedSession_WrapsFirstPromptWithKandevSystemBlock` for prepared-launch fixtures.
- `TestExecuteQueuedMessage_SkipsOnTurnStartWhenAlreadyProcessed` for queue trigger ownership.
- Existing REST settlement and MCP external-ID tests for forbidden early dispatch.

## Results

Completed on 2026-09-19.

- Added server-side eligibility provenance for explicit, prompt-bearing immediate creation in the REST and MCP adapters.
- Routed marked creation starts through the shared turn-start admission boundary before prompt composition and provider dispatch.
- Preserved destination session/profile resolution, WIP queueing, attachment and prompt handling, and one-message ownership.
- Added scoped passthrough evidence for direct and queued initial turns. Running, completion, failure, stop, replacement, and dispatch-failure paths retire the evidence.
- Persisted strict-admission and queue-insertion failures through guarded launch-error ownership. Superseded sources and unrelated successors are left unchanged, while a replacement that owns the changed workflow step receives the error.
- Updated the public `on_turn_start` workflow guidance.

Verification passed:

```text
go test -tags fts5 ./internal/orchestrator -count=1
go test -tags fts5 ./internal/task/handlers ./internal/mcp/handlers -count=1
go test -tags fts5 -race ./internal/orchestrator -run '^(TestInitialCreatePrompt_|TestWorkflowE2E_InitialCreatePrompt$)' -count=1
go test -tags fts5 -race ./internal/orchestrator ./internal/task/handlers ./internal/mcp/handlers -count=1
make -C apps/backend build
make -C apps/backend lint
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

The temporary diagnostic reproduction was removed. No schema, provider protocol, or UI changes were required.
