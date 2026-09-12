---
id: "02-first-message-composition"
title: "First-message composition"
status: pending
wave: 2
depends_on: ["01-atomic-admission"]
plan: "plan.md"
requirements:
  - REQ-TASKS-INITIAL-TASK-BRIEF-001
acceptance_criteria:
  - AC-TASKS-INITIAL-TASK-BRIEF-001.1
  - AC-TASKS-INITIAL-TASK-BRIEF-001.2
  - AC-TASKS-INITIAL-TASK-BRIEF-001.4
  - AC-TASKS-INITIAL-TASK-BRIEF-001.5
  - AC-TASKS-INITIAL-TASK-BRIEF-001.8
  - AC-TASKS-INITIAL-TASK-BRIEF-001.9
  - AC-TASKS-INITIAL-TASK-BRIEF-001.10
system_design:
  - ../../specs/tasks/system-design/initial-task-brief.md
---

# Task 02: First-message composition

## Summary

Compose the original brief with the additional instruction before first-message persistence.
Dispatch the selected stored content and its matching trusted expansion.

## In scope

- Opt eligible ordinary CREATED direct messages into Task 01 admission.
- Capture and validate the task description after task/session authorization.
- Keep exact equality, empty descriptions, passthrough, saved prompts, references, and attachments correct.
- Preserve visible direct content through workflow composition and session redirection.
- Exclude Office, ephemeral Quick Chat, configuration, and already-prompted sessions.

## Out of scope

Do not change unrelated launch policy, historical messages, or public API fields.

## Acceptance

- `TestWSAddMessage_InitialTaskBrief` fails on the original branch and then proves saved and captured content contain both texts once.
- `TestStartCreatedSession_InitialTaskBrief` proves real launch composition preserves both texts with empty, placeholder, and replacing step templates.
- Retry, queued promotion, plan comments, and excluded modes pass their regression cases without a second dispatch.

## Verification

Start with failing behavior assertions before implementation. Then run this
block from the repository root:

```bash
(cd apps/backend && go test -tags fts5 ./internal/task/handlers ./internal/task/service ./internal/orchestrator -run 'InitialTaskBrief|WSAddMessage|StartCreatedSession|InitialPromptFallback' -count=1)
git diff --check
```

## Files likely touched

- `apps/backend/internal/task/handlers/message_handlers.go`
- `apps/backend/internal/task/handlers/message_handlers_initial_task_brief.go (new helper)`
- `apps/backend/internal/task/handlers/message_handlers_initial_task_brief_test.go (new)`
- `apps/backend/internal/task/service/service_messages.go`
- `apps/backend/internal/orchestrator/task_operations.go`
- `apps/backend/internal/orchestrator/task_operations_initial_task_brief_test.go (new)`

## Dependencies

Task 01 must be complete.

## Risks

Never trust a browser initial-content flag or client-supplied system block.
Do not let a second workflow transform replace already composed direct content.
Preserve the original request fingerprint across candidate preparation.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/initial-task-brief.md).
- [System design](../../specs/tasks/system-design/initial-task-brief.md).
- [Package evidence and test matrix](plan.md).

## Results

Pending. No implementation or permanent tests were added during planning.
