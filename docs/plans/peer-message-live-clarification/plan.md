---
created: 2026-10-19
status: implemented
requirements:
  - REQ-TASKS-CLARIFICATION-LIFECYCLE-001
system_design:
  - ../../specs/tasks/system-design/clarification-active-lifecycle.md
legacy_specs: []
---

# Implementation Plan: Peer Messages During a Live Question

## Overview

A queued-mode `message_task_kandev` to a session that waits on its own
`ask_user_question` was prompted directly and superseded the question. Queue it
instead, as the queue drain already does. One work order covers the handler
checks, the orchestrator accessor, tests, public messaging guidance, and specs.

## Scope

### In scope

- [REQ-TASKS-CLARIFICATION-LIFECYCLE-001](../../specs/tasks/requirements/clarification-active-lifecycle.md),
  AC `.10`.
- `dispatchTaskMessage` and the selected-session recheck in
  `dispatchPreparedTaskMessageWithAdmissionAndInterrupt`, both in
  `apps/backend/internal/mcp/handlers/handlers.go`.
- `Service.SessionHasLiveClarification` in
  `apps/backend/internal/orchestrator/clarification_guard.go`.

### Out of scope

- Interrupt-mode messages from a parent task.
- User prompts through `PromptTask`.
- Detached or superseded bundle semantics.

## Work orders

- [Task 01: Queue peer messages behind a live question](task-01-queue-peer-message-behind-live-question.md)
