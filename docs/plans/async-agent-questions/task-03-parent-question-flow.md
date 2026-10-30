---
id: "03-parent-question-flow"
title: "Route and forward async parent questions"
status: completed
wave: 3
depends_on:
  - "02-operator-question-flow"
plan: "plan.md"
requirements:
  - REQ-TASKS-ASYNC-QUESTIONS-001
  - REQ-TASKS-ASYNC-QUESTIONS-003
  - REQ-TASKS-ASYNC-QUESTIONS-005
  - REQ-TASKS-ASYNC-QUESTIONS-008
acceptance_criteria:
  - AC-TASKS-ASYNC-QUESTIONS-001.1
  - AC-TASKS-ASYNC-QUESTIONS-001.2
  - AC-TASKS-ASYNC-QUESTIONS-003.4
  - AC-TASKS-ASYNC-QUESTIONS-005.1
  - AC-TASKS-ASYNC-QUESTIONS-005.2
  - AC-TASKS-ASYNC-QUESTIONS-005.3
  - AC-TASKS-ASYNC-QUESTIONS-005.4
  - AC-TASKS-ASYNC-QUESTIONS-008.1
  - AC-TASKS-ASYNC-QUESTIONS-008.3
system_design:
  - ../../specs/tasks/system-design/async-agent-questions.md
---

# Task 03: Route and forward async parent questions

## Summary

Let an Autopilot child ask its direct parent without stopping its current work.
Let an authorized normal parent forward that same request to the operator and preserve one winning child outcome.

## In scope

- Async parent creation without the existing hard-pause call.
- Parent `scope=children` reads and an immediate durable parent-request queue intent.
- Request-kind dispatch for `message_task_kandev(reply_to_question_id=...)`.
- Normal-parent forwarding, operator answer authorization, owner/parent outcome notifications, and canonical winner races.
- Current/recorded parent validation, reparent invalidation, wrong-session rejection, and capability checks.
- Mode-specific Autopilot prompt wording and compatibility controls.

## Out of scope

- Autopilot-root operator access, multi-hop correlated forwarding, or changes to ordinary peer-message admission.
- Inbox UI rendering and new parent-question image support.

## Acceptance

1. `TestAsyncQuestionsParentKeepsChildRunning` fails on the current hard pause, then proves receipt, parent discovery, and continued child work.
2. Forwarding and competing parent/operator answers use one canonical request. Every retry and losing answer avoids another child delivery.
3. Unrelated parents, inaccessible workspaces, invalidated relationships, wrong sessions, and parent-only forwarding attempts fail without disclosure or delivery. Existing blocking parent-pause tests pass.

## Verification

Run from the repository root:

```bash
(cd apps/backend && go test -trimpath -race ./internal/task/questions -run '^TestAsyncQuestions(Parent|Forward)' -count=1)
(cd apps/backend && go test -trimpath -race ./internal/mcp/handlers -run '^(TestAsyncQuestions(Parent|Forward)|TestHandleAskParentQuestion_|TestHandleMessageTask_.*ParentQuestion)' -count=1)
(cd apps/backend && go test -trimpath ./internal/mcp/server -run '^(TestAsyncQuestionsProfileMatrix|TestAskParentQuestion|TestServerProfile_Autopilot)' -count=1)
(cd apps/backend && go test -trimpath ./internal/sysprompt -count=1)
```

Run changed-file Go lint for the actual touched paths.
Required new test methods are `TestAsyncQuestionsParentListScope`,
`TestAsyncQuestionsForwardedWinner`, `TestAsyncQuestionsForwardAuthorization`,
and `TestAsyncQuestionsParentReparentRace`.
Use the Task 01 PostgreSQL harness for the relationship/resolve concurrency scenario.

## Files likely touched

- **New:** `apps/backend/internal/task/questions/parent.go`, `forward.go`, `parent_test.go`, `forward_test.go`
- **New:** `apps/backend/internal/mcp/handlers/async_parent_question_test.go`
- `apps/backend/internal/mcp/handlers/parent_question.go`, `handlers.go`
- `apps/backend/internal/mcp/server/server.go`, `handlers.go`, and new async tool handlers
- `apps/backend/internal/mcp/profile/profile.go`
- `apps/backend/internal/task/repository/sqlite/async_questions.go`, `async_questions_postgres_test.go`
- `apps/backend/internal/sysprompt/sysprompt.go`, `sysprompt_test.go`
- `docs/specs/tasks/requirements/autopilot-mode.md` for blocking-mode scope and async references

## Dependencies

Task 02. Use the same async response and delivery service as operator answers.

## Risks

- Existing blocking parent records share clarification metadata. Classify by canonical async ID instead of metadata heuristics.
- A parent task relationship does not grant workspace access by itself.
- Forwarding must not register a user-question tool for an Autopilot session.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/async-agent-questions.md), the IDs in frontmatter.
- [Design](../../specs/tasks/system-design/async-agent-questions.md), parent answers and capability matrix.
- Existing `parent_question_test.go`, peer-message live-clarification controls, and exclusive-capability ADR.

## Results

Completed and remediated after review. Async parent creation stores the child
and parent session incumbencies, emits a distinct `parent_question` intent, and
keeps the child running. A parent outcome uses `parent_outcome`, so both parent
notifications have distinct admission identities and payloads. Parent reads,
forwarding, and canonical winner resolution revalidate the current direct-parent
binding and reject reparented or inaccessible requests.

Verification passed: questions, MCP server/handlers, and message queue suites,
including the distinct parent-question/outcome intent regression.
