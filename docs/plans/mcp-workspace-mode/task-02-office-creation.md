---
id: "02-office-creation"
title: "Preserve Office CLI creation"
status: done
wave: 2
depends_on: 
  - "01-creation-admission"
plan: "plan.md"
requirements:
  - REQ-TASKS-MCP-WORKSPACE-MODE-001
  - REQ-TASKS-MCP-WORKSPACE-MODE-002
acceptance_criteria:
  - AC-TASKS-MCP-WORKSPACE-MODE-001.2
  - AC-TASKS-MCP-WORKSPACE-MODE-002.1
  - AC-TASKS-MCP-WORKSPACE-MODE-002.3
  - AC-TASKS-MCP-WORKSPACE-MODE-002.4
  - AC-TASKS-MCP-WORKSPACE-MODE-002.5
system_design:
  - ../../specs/tasks/system-design/mcp-workspace-mode.md
---

# Task 02: Preserve Office CLI creation

## Summary

Verify that Office sessions expose no task-creation MCP tool and continue using existing skills and CLI.

## In scope

- Assert exact Office catalog membership and rejection of direct generic MCP creation, for Kanban and Office targets.
- Check existing Office skill references and run existing runtime creation tests. Do not change native creation behavior.

## Out of scope

New Office MCP tools, native Office lifecycle changes, UI, migrations, and
unrelated permission or management redesign.

## Acceptance

1. Office discovery contains neither generic task creation nor a new Office creation tool; direct creation actions are denied.
2. Office CLI runtime creation, parent/project scope, and capability tests remain green.

## Verification

Run from the repository root:

```bash
(cd apps/backend && go test ./internal/mcp/server ./internal/mcp/handlers ./internal/office/runtime -count=1)
git diff --check
```

## Files likely touched

- apps/backend/internal/mcp/server/server_test.go or a focused new Office catalog test file
- apps/backend/internal/mcp/handlers/create_task_mode_test.go
- apps/backend/internal/office/configloader/skills/kandev-task-ops/ (read-only evidence)
- apps/backend/internal/office/runtime/actions.go and existing tests (regression inputs)

## Dependencies

01-creation-admission.

## Risks

Do not turn a discovery restriction into an Office CLI restriction by placing the guard in shared native task services.

## Parallelism

`sequential`

## Inputs

- [System design](../../specs/tasks/system-design/mcp-workspace-mode.md).
- Existing MCP principal, catalog, external transport, and Office runtime tests.
- Scoped backend guidance; use TDD for implementation changes.

## Results

Verified that Office discovery contains neither `create_task_kandev` nor a
new Office creation tool. Direct generic MCP creation from an Office principal
is denied with the skills and runtime CLI guidance. Existing Office runtime
creation and capability tests remain unchanged and green.

Verification:

- `go test ./internal/mcp/server ./internal/mcp/handlers ./internal/office/runtime -count=1`: passed.
- `go test ./internal/mcp/... ./internal/backendapp -count=1`: 1,990 passed.
- `git diff --check`: passed.
