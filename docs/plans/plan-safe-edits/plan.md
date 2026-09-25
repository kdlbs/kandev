---
created: 2026-09-16
status: complete
requirements:
  - REQ-TASKS-PLAN-SAFE-001
  - REQ-TASKS-PLAN-SAFE-002
  - REQ-TASKS-PLAN-SAFE-003
  - REQ-TASKS-PLAN-SAFE-004
  - REQ-TASKS-PLAN-SAFE-005
system_design:
  - ../../specs/tasks/system-design/plan-safe-edits.md
legacy_specs: []
---

# Implementation Plan: Safe agent plan edits

## Overview

Prevent accidental agent plan replacement and provide safe correction within the same turn.
Implement durable versions first, then guarded replacement, exact edits, and revision recovery.
The package contains four sequential work orders. All four are implemented in
this checkout.

The incident involved task `8207ac52-d02a-428c-a04f-ce34dd50f26f`.
At 2026-09-16T20:30:04Z, an explicit replacement reduced its plan from 16,332 to 807 characters.
Revision 3 remained intact. Reproduce this shape with synthetic data, not the live task.

## Scope

### In scope

- Versioned agent replacements through both create and update tools.
- Pre-commit truncation rejection with explicit acknowledgement for intentional reductions.
- One exact fragment edit per call.
- Bounded history listing, exact revision reads, and conditional restore.
- Corrective MCP results, profile exposure, documentation, and contract reconciliation.

### Out of scope

- Browser layout or interaction changes, model switching, and automatic session restart.
- Markdown parsing, fuzzy edits, append deduplication, and multi-document migration.
- Automatic repair of unrelated history divergence.
- Changing or restoring the incident task.

## Technical approach

Use the existing `PlanService` lock across state checks and writes.
Add `task_plans.write_version`, with UUID rotation on every content/title write.
Keep the version independent from coalesced revision identity and metadata-only updates.

Agent replacement requires `expected_version` against existing HEAD.
Suspicious reductions need `allow_truncation=true` plus verified history preservation.
The backend returns rejection before committing, with `write_applied=false` and a correction action.

Add `edit_task_plan_kandev` with one exact unique match.
Add revision list, read, and restore tools through the existing guarded dispatcher.
Restore compares both HEAD version and the selected revision snapshot.
The browser revert path retains its current behavior.

Use the implemented [design](../../specs/tasks/system-design/plan-safe-edits.md).
Update the affected older contracts at implementation time through its transition matrix.
This package adds no rendered UI, so it has no UI preview or browser E2E requirement.

## Tests

The table records the implemented test entry points and their evidence.

| Acceptance criteria | Evidence |
| --- | --- |
| 002.1–002.3, 002.5 | `repository/sqlite/plan_version_test.go`: persistence, coalescing, migration replay, restart, rollback, delete/recreate, and marker/comment cases |
| 001.1–001.6, 002.4 | `service/plan_safe_edits_test.go` and `internal/mcp/handlers/task_plan_guard_test.go`: guarded replacement, concurrent creation/update, unavailable HEAD/history, and no-mutation cases |
| 003.1–003.5 | `service/plan_exact_edit_test.go`: `TestPlanExactEdit`; MCP schema/forwarding tests, including raw omission/null rejection for `new_text` |
| 004.1–004.7 | `service/plan_agent_recovery_test.go`: `TestPlanAgentRecovery`, coalesced-source invalidation, deterministic browser-write conflict, oversized-history restore, and marker/comment preservation; bounded repository history tests; `mcp/server/task_plan_safe_edits_integration_test.go`: authorized, cross-task, and unauthorized list/read/restore coverage |
| 005.1–005.7 | `mcp/server/task_plan_safe_edits_integration_test.go`: `TestPlanSafeEditsMCPJourney`; description, error, and profile tests |

All abbreviated IDs use `AC-TASKS-PLAN-SAFE-`.
Tests must assert unchanged title/content/history/version and no mutation events on rejection.
Use deterministic barriers for concurrency rather than timing-dependent goroutine races.
Keep existing append, size-limit, browser, authorization, and marker regressions.

## End-to-end evidence

Task 04 owns a Go integration test through actual MCP dispatch, backend handlers, service, and SQLite.
A test bridge can replace transport delivery, but cannot stub plan responses or policy.
The journey reads a long plan, appends a checklist, rejects its fragment replacement, and corrects a checkbox with exact edit.
It then exercises revision read and conditional restoration.
The agent-visible result must provide correction steps without a stop instruction.
A browser or a live LLM is not needed to prove this agent-tool contract.

## Work orders

- [x] [Task 01: Persist edit versions](task-01-edit-versions.md) (done)
- [x] [Task 02: Guard agent replacement](task-02-guarded-replacement.md) (done)
- [x] [Task 03: Add exact fragment edits](task-03-exact-edits.md) (done)
- [x] [Task 04: Add revision recovery and integration evidence](task-04-revision-recovery.md) (done)

Execution order: 01 → 02 → 03 → 04. All work orders are complete.

## Verification results

Implementation verification completed on 2026-09-17.

Checks on 2026-09-17:

- `go test ./internal/task/repository/... ./internal/task/service ./internal/task/handlers ./internal/task/planws ./internal/mcp/handlers ./internal/mcp/server -count=1`: passed.
- The same affected backend packages passed with `go test -race`.
- Review follow-up regressions passed: raw MCP `new_text` omission/null rejection, coalesced-source invalidation, deterministic browser-write conflict, oversized-history restore with metadata preservation, and MCP authorization/task-scope recovery coverage.
- `go test -race ./internal/persistence/storeconformance -count=1`: passed.
- `make lint` and `make build` from `apps/backend`: passed.
- `go run ./cmd/sqlguard ./internal`: passed.
- Public documentation tests and validator: passed; 46 published pages validated.
- `python3 scripts/list-docs.py validate`: passed, 282 decisions and 971 specifications.
- `python3 scripts/lint-spec-files.test.py`: passed, 36 tests.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check`: passed.
- PostgreSQL tests remain environment-gated and were skipped because `KANDEV_TEST_POSTGRES_DSN` was not set.

The incident task was not restored or modified. Changes remain uncommitted for review.

## Risks

- Schema changes must cover SQLite upgrades and PostgreSQL parity.
- Cached tool schemas can lack new fields. Existing replacement calls fail safely and require refreshed discovery.
- A revision can change during coalescing. Restore needs its snapshot token as well as the HEAD token.
- The guard detects large reductions, not all semantic omissions. Exact edits reduce ordinary progress-update risk.
- Browser saves remain unconditional under their existing contract. This package protects agent writes against intervening browser saves.
- Legacy error and guard tests encode post-write warnings. Reconcile their requirements before changing their assertions.
- Do not claim PostgreSQL checks passed when the DSN-gated tests skipped.
- Human conflict resolution remains necessary when intent or concurrent edits cannot be reconciled safely.

## Sources

- [Requirements](../../specs/tasks/requirements/plan-safe-edits.md), REQ-TASKS-PLAN-SAFE-001 through 005.
- [Design](../../specs/tasks/system-design/plan-safe-edits.md).
- [Decision](../../decisions/2026-09-16-conditional-agent-plan-writes.md).
- [Existing consistency contract](../../specs/tasks/requirements/plan-write-consistency.md).
- [Existing append contract](../../specs/tasks/requirements/plan-write-append-mode.md).

The existing plan-write specifications contain no linked companion implementation package.
The older backend diagnostic work order classifies missing-task errors and remains outside this change.
