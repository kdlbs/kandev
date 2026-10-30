---
id: "02-operator-question-flow"
title: "Deliver operator async questions"
status: completed
wave: 2
depends_on:
  - "01-persist-async-questions"
plan: "plan.md"
requirements:
  - REQ-TASKS-ASYNC-QUESTIONS-001
  - REQ-TASKS-ASYNC-QUESTIONS-002
  - REQ-TASKS-ASYNC-QUESTIONS-003
  - REQ-TASKS-ASYNC-QUESTIONS-004
  - REQ-TASKS-ASYNC-QUESTIONS-008
acceptance_criteria:
  - AC-TASKS-ASYNC-QUESTIONS-001.1
  - AC-TASKS-ASYNC-QUESTIONS-001.2
  - AC-TASKS-ASYNC-QUESTIONS-001.3
  - AC-TASKS-ASYNC-QUESTIONS-002.1
  - AC-TASKS-ASYNC-QUESTIONS-002.3
  - AC-TASKS-ASYNC-QUESTIONS-002.4
  - AC-TASKS-ASYNC-QUESTIONS-003.1
  - AC-TASKS-ASYNC-QUESTIONS-003.2
  - AC-TASKS-ASYNC-QUESTIONS-003.3
  - AC-TASKS-ASYNC-QUESTIONS-004.1
  - AC-TASKS-ASYNC-QUESTIONS-004.2
  - AC-TASKS-ASYNC-QUESTIONS-004.3
  - AC-TASKS-ASYNC-QUESTIONS-008.1
  - AC-TASKS-ASYNC-QUESTIONS-008.2
system_design:
  - ../../specs/tasks/system-design/async-agent-questions.md
---

# Task 02: Deliver operator async questions

## Summary

Deliver a usable operator async question path through MCP, HTTP, and queued continuation.
An agent receives a creation receipt, continues its current generation, and reads an answer before the queued notification runs.

## In scope

- Strict `async` and async-only `request_id` parsing, the receipt/result union, and early branching before blocking response extraction.
- Operator creation, author checkpoint reads, withdrawal, and shared operator response service.
- Authorized saved-image retrieval through the existing image-reader port.
- A backendapp exact-session queue adapter and cancellable durable outbox reconciliation.
- Stable replay identity, bounded retries, lifecycle invalidation, and startup recovery.
- Question-message metadata that excludes editing, Send now, auto-merge, and coalescing.
- User-question prompt/schema synchronization, while preserving omitted/false blocking instructions.

## Out of scope

- Parent forwarding, explicit dependency pause, Inbox row composition, and frontend submission controls.
- New external MCP creation capabilities.

## Acceptance

1. The RED `TestAsyncQuestionsOperatorAskKeepsTurnRunning` observes the present blocking behavior, then passes with a receipt before any answer and no generation cancellation.
2. Answer, rejection, withdrawal, checkpoint reads, and restart retry work through the shared service. Exact queue receipts prevent duplicate admission across a crash window.
3. Existing blocking ask tests pass. Foreign records, malformed modes, terminal destinations, and unsupported profiles fail without a write or dispatch.

## Verification

Run from the repository root:

```bash
(cd apps/backend && go test -trimpath -race ./internal/task/questions -run '^TestAsyncQuestions(Operator|Delivery|Withdraw|Image|Authorization)' -count=1)
(cd apps/backend && go test -trimpath ./internal/mcp/server -run '^(TestAsyncQuestions|TestAskUserQuestion|TestAskParentQuestion)' -count=1)
(cd apps/backend && go test -trimpath -race ./internal/mcp/handlers -run '^(TestAsyncQuestionsOperator|TestHandleAskUserQuestion_)' -count=1)
(cd apps/backend && go test -trimpath -race ./internal/orchestrator -run '^TestAsyncQuestionsDelivery' -count=1)
(cd apps/backend && go test -trimpath ./internal/orchestrator/messagequeue -run '^TestAsyncQuestionsQueue' -count=1)
(cd apps/backend && go test -trimpath ./internal/backendapp -run '^TestAsyncQuestions' -count=1)
(cd apps/backend && go test -trimpath ./internal/sysprompt -count=1)
```

Run changed-file Go lint for the actual touched paths.
Required new evidence includes `TestAsyncQuestionsOperatorCheckpointRead`,
`TestAsyncQuestionsDeliveryWaitsForBoundary`, `TestAsyncQuestionsDeliveryBehindLiveClarification`,
and `TestAsyncQuestionsDeliveryAdmissionReplay`.
Use a blocked dispatcher and explicit acknowledgements, not timing sleeps, for negative cancellation assertions.

## Files likely touched

- **New:** `apps/backend/internal/task/questions/service.go`, `delivery.go`, `http_handlers.go`, `withdraw_test.go`, and focused tests
- **New:** `apps/backend/internal/backendapp/async_questions.go`, `async_questions_test.go`
- **New:** `apps/backend/internal/mcp/server/async_questions.go`, `async_questions_test.go`
- **New:** `apps/backend/internal/mcp/handlers/async_questions.go`, `async_question_operator_test.go`
- **New:** `apps/backend/internal/orchestrator/async_question_delivery_test.go`
- `apps/backend/internal/mcp/server/server.go`, `handlers.go`, `sysprompt_sync_test.go`
- `apps/backend/internal/mcp/handlers/handlers_ask.go`
- `apps/backend/internal/mcp/profile/profile.go`
- `apps/backend/internal/orchestrator/messagequeue/admission_metadata.go` and the queue edit/merge/Send now predicates
- `apps/backend/internal/backendapp/helpers.go`, `adapters_question_images.go`
- `apps/backend/pkg/websocket/actions.go`
- `apps/backend/internal/sysprompt/sysprompt.go`, `sysprompt_test.go`
- `docs/specs/tasks/requirements/user-question-turn-boundary.md` for explicit mode scope and a link to the new contract

## Dependencies

Task 01. The queue adapter must use its immutable durable intent, never regenerate an admission identity on retry.

## Risks

- A receipt routed through `extractQuestionAnswers` can look like an incomplete blocking answer.
- The current timeout fallback must not pause an async author.
- Queue admission acknowledgement must remain distinct from provider prompt acknowledgement.
- Parent-only and other excluded profiles must not acquire operator creation through a backend route.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/async-agent-questions.md), the IDs in frontmatter.
- [Design](../../specs/tasks/system-design/async-agent-questions.md), creation, queued delivery, HTTP, and compatibility sections.
- Existing `ask_user_question_test.go`, clarification retry tests, and queue admission receipt tests.
- Backend scoped guidance and TDD backend testing reference.

## Results

Completed and remediated after review. Operator async creation returns a durable
receipt without a blocker; the outbox adapter now performs the queue readiness
wake after admission; the output schema is a discriminated union that preserves
arbitrary blocking item IDs; unsupported MCP surfaces reject async creation and
do not advertise the async arguments; child/history reads carry cursors; async
questions expose an authorized saved-image route; and `task.question.changed`
invalidates the Inbox.

Verification passed: questions, MCP server/handlers, message queue,
clarification, sysprompt, and previous-stable upgrade suites plus changed-file
Go lint. HTTP routes fail closed when the workspace authorizer or stored
workspace is absent; the output schema declares a root object; and the async
desktop/mobile E2E specs passed locally.
