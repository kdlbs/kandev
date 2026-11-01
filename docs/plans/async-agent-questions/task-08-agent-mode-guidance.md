---
id: "08-agent-mode-guidance"
title: "Guide agents on choosing a mode"
status: completed
wave: 5
depends_on:
  - "05-inbox-and-mobile"
plan: "plan.md"
requirements:
  - REQ-TASKS-ASYNC-QUESTIONS-008
  - REQ-TASKS-ASYNC-QUESTIONS-010
acceptance_criteria:
  - AC-TASKS-ASYNC-QUESTIONS-008.1
  - AC-TASKS-ASYNC-QUESTIONS-008.3
  - AC-TASKS-ASYNC-QUESTIONS-010.1
  - AC-TASKS-ASYNC-QUESTIONS-010.2
  - AC-TASKS-ASYNC-QUESTIONS-010.3
system_design:
  - ../../specs/tasks/system-design/async-agent-questions.md
---

# Task 08: Guide agents on choosing a mode

## Summary

Tell agents when to use async or blocking mode, and what to do after asking.
Async guidance reaches only sessions that can use async mode, and the outgoing blocking contract stays unchanged for callers that omit the flag.

## In scope

- An async-capability input on `KandevContextOptions`, following `asyncQuestionsSupported`.
- The selection rule and the after-asking rule in the operator, parent, and Autopilot sections.
- Conditional wording in both ask tool descriptions, including removing "truly blocking" and unconditional pause claims from the async case.
- Matching wording in `docs/public/agent-communication.md`.

## Out of scope

- Any backend enforcement of the choice, or changing the default mode.
- Real-agent evaluation of whether models follow the guidance.

## Acceptance

1. Capable operator and parent sessions receive the selection rule and name the read, wait, and withdraw tools.
2. Office, configuration, quick-chat, and external surfaces receive no async guidance and no async tool wording.
3. Omitted or false `async` keeps its schema, responses, and existing blocking instructions.

## Verification

Run from the repository root:

```bash
(cd apps/backend && go test -trimpath ./internal/sysprompt -count=1)
(cd apps/backend && go test -trimpath ./internal/mcp/server -count=1)
bash scripts/lint-go-changed <changed go files>
node scripts/validate-public-docs.mjs
```

Required new tests: `TestAsyncQuestionGuidanceByCapability` and `TestAsyncQuestionToolDescriptionsByCapability`.

## Files likely touched

- `apps/backend/internal/sysprompt/sysprompt.go`, `sysprompt_test.go`
- `apps/backend/internal/mcp/server/server.go`, `async_questions_test.go`, `tool_metadata_test.go`
- Callers of `InjectKandevContextWithOptions`
- `docs/public/agent-communication.md`

## Dependencies

Task 05. This work order does not depend on Tasks 06 or 07.

## Risks

- `KandevContext()` renders the question section without options. Check that callers on other surfaces do not get async wording through this path.
- A longer prompt adds cost to every session. Keep the rule to a few short sentences.
- Tests prove the guidance exists, not that agents follow it.

## Parallelism

`parallel-safe`

## Inputs

- [Agent mode guidance](../../specs/tasks/system-design/async-agent-questions.md#agent-mode-guidance-remediation).
- [Requirement 010](../../specs/tasks/requirements/async-agent-questions.md#req-tasks-async-questions-010-agent-guidance-for-choosing-a-mode).

## Results

Implemented capability-scoped async guidance for operator and parent question tools. Unsupported MCP surfaces keep their existing blocking contract. Updated the public agent communication guide.

Passed:

- `go test -trimpath ./internal/sysprompt -count=1`
- `go test -trimpath ./internal/mcp/server -count=1`
- Changed-file Go lint: `0 issues`
- Public documentation validation
