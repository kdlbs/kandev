---
id: "01-creation-admission"
title: "Enforce MCP creation admission"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-MCP-WORKSPACE-MODE-001
  - REQ-TASKS-MCP-WORKSPACE-MODE-002
  - REQ-TASKS-MCP-WORKSPACE-MODE-003
acceptance_criteria:
  - AC-TASKS-MCP-WORKSPACE-MODE-001.1
  - AC-TASKS-MCP-WORKSPACE-MODE-001.3
  - AC-TASKS-MCP-WORKSPACE-MODE-001.4
  - AC-TASKS-MCP-WORKSPACE-MODE-001.5
  - AC-TASKS-MCP-WORKSPACE-MODE-001.6
  - AC-TASKS-MCP-WORKSPACE-MODE-002.2
  - AC-TASKS-MCP-WORKSPACE-MODE-003.4
system_design:
  - ../../specs/tasks/system-design/mcp-workspace-mode.md
---

# Task 01: Enforce MCP creation admission

## Summary

Add trusted caller and destination validation before generic MCP creation. Preserve existing valid Kanban behavior.

## In scope

- Use TDD for principal/external identity, wrong-mode targets, defaults, foreign references, retries, and zero side effects.
- Preserve source attribution, automation policy, profile inheritance, deferred launch, and existing external tool schemas.

## Out of scope

New Office MCP tools, native Office lifecycle changes, UI, migrations, and
unrelated permission or management redesign.

## Acceptance

1. Untrusted and Office session calls fail before repository registration, task data exposure, insertion, or launch.
2. Valid Kanban requests retain existing behavior; the external marker preserves both destination modes.

## Verification

Run from the repository root:

```bash
(cd apps/backend && go test ./internal/mcp/handlers ./internal/mcp/scope ./internal/mcp/origin ./internal/mcp/server -count=1)
git diff --check
```

## Files likely touched

- apps/backend/internal/mcp/handlers/handlers.go
- apps/backend/internal/mcp/handlers/create_task_mode.go (new)
- apps/backend/internal/mcp/handlers/create_task_mode_test.go (new)
- apps/backend/internal/mcp/server/handlers.go and focused tests

## Dependencies

None.

## Risks

Identity-free test fixtures must be updated to represent actual callers, without a production bypass.

## Parallelism

`sequential`

## Inputs

- [System design](../../specs/tasks/system-design/mcp-workspace-mode.md).
- Existing MCP principal, catalog, external transport, and Office runtime tests.
- Scoped backend guidance; use TDD for implementation changes.

## Results

Implemented trusted creation admission in `handleCreateTask` before repository
resolution. The gate distinguishes trusted Kanban session, Office session,
automation, and external transport callers; validates parent, workspace,
workflow, mode, and caller identity; and prevents cross-workspace source
repository inheritance for Kanban roots. Existing profile, deferred launch,
external-ID, and automation behavior remains covered by the handler suite.
Verified Kanban principals now bind omitted source task/session IDs before
profile/runtime resolution, preserving creator-session inheritance and genesis
ledger attribution. Found external-ID results are revalidated for workspace
visibility and `IsFromOffice` before serialization, including legacy mixed-mode
Office collisions in both settled and unsettled states.

The cross-workspace identity regression now scopes both the owner identity and
the resolver-derived task/session principal, with a same-user creation control.

Verification:

- `go test ./internal/mcp/handlers -count=1`: 644 passed.
- `go test ./internal/mcp/server ./internal/backendapp ./internal/integration -count=1`: 1,382 passed.
- `git diff --check`: passed.
