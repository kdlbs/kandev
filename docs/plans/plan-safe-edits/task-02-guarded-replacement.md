---
id: "02-guarded-replacement"
title: "Guard agent replacement"
status: done
wave: 2
depends_on:
  - "01-edit-versions"
plan: "plan.md"
requirements:
  - REQ-TASKS-PLAN-SAFE-001
  - REQ-TASKS-PLAN-SAFE-002
  - REQ-TASKS-PLAN-SAFE-005
acceptance_criteria:
  - AC-TASKS-PLAN-SAFE-001.1
  - AC-TASKS-PLAN-SAFE-001.2
  - AC-TASKS-PLAN-SAFE-001.3
  - AC-TASKS-PLAN-SAFE-001.4
  - AC-TASKS-PLAN-SAFE-001.5
  - AC-TASKS-PLAN-SAFE-001.6
  - AC-TASKS-PLAN-SAFE-002.4
  - AC-TASKS-PLAN-SAFE-005.1
  - AC-TASKS-PLAN-SAFE-005.2
  - AC-TASKS-PLAN-SAFE-005.5
  - AC-TASKS-PLAN-SAFE-005.6
system_design:
  - ../../specs/tasks/system-design/plan-safe-edits.md
---

# Task 02: Guard agent replacement

## Summary

Reject stale and suspicious agent replacements before mutation. Return corrective errors and versioned write acknowledgements through the actual MCP path.

## In scope

- Add expected_version and allow_truncation to create/update schemas, forwarding, typed requests, and service admission.
- Cover the create-upsert alternative and initial-create race. Enforce checks inside the existing plan lock.
- Map typed errors through planws and the MCP bridge without discarding reason data.
- Replace post-write stop warnings on agent paths. Reconcile the consistency, append-agent-text, and size-limit clauses named by the design.

## Out of scope

Exact-edit and revision tools, browser admission changes, and automatic conflict reconciliation.

## Acceptance

- A 16,332-character plan cannot become an 807-character fragment without a matching version and explicit acknowledgement.
- Every rejected operation leaves plan/history/markers unchanged and emits no mutation event. A subsequent valid write succeeds.
- Schemas and bridges preserve both new fields. Compact success responses expose the committed version, and errors provide correction steps.

## TDD and evidence

Reproduce the incident through handlers. Assert no writes on missing/stale versions, shrink rejection, failed HEAD reads, and unavailable preservation history. Cover exact threshold boundaries and Unicode. Force a concurrent browser save before agent admission and assert conflict.

Test entry points and acceptance mappings are in [the manifest](plan.md#tests).
Update existing assertions only where the design explicitly changes their contract.

## Verification

Run from the repository root. Set `KANDEV_TEST_POSTGRES_DSN` to an isolated test database for PostgreSQL coverage.
Record a missing DSN as skipped coverage, never as a pass.


```bash
(cd apps/backend && go test -race ./internal/task/service ./internal/task/planws ./internal/mcp/handlers ./internal/mcp/server -count=1)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files changed

- `apps/backend/internal/task/service/plan_service.go`
- `apps/backend/internal/task/service/plan_truncation.go`
- `apps/backend/internal/task/service/plan_safe_edits_test.go (new)`
- `apps/backend/internal/mcp/handlers/handlers.go`
- `apps/backend/internal/mcp/handlers/task_plan_guard.go and task_plan_guard_test.go`
- `apps/backend/internal/mcp/server/server.go`
- `apps/backend/internal/mcp/server/handlers.go`
- `apps/backend/internal/mcp/server/compact_results_test.go`
- `apps/backend/internal/mcp/server/task_plan_append_mode_test.go`
- `apps/backend/internal/task/planws/errors.go`
- `docs/specs/tasks/requirements/plan-write-consistency.md`
- `docs/specs/tasks/requirements/plan-write-append-mode.md`
- `docs/specs/tasks/requirements/plan-content-size-limit.md`
- `docs/specs/tasks/system-design/plan-write-consistency.md`
- `docs/specs/tasks/system-design/plan-write-append-mode.md`
- `docs/specs/tasks/system-design/plan-write-append-mode-agent-text.md`

The implementation also updates the task-plan error contract, Office prompt,
server catalog, and affected compatibility tests.

## Dependencies

Task 01: Persist edit versions.

## Risks

Default replacement remains a whole-document operation. New preconditions must apply even when the caller omits mode or uses create.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/plan-safe-edits.md).
- [System design](../../specs/tasks/system-design/plan-safe-edits.md).
- [Decision](../../decisions/2026-09-16-conditional-agent-plan-writes.md).
- Existing `plan_service_concurrency_test.go`, `task_plan_guard_test.go`, and `task_plan_append_mode_test.go`.
- Backend guidance and the TDD backend-testing reference.

## Results

Implemented trusted agent admission for create and update replacements. Existing
plans require a matching `expected_version`; suspicious reductions require
explicit acknowledgement and verified history before a separate revision is
written. Rejections carry stable reason data, no-write status, and correction
guidance through the WebSocket and MCP layers.

Added deterministic concurrent-create and concurrent-update coverage, including
unreadable HEAD/history cases and Unicode reduction detection. The affected
backend tests and lint checks passed.
