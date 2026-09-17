---
id: "04-revision-recovery"
title: "Add revision recovery and integration evidence"
status: done
wave: 4
depends_on:
  - "03-exact-edits"
plan: "plan.md"
requirements:
  - REQ-TASKS-PLAN-SAFE-004
  - REQ-TASKS-PLAN-SAFE-005
acceptance_criteria:
  - AC-TASKS-PLAN-SAFE-004.1
  - AC-TASKS-PLAN-SAFE-004.2
  - AC-TASKS-PLAN-SAFE-004.3
  - AC-TASKS-PLAN-SAFE-004.4
  - AC-TASKS-PLAN-SAFE-004.5
  - AC-TASKS-PLAN-SAFE-004.6
  - AC-TASKS-PLAN-SAFE-004.7
  - AC-TASKS-PLAN-SAFE-005.1
  - AC-TASKS-PLAN-SAFE-005.2
  - AC-TASKS-PLAN-SAFE-005.3
  - AC-TASKS-PLAN-SAFE-005.4
  - AC-TASKS-PLAN-SAFE-005.5
  - AC-TASKS-PLAN-SAFE-005.6
  - AC-TASKS-PLAN-SAFE-005.7
system_design:
  - ../../specs/tasks/system-design/plan-safe-edits.md
---

# Task 04: Add revision recovery and integration evidence

## Summary

Expose bounded history reads and conditional agent restoration. Complete deterministic MCP journey coverage and publish recovery guidance with the implementation.

## In scope

- Add metadata pagination and task-scoped revision reads without loading all revision bodies.
- Add snapshot tokens and conditional restore through shared revert internals, preserving browser behavior and agent attribution.
- Register all tools and actions through existing profile and guarded-dispatch paths.
- Build the real MCP-to-service journey regression and synchronize tool instructions, public documentation, and affected contract statuses.

## Out of scope

Automatic incident restoration, automatic session restart, revision retention changes, and live LLM evaluations.

## Acceptance

- List/read/restore work through MCP with bounded metadata pages, exact content, correct task scope, and agent attribution.
- Source coalescing, destination edits, unknown history, missing plans, and unauthorized targets reject without mutation. Valid restore preserves markers and previous history.
- The incident journey rejects the bad replacement and then completes a valid exact edit. Guidance does not require a user stop for safe correction.

## TDD and evidence

Test descending cursor pagination without gaps, cross-task IDs, mutable source snapshots, stale destination versions, oversized historical restore, and repeat-after-lost-response conflicts. Use deterministic concurrency barriers. The integration bridge must call actual handlers and service methods, not canned backend responses.

Test entry points and acceptance mappings are in [the manifest](plan.md#tests).
Update existing assertions only where the design explicitly changes their contract.

## Verification

Run from the repository root. Set `KANDEV_TEST_POSTGRES_DSN` to an isolated test database for PostgreSQL coverage.
Record a missing DSN as skipped coverage, never as a pass.
The normal repository command includes the environment-gated PostgreSQL tests.

```bash
(cd apps/backend && go test -race ./internal/task/repository/... ./internal/task/service ./internal/task/handlers ./internal/task/planws ./internal/mcp/handlers ./internal/mcp/server -count=1)
(cd apps/backend && go test ./internal/mcp/server -run '^TestPlanSafeEditsMCPJourney$' -count=1 -v)
(cd apps/backend && go run ./cmd/sqlguard ./internal)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files changed

- `apps/backend/internal/task/repository/sqlite/plan.go`
- `apps/backend/internal/task/service/plan_service.go`
- `apps/backend/internal/task/service/plan_agent_recovery.go (new)`
- `apps/backend/internal/task/service/plan_agent_recovery_test.go (new)`
- `apps/backend/internal/mcp/handlers/task_plan_recovery.go (new)`
- `apps/backend/internal/mcp/handlers/handlers.go`
- `apps/backend/internal/mcp/server/task_plan_recovery.go (new)`
- `apps/backend/internal/mcp/server/server.go`
- `apps/backend/internal/mcp/server/task_plan_safe_edits_integration_test.go (new)`
- `apps/backend/pkg/websocket/actions.go`
- `docs/public/tasks-and-workflows.md`
- `apps/backend/AGENTS.md`
- `docs/specs/tasks/requirements/plan-safe-edits.md`
- `docs/specs/tasks/system-design/plan-safe-edits.md`

The implementation also updates public documentation, repository guidance,
coverage metadata, and the Office MCP prompt.

## Dependencies

Task 03: Add exact fragment edits.

## Risks

Existing GetRevision reads by ID before task authorization, and RevertPlan stamps the user author kind. Do not expose either unchanged as the new agent contract.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/plan-safe-edits.md).
- [System design](../../specs/tasks/system-design/plan-safe-edits.md).
- [Decision](../../decisions/2026-09-16-conditional-agent-plan-writes.md).
- Existing `plan_service_concurrency_test.go`, `task_plan_guard_test.go`, and `task_plan_append_mode_test.go`.
- Backend guidance and the TDD backend-testing reference.

## Results

Implemented bounded revision metadata pagination, exact task-scoped revision
reads, snapshot tokens, and conditional agent restore. Restore preserves the
source and prior history, records agent attribution and the source reference,
and reports `already_current` without writing. All four plan recovery tools are
available on task and Office plan surfaces through the guarded dispatcher.

Added a real MCP-to-service-to-SQLite journey covering append, rejected fragment
replacement, exact correction, revision read, and conditional restore. Public
docs and repository guidance now describe the safe contract. Full affected
tests, race checks, lint, build, SQL guard, and documentation validation passed.

Review follow-up coverage now verifies that a previously valid source token is
invalidated by in-place revision coalescing, that a browser write admitted
during a restore wins deterministically, and that rejected restores leave HEAD,
version, history, and events unchanged. It also covers oversized historical
restore with comment and implementation-marker preservation, raw MCP omission
and `null` handling for `new_text`, and authorized, cross-task, and
unauthorized history reads and restores without foreign revision disclosure.
