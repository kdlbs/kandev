---
created: 2026-10-20
status: complete
requirements:
  - REQ-TASKS-CLARIFICATION-UNANSWERED-001
system_design:
  - ../../specs/tasks/system-design/clarification-active-lifecycle.md
legacy_specs: []
---

# Implementation plan: Unanswered clarification inbox

## Overview

Add an opt-in, non-blocking view of questions an agent moved past without an answer, per the
[requirements](../../specs/tasks/requirements/clarification-unanswered-inbox.md) and the
[system design](../../specs/tasks/system-design/clarification-active-lifecycle.md#unanswered-clarifications).
The active clarification rules, overlays, task-row projection, and workflow guards stay unchanged;
with the workspace setting off, every surface behaves as before.

## Scope

### In scope

- Two workspace settings with migration defaults, partial updates, events, and boot state.
- One repository read and conditional metadata writes for unanswered bundles: late-answer close,
  user close, lazy retention, and duplicate merge.
- Needs-you inbox endpoints and section, the late-answer hook's close, and the send-to-task choice.
- The `include_unanswered` option of `list_pending_questions_kandev`.
- Settings, Workspace card in every shipped locale.

### Out of scope

- The unified control-room inbox and coordinator prompt changes.
- Any change to `answer_question_kandev`, `/respond`, or the active lifecycle.

## Work orders

- [x] [Task 01: Unanswered clarification inbox](task-01-unanswered-inbox.md) (done)

## Verification results

See the work order. Backend unit tests (SQLite repository, clarification handlers, MCP handlers) and
frontend unit tests pass; the Playwright spec is left for CI.
