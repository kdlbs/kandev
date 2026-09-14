---
id: "04-session-resume"
title: "Resume completed conversations safely"
status: done
wave: 4
depends_on: 
  - 03-runtime-completion
plan: "plan.md"
requirements:
  - REQ-TASKS-COMPLETION-002
acceptance_criteria:
  - AC-TASKS-COMPLETION-002.2
  - AC-TASKS-COMPLETION-002.3
  - AC-TASKS-COMPLETION-002.4
  - AC-TASKS-COMPLETION-002.6
  - AC-TASKS-COMPLETION-002.7
  - AC-TASKS-COMPLETION-002.8
  - AC-TASKS-COMPLETION-002.9
  - AC-TASKS-COMPLETION-002.10
  - AC-TASKS-COMPLETION-002.11
  - AC-TASKS-COMPLETION-002.12
  - AC-TASKS-COMPLETION-002.13
system_design:
  - ../../specs/tasks/system-design/task-completion.md
---

# Task 04: Resume completed conversations safely

## Summary

Admit explicit completed-session recovery through the existing lifecycle path. Keep the selected conversation and its ownership stable through messages, queue dispatch, cleanup, and failures.

## In scope

- Add a narrow internal permission for completed recovery from session.recover resume and pinned follow-up admission. Authorize task/session binding and prompt permission before side effects; keep passive ensure/status/launch terminal guards.
- Support missing executor rows after cleanup, preserve provider identity/history and queue incarnation, and persist guarded STARTING before credential acquisition. At readiness restore input without replaying a workflow or initial prompt.
- Persist completion_follow_up for manually revived historical sessions; exclude them from automatic profile reuse and task/workflow reconciliation until explicit ownership handoff.
- Route completed MCP targets through the same resume admission. Keep live-session preference and pin explicit/fallback targets; preserve FAILED/CANCELLED rejection and dedicated recovery paths.
- Cover double resume, resume/send order, old stream/exit/reclaim vs new execution, archive/delete/stop, queued prompt survival, active clarification, provider history restoration, and profile/runtime failure.

## Out of scope

No new MCP tool, new queue, Office bypass, automatic completed-session revival, or fresh conversation fallback.

## Acceptance

- Explicit Resume and follow-up use the same session and conversation, preserve primary/task/workflow ownership, and cannot mutate a foreign task/session pair.
- Completed recovery works after reclamation; duplicate and stale operations cannot clear new work or revive stopped/archived work. Queued prompts preserve FIFO and Auto-run.
- Passive opening never starts completed work; FAILED/CANCELLED behavior and current clarification barriers remain unchanged.

## TDD entry

Create completed_session_resume_test.go with TestCompletedSessionResumePreservesConversation and TestCompletedSessionResumeRaces, exercising RecoverSession and real guarded writers with slow runtime fakes. First show current completed rejection. Add TestCompletedSessionFollowUpOwnership for a retired non-primary session and protect the existing completed-sibling fallback regression.

## Verification

Run from `apps/backend/`. PostgreSQL-dependent cases use an isolated `KANDEV_TEST_POSTGRES_DSN`; never point tests at the reported installation.

```bash
rtk go test -race ./internal/orchestrator -run 'Test.*(CompletedSession|ResumeTaskSession|RecoverSession|SessionKeyedEntryPoints|GetTaskSessionStatus|Reclaim|SessionStartingRecovery|WorkflowProfileSession)' -count=1
rtk go test -race ./internal/orchestrator/executor -run 'Test.*(Resume|TerminalSessionState)' -count=1
rtk go test -race ./internal/task/handlers -run 'Test.*(Message|Prompt)' -count=1
rtk go test -race ./internal/mcp/handlers -run 'Test.*(MessageTask|MessageTarget|ParentQuestion)' -count=1
```


## Files likely touched

- `apps/backend/internal/orchestrator/session_launch.go`
- `apps/backend/internal/orchestrator/task_operations.go`
- `apps/backend/internal/orchestrator/executor/executor_resume.go`
- `apps/backend/internal/orchestrator/event_handlers_streaming.go`
- `apps/backend/internal/orchestrator/event_handlers_agent.go`
- `apps/backend/internal/orchestrator/event_handlers_workflow.go`
- `apps/backend/internal/orchestrator/workflow_profile_session_lifecycle.go`
- `apps/backend/internal/task/models/models.go`
- `apps/backend/internal/task/handlers/message_handlers.go`
- `apps/backend/internal/mcp/handlers/message_target_session.go`
- `apps/backend/internal/mcp/handlers/handlers.go`
- `apps/backend/internal/mcp/handlers/parent_question.go`
- `apps/backend/internal/orchestrator/completed_session_resume_test.go (new)`
- `apps/backend/internal/mcp/handlers/message_target_session_test.go`

## Dependencies

Complete 03-runtime-completion first.

## Risks

Use channel barriers, not sleeps, for races. State alone is insufficient against same-state replacement; retain execution/generation claims and tombstones. Avoid lock inversion between lifecycle guards, runtime cleanup, and event callbacks. Missing runtime data must not silently clear a provider resume token.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/task-completion.md), criteria in frontmatter.
- [System design](../../specs/tasks/system-design/task-completion.md).
- [Plan](plan.md), confirmed source trace and existing reproduction tests.
- Nearby Go repository/handler tests supply fixture conventions.
- Follow `/tdd` in the primary session.

## Results

The first red regression, `TestCompletedSessionFollowUpTurnReturnsToWaiting`,
showed that a resumed completed session reached `RUNNING` after its follow-up
instead of returning to `WAITING_FOR_INPUT`. The lifecycle path now recognizes
the durable completion-follow-up marker and settles the same session without
replaying workflow actions or changing task completion.

Final verification passed:

```text
orchestrator completed-session/resume/reclaim gate: 153 tests
executor resume/terminal-state gate: 103 tests
task-handler message/prompt gate: 103 tests
MCP message-target/parent-question gate: 65 tests
desktop completed-chat E2E: 1 test
mobile completed-chat E2E: 1 test
desktop recovery-neighbor E2E: 23 tests
mobile recovery-neighbor E2E: 4 tests
```

The same-session resume preserves the selected session, provider history,
primary ownership, task state, workflow step, queued prompt ordering, and
non-primary historical ownership. Passive opening remains non-revivifying, and
FAILED/CANCELLED recovery behavior remains unchanged.
