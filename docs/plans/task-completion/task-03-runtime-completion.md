---
id: "03-runtime-completion"
title: "Apply configured task completion"
status: done
wave: 3
depends_on: 
  - 02-portable-contracts
plan: "plan.md"
requirements:
  - REQ-TASKS-COMPLETION-001
  - REQ-TASKS-COMPLETION-002
acceptance_criteria:
  - AC-TASKS-COMPLETION-001.2
  - AC-TASKS-COMPLETION-001.3
  - AC-TASKS-COMPLETION-001.9
  - AC-TASKS-COMPLETION-001.10
  - AC-TASKS-COMPLETION-001.11
  - AC-TASKS-COMPLETION-001.12
  - AC-TASKS-COMPLETION-002.5
  - AC-TASKS-COMPLETION-002.10
  - AC-TASKS-COMPLETION-002.13
system_design:
  - ../../specs/tasks/system-design/task-completion.md
---

# Task 03: Apply configured task completion

## Summary

Make the saved setting authoritative for task completion and keep ordinary completed task conversations promptable. Preserve event ordering, clarification behavior, and idle cleanup.

## In scope

- Replace name-based completion with final-position eligibility plus the explicit setting, including initial creation, manual/bulk moves, queued promotion, workflow-store transitions, cancellation-driven transitions, and automatic steps. Resolve actual successor/order evidence in every path.
- Make parent rollups depend on persisted task outcomes; remove step-membership fallback and preserve DependencyStatusForTask's existing state-only resolution. Do not evaluate resident tasks when only a step setting changes.
- Replace successful child terminal collapse with the root waiting behavior while retaining active-question handling and fail-closed resource reclamation. Preserve failed/cancelled receipt behavior.
- Guard chat turns on completed tasks against task-state promotion and automatic turn-start/turn-complete workflow transitions, including built-in Done-to-In-Progress actions.

## Out of scope

Historical COMPLETED-session resume and the new editor are later work.

## Acceptance

- Checked final arbitrary-name steps complete; unchecked final Done and checked non-final steps do not. Reordering retains values without transferring completion to the new final step. Every entry path agrees, and explicit movement can still reopen a task.
- Only committed successful outcomes resolve dependencies; existing failed/cancelled parent rollups and duplicate suppression remain intact.
- Root/child follow-ups keep completed task state and step; active clarification remains answerable, cleanup retains recovery data, and old receipts cannot affect newer work.

## TDD entry

Add workflow_completion_policy_test.go with TestCompletionPolicyEntryPaths and TestCompletionPolicyNotifications. First assert an unchecked final Done remains nonterminal and a checked final arbitrary-name step completes. Cover a checked step moved earlier: it cannot complete work while non-final. Change the old terminal-child regression to expect a promptable session and run it RED before changing handlers.

## Verification

Run from `apps/backend/`. PostgreSQL-dependent cases use an isolated `KANDEV_TEST_POSTGRES_DSN`; never point tests at the reported installation.

```bash
rtk go test -race ./internal/workflow/models ./internal/task/service -run 'Test.*(Completion|Terminal|Move|Workflow|Dependenc)' -count=1
rtk go test -race ./internal/orchestrator -run 'Test.*(CompletionPolicy|ChildrenCompleted|TerminalStep|HandleAgentCompleted_Subtask|HandleAgentCompleted_NonTerminalSubtask|HandleAgentCompleted_Sibling|SubtaskTerminal|Reclaim|Clarification|Workflow.*Cancel)' -count=1
```


## Files likely touched

- `apps/backend/internal/workflow/models/terminal.go`
- `apps/backend/internal/workflow/models/terminal_test.go`
- `apps/backend/internal/task/service/service_workflow.go`
- `apps/backend/internal/task/service/service_dependencies.go`
- `apps/backend/internal/orchestrator/workflow_store.go`
- `apps/backend/internal/orchestrator/task_launch_workflow.go`
- `apps/backend/internal/orchestrator/event_handlers_workflow.go`
- `apps/backend/internal/orchestrator/event_handlers_children_completed.go`
- `apps/backend/internal/orchestrator/event_handlers_dependencies.go`
- `apps/backend/internal/orchestrator/event_handlers_streaming.go`
- `apps/backend/internal/orchestrator/event_handlers_agent.go`
- `apps/backend/internal/orchestrator/workflow_completion_policy_test.go (new)`
- `apps/backend/internal/orchestrator/event_handlers_subtask_waiting_test.go`
- `apps/backend/internal/orchestrator/event_handlers_reclaim_wiring_test.go`

## Dependencies

Complete 02-portable-contracts first.

## Risks

Do not conflate failed-child terminal rollups with successful dependencies. Do not infer completion after a failed state write. Avoid notification replay from chat activity or checkbox edits. Existing tests that seed Done by name must set the new boolean intentionally.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/task-completion.md), criteria in frontmatter.
- [System design](../../specs/tasks/system-design/task-completion.md).
- [Plan](plan.md), confirmed source trace and existing reproduction tests.
- Nearby Go repository/handler tests supply fixture conventions.
- Follow `/tdd` in the primary session.

## Results

Done. TDD evidence:

- RED: `TestHandleAgentCompleted_SubtaskWithoutRequestsInputRemainsPromptable`
  first failed because the successful child was collapsed to COMPLETED.
- The runtime predicate now requires a final step with
  `complete_task_on_enter: true`; arbitrary final names are supported, unchecked
  final Done is nonterminal, and checked non-final steps are nonterminal.
- Successful child terminal receipts remain promptable and reclaim their idle
  provider runtime. Failed/cancelled receipt behavior and clarification barriers
  remain unchanged.

Verification:

- `rtk go test -race ./internal/task/service -run 'Test.*(Completion|Terminal|Move|Workflow|Dependenc)' -count=1` (218 passed)
- `rtk go test -race ./internal/orchestrator -run 'Test.*(CompletionPolicy|ChildrenCompleted|TerminalStep|HandleAgentCompleted_Subtask|HandleAgentCompleted_NonTerminalSubtask|HandleAgentCompleted_Sibling|SubtaskTerminal|Reclaim|Clarification|Workflow.*Cancel)' -count=1` (159 passed)
