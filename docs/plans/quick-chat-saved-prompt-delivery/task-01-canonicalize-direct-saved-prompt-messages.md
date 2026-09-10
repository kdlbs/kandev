---
id: "01-canonicalize-direct-saved-prompt-messages"
title: "Canonicalize direct saved-prompt messages"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-SAVED-PROMPT-DELIVERY-001
acceptance_criteria:
  - AC-TASKS-SAVED-PROMPT-DELIVERY-001.1
  - AC-TASKS-SAVED-PROMPT-DELIVERY-001.2
  - AC-TASKS-SAVED-PROMPT-DELIVERY-001.3
  - AC-TASKS-SAVED-PROMPT-DELIVERY-001.4
  - AC-TASKS-SAVED-PROMPT-DELIVERY-001.5
  - AC-TASKS-SAVED-PROMPT-DELIVERY-001.6
  - AC-TASKS-SAVED-PROMPT-DELIVERY-001.8
system_design:
  - ../../specs/tasks/system-design/saved-prompt-delivery.md
---

# Task 01: Canonicalize Direct Saved-Prompt Messages

## Summary

Prepare saved-prompt references before direct-message persistence and dispatch.
Retain only backend-generated prompt context through Quick Chat
canonicalization.

## In scope

- Remove legacy browser prompt-definition blocks during prompt preparation.
- Reuse backend saved-prompt resolution and sanitization.
- Add a narrow orchestrator preparation method for task message handlers.
- Pass exact expansion content through the trusted-content channel.
- Add focused prompt-service and message-handler tests.

## Out of scope

- Queue-handler changes.
- Frontend production changes.
- Passthrough expansion.
- Database or protocol migrations.

## Acceptance

- A selected or typed known reference produces one backend expansion in eager
  Quick Chat.
- A forged or stale browser definition cannot reach the agent as trusted
  context.
- The stored direct message equals the dispatched prompt and passthrough stays
  literal.

## Verification

```bash
cd apps/backend
rtk go test ./internal/prompts/service ./internal/sysprompt ./internal/orchestrator ./internal/task/handlers ./cmd/mock-agent -count=1
```

## Files likely touched

- `apps/backend/internal/prompts/service/expansion.go`
- `apps/backend/internal/prompts/service/expansion_test.go`
- `apps/backend/internal/orchestrator/service.go`
- `apps/backend/internal/orchestrator/task_operations.go`
- `apps/backend/internal/task/handlers/message_handlers.go`
- `apps/backend/internal/task/handlers/message_handlers_saved_prompt_test.go`

## Dependencies

None.

## Risks

- A broad block matcher can remove unrelated hidden context.
- Created-session workflow composition must preserve acceptance-time context
  rather than re-reading mutable saved-prompt records.

## Parallelism

`sequential`

## Inputs

- `REQ-TASKS-SAVED-PROMPT-DELIVERY-001`
- `docs/specs/tasks/system-design/saved-prompt-delivery.md`
- Existing prompt-expansion and Quick Chat title-context tests.

## Results

Implemented backend-owned direct prompt preparation, exact trusted-context
forwarding through eager CREATED-session startup, and regression coverage for
browser-definition removal, persistence/dispatch equality, and the trusted
startup seam.

Verification:

- `cd apps/backend && rtk go test ./internal/prompts/service ./internal/sysprompt ./internal/orchestrator ./internal/task/handlers ./cmd/mock-agent -count=1` - 3,283 passed.
