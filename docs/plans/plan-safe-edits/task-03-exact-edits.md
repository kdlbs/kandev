---
id: "03-exact-edits"
title: "Add exact fragment edits"
status: done
wave: 3
depends_on:
  - "02-guarded-replacement"
plan: "plan.md"
requirements:
  - REQ-TASKS-PLAN-SAFE-003
  - REQ-TASKS-PLAN-SAFE-005
acceptance_criteria:
  - AC-TASKS-PLAN-SAFE-003.1
  - AC-TASKS-PLAN-SAFE-003.2
  - AC-TASKS-PLAN-SAFE-003.3
  - AC-TASKS-PLAN-SAFE-003.4
  - AC-TASKS-PLAN-SAFE-003.5
  - AC-TASKS-PLAN-SAFE-005.1
  - AC-TASKS-PLAN-SAFE-005.2
  - AC-TASKS-PLAN-SAFE-005.5
  - AC-TASKS-PLAN-SAFE-005.6
  - AC-TASKS-PLAN-SAFE-005.7
system_design:
  - ../../specs/tasks/system-design/plan-safe-edits.md
---

# Task 03: Add exact fragment edits

## Summary

Add a small exact-text editing tool for routine checklist changes. Reuse conditional admission and avoid a Markdown parser.

## In scope

- Register edit_task_plan_kandev and its guarded backend action with version, old_text, new_text, and optional truncation acknowledgement.
- Compose inside the existing task lock, require one exact occurrence including overlap checks, and preserve surrounding bytes.
- Reuse final-content validation, limits, history, attribution, and compact responses. Preserve append behavior with optional expected_version.

## Out of scope

Fuzzy matching, heading identity, edit batches, automatic token refresh, and revision recovery.

## Acceptance

- One checkbox edit changes only the requested span and preserves review sections, title, and exact surrounding bytes.
- Empty old text, ambiguous/absent matches, stale tokens, oversized composition, and unacknowledged large reductions reject without mutation.
- The tool appears only on existing plan-write surfaces. Invalid arguments cannot become replacement or append operations.

## TDD and evidence

Add checkbox, Unicode, CRLF, overlap, deletion, missing new_text, identical replacement, final-empty, and size-boundary cases. Assert untouched bytes around the span. Test schema-to-service propagation and excluded profiles.

Test entry points and acceptance mappings are in [the manifest](plan.md#tests).
Update existing assertions only where the design explicitly changes their contract.

## Verification

Run from the repository root. Set `KANDEV_TEST_POSTGRES_DSN` to an isolated test database for PostgreSQL coverage.
Record a missing DSN as skipped coverage, never as a pass.


```bash
(cd apps/backend && go test -race ./internal/task/service ./internal/task/planws ./internal/mcp/handlers ./internal/mcp/server -count=1)
git diff --check
```

## Files changed

- `apps/backend/internal/task/service/plan_exact_edit.go (new)`
- `apps/backend/internal/task/service/plan_exact_edit_test.go (new)`
- `apps/backend/internal/mcp/handlers/task_plan_edit.go (new)`
- `apps/backend/internal/mcp/handlers/handlers.go`
- `apps/backend/internal/mcp/server/task_plan_edit.go (new)`
- `apps/backend/internal/mcp/server/server.go`
- `apps/backend/pkg/websocket/actions.go`
- `apps/backend/internal/mcp/server/task_plan_append_mode_test.go`

The implementation also updates the WebSocket action catalog and append-mode
compatibility coverage.

## Dependencies

Task 02: Guard agent replacement.

## Risks

Non-overlapping string counts can miss ambiguous overlapping matches. No whitespace normalization is allowed.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/plan-safe-edits.md).
- [System design](../../specs/tasks/system-design/plan-safe-edits.md).
- [Decision](../../decisions/2026-09-16-conditional-agent-plan-writes.md).
- Existing `plan_service_concurrency_test.go`, `task_plan_guard_test.go`, and `task_plan_append_mode_test.go`.
- Backend guidance and the TDD backend-testing reference.

## Results

Implemented `edit_task_plan_kandev` with exact byte-span replacement, overlap
detection, version checks, size validation, truncation protection, deletion,
and compact acknowledgements. Surrounding content and line endings remain
unchanged. Append keeps its existing composition and accepts an optional
version guard.

Checkbox, CRLF, Unicode, ambiguous, missing, deletion, final-empty, size, and
MCP forwarding cases passed.
