---
created: 2026-09-15
status: completed
requirements:
  - REQ-TWS-001
  - REQ-TWS-002
  - REQ-TWS-005
system_design:
  - ../../specs/typed-workflow-state/system-design/typed-workflow-state.md
---

# Implementation Plan: Typed Workflow State — Step-Entry Number

## Overview

Compute the workflow step-entry number server-side and substitute it into
workflow prompt templates via `{step_entry_number}`, so an agent enforcing a
review-round cap never has to count its own prose. See
[../../specs/typed-workflow-state/system-design/typed-workflow-state.md](../../specs/typed-workflow-state/system-design/typed-workflow-state.md),
[../../specs/typed-workflow-state/requirements/step-entry-number.md](../../specs/typed-workflow-state/requirements/step-entry-number.md),
and
[../../specs/typed-workflow-state/requirements/concurrency-and-idempotency.md](../../specs/typed-workflow-state/requirements/concurrency-and-idempotency.md).

## Backend

- `apps/backend/internal/sysprompt/sysprompt.go`: add `InterpolateStepEntryNumber`,
  a pure function substituting `{step_entry_number}` with no DB or context
  access of its own. Task-ID substitution remains a separate prompt-building
  operation.
- `apps/backend/internal/task/repository/sqlite/step_transitions.go`: add
  `Repository.CountStepEntries`, backed by `SELECT COUNT(*) FROM
  task_step_transitions WHERE task_id = ? AND to_workflow_step_id = ?`. The
  repository returns the raw count; the orchestrator maps zero to 1 so a
  pre-ledger task still resolves to entry 1.
- `apps/backend/internal/orchestrator/task_operations.go`: both prompt-building
  call sites in `buildWorkflowPromptWithContext` compute the count and pass it
  through, gated on literal-token presence (no query when the template does not
  ask). A count-query error leaves the token literal and logs a warning rather
  than failing prompt building.
- `apps/backend/internal/orchestrator/service.go`: wiring for the new
  repository method.

## Tests

- `apps/backend/internal/sysprompt/sysprompt_step_entry_number_test.go`: pure
  substitution — 1-based numbering, unknown tokens left untouched, `{task_id}`
  substitution unchanged, base-10 rendering.
- `apps/backend/internal/task/repository/sqlite/step_transitions_count_test.go`:
  `CountStepEntries`, including the zero-row (pre-ledger) case.
- `apps/backend/internal/orchestrator/step_entry_number_prompt_test.go`: both
  production call sites, the count-query-error degrade path with a field-value
  warn-log assertion, the `{unknown_token}` case, and the
  `basePrompt`-embedded-literal case.

## Implementation Wave

- [x] [task-01-step-entry-number](task-01-step-entry-number.md) — complete.

## Verification

Backend-only change: no workflow YAML, plan write API, or plan revision model
touched. E2E was not required because there is no UI surface. The repository
method uses the shared database adapter and parameter rebinding, so it supports
SQLite and PostgreSQL without a schema change. Full validation receipts, review
history, and the E2E decision are recorded in the requirement and system-design
documents linked above.
