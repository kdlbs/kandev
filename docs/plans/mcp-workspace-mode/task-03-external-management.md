---
id: "03-external-management"
title: "Preserve external task management across modes"
status: done
wave: 3
depends_on: 
  - "02-office-creation"
plan: "plan.md"
requirements:
  - REQ-TASKS-MCP-WORKSPACE-MODE-003
acceptance_criteria:
  - AC-TASKS-MCP-WORKSPACE-MODE-003.1
  - AC-TASKS-MCP-WORKSPACE-MODE-003.2
  - AC-TASKS-MCP-WORKSPACE-MODE-003.3
  - AC-TASKS-MCP-WORKSPACE-MODE-003.4
  - AC-TASKS-MCP-WORKSPACE-MODE-003.5
system_design:
  - ../../specs/tasks/system-design/mcp-workspace-mode.md
---

# Task 03: Preserve external task management across modes

## Summary

Verify external creation and management across both workspace modes through existing tools, and document the caller boundary.

## In scope

- Add isolated transport/composition tests for both modes, unchanged creation schemas, management access, foreign resources, and auth-disabled admission.
- Update the public MCP reference and affected specification links. No tool-name migration or Office creation tool is needed.

## Out of scope

New Office MCP tools, native Office lifecycle changes, UI, migrations, and
unrelated permission or management redesign.

## Acceptance

1. External create_task_kandev can reach authorized Kanban and Office workspaces; session restrictions do not leak into external calls.
2. Existing external list/read/move/state/archive/delete operations retain authorization and lifecycle behavior.
3. Docs describe Kanban MCP, Office skills/CLI, and external MCP accurately.

## Verification

Run from the repository root:

```bash
(cd apps/backend && go test ./internal/mcp/... ./internal/backendapp -count=1)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- apps/backend/internal/backendapp/mcp_workspace_mode_test.go (new)
- apps/backend/internal/mcp/server/external_integration_test.go
- docs/public/automation-and-mcp.md
- docs/specs/integrations/requirements/external-mcp.md (only if clarification is needed)
- This plan and its paired specifications

## Dependencies

02-office-creation.

## Risks

Do not expand external assignment, launch, or management semantics while testing compatibility.

## Parallelism

`sequential`

## Inputs

- [System design](../../specs/tasks/system-design/mcp-workspace-mode.md).
- Existing MCP principal, catalog, external transport, and Office runtime tests.
- Scoped backend guidance; use TDD for implementation changes.

## Results

Verified that trusted external creation reaches authorized Kanban and Office
workspaces through the existing `create_task_kandev` contract, including
external-ID retries that return either mode. The isolated
`backendapp.TestExternalMCPTaskModesReachPersistenceAndManagement` composition
test drives HTTP JSON-RPC through the real external server, dispatcher, handler,
and SQLite persistence, then lists and state-updates both tasks. The handler
suite also rejects fabricated session provenance, selects the sole writable
workspace when readable workspaces are also visible, requires an explicit
workspace when multiple writable destinations are available, and hides legacy
mixed-mode Office tasks from Kanban retries. Existing external discovery,
lifecycle, conversation, session, question, and management handlers remain on
the same surface. The isolated composition test covers list/read, state update,
move, archive, and delete for each mode. Public documentation now distinguishes
session MCP, Office skills and CLI, external MCP, and materialized-workspace
policy.

Verification:

- `go test ./internal/mcp/handlers -count=1`: 644 passed.
- `go test ./internal/mcp/server ./internal/backendapp ./internal/integration -count=1`: 1,382 passed, including the real external HTTP composition test and MCP WebSocket creation scenarios with trusted session identity.
- `node --test scripts/validate-public-docs.test.mjs`: 62 passed.
- `node scripts/validate-public-docs.mjs`: 46 pages validated.
- `python3 scripts/lint-spec-files.test.py`: 36 passed.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check`: passed.
