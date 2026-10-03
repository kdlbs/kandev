---
id: "01-bounded-reads"
title: "Bounded version-aware plan reads"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-PLAN-READ-001
  - REQ-TASKS-PLAN-READ-002
  - REQ-TASKS-PLAN-READ-003
acceptance_criteria:
  - AC-TASKS-PLAN-READ-001.1
  - AC-TASKS-PLAN-READ-001.2
  - AC-TASKS-PLAN-READ-001.3
  - AC-TASKS-PLAN-READ-001.4
  - AC-TASKS-PLAN-READ-001.5
  - AC-TASKS-PLAN-READ-001.6
  - AC-TASKS-PLAN-READ-002.1
  - AC-TASKS-PLAN-READ-002.2
  - AC-TASKS-PLAN-READ-002.3
  - AC-TASKS-PLAN-READ-002.4
  - AC-TASKS-PLAN-READ-003.1
system_design:
  - ../../specs/tasks/system-design/plan-partial-reads.md
---

# Task 01: Bounded version-aware plan reads

## Summary

Deliver the existing MCP read tool's range and version contract end to end.
Agents receive an exact bounded fragment and coherent metadata while default
full reads, task authorization, and existing safe writes remain compatible.

## In scope

- Add presence-aware offset, limit, and version arguments and strict validation
  at both MCP and backend boundaries. Avoid permissive getters that turn an
  invalid range argument into absence or zero.
- Add service projection under the existing task lock and owner authorization.
  Preserve the snapshot helper for other consumers without nested lock acquisition.
- Produce exact content and pagination metadata without returning a full plan
  anywhere in a partial response. Bridge stable conflicts and range failures.
- TDD for defaults, zero offset, safe integer bounds, fractional/null/wrong
  types, Unicode/CRLF, EOF, beyond-EOF, empty legacy heads, missing plan, storage
  failure, oversized heads, one long line, and no mutation/event side effects.
- Add `TestPlanPartialReadMCPJourney` using the dispatcher/SQLite fixture in
  `task_plan_safe_edits_integration_test.go`. Assert read/edit preservation,
  stale and ambiguous edit rejection, content-free conflicts, and intervening
  title/content/delete-recreate writes during pagination.
- Assert profile exposure remains unchanged and no backend response exposes
  content, counts, or version to unauthorized callers.

## Out of scope

- Agent system prompt adoption and public page updates (Task 02).
- Write schema changes, new database state, and browser DTO changes.

## Acceptance

1. Bounded reads and metadata satisfy every linked `001` criterion through the
   real MCP bridge, while no-range reads keep their exact existing payload.
2. Version-pinned pages and exact edits reject stale state and preserve owner
   authorization, history, comments, markers, and existing exposure rules.
3. Focused tests pass, including existing exact-edit/full-read journeys; neither
   rejected nor successful reads publish mutation events or write state.

## Verification

Run from the repository root after implementing the named tests:

```bash
(cd apps/backend && go test -trimpath ./internal/task/service -run 'PlanPartialRead|ExactEdit|AgentReplace|AgentAppend' -count=1)
(cd apps/backend && go test -trimpath ./internal/mcp/server ./internal/mcp/handlers ./internal/task/planws -run 'PlanPartialRead|PlanSafeEditsMCPJourney|PlanReadReturnsMetadataAndExactContentBlocks|MCPPlanExactEdit|GetTaskPlan|GetError|PlanSafety|PlanTools_Descriptions' -count=1)
git diff --check
```

Name new service/handler/schema/forwarding tests with `PlanPartialRead` so these
commands execute every added test. Add targeted read error tests under the same
name in `planws`; retain existing profile assertions in schema tests.

## Files likely touched

- `apps/backend/internal/mcp/server/server.go`
- `apps/backend/internal/mcp/server/handlers.go`
- `apps/backend/internal/mcp/server/task_plan_partial_read_test.go` (new)
- `apps/backend/internal/mcp/server/task_plan_partial_read_integration_test.go` (new)
- `apps/backend/internal/mcp/server/handlers_test.go` (wire mock shape)
- `apps/backend/internal/mcp/handlers/handlers.go`
- `apps/backend/internal/mcp/handlers/task_plan_guard.go`
- `apps/backend/internal/mcp/handlers/task_plan_partial_read_test.go` (new)
- `apps/backend/internal/task/service/plan_service.go`
- `apps/backend/internal/task/contract/plan_read.go` (new shared options/parser)
- `apps/backend/internal/task/service/plan_partial_read.go` (new)
- `apps/backend/internal/task/service/plan_partial_read_test.go` (new)
- `apps/backend/internal/task/planws/errors.go`
- `apps/backend/internal/task/planws/errors_test.go`
- `apps/backend/internal/task/planws/plan_partial_read_test.go` (new)

## Dependencies

None. Reuse existing conditional-write versions and safe-edit integration
fixtures; no migration or shared schema generation is needed.

## Risks

- JSON number conversion and arithmetic must not overflow or drop presence.
- Changing shared read error mapping must not expose authorized-state details
  to callers rejected before the snapshot read.
- Successful read metadata must describe the projected row, including when a
  later write occurs before the response is consumed.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/plan-partial-reads.md)
- [Design](../../specs/tasks/system-design/plan-partial-reads.md), read contract,
  snapshot and authorization, range projection, and read/edit flow.
- Existing `GetPlanSnapshot`, `planReadPayload`, `getTaskPlanHandler`, and
  `TestPlanSafeEditsMCPJourney`.
- `apps/backend/AGENTS.md`.

## Results

Implemented the range projection, shared strict options parser, discovery
schema, forwarding, and typed error bridge. Existing get-tool mock assertions
now accept the mixed string/integer payload without changing wire behavior.

- Both verification commands above passed (including existing full-read,
  cross-task addressing, exact-edit, and description compatibility tests).
- The same four service/MCP/error packages passed with `-trimpath -race` and
  the focused range/safe-edit test selection.
- `git diff --check` passed.
- Initial RED reproduced full-plan output for range requests, ignored read
  versions, and absent discovery fields before the implementation.

No storage migration, browser DTO change, or new write mode was needed.
