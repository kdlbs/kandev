---
id: "01-selection-contract"
title: "Persist native automation selections"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-OFFICE-AUTOMATION-MCP-001
  - REQ-OFFICE-AUTOMATION-MCP-002
acceptance_criteria:
  - AC-OFFICE-AUTOMATION-MCP-001.1
  - AC-OFFICE-AUTOMATION-MCP-001.3
  - AC-OFFICE-AUTOMATION-MCP-001.4
  - AC-OFFICE-AUTOMATION-MCP-001.5
  - AC-OFFICE-AUTOMATION-MCP-001.6
  - AC-OFFICE-AUTOMATION-MCP-002.3
system_design:
  - ../../specs/office/system-design/automation-mcp-selection.md
---

# Task 01: Persist native automation selections

## Summary

Persist a typed automation selection and expose authorized native read/write/options operations.
This is the independent persistence and API boundary. It does not enable new runtime tools yet.

## In scope

Models, SQLite/PostgreSQL migration, service validation, native intent, conditional writes, options projections, mode changes, and export omission.
Reject MCP/plugin selection writes and preserve unrelated updates. Provide a narrow launch reader with validated durable ownership.

## Out of scope

Runtime package injection, external delivery, and rendered UI.

## Acceptance

- Selection saves are atomic, revision-checked, native-only, workspace-authorized, and preserve omission semantics.
- Old rows and imports retain defaults. All projections and exports handle the new fields explicitly.
- Options use shared descriptors and safe catalog summaries without connecting to endpoints.

## Verification

Run from the repository root. Use `/tdd` for meaningful logic tests.
New test paths named by this work order must exist before its verification completes.

Provision an isolated test PostgreSQL service and set `KANDEV_TEST_POSTGRES_DSN` before the database command. Do not use production data.

```bash
(cd apps/backend && go test -tags fts5 -trimpath ./internal/automation -run 'AutomationMCP|AutomationPluginTools' -count=1)
(cd apps/backend && go test -tags fts5 -trimpath ./internal/mcp/handlers ./internal/mcp/server -run 'ConfigAutomation|AutomationMCP' -count=1)
(cd apps/backend && test -n "$KANDEV_TEST_POSTGRES_DSN" && go test -tags fts5 -trimpath ./internal/automation -run 'AutomationMCP.*Postgres|AutomationPluginToolsPostgres' -count=1)
git diff --check
```

Run `bash scripts/lint-go-changed` with the changed Go file paths. Record the actual expanded command and result.

## Files likely touched

- `apps/backend/internal/automation/models.go`
- `apps/backend/internal/automation/store.go`
- `apps/backend/internal/automation/service.go`
- `apps/backend/internal/automation/handlers.go`
- `apps/backend/internal/automation/plugin_tools.go`
- `apps/backend/internal/automation/export*.go`
- `apps/backend/internal/backendapp/`
- `apps/backend/pkg/websocket/`
- `apps/backend/internal/automation/mcp_selection_test.go (new)`
- `apps/backend/internal/automation/mcp_selection_postgres_test.go (new)`

## Dependencies

None.

## Risks

Keep the exact committed selection response. A later reread can mislabel another writer’s update.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/office/requirements/automation-mcp-selection.md).
- [System design](../../specs/office/system-design/automation-mcp-selection.md).
- [Authority decision](../../decisions/2026-11-04-automation-mcp-selection.md).
- Existing plugin-tool save tests, built-in package binding tests, and workspace resolver tests.

## Results

Selection models, migrations, conditional saves, safe options, export omission, and the durable launch reader are implemented.

- The combined command and results in [the plan](plan.md#verification-results) cover this work order. Automation tests include native-only writes, positive native controls, invalid and stale choices, exact revisions, durable ownership, safe option summaries, and portable export omission.
- `go test -tags fts5 -trimpath ./internal/automation ./internal/agent/settings/store -count=1`, with the isolated PostgreSQL DSN: passed after correcting the UTF8 and secret-resolution test fixtures.
- `go test -tags fts5 -trimpath ./internal/automation -run AutomationMCP -count=1`, with PostgreSQL enabled: passed after the final PostgreSQL reference-count assertions.
- Real PostgreSQL migration replay, stale saves, omission preservation, and reference counts passed. No PostgreSQL tests were skipped.
- Native Configuration grant-denial coverage lives in `internal/automation/mcp_selection_test.go`, where the actual service and shared dispatcher can enforce the save boundary.
- Changed-Go lint is recorded centrally in the plan.

### PR remediation

PR remediation verifies specific ownership and options errors, limits repository lookups to 100 IDs, and counts durable definition references without write-time package validation. The expanded PostgreSQL-backed command and results are in the plan.
