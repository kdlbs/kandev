---
id: "03-workspace-delivery"
title: "Deliver workspace servers and task templates"
status: done
wave: 3
depends_on:
  - "02-native-packages"
plan: "plan.md"
requirements:
  - REQ-OFFICE-AUTOMATION-MCP-001
  - REQ-OFFICE-AUTOMATION-MCP-003
acceptance_criteria:
  - AC-OFFICE-AUTOMATION-MCP-001.2
  - AC-OFFICE-AUTOMATION-MCP-001.3
  - AC-OFFICE-AUTOMATION-MCP-001.4
  - AC-OFFICE-AUTOMATION-MCP-001.5
  - AC-OFFICE-AUTOMATION-MCP-003.1
  - AC-OFFICE-AUTOMATION-MCP-003.2
  - AC-OFFICE-AUTOMATION-MCP-003.3
  - AC-OFFICE-AUTOMATION-MCP-003.4
  - AC-OFFICE-AUTOMATION-MCP-003.5
system_design:
  - ../../specs/office/system-design/automation-mcp-selection.md
---

# Task 03: Deliver workspace servers and task templates

## Summary

Compose automation-selected workspace MCP servers through the existing resolver.
Apply normal-task templates before first start without overwriting later task-owned choices.

## In scope

Add backend-only resolution input and provenance. Update transactional and fallback resolver paths and safe accepted contribution metadata.
Cover secrets, transports, collisions, catalog usage, guarded deletion and retained unavailable references, private process lifecycle, normal-task creation, and retry idempotence.

## Out of scope

A new transport/client, provider integration, external-service sandbox, or UI.

## Acceptance

- The effective server set is the deduplicated union with existing inheritance, with accurate origin and unavailable decisions.
- Private retained processes preserve accepted direct additions. Fresh starts read current choices. Recovery cannot accept foreign binding data.
- Normal task creation applies the template before launch. Partial application prevents start, retries remain idempotent, and reused tasks keep human edits.

## Verification

Run from the repository root. Use `/tdd` for meaningful logic tests.
New test paths named by this work order must exist before its verification completes.

Provision an isolated test PostgreSQL service and set `KANDEV_TEST_POSTGRES_DSN` before the database command. Do not use production data.

```bash
(cd apps/backend && go test -tags fts5 -trimpath ./internal/agent/mcpconfig ./internal/agent/settings/store -run 'AutomationMCP|MCPResolution|MCPSelection|MCPDelete' -count=1)
(cd apps/backend && go test -tags fts5 -trimpath ./internal/orchestrator ./internal/orchestrator/executor ./internal/agent/runtime/lifecycle -run 'AutomationMCP|Automation.*Retry|Automation.*Continuation|MCP.*Setup' -count=1)
(cd apps/backend && test -n "$KANDEV_TEST_POSTGRES_DSN" && go test -tags fts5 -trimpath ./internal/agent/settings/store ./internal/automation -run 'Postgres.*AutomationMCP|AutomationMCP.*Postgres|PostgresMCPResolution|PostgresConfirmedMCPDelete' -count=1)
git diff --check
```

Run `bash scripts/lint-go-changed` with the changed Go file paths. Record the actual expanded command and result.

## Files likely touched

- `apps/backend/internal/agent/mcpconfig/resolve_effective.go`
- `apps/backend/internal/agent/mcpconfig/catalog_usage.go`
- `apps/backend/internal/agent/settings/store/mcp_resolution_inputs.go`
- `apps/backend/internal/agent/runtime/lifecycle/mcp_accepted_setup.go`
- `apps/backend/internal/orchestrator/event_handlers_automation.go`
- `apps/backend/internal/backendapp/mcp_selection.go`
- `apps/backend/internal/automation/`
- `apps/backend/internal/agent/mcpconfig/automation_selection_test.go (new)`
- `apps/backend/internal/orchestrator/automation_mcp_selection_test.go (new)`

## Dependencies

02-native-packages.

## Risks

Update both transactional and fallback resolution. Do not apply a newly saved automation contribution during a same-process reconnect.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/office/requirements/automation-mcp-selection.md).
- [System design](../../specs/office/system-design/automation-mcp-selection.md).
- [Authority decision](../../decisions/2026-11-04-automation-mcp-selection.md).
- Existing plugin-tool save tests, built-in package binding tests, and workspace resolver tests.

## Results

Automation origins compose through both resolver paths. Launch metadata captures the direct contribution. Normal-task creation forwards validated package preferences and direct servers before start.

- The combined backend command in [the plan](plan.md#verification-results) passed all resolver, retry, continuation, and setup cases with no skips.
- SQLite and real PostgreSQL automation resolution passed. PostgreSQL snapshot and confirmed-deletion checks passed. Secret resolution checked the definition's secret ID and workspace at the delivery boundary.
- Catalog deletion counts automation references and requires confirmation. Confirmed deletion leaves an unavailable reference for explicit native removal. This avoids partial cross-store cleanup and is reflected in the design.
- The normal-task browser scenario checked first-process acceptance and changed task preferences through the native API. It reused the task and proved that the template preserved the task's choices.
- Race checks for automation and executor selections passed.
- Changed-Go lint is recorded centrally in the plan.

### PR remediation

PR remediation verifies durable templates for retry snapshots, exact package/revision content at task creation, batched workspace usage reads, and the route-owned HTTP deletion guard. Forced deletion is followed by a missing-definition assertion. The expanded backend command and results are in the plan.
